package handlers

import (
	"testing"

	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSubmitLocalRunRequiresWorkflow(t *testing.T) {
	_, err := SubmitLocalRun(&config.Config{}, CreateRunRequest{Target: "example.com"})
	require.Error(t, err)
	var clientErr *mcpClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, -32602, clientErr.code)
	require.Contains(t, clientErr.message, "flow")
}

func TestSubmitLocalRunRequiresTarget(t *testing.T) {
	_, err := SubmitLocalRun(&config.Config{}, CreateRunRequest{Module: "basic-recon"})
	require.Error(t, err)
	var clientErr *mcpClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, -32602, clientErr.code)
	require.Contains(t, clientErr.message, "target")
}

func TestSubmitLocalRunRejectsDistributedMode(t *testing.T) {
	_, err := SubmitLocalRun(&config.Config{}, CreateRunRequest{
		Module:  "basic-recon",
		Target:  "example.com",
		RunMode: "distributed",
	})
	require.Error(t, err)
	var clientErr *mcpClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, -32602, clientErr.code)
}

func TestCreateRunRequestFromMCPArgs(t *testing.T) {
	req := createRunRequestFromMCPArgs(map[string]interface{}{
		"flow":     "subdomain-enum",
		"target":   "example.com",
		"priority": "high",
		"params": map[string]interface{}{
			"threads": "10",
		},
	})
	require.Equal(t, "subdomain-enum", req.Flow)
	require.Equal(t, "example.com", req.Target)
	require.Equal(t, "high", req.Priority)
	require.Equal(t, "10", req.Params["threads"])
}
