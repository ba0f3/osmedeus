package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	AIApprovalStatusPending  = "pending"
	AIApprovalStatusApproved = "approved"
	AIApprovalStatusRejected = "rejected"
	AIApprovalStatusExpired  = "expired"
	AIApprovalStatusExecuted = "executed"
)

const (
	AIActionStartRun              = "start_run"
	AIActionSaveWorkflowNormal    = "save_workflow_normal"
	AIActionSaveWorkflowTemporary = "save_workflow_temporary"
	AIActionPromoteWorkflow       = "promote_workflow"
	AIActionOverwriteWorkflow     = "overwrite_workflow"
)

// AIApproval tracks pending and completed AI-initiated actions.
type AIApproval struct {
	bun.BaseModel `bun:"table:ai_approvals,alias:aa"`

	ID                   string     `bun:"id,pk" json:"id"`
	ActionType           string     `bun:"action_type,notnull" json:"action_type"`
	Status               string     `bun:"status,notnull" json:"status"`
	RequestedPayloadJSON string     `bun:"requested_payload_json,type:text" json:"requested_payload_json"`
	ResultPayloadJSON    string     `bun:"result_payload_json,type:text" json:"result_payload_json,omitempty"`
	RequesterSource      string     `bun:"requester_source" json:"requester_source,omitempty"`
	CreatedAt            time.Time  `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	ApprovedAt           *time.Time `bun:"approved_at" json:"approved_at,omitempty"`
	RejectedAt           *time.Time `bun:"rejected_at" json:"rejected_at,omitempty"`
	ExecutedAt           *time.Time `bun:"executed_at" json:"executed_at,omitempty"`
	ExpiresAt            *time.Time `bun:"expires_at" json:"expires_at,omitempty"`
}

// AIGeneratedWorkflow stores generated workflow YAML and validation metadata.
type AIGeneratedWorkflow struct {
	bun.BaseModel `bun:"table:ai_generated_workflows,alias:agw"`

	ID               int64     `bun:"id,pk,autoincrement" json:"id"`
	Prompt           string    `bun:"prompt,type:text" json:"prompt"`
	Purpose          string    `bun:"purpose" json:"purpose,omitempty"`
	GeneratedYAML    string    `bun:"generated_yaml,type:text" json:"generated_yaml"`
	ValidationStatus string    `bun:"validation_status" json:"validation_status"`
	ValidationErrors string    `bun:"validation_errors,type:text" json:"validation_errors,omitempty"`
	SaveMode         string    `bun:"save_mode" json:"save_mode,omitempty"`
	WorkflowPath     string    `bun:"workflow_path" json:"workflow_path,omitempty"`
	RunUUID          string    `bun:"run_uuid" json:"run_uuid,omitempty"`
	ApprovalID       string    `bun:"approval_id" json:"approval_id,omitempty"`
	CreatedAt        time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}

func CreateAIApproval(ctx context.Context, approval *AIApproval) error {
	if db == nil {
		return fmt.Errorf("database not connected")
	}
	if approval.ID == "" {
		approval.ID = uuid.New().String()
	}
	if approval.Status == "" {
		approval.Status = AIApprovalStatusPending
	}
	_, err := db.NewInsert().Model(approval).Exec(ctx)
	return err
}

func GetAIApproval(ctx context.Context, id string) (*AIApproval, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	var approval AIApproval
	err := db.NewSelect().Model(&approval).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func UpdateAIApproval(ctx context.Context, approval *AIApproval) error {
	if db == nil {
		return fmt.Errorf("database not connected")
	}
	_, err := db.NewUpdate().Model(approval).WherePK().Exec(ctx)
	return err
}

func CreateAIGeneratedWorkflow(ctx context.Context, record *AIGeneratedWorkflow) error {
	if db == nil {
		return fmt.Errorf("database not connected")
	}
	_, err := db.NewInsert().Model(record).Exec(ctx)
	return err
}

func UpdateAIGeneratedWorkflow(ctx context.Context, record *AIGeneratedWorkflow) error {
	if db == nil {
		return fmt.Errorf("database not connected")
	}
	_, err := db.NewUpdate().Model(record).WherePK().Exec(ctx)
	return err
}

func GetAIGeneratedWorkflow(ctx context.Context, id int64) (*AIGeneratedWorkflow, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	var record AIGeneratedWorkflow
	err := db.NewSelect().Model(&record).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &record, nil
}
