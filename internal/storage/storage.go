package storage

import (
	"context"
)

// Storage общий интерфейс для всех типов хранилищ
type Storage interface {
	// Save сохраняет связь shortURL -> originalURL
	Save(shortURL, originalURL string) error
	// SaveBatch сохраняет связь shortURL -> originalURL побатчево
	SaveBatch(entries map[string]string) error
	// Get возвращает originalURL по shortURL
	Get(shortURL string) (string, error)
	// GetAll возвращает все записи для загрузки в память
	GetAll() (map[string]string, error)
	// Ping проверяет доступность хранилища
	Ping(ctx context.Context) error
	// Close освобождает ресурсы
	Close() error
}
