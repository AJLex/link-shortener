package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	models "github.com/AJLex/link-shortener/internal/model"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"
)

var ErrExists = errors.New("exists")

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

func (p *PostgresStorage) Save(shortURL, originalURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var resultShortURL string
	err := p.db.QueryRowContext(ctx, `
        INSERT INTO urls (short_code, original_url) 
        VALUES ($1, $2)
        ON CONFLICT (original_url)
        DO NOTHING
        RETURNING short_code
    `, shortURL, originalURL).Scan(&resultShortURL)

	if err == sql.ErrNoRows {
		// Запись уже существует, получаем существующий short_code
		err = p.db.QueryRowContext(ctx,
			"SELECT short_code FROM urls WHERE original_url = $1",
			originalURL,
		).Scan(&resultShortURL)
		if err != nil {
			return "", err
		}
		return resultShortURL, ErrExists
	}

	if err != nil {
		return "", err
	}

	return resultShortURL, nil
}

func (p *PostgresStorage) SaveWithUser(shortURL, originalURL, userID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var resultShortURL string
	err := p.db.QueryRowContext(ctx, `
        INSERT INTO urls (short_code, original_url, user_id) 
        VALUES ($1, $2, $3)
        ON CONFLICT (original_url)
        DO NOTHING
        RETURNING short_code
    `, shortURL, originalURL, userID).Scan(&resultShortURL)

	if err == sql.ErrNoRows {
		// Запись уже существует, получаем существующий short_code
		err = p.db.QueryRowContext(ctx,
			"SELECT short_code FROM urls WHERE original_url = $1",
			originalURL,
		).Scan(&resultShortURL)
		if err != nil {
			return "", err
		}
		return resultShortURL, ErrExists
	}

	if err != nil {
		return "", err
	}

	return resultShortURL, nil
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

func (p *PostgresStorage) SaveBatchWithUser(entries map[string]string, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO urls (short_code, original_url, user_id) VALUES ($1, $2, $3)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for shortCode, originalURL := range entries {
		if _, err := stmt.ExecContext(ctx, shortCode, originalURL, userID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (p *PostgresStorage) GetByUser(userID string) ([]models.UserURL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT short_code, original_url FROM urls WHERE user_id = $1 AND is_deleted = FALSE`
	rows, err := p.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.UserURL
	for rows.Next() {
		var shortCode, originalURL string
		if err := rows.Scan(&shortCode, &originalURL); err != nil {
			return nil, err
		}
		result = append(result, models.UserURL{
			ShortURL:    shortCode,
			OriginalURL: originalURL,
		})
	}

	return result, rows.Err()
}

// GetWithDeletedFlag возвращает originalURL и флаг удаления по shortURL
func (p *PostgresStorage) GetWithDeletedFlag(shortURL string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var originalURL string
	var isDeleted bool
	query := `SELECT original_url, is_deleted FROM urls WHERE short_code = $1`
	err := p.db.QueryRowContext(ctx, query, shortURL).Scan(&originalURL, &isDeleted)

	if err != nil {
		return "", false, err
	}

	return originalURL, isDeleted, nil
}

// DeleteBatch помечает URL как удалённые (batch update)
func (p *PostgresStorage) DeleteBatch(ctx context.Context, shortCodes []string, userID string) error {
	if len(shortCodes) == 0 {
		return nil
	}

	// Используем pq.Array для передачи массива в PostgreSQL
	query := `
		UPDATE urls 
		SET is_deleted = TRUE 
		WHERE short_code = ANY($1)
		  AND user_id = $2
		  AND is_deleted = FALSE
	`

	_, err := p.db.ExecContext(ctx, query, pq.Array(shortCodes), userID)
	return err
}
