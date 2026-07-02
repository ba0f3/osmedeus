package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/j3ssie/osmedeus/v5/internal/ai"
	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/j3ssie/osmedeus/v5/internal/database"
	oslogger "github.com/j3ssie/osmedeus/v5/internal/logger"
	"go.uber.org/zap"
)

const mcpInternalErrorMsg = "Internal error"

type mcpRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id,omitempty"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpClientError struct {
	code    int
	message string
}

func (e *mcpClientError) Error() string {
	return e.message
}

func newMCPClientError(code int, message string) error {
	return &mcpClientError{code: code, message: message}
}

func mcpErrorFrom(err error, reqID interface{}, method string) *mcpError {
	var clientErr *mcpClientError
	if errors.As(err, &clientErr) {
		return &mcpError{Code: clientErr.code, Message: clientErr.message}
	}
	oslogger.Get().Error("MCP request failed",
		zap.Any("id", reqID),
		zap.String("method", method),
		zap.Error(err))
	return &mcpError{Code: -32603, Message: mcpInternalErrorMsg}
}

func MCP(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req mcpRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(mcpResponse{
				JSONRPC: "2.0",
				Error:   &mcpError{Code: -32700, Message: "invalid JSON"},
			})
		}
		result, err := dispatchMCPTool(c.UserContext(), cfg, req)
		resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}
		if err != nil {
			resp.Error = mcpErrorFrom(err, req.ID, req.Method)
		} else {
			resp.Result = result
		}
		return c.JSON(resp)
	}
}

func MCPHealth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"name":   "osmedeus",
			"status": "ok",
			"tools":  len(mcpToolDefinitions()),
		})
	}
}

func dispatchMCPTool(ctx context.Context, cfg *config.Config, req mcpRequest) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "osmedeus",
				"version": "dev",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		}, nil
	case "tools/list":
		return map[string]interface{}{"tools": mcpToolDefinitions()}, nil
	case "tools/call":
		return callMCPTool(ctx, cfg, req.Params)
	default:
		return nil, newMCPClientError(-32601, "Method not found")
	}
}

func mcpToolDefinitions() []map[string]interface{} {
	tools := []struct {
		name        string
		description string
	}{
		{"osmedeus.context.resolve_target", "Resolve an Osmedeus target to known workspaces and run counts"},
		{"osmedeus.context.summary", "Summarize target context including workspaces and recent runs"},
		{"osmedeus.assets.search", "Search discovered assets"},
		{"osmedeus.vulns.search", "Search discovered vulnerabilities"},
		{"osmedeus.runs.list", "List workflow runs"},
		{"osmedeus.runs.get", "Get a single run by UUID"},
		{"osmedeus.artifacts.list", "List artifacts with pagination and filters"},
		{"osmedeus.artifacts.read", "Read bounded artifact content from a workspace"},
		{"osmedeus.workflows.search", "Search indexed workflows"},
		{"osmedeus.workflows.get", "Get workflow metadata and definition"},
		{"osmedeus.workflows.generate", "Generate workflow YAML using configured LLM providers"},
		{"osmedeus.workflows.validate", "Validate workflow YAML content"},
		{"osmedeus.workflows.promote_temp", "Promote a temporary generated workflow to the normal workflow folder (requires approval)"},
		{"osmedeus.approvals.request", "Request approval for gated actions such as run launch or workflow save"},
		{"osmedeus.approvals.get", "Get approval status and payload"},
		{"osmedeus.approvals.approve", "Approve or reject a pending approval"},
		{"osmedeus.runs.plan", "Plan a one-shot scan using existing context and workflows"},
		{"osmedeus.runs.start", "Start a local workflow run directly (bypasses approval gate)"},
		{"osmedeus.runs.start_approved", "Start a local run using an approved start_run approval"},
	}
	out := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]interface{}{
			"name":        tool.name,
			"description": tool.description,
		})
	}
	return out
}

