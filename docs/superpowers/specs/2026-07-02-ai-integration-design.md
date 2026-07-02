# Osmedeus AI Integration Design

Date: 2026-07-02

## Purpose

Osmedeus already stores valuable scan context in its database and workspace artifacts, but the current `osmedeus agent` path mostly delegates a raw prompt to an ACP agent with only a working directory. The result is that agents perform fresh recon instead of using prior Osmedeus runs, assets, vulnerabilities, artifacts, and workflows.

This design adds a shared AI integration layer and exposes it through the CLI agent, existing HTTP server, and MCP tools. The goal is to let AI agents answer questions, generate workflows, and plan scans using Osmedeus context while requiring user approval before writes or scan execution.

## Goals

- Let `osmedeus agent "tell me what you know about example.com"` query Osmedeus context through tools instead of guessing from the prompt.
- Add remote MCP tools on the existing HTTP server so external AI agents can work with Osmedeus.
- Support workflow generation with validation and user choice between reusable and temporary storage.
- Support one-shot vuln scan planning with explicit approval before saving workflows or launching runs.
- Reuse one implementation for CLI, API, and MCP surfaces.

## Non-Goals

- Do not add a separate stdio MCP daemon as the primary architecture.
- Do not inject large automatic database context into every agent prompt.
- Do not let agents write workflows, overwrite files, delete data, or launch scans without approval.
- Do not replace the existing workflow engine, parser, run creation path, or database repositories.

## Recommended Architecture

Add an `internal/ai` package as the shared service layer.

### Services

`ContextService`

Resolves targets and workspaces, then provides concise context from existing Osmedeus data:

- `runs`
- `assets`
- `vulnerabilities`
- `artifacts`
- `step_results`
- `workflow_meta`
- `agent_sessions`

The service should expose structured methods used by MCP tools. It should not prebuild a large context packet for every prompt.

`ToolService`

Implements the backend operations exposed as MCP tools. CLI, HTTP API handlers, and MCP handlers should call this service instead of duplicating query logic.

`PlannerService`

Turns a user request into an action plan. It can recommend an existing workflow, propose generation, request approval, or summarize available context.

`WorkflowGenerator`

Generates workflow YAML, validates it with the existing parser/validator, and saves it only after approval. It supports both normal reusable workflows and temporary per-run workflows.

`ApprovalService`

Tracks pending, approved, rejected, and expired AI actions. Writes and scan execution must go through this service.

`SummaryService`

Summarizes completed runs from database and artifact data, and records useful agent output in `agent_sessions`.

## Agent Context Model

`osmedeus agent` should provide Osmedeus MCP tools to the selected ACP agent. The LLM/agent decides which tools to call based on the user request.

The prompt should include only a small bootstrap instruction, for example:

```text
You are running inside Osmedeus. Use the osmedeus MCP tools to inspect prior runs,
assets, vulnerabilities, workflows, artifacts, and to request approval before
writing workflows or launching scans.
```

The default must not be a guessed target context dump.

### CLI Behavior

Default:

```bash
osmedeus agent "tell me what you know about example.com"
```

Expected behavior:

1. The CLI configures the ACP session with the Osmedeus remote MCP server.
2. The agent receives the user prompt and small bootstrap instruction.
3. The agent calls tools such as target resolution, asset search, vulnerability search, and run listing.
4. The final answer is based on tool results.

Useful flags:

```bash
osmedeus agent "..." --no-mcp
osmedeus agent "..." --mcp-url http://127.0.0.1:8002/osm/mcp
osmedeus agent "..." --require-approval
```

`--no-mcp` runs the raw ACP agent and should be explicit.

If MCP is unavailable, the CLI should report the missing endpoint and suggest starting `osmedeus server`. It should not silently fall back to ungrounded behavior unless the user passes `--no-mcp`.

If a selected ACP agent cannot accept MCP server configuration, the CLI should report that limitation and require `--no-mcp` to continue without tools.

## MCP Server

Expose MCP through the existing HTTP server.

```bash
osmedeus server
```

The server exposes a remote MCP endpoint such as:

```text
http://127.0.0.1:8002/osm/mcp
```

Authentication should use existing server auth unless auth is disabled for the server.

### Configuration

Add server config:

```yaml
server:
  mcp:
    enabled: true
    path: /osm/mcp
    require_auth: true
```

Add helper commands for client configuration:

```bash
osmedeus mcp config --print
osmedeus mcp config --client claude
osmedeus mcp config --client codex
```

Example output:

```json
{
  "mcpServers": {
    "osmedeus": {
      "type": "http",
      "url": "http://127.0.0.1:8002/osm/mcp",
      "headers": {
        "Authorization": "Bearer ${OSMEDEUS_API_TOKEN}"
      }
    }
  }
}
```

## MCP Tools

Initial tools:

