package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AJLex/link-shortener/internal/config"
	models "github.com/AJLex/link-shortener/internal/model"
	"github.com/AJLex/link-shortener/internal/storage/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	// defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

// Вспомогательная функция для создания тестового конфига
func createTestConfig() *config.Config {
	return &config.Config{
		ServerAddress:   "localhost:8080",
		BaseURL:         "http://localhost:8080",
		FileStoragePath: "/temp",
	}
}

// Расширенная версия с проверкой ошибок
func TestHandlerRoot_StoreAndRedirect_Comprehensive(t *testing.T) {
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockDB := mock.NewMockStorage(ctrl)

			cfg := createTestConfig()
			// mockDB.EXPECT().
			// 	GetAll().
			// 	Return(nil, nil).
			// 	Times(1)
			if tc.expectSuccess {
				mockDB.EXPECT().
					Save(gomock.Any(), tc.originalURL).
					Return("", nil).
					Times(1)
			} else {
				mockDB.EXPECT().
					Save(gomock.Any(), tc.originalURL).Times(0)
			}

			us := NewURLShortener(cfg.BaseURL, mockDB)
			handler := us.mainHandler()

			headers := map[string]string{
				"Content-Type": "text/plain",
			}

			resp, _ := testRequest(t, handler, http.MethodPost, "/", bytes.NewBufferString(tc.originalURL), headers)
			defer resp.Body.Close()

			if tc.expectSuccess {
				assert.Equal(t, http.StatusCreated, resp.StatusCode, tc.description)
			} else {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode, tc.description)
			}
		})
	}
}

func TestHandlerPostJson(t *testing.T) {
	testCases := []struct {
		name          string
		body          string
		method        string
		contentType   string
		statusCode    int
		expectSuccess bool
		description   string
	}{
		{
			name:          "ValidRequest",
			body:          "{\"url\": \"https://google.com\"}",
			contentType:   models.TypeApplicationJSON,
			statusCode:    http.StatusCreated,
			expectSuccess: true,
			description:   "Валидный HTTPS URL",
		},
		{
			name:          "EmptyURL",
			body:          "{\"url\": \"\"}",
			contentType:   models.TypeApplicationJSON,
			statusCode:    http.StatusBadRequest,
			expectSuccess: false,
			description:   "Пустой URL должен вернуть ошибку",
		},
		{
			name:          "WhitespaceURL",
			body:          "{\"url\": \"\"}",
			contentType:   models.TypeApplicationJSON,
			statusCode:    http.StatusBadRequest,
			expectSuccess: false,
			description:   "URL из пробелов должен вернуть ошибку",
		},
		{
			name:          "MissingKey",
			body:          "{\"foo\": \"bar\"}",
			contentType:   models.TypeApplicationJSON,
			statusCode:    http.StatusBadRequest,
			expectSuccess: false,
			description:   "JSON без ключа url должен вернуть ошибку",
		},
		{
			name:          "UnsupportedMediaType",
			body:          "{\"url\": \"https://google.com\"}",
			contentType:   models.TypeTextPlain,
			statusCode:    http.StatusUnsupportedMediaType,
			expectSuccess: false,
			description:   "Неправильный тип передаваемого контента",
		},
		{
			name:          "WrongContent",
			body:          "hello world!",
			contentType:   models.TypeApplicationJSON,
			statusCode:    http.StatusBadRequest,
			expectSuccess: false,
			description:   "Проблемы при десерилизации должны вернуть ошибку",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockDB := mock.NewMockStorage(ctrl)

			cfg := createTestConfig()
			// mockDB.EXPECT().
			// 	GetAll().
			// 	Return(nil, nil).
			// 	Times(1)
			if tc.expectSuccess {
				mockDB.EXPECT().
					Save(gomock.Any(), "https://google.com").
					Return("", nil).
					Times(1)
			} else {
				mockDB.EXPECT().
					Save(gomock.Any(), nil).Times(0)
			}

			us := NewURLShortener(cfg.BaseURL, mockDB)
			handler := us.mainHandler()
			fmt.Println(tc.contentType, tc.body)

			headers := map[string]string{
				"Content-Type": tc.contentType,
			}

			resp, _ := testRequest(t, handler, http.MethodPost, "/api/shorten", bytes.NewBufferString(tc.body), headers)
			defer resp.Body.Close()

			if tc.expectSuccess {
				assert.Equal(t, tc.statusCode, resp.StatusCode, tc.description)
			} else {
				assert.Equal(t, tc.statusCode, resp.StatusCode, tc.description)
			}
		})
	}
}

