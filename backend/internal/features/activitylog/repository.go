package activitylog

import (
	"context"
	"time"

	"backend/internal/utilities/dbretry"

	"gorm.io/gorm"
)

type ActivityLog struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	Role       string    `gorm:"size:50;not null;index:idx_role_event_time,priority:1"`
	EventType  string    `gorm:"size:20;not null;index:idx_role_event_time,priority:2"` // "api_request" | "page_view"
	Method     string    `gorm:"size:10"`
	Path       string    `gorm:"size:500"` // Gin route template — api_request only
	Page       string    `gorm:"size:500"` // page name — page_view only
	StatusCode int
	DurationMs int       `gorm:"not null;default:0"`
	OccurredAt time.Time `gorm:"not null;index:idx_role_event_time,priority:3"`
	CreatedAt  time.Time
}

type RequestStat struct {
	Role     string
	Method   string
	Path     string
	Count    int64
	AvgDurMs float64
}

type PageViewStat struct {
	Role     string
	Page     string
	Count    int64
	AvgDurMs float64
}

type RoleTotalStat struct {
	Role  string
	Total int64
}

type TopEndpointStat struct {
	Role   string
	Path   string
	Method string
	Count  int64
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, entry *ActivityLog) error {
	return dbretry.Do(func() error {
		return r.db.WithContext(ctx).Create(entry).Error
	})
}

func (r *Repository) RequestStats(ctx context.Context, from, to time.Time, role string) ([]*RequestStat, error) {
	var results []*RequestStat
	err := dbretry.Do(func() error {
		q := r.db.WithContext(ctx).
			Model(&ActivityLog{}).
			Select("role, method, path, COUNT(*) AS count, AVG(duration_ms) AS avg_dur_ms").
			Where("event_type = ?", "api_request").
			Where("occurred_at BETWEEN ? AND ?", from, to).
			Group("role, method, path").
			Order("count DESC")
		if role != "" {
			q = q.Where("role = ?", role)
		}
		return q.Scan(&results).Error
	})
	return results, err
}

func (r *Repository) PageViewStats(ctx context.Context, from, to time.Time, role string) ([]*PageViewStat, error) {
	var results []*PageViewStat
	err := dbretry.Do(func() error {
		q := r.db.WithContext(ctx).
			Model(&ActivityLog{}).
			Select("role, page, COUNT(*) AS count, AVG(duration_ms) AS avg_dur_ms").
			Where("event_type = ?", "page_view").
			Where("occurred_at BETWEEN ? AND ?", from, to).
			Group("role, page").
			Order("count DESC")
		if role != "" {
			q = q.Where("role = ?", role)
		}
		return q.Scan(&results).Error
	})
	return results, err
}

func (r *Repository) TotalsByRole(ctx context.Context) ([]*RoleTotalStat, error) {
	var results []*RoleTotalStat
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).
			Model(&ActivityLog{}).
			Select("role, COUNT(*) AS total").
			Group("role").
			Order("total DESC").
			Scan(&results).Error
	})
	return results, err
}

func (r *Repository) TopEndpointsPerRole(ctx context.Context) ([]*TopEndpointStat, error) {
	var results []*TopEndpointStat
	sql := `
		SELECT role, path, method, count
		FROM (
			SELECT role, path, method, COUNT(*) AS count,
			       ROW_NUMBER() OVER (PARTITION BY role ORDER BY COUNT(*) DESC) AS rn
			FROM activity_logs
			WHERE event_type = 'api_request'
			GROUP BY role, path, method
		) ranked
		WHERE rn = 1
	`
	err := dbretry.Do(func() error {
		return r.db.WithContext(ctx).Raw(sql).Scan(&results).Error
	})
	return results, err
}
