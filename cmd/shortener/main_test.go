package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRequest(t *testing.T, handler http.Handler, method, path string, body io.Reader, headers map[string]string) (*http.Response, string) {
	req, err := http.NewRequest(method, "http://test"+path, body)
	require.NoError(t, err)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

// Расширенная версия с проверкой ошибок
func TestMainHandler_StoreAndRedirect_Comprehensive(t *testing.T) {
	testCases := []struct {
		name          string
		originalURL   string
		expectSuccess bool
		description   string
	}{
		{
			name:          "ValidHTTPS",
			originalURL:   "https://google.com",
			expectSuccess: true,
			description:   "Валидный HTTPS URL",
		},
		{
			name:          "ValidHTTP",
			originalURL:   "http://example.com",
			expectSuccess: true,
			description:   "Валидный HTTP URL",
		},
		{
			name:          "EmptyURL",
			originalURL:   "",
			expectSuccess: false,
			description:   "Пустой URL должен вернуть ошибку",
		},
		{
			name:          "WhitespaceURL",
			originalURL:   "   ",
			expectSuccess: false,
			description:   "URL из пробелов должен вернуть ошибку",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			us := NewURLShortener()
			handler := us.mainHandler()

			headers := map[string]string{
				"Content-Type": "text/plain",
			}

			resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString(tc.originalURL), headers)

			if tc.expectSuccess {
				assert.Equal(t, http.StatusCreated, resp.StatusCode, tc.description)
			} else {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode, tc.description)
			}
		})
	}
}

func TestMainHandler_RootPath_POST(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	originalURL := "https://example.com"
	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, body := testRequest(t, handler, "POST", "/", bytes.NewBufferString(originalURL), headers)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Contains(t, body, "http://localhost:8080/")
}

func TestURLShortener_StoreAndRetrieve(t *testing.T) {
	us := NewURLShortener()
	originalURL := "https://example.com"

	shortCode := us.Store(originalURL)
	require.NotEmpty(t, shortCode)

	retrievedURL, exists := us.Retrieve(shortCode)
	assert.True(t, exists)
	assert.Equal(t, originalURL, retrievedURL)
}

func TestMainHandler_RootPath_InvalidMethod(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	invalidMethods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		t.Run(method, func(t *testing.T) {
			resp, _ := testRequest(t, handler, method, "/", nil, nil)
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

func TestMainHandler_ShortURL_InvalidMethod(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	invalidMethods := []string{"POST", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		t.Run(method, func(t *testing.T) {
			resp, _ := testRequest(t, handler, method, "/abc", nil, nil)
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

func TestMainHandler_NotFound(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	resp, _ := testRequest(t, handler, "GET", "/nonexistent", nil, nil)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMainHandler_InvalidContentType(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString("https://example.com"), headers)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMainHandler_EmptyBody(t *testing.T) {
	us := NewURLShortener()
	handler := us.mainHandler()

	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString(""), headers)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestURLShortener_RetrieveNonExistent(t *testing.T) {
	us := NewURLShortener()

	retrievedURL, exists := us.Retrieve("nonexistent")
	assert.False(t, exists)
	assert.Empty(t, retrievedURL)
}

func TestGenerateUniqueShortURL_Uniqueness(t *testing.T) {
	us := NewURLShortener()

	codes := make(map[string]bool)
	const numCodes = 10

	for i := 0; i < numCodes; i++ {
		code := us.GenerateUniqueShortURL()
		assert.False(t, codes[code], "Generated duplicate code: %s", code)
		codes[code] = true
	}

	assert.Len(t, codes, numCodes)
}
