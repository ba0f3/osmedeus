package cli

import (
	"encoding/json"
	"fmt"

	"github.com/j3ssie/osmedeus/v5/internal/config"
	"github.com/spf13/cobra"
)

var mcpPrint bool

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Configure Osmedeus MCP clients",
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print MCP client configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		if cfg == nil {
			return fmt.Errorf("configuration not loaded")
		}
		out, err := buildMCPConfigJSON(cfg.Server.GetMCPURL(), cfg.Server.IsMCPAuthRequired())
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	},
}

func init() {
	mcpConfigCmd.Flags().BoolVar(&mcpPrint, "print", true, "print MCP JSON configuration")
	mcpCmd.AddCommand(mcpConfigCmd)
}

func buildMCPConfigJSON(url string, requireAuth bool) (string, error) {
	server := map[string]interface{}{
		"type": "http",
		"url":  url,
	}
	if requireAuth {
		server["headers"] = map[string]string{
			"Authorization": "Bearer ${OSMEDEUS_API_TOKEN}",
		}
	}
	payload := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"osmedeus": server,
		},
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