- `osmedeus.context.resolve_target`
- `osmedeus.context.summary`
- `osmedeus.assets.search`
- `osmedeus.vulns.search`
- `osmedeus.runs.list`
- `osmedeus.runs.get`
- `osmedeus.artifacts.list`
- `osmedeus.artifacts.read`
- `osmedeus.workflows.search`
- `osmedeus.workflows.get`
- `osmedeus.workflows.generate`
- `osmedeus.workflows.validate`
- `osmedeus.workflows.promote_temp`
- `osmedeus.approvals.request`
- `osmedeus.approvals.get`
- `osmedeus.approvals.approve`
- `osmedeus.runs.plan`
- `osmedeus.runs.start_approved`

Tool outputs must cap large fields such as raw HTTP responses, screenshots, blob content, and large artifacts. Artifact reading should require explicit path and size limits.

## Workflow Generation

Workflow generation is an approval-gated tool flow.

Flow:

1. Search existing workflows by tags, descriptions, params, and target type.
2. Generate a workflow plan.
3. Generate YAML using Osmedeus workflow schema and existing module patterns.
4. Validate YAML with the existing parser/validator.
5. Show a preview or diff to the user.
6. Ask the user where to save the workflow:

```text
Save this generated workflow as:
1. Normal reusable workflow in the workflow folder
2. Temporary workflow for this run only
3. Do not save
```

Storage behavior:

- Normal reusable workflow: save to the configured workflow folder, index it, and show it in `osmedeus workflow list`.
- Temporary workflow: save under the workspace or run area, use it only for the approved run, and do not index it globally.
- Do not save: return the generated YAML or plan only.

If a normal workflow name already exists, require explicit overwrite approval or suggest a suffixed name.

Suggested generated workflow naming:

```text
ai-<purpose>-<target-type>.yaml
```

## One-Shot Vuln Scan

For a request such as:

```bash
osmedeus agent "scan example.com for likely vulns"
```

Expected tool flow:

1. Resolve target and workspace.
2. Inspect prior runs, assets, vulnerabilities, workflows, and relevant artifacts.
3. Search existing workflows first.
4. If an existing workflow fits, propose it.
5. If no workflow fits, generate and validate a workflow.
6. Ask whether to save generated workflow as normal, temporary, or not at all.
7. Request approval before launching any scan.
8. Start the approved run using the existing run creation path.
9. Monitor run status.
10. Summarize findings from database records, step results, and artifacts.

Default recommendations:

- Prefer an existing workflow when it fits.
- Suggest temporary storage for workflows generated only for a specific one-shot target.
- Suggest normal reusable storage for workflows that are generally useful.

## Approval Boundaries

No approval required:

- Reading database context.
- Listing workflows.
- Reading bounded artifacts.
- Generating plans and previews.
- Validating generated workflow YAML.

Approval required:

- Saving generated workflows.
- Launching scans or queued runs.
- Promoting a temporary workflow to a normal workflow.

Explicit stronger approval required:

- Overwriting an existing workflow.
- Running destructive operations.
- Deleting data.

## Data Model

Add `ai_approvals`:

- ID
- action type
- status: pending, approved, rejected, expired, executed
- requested payload JSON
- result payload JSON
- created at
- approved/rejected/executed timestamps
- requester/source metadata

Add `ai_generated_workflows`:

- ID
- prompt
- purpose
- generated YAML
- validation status
- validation errors
- save mode: normal, temporary, none
- workflow path
- run UUID for temporary workflows
- approval ID
- created at

Extend or reuse `agent_sessions`:

- prompt
- final output
- tool calls JSON
- selected MCP URL
- approval IDs
- run ID or run UUID when applicable

## Error Handling

- MCP unavailable: report the endpoint problem and suggest `osmedeus server`.
- Agent lacks MCP support: fail with a clear message unless `--no-mcp` is passed.
- Workflow validation failed: return structured validation errors and do not offer save/run approval.
- Approval rejected or expired: do not execute the pending action.
- Run failed: summarize failed steps, errors, and relevant artifacts.
- Large result requested: truncate safely and include a continuation option where practical.

## Testing

Unit tests:

- `ContextService` target/workspace resolution.
- `ToolService` query behavior and output caps.
- Approval state transitions.
- Workflow save modes: normal, temporary, none.
- Workflow validation failure handling.

Handler tests:

- MCP tool listing and tool calls.
- MCP auth behavior.
- Approval-required responses for write/run tools.

CLI tests:

- `osmedeus agent --mcp-url`.
- `osmedeus agent --no-mcp`.
- MCP unavailable behavior.
- Selected ACP agent without MCP support.
- Approval prompts.

Integration-style tests:

- Generate, validate, and save temporary workflow.
- Generate, validate, and save normal workflow.
- Plan one-shot scan with existing workflow.
- Plan one-shot scan with generated workflow and approval.

## Implementation Order

1. Add `internal/ai` service interfaces and read-only context/tool operations.
2. Add HTTP MCP endpoint and read-only tools.
3. Wire `osmedeus agent` to configure ACP sessions with remote MCP tools by default.
4. Add approval service and approval data model.
5. Add workflow generation, validation, and save modes.
6. Add one-shot scan planning and approved run launch.
7. Add summary and session persistence improvements.

