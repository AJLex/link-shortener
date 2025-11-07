package storage

import (
	"context"
	"fmt"
	"maps"
	"sync"
)

type MemoryStorage struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]string),
	}
}

func (m *MemoryStorage) Save(shortURL, originalURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[shortURL] = originalURL
	return nil
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
