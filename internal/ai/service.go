package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/database"
	"github.com/uptrace/bun"
)

type Service struct {
	db       *bun.DB
	maxLimit int
}

func NewService(db *bun.DB, cfg ServiceConfig) *Service {
	maxLimit := cfg.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 50
	}
	return &Service{db: db, maxLimit: maxLimit}
}

func (s *Service) requireDB() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("database not connected")
	}
	return nil
}

func (s *Service) clampLimit(limit int) int {
	if limit <= 0 {
		return s.maxLimit
	}
	if limit > s.maxLimit {
		return s.maxLimit
	}
	return limit
}

func (s *Service) ResolveTarget(ctx context.Context, req ResolveTargetRequest) (*ResolveTargetResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}

	var workspaces []database.Workspace
	err := s.db.NewSelect().
		Model(&workspaces).
		Where("name = ? OR name LIKE ?", target, "%"+target+"%").
		Order("updated_at DESC").
		Limit(s.maxLimit).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	count, err := s.db.NewSelect().
		Model((*database.Run)(nil)).
		Where("target = ? OR workspace = ?", target, target).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	resp := &ResolveTargetResponse{Target: target, RunCount: count}
	for _, ws := range workspaces {
		summary := WorkspaceSummary{
			Name:        ws.Name,
			TotalAssets: ws.TotalAssets,
			TotalVulns:  ws.TotalVulns,
			RiskScore:   ws.RiskScore,
		}
		if ws.LastRun != nil {
			summary.LastRun = ws.LastRun.Format("2006-01-02T15:04:05Z07:00")
		}
		resp.Workspaces = append(resp.Workspaces, summary)
	}
	return resp, nil
}

func (s *Service) SearchAssets(ctx context.Context, req SearchAssetsRequest) (*SearchAssetsResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	limit := s.clampLimit(req.Limit)
	filters := map[string]string{}
	if req.Workspace != "" {
		filters["workspace"] = req.Workspace
	}
	fuzzy := map[string]string{}
	if req.AssetType != "" {
		fuzzy["asset_type"] = req.AssetType
	}
	records, err := database.GetTableRecords(ctx, "assets", req.Offset, limit, filters, fuzzy, req.Search, database.AssetHeavyColumns)
	if err != nil {
		return nil, err
	}
	assets, ok := records.Records.([]database.Asset)
	if !ok {
		return nil, fmt.Errorf("unexpected assets result type")
	}
	return &SearchAssetsResponse{
		Total:   records.TotalCount,
		Limit:   limit,
		Offset:  req.Offset,
		Records: assets,
	}, nil
}

func (s *Service) SearchVulnerabilities(ctx context.Context, req SearchVulnerabilitiesRequest) (*SearchVulnerabilitiesResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	limit := s.clampLimit(req.Limit)
	filters := map[string]string{}
	if req.Workspace != "" {
		filters["workspace"] = req.Workspace
	}
	if req.Severity != "" {
		filters["severity"] = req.Severity
	}
	records, err := database.GetTableRecords(ctx, "vulnerabilities", req.Offset, limit, filters, nil, req.Search, nil)
	if err != nil {
		return nil, err
	}
	vulns, ok := records.Records.([]database.Vulnerability)
	if !ok {
		return nil, fmt.Errorf("unexpected vulnerabilities result type")
	}
	return &SearchVulnerabilitiesResponse{
		Total:   records.TotalCount,
		Limit:   limit,
		Offset:  req.Offset,
		Records: vulns,
	}, nil
}

func (s *Service) ListRuns(ctx context.Context, req ListRunsRequest) (*ListRunsResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	limit := s.clampLimit(req.Limit)
	filters := map[string]string{}
	if req.Workspace != "" {
		filters["workspace"] = req.Workspace
	}
	if req.Status != "" {
		filters["status"] = req.Status
	}
	fuzzy := map[string]string{}
	if req.Target != "" {
		fuzzy["target"] = req.Target
	}
	records, err := database.GetTableRecords(ctx, "runs", req.Offset, limit, filters, fuzzy, "", nil)
	if err != nil {
		return nil, err
	}
	runs, ok := records.Records.([]database.Run)
	if !ok {
		return nil, fmt.Errorf("unexpected runs result type")
	}
	return &ListRunsResponse{
		Total:   records.TotalCount,
		Limit:   limit,
		Offset:  req.Offset,
		Records: runs,
	}, nil
}
