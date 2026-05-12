package db

import (
	"context"
	"os"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func InitDB() error {
	connString := os.Getenv("POSTGRES_URL")
	if connString == "" {
		connString = "postgres://recon:recon@localhost:5432/recon?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return err
	}
	Pool = pool
	return nil
}