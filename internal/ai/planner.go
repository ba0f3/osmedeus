package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/database"
)

type PlanRunRequest struct {
	Target    string `json:"target"`
	Goal      string `json:"goal"`
	Prompt    string `json:"prompt,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type PlanRunRecommendation struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type PlanRunResponse struct {
	Target          string                  `json:"target"`
	Workspace       string                  `json:"workspace,omitempty"`
	Goal            string                  `json:"goal"`
	ContextSummary  *ContextSummaryResponse `json:"context_summary,omitempty"`
	WorkflowMatches []database.WorkflowMeta `json:"workflow_matches,omitempty"`
	Recommendation  PlanRunRecommendation   `json:"recommendation"`
	NextSteps       []string                `json:"next_steps"`
}

func (s *Service) PlanRun(ctx context.Context, req PlanRunRequest) (*PlanRunResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		goal = strings.TrimSpace(req.Prompt)
	}
	if goal == "" {
		return nil, fmt.Errorf("goal or prompt is required")
	}

	summary, _ := s.ContextSummary(ctx, ContextSummaryRequest{
		Target:    target,
		Workspace: req.Workspace,
	})

	searchTerms := extractSearchTerms(goal, target)
	workflows, _ := s.SearchWorkflows(ctx, SearchWorkflowsRequest{
		Search: searchTerms,
		Limit:  10,
	})

	resp := &PlanRunResponse{
		Target:         target,
		Workspace:      req.Workspace,
		Goal:           goal,
		ContextSummary: summary,
		NextSteps: []string{
			"Review workflow_matches or generate a new workflow",
			"Validate generated YAML before save",
			"Request approval before saving workflows or launching scans",
			"Use osmedeus.runs.start_approved after approval",
		},
	}
	if workflows != nil {
		resp.WorkflowMatches = workflows.Records
	}

	if workflows != nil && len(workflows.Records) > 0 {
		best := workflows.Records[0]
		resp.Recommendation = PlanRunRecommendation{
			Type:        "existing_workflow",
			Name:        best.Name,
			Description: best.Description,
			Reason:      "Existing indexed workflow matches the scan goal",
		}
	} else {
		resp.Recommendation = PlanRunRecommendation{
			Type:   "generate_workflow",
			Reason: "No suitable existing workflow found; generate and validate a workflow",
		}
	}

	return resp, nil
}

// ApprovedRunPayload is the run request embedded in a start_run approval.
type ApprovedRunPayload struct {
	Flow            string            `json:"flow,omitempty"`
	Module          string            `json:"module,omitempty"`
	Target          string            `json:"target,omitempty"`
	Targets         []string          `json:"targets,omitempty"`
	TargetFile      string            `json:"target_file,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	Priority        string            `json:"priority,omitempty"`
	RunMode         string            `json:"run_mode,omitempty"`
	RunnerType      string            `json:"runner_type,omitempty"`
	DockerImage     string            `json:"docker_image,omitempty"`
	SSHHost         string            `json:"ssh_host,omitempty"`
	HeuristicsCheck string            `json:"heuristics_check,omitempty"`
	RepeatWaitTime  string            `json:"repeat_wait_time,omitempty"`
	Timeout         int               `json:"timeout,omitempty"`
	Concurrency     int               `json:"concurrency,omitempty"`
	ThreadsHold     int               `json:"threads_hold,omitempty"`
	EmptyTarget     bool              `json:"empty_target,omitempty"`
	Repeat          bool              `json:"repeat,omitempty"`
}

type StartApprovedRunRequest struct {
	ApprovalID string `json:"approval_id"`
}

type StartApprovedRunResponse struct {
	ApprovalID string             `json:"approval_id"`
	RunPayload ApprovedRunPayload `json:"run_payload"`
}

func (s *Service) ResolveApprovedRun(ctx context.Context, req StartApprovedRunRequest) (*StartApprovedRunResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	approval, err := s.requireApprovedAction(ctx, req.ApprovalID, database.AIActionStartRun)
	if err != nil {
		return nil, err
	}

	var payload ApprovedRunPayload
	if err := json.Unmarshal([]byte(approval.RequestedPayloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("invalid approval payload")
	}

	return &StartApprovedRunResponse{
		ApprovalID: approval.ID,
		RunPayload: payload,
	}, nil
}

func (s *Service) CompleteApprovedRun(ctx context.Context, approvalID string, result map[string]string) error {
	return s.markApprovalExecuted(ctx, approvalID, result)
}

func extractSearchTerms(goal, target string) string {
	goal = strings.ToLower(goal)
	for _, word := range []string{"scan", "for", "likely", "vulns", "vulnerabilities", "the", "a", "an"} {
		goal = strings.ReplaceAll(goal, word, " ")
	}
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return target
	}
	return goal
}
