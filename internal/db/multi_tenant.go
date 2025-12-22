package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type ClientDB struct {
	mu         sync.RWMutex
	mysqlPools map[string]*sql.DB
	chPools    map[string]clickhouse.Conn
}

var (
	clientDBInstance *ClientDB
	once             sync.Once
)

// ✅ Create singleton ClientDB instance
func NewClientDB() *ClientDB {
	once.Do(func() {
		clientDBInstance = &ClientDB{
			mysqlPools: make(map[string]*sql.DB),
			chPools:    make(map[string]clickhouse.Conn),
		}
	})
	return clientDBInstance
}

func (c *ClientDB) GetMysqlDB(clientID, projectID string) (*sql.DB, error) {
	key := fmt.Sprintf("%s_%s", clientID, projectID)

	// ✅ Return cached connection if exists
	c.mu.RLock()
	db, exists := c.mysqlPools[key]
	c.mu.RUnlock()
	if exists {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Remove broken connection
		c.mu.Lock()
		delete(c.mysqlPools, key)
		c.mu.Unlock()
	}

	dbConfig, err := fetchMysqlDBConfigFromMasterTable(clientID, projectID)
	if err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		dbConfig.User, dbConfig.Password, dbConfig.Host, dbConfig.Database)

	newDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	newDB.SetMaxOpenConns(10)
	newDB.SetMaxIdleConns(5)
	newDB.SetConnMaxLifetime(time.Hour)

	if err := newDB.Ping(); err != nil {
		return nil, fmt.Errorf("MySQL ping failed: %w", err)
	}

	c.mu.Lock()
	c.mysqlPools[key] = newDB
	c.mu.Unlock()

	return newDB, nil
}

func (c *ClientDB) GetCHDB(clientID, projectID string) (clickhouse.Conn, error) {
	key := fmt.Sprintf("%s_%s", clientID, projectID)

	// ✅ Return cached & live connection if available
	c.mu.RLock()
	db, exists := c.chPools[key]
	c.mu.RUnlock()
	if exists {
		if err := db.Ping(context.Background()); err == nil {
			return db, nil
		}
		// Remove broken connection
		c.mu.Lock()
		delete(c.chPools, key)
		c.mu.Unlock()
	}

	dbConfig, err := fetchClickhouseDBConfigFromMasterTable(clientID, projectID)
	if err != nil {
		return nil, err
	}

	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", dbConfig.Host, 9000)},
		Auth: clickhouse.Auth{
			Database: dbConfig.Database,
			Username: dbConfig.User,
			Password: dbConfig.Password,
		},
		Settings: map[string]interface{}{
			"max_execution_time": 60,
		},
		DialTimeout:      10 * time.Second,
		MaxOpenConns:     10,
		MaxIdleConns:     5,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
	}

	chConn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open ClickHouse connection: %w", err)
	}

	if err := chConn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ClickHouse ping failed: %w", err)
	}

	c.mu.Lock()
	c.chPools[key] = chConn
	c.mu.Unlock()

	return chConn, nil
}

// ✅ Graceful shutdown for all pools
func (c *ClientDB) CloseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, conn := range c.mysqlPools {
		conn.Close()
		delete(c.mysqlPools, key)
	}

	for key, conn := range c.chPools {
		conn.Close()
		delete(c.chPools, key)
	}
}

// MySQL DB config
type MySQLConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// ClickHouse DB config
type CHConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

func fetchMysqlDBConfigFromMasterTable(clientID, projectID string) (*MySQLConfig, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Could not load .env file, continuing...")
	}
	masterDSN := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASS"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_DB"),
	)

	masterDB, err := sql.Open("mysql", masterDSN)
	if err != nil {
		return nil, err
	}
	defer masterDB.Close()

	var dbcfg MySQLConfig
	err = masterDB.QueryRow(`
		SELECT host, database_name, user, password
		FROM master_database_credentials
		WHERE client_id=? AND project_id=? AND driver = ?`,
		clientID, projectID, "mysql").
		Scan(&dbcfg.Host, &dbcfg.Database, &dbcfg.User, &dbcfg.Password)
	if err != nil {
		return nil, err
	}

	return &dbcfg, nil
}

func fetchClickhouseDBConfigFromMasterTable(clientID, projectID string) (*CHConfig, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Could not load .env file, continuing...")
	}
	masterDSN := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASS"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_DB"),
	)

	masterDB, err := sql.Open("mysql", masterDSN)
	if err != nil {
		return nil, err
	}
	defer masterDB.Close()

	var dbcfg CHConfig
	err = masterDB.QueryRow(`
		SELECT host, database_name, user, password, port
		FROM master_database_credentials
		WHERE client_id=? AND project_id=? AND driver = ?`,
		clientID, projectID, "clickhouse").
		Scan(&dbcfg.Host, &dbcfg.Database, &dbcfg.User, &dbcfg.Password, &dbcfg.Port)
	if err != nil {
		return nil, err
	}

	return &dbcfg, nil
}

func RowsToMap(rows *sql.Rows) ([]map[string]interface{}, error) {
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}
