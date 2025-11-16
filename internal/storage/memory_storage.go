package storage

import (
	"context"
	"fmt"
	"maps"
	"sync"

	models "github.com/AJLex/link-shortener/internal/model"
)

type urlEntry struct {
	originalURL string
	userID      string
}

type MemoryStorage struct {
	mu   sync.RWMutex
	data map[string]string   // shortCode -> originalURL (для обратной совместимости)
	urls map[string]urlEntry // shortCode -> urlEntry (с user_id)
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]string),
		urls: make(map[string]urlEntry),
	}
}

func (m *MemoryStorage) Save(shortURL, originalURL string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[shortURL] = originalURL
	return "", nil
}

func (m *MemoryStorage) Get(shortURL string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	originalURL, exists := m.data[shortURL]
	if !exists {
		return "", fmt.Errorf("URL not found")
	}
	return originalURL, nil
}

func (m *MemoryStorage) GetAll() (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Возвращаем копию данных
	result := make(map[string]string)
	for k, v := range m.data {
		result[k] = v
	}
	return result, nil
}

func (m *MemoryStorage) Ping(ctx context.Context) error {
	// In-memory хранилище всегда доступно
	return nil
}

func (m *MemoryStorage) Close() error {
	// Не нужно освобождать ресурсы
	return nil
}

func (m *MemoryStorage) SaveBatch(entries map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	maps.Copy(m.data, entries)
	return nil
}

func (m *MemoryStorage) SaveWithUser(shortURL, originalURL, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[shortURL] = originalURL
	m.urls[shortURL] = urlEntry{
		originalURL: originalURL,
		userID:      userID,
	}
	return "", nil
}

func (m *MemoryStorage) SaveBatchWithUser(entries map[string]string, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	maps.Copy(m.data, entries)
	for shortCode, originalURL := range entries {
		m.urls[shortCode] = urlEntry{
			originalURL: originalURL,
			userID:      userID,
		}
	}
	return nil
}

func (m *MemoryStorage) GetByUser(userID string) ([]models.UserURL, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []models.UserURL
	for shortCode, entry := range m.urls {
		if entry.userID == userID {
			result = append(result, models.UserURL{
				ShortURL:    shortCode,
				OriginalURL: entry.originalURL,
			})
		}
	}
	return result, nil
}
