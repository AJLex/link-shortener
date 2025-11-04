package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/AJLex/link-shortener/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewConnection(cfg config.Config) (*sql.DB, error) {

	db, err := sql.Open("pgx", cfg.PostgreSQLDns)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	return db, nil
}

func PingDB(db DBInterface) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := db.PingContext(ctx)

	return err
}
