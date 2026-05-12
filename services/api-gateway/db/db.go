package db

import (
	"context"
	"os"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func InitDB() error {
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		connString = "postgres://postgres:postgres@localhost:5432/payment_reconciliation?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return err
	}
	Pool = pool
	return nil
}