package course

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/application/middleware"
	"backend/internal/utilities/dbretry"
)

// Course is the database model for a course entity. Each course belongs to a
// school and is taught by a teacher. Course codes are unique per school.
type Course struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	SchoolID      uint64     `gorm:"uniqueIndex:uq_school_code;index;not null"`
	TeacherID     uint64     `gorm:"index;not null"`
	Name          string     `gorm:"size:200;not null"`
	Code          string     `gorm:"uniqueIndex:uq_school_code;size:20;not null"`
	Description   string     `gorm:"size:2000"`
	Subject       string     `gorm:"size:100;index"`
	GradeLevel    string     `gorm:"size:50"`
	Language      string     `gorm:"size:50"`
	Room          string     `gorm:"size:100"`
	Schedule      string     `gorm:"size:500"`
	MaxEnrollment uint
	Credits       uint
	Status        string     `gorm:"size:20;not null;default:'active'"`
	Visibility    string     `gorm:"size:10;not null;default:'public'"`
	StartDate     *time.Time
	EndDate       *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Course model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// FindByID returns the course with the given primary key, or nil if no row
// exists. Any unexpected database error is returned as a non-nil error.
func (r *Repository) FindByID(id uint64) (*Course, error) {
	var c Course
	err := dbretry.Do(func() error {
		return r.db.First(&c, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindByIDs returns courses whose primary key is in the given slice, ordered
// to match the input slice. Rows that do not exist are silently omitted.
func (r *Repository) FindByIDs(ids []uint64) ([]*Course, error) {
	var courses []*Course
	err := dbretry.Do(func() error {
		return r.db.Where("id IN ?", ids).Find(&courses).Error
	})
	if err != nil {
		return nil, err
	}
	index := make(map[uint64]*Course, len(courses))
	for _, c := range courses {
		index[c.ID] = c
	}
	ordered := make([]*Course, 0, len(ids))
	for _, id := range ids {
		if c, ok := index[id]; ok {
			ordered = append(ordered, c)
		}
	}
	return ordered, nil
}

// FindBySchoolAndCode returns the course with the given school and code, or
// nil if none exists. Used to pre-validate uniqueness before a write.
func (r *Repository) FindBySchoolAndCode(schoolID uint64, code string) (*Course, error) {
	var c Course
	err := dbretry.Do(func() error {
		return r.db.Where("school_id = ? AND code = ?", schoolID, code).First(&c).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// SearchFilter holds optional predicates for the Search query. Zero values
// mean the field is not filtered.
type SearchFilter struct {
	Name       string // LIKE %name%
	Code       string // LIKE %code%
	SchoolID   uint64 // exact match
	TeacherID  uint64 // exact match
	Subject    string // exact match
	GradeLevel string // exact match
	Status     string // exact match
	Language   string // exact match
}

// Search returns courses matching all non-zero fields in the filter.
func (r *Repository) Search(f SearchFilter) ([]*Course, error) {
	var courses []*Course
	err := dbretry.Do(func() error {
		q := r.db.Model(&Course{})
		if f.Name != "" {
			q = q.Where("name LIKE ?", "%"+f.Name+"%")
		}
		if f.Code != "" {
			q = q.Where("code LIKE ?", "%"+f.Code+"%")
		}
		if f.SchoolID != 0 {
			q = q.Where("school_id = ?", f.SchoolID)
		}
		if f.TeacherID != 0 {
			q = q.Where("teacher_id = ?", f.TeacherID)
		}
		if f.Subject != "" {
			q = q.Where("LOWER(subject) = LOWER(?)", f.Subject)
		}
		if f.GradeLevel != "" {
			q = q.Where("LOWER(grade_level) = LOWER(?)", f.GradeLevel)
		}
		if f.Status != "" {
			q = q.Where("status = ?", f.Status) // status is a controlled enum; exact match is intentional
		}
		if f.Language != "" {
			q = q.Where("LOWER(language) = LOWER(?)", f.Language)
		}
		return q.Find(&courses).Error
	})
	if err != nil {
		return nil, err
	}
	return courses, nil
}

// Create inserts a new course row and returns the persisted record with its
// generated ID and timestamps populated. If a course with the same code
// already exists in the school, a 409 APIError is returned.
func (r *Repository) Create(c *Course) (*Course, error) {
	err := dbretry.Do(func() error {
		return r.db.Create(c).Error
	})
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "COURSE_CODE_CONFLICT",
				Message: "A course with that code already exists in this school",
			}
		}
		return nil, err
	}
	return c, nil
}

// Update overwrites the mutable fields of the course identified by id. The
// read and write are wrapped in a transaction with a SELECT FOR UPDATE lock
// to prevent lost updates. Returns nil, nil when no row with that id exists.
func (r *Repository) Update(id uint64, fields map[string]any) (*Course, error) {
	var c Course
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, id).Error; err != nil {
				return err
			}
			result := tx.Model(&c).Updates(fields)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "COURSE_CODE_CONFLICT",
				Message: "A course with that code already exists in this school",
			}
		}
		return nil, err
	}
	return r.FindByID(id)
}

// Delete removes the course row with the given id. Returns true if a row was
// deleted, false if no matching row existed, and an error for any database
// failure.
func (r *Repository) Delete(id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Delete(&Course{}, id)
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
