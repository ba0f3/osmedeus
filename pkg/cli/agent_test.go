package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveAgentMessage_Positional(t *testing.T) {
	msg, err := resolveAgentMessage([]string{"hello", "world"})
	assert.NoError(t, err)
	assert.Equal(t, "hello world", msg)
}

func TestResolveAgentMessage_SingleArg(t *testing.T) {
	msg, err := resolveAgentMessage([]string{"test message"})
	assert.NoError(t, err)
	assert.Equal(t, "test message", msg)
}

func TestResolveAgentMessage_NoArgs(t *testing.T) {
	_, err := resolveAgentMessage(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no message provided")
}

func TestResolveAgentMCPConfigDisabled(t *testing.T) {
	cfg := resolveAgentMCPConfig("http://127.0.0.1:8002/osm/mcp", true, "token")
	assert.Empty(t, cfg.MCPURL)
	assert.Empty(t, cfg.MCPToken)
}

func TestResolveAgentMCPConfigEnabled(t *testing.T) {
	cfg := resolveAgentMCPConfig("http://127.0.0.1:8002/osm/mcp", false, "token")
	assert.Equal(t, "http://127.0.0.1:8002/osm/mcp", cfg.MCPURL)
	assert.Equal(t, "token", cfg.MCPToken)
	assert.Equal(t, "osmedeus", cfg.MCPName)
}

func TestResolveAgentMessage_EmptyArgs(t *testing.T) {
	_, err := resolveAgentMessage([]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no message provided")
}
