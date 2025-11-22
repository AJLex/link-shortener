package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AJLex/link-shortener/internal/config"
	models "github.com/AJLex/link-shortener/internal/model"
	"github.com/AJLex/link-shortener/internal/storage"
	"github.com/AJLex/link-shortener/internal/storage/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testRequest выполняет HTTP-запрос к handler и возвращает response и body
func testRequest(t *testing.T, handler http.Handler, method, path string, body io.Reader, headers map[string]string) (*http.Response, string) {
	req, err := http.NewRequest(method, "http://test"+path, body)
	require.NoError(t, err)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

// createTestConfig создает тестовую конфигурацию
func createTestConfig() *config.Config {
	return &config.Config{
		ServerAddress:   "localhost:8080",
		BaseURL:         "http://localhost:8080",
		FileStoragePath: "/temp",
	}
}

// newTestMemoryStorage создает in-memory storage для тестов
func newTestMemoryStorage(t *testing.T) storage.Storage {
	return storage.NewMemoryStorage()
}

// newTestFileStorage создает file storage с временным файлом для тестов
func newTestFileStorage(t *testing.T) storage.Storage {
	tempFile := t.TempDir() + "/test_storage.json"
	fileStorage := storage.NewFileStorage(tempFile)

	// Автоматическая очистка при завершении теста
	t.Cleanup(func() {
		fileStorage.Close()
	})

	return fileStorage
}

// newTestPostgresStorageMock создает мокированный PostgreSQL storage для тестов
func newTestPostgresStorageMock(t *testing.T) storage.Storage {
	ctrl := gomock.NewController(t)
	mockDB := mock.NewMockStorage(ctrl)

	// Настраиваем поведение мока для позитивных сценариев
	// Внутреннее хранилище для эмуляции БД
	data := make(map[string]string)

	// Save - сохраняет и возвращает shortCode
	mockDB.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(shortCode, originalURL string) (string, error) {
			// Проверяем, существует ли уже такой originalURL
			for existingShort, existingURL := range data {
				if existingURL == originalURL {
					return existingShort, storage.ErrExists
				}
			}
			data[shortCode] = originalURL
			return shortCode, nil
		}).
		AnyTimes()

	// Get - получает originalURL по shortCode
	mockDB.EXPECT().
		Get(gomock.Any()).
		DoAndReturn(func(shortCode string) (string, error) {
			if url, exists := data[shortCode]; exists {
				return url, nil
			}
			return "", errors.New("not found")
		}).
		AnyTimes()

	// GetAll - возвращает все записи
	mockDB.EXPECT().
		GetAll().
		DoAndReturn(func() (map[string]string, error) {
			result := make(map[string]string)
			for k, v := range data {
				result[k] = v
			}
			return result, nil
		}).
		AnyTimes()

	// Ping - всегда успешен для PostgreSQL мока
	mockDB.EXPECT().
		Ping(gomock.Any()).
		Return(nil).
		AnyTimes()

	// Close - ничего не делает
	mockDB.EXPECT().
		Close().
		Return(nil).
		AnyTimes()

	// SaveBatch - пакетное сохранение
	mockDB.EXPECT().
		SaveBatch(gomock.Any()).
		DoAndReturn(func(entries map[string]string) error {
			for shortCode, originalURL := range entries {
				data[shortCode] = originalURL
			}
			return nil
		}).
		AnyTimes()

	// SaveWithUser - сохраняет с привязкой к пользователю
	mockDB.EXPECT().
		SaveWithUser(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(shortCode, originalURL, userID string) (string, error) {
			// Проверяем, существует ли уже такой originalURL
			for existingShort, existingURL := range data {
				if existingURL == originalURL {
					return existingShort, storage.ErrExists
				}
			}
			data[shortCode] = originalURL
			return shortCode, nil
		}).
		AnyTimes()

	// SaveBatchWithUser - пакетное сохранение с пользователем
	mockDB.EXPECT().
		SaveBatchWithUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(entries map[string]string, userID string) error {
			for shortCode, originalURL := range entries {
				data[shortCode] = originalURL
			}
			return nil
		}).
		AnyTimes()

	// GetByUser - возвращает URL пользователя (для упрощения возвращаем все)
	mockDB.EXPECT().
		GetByUser(gomock.Any()).
		DoAndReturn(func(userID string) ([]models.UserURL, error) {
			var result []models.UserURL
			for shortCode, originalURL := range data {
				result = append(result, models.UserURL{
					ShortURL:    shortCode,
					OriginalURL: originalURL,
				})
			}
			return result, nil
		}).
		AnyTimes()

	// GetWithDeletedFlag - получает originalURL и флаг удаления
	mockDB.EXPECT().
		GetWithDeletedFlag(gomock.Any()).
		DoAndReturn(func(shortCode string) (string, bool, error) {
			if url, exists := data[shortCode]; exists {
				return url, false, nil // isDeleted = false для мока
			}
			return "", false, errors.New("not found")
		}).
		AnyTimes()

	// DeleteBatch - помечает URL как удалённые (для мока просто удаляем)
	mockDB.EXPECT().
		DeleteBatch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, shortCodes []string, userID string) error {
			// Для упрощения просто удаляем из data
			for _, code := range shortCodes {
				delete(data, code)
			}
			return nil
		}).
		AnyTimes()

	return mockDB
}

