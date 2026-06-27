// Package postgres implements domain repository interfaces using GORM.
package postgres

import (
	"time"

	"github.com/google/uuid"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
)

// UserModel is the GORM mapping for the users table.
// It is separate from the domain model to avoid coupling domain to GORM.
type UserModel struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string    `gorm:"uniqueIndex;not null"`
	Name         string    `gorm:"not null"`
	PasswordHash string    `gorm:"column:password_hash;not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

// TableName overrides the default table name.
func (UserModel) TableName() string {
	return "users"
}

func toDomainUser(m *UserModel) *model.User {
	return &model.User{
		ID:           uuid.MustParse(m.ID),
		Email:        m.Email,
		Name:         m.Name,
		PasswordHash: m.PasswordHash,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toGormUser(d *model.User) *UserModel {
	return &UserModel{
		ID:           d.ID.String(),
		Email:        d.Email,
		Name:         d.Name,
		PasswordHash: d.PasswordHash,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

// SessionModel is the GORM mapping for the sessions table.
type SessionModel struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID           string    `gorm:"type:uuid;column:user_id;not null;index"`
	RefreshTokenHash string    `gorm:"column:refresh_token;not null"`
	UserAgent        string    `gorm:"not null;default:''"`
	IP               string    `gorm:"type:varchar(45);not null;default:''"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	LastUsedAt       time.Time `gorm:"autoUpdateTime"`
	ExpiresAt        time.Time `gorm:"not null"`
	Revoked          bool      `gorm:"not null;default:false"`
}

// TableName overrides the default table name.
func (SessionModel) TableName() string {
	return "sessions"
}

func toDomainSession(m *SessionModel) *model.Session {
	return &model.Session{
		ID:               uuid.MustParse(m.ID),
		UserID:           uuid.MustParse(m.UserID),
		RefreshTokenHash: m.RefreshTokenHash,
		UserAgent:        m.UserAgent,
		IP:               m.IP,
		CreatedAt:        m.CreatedAt,
		LastUsedAt:       m.LastUsedAt,
		ExpiresAt:        m.ExpiresAt,
		Revoked:          m.Revoked,
	}
}

func toGormSession(d *model.Session) *SessionModel {
	return &SessionModel{
		ID:               d.ID.String(),
		UserID:           d.UserID.String(),
		RefreshTokenHash: d.RefreshTokenHash,
		UserAgent:        d.UserAgent,
		IP:               d.IP,
		CreatedAt:        d.CreatedAt,
		LastUsedAt:       d.LastUsedAt,
		ExpiresAt:        d.ExpiresAt,
		Revoked:          d.Revoked,
	}
}
