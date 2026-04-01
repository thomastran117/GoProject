package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

type Repository struct {
	db *gorm.DB
}

type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	Email        string    `gorm:"uniqueIndex;size:255;not null"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	Role         string    `gorm:";size:255;not null"`
	SchoolID     *uint64   `gorm:"column:school_id;index"`
	GoogleID     *string   `gorm:"column:google_id;uniqueIndex;size:255"`
	MicrosoftID  *string   `gorm:"column:microsoft_id;uniqueIndex;size:255"`
	CreatedAt    time.Time
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByEmail(email string) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.Where("email = ?", email).First(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) FindByGoogleID(googleID string) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.Where("google_id = ?", googleID).First(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) FindByID(id uint64) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.First(&u, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindOrCreateByGoogleID finds or creates a user for a Google sign-in.
// Lookup order:
//  1. By Google sub — the stable, permanent Google user ID.
//  2. By email — links an existing password account on first Google sign-in,
//     and writes the Google sub back so future logins hit path 1.
//  3. Insert — new user; falls back to a lookup on duplicate-key to handle
//     a concurrent request racing to create the same row.
func (r *Repository) FindOrCreateByGoogleID(googleID, email string) (*User, error) {
	if u, err := r.FindByGoogleID(googleID); err != nil || u != nil {
		return u, err
	}

	if u, err := r.FindByEmail(email); err != nil {
		return nil, err
	} else if u != nil {
		return r.linkGoogleID(u.ID, googleID)
	}

	u, err := r.createWithGoogleID(email, googleID)
	if err == nil {
		return u, nil
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		// Concurrent request won the race; fetch whichever row was created.
		if u, err := r.FindByGoogleID(googleID); err != nil || u != nil {
			return u, err
		}
		return r.FindByEmail(email)
	}

	return nil, err
}

// FindOrCreateByMicrosoftID finds or creates a user for a Microsoft sign-in.
// Uses the same insert-first strategy as FindOrCreateByGoogleID.
func (r *Repository) FindOrCreateByMicrosoftID(microsoftID, email string) (*User, error) {
	if u, err := r.FindByMicrosoftID(microsoftID); err != nil || u != nil {
		return u, err
	}

	if u, err := r.FindByEmail(email); err != nil {
		return nil, err
	} else if u != nil {
		return r.linkMicrosoftID(u.ID, microsoftID)
	}

	u, err := r.createWithMicrosoftID(email, microsoftID)
	if err == nil {
		return u, nil
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		if u, err := r.FindByMicrosoftID(microsoftID); err != nil || u != nil {
			return u, err
		}
		return r.FindByEmail(email)
	}

	return nil, err
}

func (r *Repository) FindByMicrosoftID(microsoftID string) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.Where("microsoft_id = ?", microsoftID).First(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// linkGoogleID atomically sets google_id on an existing user row.
// SELECT FOR UPDATE prevents two concurrent requests from both reading a nil
// google_id and issuing duplicate writes.
func (r *Repository) linkGoogleID(userID uint64, googleID string) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID).Error; err != nil {
				return err
			}
			if u.GoogleID != nil {
				return nil // already linked — idempotent
			}
			if err := tx.Model(&u).Update("google_id", googleID).Error; err != nil {
				return err
			}
			u.GoogleID = &googleID
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// linkMicrosoftID atomically sets microsoft_id on an existing user row.
func (r *Repository) linkMicrosoftID(userID uint64, microsoftID string) (*User, error) {
	var u User
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID).Error; err != nil {
				return err
			}
			if u.MicrosoftID != nil {
				return nil // already linked — idempotent
			}
			if err := tx.Model(&u).Update("microsoft_id", microsoftID).Error; err != nil {
				return err
			}
			u.MicrosoftID = &microsoftID
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) createWithMicrosoftID(email, microsoftID string) (*User, error) {
	u := &User{
		Email:        email,
		PasswordHash: "",
		Role:         RolePending,
		MicrosoftID:  &microsoftID,
	}
	err := dbretry.Do(func() error {
		return r.db.Create(u).Error
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) createWithGoogleID(email, googleID string) (*User, error) {
	u := &User{
		Email:        email,
		PasswordHash: "",
		Role:         RolePending,
		GoogleID:     &googleID,
	}
	err := dbretry.Do(func() error {
		return r.db.Create(u).Error
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) UpdateRole(userID uint64, role string) (*User, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Model(&User{}).Where("id = ?", userID).Update("role", role)
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("auth: UpdateRole: no rows affected for user %d", userID)
	}
	return r.FindByID(userID)
}

func (r *Repository) Create(email, passwordHash, role string, schoolID *uint64) (*User, error) {
	u := &User{
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
		SchoolID:     schoolID,
	}
	err := dbretry.Do(func() error {
		return r.db.Create(u).Error
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}
