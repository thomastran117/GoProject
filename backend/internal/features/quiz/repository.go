package quiz

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

// Quiz is the database model for a course quiz.
type Quiz struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	CourseID    uint64     `gorm:"index;not null"`
	AuthorID    uint64     `gorm:"index;not null"`
	Title       string     `gorm:"size:300;not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	QuizType    string     `gorm:"size:20;not null;default:'in_house'"` // "link" | "in_house"
	ExternalURL string     `gorm:"size:2048;not null;default:''"`
	Status      string     `gorm:"size:20;not null;default:'draft'"` // "draft" | "published" | "closed"
	DueAt       *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// QuizQuestion is a single question belonging to a quiz.
type QuizQuestion struct {
	ID            uint64       `gorm:"primaryKey;autoIncrement"`
	QuizID        uint64       `gorm:"index;not null"`
	SortOrder     int          `gorm:"not null;default:0"`
	QuestionType  string       `gorm:"size:30;not null"` // "multiple_choice"|"short_answer"|"fill_in_blank"|"long_answer"
	Text          string       `gorm:"type:text;not null"`
	ImageBlobKey  string       `gorm:"size:500;not null;default:''"`
	Weight        float64      `gorm:"not null;default:1"`
	CorrectAnswer string       `gorm:"size:1000;not null;default:''"` // fill_in_blank only
	Choices       []QuizChoice `gorm:"foreignKey:QuestionID"`         // GORM association, not a column
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// QuizChoice is one option for a multiple_choice question.
type QuizChoice struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	QuestionID uint64    `gorm:"index;not null"`
	SortOrder  int       `gorm:"not null;default:0"`
	Text       string    `gorm:"size:1000;not null"`
	IsCorrect  bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// QuizSubmission tracks a student's attempt at a quiz.
type QuizSubmission struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	QuizID      uint64     `gorm:"uniqueIndex:uq_quiz_submission;not null"`
	StudentID   uint64     `gorm:"uniqueIndex:uq_quiz_submission;not null"`
	Status      string     `gorm:"size:20;not null;default:'in_progress'"` // "in_progress"|"submitted"|"graded"
	StartedAt   time.Time  `gorm:"not null"`
	SubmittedAt *time.Time
	Score       *float64
	MaxScore    float64 `gorm:"not null;default:0"`
	GradeID     *uint64 `gorm:"index"` // FK → grades.id
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// QuizAnswer stores a student's response to one question within a submission.
type QuizAnswer struct {
	ID            uint64   `gorm:"primaryKey;autoIncrement"`
	SubmissionID  uint64   `gorm:"uniqueIndex:uq_quiz_answer;not null"`
	QuestionID    uint64   `gorm:"uniqueIndex:uq_quiz_answer;not null"`
	AnswerText    string   `gorm:"type:text;not null;default:''"` // comma-sep choice IDs for MC, free text otherwise
	PointsAwarded *float64
	NeedsReview   bool `gorm:"not null;default:false"` // true for SA/LA until manually graded
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Repository wraps the GORM database connection and provides persistence
// operations for Quiz and related models.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ─── Quiz CRUD ───────────────────────────────────────────────────────────────

// Create inserts a new quiz and returns the persisted record.
func (r *Repository) Create(ctx context.Context, q *Quiz) (*Quiz, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(q).Error
	})
	if err != nil {
		return nil, err
	}
	return q, nil
}

// FindByID returns the quiz with the given primary key, or nil if not found.
func (r *Repository) FindByID(ctx context.Context, id uint64) (*Quiz, error) {
	var q Quiz
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

// FindByCourse returns all quizzes for a course ordered by created_at DESC.
func (r *Repository) FindByCourse(ctx context.Context, courseID uint64) ([]*Quiz, error) {
	var quizzes []*Quiz
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("course_id = ?", courseID).
			Order("created_at DESC").Find(&quizzes).Error
	})
	if err != nil {
		return nil, err
	}
	return quizzes, nil
}

// Update applies field changes to the quiz identified by id using a SELECT FOR
// UPDATE transaction. Returns nil, nil when no matching row exists.
func (r *Repository) Update(ctx context.Context, id uint64, fields map[string]any) (*Quiz, error) {
	var q Quiz
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
	return r.FindByID(ctx, id)
}

// Delete removes a quiz row. Returns true if a row was deleted.
func (r *Repository) Delete(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&Quiz{}, id)
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

// CreateQuestion inserts a new question and returns the persisted record.
func (r *Repository) CreateQuestion(ctx context.Context, q *QuizQuestion) (*QuizQuestion, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(q).Error
	})
	if err != nil {
		return nil, err
	}
	return q, nil
}

// FindQuestionByID returns the question with the given id, or nil.
func (r *Repository) FindQuestionByID(ctx context.Context, id uint64) (*QuizQuestion, error) {
	var q QuizQuestion
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

// FindQuestionsWithChoices returns all questions for a quiz with their choices
// preloaded, ordered by sort_order ASC.
func (r *Repository) FindQuestionsWithChoices(ctx context.Context, quizID uint64) ([]*QuizQuestion, error) {
	var questions []*QuizQuestion
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("quiz_id = ?", quizID).
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

// UpdateQuestion applies field changes to the question using SELECT FOR UPDATE.
func (r *Repository) UpdateQuestion(ctx context.Context, id uint64, fields map[string]any) (*QuizQuestion, error) {
	var q QuizQuestion
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

// DeleteQuestion removes a question row. Returns true if deleted.
func (r *Repository) DeleteQuestion(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&QuizQuestion{}, id)
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

// DeleteChoicesByQuestion removes all choices for a given question.
func (r *Repository) DeleteChoicesByQuestion(ctx context.Context, questionID uint64) error {
	return dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("question_id = ?", questionID).Delete(&QuizChoice{}).Error
	})
}

// CountQuestionsByQuiz returns the number of questions belonging to a quiz.
func (r *Repository) CountQuestionsByQuiz(ctx context.Context, quizID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&QuizQuestion{}).Where("quiz_id = ?", quizID).Count(&count).Error
	})
	return count, err
}

// ─── Choice ──────────────────────────────────────────────────────────────────

// CreateChoice inserts a new MC choice.
func (r *Repository) CreateChoice(ctx context.Context, c *QuizChoice) (*QuizChoice, error) {
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(c).Error
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// FindChoiceByID returns the choice with the given id, or nil.
func (r *Repository) FindChoiceByID(ctx context.Context, id uint64) (*QuizChoice, error) {
	var c QuizChoice
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

// FindChoicesByQuestion returns all choices for a question ordered by sort_order.
func (r *Repository) FindChoicesByQuestion(ctx context.Context, questionID uint64) ([]*QuizChoice, error) {
	var choices []*QuizChoice
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("question_id = ?", questionID).
			Order("sort_order ASC, id ASC").Find(&choices).Error
	})
	if err != nil {
		return nil, err
	}
	return choices, nil
}

// CountChoicesByQuestion returns the number of choices for a question.
func (r *Repository) CountChoicesByQuestion(ctx context.Context, questionID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&QuizChoice{}).Where("question_id = ?", questionID).Count(&count).Error
	})
	return count, err
}

// UpdateChoice applies field changes to a choice using SELECT FOR UPDATE.
func (r *Repository) UpdateChoice(ctx context.Context, id uint64, fields map[string]any) (*QuizChoice, error) {
	var c QuizChoice
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

// DeleteChoice removes a choice row. Returns true if deleted.
func (r *Repository) DeleteChoice(ctx context.Context, id uint64) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.WithContext(ctx).Delete(&QuizChoice{}, id)
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

// CreateSubmission inserts a new submission. Uses OnConflict DoNothing so
// duplicate starts return a zero-ID record instead of an error.
func (r *Repository) CreateSubmission(ctx context.Context, s *QuizSubmission) (*QuizSubmission, error) {
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

// FindSubmissionByID returns the submission with the given id, or nil.
func (r *Repository) FindSubmissionByID(ctx context.Context, id uint64) (*QuizSubmission, error) {
	var s QuizSubmission
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

// FindSubmissionByQuizAndStudent returns the submission for a specific quiz/student pair, or nil.
func (r *Repository) FindSubmissionByQuizAndStudent(ctx context.Context, quizID, studentID uint64) (*QuizSubmission, error) {
	var s QuizSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Where("quiz_id = ? AND student_id = ?", quizID, studentID).
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

// FindSubmissionsByQuiz returns all submissions for a quiz ordered by started_at.
func (r *Repository) FindSubmissionsByQuiz(ctx context.Context, quizID uint64) ([]*QuizSubmission, error) {
	var subs []*QuizSubmission
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("quiz_id = ?", quizID).
			Order("started_at ASC").Find(&subs).Error
	})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// CountActiveSubmissions returns the number of in_progress or submitted submissions for a quiz.
func (r *Repository) CountActiveSubmissions(ctx context.Context, quizID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Model(&QuizSubmission{}).
			Where("quiz_id = ? AND status IN ('in_progress','submitted')", quizID).
			Count(&count).Error
	})
	return count, err
}

// UpdateSubmission applies field changes to a submission using SELECT FOR UPDATE.
func (r *Repository) UpdateSubmission(ctx context.Context, id uint64, fields map[string]any) (*QuizSubmission, error) {
	var s QuizSubmission
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

// UpsertAnswer inserts or updates an answer for a (submission, question) pair.
func (r *Repository) UpsertAnswer(ctx context.Context, a *QuizAnswer) (*QuizAnswer, error) {
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
	// Re-fetch to get the actual row (ID may be 0 if the upsert hit the conflict path)
	var found QuizAnswer
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

// FindAnswersBySubmission returns all answers for a submission.
func (r *Repository) FindAnswersBySubmission(ctx context.Context, submissionID uint64) ([]*QuizAnswer, error) {
	var answers []*QuizAnswer
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Where("submission_id = ?", submissionID).Find(&answers).Error
	})
	if err != nil {
		return nil, err
	}
	return answers, nil
}

// UpdateAnswer applies field changes to an answer using SELECT FOR UPDATE.
func (r *Repository) UpdateAnswer(ctx context.Context, id uint64, fields map[string]any) (*QuizAnswer, error) {
	var a QuizAnswer
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
	var updated QuizAnswer
	fetchErr := dbretry.Do(func() error {
		return r.db.WithContext(ctx).First(&updated, id).Error
	})
	if fetchErr != nil {
		return nil, fetchErr
	}
	return &updated, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// FormatChoiceIDs encodes a slice of choice IDs as a comma-separated string.
func FormatChoiceIDs(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}

// ParseChoiceIDs decodes a comma-separated string of choice IDs.
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
