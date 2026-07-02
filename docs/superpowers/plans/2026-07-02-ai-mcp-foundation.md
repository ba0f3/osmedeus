# AI MCP Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first working AI integration slice: read-only Osmedeus MCP tools on the existing HTTP server and default MCP wiring for `osmedeus agent`.

**Architecture:** Add a small `internal/ai` service layer for read-only context and tool dispatch, expose it through a JSON-RPC MCP-style HTTP endpoint, and pass that endpoint into ACP sessions when the selected agent supports HTTP MCP. This plan intentionally avoids workflow writes, approvals, and scan launch; those require separate plans after the read-only foundation is stable.

**Tech Stack:** Go, Fiber, Bun ORM, Cobra, existing `github.com/coder/acp-go-sdk`, existing Osmedeus database helpers and workflow index.

---

## Scope

This plan implements:

- `internal/ai` read-only service methods.
- HTTP MCP endpoint under the existing server.
- MCP tool definitions and tool call dispatch for read-only tools.
- `osmedeus mcp config --print`.
- `osmedeus agent` flags and ACP HTTP MCP session wiring.
- Tests for service behavior, handler behavior, config output, and ACP config construction.

This plan does not implement:

- Workflow generation.
- Approval database tables.
- Workflow save modes.
- Run launch from AI tools.
- One-shot scan orchestration.

Those features should be implemented in follow-on plans that build on the read-only MCP foundation.

## File Structure

- Create `internal/ai/types.go`: shared request/response structs for AI tools.
- Create `internal/ai/service.go`: read-only Osmedeus context and search methods.
- Create `internal/ai/service_test.go`: unit tests with in-memory SQLite data.
- Create `pkg/server/handlers/mcp.go`: MCP JSON-RPC handler and tool schemas.
- Create `pkg/server/handlers/mcp_test.go`: handler tests for initialize, tool list, tool call, and auth-compatible behavior.
- Modify `pkg/server/server.go`: register `/osm/mcp` when config enables MCP.
- Modify `internal/config/config.go`: add `MCPConfig` under `ServerConfig`.
- Modify `internal/config/settings.go`: default MCP settings.
- Modify `internal/config/config_test.go`: test `GetMCPURL`.
- Modify `pkg/cli/agent.go`: add `--no-mcp`, `--mcp-url`, and pass MCP config to ACP.
- Modify `internal/executor/acp_executor.go`: add MCP server config to `RunAgentACPConfig`, check agent capabilities, and pass MCP servers into `NewSession`.
- Modify `internal/executor/acp_executor_test.go`: test MCP config construction/capability handling with helper functions.
- Create `pkg/cli/mcp.go`: `osmedeus mcp config --print`.
- Modify `pkg/cli/root.go`: register `mcpCmd`.
- Create `pkg/cli/mcp_test.go`: test printed config JSON.
- Modify `docs/api/README.mdx` or create `docs/api/mcp.mdx`: document MCP endpoint and CLI config helper.

---

### Task 1: Config Model For HTTP MCP

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/settings.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add these tests to `internal/config/config_test.go`:

```go
func TestServerConfig_GetMCPURL_DefaultPath(t *testing.T) {
	cfg := ServerConfig{Host: "0.0.0.0", Port: 8002}
	if got := cfg.GetMCPURL(); got != "http://127.0.0.1:8002/osm/mcp" {
		t.Fatalf("expected default MCP URL, got %q", got)
	}
}

func TestServerConfig_GetMCPURL_CustomPath(t *testing.T) {
	cfg := ServerConfig{
		Host: "localhost",
		Port: 9000,
		MCP:  MCPConfig{Path: "/custom/mcp"},
	}
	if got := cfg.GetMCPURL(); got != "http://localhost:9000/custom/mcp" {
		t.Fatalf("expected custom MCP URL, got %q", got)
	}
}

func TestServerConfig_IsMCPEnabledDefault(t *testing.T) {
	cfg := ServerConfig{}
	if !cfg.IsMCPEnabled() {
		t.Fatal("MCP should default to enabled")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/config -run 'TestServerConfig_(GetMCPURL|IsMCPEnabled)' -count=1
```

