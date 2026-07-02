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

func TestMCPHandlerAllowsGetHealth(t *testing.T) {
	app := fiber.New()
	app.Get("/osm/mcp", MCPHealth())
	req := httptest.NewRequest(http.MethodGet, "/osm/mcp", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
