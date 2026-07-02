package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/j3ssie/osmedeus/v5/pkg/server/middleware"
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

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded mcpResponse
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	result, ok := decoded.Result.(map[string]interface{})
	require.True(t, ok)
	tools, ok := result["tools"].([]interface{})
	require.True(t, ok)
	require.Len(t, tools, 19)
	first, ok := tools[0].(map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, first["inputSchema"])
	schema, ok := first["inputSchema"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "object", schema["type"])
}

func TestMCPInitializedNotificationReturnsNoContent(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded mcpResponse
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	require.NotNil(t, decoded.Error)
	require.Equal(t, -32601, decoded.Error.Code)
	require.Equal(t, "Method not found", decoded.Error.Message)
}

func TestMCPInternalErrorDoesNotLeakDetails(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"osmedeus.runs.list","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded mcpResponse
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	require.NotNil(t, decoded.Error)
	require.Equal(t, -32603, decoded.Error.Code)
	require.Equal(t, mcpInternalErrorMsg, decoded.Error.Message)
	require.NotContains(t, decoded.Error.Message, "database")
}

func TestMCPRequiresAuthWhenEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 8002
	cfg.Server.JWT.SecretSigningKey = "test-secret"
	cfg.Server.EnabledAuthAPI = true
	cfg.Server.AuthAPIKey = "secret-key"

	app := fiber.New()
	app.Post("/osm/mcp", middleware.CombinedAuth(cfg), MCP(cfg))
	body := `{"jsonrpc":"2.0","id":5,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestMCPHandlerAllowsGetHealth(t *testing.T) {
	app := fiber.New()
	app.Get("/osm/mcp", MCPHealth())
	req := httptest.NewRequest(http.MethodGet, "/osm/mcp", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPRunsStartRequiresTarget(t *testing.T) {
	app := fiber.New()
	app.Post("/osm/mcp", MCP(&config.Config{}))
	body := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"osmedeus.runs.start","arguments":{"module":"basic-recon"}}}`
	req := httptest.NewRequest(http.MethodPost, "/osm/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded mcpResponse
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	require.NotNil(t, decoded.Error)
	require.Equal(t, -32602, decoded.Error.Code)
	require.Contains(t, decoded.Error.Message, "target")
}