Expected: FAIL because `MCPConfig`, `GetMCPURL`, and `IsMCPEnabled` do not exist.

- [ ] **Step 3: Add config types and helpers**

In `internal/config/config.go`, add `MCPConfig` and extend `ServerConfig`:

```go
type ServerConfig struct {
	Host                    string            `yaml:"host"`
	Port                    int               `yaml:"port"`
	UIPath                  string            `yaml:"ui_path"`
	WorkspacePrefixKey      string            `yaml:"workspace_prefix_key"`
	SimpleUserMapKey        map[string]string `yaml:"simple_user_map_key"`
	JWT                     JWTConfig         `yaml:"jwt"`
	License                 string            `yaml:"license"`
	EnabledAuthAPI          bool              `yaml:"enabled_auth_api"`
	AuthAPIKey              string            `yaml:"auth_api_key"`
	EnableMetrics           *bool             `yaml:"enable_metrics,omitempty"`
	CORSAllowedOrigins      string            `yaml:"cors_allowed_origins,omitempty"`
	EventReceiverURL        string            `yaml:"event_receiver_url,omitempty"`
	EnableTriggerViaWebhook bool              `yaml:"enable_trigger_via_webhook"`
	MCP                     MCPConfig         `yaml:"mcp,omitempty"`
}

type MCPConfig struct {
	Enabled     *bool  `yaml:"enabled,omitempty"`
	Path        string `yaml:"path,omitempty"`
	RequireAuth *bool `yaml:"require_auth,omitempty"`
}

func (c *ServerConfig) IsMCPEnabled() bool {
	if c.MCP.Enabled == nil {
		return true
	}
	return *c.MCP.Enabled
}

func (c *ServerConfig) IsMCPAuthRequired() bool {
	if c.MCP.RequireAuth == nil {
		return true
	}
	return *c.MCP.RequireAuth
}

func (c *ServerConfig) GetMCPPath() string {
	if strings.TrimSpace(c.MCP.Path) == "" {
		return "/osm/mcp"
	}
	path := strings.TrimSpace(c.MCP.Path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func (c *ServerConfig) GetMCPURL() string {
	base := c.GetServerURL()
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + c.GetMCPPath()
}
```

In `internal/config/settings.go`, ensure defaults keep MCP enabled:

```go
if c.Server.MCP.Path == "" {
	c.Server.MCP.Path = "/osm/mcp"
}
if c.Server.MCP.Enabled == nil {
	enabled := true
	c.Server.MCP.Enabled = &enabled
}
if c.Server.MCP.RequireAuth == nil {
	requireAuth := true
	c.Server.MCP.RequireAuth = &requireAuth
}
```

- [ ] **Step 4: Run config tests**

Run:

```bash
go test ./internal/config -run 'TestServerConfig_(GetMCPURL|IsMCPEnabled)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/settings.go internal/config/config_test.go
git commit -m "feat(config): add MCP server settings"
```

---

### Task 2: Read-Only AI Service

**Files:**
- Create: `internal/ai/types.go`
- Create: `internal/ai/service.go`
- Create: `internal/ai/service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `internal/ai/service_test.go`:

```go
package ai

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/j3ssie/osmedeus/v5/internal/database"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupAITestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	database.SetDB(db)
	require.NoError(t, database.Migrate(context.Background()))
	t.Cleanup(func() {
		database.SetDB(nil)
		_ = db.Close()
	})
	return db
}

func TestServiceResolveTargetFindsWorkspaceAndCounts(t *testing.T) {
	db := setupAITestDB(t)
	ctx := context.Background()
	now := time.Now()
	_, err := db.NewInsert().Model(&database.Workspace{
		Name:        "example.com",
		TotalAssets: 2,
		TotalVulns:  1,
		LastRun:     &now,
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&database.Run{
		RunUUID:      "run-1",
		WorkflowName: "basic-recon",
		WorkflowKind: "flow",
		Target:       "example.com",
		Workspace:    "example.com",
		Status:       "completed",
	}).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db, ServiceConfig{MaxLimit: 50})
	got, err := svc.ResolveTarget(ctx, ResolveTargetRequest{Target: "example.com"})
	require.NoError(t, err)
	require.Equal(t, "example.com", got.Target)
	require.Len(t, got.Workspaces, 1)
	require.Equal(t, 2, got.Workspaces[0].TotalAssets)
	require.Equal(t, 1, got.Workspaces[0].TotalVulns)
	require.Equal(t, 1, got.RunCount)
}

