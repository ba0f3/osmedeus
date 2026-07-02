package handlers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/j3ssie/osmedeus/v5/internal/core"
	"github.com/j3ssie/osmedeus/v5/internal/database"
	"github.com/j3ssie/osmedeus/v5/internal/executor"
	"github.com/j3ssie/osmedeus/v5/internal/parser"
)

// SubmitRunResponse is returned when a run is queued for local execution.
type SubmitRunResponse struct {
	Message     string   `json:"message"`
	Workflow    string   `json:"workflow"`
	Kind        string   `json:"kind"`
	TargetCount int      `json:"target_count"`
	Priority    string   `json:"priority"`
	RunMode     string   `json:"run_mode"`
	JobID       string   `json:"job_id"`
	Status      string   `json:"status"`
	PollURL     string   `json:"poll_url"`
	Target      string   `json:"target,omitempty"`
	RunUUID     string   `json:"run_uuid,omitempty"`
	Targets     []string `json:"targets,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
	RunUUIDs    []string `json:"run_uuids,omitempty"`
}

// SubmitLocalRun validates and starts a local-mode workflow run asynchronously.
// It mirrors the local execution path used by POST /osm/api/runs.
func SubmitLocalRun(cfg *config.Config, req CreateRunRequest) (*SubmitRunResponse, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	if !req.SkipValidation {
		if err := validateCreateRunInput(&req); err != nil {
			return nil, newMCPClientError(-32602, err.Error())
		}
	}

	if req.EmptyTarget {
		req.Target = generateEmptyTarget()
	}

	targets, err := collectTargetsFromRequest(&req)
	if err != nil {
		return nil, newMCPClientError(-32602, err.Error())
	}
	if len(targets) == 0 {
		return nil, newMCPClientError(-32602, "At least one target is required (target, targets, target_file, or empty_target)")
	}

	if req.HeuristicsCheck != "" {
		validHeuristics := map[string]bool{"none": true, "basic": true, "advanced": true}
		if !validHeuristics[req.HeuristicsCheck] {
			return nil, newMCPClientError(-32602, "Invalid heuristics_check value. Must be: none, basic, or advanced")
		}
	}

	workflowName := req.Flow
	isFlow := true
	if workflowName == "" {
		workflowName = req.Module
		isFlow = false
	}
	if workflowName == "" {
		return nil, newMCPClientError(-32602, "Either 'flow' or 'module' is required")
	}

	loader := parser.NewLoader(cfg.WorkflowsPath)
	workflow, err := loader.LoadWorkflow(workflowName)
	if err != nil {
		return nil, newMCPClientError(-32602, "Workflow not found")
	}

	params := req.Params
	if params == nil {
		params = make(map[string]string)
	}

	priority := req.Priority
	if priority == "" {
		priority = "normal"
	}
	validPriorities := map[string]bool{"low": true, "normal": true, "medium": true, "high": true, "critical": true}
	if !validPriorities[priority] {
		return nil, newMCPClientError(-32602, "Invalid priority. Must be one of: low, normal, medium, high, critical")
	}
	if priority == "medium" {
		priority = "normal"
	}

	runMode := req.RunMode
	if runMode == "" {
		runMode = "local"
	}
	if runMode != "local" {
		return nil, newMCPClientError(-32602, "Only local run_mode is supported from MCP; use the API for distributed or cloud runs")
	}

	if req.RunnerType != "" {
		params["runner_type"] = req.RunnerType
	}
	if req.DockerImage != "" {
		params["docker_image"] = req.DockerImage
	}
	if req.SSHHost != "" {
		params["ssh_host"] = req.SSHHost
	}
	if req.Timeout > 0 {
		params["timeout"] = fmt.Sprintf("%d", req.Timeout)
	}
	if req.ThreadsHold > 0 {
		params["threads_hold"] = fmt.Sprintf("%d", req.ThreadsHold)
	}
	if req.HeuristicsCheck != "" {
		params["heuristics_check"] = req.HeuristicsCheck
	}
	if req.Repeat {
		params["repeat"] = "true"
	}
	if req.RepeatWaitTime != "" {
		params["repeat_wait_time"] = req.RepeatWaitTime
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	cfgCopy := config.Get()
	if cfgCopy == nil {
		cfgCopy = cfg
	}

	jobID := uuid.New().String()[:8]
	var runIDs []string

	if len(targets) == 1 {
		params["target"] = targets[0]

		ctx := context.Background()
		run, createErr := createRunRecord(ctx, cfgCopy, workflow, loader, targets[0], params, "mcp", jobID, priority, runMode)
		if createErr != nil {
			return nil, createErr
		}
		if run != nil {
			runIDs = append(runIDs, run.RunUUID)
		}

		exec := executor.NewExecutor()
		exec.SetServerMode(true)
		exec.SetLoader(loader)
		if req.EmptyTarget {
			exec.SetSkipWorkspace(true)
		}
		if run != nil {
			exec.SetDBRunUUID(run.RunUUID)
			exec.SetDBRunID(run.ID)
			exec.SetOnStepCompleted(func(stepCtx context.Context, dbRunUUID string) {
				_ = database.IncrementRunCompletedSteps(stepCtx, dbRunUUID)
			})
		}

		go func(runUUID string, wf *core.Workflow, execParams map[string]string) {
			ctx := context.Background()
			var execErr error
			if isFlow && wf.IsFlow() {
				_, execErr = exec.ExecuteFlow(ctx, wf, execParams, cfgCopy)
			} else {
				_, execErr = exec.ExecuteModule(ctx, wf, execParams, cfgCopy)
			}
			if runUUID != "" {
				if execErr != nil {
					_ = database.UpdateRunStatus(ctx, runUUID, "failed", execErr.Error())
				} else {
					_ = database.UpdateRunStatus(ctx, runUUID, "completed", "")
				}
			}
		}(func() string {
			if run != nil {
				return run.RunUUID
			}
			return ""
		}(), workflow, cloneStringMap(params))
	} else {
		go executeRunsConcurrently(workflow, targets, params, cfgCopy, concurrency, isFlow, jobID, priority, runMode)
	}

	resp := &SubmitRunResponse{
		Message:     "Run started",
		Workflow:    workflow.Name,
		Kind:        string(workflow.Kind),
		TargetCount: len(targets),
		Priority:    priority,
		RunMode:     runMode,
		JobID:       jobID,
		Status:      "queued",
		PollURL:     fmt.Sprintf("/osm/api/jobs/%s", jobID),
		RunUUIDs:    runIDs,
	}
	if len(targets) == 1 {
		resp.Target = targets[0]
		if len(runIDs) > 0 {
			resp.RunUUID = runIDs[0]
		}
	} else {
		resp.Targets = targets
		resp.Concurrency = concurrency
	}
	return resp, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func createRunRequestFromMCPArgs(args map[string]interface{}) CreateRunRequest {
	req := CreateRunRequest{
		Flow:            stringArg(args, "flow"),
		Module:          stringArg(args, "module"),
		Target:          stringArg(args, "target"),
		TargetFile:      stringArg(args, "target_file"),
		Priority:        stringArg(args, "priority"),
		RunMode:         stringArg(args, "run_mode"),
		RunnerType:      stringArg(args, "runner_type"),
		DockerImage:     stringArg(args, "docker_image"),
		SSHHost:         stringArg(args, "ssh_host"),
		HeuristicsCheck: stringArg(args, "heuristics_check"),
		RepeatWaitTime:  stringArg(args, "repeat_wait_time"),
		Timeout:         intArg(args, "timeout"),
		Concurrency:     intArg(args, "concurrency"),
		ThreadsHold:     intArg(args, "threads_hold"),
		EmptyTarget:     boolArg(args, "empty_target"),
		Repeat:          boolArg(args, "repeat"),
	}
	if raw, ok := args["targets"].([]interface{}); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok && s != "" {
				req.Targets = append(req.Targets, s)
			}
		}
	}
	if raw, ok := args["params"].(map[string]interface{}); ok {
		req.Params = make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				req.Params[k] = s
			}
		}
	}
	return req
}

func boolArg(args map[string]interface{}, key string) bool {
	if args == nil {
		return false
	}
	v, ok := args[key].(bool)
	return ok && v
}
