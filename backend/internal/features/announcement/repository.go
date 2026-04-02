package announcement

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// Announcement is the database model for a course announcement. Pinned
// announcements sort to the top of course listings. A nil PublishedAt means
// the announcement is a draft; a nil ExpiresAt means it never expires.
type Announcement struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	CourseID    uint64     `gorm:"index;not null"`
	AuthorID    uint64     `gorm:"index;not null"`
	Title       string     `gorm:"size:300;not null"`
	Body        string     `gorm:"type:text;not null"`
	Priority    string     `gorm:"size:20;not null;default:'normal'"` // normal | high | urgent
	IsPinned    bool       `gorm:"not null;default:false"`
	PublishedAt *time.Time // NULL = draft / not yet published
	ExpiresAt   *time.Time // NULL = no expiry
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Announcement model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Page describes a single page of paginated results.
type Page struct {
	Number int // 1-based page number
	Size   int // rows per page
}

// SearchFilter holds optional predicates for the Search query. Zero values
// mean the field is not filtered.
type SearchFilter struct {
	Title    string // LIKE %title%
	CourseID uint64 // exact match
	AuthorID uint64 // exact match
	Priority string // exact match
}

// FindByID returns the announcement with the given primary key, or nil if no
// row exists. Any unexpected database error is returned as a non-nil error.
func (r *Repository) FindByID(id uint64) (*Announcement, error) {
	var a Announcement
	err := dbretry.Do(func() error {
		return r.db.First(&a, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// FindByCourse returns a paginated page of announcements for the given course,
// ordered with pinned announcements first and then by most recent. It also
// returns the total count of matching rows for pagination metadata.
func (r *Repository) FindByCourse(courseID uint64, p Page) ([]*Announcement, int64, error) {
	var announcements []*Announcement
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Announcement{}).Where("course_id = ?", courseID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("is_pinned DESC, created_at DESC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&announcements).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return announcements, total, nil
}

// Search returns a paginated page of announcements matching all non-zero fields
// in the filter, along with the total count of matching rows.
func (r *Repository) Search(f SearchFilter, p Page) ([]*Announcement, int64, error) {
	var announcements []*Announcement
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Announcement{})
		if f.Title != "" {
			q = q.Where("title LIKE ?", "%"+f.Title+"%")
		}
		if f.CourseID != 0 {
			q = q.Where("course_id = ?", f.CourseID)
		}
		if f.AuthorID != 0 {
			q = q.Where("author_id = ?", f.AuthorID)
		}
		if f.Priority != "" {
			q = q.Where("priority = ?", f.Priority)
		}
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("created_at DESC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&announcements).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return announcements, total, nil
}

// Create inserts a new announcement row and returns the persisted record with
// its generated ID and timestamps populated.
func (r *Repository) Create(a *Announcement) (*Announcement, error) {
	err := dbretry.Do(func() error {
		return r.db.Create(a).Error
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Update overwrites the mutable fields of the announcement identified by id.
// The read and write are wrapped in a transaction with a SELECT FOR UPDATE lock
// to prevent lost updates. Returns nil, nil when no row with that id exists.
func (r *Repository) Update(id uint64, fields map[string]any) (*Announcement, error) {
	var a Announcement
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
				return err
			}
			result := tx.Model(&a).Updates(fields)
			if result.Error != nil {
				return result.Error
			}
			return nil
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindByID(id)
}

// Delete removes the announcement row with the given id. Returns true if a row
// was deleted, false if no matching row existed, and an error for any database
// failure.
func (r *Repository) Delete(id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Delete(&Announcement{}, id)
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		return nil
	})
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}
