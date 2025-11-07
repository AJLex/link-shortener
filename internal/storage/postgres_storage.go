package storage

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStorage struct {
	db *sql.DB
}

func NewPostgresStorage(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &PostgresStorage{db: db}, nil
}

func (p *PostgresStorage) Save(shortURL, originalURL string) error {
	query := `INSERT INTO urls (short_code, original_url) VALUES ($1, $2) 
              ON CONFLICT (short_code) DO UPDATE SET original_url = EXCLUDED.original_url`
	_, err := p.db.Exec(query, shortURL, originalURL)
	return err
}

func (p *PostgresStorage) Get(shortURL string) (string, error) {
	var originalURL string
	query := `SELECT original_url FROM urls WHERE short_code = $1`
	err := p.db.QueryRow(query, shortURL).Scan(&originalURL)

	return originalURL, err
}

func (p *PostgresStorage) GetAll() (map[string]string, error) {
	query := `SELECT short_code, original_url FROM urls`
	rows, err := p.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var shortCode, originalURL string
		if err := rows.Scan(&shortCode, &originalURL); err != nil {
			return nil, err
		}
		result[shortCode] = originalURL
	}
	return result, rows.Err()
}

func (p *PostgresStorage) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *PostgresStorage) Close() error {
	return p.db.Close()
}

func (p *PostgresStorage) SaveBatch(entries map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO urls (short_code, original_url) VALUES ($1, $2)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for shortCode, originalURL := range entries {
		if _, err := stmt.ExecContext(ctx, shortCode, originalURL); err != nil {
			return err
		}
	}

	return tx.Commit()
}