// storageTestCase описывает тестовый случай для разных типов storage
type storageTestCase struct {
	name          string
	createStorage func(t *testing.T) storage.Storage
}

// getAllStorageTypes возвращает все доступные типы storage для параметризованных тестов
func getAllStorageTypes() []storageTestCase {
	return []storageTestCase{
		{
			name:          "MemoryStorage",
			createStorage: newTestMemoryStorage,
		},
		{
			name:          "FileStorage",
			createStorage: newTestFileStorage,
		},
		{
			name:          "PostgresStorage(Mock)",
			createStorage: newTestPostgresStorageMock,
		},
	}
}

// TestURLShortener_StoreAndRetrieve_AllStorages проверяет Store→Retrieve для всех типов storage
func TestURLShortener_StoreAndRetrieve_AllStorages(t *testing.T) {
	testCases := []struct {
		name        string
		originalURL string
	}{
		{
			name:        "HTTPS URL",
			originalURL: "https://google.com",
		},
		{
			name:        "HTTP URL",
			originalURL: "http://example.com",
		},
		{
			name:        "Long URL",
			originalURL: "https://example.com/very/long/path/with/many/segments?param1=value1&param2=value2",
		},
	}

	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					cfg := createTestConfig()
					store := storageCase.createStorage(t)
					us := NewURLShortener(cfg.BaseURL, store)

					// Store URL
					shortCode, err := us.Store(tc.originalURL)
					require.NoError(t, err)
					require.NotEmpty(t, shortCode)

					// Retrieve URL
					retrievedURL, exists := us.Retrieve(shortCode)
					assert.True(t, exists, "URL должен быть найден")
					assert.Equal(t, tc.originalURL, retrievedURL, "Полученный URL должен совпадать с оригинальным")
				})
			}
		})
	}
}

// TestHandlerRoot_POST_AllStorages проверяет POST / для всех типов storage
func TestHandlerRoot_POST_AllStorages(t *testing.T) {
	testCases := []struct {
		name           string
		originalURL    string
		expectSuccess  bool
		expectedStatus int
		description    string
	}{
		{
			name:           "ValidHTTPS",
			originalURL:    "https://google.com",
			expectSuccess:  true,
			expectedStatus: http.StatusCreated,
			description:    "Валидный HTTPS URL",
		},
		{
			name:           "ValidHTTP",
			originalURL:    "http://example.com",
			expectSuccess:  true,
			expectedStatus: http.StatusCreated,
			description:    "Валидный HTTP URL",
		},
		{
			name:           "EmptyURL",
			originalURL:    "",
			expectSuccess:  false,
			expectedStatus: http.StatusBadRequest,
			description:    "Пустой URL должен вернуть ошибку",
		},
		{
			name:           "WhitespaceURL",
			originalURL:    "   ",
			expectSuccess:  false,
			expectedStatus: http.StatusBadRequest,
			description:    "URL из пробелов должен вернуть ошибку",
		},
	}

	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					cfg := createTestConfig()
					store := storageCase.createStorage(t)
					us := NewURLShortener(cfg.BaseURL, store)
					handler := us.mainHandler(*cfg)

					headers := map[string]string{
						"Content-Type": "text/plain",
					}

					resp, body := testRequest(t, handler, http.MethodPost, "/", bytes.NewBufferString(tc.originalURL), headers)
					defer resp.Body.Close()

					assert.Equal(t, tc.expectedStatus, resp.StatusCode, tc.description)

					if tc.expectSuccess {
						// Проверяем, что получили короткую ссылку
						assert.Contains(t, body, cfg.BaseURL)
						assert.NotEmpty(t, body)
					}
				})
			}
		})
	}
}

