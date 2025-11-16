package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAuthMiddleware(t *testing.T) {
	secretKey := "test-secret"
	cookieName := "auth_token"
	cookieMaxAge := 86400

	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name            string
		setupRequest    func() *http.Request
		checkResponse   func(t *testing.T, w *httptest.ResponseRecorder, userID string)
		expectNewCookie bool
	}{
		{
			name: "no cookie - creates new user",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				return req
			},
			expectNewCookie: true,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder, userID string) {
				assert.NotEmpty(t, userID)
				resp := w.Result()
				defer resp.Body.Close()
				cookies := resp.Cookies()
				require.Len(t, cookies, 1)
				assert.Equal(t, cookieName, cookies[0].Name)
			},
		},
		{
			name: "valid cookie - extracts user ID",
			setupRequest: func() *http.Request {
				token, _ := GenerateToken("existing-user-123", secretKey, 24*time.Hour)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.AddCookie(&http.Cookie{
					Name:  cookieName,
					Value: token,
				})
				return req
			},
			expectNewCookie: false,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder, userID string) {
				assert.Equal(t, "existing-user-123", userID)
			},
		},
		{
			name: "invalid cookie - creates new user",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.AddCookie(&http.Cookie{
					Name:  cookieName,
					Value: "invalid-token",
				})
				return req
			},
			expectNewCookie: true,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder, userID string) {
				assert.NotEmpty(t, userID)
				assert.NotEqual(t, "invalid-token", userID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedUserID string

			// Создаем тестовый handler, который захватывает userID из context
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				userID, ok := GetUserID(r.Context())
				require.True(t, ok, "userID should be in context")
				capturedUserID = userID
				w.WriteHeader(http.StatusOK)
			})

			// Оборачиваем в middleware
			middleware := AuthMiddleware(secretKey, cookieName, cookieMaxAge, logger)
			handler := middleware(testHandler)

			// Выполняем запрос
			req := tt.setupRequest()
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Проверяем результат
			tt.checkResponse(t, w, capturedUserID)
		})
	}
}

func TestGetUserID(t *testing.T) {
	secretKey := "test-secret"
	cookieName := "auth_token"
	cookieMaxAge := 86400

	logger, _ := zap.NewDevelopment()

	t.Run("extracts userID from context", func(t *testing.T) {
		var extractedUserID string

		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserID(r.Context())
			assert.True(t, ok)
			extractedUserID = userID
		})

		middleware := AuthMiddleware(secretKey, cookieName, cookieMaxAge, logger)
		handler := middleware(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.NotEmpty(t, extractedUserID)
	})
}
