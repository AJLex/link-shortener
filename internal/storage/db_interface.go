package storage

import (
	"context"
)

// DBInterface - интерфейс для методов БД, которые мы используем
type DBInterface interface {
	PingContext(ctx context.Context) error
	Close() error
}
