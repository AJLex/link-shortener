package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

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
	_ = storage.load()
	return storage
}

// Реализация методов интерфейса Storage

func (s *FileStorage) Save(shortURL, originalURL string) (string, error) {
	entry := models.URLEntry{
		UUID:        generateID(),
		ShortURL:    shortURL,
		OriginalURL: originalURL,
	}
	return "", s.saveEntry(entry)
}

func (s *FileStorage) Get(shortURL string) (string, error) {
	entry, err := s.findByShortURL(shortURL)
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", fmt.Errorf("URL not found")
	}
	return entry.OriginalURL, nil
}

func (s *FileStorage) GetAll() (map[string]string, error) {
	entries := s.getEntries()
	result := make(map[string]string)
	for _, entry := range entries {
		result[entry.ShortURL] = entry.OriginalURL
	}
	return result, nil
}

func (s *FileStorage) Ping(ctx context.Context) error {
	// Для файлового хранилища всегда доступно
	return nil
}

func (s *FileStorage) Close() error {
	// Для файлового хранилища не нужно закрывать ресурсы
	return nil
}

func (s *FileStorage) load() error {
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

func (s *FileStorage) saveEntry(entry models.URLEntry) error {
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
func (s *FileStorage) getEntries() []models.URLEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Возвращаем копию, чтобы избежать гонок данных
	entries := make([]models.URLEntry, len(s.entries))
	copy(entries, s.entries)
	return entries
}

func (s *FileStorage) findByShortURL(shortURL string) (*models.URLEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.entries {
		if entry.ShortURL == shortURL {
			return &entry, nil
		}
	}
	return nil, nil // не найдено - не ошибка
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *FileStorage) SaveBatch(entries map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for shortURL, originalURL := range entries {
		entry := models.URLEntry{
			UUID:        generateID(),
			ShortURL:    shortURL,
			OriginalURL: originalURL,
		}
		s.entries = append(s.entries, entry)
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
