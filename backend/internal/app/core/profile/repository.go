package profile

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/config/middleware"
)

// Profile is the database model for a user profile. Each profile is linked
// 1-to-1 with an auth user via UserID and carries a unique display username
// and an optional avatar URL.
type Profile struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"uniqueIndex;not null"`
	Username  string    `gorm:"uniqueIndex;size:100;not null"`
	AvatarURL string    `gorm:"column:avatar_url;size:2048"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Profile model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// FindByID returns the profile with the given primary key, or nil if no row
// exists. Any unexpected database error is returned as a non-nil error.
func (r *Repository) FindByID(id uint64) (*Profile, error) {
	var p Profile
	result := r.db.First(&p, id)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &p, nil
}

// FindAll returns every profile row in the table. Returns an empty slice (not
// nil) when no profiles exist.
func (r *Repository) FindAll() ([]*Profile, error) {
	var profiles []*Profile
	if err := r.db.Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// FindByIDs returns all profiles whose primary key is in the given slice.
// Rows that do not exist are silently omitted from the result; the caller
// should compare lengths if exact matching is required.
func (r *Repository) FindByIDs(ids []uint64) ([]*Profile, error) {
	var profiles []*Profile
	if err := r.db.Where("id IN ?", ids).Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// Create inserts a new profile row and returns the persisted record with its
// generated ID and timestamps populated. If a profile already exists for the
// given userID or username (unique constraint violation), a 409 APIError is
// returned so the caller receives a clean conflict response rather than a raw
// database error.
func (r *Repository) Create(userID uint64, username, avatarURL string) (*Profile, error) {
	p := &Profile{
		UserID:    userID,
		Username:  username,
		AvatarURL: avatarURL,
	}
	if err := r.db.Create(p).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "PROFILE_ALREADY_EXISTS",
				Message: "A profile with that user ID or username already exists",
			}
		}
		return nil, err
	}
	return p, nil
}

// Update overwrites the username and avatar_url of the profile identified by
// id. The read and write are wrapped in a transaction with a SELECT FOR UPDATE
// lock to prevent lost updates and TOCTOU races. After the transaction commits,
// the row is re-fetched so the returned struct reflects the actual committed
// state (including the database-assigned UpdatedAt). Returns nil, nil when no
// row with that id exists.
func (r *Repository) Update(id uint64, username, avatarURL string) (*Profile, error) {
	var p Profile
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return err
		}
		result := tx.Model(&p).Updates(map[string]any{
			"username":   username,
			"avatar_url": avatarURL,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindByID(id)
}

// Delete removes the profile row with the given id. Returns true if a row was
// deleted, false if no matching row existed, and an error for any database
// failure.
func (r *Repository) Delete(id uint64) (bool, error) {
	result := r.db.Delete(&Profile{}, id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
