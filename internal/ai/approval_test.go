package ai

import (
	"testing"
	"time"

	"github.com/j3ssie/osmedeus/v5/internal/database"
	"github.com/stretchr/testify/require"
)

func TestIsKnownApprovalAction(t *testing.T) {
	require.True(t, isKnownApprovalAction(database.AIActionStartRun))
	require.False(t, isKnownApprovalAction("unknown"))
}

func TestSanitizeSlug(t *testing.T) {
	require.Equal(t, "domain-recon", sanitizeSlug("Domain Recon", "fallback"))
	require.Equal(t, "fallback", sanitizeSlug("!!!", "fallback"))
}

func TestSanitizeArtifactRelPathRejectsTraversal(t *testing.T) {
	_, ok := sanitizeArtifactRelPath("../secret.txt")
	require.False(t, ok)
	rel, ok := sanitizeArtifactRelPath("reports/summary.md")
	require.True(t, ok)
	require.Equal(t, "reports/summary.md", rel)
}

func TestExtractYAMLFromLLMOutputStripsFence(t *testing.T) {
	input := "```yaml\nname: test\nkind: module\n```"
	require.Contains(t, extractYAMLFromLLMOutput(input), "name: test")
}

func TestSuggestedWorkflowName(t *testing.T) {
	require.Equal(t, "ai-vuln-scan-domain", suggestedWorkflowName("vuln scan", "domain"))
}

func TestDefaultApprovalTTLPositive(t *testing.T) {
	require.Greater(t, defaultApprovalTTL, time.Minute)
}
