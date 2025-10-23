package storage

import (
	"encoding/json"
	"os"
	"sync"

	models "github.com/AJLex/link-shortener/internal/model"
)

type FileStorage struct {
	filePath string
	entries  []models.URLEntry
	mu       sync.RWMutex
}

func NewFileStorage(filePath string) *FileStorage {
	storage := &FileStorage{
		filePath: filePath,
		entries:  []models.URLEntry{},
	}
	// Автоматически загружаем данные при создании
	_ = storage.Load()
	return storage
}

func (s *FileStorage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Файл не существует - нормальная ситуация
		}
		return err
	}

	return json.Unmarshal(data, &s.entries)
}

func (s *FileStorage) Save(entry models.URLEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, entry)

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// GetEntries возвращает копию всех записей
func (s *FileStorage) GetEntries() []models.URLEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Возвращаем копию, чтобы избежать гонок данных
	entries := make([]models.URLEntry, len(s.entries))
	copy(entries, s.entries)
	return entries
}

func (s *FileStorage) FindByShortURL(shortURL string) (*models.URLEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.entries {
		if entry.ShortURL == shortURL {
			return &entry, nil
		}
	}
	return nil, nil // не найдено - не ошибка
}
