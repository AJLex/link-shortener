package storage

import (
	"context"

	models "github.com/AJLex/link-shortener/internal/model"
)

// Storage общий интерфейс для всех типов хранилищ
type Storage interface {
	// Save сохраняет связь shortURL -> originalURL
	// В случае БД, если уже есть запись с originalURL, то вернется shortURL
	Save(shortURL, originalURL string) (string, error)
	// SaveWithUser сохраняет связь shortURL -> originalURL с привязкой к пользователю
	SaveWithUser(shortURL, originalURL, userID string) (string, error)
	// SaveBatch сохраняет связь shortURL -> originalURL побатчево
	SaveBatch(entries map[string]string) error
	// SaveBatchWithUser сохраняет связь shortURL -> originalURL побатчево с привязкой к пользователю
	SaveBatchWithUser(entries map[string]string, userID string) error
	// Get возвращает originalURL по shortURL
	Get(shortURL string) (string, error)
	// GetAll возвращает все записи для загрузки в память
	GetAll() (map[string]string, error)
	// GetByUser возвращает все URL пользователя
	GetByUser(userID string) ([]models.UserURL, error)
	// GetWithDeletedFlag возвращает originalURL и флаг удаления по shortURL
	GetWithDeletedFlag(shortURL string) (originalURL string, isDeleted bool, err error)
	// DeleteBatch помечает URL как удалённые (batch update)
	// shortCodes - список кодов для удаления
	// userID - ID пользователя (для проверки владения)
	DeleteBatch(ctx context.Context, shortCodes []string, userID string) error
	// Ping проверяет доступность хранилища
	Ping(ctx context.Context) error
	// Close освобождает ресурсы
	Close() error
}
