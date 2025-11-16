package auth

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type contextKey string

const UserIDContextKey contextKey = "userID"

// AuthMiddleware middleware для аутентификации пользователя через JWT
func AuthMiddleware(secretKey string, cookieName string, cookieMaxAge int, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var userID string

			// Пытаемся получить куку
			cookie, err := r.Cookie(cookieName)
			if err == nil && cookie.Value != "" {
				// Кука существует, валидируем токен
				userID, err = ValidateToken(cookie.Value, secretKey)
				if err != nil {
					logger.Debug("invalid token, generating new one", zap.Error(err))
					userID = "" // Сбрасываем, создадим новый
				}
			}

			// Если userID пустой (нет куки или токен невалидный), создаем новый
			if userID == "" {
				userID = GenerateUserID()
				token, err := GenerateToken(userID, secretKey, time.Duration(cookieMaxAge)*time.Second)
				if err != nil {
					logger.Error("failed to generate token", zap.Error(err))
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				// Устанавливаем куку
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    token,
					Path:     "/",
					MaxAge:   cookieMaxAge,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})

				logger.Debug("new user created", zap.String("userID", userID))
			}

			// Добавляем userID в context
			ctx := context.WithValue(r.Context(), UserIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID извлекает userID из context
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDContextKey).(string)
	return userID, ok
}
