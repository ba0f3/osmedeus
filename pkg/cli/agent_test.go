package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPURLTrustsTokenRequiresMatchingPort(t *testing.T) {
	trusted := "http://127.0.0.1:8002/osm/mcp"
	require.True(t, mcpURLTrustsToken("http://127.0.0.1:8002/osm/mcp", trusted))
	require.True(t, mcpURLTrustsToken("http://localhost:8002/osm/mcp", trusted))
	require.False(t, mcpURLTrustsToken("http://127.0.0.1:9999/osm/mcp", trusted))
}

func TestMCPURLTrustsTokenRequiresMatchingScheme(t *testing.T) {
	trusted := "http://127.0.0.1:8002/osm/mcp"
	require.False(t, mcpURLTrustsToken("https://127.0.0.1:8002/osm/mcp", trusted))
}

func TestResolveAgentMCPConfigOmitsTokenForUntrustedPort(t *testing.T) {
	cfg := resolveAgentMCPConfig(
		"http://127.0.0.1:9999/osm/mcp",
		false,
		"secret-token",
		"http://127.0.0.1:8002/osm/mcp",
		false,
	)
	require.Empty(t, cfg.MCPToken)
}

func TestResolveAgentMCPConfigSendsTokenForTrustedURL(t *testing.T) {
	cfg := resolveAgentMCPConfig(
		"http://127.0.0.1:8002/osm/mcp",
		false,
		"secret-token",
		"http://127.0.0.1:8002/osm/mcp",
		false,
	)
	require.Equal(t, "secret-token", cfg.MCPToken)
}
