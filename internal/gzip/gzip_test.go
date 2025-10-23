package gzip

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGzipMiddleware тестирует полную функциональность middleware
func TestGzipMiddleware(t *testing.T) {
	// Создаем простой handler для тестирования
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "test response"}`))
	})

	tests := []struct {
		name               string
		acceptEncoding     string
		contentType        string
		contentEncoding    string
		requestBody        string
		compressBody       bool
		expectedCompressed bool
	}{
		{
			name:               "Клиент поддерживает gzip с JSON ответом",
			acceptEncoding:     "gzip",
			contentType:        "application/json",
			requestBody:        "",
			expectedCompressed: true,
		},
		{
			name:               "Клиент поддерживает gzip с HTML ответом",
			acceptEncoding:     "gzip",
			contentType:        "text/html",
			requestBody:        "",
			expectedCompressed: true,
		},
		{
			name:               "Клиент не поддерживает gzip",
			acceptEncoding:     "",
			contentType:        "application/json",
			requestBody:        "",
			expectedCompressed: false,
		},
		{
			name:               "Несжимаемый content type",
			acceptEncoding:     "gzip",
			contentType:        "image/png",
			requestBody:        "",
			expectedCompressed: false,
		},
		{
			name:               "Клиент отправляет gzip сжатый запрос",
			acceptEncoding:     "gzip",
			contentType:        "application/json",
			contentEncoding:    "gzip",
			requestBody:        "сжатые данные",
			compressBody:       true,
			expectedCompressed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.compressBody {
				// Сжимаем тело запроса
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				gz.Write([]byte(tt.requestBody))
				gz.Close()
				body = &buf
			} else {
				body = strings.NewReader(tt.requestBody)
			}

			// Создаем тестовый запрос
			req := httptest.NewRequest("POST", "/test", body)
			req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			req.Header.Set("Content-Type", tt.contentType)
			if tt.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tt.contentEncoding)
			}

			// Создаем response recorder
			recorder := httptest.NewRecorder()

			// Применяем middleware и обрабатываем запрос
			handler := GzipMiddleware(testHandler)
			handler.ServeHTTP(recorder, req)

			// Проверяем ответ
			if tt.expectedCompressed {
				contentEncoding := recorder.Header().Get("Content-Encoding")
				if contentEncoding != "gzip" {
					t.Errorf("Ожидался сжатый ответ с Content-Encoding 'gzip', получен '%s'", contentEncoding)
				}
			} else {
				contentEncoding := recorder.Header().Get("Content-Encoding")
				if contentEncoding == "gzip" {
					t.Error("Ответ не должен быть сжат, но Content-Encoding установлен в 'gzip'")
				}
			}

			// Проверяем статус код
			if recorder.Code != http.StatusOK {
				t.Errorf("Ожидался статус 200, получен %d", recorder.Code)
			}
		})
	}
}

// TestGzipMiddlewareErrorHandling тестирует обработку ошибок в middleware
func TestGzipMiddlewareErrorHandling(t *testing.T) {
	// Тестируем с поврежденным gzip телом запроса
	t.Run("Поврежденное gzip тело запроса", func(t *testing.T) {
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler не должен вызываться с поврежденными gzip данными")
		})

		req := httptest.NewRequest("POST", "/test", strings.NewReader("поврежденные gzip данные"))
		req.Header.Set("Content-Encoding", "gzip")

		recorder := httptest.NewRecorder()

		handler := GzipMiddleware(testHandler)
		handler.ServeHTTP(recorder, req)

		// Должен вернуть 500 Internal Server Error
		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("Ожидался статус 500 для поврежденных gzip данных, получен %d", recorder.Code)
		}
	})
}
