package db

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

var mysqlDB *sql.DB

// InitMySQL initializes MySQL connection using env DSN
func InitMySQL() {
	var err error
	dsn := os.Getenv("MYSQL_DSN") // Expect DSN in env
	mysqlDB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("❌ Could not connect to MySQL: %v", err)
	}
}

func GetMySQL() *sql.DB {
	return mysqlDB
}
