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

func (r *Repository) FindByID(id uint64) (*User, error) {
	var u User
	result := r.db.First(&u, id)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &u, nil
}

// FindOrCreateByEmail looks up a user by email; if not found, creates one with an
// empty password hash (OAuth users have no password) and the default "user" role.
func (r *Repository) FindOrCreateByEmail(email string) (*User, error) {
	u, err := r.FindByEmail(email)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}
	return r.Create(email, "", "user")
}

func (r *Repository) Create(email, passwordHash, role string) (*User, error) {
	u := &User{
		Email:        email,
		PasswordHash: passwordHash,
		Role: role,
	}
	if result := r.db.Create(u); result.Error != nil {
		return nil, result.Error
	}
	return u, nil
}