// TestHandlerPostJSON_AllStorages проверяет POST /api/shorten для всех типов storage
func TestHandlerPostJSON_AllStorages(t *testing.T) {
	testCases := []struct {
		name           string
		body           string
		contentType    string
		expectedStatus int
		expectSuccess  bool
		description    string
	}{
		{
			name:           "ValidRequest",
			body:           `{"url": "https://google.com"}`,
			contentType:    models.TypeApplicationJSON,
			expectedStatus: http.StatusCreated,
			expectSuccess:  true,
			description:    "Валидный HTTPS URL",
		},
		{
			name:           "EmptyURL",
			body:           `{"url": ""}`,
			contentType:    models.TypeApplicationJSON,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
			description:    "Пустой URL должен вернуть ошибку",
		},
		{
			name:           "MissingKey",
			body:           `{"foo": "bar"}`,
			contentType:    models.TypeApplicationJSON,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
			description:    "JSON без ключа url должен вернуть ошибку",
		},
		{
			name:           "UnsupportedMediaType",
			body:           `{"url": "https://google.com"}`,
			contentType:    models.TypeTextPlain,
			expectedStatus: http.StatusUnsupportedMediaType,
			expectSuccess:  false,
			description:    "Неправильный тип передаваемого контента",
		},
		{
			name:           "WrongContent",
			body:           "hello world!",
			contentType:    models.TypeApplicationJSON,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
			description:    "Проблемы при десериализации должны вернуть ошибку",
		},
	}

	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					cfg := createTestConfig()
					store := storageCase.createStorage(t)
					us := NewURLShortener(cfg.BaseURL, store)
					handler := us.mainHandler(*cfg)

					headers := map[string]string{
						"Content-Type": tc.contentType,
					}

					resp, body := testRequest(t, handler, http.MethodPost, "/api/shorten", bytes.NewBufferString(tc.body), headers)
					defer resp.Body.Close()

					assert.Equal(t, tc.expectedStatus, resp.StatusCode, tc.description)

					if tc.expectSuccess {
						// Проверяем, что получили JSON с короткой ссылкой
						assert.Contains(t, body, `"result"`)
						assert.Contains(t, body, cfg.BaseURL)
					}
				})
			}
		})
	}
}

// TestHandlerRedirect_AllStorages проверяет GET /{shortCode} для всех типов storage
func TestHandlerRedirect_AllStorages(t *testing.T) {
	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			cfg := createTestConfig()
			store := storageCase.createStorage(t)
			us := NewURLShortener(cfg.BaseURL, store)
			handler := us.mainHandler(*cfg)

			// Сохраняем URL
			originalURL := "https://example.com"
			shortCode, err := us.Store(originalURL)
			require.NoError(t, err)

			// Делаем GET запрос
			resp, _ := testRequest(t, handler, http.MethodGet, "/"+shortCode, nil, nil)
			defer resp.Body.Close()

			// Проверяем редирект
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			assert.Equal(t, originalURL, resp.Header.Get("Location"))
		})
	}
}

// TestMultipleStoreAndRetrieve_AllStorages проверяет множественные операции
func TestMultipleStoreAndRetrieve_AllStorages(t *testing.T) {
	urls := []string{
		"https://google.com",
		"https://yandex.ru",
		"https://github.com",
		"https://stackoverflow.com",
	}

	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			cfg := createTestConfig()
			store := storageCase.createStorage(t)
			us := NewURLShortener(cfg.BaseURL, store)

			// Сохраняем все URL и запоминаем их short codes
			shortCodes := make(map[string]string) // shortCode -> originalURL

			for _, url := range urls {
				shortCode, err := us.Store(url)
				require.NoError(t, err)
				require.NotEmpty(t, shortCode)
				shortCodes[shortCode] = url
			}

			// Проверяем, что все URL можно получить обратно
			for shortCode, expectedURL := range shortCodes {
				retrievedURL, exists := us.Retrieve(shortCode)
				assert.True(t, exists, "URL для кода %s должен существовать", shortCode)
				assert.Equal(t, expectedURL, retrievedURL, "URL должен совпадать")
			}
		})
	}
}

