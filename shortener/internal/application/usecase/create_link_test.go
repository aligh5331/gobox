package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
	"github.com/aligh5331/gobox/shortener/internal/domain/repository"
)

// mockSlugGen is a deterministic slug generator for tests.
type mockSlugGen struct {
	slug string
	err  error
}

func (m *mockSlugGen) Generate() (string, error) {
	return m.slug, m.err
}

func TestCreateLinkUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	slugs := &mockSlugGen{slug: "abc123"}
	uc := NewCreateLinkUseCase(repo, slugs, "http://localhost:8082")

	userID := uuid.New().String()
	fileID := uuid.New().String()

	repo.On("Create", mock.Anything, mock.MatchedBy(func(l *model.ShortLink) bool {
		return l.Slug == "abc123" && l.FileID.String() == fileID
	})).Return(nil)

	output, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    userID,
		FileID:    fileID,
		TargetURL: "http://example.com/file",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "abc123", output.Link.Slug)
	repo.AssertExpectations(t)
}

func TestCreateLinkUseCase_MissingFileID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	slugs := &mockSlugGen{slug: "abc123"}
	uc := NewCreateLinkUseCase(repo, slugs, "")

	_, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    uuid.New().String(),
		FileID:    "",
		TargetURL: "http://example.com/file",
	})

	assert.ErrorIs(t, err, ErrMissingFileID)
}

func TestCreateLinkUseCase_InvalidUserID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	slugs := &mockSlugGen{slug: "abc123"}
	uc := NewCreateLinkUseCase(repo, slugs, "")

	_, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    "not-a-uuid",
		FileID:    uuid.New().String(),
		TargetURL: "http://example.com/file",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid user_id")
}

func TestCreateLinkUseCase_InvalidFileID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	slugs := &mockSlugGen{slug: "abc123"}
	uc := NewCreateLinkUseCase(repo, slugs, "")

	_, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    uuid.New().String(),
		FileID:    "not-a-uuid",
		TargetURL: "http://example.com/file",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid file_id")
}

func TestCreateLinkUseCase_SlugCollisionThenSuccess(t *testing.T) {
	repo := new(MockShortLinkRepo)
	// Generate first collision, then success on second attempt
	slugs := &mockSlugGen{slug: "def456"}
	uc := NewCreateLinkUseCase(repo, slugs, "")

	userID := uuid.New().String()
	fileID := uuid.New().String()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.ShortLink")).
		Return(repository.ErrDuplicateSlug).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.ShortLink")).
		Return(nil).Once()

	output, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    userID,
		FileID:    fileID,
		TargetURL: "http://example.com/file",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "def456", output.Link.Slug)
	repo.AssertExpectations(t)
}

func TestCreateLinkUseCase_SlugCollisionExhausted(t *testing.T) {
	repo := new(MockShortLinkRepo)
	slugs := &mockSlugGen{slug: "ghi789"}
	uc := NewCreateLinkUseCase(repo, slugs, "")

	userID := uuid.New().String()
	fileID := uuid.New().String()

	// All attempts return duplicate slug error
	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.ShortLink")).
		Return(repository.ErrDuplicateSlug)

	_, err := uc.Execute(context.Background(), CreateLinkInput{
		UserID:    userID,
		FileID:    fileID,
		TargetURL: "http://example.com/file",
	})

	assert.ErrorIs(t, err, ErrSlugCollision)
}
