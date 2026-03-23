package auth

import (
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	Email        string    `gorm:"uniqueIndex;size:255;not null"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	Role         string    `gorm:";size:255;not null"`
	CreatedAt    time.Time
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByEmail(email string) (*User, error) {
	var u User
	result := r.db.Where("email = ?", email).First(&u)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &u, nil
}

func (r *Repository) Create(email, passwordHash string) (*User, error) {
	u := &User{
		Email:        email,
		PasswordHash: passwordHash,
	}
	if result := r.db.Create(u); result.Error != nil {
		return nil, result.Error
	}
	return u, nil
}
