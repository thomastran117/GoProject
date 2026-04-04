package submission

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// AssignmentSubmission is the database model for a student's file submission
// against an assignment. The composite unique index (uq_submission) ensures
// exactly one submission row per (assignment, student) pair.
type AssignmentSubmission struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	AssignmentID uint64    `gorm:"uniqueIndex:uq_submission;index;not null"`
	StudentID    uint64    `gorm:"uniqueIndex:uq_submission;index;not null"`
	BlobKey      string    `gorm:"size:500;not null"`
	FileName     string    `gorm:"size:300;not null"`
	Status       string    `gorm:"size:20;not null;default:'submitted'"` // submitted | late | graded
	Grade        *uint     // NULL until graded
	Feedback     string    `gorm:"type:text"`
	SubmittedAt  time.Time `gorm:"not null"`
	UpdatedAt    time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the AssignmentSubmission model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new submission row. Uses OnConflict DoNothing so a duplicate
// (assignment_id, student_id) pair is silently ignored rather than returning a
// DB error. The caller must check s.ID == 0 to detect the duplicate case.
func (r *Repository) Create(s *AssignmentSubmission) (*AssignmentSubmission, error) {
	err := dbretry.Do(func() error {
		return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(s).Error
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// FindByID returns the submission with the given primary key, or nil if no row
// exists.
func (r *Repository) FindByID(id uint64) (*AssignmentSubmission, error) {
	var s AssignmentSubmission
	err := dbretry.Do(func() error {
		return r.db.First(&s, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByAssignmentAndStudent returns the single submission for a
// (assignmentID, studentID) pair, or nil, nil if none exists.
func (r *Repository) FindByAssignmentAndStudent(assignmentID, studentID uint64) (*AssignmentSubmission, error) {
	var s AssignmentSubmission
	err := dbretry.Do(func() error {
		return r.db.Where("assignment_id = ? AND student_id = ?", assignmentID, studentID).First(&s).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByAssignment returns all submissions for the given assignment ordered by
// SubmittedAt ascending.
func (r *Repository) FindByAssignment(assignmentID uint64) ([]*AssignmentSubmission, error) {
	var submissions []*AssignmentSubmission
	err := dbretry.Do(func() error {
		return r.db.Where("assignment_id = ?", assignmentID).
			Order("submitted_at ASC").
			Find(&submissions).Error
	})
	if err != nil {
		return nil, err
	}
	return submissions, nil
}

// Grade updates the status, grade value, and feedback on the submission
// identified by id inside a SELECT FOR UPDATE transaction. Returns nil, nil
// when no matching row exists.
func (r *Repository) Grade(id uint64, grade uint, feedback string) (*AssignmentSubmission, error) {
	var s AssignmentSubmission
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, id).Error; err != nil {
				return err
			}
			return tx.Model(&s).Updates(map[string]any{
				"status":   "graded",
				"grade":    grade,
				"feedback": feedback,
			}).Error
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
