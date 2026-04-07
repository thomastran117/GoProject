package activitylog

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/features/auth"
)

type activityLogRepository interface {
	Create(ctx context.Context, entry *ActivityLog) error
	RequestStats(ctx context.Context, from, to time.Time, role string) ([]*RequestStat, error)
	PageViewStats(ctx context.Context, from, to time.Time, role string) ([]*PageViewStat, error)
	TotalsByRole(ctx context.Context) ([]*RoleTotalStat, error)
	TopEndpointsPerRole(ctx context.Context) ([]*TopEndpointStat, error)
}

type RecordPageViewParams struct {
	Page       string
	DurationMs int
}

type RequestStatsResponse struct {
	Role     string  `json:"role"`
	Method   string  `json:"method"`
	Path     string  `json:"path"`
	Count    int64   `json:"count"`
	AvgDurMs float64 `json:"avg_duration_ms"`
}

type PageViewStatsResponse struct {
	Role     string  `json:"role"`
	Page     string  `json:"page"`
	Count    int64   `json:"count"`
	AvgDurMs float64 `json:"avg_duration_ms"`
}

type RoleTotalResponse struct {
	Role  string `json:"role"`
	Total int64  `json:"total"`
}

type TopEndpointResponse struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Method string `json:"method"`
	Count  int64  `json:"count"`
}

type SummaryResponse struct {
	TotalsByRole        []*RoleTotalResponse   `json:"totals_by_role"`
	MostActiveRole      string                 `json:"most_active_role"`
	TopEndpointsPerRole []*TopEndpointResponse `json:"top_endpoints_per_role"`
}

type Service struct {
	repo activityLogRepository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RecordPageView(ctx context.Context, callerRole string, p RecordPageViewParams) error {
	entry := &ActivityLog{
		Role:       callerRole,
		EventType:  "page_view",
		Page:       p.Page,
		DurationMs: p.DurationMs,
		OccurredAt: time.Now().UTC(),
	}
	return s.repo.Create(ctx, entry)
}

func (s *Service) GetRequestStats(ctx context.Context, callerRole string, from, to time.Time, filterRole string) ([]*RequestStatsResponse, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Admin only"}
	}
	rows, err := s.repo.RequestStats(ctx, from, to, filterRole)
	if err != nil {
		return nil, err
	}
	out := make([]*RequestStatsResponse, len(rows))
	for i, r := range rows {
		out[i] = &RequestStatsResponse{
			Role:     r.Role,
			Method:   r.Method,
			Path:     r.Path,
			Count:    r.Count,
			AvgDurMs: r.AvgDurMs,
		}
	}
	return out, nil
}

func (s *Service) GetPageViewStats(ctx context.Context, callerRole string, from, to time.Time, filterRole string) ([]*PageViewStatsResponse, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Admin only"}
	}
	rows, err := s.repo.PageViewStats(ctx, from, to, filterRole)
	if err != nil {
		return nil, err
	}
	out := make([]*PageViewStatsResponse, len(rows))
	for i, r := range rows {
		out[i] = &PageViewStatsResponse{
			Role:     r.Role,
			Page:     r.Page,
			Count:    r.Count,
			AvgDurMs: r.AvgDurMs,
		}
	}
	return out, nil
}

func (s *Service) GetSummary(ctx context.Context, callerRole string) (*SummaryResponse, error) {
	if callerRole != auth.RoleAdmin {
		return nil, &middleware.APIError{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: "Admin only"}
	}
	totals, err := s.repo.TotalsByRole(ctx)
	if err != nil {
		return nil, err
	}
	topEndpoints, err := s.repo.TopEndpointsPerRole(ctx)
	if err != nil {
		return nil, err
	}

	totalResponses := make([]*RoleTotalResponse, len(totals))
	for i, t := range totals {
		totalResponses[i] = &RoleTotalResponse{Role: t.Role, Total: t.Total}
	}

	topResponses := make([]*TopEndpointResponse, len(topEndpoints))
	for i, t := range topEndpoints {
		topResponses[i] = &TopEndpointResponse{
			Role:   t.Role,
			Path:   t.Path,
			Method: t.Method,
			Count:  t.Count,
		}
	}

	mostActive := ""
	if len(totalResponses) > 0 {
		mostActive = totalResponses[0].Role // already sorted DESC by total
	}

	return &SummaryResponse{
		TotalsByRole:        totalResponses,
		MostActiveRole:      mostActive,
		TopEndpointsPerRole: topResponses,
	}, nil
}
