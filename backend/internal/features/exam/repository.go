package exam

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

// Exam is the database model for a course exam.
type Exam struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	CourseID    uint64     `gorm:"index;not null"`
	AuthorID    uint64     `gorm:"index;not null"`
	Title       string     `gorm:"size:300;not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	ExamType    string     `gorm:"size:20;not null;default:'in_house'"` // "link" | "in_house"
	ExternalURL string     `gorm:"size:2048;not null;default:''"`
	Status      string     `gorm:"size:20;not null;default:'draft'"` // "draft" | "published" | "closed"
	DueAt       *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExamQuestion is a single question belonging to an exam.
type ExamQuestion struct {
	ID            uint64       `gorm:"primaryKey;autoIncrement"`
	ExamID        uint64       `gorm:"index;not null"`
	SortOrder     int          `gorm:"not null;default:0"`
	QuestionType  string       `gorm:"size:30;not null"`
	Text          string       `gorm:"type:text;not null"`
	ImageBlobKey  string       `gorm:"size:500;not null;default:''"`
	Weight        float64      `gorm:"not null;default:1"`
	CorrectAnswer string       `gorm:"size:1000;not null;default:''"`
	Choices       []ExamChoice `gorm:"foreignKey:QuestionID"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ExamChoice is one option for a multiple_choice question.
type ExamChoice struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	QuestionID uint64    `gorm:"index;not null"`
	SortOrder  int       `gorm:"not null;default:0"`
	Text       string    `gorm:"size:1000;not null"`
	IsCorrect  bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ExamSubmission tracks a student's attempt at an exam.
type ExamSubmission struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	ExamID      uint64     `gorm:"uniqueIndex:uq_exam_submission;not null"`
	StudentID   uint64     `gorm:"uniqueIndex:uq_exam_submission;not null"`
	Status      string     `gorm:"size:20;not null;default:'in_progress'"`
	StartedAt   time.Time  `gorm:"not null"`
	SubmittedAt *time.Time
	Score       *float64
	MaxScore    float64 `gorm:"not null;default:0"`
	GradeID     *uint64 `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExamAnswer stores a student's response to one question within a submission.
type ExamAnswer struct {
	ID            uint64   `gorm:"primaryKey;autoIncrement"`
	SubmissionID  uint64   `gorm:"uniqueIndex:uq_exam_answer;not null"`
	QuestionID    uint64   `gorm:"uniqueIndex:uq_exam_answer;not null"`
	AnswerText    string   `gorm:"type:text;not null;default:''"`
	PointsAwarded *float64
	NeedsReview   bool `gorm:"not null;default:false"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Repository wraps the GORM database connection for Exam and related models.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ─── Exam CRUD ────────────────────────────────────────────────────────────────

func (r *Repository) Create(ctx context.Context, e *Exam) (*Exam, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(e).Error
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *Repository) FindByID(ctx context.Context, id uint64) (*Exam, error) {
	var e Exam
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&e, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) FindByCourse(ctx context.Context, courseID uint64) ([]*Exam, error) {
	var exams []*Exam
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("course_id = ?", courseID).
			Order("created_at DESC").Find(&exams).Error
	})
	if err != nil {
		return nil, err
	}
	return exams, nil
}

func (r *Repository) Update(ctx context.Context, id uint64, fields map[string]any) (*Exam, error) {
	var e Exam
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, id).Error; err != nil {
				return err
			}
			return tx.Model(&e).Updates(fields).Error
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
		result := r.db.WithContext(ctx).Delete(&Exam{}, id)
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

func (r *Repository) CreateQuestion(ctx context.Context, q *ExamQuestion) (*ExamQuestion, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(q).Error
	})
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (r *Repository) FindQuestionByID(ctx context.Context, id uint64) (*ExamQuestion, error) {
	var q ExamQuestion
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

func (r *Repository) FindQuestionsWithChoices(ctx context.Context, examID uint64) ([]*ExamQuestion, error) {
	var questions []*ExamQuestion
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("exam_id = ?", examID).
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

func (r *Repository) UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*ExamQuestion, error) {
	var q ExamQuestion
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
		result := r.db.WithContext(ctx).Delete(&ExamQuestion{}, id)
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
		return r.db.WithContext(ctx).Where("question_id = ?", questionID).Delete(&ExamChoice{}).Error
	})
}

func (r *Repository) CountQuestionsByExam(ctx context.Context, examID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&ExamQuestion{}).Where("exam_id = ?", examID).Count(&count).Error
	})
	return count, err
}

// ─── Choice ──────────────────────────────────────────────────────────────────

func (r *Repository) CreateChoice(ctx context.Context, c *ExamChoice) (*ExamChoice, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(c).Error
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) FindChoiceByID(ctx context.Context, id uint64) (*ExamChoice, error) {
	var c ExamChoice
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

func (r *Repository) FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*ExamChoice, error) {
	var choices []*ExamChoice
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
		return r.db.WithContext(ctx).Model(&ExamChoice{}).Where("question_id = ?", questionID).Count(&count).Error
	})
	return count, err
}

func (r *Repository) UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*ExamChoice, error) {
	var c ExamChoice
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
		result := r.db.WithContext(ctx).Delete(&ExamChoice{}, id)
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

func (r *Repository) CreateSubmission(ctx context.Context, s *ExamSubmission) (*ExamSubmission, error) {
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

func (r *Repository) FindSubmissionByID(ctx context.Context, id uint64) (*ExamSubmission, error) {
	var s ExamSubmission
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

func (r *Repository) FindSubmissionByExamAndStudent(ctx context.Context, examID, studentID uint64) (*ExamSubmission, error) {
	var s ExamSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("exam_id = ? AND student_id = ?", examID, studentID).
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

func (r *Repository) FindSubmissionsByExam(ctx context.Context, examID uint64) ([]*ExamSubmission, error) {
	var subs []*ExamSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("exam_id = ?", examID).
			Order("started_at ASC").Find(&subs).Error
	})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *Repository) CountActiveSubmissions(ctx context.Context, examID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&ExamSubmission{}).
			Where("exam_id = ? AND status IN ('in_progress','submitted')", examID).
			Count(&count).Error
	})
	return count, err
}

func (r *Repository) UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*ExamSubmission, error) {
	var s ExamSubmission
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

func (r *Repository) UpsertAnswer(ctx context.Context, a *ExamAnswer) (*ExamAnswer, error) {
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
	var found ExamAnswer
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

func (r *Repository) FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*ExamAnswer, error) {
	var answers []*ExamAnswer
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("submission_id = ?", submissionID).Find(&answers).Error
	})
	if err != nil {
		return nil, err
	}
	return answers, nil
}

func (r *Repository) UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*ExamAnswer, error) {
	var a ExamAnswer
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
	var updated ExamAnswer
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
