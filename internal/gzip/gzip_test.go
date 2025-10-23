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

// TestCompressWriter тестирует реализацию compressWriter
func TestCompressWriter(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		content    string
		shouldGzip bool
	}{
		{
			name:       "Status OK должен сжимать",
			statusCode: http.StatusOK,
			content:    "Hello, World!",
			shouldGzip: true,
		},
		{
			name:       "Status Created должен сжимать",
			statusCode: http.StatusCreated,
			content:    "Created resource",
			shouldGzip: true,
		},
		{
			name:       "Status Bad Request не должен сжимать",
			statusCode: http.StatusBadRequest,
			content:    "Bad request",
			shouldGzip: false,
		},
		{
			name:       "Status Internal Server Error не должен сжимать",
			statusCode: http.StatusInternalServerError,
			content:    "Server error",
			shouldGzip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем тестовый recorder для захвата ответа
			recorder := httptest.NewRecorder()

			// Создаем compress writer
			cw := newCompressWriter(recorder)

			// Тестируем метод Header
			headers := cw.Header()
			if headers == nil {
				t.Error("Header() вернул nil")
			}

			// Устанавливаем content type
			headers.Set("Content-Type", "text/plain")

			// Тестируем WriteHeader
			cw.WriteHeader(tt.statusCode)

			// Тестируем Write
			contentBytes := []byte(tt.content)
			written, err := cw.Write(contentBytes)
			if err != nil {
				t.Errorf("Write() завершился ошибкой: %v", err)
			}
			if written != len(contentBytes) {
				t.Errorf("Write() записал %d байт, ожидалось %d", written, len(contentBytes))
			}

			// Закрываем writer для выгрузки данных
			err = cw.Close()
			if err != nil {
				t.Errorf("Close() завершился ошибкой: %v", err)
			}

			// Проверяем, что content encoding установлен корректно
			contentEncoding := recorder.Header().Get("Content-Encoding")
			if tt.shouldGzip {
				if contentEncoding != "gzip" {
					t.Errorf("Ожидался Content-Encoding 'gzip', получен '%s'", contentEncoding)
				}

				// Проверяем, что контент действительно сжат
				body := recorder.Body.Bytes()
				if len(body) >= len(tt.content) {
					t.Error("Контент не был сжат - размер сжатых данных не меньше оригинала")
				}

				// Распаковываем и проверяем содержимое
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Errorf("Не удалось создать gzip reader: %v", err)
				}
				defer reader.Close()

				decompressed, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Не удалось распаковать: %v", err)
				}

				if string(decompressed) != tt.content {
					t.Errorf("Несоответствие распакованного контента: получен '%s', ожидался '%s'", string(decompressed), tt.content)
				}
			} else {
				if contentEncoding == "gzip" {
					t.Error("Content-Encoding не должен быть 'gzip' для статус-кодов ошибок")
				}
				// Для несжатых ответов контент должен быть как записанный
				if recorder.Body.String() != tt.content {
					t.Errorf("Несоответствие контента: получен '%s', ожидался '%s'", recorder.Body.String(), tt.content)
				}
			}
		})
	}
}

// TestCompressReader тестирует реализацию compressReader
func TestCompressReader(t *testing.T) {
	originalContent := "Это тестовый контент, который будет сжат"

	// Создаем сжатые данные
	var compressedBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&compressedBuf)
	_, err := gzWriter.Write([]byte(originalContent))
	if err != nil {
		t.Fatalf("Не удалось сжать тестовые данные: %v", err)
	}
	gzWriter.Close()

	// Тестируем успешную распаковку
	t.Run("Успешная распаковка", func(t *testing.T) {
		originalReader := io.NopCloser(bytes.NewReader(compressedBuf.Bytes()))

		cr, err := newCompressReader(originalReader)
		if err != nil {
			t.Fatalf("newCompressReader завершился ошибкой: %v", err)
		}
		defer cr.Close()

		decompressed, err := io.ReadAll(cr)
		if err != nil {
			t.Fatalf("Read завершился ошибкой: %v", err)
		}

		if string(decompressed) != originalContent {
			t.Errorf("Несоответствие распакованного контента: получен '%s', ожидался '%s'", string(decompressed), originalContent)
		}
	})

	// Тестируем с некорректными gzip данными
	t.Run("Некорректные gzip данные", func(t *testing.T) {
		invalidReader := io.NopCloser(bytes.NewReader([]byte("не gzip данные")))

		_, err := newCompressReader(invalidReader)
		if err == nil {
			t.Error("Ожидалась ошибка с некорректными gzip данными, но ошибки не было")
		}
	})
}

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
			handler := gzipMiddleware(testHandler)
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

		handler := gzipMiddleware(testHandler)
		handler.ServeHTTP(recorder, req)

		// Должен вернуть 500 Internal Server Error
		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("Ожидался статус 500 для поврежденных gzip данных, получен %d", recorder.Code)
		}
	})
}
