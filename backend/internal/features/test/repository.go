package test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/utilities/dbretry"
)

// Test is the database model for a course test.
type Test struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	CourseID    uint64     `gorm:"index;not null"`
	AuthorID    uint64     `gorm:"index;not null"`
	Title       string     `gorm:"size:300;not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	TestType    string     `gorm:"size:20;not null;default:'in_house'"` // "link" | "in_house"
	ExternalURL string     `gorm:"size:2048;not null;default:''"`
	Status      string     `gorm:"size:20;not null;default:'draft'"` // "draft" | "published" | "closed"
	DueAt       *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TestQuestion is a single question belonging to a test.
type TestQuestion struct {
	ID            uint64       `gorm:"primaryKey;autoIncrement"`
	TestID        uint64       `gorm:"index;not null"`
	SortOrder     int          `gorm:"not null;default:0"`
	QuestionType  string       `gorm:"size:30;not null"`
	Text          string       `gorm:"type:text;not null"`
	ImageBlobKey  string       `gorm:"size:500;not null;default:''"`
	Weight        float64      `gorm:"not null;default:1"`
	CorrectAnswer string       `gorm:"size:1000;not null;default:''"`
	Choices       []TestChoice `gorm:"foreignKey:QuestionID"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TestChoice is one option for a multiple_choice question.
type TestChoice struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	QuestionID uint64    `gorm:"index;not null"`
	SortOrder  int       `gorm:"not null;default:0"`
	Text       string    `gorm:"size:1000;not null"`
	IsCorrect  bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TestSubmission tracks a student's attempt at a test.
type TestSubmission struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	TestID      uint64     `gorm:"uniqueIndex:uq_test_submission;not null"`
	StudentID   uint64     `gorm:"uniqueIndex:uq_test_submission;not null"`
	Status      string     `gorm:"size:20;not null;default:'in_progress'"`
	StartedAt   time.Time  `gorm:"not null"`
	SubmittedAt *time.Time
	Score       *float64
	MaxScore    float64 `gorm:"not null;default:0"`
	GradeID     *uint64 `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TestAnswer stores a student's response to one question within a submission.
type TestAnswer struct {
	ID            uint64   `gorm:"primaryKey;autoIncrement"`
	SubmissionID  uint64   `gorm:"uniqueIndex:uq_test_answer;not null"`
	QuestionID    uint64   `gorm:"uniqueIndex:uq_test_answer;not null"`
	AnswerText    string   `gorm:"type:text;not null;default:''"`
	PointsAwarded *float64
	NeedsReview   bool `gorm:"not null;default:false"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Repository wraps the GORM database connection for Test and related models.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ─── Test CRUD ────────────────────────────────────────────────────────────────

func (r *Repository) Create(ctx context.Context, t *Test) (*Test, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(t).Error
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *Repository) FindByID(ctx context.Context, id uint64) (*Test, error) {
	var t Test
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&t, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) FindByCourse(ctx context.Context, courseID uint64) ([]*Test, error) {
	var tests []*Test
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("course_id = ?", courseID).
			Order("created_at DESC").Find(&tests).Error
	})
	if err != nil {
		return nil, err
	}
	return tests, nil
}

func (r *Repository) Update(ctx context.Context, id uint64, fields map[string]any) (*Test, error) {
	var t Test
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, id).Error; err != nil {
				return err
			}
			return tx.Model(&t).Updates(fields).Error
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

func (r *Repository) Delete(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&Test{}, id)
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

// ─── Question ────────────────────────────────────────────────────────────────

func (r *Repository) CreateQuestion(ctx context.Context, q *TestQuestion) (*TestQuestion, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(q).Error
	})
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (r *Repository) FindQuestionByID(ctx context.Context, id uint64) (*TestQuestion, error) {
	var q TestQuestion
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&q, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *Repository) FindQuestionsWithChoices(ctx context.Context, testID uint64) ([]*TestQuestion, error) {
	var questions []*TestQuestion
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("test_id = ?", testID).
			Order("sort_order ASC, id ASC").
			Preload("Choices", func(db *gorm.DB) *gorm.DB {
				return db.Order("sort_order ASC, id ASC")
			}).
			Find(&questions).Error
	})
	if err != nil {
		return nil, err
	}
	return questions, nil
}

