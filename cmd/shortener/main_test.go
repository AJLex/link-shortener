package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNewURLShortener(t *testing.T) {
	us := NewURLShortener()

	if us.data == nil {
		t.Error("Expected data map to be initialized")
	}

	if len(us.data) != 0 {
		t.Error("Expected empty data map")
	}
}

func TestGenerateUniqueShortURL(t *testing.T) {
	us := NewURLShortener()

	// Генерируем несколько short URL и проверяем их уникальность
	shortURLs := make(map[string]bool)

	for i := 0; i < 100; i++ {
		shortURL := us.GenerateUniqueShortURL()

		if len(shortURL) != 10 {
			t.Errorf("Expected short URL length 10, got %d", len(shortURL))
		}

		if shortURLs[shortURL] {
			t.Errorf("Duplicate short URL generated: %s", shortURL)
		}

		shortURLs[shortURL] = true
	}
}

func TestStoreAndRetrieve(t *testing.T) {
	us := NewURLShortener()

	originalURL := "https://example.com"
	shortCode := us.Store(originalURL)

	if shortCode == "" {
		t.Error("Expected non-empty short code")
	}

	// Проверяем, что ссылка сохранилась
	retrievedURL, exists := us.Retrieve(shortCode)
	if !exists {
		t.Error("Expected URL to exist")
	}

	if retrievedURL != originalURL {
		t.Errorf("Expected %s, got %s", originalURL, retrievedURL)
	}

	// Проверяем несуществующую ссылку
	_, exists = us.Retrieve("nonexistent")
	if exists {
		t.Error("Expected URL not to exist")
	}
}

func TestConcurrentStoreAndRetrieve(t *testing.T) {
	us := NewURLShortener()
	var wg sync.WaitGroup
	urls := make(map[string]string)
	var mu sync.Mutex

	// Запускаем несколько горутин для параллельного сохранения
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			url := strings.Repeat("a", i+1)
			shortCode := us.Store(url)

			mu.Lock()
			urls[shortCode] = url
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Проверяем, что все ссылки сохранились корректно
	for shortCode, expectedURL := range urls {
		actualURL, exists := us.Retrieve(shortCode)
		if !exists {
			t.Errorf("URL with code %s should exist", shortCode)
		}
		if actualURL != expectedURL {
			t.Errorf("Expected %s, got %s", expectedURL, actualURL)
		}
	}
}

func TestHandlerRoot_POST_Success(t *testing.T) {
	us := NewURLShortener()

	originalURL := "https://example.com"
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(originalURL))
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()
	us.handlerRoot(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	body := w.Body.String()
	if !strings.HasPrefix(body, localhost) {
		t.Errorf("Expected response to start with %s, got %s", localhost, body)
	}

	// Извлекаем short code из ответа
	shortCode := strings.TrimPrefix(body, localhost)

	// Проверяем, что ссылка действительно сохранилась
	retrievedURL, exists := us.Retrieve(shortCode)
	if !exists {
		t.Error("URL should be stored")
	}
	if retrievedURL != originalURL {
		t.Errorf("Expected %s, got %s", originalURL, retrievedURL)
	}
}

func TestHandlerRoot_POST_InvalidMethods(t *testing.T) {
	us := NewURLShortener()

	invalidMethods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		req := httptest.NewRequest(method, "/", nil)
		w := httptest.NewRecorder()

		us.handlerRoot(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for method %s, got %d", http.StatusBadRequest, method, w.Code)
		}
	}
}

func TestHandlerRoot_POST_InvalidContentType(t *testing.T) {
	us := NewURLShortener()

	req := httptest.NewRequest("POST", "/", bytes.NewBufferString("https://example.com"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	us.handlerRoot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandlerRoot_POST_EmptyBody(t *testing.T) {
	us := NewURLShortener()

	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()
	us.handlerRoot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandlerRoot_POST_WhitespaceBody(t *testing.T) {
	us := NewURLShortener()

	req := httptest.NewRequest("POST", "/", bytes.NewBufferString("   \n  \t  "))
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()
	us.handlerRoot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandlerGet_GET_Success(t *testing.T) {
	us := NewURLShortener()
	originalURL := "https://example.com"
	shortCode := us.Store(originalURL)

	req := httptest.NewRequest("GET", "/"+shortCode, nil)
	w := httptest.NewRecorder()

	us.handlerGet(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}

	location := w.Header().Get("Location")
	if location != originalURL {
		t.Errorf("Expected Location header %s, got %s", originalURL, location)
	}
}

func TestHandlerGet_GET_NotFound(t *testing.T) {
	us := NewURLShortener()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	us.handlerGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandlerGet_InvalidMethods(t *testing.T) {
	us := NewURLShortener()

	invalidMethods := []string{"POST", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		req := httptest.NewRequest(method, "/abc", nil)
		w := httptest.NewRecorder()

		us.handlerGet(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for method %s, got %d", http.StatusBadRequest, method, w.Code)
		}
	}
}

func TestMainHandler_RootPath_POST(t *testing.T) {
	us := NewURLShortener()

	originalURL := "https://example.com"
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(originalURL))
	req.Header.Set("Content-Type", "text/plain")

	w := httptest.NewRecorder()
	us.mainHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}
}

func TestMainHandler_RootPath_InvalidMethod(t *testing.T) {
	us := NewURLShortener()

	invalidMethods := []string{"GET", "PUT", "DELETE"}

	for _, method := range invalidMethods {
		req := httptest.NewRequest(method, "/", nil)
		w := httptest.NewRecorder()

		us.mainHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for method %s, got %d", http.StatusBadRequest, method, w.Code)
		}
	}
}

func TestMainHandler_ShortURL_GET(t *testing.T) {
	us := NewURLShortener()
	originalURL := "https://example.com"
	shortCode := us.Store(originalURL)

	req := httptest.NewRequest("GET", "/"+shortCode, nil)
	w := httptest.NewRecorder()

	us.mainHandler(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
}

func TestMainHandler_ShortURL_InvalidMethod(t *testing.T) {
	us := NewURLShortener()

	invalidMethods := []string{"POST", "PUT", "DELETE"}

	for _, method := range invalidMethods {
		req := httptest.NewRequest(method, "/abc", nil)
		w := httptest.NewRecorder()

		us.mainHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for method %s, got %d", http.StatusBadRequest, method, w.Code)
		}
	}
}

func TestMainHandler_EmptyPath(t *testing.T) {
	us := NewURLShortener()

	req := httptest.NewRequest("GET", localhost, nil)
	w := httptest.NewRecorder()

	us.mainHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
