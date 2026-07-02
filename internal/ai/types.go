package ai

import "github.com/j3ssie/osmedeus/v5/internal/database"

type ServiceConfig struct {
	MaxLimit int
}

type ResolveTargetRequest struct {
	Target string `json:"target"`
}

type WorkspaceSummary struct {
	Name        string  `json:"name"`
	TotalAssets int     `json:"total_assets"`
	TotalVulns  int     `json:"total_vulns"`
	RiskScore   float64 `json:"risk_score"`
	LastRun     string  `json:"last_run,omitempty"`
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

type SearchVulnerabilitiesResponse struct {
	Total   int                      `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
	Records []database.Vulnerability `json:"records"`
}

type ListRunsRequest struct {
	Target    string `json:"target,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type ListRunsResponse struct {
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	Records []database.Run `json:"records"`
}
