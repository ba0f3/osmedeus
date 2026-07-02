package ai

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/database"
)

const defaultArtifactReadLimit = 65536

type ListArtifactsRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Search    string `json:"search,omitempty"`
	RunUUID   string `json:"run_uuid,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type ListArtifactsResponse struct {
	Total   int                 `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
	Records []database.Artifact `json:"records"`
}

func (s *Service) ListArtifacts(ctx context.Context, req ListArtifactsRequest) (*ListArtifactsResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	limit := s.clampLimit(req.Limit)
	result, err := database.ListArtifacts(ctx, database.ArtifactQuery{
		Workspace: req.Workspace,
		Search:    req.Search,
		Offset:    req.Offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	records := result.Data
	if req.RunUUID != "" {
		filtered := make([]database.Artifact, 0)
		for _, a := range records {
			if strconv.FormatInt(a.RunID, 10) == req.RunUUID || strings.Contains(a.Name, req.RunUUID) {
				filtered = append(filtered, a)
			}
		}
		records = filtered
	}
	return &ListArtifactsResponse{
		Total:   result.TotalCount,
		Limit:   limit,
		Offset:  req.Offset,
		Records: records,
	}, nil
}

type ReadArtifactRequest struct {
	Workspace    string `json:"workspace"`
	ArtifactPath string `json:"artifact_path"`
	MaxBytes     int    `json:"max_bytes,omitempty"`
}

type ReadArtifactResponse struct {
	Workspace    string `json:"workspace"`
	ArtifactPath string `json:"artifact_path"`
	SizeBytes    int64  `json:"size_bytes"`
	Truncated    bool   `json:"truncated"`
	Content      string `json:"content"`
	ContentType  string `json:"content_type,omitempty"`
}

func (s *Service) ReadArtifact(ctx context.Context, req ReadArtifactRequest) (*ReadArtifactResponse, error) {
	if err := s.requireConfig(); err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(req.Workspace)
	if !isValidWorkspaceName(workspace) {
		return nil, fmt.Errorf("invalid workspace name")
	}
	cleanRel, ok := sanitizeArtifactRelPath(req.ArtifactPath)
	if !ok {
		return nil, fmt.Errorf("invalid artifact_path")
	}

	workspaceDir, err := s.resolveWorkspaceDir(ctx, workspace)
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(workspaceDir, cleanRel)
	if !isPathUnderBase(fullPath, workspaceDir) {
		return nil, fmt.Errorf("path traversal attempt detected")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("artifact not found")
	}
	if info.IsDir() {
		return nil, fmt.Errorf("artifact_path must point to a file")
	}

	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultArtifactReadLimit
	}
	truncated := info.Size() > int64(maxBytes)
	readSize := int64(maxBytes)
	if !truncated {
		readSize = info.Size()
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact")
	}
	defer f.Close()

	limited := io.LimitReader(f, readSize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact")
	}

	return &ReadArtifactResponse{
		Workspace:    workspace,
		ArtifactPath: cleanRel,
		SizeBytes:    info.Size(),
		Truncated:    truncated,
		Content:      string(data),
		ContentType:  detectContentType(cleanRel),
	}, nil
}

func (s *Service) resolveWorkspaceDir(ctx context.Context, workspace string) (string, error) {
	ws, err := database.GetWorkspaceByName(ctx, workspace)
	if err == nil && ws != nil && ws.LocalPath != "" {
		return ws.LocalPath, nil
	}
	return filepath.Join(s.cfg.GetWorkspacesDir(), workspace), nil
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json", ".jsonl":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".md":
		return "text/markdown"
	case ".txt", ".log":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
