package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/j3ssie/osmedeus/v5/internal/core"
	"github.com/j3ssie/osmedeus/v5/internal/database"
	"github.com/j3ssie/osmedeus/v5/internal/parser"
)

const (
	workflowSaveModeNone       = "none"
	workflowSaveModeNormal     = "normal"
	workflowSaveModeTemporary  = "temporary"
	validationStatusValid      = "valid"
	validationStatusInvalid      = "invalid"
	validationStatusNotValidated = "not_validated"
)

type SearchWorkflowsRequest struct {
	Search string   `json:"search,omitempty"`
	Kind   string   `json:"kind,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	Offset int      `json:"offset,omitempty"`
}

type SearchWorkflowsResponse struct {
	Total   int                    `json:"total"`
	Limit   int                    `json:"limit"`
	Offset  int                    `json:"offset"`
	Records []database.WorkflowMeta `json:"records"`
}

func (s *Service) SearchWorkflows(ctx context.Context, req SearchWorkflowsRequest) (*SearchWorkflowsResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	limit := s.clampLimit(req.Limit)
	result, err := database.ListWorkflowsFromDB(ctx, database.WorkflowQuery{
		Tags:   req.Tags,
		Kind:   req.Kind,
		Search: req.Search,
		Offset: req.Offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	return &SearchWorkflowsResponse{
		Total:   result.TotalCount,
		Limit:   limit,
		Offset:  req.Offset,
		Records: result.Data,
	}, nil
}

type GetWorkflowRequest struct {
	Name string `json:"name"`
}

type GetWorkflowResponse struct {
	Meta     *database.WorkflowMeta `json:"meta,omitempty"`
	Workflow *core.Workflow         `json:"workflow,omitempty"`
}

func (s *Service) GetWorkflow(ctx context.Context, req GetWorkflowRequest) (*GetWorkflowResponse, error) {
	if err := s.requireConfig(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	resp := &GetWorkflowResponse{}
	if s.db != nil {
		meta, err := database.GetWorkflowFromDB(ctx, name)
		if err == nil {
			resp.Meta = meta
		}
	}

	loader := parser.NewLoader(s.cfg.WorkflowsPath)
	workflow, err := loader.LoadWorkflow(name)
	if err != nil {
		if resp.Meta == nil {
			return nil, fmt.Errorf("workflow not found")
		}
		return resp, nil
	}
	resp.Workflow = workflow
	return resp, nil
}

type ValidateWorkflowRequest struct {
	YAML                 string `json:"yaml,omitempty"`
	GeneratedWorkflowID  int64  `json:"generated_workflow_id,omitempty"`
}

type ValidateWorkflowResponse struct {
	Valid            bool     `json:"valid"`
	ValidationStatus string   `json:"validation_status"`
	Errors           []string `json:"errors,omitempty"`
	WorkflowName     string   `json:"workflow_name,omitempty"`
}

func (s *Service) ValidateWorkflowYAML(ctx context.Context, req ValidateWorkflowRequest) (*ValidateWorkflowResponse, error) {
	yamlContent := strings.TrimSpace(req.YAML)
	var record *database.AIGeneratedWorkflow
	if yamlContent == "" && req.GeneratedWorkflowID > 0 {
		if err := s.requireDB(); err != nil {
			return nil, err
		}
		var err error
		record, err = database.GetAIGeneratedWorkflow(ctx, req.GeneratedWorkflowID)
		if err != nil {
			return nil, fmt.Errorf("generated workflow not found")
		}
		yamlContent = record.GeneratedYAML
	}
	if yamlContent == "" {
		return nil, fmt.Errorf("yaml or generated_workflow_id is required")
	}

	workflow, parseErr := parser.ParseContent([]byte(yamlContent))
	resp := &ValidateWorkflowResponse{
		ValidationStatus: validationStatusInvalid,
	}
	if parseErr != nil {
		resp.Errors = []string{parseErr.Error()}
		if record != nil {
			record.ValidationStatus = validationStatusInvalid
			record.ValidationErrors = parseErr.Error()
			_ = database.UpdateAIGeneratedWorkflow(ctx, record)
		}
		return resp, nil
	}
	if validateErr := parser.Validate(workflow); validateErr != nil {
		resp.Errors = []string{validateErr.Error()}
		if record != nil {
			record.ValidationStatus = validationStatusInvalid
			record.ValidationErrors = validateErr.Error()
			_ = database.UpdateAIGeneratedWorkflow(ctx, record)
		}
		return resp, nil
	}

	resp.Valid = true
	resp.ValidationStatus = validationStatusValid
	resp.WorkflowName = workflow.Name
	if record != nil {
		record.ValidationStatus = validationStatusValid
		record.ValidationErrors = ""
		_ = database.UpdateAIGeneratedWorkflow(ctx, record)
	}
	return resp, nil
}

type GenerateWorkflowRequest struct {
	Prompt              string `json:"prompt"`
	Purpose             string `json:"purpose,omitempty"`
	TargetType          string `json:"target_type,omitempty"`
	Target              string `json:"target,omitempty"`
	SaveMode            string `json:"save_mode,omitempty"`
	WorkflowName        string `json:"workflow_name,omitempty"`
	Workspace           string `json:"workspace,omitempty"`
	ApprovalID          string `json:"approval_id,omitempty"`
	Overwrite           bool   `json:"overwrite,omitempty"`
}

type GenerateWorkflowResponse struct {
	GeneratedWorkflowID int64                    `json:"generated_workflow_id"`
	WorkflowName        string                   `json:"workflow_name,omitempty"`
	GeneratedYAML       string                   `json:"generated_yaml"`
	Validation          *ValidateWorkflowResponse `json:"validation"`
	Saved               bool                     `json:"saved"`
	WorkflowPath        string                   `json:"workflow_path,omitempty"`
	SaveMode            string                   `json:"save_mode"`
}

func (s *Service) GenerateWorkflow(ctx context.Context, req GenerateWorkflowRequest) (*GenerateWorkflowResponse, error) {
	if err := s.requireConfig(); err != nil {
		return nil, err
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	userPrompt := fmt.Sprintf("Purpose: %s\nTarget type: %s\nTarget: %s\n\nUser request:\n%s",
		req.Purpose, req.TargetType, req.Target, prompt)
	generated, err := s.chatCompletion(ctx, workflowGenerationSystemPrompt(), userPrompt)
	if err != nil {
		return nil, err
	}
	yamlContent := extractYAMLFromLLMOutput(generated)

	validation, err := s.ValidateWorkflowYAML(ctx, ValidateWorkflowRequest{YAML: yamlContent})
	if err != nil {
		return nil, err
	}

	saveMode := strings.TrimSpace(req.SaveMode)
	if saveMode == "" {
		saveMode = workflowSaveModeNone
	}

	record := &database.AIGeneratedWorkflow{
		Prompt:           prompt,
		Purpose:          req.Purpose,
		GeneratedYAML:    yamlContent,
		ValidationStatus: validation.ValidationStatus,
		SaveMode:         saveMode,
	}
	if len(validation.Errors) > 0 {
		record.ValidationErrors = strings.Join(validation.Errors, "; ")
	}
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := database.CreateAIGeneratedWorkflow(ctx, record); err != nil {
		return nil, err
	}

	resp := &GenerateWorkflowResponse{
		GeneratedWorkflowID: record.ID,
		GeneratedYAML:       yamlContent,
		Validation:          validation,
		SaveMode:            saveMode,
		WorkflowName:        validation.WorkflowName,
	}
	if req.WorkflowName != "" {
		resp.WorkflowName = req.WorkflowName
	} else if resp.WorkflowName == "" {
		resp.WorkflowName = suggestedWorkflowName(req.Purpose, req.TargetType)
	}

	if saveMode != workflowSaveModeNone {
		if !validation.Valid {
			return nil, fmt.Errorf("cannot save workflow: validation failed")
		}
		path, saveErr := s.saveGeneratedWorkflow(ctx, saveGeneratedWorkflowInput{
			Record:       record,
			WorkflowName: resp.WorkflowName,
			SaveMode:     saveMode,
			Workspace:    req.Workspace,
			ApprovalID:   req.ApprovalID,
			Overwrite:    req.Overwrite,
		})
		if saveErr != nil {
			return nil, saveErr
		}
		resp.Saved = true
		resp.WorkflowPath = path
		record.WorkflowPath = path
		record.ApprovalID = req.ApprovalID
		_ = database.UpdateAIGeneratedWorkflow(ctx, record)
	}

	return resp, nil
}

type PromoteTempWorkflowRequest struct {
	GeneratedWorkflowID int64  `json:"generated_workflow_id"`
	WorkflowName        string `json:"workflow_name,omitempty"`
	ApprovalID          string `json:"approval_id"`
	Overwrite           bool   `json:"overwrite,omitempty"`
}

type PromoteTempWorkflowResponse struct {
	WorkflowName string `json:"workflow_name"`
	WorkflowPath string `json:"workflow_path"`
	Promoted     bool   `json:"promoted"`
}

func (s *Service) PromoteTempWorkflow(ctx context.Context, req PromoteTempWorkflowRequest) (*PromoteTempWorkflowResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := s.requireConfig(); err != nil {
		return nil, err
	}
	if req.GeneratedWorkflowID <= 0 {
		return nil, fmt.Errorf("generated_workflow_id is required")
	}
	if _, err := s.requireApprovedAction(ctx, req.ApprovalID, database.AIActionPromoteWorkflow); err != nil {
		return nil, err
	}

	record, err := database.GetAIGeneratedWorkflow(ctx, req.GeneratedWorkflowID)
	if err != nil {
		return nil, fmt.Errorf("generated workflow not found")
	}
	if record.SaveMode != workflowSaveModeTemporary {
		return nil, fmt.Errorf("workflow is not stored as temporary")
	}

	workflowName := strings.TrimSpace(req.WorkflowName)
	if workflowName == "" {
		workflow, parseErr := parser.ParseContent([]byte(record.GeneratedYAML))
		if parseErr != nil || workflow.Name == "" {
			return nil, fmt.Errorf("workflow_name is required")
		}
		workflowName = workflow.Name
	}

	path, err := s.saveGeneratedWorkflow(ctx, saveGeneratedWorkflowInput{
		Record:       record,
		WorkflowName: workflowName,
		SaveMode:     workflowSaveModeNormal,
		ApprovalID:   req.ApprovalID,
		Overwrite:    req.Overwrite,
	})
	if err != nil {
		return nil, err
	}

	record.SaveMode = workflowSaveModeNormal
	record.WorkflowPath = path
	record.ApprovalID = req.ApprovalID
	_ = database.UpdateAIGeneratedWorkflow(ctx, record)
	_ = s.markApprovalExecuted(ctx, req.ApprovalID, map[string]string{
		"workflow_name": workflowName,
		"workflow_path": path,
	})

	return &PromoteTempWorkflowResponse{
		WorkflowName: workflowName,
		WorkflowPath: path,
		Promoted:     true,
	}, nil
}

type saveGeneratedWorkflowInput struct {
	Record       *database.AIGeneratedWorkflow
	WorkflowName string
	SaveMode     string
	Workspace    string
	ApprovalID   string
	Overwrite    bool
}

func (s *Service) saveGeneratedWorkflow(ctx context.Context, in saveGeneratedWorkflowInput) (string, error) {
	if in.SaveMode == workflowSaveModeNone {
		return "", fmt.Errorf("save_mode cannot be none")
	}

	actionType := database.AIActionSaveWorkflowNormal
	if in.SaveMode == workflowSaveModeTemporary {
		actionType = database.AIActionSaveWorkflowTemporary
	}
	if in.Overwrite {
		actionType = database.AIActionOverwriteWorkflow
	}
	if _, err := s.requireApprovedAction(ctx, in.ApprovalID, actionType); err != nil {
		return "", err
	}

	workflow, err := parser.ParseContent([]byte(in.Record.GeneratedYAML))
	if err != nil {
		return "", err
	}
	if in.WorkflowName != "" {
		workflow.Name = in.WorkflowName
	}
	if workflow.Name == "" {
		return "", fmt.Errorf("workflow name is required")
	}

	var destPath string
	switch in.SaveMode {
	case workflowSaveModeNormal, workflowSaveModeTemporary:
	default:
		return "", fmt.Errorf("invalid save_mode: %s", in.SaveMode)
	}

	if in.SaveMode == workflowSaveModeNormal {
		destPath = filepath.Join(s.cfg.WorkflowsPath, workflowFileName(workflow.Name))
		if !in.Overwrite {
			if _, statErr := os.Stat(destPath); statErr == nil {
				return "", fmt.Errorf("workflow %q already exists; request overwrite approval or choose another name", workflow.Name)
			}
		}
	} else {
		workspace := strings.TrimSpace(in.Workspace)
		if workspace == "" {
			return "", fmt.Errorf("workspace is required for temporary workflow save")
		}
		if !isValidWorkspaceName(workspace) {
			return "", fmt.Errorf("invalid workspace name")
		}
		tempDir := filepath.Join(s.cfg.GetWorkspacesDir(), workspace, "ai-workflows")
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create temporary workflow directory")
		}
		destPath = filepath.Join(tempDir, workflowFileName(workflow.Name))
	}

	workflow.FilePath = destPath
	if err := os.WriteFile(destPath, []byte(in.Record.GeneratedYAML), 0o644); err != nil {
		return "", fmt.Errorf("failed to write workflow file")
	}

	if in.SaveMode == workflowSaveModeNormal {
		if indexErr := database.IndexWorkflow(ctx, workflow); indexErr != nil {
			return "", indexErr
		}
	}

	_ = s.markApprovalExecuted(ctx, in.ApprovalID, map[string]string{
		"workflow_name": workflow.Name,
		"workflow_path": destPath,
		"save_mode":     in.SaveMode,
	})
	return destPath, nil
}

func workflowFileName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(strings.ToLower(name), ".yaml") || strings.HasSuffix(strings.ToLower(name), ".yml") {
		return name
	}
	return name + ".yaml"
}