// TestHandlerRoot_StorageError проверяет обработку ошибок storage
func TestHandlerRoot_StorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	// Мокаем ошибку при сохранении
	mockDB.EXPECT().
		SaveWithUser(gomock.Any(), "https://example.com", gomock.Any()).
		Return("", errors.New("storage write error")).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, _ := testRequest(t, handler, http.MethodPost, "/", bytes.NewBufferString("https://example.com"), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Ошибка storage должна вернуть 500")
}

// TestHandlerRedirect_NotFound проверяет GET несуществующего short code
func TestHandlerRedirect_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	// Мокаем отсутствие URL в storage (используем GetWithDeletedFlag для БД)
	mockDB.EXPECT().
		GetWithDeletedFlag("nonexistent").
		Return("", false, errors.New("not found")).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	resp, _ := testRequest(t, handler, "GET", "/nonexistent", nil, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Несуществующий код должен вернуть ошибку")
}

// TestURLShortener_Retrieve_StorageError проверяет ошибку при получении из storage
func TestURLShortener_Retrieve_StorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	mockDB.EXPECT().
		Get("testcode").
		Return("", errors.New("storage read error")).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)

	retrievedURL, exists := us.Retrieve("testcode")
	assert.False(t, exists, "При ошибке storage URL не должен быть найден")
	assert.Empty(t, retrievedURL)
}

// TestDBPing_Success проверяет успешный ping БД
func TestDBPing_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	mockDB.EXPECT().
		Ping(gomock.Any()).
		Return(nil).
		Times(1)

	resp, _ := testRequest(t, handler, http.MethodGet, "/ping", nil, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestDBPing_Failure проверяет неуспешный ping БД
func TestDBPing_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()
	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	mockDB.EXPECT().
		Ping(gomock.Any()).
		Return(errors.New("database connection failed")).
		Times(1)

	resp, _ := testRequest(t, handler, http.MethodGet, "/ping", nil, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestHandlerRoot_Conflict проверяет обработку конфликта (дубликата URL)
func TestHandlerRoot_Conflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	// Мокаем ситуацию, когда URL уже существует
	mockDB.EXPECT().
		SaveWithUser(gomock.Any(), "https://example.com", gomock.Any()).
		Return("existing123", storage.ErrExists).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, body := testRequest(t, handler, http.MethodPost, "/", bytes.NewBufferString("https://example.com"), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode, "Дубликат URL должен вернуть 409")
	assert.Contains(t, body, "existing123", "Должен вернуть существующий короткий код")
}

// TestMainHandler_InvalidMethods проверяет обработку недопустимых HTTP методов
func TestMainHandler_InvalidMethods(t *testing.T) {
	testCases := []struct {
		path           string
		invalidMethods []string
		description    string
	}{
		{
			path:           "/",
			invalidMethods: []string{"GET", "PUT", "DELETE", "PATCH"},
			description:    "Root path должен принимать только POST",
		},
		{
			path:           "/abc",
			invalidMethods: []string{"POST", "PUT", "DELETE", "PATCH"},
			description:    "Short URL path должен принимать только GET",
		},
		{
			path:           "/api/shorten",
			invalidMethods: []string{"GET", "PUT", "DELETE", "PATCH"},
			description:    "API shorten должен принимать только POST",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			cfg := createTestConfig()
			store := newTestMemoryStorage(t)
			us := NewURLShortener(cfg.BaseURL, store)
			handler := us.mainHandler(*cfg)

			for _, method := range tc.invalidMethods {
				t.Run(method, func(t *testing.T) {
					resp, _ := testRequest(t, handler, method, tc.path, nil, nil)
					defer resp.Body.Close()
					assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, tc.description)
				})
			}
		})
	}
}

// TestMainHandler_InvalidContentType проверяет обработку неправильного Content-Type
func TestMainHandler_InvalidContentType(t *testing.T) {
	cfg := createTestConfig()
	store := newTestMemoryStorage(t)
	us := NewURLShortener(cfg.BaseURL, store)
	handler := us.mainHandler(*cfg)

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, "POST", "/", bytes.NewBufferString("https://example.com"), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Неправильный Content-Type должен вернуть ошибку")
}