func TestURLShortener_StoreAndRetrieve(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	originalURL := "https://example.com"
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)
	mockDB.EXPECT().
		Save(gomock.Any(), originalURL).
		Return("", nil).
		Times(1)
	mockDB.EXPECT().
		Get(gomock.Any()).
		Return(originalURL, nil).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)

	shortCode, _ := us.Store(originalURL)
	require.NotEmpty(t, shortCode)

	retrievedURL, exists := us.Retrieve(shortCode)
	assert.True(t, exists)
	assert.Equal(t, originalURL, retrievedURL)
}

func TestMainHandler_RootPath_InvalidMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	invalidMethods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		t.Run(method, func(t *testing.T) {
			resp, _ := testRequest(t, handler, method, "/", nil, nil)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

func TestMainHandler_ShortURL_InvalidMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	invalidMethods := []string{"POST", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		t.Run(method, func(t *testing.T) {
			resp, _ := testRequest(t, handler, method, "/abc", nil, nil)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

func TestMainHandler_PostJson_InvalidMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	invalidMethods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range invalidMethods {
		t.Run(method, func(t *testing.T) {
			resp, _ := testRequest(t, handler, method, "/api/shorten", nil, nil)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

func TestMainHandler_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	mockDB.EXPECT().
		Get(gomock.Any()).
		Return("", errors.New("not found")).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	resp, _ := testRequest(t, handler, "GET", "/nonexistent", nil, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMainHandler_InvalidContentType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	cfg := createTestConfig()

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString("https://example.com"), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMainHandler_EmptyBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	cfg := createTestConfig()

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler()

	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString(""), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestURLShortener_RetrieveNonExistent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	mockDB.EXPECT().
		Get(gomock.Any()).
		Return("", errors.New("not found")).
		Times(1)

	cfg := createTestConfig()

	us := NewURLShortener(cfg.BaseURL, mockDB)

	retrievedURL, exists := us.Retrieve("nonexistent")
	assert.False(t, exists)
	assert.Empty(t, retrievedURL)
}

func TestGenerateUniqueShortURL_Uniqueness(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	cfg := createTestConfig()

	us := NewURLShortener(cfg.BaseURL, mockDB)
	codes := make(map[string]bool)
	const numCodes = 10

	for i := 0; i < numCodes; i++ {
		code := us.GenerateUniqueShortURL()
		assert.False(t, codes[code], "Generated duplicate code: %s", code)
		codes[code] = true
	}

	assert.Len(t, codes, numCodes)
}

func TestURLShortener_DBPing(t *testing.T) {
	values := []bool{true, false}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)
	// mockDB.EXPECT().
	// 	GetAll().
	// 	Return(nil, nil).
	// 	Times(1)

	cfg := createTestConfig()

	us := NewURLShortener(cfg.BaseURL, mockDB)
	for _, expectSuccess := range values {
		if expectSuccess {
			mockDB.EXPECT().
				Ping(gomock.Any()).
				Return(nil)
		} else {
			mockDB.EXPECT().
				Ping(gomock.Any()).
				Return(errors.New("database connection failed"))
		}

		handler := us.mainHandler()

		resp, _ := testRequest(t, handler, http.MethodGet, "/ping", nil, nil)
		defer resp.Body.Close()

		if expectSuccess {
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		} else {
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		}
	}
}
