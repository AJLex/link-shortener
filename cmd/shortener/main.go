package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/AJLex/link-shortener/internal/config"
	"github.com/AJLex/link-shortener/internal/logger"
	models "github.com/AJLex/link-shortener/internal/model"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type URLShortener struct {
	mu      sync.RWMutex
	data    map[string]string // shortURL -> originalURL
	baseURL string
}

func NewURLShortener(baseURL string) *URLShortener {
	return &URLShortener{
		data:    make(map[string]string),
		baseURL: baseURL,
	}
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
func (us *URLShortener) Store(originalURL string) string {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Генерируем уникальный short code
	shortCode := us.GenerateUniqueShortURL()

	us.data[shortCode] = originalURL
	return shortCode
}

// Retrieve получает оригинальную ссылку по короткому коду
func (us *URLShortener) Retrieve(shortCode string) (string, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	longURL, exists := us.data[shortCode]
	return longURL, exists
}

// функция main вызывается автоматически при запуске приложения
func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

// функция run будет полезна при инициализации зависимостей сервера перед запуском
func run() error {
	cfg := config.LoadConfig()
	us := NewURLShortener(cfg.BaseURL)

	if err := logger.Initialize(zap.InfoLevel.String()); err != nil {
		return err
	}

	logger.Log.Info("Running server", zap.String("address", cfg.ServerAddress))

	return http.ListenAndServe(cfg.ServerAddress, logger.RequestLogger(us.mainHandler()))
}

func (us *URLShortener) handlerRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusBadRequest)
		return
	}

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

	shortCode := us.Store(originalURL)

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(strings.Join([]string{us.baseURL, shortCode}, "/")))
}

func (us *URLShortener) handlerGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method is allowed", http.StatusBadRequest)
		return
	}

	shortCode := r.URL.Path[1:]
	if original, exists := us.Retrieve(shortCode); exists {
		w.Header().Set("Location", original)
		w.WriteHeader(http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "Not found", http.StatusBadRequest)
	}
}

func (us *URLShortener) handlerPostJson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusBadRequest)
		return
	}

	if contentType := r.Header.Get("Content-Type"); contentType != models.TypeApplicationJson {
		logger.Log.Debug("unsupported request type", zap.String("type", contentType))
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	var req models.ShortenRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	originalURL := strings.TrimSpace(string(req.URL))

	if originalURL == "" {
		http.Error(w, "URL cannot be empty", http.StatusBadRequest)
		return
	}

	shortCode := us.Store(originalURL)

	resp := models.ShortenResponse{
		Result: strings.Join([]string{us.baseURL, shortCode}, "/"),
	}

	w.Header().Set("Content-Type", models.TypeApplicationJson)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (us *URLShortener) mainHandler() chi.Router {
	r := chi.NewRouter()

	r.Post("/", us.handlerRoot)
	r.Get("/{shortCode}", us.handlerGet)
	r.Post("/api/shorten", us.handlerPostJson)
	return r
}