func callMCPTool(ctx context.Context, cfg *config.Config, params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})

	switch name {
	case "osmedeus.runs.start":
		if cfg == nil {
			return nil, fmt.Errorf("configuration not loaded")
		}
		req := createRunRequestFromMCPArgs(args)
		return SubmitLocalRun(cfg, req)
	case "osmedeus.runs.start_approved":
		if cfg == nil {
			return nil, fmt.Errorf("configuration not loaded")
		}
		svc := newMCPService(cfg)
		resolved, err := svc.ResolveApprovedRun(ctx, ai.StartApprovedRunRequest{
			ApprovalID: stringArg(args, "approval_id"),
		})
		if err != nil {
			return nil, err
		}
		submitResp, err := SubmitLocalRun(cfg, approvedRunPayloadToCreateRun(resolved.RunPayload))
		if err != nil {
			return nil, err
		}
		_ = svc.CompleteApprovedRun(ctx, resolved.ApprovalID, map[string]string{
			"run_uuid": submitResp.RunUUID,
			"job_id":   submitResp.JobID,
			"workflow": submitResp.Workflow,
		})
		return submitResp, nil
	default:
		svc := newMCPService(cfg)
		switch name {
		case "osmedeus.context.resolve_target":
			return svc.ResolveTarget(ctx, ai.ResolveTargetRequest{Target: stringArg(args, "target")})
		case "osmedeus.context.summary":
			return svc.ContextSummary(ctx, ai.ContextSummaryRequest{
				Target:    stringArg(args, "target"),
				Workspace: stringArg(args, "workspace"),
			})
		case "osmedeus.assets.search":
			return svc.SearchAssets(ctx, ai.SearchAssetsRequest{
				Workspace: stringArg(args, "workspace"),
				Search:    stringArg(args, "search"),
				AssetType: stringArg(args, "asset_type"),
				Limit:     intArg(args, "limit"),
				Offset:    intArg(args, "offset"),
			})
		case "osmedeus.vulns.search":
			return svc.SearchVulnerabilities(ctx, ai.SearchVulnerabilitiesRequest{
				Workspace: stringArg(args, "workspace"),
				Severity:  stringArg(args, "severity"),
				Search:    stringArg(args, "search"),
				Limit:     intArg(args, "limit"),
				Offset:    intArg(args, "offset"),
			})
		case "osmedeus.runs.list":
			return svc.ListRuns(ctx, ai.ListRunsRequest{
				Target:    stringArg(args, "target"),
				Workspace: stringArg(args, "workspace"),
				Status:    stringArg(args, "status"),
				Limit:     intArg(args, "limit"),
				Offset:    intArg(args, "offset"),
			})
		case "osmedeus.runs.get":
			return svc.GetRun(ctx, ai.GetRunRequest{
				RunUUID:          stringArg(args, "run_uuid"),
				IncludeSteps:     boolArg(args, "include_steps"),
				IncludeArtifacts: boolArg(args, "include_artifacts"),
			})
		case "osmedeus.artifacts.list":
			return svc.ListArtifacts(ctx, ai.ListArtifactsRequest{
				Workspace: stringArg(args, "workspace"),
				Search:    stringArg(args, "search"),
				RunUUID:   stringArg(args, "run_uuid"),
				Limit:     intArg(args, "limit"),
				Offset:    intArg(args, "offset"),
			})
		case "osmedeus.artifacts.read":
			return svc.ReadArtifact(ctx, ai.ReadArtifactRequest{
				Workspace:    stringArg(args, "workspace"),
				ArtifactPath: stringArg(args, "artifact_path"),
				MaxBytes:     intArg(args, "max_bytes"),
			})
		case "osmedeus.workflows.search":
			return svc.SearchWorkflows(ctx, ai.SearchWorkflowsRequest{
				Search: stringArg(args, "search"),
				Kind:   stringArg(args, "kind"),
				Tags:   stringSliceArg(args, "tags"),
				Limit:  intArg(args, "limit"),
				Offset: intArg(args, "offset"),
			})
		case "osmedeus.workflows.get":
			return svc.GetWorkflow(ctx, ai.GetWorkflowRequest{Name: stringArg(args, "name")})
		case "osmedeus.workflows.generate":
			return svc.GenerateWorkflow(ctx, ai.GenerateWorkflowRequest{
				Prompt:       stringArg(args, "prompt"),
				Purpose:      stringArg(args, "purpose"),
				TargetType:   stringArg(args, "target_type"),
				Target:       stringArg(args, "target"),
				SaveMode:     stringArg(args, "save_mode"),
				WorkflowName: stringArg(args, "workflow_name"),
				Workspace:    stringArg(args, "workspace"),
				ApprovalID:   stringArg(args, "approval_id"),
				Overwrite:    boolArg(args, "overwrite"),
			})
		case "osmedeus.workflows.validate":
			return svc.ValidateWorkflowYAML(ctx, ai.ValidateWorkflowRequest{
				YAML:                stringArg(args, "yaml"),
				GeneratedWorkflowID: int64Arg(args, "generated_workflow_id"),
			})
		case "osmedeus.workflows.promote_temp":
			return svc.PromoteTempWorkflow(ctx, ai.PromoteTempWorkflowRequest{
				GeneratedWorkflowID: int64Arg(args, "generated_workflow_id"),
				WorkflowName:        stringArg(args, "workflow_name"),
				ApprovalID:          stringArg(args, "approval_id"),
				Overwrite:           boolArg(args, "overwrite"),
			})
		case "osmedeus.approvals.request":
			return svc.RequestApproval(ctx, ai.RequestApprovalRequest{
				ActionType:      stringArg(args, "action_type"),
				Payload:         mapArg(args, "payload"),
				RequesterSource: stringArg(args, "requester_source"),
				TTLMinutes:      intArg(args, "ttl_minutes"),
			})
		case "osmedeus.approvals.get":
			return svc.GetApproval(ctx, ai.GetApprovalRequest{ApprovalID: stringArg(args, "approval_id")})
		case "osmedeus.approvals.approve":
			return svc.Approve(ctx, ai.ApproveRequest{
				ApprovalID: stringArg(args, "approval_id"),
				Decision:   stringArg(args, "decision"),
			})
		case "osmedeus.runs.plan":
			return svc.PlanRun(ctx, ai.PlanRunRequest{
				Target:    stringArg(args, "target"),
				Goal:      stringArg(args, "goal"),
				Prompt:    stringArg(args, "prompt"),
				Workspace: stringArg(args, "workspace"),
			})
		default:
			return nil, newMCPClientError(-32601, "Unknown tool")
		}
	}
}

