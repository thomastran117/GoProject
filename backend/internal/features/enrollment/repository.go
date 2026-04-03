package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/utilities/dbretry"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const inviteTTL = 30 * 24 * time.Hour

// Enrollment is the database model for a course enrollment. The composite
// unique index ensures a user can only have one enrollment record per course.
type Enrollment struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	CourseID   uint64    `gorm:"uniqueIndex:uq_enrollment;index;not null"`
	UserID     uint64    `gorm:"uniqueIndex:uq_enrollment;index;not null"`
	Status     string    `gorm:"size:20;not null;default:'active'"` // active, dropped
	EnrolledAt time.Time `gorm:"not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CourseInvite is the in-memory representation of a Redis-stored invite.
// Invites are NOT persisted to MySQL; they live in Redis with a 30-day TTL.
type CourseInvite struct {
	Code      string    `json:"code"`
	CourseID  uint64    `json:"course_id"`
	InviteeID uint64    `json:"invitee_id"`
	InviterID uint64    `json:"inviter_id"`
	Status    string    `json:"status"` // pending, accepted, revoked
	CreatedAt time.Time `json:"created_at"`
}

// Page holds pagination parameters.
type Page struct {
	Number int
	Size   int
}

// Repository wraps both the GORM database (for Enrollment records) and the
// Redis client (for CourseInvite records).
type Repository struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewRepository creates a Repository backed by the given DB and Redis client.
func NewRepository(db *gorm.DB, rdb *redis.Client) *Repository {
	return &Repository{db: db, rdb: rdb}
}

// ── Redis key helpers ────────────────────────────────────────────────────────

func inviteKey(code string) string {
	return fmt.Sprintf("invite:%s", code)
}

func courseInviteSetKey(courseID uint64) string {
	return fmt.Sprintf("course:%d:invites", courseID)
}

func courseInviteeLookupKey(courseID, inviteeID uint64) string {
	return fmt.Sprintf("course:%d:invitee:%d", courseID, inviteeID)
}

// ── Invite code generation ───────────────────────────────────────────────────

func generateInviteCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// ── Enrollment (MySQL) methods ───────────────────────────────────────────────

// FindEnrollment returns the enrollment for the given course and user, or
// nil, nil if no row exists.
func (r *Repository) FindEnrollment(courseID, userID uint64) (*Enrollment, error) {
	var e Enrollment
	err := dbretry.Do(func() error {
		return r.db.Where("course_id = ? AND user_id = ?", courseID, userID).First(&e).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// FindEnrollmentsByCourse returns active enrollments for the given course,
// paginated. Also returns the total count of active enrollments.
func (r *Repository) FindEnrollmentsByCourse(courseID uint64, page Page) ([]*Enrollment, int64, error) {
	var enrollments []*Enrollment
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Enrollment{}).Where("course_id = ? AND status = 'active'", courseID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		offset := (page.Number - 1) * page.Size
		return q.Offset(offset).Limit(page.Size).Find(&enrollments).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return enrollments, total, nil
}

// FindEnrollmentsByUser returns active enrollments for the given user,
// paginated. Also returns the total count.
func (r *Repository) FindEnrollmentsByUser(userID uint64, page Page) ([]*Enrollment, int64, error) {
	var enrollments []*Enrollment
	var total int64
	err := dbretry.Do(func() error {
		q := r.db.Model(&Enrollment{}).Where("user_id = ? AND status = 'active'", userID)
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		offset := (page.Number - 1) * page.Size
		return q.Offset(offset).Limit(page.Size).Find(&enrollments).Error
	})
	if err != nil {
		return nil, 0, err
	}
	return enrollments, total, nil
}

// CountEnrollmentsByCourse returns the number of active enrollments for the
// given course.
func (r *Repository) CountEnrollmentsByCourse(courseID uint64) (int64, error) {
	var count int64
	err := dbretry.Do(func() error {
		return r.db.Model(&Enrollment{}).Where("course_id = ? AND status = 'active'", courseID).Count(&count).Error
	})
	return count, err
}

// CreateEnrollment inserts a new enrollment row. Returns a 409 APIError if
// the unique constraint fires (duplicate enrollment).
func (r *Repository) CreateEnrollment(e *Enrollment) (*Enrollment, error) {
	err := dbretry.Do(func() error {
		return r.db.Create(e).Error
	})
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ALREADY_ENROLLED", Message: "You are already enrolled in this course"}
		}
		return nil, err
	}
	return e, nil
}

// UpdateEnrollmentStatus sets the status field on the enrollment identified by
// courseID+userID. Returns false if no matching row exists.
func (r *Repository) UpdateEnrollmentStatus(courseID, userID uint64, status string) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		result := r.db.Model(&Enrollment{}).
			Where("course_id = ? AND user_id = ?", courseID, userID).
			Update("status", status)
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

// ── CourseInvite (Redis) methods ─────────────────────────────────────────────

// CreateInvite stores the invite in Redis under three keys:
//   - invite:{code}                     → JSON blob
//   - course:{courseId}:invites         → set member (for listing)
//   - course:{courseId}:invitee:{id}    → code (for lookup by course+invitee)
func (r *Repository) CreateInvite(ctx context.Context, inv *CourseInvite) error {
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, inviteKey(inv.Code), data, inviteTTL)
	pipe.SAdd(ctx, courseInviteSetKey(inv.CourseID), inv.Code)
	pipe.Expire(ctx, courseInviteSetKey(inv.CourseID), inviteTTL)
	pipe.Set(ctx, courseInviteeLookupKey(inv.CourseID, inv.InviteeID), inv.Code, inviteTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// FindInviteByCode looks up an invite by its 10-character code. Returns
// nil, nil if the key does not exist (not found or expired).
func (r *Repository) FindInviteByCode(ctx context.Context, code string) (*CourseInvite, error) {
	data, err := r.rdb.Get(ctx, inviteKey(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var inv CourseInvite
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// FindInviteByCourseAndInvitee looks up the invite code for a (course, invitee)
// pair and then fetches the full invite. Returns nil, nil if none exists.
func (r *Repository) FindInviteByCourseAndInvitee(ctx context.Context, courseID, inviteeID uint64) (*CourseInvite, error) {
	code, err := r.rdb.Get(ctx, courseInviteeLookupKey(courseID, inviteeID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindInviteByCode(ctx, code)
}

// FindInvitesByCourse returns all invites stored for the given course by
// reading the invite-code set and fetching each invite in a pipeline.
func (r *Repository) FindInvitesByCourse(ctx context.Context, courseID uint64) ([]*CourseInvite, error) {
	codes, err := r.rdb.SMembers(ctx, courseInviteSetKey(courseID)).Result()
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return []*CourseInvite{}, nil
	}

	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(codes))
	for i, code := range codes {
		cmds[i] = pipe.Get(ctx, inviteKey(code))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	var invites []*CourseInvite
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if errors.Is(err, redis.Nil) {
			continue // invite expired; skip
		}
		if err != nil {
			return nil, err
		}
		var inv CourseInvite
		if err := json.Unmarshal(data, &inv); err != nil {
			return nil, err
		}
		invites = append(invites, &inv)
	}
	return invites, nil
}

// UpdateInviteStatus reads the existing invite, changes its status, and writes
// it back with the remaining TTL preserved.
func (r *Repository) UpdateInviteStatus(ctx context.Context, code, status string) (*CourseInvite, error) {
	key := inviteKey(code)

	ttl, err := r.rdb.TTL(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	data, err := r.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, &middleware.APIError{Status: http.StatusNotFound, Code: "INVITE_NOT_FOUND", Message: "Invite not found or expired"}
	}
	if err != nil {
		return nil, err
	}

	var inv CourseInvite
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	inv.Status = status

	updated, err := json.Marshal(&inv)
	if err != nil {
		return nil, err
	}

	effectiveTTL := ttl
	if effectiveTTL <= 0 {
		effectiveTTL = inviteTTL
	}
	if err := r.rdb.Set(ctx, key, updated, effectiveTTL).Err(); err != nil {
		return nil, err
	}
	return &inv, nil
}

// AcceptInviteAndEnroll atomically (best-effort) marks the invite as accepted
// in Redis and creates the enrollment in MySQL.
func (r *Repository) AcceptInviteAndEnroll(ctx context.Context, code string, e *Enrollment) (*CourseInvite, *Enrollment, error) {
	inv, err := r.UpdateInviteStatus(ctx, code, "accepted")
	if err != nil {
		return nil, nil, err
	}

	created, err := r.CreateEnrollment(e)
	if err != nil {
		return nil, nil, err
	}
	return inv, created, nil
}

// ── SELECT FOR UPDATE helper (used internally) ───────────────────────────────

// updateEnrollmentWithLock wraps an enrollment update in a transaction with a
// row-level lock to prevent concurrent lost updates.
func (r *Repository) updateEnrollmentWithLock(courseID, userID uint64, status string) (bool, error) {
	var rowsAffected int64
	err := dbretry.Do(func() error {
		return r.db.Transaction(func(tx *gorm.DB) error {
			var e Enrollment
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("course_id = ? AND user_id = ?", courseID, userID).
				First(&e).Error; err != nil {
				return err
			}
			result := tx.Model(&e).Update("status", status)
			rowsAffected = result.RowsAffected
			return result.Error
		})
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return rowsAffected > 0, err
}
