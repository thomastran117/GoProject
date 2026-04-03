package assignment

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// Assignment is the database model for a course assignment. A nil DueAt means
// there is no deadline. Status tracks the assignment lifecycle: draft,
// published, or closed.
type Assignment struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	CourseID    uint64     `gorm:"index;not null"`
	AuthorID    uint64     `gorm:"index;not null"`
	Title       string     `gorm:"size:300;not null"`
	Description string     `gorm:"type:text;not null"`
	DueAt       *time.Time // NULL = no deadline
	Points      uint       `gorm:"not null;default:0"`
	Status      string     `gorm:"size:20;not null;default:'draft'"` // draft | published | closed
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AssignmentView records that a user has viewed a specific assignment.
// The unique index on (user_id, assignment_id) ensures at-most-one row per pair.
type AssignmentView struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	UserID       uint64    `gorm:"index:idx_assignment_view_user_assignment,unique;not null"`
	AssignmentID uint64    `gorm:"index:idx_assignment_view_user_assignment,unique;not null"`
	ViewedAt     time.Time `gorm:"not null"`
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Assignment model.
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
	Status   string // exact match
}

// FindByID returns the assignment with the given primary key, or nil if no row
// exists.
func (r *Repository) FindByID(id uint64) (*Assignment, error) {
	var a Assignment
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

// FindByCourse returns a paginated page of assignments for the given course,
// ordered by most recent. It also returns the total count for pagination metadata.
func (r *Repository) FindByCourse(courseID uint64, p Page) ([]*Assignment, int64, error) {
	var assignments []*Assignment
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Assignment{}).Where("course_id = ?", courseID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("created_at DESC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&assignments).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return assignments, total, nil
}

// Search returns a paginated page of assignments matching all non-zero fields
// in the filter, along with the total count of matching rows.
func (r *Repository) Search(f SearchFilter, p Page) ([]*Assignment, int64, error) {
	var assignments []*Assignment
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Assignment{})
		if f.Title != "" {
			q = q.Where("title LIKE ?", "%"+f.Title+"%")
		}
		if f.CourseID != 0 {
			q = q.Where("course_id = ?", f.CourseID)
		}
		if f.AuthorID != 0 {
			q = q.Where("author_id = ?", f.AuthorID)
		}
		if f.Status != "" {
			q = q.Where("status = ?", f.Status)
		}
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("created_at DESC").
			Limit(p.Size).
			Offset((p.Number - 1) * p.Size).
			Find(&assignments).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return assignments, total, nil
}

// Create inserts a new assignment row and returns the persisted record with its
// generated ID and timestamps populated.
func (r *Repository) Create(a *Assignment) (*Assignment, error) {
	err := dbretry.Do(func() error {
		return r.db.Create(a).Error
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Update overwrites the mutable fields of the assignment identified by id.
// The read and write are wrapped in a transaction with a SELECT FOR UPDATE lock
// to prevent lost updates. Returns nil, nil when no row with that id exists.
func (r *Repository) Update(id uint64, fields map[string]any) (*Assignment, error) {
	var a Assignment
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
				return err
			}
			return tx.Model(&a).Updates(fields).Error
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

// MarkViewed records that userID has viewed assignmentID.
// If a row already exists the insert is silently ignored (idempotent).
func (r *Repository) MarkViewed(userID, assignmentID uint64) error {
	return dbretry.Do(func() error {
		return r.db.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&AssignmentView{UserID: userID, AssignmentID: assignmentID, ViewedAt: time.Now()}).Error
	})
}

// FindViewedIDs returns a set of assignment IDs (from the provided ids slice)
// that userID has already viewed.
func (r *Repository) FindViewedIDs(userID uint64, ids []uint64) (map[uint64]bool, error) {
	result := make(map[uint64]bool)
	if len(ids) == 0 {
		return result, nil
	}
	var viewedIDs []uint64
	err := dbretry.Do(func() error {
		return r.db.Model(&AssignmentView{}).
			Where("user_id = ? AND assignment_id IN ?", userID, ids).
			Pluck("assignment_id", &viewedIDs).Error
	})
	if err != nil {
		return nil, err
	}
	for _, id := range viewedIDs {
		result[id] = true
	}
	return result, nil
}

// Delete removes the assignment row with the given id. Returns true if a row
// was deleted, false if no matching row existed, and an error for any database
// failure.
func (r *Repository) Delete(id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Delete(&Assignment{}, id)
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
