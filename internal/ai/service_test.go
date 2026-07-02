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
