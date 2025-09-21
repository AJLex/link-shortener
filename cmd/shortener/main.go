package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
)

var localhost string = "http://localhost:8080/"

type URLShortener struct {
	mu   sync.RWMutex
	data map[string]string // shortURL -> originalURL
}

func NewURLShortener() *URLShortener {
	return &URLShortener{
		data: make(map[string]string),
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
	us := NewURLShortener()

	http.HandleFunc("/", us.mainHandler)

	return http.ListenAndServe(`:8080`, nil)
}

func (us *URLShortener) handlerRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "text/plain" {
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
	w.Write([]byte(localhost + shortCode))
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

func (us *URLShortener) mainHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch path {
	case "":
		http.Error(w, "Path is empty", http.StatusBadRequest)
		return

	case "/":
		if r.Method == http.MethodPost {
			us.handlerRoot(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusBadRequest)
		}
		return

	default:
		// Любой другой путь - обрабатываем GET для редиректа
		if r.Method == http.MethodGet {
			us.handlerGet(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusBadRequest)
		}
		return
	}
}