func (r *Repository) UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*TestQuestion, error) {
	var q TestQuestion
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&q, id).Error; err != nil {
				return err
			}
			return tx.Model(&q).Updates(fields).Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindQuestionByID(ctx, id)
}

func (r *Repository) DeleteQuestion(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&TestQuestion{}, id)
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

func (r *Repository) DeleteChoicesByQuestion(ctx context.Context, questionID uint64) error {
	return dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("question_id = ?", questionID).Delete(&TestChoice{}).Error
	})
}

func (r *Repository) CountQuestionsByTest(ctx context.Context, testID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&TestQuestion{}).Where("test_id = ?", testID).Count(&count).Error
	})
	return count, err
}

// ─── Choice ──────────────────────────────────────────────────────────────────

func (r *Repository) CreateChoice(ctx context.Context, c *TestChoice) (*TestChoice, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(c).Error
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) FindChoiceByID(ctx context.Context, id uint64) (*TestChoice, error) {
	var c TestChoice
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&c, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*TestChoice, error) {
	var choices []*TestChoice
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("question_id = ?", questionID).
			Order("sort_order ASC, id ASC").Find(&choices).Error
	})
	if err != nil {
		return nil, err
	}
	return choices, nil
}

func (r *Repository) CountChoicesByQuestion(ctx context.Context, questionID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&TestChoice{}).Where("question_id = ?", questionID).Count(&count).Error
	})
	return count, err
}

func (r *Repository) UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*TestChoice, error) {
	var c TestChoice
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, id).Error; err != nil {
				return err
			}
			return tx.Model(&c).Updates(fields).Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindChoiceByID(ctx, id)
}

func (r *Repository) DeleteChoice(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&TestChoice{}, id)
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

// ─── Submission ──────────────────────────────────────────────────────────────

func (r *Repository) CreateSubmission(ctx context.Context, s *TestSubmission) (*TestSubmission, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(s).Error
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *Repository) FindSubmissionByID(ctx context.Context, id uint64) (*TestSubmission, error) {
	var s TestSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&s, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) FindSubmissionByTestAndStudent(ctx context.Context, testID, studentID uint64) (*TestSubmission, error) {
	var s TestSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("test_id = ? AND student_id = ?", testID, studentID).
			First(&s).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) FindSubmissionsByTest(ctx context.Context, testID uint64) ([]*TestSubmission, error) {
	var subs []*TestSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("test_id = ?", testID).
			Order("started_at ASC").Find(&subs).Error
	})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *Repository) CountActiveSubmissions(ctx context.Context, testID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&TestSubmission{}).
			Where("test_id = ? AND status IN ('in_progress','submitted')", testID).
			Count(&count).Error
	})
	return count, err
}

func (r *Repository) UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*TestSubmission, error) {
	var s TestSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, id).Error; err != nil {
				return err
			}
			return tx.Model(&s).Updates(fields).Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindSubmissionByID(ctx, id)
}

// ─── Answer ───────────────────────────────────────────────────────────────────

func (r *Repository) UpsertAnswer(ctx context.Context, a *TestAnswer) (*TestAnswer, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "submission_id"}, {Name: "question_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"answer_text", "updated_at"}),
			}).
			Create(a).Error
	})
	if err != nil {
		return nil, err
	}
	var found TestAnswer
	fetchErr := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("submission_id = ? AND question_id = ?", a.SubmissionID, a.QuestionID).
			First(&found).Error
	})
	if fetchErr != nil {
		return nil, fetchErr
	}
	return &found, nil
}

func (r *Repository) FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*TestAnswer, error) {
	var answers []*TestAnswer
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("submission_id = ?", submissionID).Find(&answers).Error
	})
	if err != nil {
		return nil, err
	}
	return answers, nil
}

func (r *Repository) UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*TestAnswer, error) {
	var a TestAnswer
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	var updated TestAnswer
	fetchErr := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&updated, id).Error
	})
	if fetchErr != nil {
		return nil, fetchErr
	}
	return &updated, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func FormatChoiceIDs(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}

func ParseChoiceIDs(s string) []uint64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	ids := make([]uint64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var id uint64
		if _, err := fmt.Sscanf(p, "%d", &id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
