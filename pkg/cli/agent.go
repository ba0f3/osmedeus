package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/j3ssie/osmedeus/v5/internal/core"
	"github.com/j3ssie/osmedeus/v5/internal/executor"
	"github.com/j3ssie/osmedeus/v5/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	agentName                string
	agentModel               string
	agentCwd                 string
	agentStdin               bool
	agentTimeout             string
	agentList                bool
	agentNoMCP               bool
	agentMCPURL              string
	agentMCPAllowRemoteToken bool
)

// agentCmd runs an ACP agent interactively from the terminal.
var agentCmd = &cobra.Command{
	Use:   "agent [message]",
	Short: "Run an ACP agent interactively",
	Long:  UsageAgent(),
	RunE:  runAgent,
}

func init() {
	agentCmd.Flags().StringVar(&agentName, "agent", core.DefaultACPAgent, "agent to use (see --list for available agents)")
	agentCmd.Flags().StringVar(&agentModel, "model", "", "LLM model to use instead of the agent default")
	agentCmd.Flags().StringVar(&agentCwd, "cwd", "", "working directory for the agent (default: current directory)")
	agentCmd.Flags().BoolVar(&agentStdin, "stdin", false, "read message from stdin")
	agentCmd.Flags().StringVar(&agentTimeout, "timeout", "30m", "timeout duration (e.g., 30m, 1h)")
	agentCmd.Flags().BoolVar(&agentList, "list", false, "list available agents")
	agentCmd.Flags().BoolVar(&agentNoMCP, "no-mcp", false, "run without Osmedeus MCP tools")
	agentCmd.Flags().StringVar(&agentMCPURL, "mcp-url", "", "Osmedeus MCP URL (default: configured server URL + /osm/mcp)")
	agentCmd.Flags().BoolVar(&agentMCPAllowRemoteToken, "mcp-allow-remote-token", false, "allow sending OSMEDEUS_API_TOKEN to a remote --mcp-url host")
}

func runAgent(cmd *cobra.Command, args []string) error {
	printer := terminal.NewPrinter()

	// List agents
	if agentList {
		names := executor.ListAgentNames()
		sort.Strings(names)
		printer.Section("Available ACP Agents")
		fmt.Println()
		for _, name := range names {
			fmt.Printf("  %s %s\n", terminal.SymbolBullet, terminal.Cyan(name))
		}
		fmt.Println()
		return nil
	}

	// Resolve message
	message, err := resolveAgentMessage(args)
	if err != nil {
		printer.Error("%s", err)
		return err
	}

	// Parse timeout
	timeout, err := parseRunDuration(agentTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	appCfg := config.Get()
	mcpURL := agentMCPURL
	trustedMCPURL := ""
	if appCfg != nil {
		trustedMCPURL = appCfg.Server.GetMCPURL()
		if mcpURL == "" {
			mcpURL = trustedMCPURL
		}
	}
	token := os.Getenv("OSMEDEUS_API_TOKEN")
	mcpCfg := resolveAgentMCPConfig(mcpURL, agentNoMCP, token, trustedMCPURL, agentMCPAllowRemoteToken)
	if !agentNoMCP && token != "" && mcpURL != "" && mcpCfg.MCPToken == "" {
		printer.Warning("not sending OSMEDEUS_API_TOKEN to remote MCP URL %s; use --mcp-allow-remote-token to override", mcpURL)
	}

	// Build config
	cfg := &executor.RunAgentACPConfig{
		Cwd:          agentCwd,
		Model:        agentModel,
		StreamWriter: os.Stdout,
		MCPURL:       mcpCfg.MCPURL,
		MCPToken:     mcpCfg.MCPToken,
		MCPName:      mcpCfg.MCPName,
	}

	_, _, err = executor.RunAgentACP(ctx, message, agentName, cfg)
	if err != nil {
		printer.Error("Agent failed: %s", err)
		return err
	}

	return nil
}

// resolveAgentMessage determines the message from positional args, --stdin, or piped stdin.
func resolveAgentMessage(args []string) (string, error) {
	// Positional argument (not "-")
	if len(args) > 0 && args[0] != "-" {
		return strings.Join(args, " "), nil
	}

	// --stdin flag or "-" argument
	if agentStdin || (len(args) > 0 && args[0] == "-") {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			return "", fmt.Errorf("empty message from stdin")
		}
		return msg, nil
	}

	return "", fmt.Errorf("no message provided: use positional argument, --stdin, or pipe with -")
}

func resolveAgentMCPConfig(mcpURL string, disabled bool, token string, trustedMCPURL string, allowRemoteToken bool) executor.RunAgentACPConfig {
	if disabled {
		return executor.RunAgentACPConfig{}
	}
	cfg := executor.RunAgentACPConfig{
		MCPURL:  mcpURL,
		MCPName: "osmedeus",
	}
	if token != "" && (allowRemoteToken || mcpURLTrustsToken(mcpURL, trustedMCPURL)) {
		cfg.MCPToken = token
	}
	return cfg
}

func mcpURLTrustsToken(mcpURL, trustedMCPURL string) bool {
	if mcpURL == "" || trustedMCPURL == "" {
		return false
	}
	mcpHost, err := urlHost(mcpURL)
	if err != nil {
		return false
	}
	trustedHost, err := urlHost(trustedMCPURL)
	if err != nil {
		return false
	}
	return hostsEquivalent(mcpHost, trustedHost)
}

func urlHost(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in URL")
	}
	return parsed.Hostname(), nil
}

func hostsEquivalent(a, b string) bool {
	return normalizeHost(a) == normalizeHost(b)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return "localhost"
	default:
		return host
	}
}