func newMCPService(cfg *config.Config) *ai.Service {
	return ai.NewService(database.GetDB(), ai.ServiceConfig{
		MaxLimit:  50,
		AppConfig: cfg,
	})
}

func approvedRunPayloadToCreateRun(payload ai.ApprovedRunPayload) CreateRunRequest {
	return CreateRunRequest{
		Flow:            payload.Flow,
		Module:          payload.Module,
		Target:          payload.Target,
		Targets:         payload.Targets,
		TargetFile:      payload.TargetFile,
		Params:          payload.Params,
		Priority:        payload.Priority,
		RunMode:         payload.RunMode,
		RunnerType:      payload.RunnerType,
		DockerImage:     payload.DockerImage,
		SSHHost:         payload.SSHHost,
		HeuristicsCheck: payload.HeuristicsCheck,
		RepeatWaitTime:  payload.RepeatWaitTime,
		Timeout:         payload.Timeout,
		Concurrency:     payload.Concurrency,
		ThreadsHold:     payload.ThreadsHold,
		EmptyTarget:     payload.EmptyTarget,
		Repeat:          payload.Repeat,
	}
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	v, _ := args[key].(string)
	return v
}

func intArg(args map[string]interface{}, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func int64Arg(args map[string]interface{}, key string) int64 {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func stringSliceArg(args map[string]interface{}, key string) []string {
	if args == nil {
		return nil
	}
	raw, ok := args[key].([]interface{})
	if !ok {
		if single, ok := args[key].(string); ok && single != "" {
			return []string{single}
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	if args == nil {
		return nil
	}
	raw, _ := args[key].(map[string]interface{})
	return raw
}
