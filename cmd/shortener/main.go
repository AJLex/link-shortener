package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AJLex/link-shortener/internal/config"
	"github.com/AJLex/link-shortener/internal/gzip"
	"github.com/AJLex/link-shortener/internal/logger"
	models "github.com/AJLex/link-shortener/internal/model"
	"github.com/AJLex/link-shortener/internal/storage"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// функция main вызывается автоматически при запуске приложения
func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

type URLShortener struct {
	mu      sync.RWMutex
	data    map[string]string // shortURL -> originalURL
	baseURL string
	storage *storage.FileStorage // добавляем хранилище
	db      storage.DBInterface
}

func NewURLShortener(baseURL string, storage *storage.FileStorage, db storage.DBInterface) *URLShortener {
	shortener := &URLShortener{
		data:    make(map[string]string),
		baseURL: baseURL,
		storage: storage,
		db:      db,
	}

	// Загружаем данные из хранилища
	shortener.loadFromStorage()
	return shortener
}

func (us *URLShortener) loadFromStorage() error {
	if us.storage == nil {
		return nil
	}

	// Загружаем данные из файла
	if err := us.storage.Load(); err != nil {
		return err
	}

	// Преобразуем загруженные данные в map
	us.mu.Lock()
	defer us.mu.Unlock()

	// Предполагая, что у FileStorage есть метод GetEntries()
	entries := us.storage.GetEntries()
	for _, entry := range entries {
		us.data[entry.ShortURL] = entry.OriginalURL
	}

	return nil
}

// SaveToStorage сохраняет URL в хранилище
func (us *URLShortener) SaveToStorage(shortURL, originalURL string) error {
	if us.storage == nil {
		return nil
	}

	// Генерируем ID (можно использовать shortURL или что-то другое)
	entry := models.URLEntry{
		UUID:        generateID(),
		ShortURL:    shortURL,
		OriginalURL: originalURL,
	}

	return us.storage.Save(entry)
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GenerateShortURL создает короткий код из хеша
func (us *URLShortener) GenerateUniqueShortURL() string {
	// Генерируем случайные байты
	randomBytes := make([]byte, 8)

	io.ReadFull(rand.Reader, randomBytes)

	// Кодируем в Base64URL и обрезаем до 10 символов
	shortCode := base64.URLEncoding.EncodeToString(randomBytes)[:10]
	return shortCode
}

// Store сохраняет ссылку и возвращает короткий код
func (us *URLShortener) Store(originalURL string) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Генерируем уникальный short code
	shortCode := us.GenerateUniqueShortURL()

	us.data[shortCode] = originalURL

	err := us.SaveToStorage(shortCode, originalURL)

	return shortCode, err
}

// Retrieve получает оригинальную ссылку по короткому коду
func (us *URLShortener) Retrieve(shortCode string) (string, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	longURL, exists := us.data[shortCode]
	return longURL, exists
}

func (us *URLShortener) handlerRoot(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType != models.TypeTextPlain {
		http.Error(w, "Content-Type must be text/plain", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	originalURL := strings.TrimSpace(string(body))

	if originalURL == "" {
		http.Error(w, "URL cannot be empty", http.StatusBadRequest)
		return
	}

	shortCode, err := us.Store(originalURL)
	if err != nil {
		logger.Log.Info("cannot store", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(strings.Join([]string{us.baseURL, shortCode}, "/")))
}

func (us *URLShortener) redirectToOriginal(w http.ResponseWriter, r *http.Request) {
	shortCode := r.URL.Path[1:]
	if original, exists := us.Retrieve(shortCode); exists {
		w.Header().Set("Location", original)
		w.WriteHeader(http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "Not found", http.StatusBadRequest)
	}
}

func (us *URLShortener) DBPing(w http.ResponseWriter, r *http.Request) {
	if err := storage.PingDB(us.db); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (us *URLShortener) handlerPostJSON(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != models.TypeApplicationJSON {
		logger.Log.Debug("unsupported request type", zap.String("type", contentType))
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	var req models.ShortenRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	originalURL := strings.TrimSpace(string(req.URL))

	if originalURL == "" {
		http.Error(w, "URL cannot be empty", http.StatusBadRequest)
		return
	}

	shortCode, err := us.Store(originalURL)

	if err != nil {
		logger.Log.Info("cannot store", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := models.ShortenResponse{
		Result: strings.Join([]string{us.baseURL, shortCode}, "/"),
	}

	w.Header().Set("Content-Type", models.TypeApplicationJSON)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (us *URLShortener) mainHandler() chi.Router {
	r := chi.NewRouter()

	r.Get("/{shortCode}", us.redirectToOriginal)
	r.Get("/ping", us.DBPing)

	r.Post("/", us.handlerRoot)
	r.Post("/api/shorten", us.handlerPostJSON)
	return r
}

// функция run будет полезна при инициализации зависимостей сервера перед запуском
func run() error {
	cfg := config.LoadConfig()

	db, err := storage.NewConnection(cfg)

	if err != nil {
		return err
	}

	defer db.Close() // закрываем в main

	// Создаем файловое хранилище
	fileStorage := storage.NewFileStorage(cfg.FileStoragePath)

	// Создаем shortener с хранилищем
	us := NewURLShortener(cfg.BaseURL, fileStorage, db)

	if err := logger.Initialize(zap.InfoLevel.String()); err != nil {
		return err
	}

	logger.Log.Info("Running server", zap.String("address", cfg.ServerAddress))

	return http.ListenAndServe(cfg.ServerAddress, logger.RequestLogger(gzip.GzipMiddleware(us.mainHandler())))
}
