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
	headers, ok := server["headers"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "${OSM_API_KEY}", headers["x-osm-api-key"])
}
