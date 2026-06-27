package usecase

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeCursor(t *testing.T) {
	originalID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	token := encodeCursor(originalID)
	assert.NotEmpty(t, token)

	decodedID, err := decodeCursor(token)
	assert.NoError(t, err)
	assert.Equal(t, originalID, decodedID)
}

func TestDecodeCursor_Empty(t *testing.T) {
	id, err := decodeCursor("")
	assert.NoError(t, err)
	assert.Equal(t, uuid.Nil, id)
}

func TestDecodeCursor_Invalid(t *testing.T) {
	_, err := decodeCursor("not-valid-base64!!")
	assert.Error(t, err)

	_, err = decodeCursor("{}")
	assert.Error(t, err)
}

func TestClampPageSize_Default(t *testing.T) {
	assert.Equal(t, DefaultPageSize, clampPageSize(0))
}

func TestClampPageSize_Negative(t *testing.T) {
	assert.Equal(t, DefaultPageSize, clampPageSize(-5))
}

func TestClampPageSize_MaxCap(t *testing.T) {
	assert.Equal(t, MaxPageSize, clampPageSize(500))
}

func TestClampPageSize_Valid(t *testing.T) {
	assert.Equal(t, 50, clampPageSize(50))
}
