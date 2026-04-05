package grade

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// Grade is the database model for a student grade within a course.
// Exactly one of AssignmentID, QuizID, TestID, ExamID must be non-nil,
// linking the grade to a specific piece of course content.
type Grade struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	CourseID     uint64    `gorm:"index;not null"`
	StudentID    uint64    `gorm:"index;not null"`
	AssignmentID *uint64   `gorm:"index"` // FK → assignments
	QuizID       *uint64   `gorm:"index"` // FK → quizzes
	TestID       *uint64   `gorm:"index"` // FK → tests
	ExamID       *uint64   `gorm:"index"` // FK → exams
	Title        string    `gorm:"size:300;not null"`
	Score        float64   `gorm:"not null;default:0"`
	MaxScore     float64   `gorm:"not null;default:100"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the Grade model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new grade row and returns the persisted record with its
// generated ID and timestamps populated.
func (r *Repository) Create(ctx context.Context, g *Grade) (*Grade, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(g).Error
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

// FindByID returns the grade with the given primary key, or nil if no row exists.
func (r *Repository) FindByID(ctx context.Context, id uint64) (*Grade, error) {
	var g Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&g, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// FindByCourse returns all grades for the given course ordered by student_id
// ascending, then created_at ascending.
func (r *Repository) FindByCourse(ctx context.Context, courseID uint64) ([]*Grade, error) {
	var grades []*Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("course_id = ?", courseID).
			Order("student_id ASC, created_at ASC").
			Find(&grades).Error
	})
	if err != nil {
		return nil, err
	}
	return grades, nil
}

// FindByCourseAndStudent returns all grades for a specific student in a course
// ordered by created_at ascending.
func (r *Repository) FindByCourseAndStudent(ctx context.Context, courseID, studentID uint64) ([]*Grade, error) {
	var grades []*Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("course_id = ? AND student_id = ?", courseID, studentID).
			Order("created_at ASC").
			Find(&grades).Error
	})
	if err != nil {
		return nil, err
	}
	return grades, nil
}

// Update overwrites mutable fields of the grade identified by id inside a
// SELECT FOR UPDATE transaction. Returns nil, nil when no matching row exists.
func (r *Repository) Update(ctx context.Context, id uint64, fields map[string]any) (*Grade, error) {
	var g Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&g, id).Error; err != nil {
				return err
			}
			return tx.Model(&g).Updates(fields).Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

// FindByQuizAndStudent returns the grade linked to the given quiz for a specific
// student, or nil if no such row exists.
func (r *Repository) FindByQuizAndStudent(ctx context.Context, quizID, studentID uint64) (*Grade, error) {
	var g Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("quiz_id = ? AND student_id = ?", quizID, studentID).
			First(&g).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// FindByTestAndStudent returns the grade linked to the given test for a specific
// student, or nil if no such row exists.
func (r *Repository) FindByTestAndStudent(ctx context.Context, testID, studentID uint64) (*Grade, error) {
	var g Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("test_id = ? AND student_id = ?", testID, studentID).
			First(&g).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// FindByExamAndStudent returns the grade linked to the given exam for a specific
// student, or nil if no such row exists.
func (r *Repository) FindByExamAndStudent(ctx context.Context, examID, studentID uint64) (*Grade, error) {
	var g Grade
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("exam_id = ? AND student_id = ?", examID, studentID).
			First(&g).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// Delete removes the grade row with the given id. Returns true if a row was
// deleted, false if no matching row existed.
func (r *Repository) Delete(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&Grade{}, id)
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
