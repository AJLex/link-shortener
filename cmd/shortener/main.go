package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AJLex/link-shortener/internal/auth"
	"github.com/AJLex/link-shortener/internal/config"
	"github.com/AJLex/link-shortener/internal/deleter"
	"github.com/AJLex/link-shortener/internal/gzip"
	"github.com/AJLex/link-shortener/internal/logger"
	models "github.com/AJLex/link-shortener/internal/model"
	"github.com/AJLex/link-shortener/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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
	storage storage.Storage  // единый интерфейс хранилища
	deleter *deleter.Deleter // асинхронный обработчик удаления
}

func NewURLShortener(baseURL string, storage storage.Storage) *URLShortener {
	// Создаём deleter с параметрами:
	// batchSize: 100 - размер батча для обновления
	// workers: 20 - количество fan-in воркеров (увеличено для высокой нагрузки)
	// flushTimeout: 100ms - короткий таймаут для быстрой обработки в тестах
	del := deleter.NewDeleter(storage, 100, 20, 100*time.Millisecond, logger.Log)
	del.Start()

	shortener := &URLShortener{
		data:    make(map[string]string),
		baseURL: baseURL,
		storage: storage,
		deleter: del,
	}

	// Загружаем данные из хранилища
	shortener.loadFromStorage()
	return shortener
}

func (us *URLShortener) isMemoryBasedStorage() bool {
	// Проверяем, нужно ли использовать in-memory кэш
	_, isMemory := us.storage.(*storage.MemoryStorage)
	_, isFile := us.storage.(*storage.FileStorage)
	return isMemory || isFile
}

func (us *URLShortener) loadFromStorage() error {
	if us.storage == nil {
		return nil
	}

	// Загружаем данные в память ТОЛЬКО для memory/file storage
	if !us.isMemoryBasedStorage() {
		return nil
	}

	entries, err := us.storage.GetAll()
	if err != nil {
		return err
	}

	us.mu.Lock()
	defer us.mu.Unlock()
	for shortURL, originalURL := range entries {
		us.data[shortURL] = originalURL
	}

	return nil
}

// Store сохраняет ссылку и возвращает короткий код
func (us *URLShortener) Store(originalURL string) (string, error) {
	// Генерируем уникальный short code
	shortCode := us.GenerateUniqueShortURL()

	// Сохраняем в хранилище
	existing, err := us.storage.Save(shortCode, originalURL)
	if err != nil {
		return existing, err
	}

	// Для memory/file storage обновляем локальный кэш
	if us.isMemoryBasedStorage() {
		us.mu.Lock()
		us.data[shortCode] = originalURL
		us.mu.Unlock()
	}

	return shortCode, nil
}

// StoreWithUser сохраняет ссылку с привязкой к пользователю и возвращает короткий код
func (us *URLShortener) StoreWithUser(originalURL, userID string) (string, error) {
	// Генерируем уникальный short code
	shortCode := us.GenerateUniqueShortURL()

	// Сохраняем в хранилище с userID
	existing, err := us.storage.SaveWithUser(shortCode, originalURL, userID)
	if err != nil {
		return existing, err
	}

	// Для memory/file storage обновляем локальный кэш
	if us.isMemoryBasedStorage() {
		us.mu.Lock()
		us.data[shortCode] = originalURL
		us.mu.Unlock()
	}

	return shortCode, nil
}

