package handlers

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/j3ssie/osmedeus/v5/internal/ai"
	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/j3ssie/osmedeus/v5/internal/database"
)

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
			resp.Error = &mcpError{Code: -32603, Message: err.Error()}
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
		return nil, fmt.Errorf("unsupported MCP method: %s", req.Method)
	}
}

func mcpToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "osmedeus.context.resolve_target", "description": "Resolve an Osmedeus target to known workspaces and run counts"},
		{"name": "osmedeus.assets.search", "description": "Search discovered assets"},
		{"name": "osmedeus.vulns.search", "description": "Search discovered vulnerabilities"},
		{"name": "osmedeus.runs.list", "description": "List workflow runs"},
	}
}

func callMCPTool(ctx context.Context, cfg *config.Config, params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})
	db := database.GetDB()
	if db == nil && cfg != nil {
		var err error
		db, err = database.Connect(cfg)
		if err != nil {
			return nil, err
		}
	}
	svc := ai.NewService(db, ai.ServiceConfig{MaxLimit: 50})
	switch name {
	case "osmedeus.context.resolve_target":
		return svc.ResolveTarget(ctx, ai.ResolveTargetRequest{Target: stringArg(args, "target")})
	case "osmedeus.assets.search":
		return svc.SearchAssets(ctx, ai.SearchAssetsRequest{
			Workspace: stringArg(args, "workspace"),
			Search:    stringArg(args, "search"),
			AssetType: stringArg(args, "asset_type"),
			Limit:     intArg(args, "limit"),
		})
	case "osmedeus.vulns.search":
		return svc.SearchVulnerabilities(ctx, ai.SearchVulnerabilitiesRequest{
			Workspace: stringArg(args, "workspace"),
			Severity:  stringArg(args, "severity"),
			Search:    stringArg(args, "search"),
			Limit:     intArg(args, "limit"),
		})
	case "osmedeus.runs.list":
		return svc.ListRuns(ctx, ai.ListRunsRequest{
			Target:    stringArg(args, "target"),
			Workspace: stringArg(args, "workspace"),
			Status:    stringArg(args, "status"),
			Limit:     intArg(args, "limit"),
		})
	default:
		return nil, fmt.Errorf("unknown MCP tool: %s", name)
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
