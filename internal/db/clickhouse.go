package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/joho/godotenv"
)

var (
	clickDB         clickhouse.Conn
	logClickQueries bool
)

func InitClickhouse() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Could not load .env file, continuing...")
	}

	// Enable query logging if env set
	logClickQueries = os.Getenv("CLICKHOUSE_LOG_QUERIES") == "true"

	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", os.Getenv("CLICKHOUSE_HOST"), os.Getenv("CLICKHOUSE_PORT"))},
		Auth: clickhouse.Auth{
			Database: os.Getenv("CLICKHOUSE_DB"),
			Username: os.Getenv("CLICKHOUSE_USER"),
			Password: os.Getenv("CLICKHOUSE_PASS"),
		},
		DialTimeout: 5 * time.Second,
	}

	var err error
	clickDB, err = clickhouse.Open(opts)
	if err != nil {
		log.Fatalf("❌ Failed to create ClickHouse connection: %v", err)
	}

	ctx := context.Background()
	if err = clickDB.Ping(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}

	log.Println("✅ ClickHouse connection established")
}

func GetClickhouse() clickhouse.Conn {
	return clickDB
}

func IsQueryLoggingEnabled() bool {
	return logClickQueries
}
