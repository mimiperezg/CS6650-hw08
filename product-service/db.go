package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"
	_ "github.com/go-sql-driver/mysql"
)

func openDB() (*sql.DB, error) {
	// read env variables for database connection
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	// build DSN
	// format string for user:password@tcp(host:port)/dbname?parseTime=true
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, password, host, port, dbName)
	// open database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// ping database to make sure the connection is alive
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}