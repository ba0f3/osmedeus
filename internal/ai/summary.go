package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/database"
)

type ContextSummaryRequest struct {
	Target    string `json:"target"`
	Workspace string `json:"workspace,omitempty"`
}

type ContextSummaryResponse struct {
	Target       string             `json:"target"`
	Workspaces   []WorkspaceSummary `json:"workspaces"`
	RunCount     int                `json:"run_count"`
	RunsByStatus map[string]int     `json:"runs_by_status,omitempty"`
	RecentRuns   []database.Run     `json:"recent_runs,omitempty"`
}

func (s *Service) ContextSummary(ctx context.Context, req ContextSummaryRequest) (*ContextSummaryResponse, error) {
	resolved, err := s.ResolveTarget(ctx, ResolveTargetRequest{Target: req.Target})
	if err != nil {
		return nil, err
	}

	resp := &ContextSummaryResponse{
		Target:       resolved.Target,
		Workspaces:   resolved.Workspaces,
		RunCount:     resolved.RunCount,
		RunsByStatus: map[string]int{},
	}

	runs, err := s.ListRuns(ctx, ListRunsRequest{
		Target:    req.Target,
		Workspace: req.Workspace,
		Limit:     10,
	})
	if err == nil && runs != nil {
		resp.RecentRuns = runs.Records
		for _, run := range runs.Records {
			resp.RunsByStatus[run.Status]++
		}
	}

	return resp, nil
}

type GetRunRequest struct {
	RunUUID          string `json:"run_uuid"`
	IncludeSteps     bool   `json:"include_steps,omitempty"`
	IncludeArtifacts bool   `json:"include_artifacts,omitempty"`
}

type GetRunResponse struct {
	Run *database.Run `json:"run"`
}

func (s *Service) GetRun(ctx context.Context, req GetRunRequest) (*GetRunResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	runUUID := strings.TrimSpace(req.RunUUID)
	if runUUID == "" {
		return nil, fmt.Errorf("run_uuid is required")
	}
	run, err := database.GetRunByID(ctx, runUUID, req.IncludeSteps, req.IncludeArtifacts)
	if err != nil {
		return nil, err
	}
	return &GetRunResponse{Run: run}, nil
}
