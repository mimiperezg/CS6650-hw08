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

func createTables(db *sql.DB) error {
	// create carts table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS carts (
		id INT PRIMARY KEY AUTO_INCREMENT,
		customer_id INT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_customer_id (customer_id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create carts table: %w", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cart_items (
		cart_id INT NOT NULL,
		product_id INT NOT NULL,
		quantity INT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (cart_id, product_id), 
		FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE,
		CHECK (quantity > 0)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create cart_items table: %w", err)
	}

	return nil

}