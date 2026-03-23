package auth

import (
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
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

// FindOrCreateByEmail atomically finds or creates an OAuth user.
// It attempts the insert first; if a duplicate-key error occurs (concurrent
// request already created the row), it falls back to a lookup.
func (r *Repository) FindOrCreateByEmail(email string) (*User, error) {
	u, err := r.Create(email, "", "user")
	if err == nil {
		return u, nil
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		// Row was inserted by a concurrent request; fetch it.
		return r.FindByEmail(email)
	}

	return nil, err
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
