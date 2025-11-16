package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	secretKey := "test-secret-key"
	userID := "test-user-123"
	duration := 24 * time.Hour

	token, err := GenerateToken(userID, secretKey, duration)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateToken(t *testing.T) {
	secretKey := "test-secret-key"
	userID := "test-user-123"
	duration := 24 * time.Hour

	tests := []struct {
		name      string
		setupFunc func() string
		wantErr   bool
		wantUser  string
	}{
		{
			name: "valid token",
			setupFunc: func() string {
				token, _ := GenerateToken(userID, secretKey, duration)
				return token
			},
			wantErr:  false,
			wantUser: userID,
		},
		{
			name: "invalid token",
			setupFunc: func() string {
				return "invalid.token.here"
			},
			wantErr: true,
		},
		{
			name: "token with wrong secret",
			setupFunc: func() string {
				token, _ := GenerateToken(userID, "wrong-secret", duration)
				return token
			},
			wantErr: true,
		},
		{
			name: "expired token",
			setupFunc: func() string {
				token, _ := GenerateToken(userID, secretKey, -1*time.Hour)
				return token
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.setupFunc()
			gotUserID, err := ValidateToken(token, secretKey)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantUser, gotUserID)
			}
		})
	}
}

func TestGenerateUserID(t *testing.T) {
	userID1 := GenerateUserID()
	userID2 := GenerateUserID()

	assert.NotEmpty(t, userID1)
	assert.NotEmpty(t, userID2)
	assert.NotEqual(t, userID1, userID2, "Generated user IDs should be unique")
}