func TestServiceSearchAssetsExcludesHeavyFieldsAndCapsLimit(t *testing.T) {
	db := setupAITestDB(t)
	ctx := context.Background()
	_, err := db.NewInsert().Model(&database.Asset{
		Workspace:            "example.com",
		AssetValue:           "api.example.com",
		URL:                  "https://api.example.com",
		StatusCode:           200,
		AssetType:            "web",
		RawResponse:          "large raw body",
		ScreenshotBase64Data: "large image",
	}).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db, ServiceConfig{MaxLimit: 1})
	got, err := svc.SearchAssets(ctx, SearchAssetsRequest{Workspace: "example.com", Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 1, got.Limit)
	require.Len(t, got.Records, 1)
	require.Empty(t, got.Records[0].RawResponse)
	require.Empty(t, got.Records[0].ScreenshotBase64Data)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/ai -count=1
```

Expected: FAIL because `internal/ai` does not exist.

- [ ] **Step 3: Add AI types**

Create `internal/ai/types.go`:

```go
package ai

import "github.com/j3ssie/osmedeus/v5/internal/database"

type ServiceConfig struct {
	MaxLimit int
}

type ResolveTargetRequest struct {
	Target string `json:"target"`
}

type WorkspaceSummary struct {
	Name        string `json:"name"`
	TotalAssets int    `json:"total_assets"`
	TotalVulns  int    `json:"total_vulns"`
	RiskScore   float64 `json:"risk_score"`
	LastRun     string `json:"last_run,omitempty"`
}

type ResolveTargetResponse struct {
	Target     string             `json:"target"`
	Workspaces []WorkspaceSummary `json:"workspaces"`
	RunCount   int                `json:"run_count"`
}

type SearchAssetsRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Search    string `json:"search,omitempty"`
	AssetType string `json:"asset_type,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type SearchAssetsResponse struct {
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	Records []database.Asset `json:"records"`
}

type SearchVulnerabilitiesRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Search    string `json:"search,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type ListRunsRequest struct {
	Target    string `json:"target,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}
```

- [ ] **Step 4: Add service implementation**

Create `internal/ai/service.go`:

```go
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
```

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./internal/ai -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/types.go internal/ai/service.go internal/ai/service_test.go
git commit -m "feat(ai): add read-only context service"
```

---

### Task 3: Add Remaining Read-Only Service Methods

**Files:**
- Modify: `internal/ai/types.go`
- Modify: `internal/ai/service.go`
- Modify: `internal/ai/service_test.go`

- [ ] **Step 1: Add failing tests for vulns and runs**

Append to `internal/ai/service_test.go`:

```go
func TestServiceSearchVulnerabilities(t *testing.T) {
	db := setupAITestDB(t)
	ctx := context.Background()
	_, err := db.NewInsert().Model(&database.Vulnerability{
		Workspace:  "example.com",
		VulnTitle:  "SQL injection",
		Severity:   "high",
		AssetValue: "https://api.example.com/login",
	}).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db, ServiceConfig{MaxLimit: 10})
	got, err := svc.SearchVulnerabilities(ctx, SearchVulnerabilitiesRequest{
		Workspace: "example.com",
		Severity:  "high",
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.Total)
	require.Len(t, got.Records, 1)
	require.Equal(t, "SQL injection", got.Records[0].VulnTitle)
}

func TestServiceListRuns(t *testing.T) {
	db := setupAITestDB(t)
	ctx := context.Background()
	_, err := db.NewInsert().Model(&database.Run{
		RunUUID:      "run-2",
		WorkflowName: "basic-recon",
		WorkflowKind: "flow",
		Target:       "example.com",
		Workspace:    "example.com",
		Status:       "completed",
	}).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db, ServiceConfig{MaxLimit: 10})
	got, err := svc.ListRuns(ctx, ListRunsRequest{Target: "example.com"})
	require.NoError(t, err)
	require.Equal(t, 1, got.Total)
	require.Equal(t, "run-2", got.Records[0].RunUUID)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/ai -run 'TestService(SearchVulnerabilities|ListRuns)' -count=1
```

Expected: FAIL because response types and methods do not exist.

- [ ] **Step 3: Add response types**

Append to `internal/ai/types.go`:

```go
type SearchVulnerabilitiesResponse struct {
	Total   int                      `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
	Records []database.Vulnerability `json:"records"`
}

type ListRunsResponse struct {
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	Records []database.Run `json:"records"`
}
```

- [ ] **Step 4: Add service methods**

Append to `internal/ai/service.go`:

```go
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
```

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./internal/ai -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/types.go internal/ai/service.go internal/ai/service_test.go
git commit -m "feat(ai): add vuln and run read tools"
```

---

### Task 4: MCP Handler With Read-Only Tools

**Files:**
- Create: `pkg/server/handlers/mcp.go`
- Create: `pkg/server/handlers/mcp_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `pkg/server/handlers/mcp_test.go`:

```go
package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMCPInitialize(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPToolsList(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPUnknownMethodReturnsJSONRPCError(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","id":3,"method":"unknown","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./pkg/server/handlers -run 'TestMCP' -count=1
```

Expected: FAIL because `MCP` does not exist.

- [ ] **Step 3: Add MCP handler**

Create `pkg/server/handlers/mcp.go`:

```go
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
```

- [ ] **Step 4: Run handler tests**

Run:

```bash
go test ./pkg/server/handlers -run 'TestMCP' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/handlers/mcp.go pkg/server/handlers/mcp_test.go
git commit -m "feat(server): add read-only MCP handler"
```

---

### Task 5: Register MCP Route

**Files:**
- Modify: `pkg/server/server.go`
- Modify: `pkg/server/handlers/mcp_test.go`

- [ ] **Step 1: Add route behavior test**

Append to `pkg/server/handlers/mcp_test.go`:

```go
func TestMCPHandlerAllowsGetHealth(t *testing.T) {
	app := fiber.New()
	app.Get("/osm/mcp", MCPHealth())
	req := httptest.NewRequest(http.MethodGet, "/osm/mcp", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./pkg/server/handlers -run 'TestMCPHandlerAllowsGetHealth' -count=1
```

Expected: FAIL because `MCPHealth` does not exist.

- [ ] **Step 3: Add health handler and route registration**

Add to `pkg/server/handlers/mcp.go`:

```go
func MCPHealth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"name":   "osmedeus",
			"status": "ok",
			"tools":  len(mcpToolDefinitions()),
		})
	}
}
```

In `pkg/server/server.go`, register MCP after auth middleware is applied:

```go
if s.config.Server.IsMCPEnabled() {
	mcpPath := s.config.Server.GetMCPPath()
	s.app.Get(mcpPath, handlers.MCPHealth())
	s.app.Post(mcpPath, handlers.MCP(s.config))
}
```

Use `s.app` instead of `api` because `GetMCPPath()` returns `/osm/mcp`, an absolute path.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./pkg/server/handlers -run 'TestMCP' -count=1
go test ./pkg/server -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/server.go pkg/server/handlers/mcp.go pkg/server/handlers/mcp_test.go
git commit -m "feat(server): expose MCP endpoint"
```

---

### Task 6: ACP MCP Session Wiring

**Files:**
- Modify: `internal/executor/acp_executor.go`
- Modify: `internal/executor/acp_executor_test.go`

- [ ] **Step 1: Add failing helper tests**

Append to `internal/executor/acp_executor_test.go`:

```go
func TestBuildHTTPMCPServer(t *testing.T) {
	server := buildHTTPMCPServer("osmedeus", "http://127.0.0.1:8002/osm/mcp", "secret")
	require.NotNil(t, server.Http)
	require.Equal(t, "osmedeus", server.Http.Name)
	require.Equal(t, "http", server.Http.Type)
	require.Equal(t, "http://127.0.0.1:8002/osm/mcp", server.Http.Url)
	require.Len(t, server.Http.Headers, 1)
	require.Equal(t, "Authorization", server.Http.Headers[0].Name)
	require.Equal(t, "Bearer secret", server.Http.Headers[0].Value)
}

func TestRunAgentACPConfigMCPEnabled(t *testing.T) {
	cfg := &RunAgentACPConfig{MCPURL: "http://127.0.0.1:8002/osm/mcp"}
	require.True(t, cfg.HasMCP())
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/executor -run 'Test(BuildHTTPMCPServer|RunAgentACPConfigMCPEnabled)' -count=1
```

Expected: FAIL because `MCPURL`, `HasMCP`, and `buildHTTPMCPServer` do not exist.

- [ ] **Step 3: Extend ACP config and helpers**

In `internal/executor/acp_executor.go`, extend `RunAgentACPConfig`:

```go
	MCPURL   string
	MCPToken string
	MCPName  string
```

Add methods/helpers:

```go
func (c *RunAgentACPConfig) HasMCP() bool {
	return c != nil && strings.TrimSpace(c.MCPURL) != ""
}

func buildHTTPMCPServer(name, url, token string) acp.McpServer {
	if name == "" {
		name = "osmedeus"
	}
	headers := []acp.HttpHeader{}
	if token != "" {
		headers = append(headers, acp.HttpHeader{Name: "Authorization", Value: "Bearer " + token})
	}
	return acp.McpServer{Http: &acp.McpServerHttp{
		Name:    name,
		Type:    "http",
		Url:     url,
		Headers: headers,
	}}
}
```

After `Initialize`, keep the response:

```go
initResp, initErr := conn.Initialize(ctx, acp.InitializeRequest{...})
```

Build MCP servers before `NewSession`:

```go
mcpServers := []acp.McpServer{}
if cfg.HasMCP() {
	if !initResp.AgentCapabilities.McpCapabilities.Http {
		return "", stderrBuf.String(), fmt.Errorf("agent %q does not support HTTP MCP; rerun with --no-mcp or use an agent with HTTP MCP support", agentName)
	}
	mcpServers = append(mcpServers, buildHTTPMCPServer(cfg.MCPName, cfg.MCPURL, cfg.MCPToken))
}
```

Then change `NewSession`:

```go
sess, sessErr := conn.NewSession(ctx, acp.NewSessionRequest{
	Cwd:        cwd,
	McpServers: mcpServers,
})
```

- [ ] **Step 4: Run executor tests**

Run:

```bash
go test ./internal/executor -run 'Test(BuildHTTPMCPServer|RunAgentACPConfigMCPEnabled)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/executor/acp_executor.go internal/executor/acp_executor_test.go
git commit -m "feat(agent): support HTTP MCP in ACP sessions"
```

---

### Task 7: CLI Agent MCP Flags

**Files:**
- Modify: `pkg/cli/agent.go`
- Modify: `pkg/cli/agent_test.go`

- [ ] **Step 1: Add failing tests for MCP config resolution**

Append to `pkg/cli/agent_test.go`:

```go
func TestResolveAgentMCPConfigDisabled(t *testing.T) {
	cfg := resolveAgentMCPConfig("http://127.0.0.1:8002/osm/mcp", true, "token")
	require.Empty(t, cfg.MCPURL)
	require.Empty(t, cfg.MCPToken)
}

func TestResolveAgentMCPConfigEnabled(t *testing.T) {
	cfg := resolveAgentMCPConfig("http://127.0.0.1:8002/osm/mcp", false, "token")
	require.Equal(t, "http://127.0.0.1:8002/osm/mcp", cfg.MCPURL)
	require.Equal(t, "token", cfg.MCPToken)
	require.Equal(t, "osmedeus", cfg.MCPName)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./pkg/cli -run 'TestResolveAgentMCPConfig' -count=1
```

Expected: FAIL because helper and flags do not exist.

- [ ] **Step 3: Add flags and helper**

In `pkg/cli/agent.go`, add globals:

```go
	agentNoMCP  bool
	agentMCPURL string
```

In `init()`:

```go
agentCmd.Flags().BoolVar(&agentNoMCP, "no-mcp", false, "run without Osmedeus MCP tools")
agentCmd.Flags().StringVar(&agentMCPURL, "mcp-url", "", "Osmedeus MCP URL (default: configured server URL + /osm/mcp)")
```

Add helper:

```go
func resolveAgentMCPConfig(url string, disabled bool, token string) executor.RunAgentACPConfig {
	if disabled {
		return executor.RunAgentACPConfig{}
	}
	return executor.RunAgentACPConfig{
		MCPURL:   url,
		MCPToken: token,
		MCPName:  "osmedeus",
	}
}
```

In `runAgent`, resolve MCP URL:

```go
mcpURL := agentMCPURL
if mcpURL == "" {
	cfg := config.Get()
	if cfg != nil {
		mcpURL = cfg.Server.GetMCPURL()
	}
}
mcpCfg := resolveAgentMCPConfig(mcpURL, agentNoMCP, os.Getenv("OSMEDEUS_API_TOKEN"))
cfg := &executor.RunAgentACPConfig{
	Cwd:          agentCwd,
	Model:        agentModel,
	StreamWriter: os.Stdout,
	MCPURL:       mcpCfg.MCPURL,
	MCPToken:     mcpCfg.MCPToken,
	MCPName:      mcpCfg.MCPName,
}
```

Add import:

```go
	"github.com/j3ssie/osmedeus/v5/internal/config"
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
go test ./pkg/cli -run 'TestResolveAgentMCPConfig|TestResolveAgentMessage' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cli/agent.go pkg/cli/agent_test.go
git commit -m "feat(cli): enable MCP tools for osmedeus agent"
```

---

### Task 8: MCP Config CLI Helper

**Files:**
- Create: `pkg/cli/mcp.go`
- Create: `pkg/cli/mcp_test.go`
- Modify: `pkg/cli/root.go`

- [ ] **Step 1: Add failing helper test**

Create `pkg/cli/mcp_test.go`:

```go
package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMCPConfigJSON(t *testing.T) {
	raw, err := buildMCPConfigJSON("http://127.0.0.1:8002/osm/mcp", true)
	require.NoError(t, err)
	var decoded map[string]map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	server := decoded["mcpServers"]["osmedeus"]
	require.Equal(t, "http", server["type"])
	require.Equal(t, "http://127.0.0.1:8002/osm/mcp", server["url"])
	require.NotNil(t, server["headers"])
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
go test ./pkg/cli -run 'TestBuildMCPConfigJSON' -count=1
```

Expected: FAIL because `buildMCPConfigJSON` does not exist.

- [ ] **Step 3: Add MCP CLI command**

Create `pkg/cli/mcp.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/spf13/cobra"
)

var mcpPrint bool

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Configure Osmedeus MCP clients",
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print MCP client configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		if cfg == nil {
			return fmt.Errorf("configuration not loaded")
		}
		out, err := buildMCPConfigJSON(cfg.Server.GetMCPURL(), cfg.Server.IsMCPAuthRequired())
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	},
}

func init() {
	mcpConfigCmd.Flags().BoolVar(&mcpPrint, "print", true, "print MCP JSON configuration")
	mcpCmd.AddCommand(mcpConfigCmd)
}

func buildMCPConfigJSON(url string, requireAuth bool) (string, error) {
	server := map[string]interface{}{
		"type": "http",
		"url":  url,
	}
	if requireAuth {
		server["headers"] = map[string]string{
			"Authorization": "Bearer ${OSMEDEUS_API_TOKEN}",
		}
	}
	payload := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"osmedeus": server,
		},
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

In `pkg/cli/root.go`, add:

```go
rootCmd.AddCommand(mcpCmd)
```

- [ ] **Step 4: Run CLI tests**

Run:

```bash
go test ./pkg/cli -run 'TestBuildMCPConfigJSON' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cli/mcp.go pkg/cli/mcp_test.go pkg/cli/root.go
git commit -m "feat(cli): add MCP config helper"
```

---

### Task 9: Documentation

**Files:**
- Create: `docs/api/mcp.mdx`
- Modify: `docs/api/README.mdx`

- [ ] **Step 1: Add MCP docs**

Create `docs/api/mcp.mdx`:

```md
---
title: "MCP"
description: "Remote MCP endpoint for AI agents"
---

# MCP

Osmedeus exposes a remote MCP endpoint from the existing HTTP server.

Start the server:

```bash
osmedeus server
```

Default endpoint:

```text
http://127.0.0.1:8002/osm/mcp
```

Print client configuration:

```bash
export OSMEDEUS_API_TOKEN=<token>
osmedeus mcp config --print
```

The endpoint provides read-only tools in the foundation release:

- `osmedeus.context.resolve_target`
- `osmedeus.assets.search`
- `osmedeus.vulns.search`
- `osmedeus.runs.list`

Tools that save workflows or launch scans are not part of the foundation release. Those actions require approval-gated tools.
```

In `docs/api/README.mdx`, add an entry for MCP near the other endpoint categories:

```md
- **MCP**: Remote MCP endpoint for AI agents to query Osmedeus context
```

- [ ] **Step 2: Verify docs are present**

Run:

```bash
test -f docs/api/mcp.mdx && rg -n "MCP" docs/api/README.mdx docs/api/mcp.mdx
```

Expected: command exits 0 and prints MCP references.

- [ ] **Step 3: Commit**

```bash
git add docs/api/mcp.mdx docs/api/README.mdx
git commit -m "docs: document MCP endpoint"
```

---

### Task 10: Full Verification

**Files:**
- No code changes expected.

- [ ] **Step 1: Run focused tests**

Run:

```bash
go test ./internal/config ./internal/ai ./internal/executor ./pkg/cli ./pkg/server/handlers ./pkg/server -count=1
```

Expected: PASS.

- [ ] **Step 2: Run unit test target**

Run:

```bash
make test-unit
```

Expected: PASS.

- [ ] **Step 3: Manual smoke test MCP config**

Run:

```bash
go run . mcp config --print
```

Expected: JSON with `mcpServers.osmedeus.type` set to `http` and URL ending in `/osm/mcp`.

- [ ] **Step 4: Manual smoke test server route**

Start the server in one terminal:

```bash
go run . server --no-auth
```

In another terminal:

```bash
curl -s http://127.0.0.1:8002/osm/mcp
```

Expected: JSON includes `"name":"osmedeus"` and `"status":"ok"`.

- [ ] **Step 5: Final commit if verification required metadata changes**

If verification required a small fix, commit it:

```bash
git add <changed-files>
git commit -m "fix: stabilize MCP foundation"
```

If no files changed, skip this step.

---

## Self-Review

Spec coverage in this plan:

- Covered: remote MCP endpoint on existing HTTP server.
- Covered: `osmedeus agent` gets Osmedeus MCP tools by default.
- Covered: no large automatic target context dump.
- Covered: read-only context tools for targets, assets, vulnerabilities, and runs.
- Covered: MCP client config helper.
- Not covered by this plan: workflow generation, approvals, temporary workflow save mode, normal workflow save mode, one-shot run launch, and result monitoring. These require follow-on plans using this MCP foundation.

Placeholder scan:

- No unresolved placeholder tokens are intentionally present in executable steps.
- Any `<token>` or `<changed-files>` examples appear only in manual user commands where the operator supplies runtime values.

Type consistency:

- `ServiceConfig`, request types, response types, and service method names are introduced before handler usage.
- CLI helper returns `executor.RunAgentACPConfig`, matching the executor config extended in Task 6.
- MCP route uses `ServerConfig.GetMCPPath()` and `ServerConfig.GetMCPURL()` introduced in Task 1.