// TestGenerateUniqueShortURL_Uniqueness проверяет уникальность генерируемых кодов
func TestGenerateUniqueShortURL_Uniqueness(t *testing.T) {
	cfg := createTestConfig()
	store := newTestMemoryStorage(t)
	us := NewURLShortener(cfg.BaseURL, store)

	codes := make(map[string]bool)
	const numCodes = 1000

	for i := 0; i < numCodes; i++ {
		code := us.GenerateUniqueShortURL()
		assert.False(t, codes[code], "Сгенерирован дубликат кода: %s", code)
		assert.Len(t, code, 10, "Длина кода должна быть 10 символов")
		codes[code] = true
	}

	assert.Len(t, codes, numCodes, "Все коды должны быть уникальными")
}

// TestURLShortener_Ping_RealStorages проверяет Ping для реальных storage
func TestURLShortener_Ping_RealStorages(t *testing.T) {
	storages := getAllStorageTypes()

	for _, storageCase := range storages {
		t.Run(storageCase.name, func(t *testing.T) {
			store := storageCase.createStorage(t)

			ctx := context.Background()
			err := store.Ping(ctx)

			assert.NoError(t, err, "Ping для %s не должен возвращать ошибку", storageCase.name)
		})
	}
}

// TestHandlerDeleteUserURLs_Success проверяет успешное удаление
func TestHandlerDeleteUserURLs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	// Mock для DeleteBatch
	mockDB.EXPECT().
		DeleteBatch(gomock.Any(), []string{"code1", "code2"}, gomock.Any()).
		Return(nil).
		MinTimes(0) // Может быть вызван асинхронно

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	body := `["code1", "code2"]`
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, http.MethodDelete, "/api/user/urls", bytes.NewBufferString(body), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode, "Должен вернуть 202 Accepted")
}

// TestHandlerDeleteUserURLs_EmptyList проверяет пустой список
func TestHandlerDeleteUserURLs_EmptyList(t *testing.T) {
	cfg := createTestConfig()
	store := newTestMemoryStorage(t)
	us := NewURLShortener(cfg.BaseURL, store)
	handler := us.mainHandler(*cfg)

	body := `[]`
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, http.MethodDelete, "/api/user/urls", bytes.NewBufferString(body), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Пустой список должен вернуть 400")
}

// TestHandlerDeleteUserURLs_InvalidJSON проверяет некорректный JSON
func TestHandlerDeleteUserURLs_InvalidJSON(t *testing.T) {
	cfg := createTestConfig()
	store := newTestMemoryStorage(t)
	us := NewURLShortener(cfg.BaseURL, store)
	handler := us.mainHandler(*cfg)

	body := `{invalid json}`
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, _ := testRequest(t, handler, http.MethodDelete, "/api/user/urls", bytes.NewBufferString(body), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Некорректный JSON должен вернуть 400")
}

// TestHandlerDeleteUserURLs_WrongContentType проверяет неправильный Content-Type
func TestHandlerDeleteUserURLs_WrongContentType(t *testing.T) {
	cfg := createTestConfig()
	store := newTestMemoryStorage(t)
	us := NewURLShortener(cfg.BaseURL, store)
	handler := us.mainHandler(*cfg)

	body := `["code1"]`
	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	resp, _ := testRequest(t, handler, http.MethodDelete, "/api/user/urls", bytes.NewBufferString(body), headers)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode, "Неправильный Content-Type должен вернуть 415")
}

// TestRedirectToOriginal_DeletedURL проверяет возврат 410 Gone для удалённого URL
func TestRedirectToOriginal_DeletedURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock.NewMockStorage(ctrl)

	cfg := createTestConfig()

	// Mock для GetWithDeletedFlag - URL удалён
	mockDB.EXPECT().
		GetWithDeletedFlag("deleted123").
		Return("https://example.com", true, nil).
		Times(1)

	us := NewURLShortener(cfg.BaseURL, mockDB)
	handler := us.mainHandler(*cfg)

	resp, _ := testRequest(t, handler, http.MethodGet, "/deleted123", nil, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusGone, resp.StatusCode, "Удалённый URL должен вернуть 410 Gone")
}
