package lecture

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// Lecture is the database model for a course lecture.
type Lecture struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	CourseID  uint64    `gorm:"index;not null"`
	AuthorID  uint64    `gorm:"index;not null"`
	Title     string    `gorm:"size:300;not null"`
	Content   string    `gorm:"type:text;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Lecture model.
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
}

// FindByID returns the lecture with the given primary key, or nil if no
// row exists.
func (r *Repository) FindByID(id uint64) (*Lecture, error) {
	var l Lecture
	err := dbretry.Do(func() error {
		return r.db.First(&l, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// FindByCourse returns a paginated page of lectures for the given course,
// ordered by creation time. It also returns the total count of matching rows.
func (r *Repository) FindByCourse(courseID uint64, p Page) ([]*Lecture, int64, error) {
	var lectures []*Lecture
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Lecture{}).Where("course_id = ?", courseID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("created_at ASC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&lectures).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return lectures, total, nil
}

// Search returns a paginated page of lectures matching all non-zero fields
// in the filter, along with the total count of matching rows.
func (r *Repository) Search(f SearchFilter, p Page) ([]*Lecture, int64, error) {
	var lectures []*Lecture
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Lecture{})
		if f.Title != "" {
			q = q.Where("title LIKE ?", "%"+f.Title+"%")
		}
		if f.CourseID != 0 {
			q = q.Where("course_id = ?", f.CourseID)
		}
		if f.AuthorID != 0 {
			q = q.Where("author_id = ?", f.AuthorID)
		}
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("created_at DESC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&lectures).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return lectures, total, nil
}

// Create inserts a new lecture row and returns the persisted record.
func (r *Repository) Create(l *Lecture) (*Lecture, error) {
	err := dbretry.Do(func() error {
		return r.db.Create(l).Error
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Update overwrites the mutable fields of the lecture identified by id.
// Returns nil, nil when no row with that id exists.
func (r *Repository) Update(id uint64, fields map[string]any) (*Lecture, error) {
	var l Lecture
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&l, id).Error; err != nil {
				return err
			}
			return tx.Model(&l).Updates(fields).Error
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

// Delete removes the lecture row with the given id. Returns true if a row
// was deleted, false if no matching row existed.
func (r *Repository) Delete(id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Delete(&Lecture{}, id)
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
