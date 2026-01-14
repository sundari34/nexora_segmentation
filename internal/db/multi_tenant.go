package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
)

/* ============================================================
   CONFIG
============================================================ */

const (
	mysqlMaxOpen    = 10
	mysqlMaxIdle    = 5
	mysqlConnLife   = time.Hour
	tenantTTL       = 30 * time.Minute
	cleanupInterval = 5 * time.Minute
)

/* ============================================================
   TYPES
============================================================ */

type pooledMySQL struct {
	db       *sql.DB
	lastUsed time.Time
}

type pooledCH struct {
	conn     clickhouse.Conn
	lastUsed time.Time
}

type ClientDB struct {
	mu         sync.RWMutex
	mysqlPools map[string]*pooledMySQL
	chPools    map[string]*pooledCH
}

/* ============================================================
   SINGLETON CLIENT DB
============================================================ */

var (
	clientDB     *ClientDB
	clientDBOnce sync.Once
)

func NewClientDB() *ClientDB {
	clientDBOnce.Do(func() {
		clientDB = &ClientDB{
			mysqlPools: make(map[string]*pooledMySQL),
			chPools:    make(map[string]*pooledCH),
		}
		go clientDB.cleanupLoop()
	})
	return clientDB
}

/* ============================================================
   MASTER MYSQL (RETRY-SAFE, NO sync.Once)
============================================================ */

var (
	masterDB *sql.DB
	masterMu sync.Mutex
)

func getMasterDB() (*sql.DB, error) {
	masterMu.Lock()
	defer masterMu.Unlock()

	// Reuse healthy connection
	if masterDB != nil {
		if err := masterDB.Ping(); err == nil {
			return masterDB, nil
		}
		masterDB.Close()
		masterDB = nil
	}

	host := os.Getenv("MYSQL_HOST")
	user := os.Getenv("MYSQL_USER")
	pass := os.Getenv("MYSQL_PASS")
	dbname := os.Getenv("MYSQL_DB")

	if host == "" || user == "" || dbname == "" {
		return nil, fmt.Errorf("MYSQL env vars not set")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		user, pass, host, dbname,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	masterDB = db
	return masterDB, nil
}

/* ============================================================
   MYSQL TENANT POOL
============================================================ */

func (c *ClientDB) GetMysqlDB(clientID, projectID string) (*sql.DB, error) {
	key := clientID + "_" + projectID

	c.mu.RLock()
	pool, ok := c.mysqlPools[key]
	c.mu.RUnlock()

	if ok {
		c.mu.Lock()
		pool.lastUsed = time.Now()
		c.mu.Unlock()
		return pool.db, nil
	}

	cfg, err := fetchMysqlDBConfig(clientID, projectID)
	if err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(mysqlMaxOpen)
	db.SetMaxIdleConns(mysqlMaxIdle)
	db.SetConnMaxLifetime(mysqlConnLife)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	c.mu.Lock()
	c.mysqlPools[key] = &pooledMySQL{db: db, lastUsed: time.Now()}
	c.mu.Unlock()

	return db, nil
}

/* ============================================================
   CLICKHOUSE TENANT POOL
============================================================ */

func (c *ClientDB) GetCHDB(clientID, projectID string) (clickhouse.Conn, error) {
	key := clientID + "_" + projectID

	c.mu.RLock()
	pool, ok := c.chPools[key]
	c.mu.RUnlock()

	if ok {
		c.mu.Lock()
		pool.lastUsed = time.Now()
		c.mu.Unlock()
		return pool.conn, nil
	}

	cfg, err := fetchClickhouseDBConfig(clientID, projectID)
	if err != nil {
		return nil, err
	}

	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
		// Protocol:        clickhouse.Native,
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(context.Background()); err != nil {
		conn.Close()
		return nil, err
	}

	c.mu.Lock()
	c.chPools[key] = &pooledCH{conn: conn, lastUsed: time.Now()}
	c.mu.Unlock()

	return conn, nil
}

/* ============================================================
   CLEANUP LOOP (TTL EVICTION)
============================================================ */

func (c *ClientDB) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		c.mu.Lock()
		for k, v := range c.mysqlPools {
			if now.Sub(v.lastUsed) > tenantTTL {
				v.db.Close()
				delete(c.mysqlPools, k)
			}
		}
		for k, v := range c.chPools {
			if now.Sub(v.lastUsed) > tenantTTL {
				v.conn.Close()
				delete(c.chPools, k)
			}
		}
		c.mu.Unlock()
	}
}

/* ============================================================
   SHUTDOWN
============================================================ */

func (c *ClientDB) CloseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.mysqlPools {
		v.db.Close()
		delete(c.mysqlPools, k)
	}

	for k, v := range c.chPools {
		v.conn.Close()
		delete(c.chPools, k)
	}

	if masterDB != nil {
		masterDB.Close()
	}
}

/* ============================================================
   CONFIG FETCHERS
============================================================ */

type MySQLConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type CHConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

func fetchMysqlDBConfig(clientID, projectID string) (*MySQLConfig, error) {
	db, err := getMasterDB()
	if err != nil {
		return nil, err
	}

	var cfg MySQLConfig
	err = db.QueryRow(`
		SELECT host, port, database_name, user, password
		FROM master_database_credentials
		WHERE client_id=? AND driver='mysql'
	`, clientID).
		Scan(&cfg.Host, &cfg.Port, &cfg.Database, &cfg.User, &cfg.Password)

	return &cfg, err
}

func fetchClickhouseDBConfig(clientID, projectID string) (*CHConfig, error) {
	db, err := getMasterDB()
	if err != nil {
		return nil, err
	}

	var cfg CHConfig
	err = db.QueryRow(`
		SELECT host, port, database_name, user, password
		FROM master_database_credentials
		WHERE client_id=? AND driver='clickhouse'
	`, clientID).
		Scan(&cfg.Host, &cfg.Port, &cfg.Database, &cfg.User, &cfg.Password)

	return &cfg, err
}