// Retrieve получает оригинальную ссылку по короткому коду
func (us *URLShortener) Retrieve(shortCode string) (string, bool) {
	// Для БД работаем напрямую с хранилищем
	if !us.isMemoryBasedStorage() {
		originalURL, err := us.storage.Get(shortCode)
		if err != nil {
			return "", false
		}
		return originalURL, true
	}

	// Для memory/file storage используем локальный кэш
	us.mu.RLock()
	defer us.mu.RUnlock()
	longURL, exists := us.data[shortCode]
	return longURL, exists
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

	// Извлекаем userID из context
	userID, _ := auth.GetUserID(r.Context())

	shortCode, err := us.StoreWithUser(originalURL, userID)
	statusCode := http.StatusCreated
	if err != nil {
		if errors.Is(err, storage.ErrExists) {
			statusCode = http.StatusConflict
		} else {
			logger.Log.Info("cannot store", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(statusCode)
	w.Write([]byte(strings.Join([]string{us.baseURL, shortCode}, "/")))
}

func (us *URLShortener) redirectToOriginal(w http.ResponseWriter, r *http.Request) {
	shortCode := r.URL.Path[1:]

	// Для БД напрямую проверяем флаг удаления
	if !us.isMemoryBasedStorage() {
		originalURL, isDeleted, err := us.storage.GetWithDeletedFlag(shortCode)
		if err != nil {
			http.Error(w, "Not found", http.StatusBadRequest)
			return
		}
		if isDeleted {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.Header().Set("Location", originalURL)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	// Для memory/file storage используем существующий метод Retrieve
	if original, exists := us.Retrieve(shortCode); exists {
		w.Header().Set("Location", original)
		w.WriteHeader(http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "Not found", http.StatusBadRequest)
	}
}

func (us *URLShortener) DBPing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()

	if err := us.storage.Ping(ctx); err != nil {
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

	// Извлекаем userID из context
	userID, _ := auth.GetUserID(r.Context())

	shortCode, err := us.StoreWithUser(originalURL, userID)

	statusCode := http.StatusCreated
	if err != nil {
		if errors.Is(err, storage.ErrExists) {
			statusCode = http.StatusConflict
		} else {
			logger.Log.Info("cannot store", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	resp := models.ShortenResponse{
		Result: strings.Join([]string{us.baseURL, shortCode}, "/"),
	}

	w.Header().Set("Content-Type", models.TypeApplicationJSON)
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (us *URLShortener) handlerBatch(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != models.TypeApplicationJSON {
		logger.Log.Debug("unsupported request type", zap.String("type", contentType))
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	var batchRequests []models.BatchRequestItem
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&batchRequests); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Проверяем пустой батч
	if len(batchRequests) == 0 {
		http.Error(w, "Batch cannot be empty", http.StatusBadRequest)
		return
	}

	// Логируем получение батча
	logger.Log.Info("processing batch request", zap.Int("total_entries", len(batchRequests)))

	// Извлекаем userID из context
	userID, _ := auth.GetUserID(r.Context())

	var batchResponses []models.BatchResponseItem
	var skippedEntries []string

	// Обрабатываем батчами по 100 записей
	for i := 0; i < len(batchRequests); i += 100 {
		end := i + 100
		if end > len(batchRequests) {
			end = len(batchRequests)
		}

		batch := batchRequests[i:end]
		batchEntries := make(map[string]string)
		batchCorrelationMap := make(map[string]string) // shortCode -> correlationID

		// Подготавливаем данные для текущего батча
		for _, item := range batch {
			originalURL := strings.TrimSpace(item.OriginalURL)
			correlationID := strings.TrimSpace(item.CorrelationID)

			// Пропускаем пустые URL или correlation_id
			if originalURL == "" || correlationID == "" {
				skippedEntries = append(skippedEntries, correlationID)
				continue
			}

			// Генерируем shortCode и сохраняем связь
			shortCode := us.GenerateUniqueShortURL()
			batchEntries[shortCode] = originalURL
			batchCorrelationMap[shortCode] = correlationID
		}

		// Сохраняем батч если есть валидные записи
		if len(batchEntries) > 0 {
			if err := us.storage.SaveBatchWithUser(batchEntries, userID); err != nil {
				logger.Log.Error("failed to save batch", zap.Error(err))
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Обновляем in-memory данные ТОЛЬКО для memory/file storage
			if us.isMemoryBasedStorage() {
				us.mu.Lock()
				maps.Copy(us.data, batchEntries)
				us.mu.Unlock()
			}

			// Формируем ответ для успешно сохраненных записей
			for shortCode, correlationID := range batchCorrelationMap {
				batchResponses = append(batchResponses, models.BatchResponseItem{
					CorrelationID: correlationID,
					ShortURL:      strings.Join([]string{us.baseURL, shortCode}, "/"),
				})
			}
		}
	}

	// Логируем пропущенные записи
	if len(skippedEntries) > 0 {
		logger.Log.Warn("skipped entries in batch",
			zap.Int("count", len(skippedEntries)),
			zap.Strings("correlation_ids", skippedEntries))
	}

	// Если не сохранили ни одной записи
	if len(batchResponses) == 0 {
		http.Error(w, "No valid URLs to process", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", models.TypeApplicationJSON)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(batchResponses); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logger.Log.Info("batch processing completed",
		zap.Int("processed", len(batchResponses)),
		zap.Int("skipped", len(skippedEntries)))
}

func (us *URLShortener) handlerGetUserURLs(w http.ResponseWriter, r *http.Request) {
	// Извлекаем userID из context
	userID, ok := auth.GetUserID(r.Context())
	if !ok || userID == "" {
		logger.Log.Debug("user ID not found in context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Получаем все URL пользователя
	urls, err := us.storage.GetByUser(userID)
	if err != nil {
		logger.Log.Error("failed to get user URLs", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Если у пользователя нет сохраненных URL
	if len(urls) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Формируем полные URL для ответа
	var response []models.UserURL
	for _, url := range urls {
		response = append(response, models.UserURL{
			ShortURL:    strings.Join([]string{us.baseURL, url.ShortURL}, "/"),
			OriginalURL: url.OriginalURL,
		})
	}

	w.Header().Set("Content-Type", models.TypeApplicationJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Log.Error("failed to encode response", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logger.Log.Info("returned user URLs",
		zap.String("userID", userID),
		zap.Int("count", len(response)))
}

func (us *URLShortener) handlerDeleteUserURLs(w http.ResponseWriter, r *http.Request) {
	// Проверка Content-Type
	if r.Header.Get("Content-Type") != models.TypeApplicationJSON {
		logger.Log.Debug("unsupported content type for delete", zap.String("type", r.Header.Get("Content-Type")))
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	// Получение userID из context
	userID, ok := auth.GetUserID(r.Context())
	if !ok || userID == "" {
		logger.Log.Debug("user ID not found in context for delete")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Парсинг массива shortCode
	var shortCodes []string
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&shortCodes); err != nil {
		logger.Log.Debug("cannot decode delete request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Валидация
	if len(shortCodes) == 0 {
		logger.Log.Debug("empty shortCodes list in delete request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Асинхронная отправка на удаление
	for _, code := range shortCodes {
		us.deleter.Delete(code, userID)
	}

	logger.Log.Info("delete request accepted",
		zap.String("userID", userID),
		zap.Int("count", len(shortCodes)))

	// Немедленный ответ 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

func (us *URLShortener) mainHandler(cfg config.Config) chi.Router {
	r := chi.NewRouter()

	// Добавляем middleware аутентификации для всех запросов
	r.Use(auth.AuthMiddleware(cfg.JWTSecret, cfg.CookieName, cfg.CookieMaxAge, logger.Log))

	r.Get("/{shortCode}", us.redirectToOriginal)
	r.Get("/ping", us.DBPing)
	r.Get("/api/user/urls", us.handlerGetUserURLs)

	r.Post("/", us.handlerRoot)
	r.Post("/api/shorten", us.handlerPostJSON)
	r.Post("/api/shorten/batch", us.handlerBatch)

	r.Delete("/api/user/urls", us.handlerDeleteUserURLs)
	return r
}

// initStorage инициализирует storage с fallback механизмом
func initStorage(cfg config.Config) storage.Storage {
	// Пробуем PostgreSQL
	if cfg.PostgreSQLDns != "" {
		db, err := storage.NewPostgresStorage(cfg.PostgreSQLDns)
		if err != nil {
			logger.Log.Warn("PostgreSQL connection failed, falling back to file storage", zap.Error(err))
		} else {
			logger.Log.Info("Successfully connected to PostgreSQL")

			// Запускаем миграции
			if err := runMigrations(cfg.PostgreSQLDns); err != nil {
				logger.Log.Warn("Migrations failed", zap.Error(err))
			}
			return db
		}
	}

	// Fallback на file storage
	if cfg.FileStoragePath != "" {
		logger.Log.Info("Using file storage", zap.String("path", cfg.FileStoragePath))
		return storage.NewFileStorage(cfg.FileStoragePath)
	}

	// Fallback на memory storage
	logger.Log.Info("Using in-memory storage")
	return storage.NewMemoryStorage()
}

// setupGracefulShutdown настраивает graceful shutdown для сервера
func setupGracefulShutdown(ctx context.Context, server *http.Server, us *URLShortener) {
	go func() {
		<-ctx.Done()

		logger.Log.Info("Shutdown signal received")

		// 1. Останавливаем deleter с таймаутом
		// Обрабатываем все накопленные задачи на удаление
		logger.Log.Info("Stopping deleter...")
		done := make(chan struct{})
		go func() {
			us.deleter.Stop()
			close(done)
		}()

		select {
		case <-done:
			logger.Log.Info("Deleter stopped successfully")
		case <-time.After(15 * time.Second):
			logger.Log.Warn("Deleter stop timeout, forcing shutdown")
		}

		// 2. Закрываем storage
		logger.Log.Info("Closing storage...")
		if err := us.storage.Close(); err != nil {
			logger.Log.Error("Storage close error", zap.Error(err))
		} else {
			logger.Log.Info("Storage closed successfully")
		}

		logger.Log.Info("Shutdown complete")

		// Примечание: HTTP сервер закроется автоматически при выходе из main()
		// Не закрываем явно, т.к. это может вызывать зависание в тестовом окружении
	}()
}

// функция run будет полезна при инициализации зависимостей сервера перед запуском
func run() error {
	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	// Инициализируем logger
	if err := logger.Initialize(zap.InfoLevel.String()); err != nil {
		return err
	}

	// Инициализируем storage с fallback
	db := initStorage(cfg)

	// Создаем shortener
	us := NewURLShortener(cfg.BaseURL, db)

	// Создаём context для graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Создаём HTTP сервер
	server := &http.Server{
		Addr:         cfg.ServerAddress,
		Handler:      logger.RequestLogger(gzip.GzipMiddleware(us.mainHandler(cfg))),
		ReadTimeout:  5 * time.Second,  // Таймаут чтения запроса
		WriteTimeout: 10 * time.Second, // Таймаут записи ответа
		IdleTimeout:  5 * time.Second,  // Короткий таймаут для быстрого закрытия idle соединений
	}

	// Настраиваем graceful shutdown
	setupGracefulShutdown(ctx, server, us)

	// Запускаем сервер
	logger.Log.Info("Server starting", zap.String("address", cfg.ServerAddress))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// runMigrations запускает миграции базы данных
func runMigrations(dsn string) error {
	// Реализация миграций с использованием golang-migrate/migrate
	m, err := migrate.New(
		"file://migrations",
		dsn,
	)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
