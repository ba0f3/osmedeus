package ai

import (
	"path/filepath"
	"strings"
)

func isPathUnderBase(fullPath, basePath string) bool {
	absFull, err1 := filepath.Abs(fullPath)
	absBase, err2 := filepath.Abs(basePath)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absFull)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func isValidWorkspaceName(name string) bool {
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return false
	}
	return true
}

func sanitizeArtifactRelPath(artifactPath string) (string, bool) {
	cleanRel := filepath.Clean(artifactPath)
	if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", false
	}
	return cleanRel, true
}
