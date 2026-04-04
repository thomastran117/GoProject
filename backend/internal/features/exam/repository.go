package exam

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"backend/internal/utilities/dbretry"
)

// Exam is the database model for a course exam.
type Exam struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	CourseID  uint64    `gorm:"index;not null"`
	Title     string    `gorm:"size:300;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository wraps the GORM database connection and provides persistence
// operations for the Exam model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// FindByID returns the exam with the given primary key, or nil if no row exists.
func (r *Repository) FindByID(id uint64) (*Exam, error) {
	var e Exam
	err := dbretry.Do(func() error {
		return r.db.First(&e, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
