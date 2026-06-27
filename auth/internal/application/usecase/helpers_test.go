package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid password", "ValidPass1", false},
		{"valid complex", "Abcdefg1", false},
		{"too short", "Ab1", true},
		{"no uppercase", "lowercase1", true},
		{"no lowercase", "UPPERCASE1", true},
		{"no digit", "NoDigitAb", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrWeakPassword)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	token, err := generateRefreshToken()
	assert.NoError(t, err)
	assert.Len(t, token, 43, "refresh token should be 43 base64url chars")
}

func TestHashToken(t *testing.T) {
	hash, err := hashToken("test-token-123")
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
