package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/j3ssie/osmedeus/v5/internal/database"
)

const defaultApprovalTTL = 30 * time.Minute

type RequestApprovalRequest struct {
	ActionType      string                 `json:"action_type"`
	Payload         map[string]interface{} `json:"payload"`
	RequesterSource string                 `json:"requester_source,omitempty"`
	TTLMinutes      int                    `json:"ttl_minutes,omitempty"`
}

type RequestApprovalResponse struct {
	ApprovalID string    `json:"approval_id"`
	Status     string    `json:"status"`
	ActionType string    `json:"action_type"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

func (s *Service) RequestApproval(ctx context.Context, req RequestApprovalRequest) (*RequestApprovalResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	actionType := strings.TrimSpace(req.ActionType)
	if actionType == "" {
		return nil, fmt.Errorf("action_type is required")
	}
	if !isKnownApprovalAction(actionType) {
		return nil, fmt.Errorf("unknown action_type: %s", actionType)
	}
	if req.Payload == nil {
		return nil, fmt.Errorf("payload is required")
	}

	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload")
	}

	ttl := defaultApprovalTTL
	if req.TTLMinutes > 0 {
		ttl = time.Duration(req.TTLMinutes) * time.Minute
	}
	expiresAt := time.Now().Add(ttl)

	approval := &database.AIApproval{
		ActionType:           actionType,
		Status:               database.AIApprovalStatusPending,
		RequestedPayloadJSON: string(payloadJSON),
		RequesterSource:      req.RequesterSource,
		ExpiresAt:            &expiresAt,
	}
	if err := database.CreateAIApproval(ctx, approval); err != nil {
		return nil, err
	}

	return &RequestApprovalResponse{
		ApprovalID: approval.ID,
		Status:     approval.Status,
		ActionType: approval.ActionType,
		ExpiresAt:  expiresAt,
	}, nil
}

type GetApprovalRequest struct {
	ApprovalID string `json:"approval_id"`
}

type GetApprovalResponse struct {
	Approval *database.AIApproval   `json:"approval"`
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

func (s *Service) GetApproval(ctx context.Context, req GetApprovalRequest) (*GetApprovalResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID == "" {
		return nil, fmt.Errorf("approval_id is required")
	}

	approval, err := database.GetAIApproval(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("approval not found")
	}
	if err := s.expireApprovalIfNeeded(ctx, approval); err != nil {
		return nil, err
	}

	resp := &GetApprovalResponse{Approval: approval}
	if approval.RequestedPayloadJSON != "" {
		var payload map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(approval.RequestedPayloadJSON), &payload); jsonErr == nil {
			resp.Payload = payload
		}
	}
	return resp, nil
}

type ApproveRequest struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"`
}

type ApproveResponse struct {
	ApprovalID string `json:"approval_id"`
	Status     string `json:"status"`
}

func (s *Service) Approve(ctx context.Context, req ApproveRequest) (*ApproveResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID == "" {
		return nil, fmt.Errorf("approval_id is required")
	}
	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision != "approve" && decision != "reject" {
		return nil, fmt.Errorf("decision must be approve or reject")
	}

	approval, err := database.GetAIApproval(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("approval not found")
	}
	if err := s.expireApprovalIfNeeded(ctx, approval); err != nil {
		return nil, err
	}
	if approval.Status != database.AIApprovalStatusPending {
		return nil, fmt.Errorf("approval is not pending")
	}

	now := time.Now()
	if decision == "approve" {
		approval.Status = database.AIApprovalStatusApproved
		approval.ApprovedAt = &now
	} else {
		approval.Status = database.AIApprovalStatusRejected
		approval.RejectedAt = &now
	}
	if err := database.UpdateAIApproval(ctx, approval); err != nil {
		return nil, err
	}

	return &ApproveResponse{
		ApprovalID: approval.ID,
		Status:     approval.Status,
	}, nil
}

func (s *Service) requireApprovedAction(ctx context.Context, approvalID, actionType string) (*database.AIApproval, error) {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return nil, fmt.Errorf("approval_id is required")
	}
	approval, err := database.GetAIApproval(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("approval not found")
	}
	if err := s.expireApprovalIfNeeded(ctx, approval); err != nil {
		return nil, err
	}
	if approval.Status != database.AIApprovalStatusApproved {
		return nil, fmt.Errorf("approval is not approved")
	}
	if approval.ActionType != actionType {
		return nil, fmt.Errorf("approval action_type mismatch: expected %s", actionType)
	}
	return approval, nil
}

func (s *Service) markApprovalExecuted(ctx context.Context, approvalID string, result map[string]string) error {
	approval, err := database.GetAIApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	now := time.Now()
	approval.Status = database.AIApprovalStatusExecuted
	approval.ExecutedAt = &now
	if result != nil {
		if data, err := json.Marshal(result); err == nil {
			approval.ResultPayloadJSON = string(data)
		}
	}
	return database.UpdateAIApproval(ctx, approval)
}

func (s *Service) expireApprovalIfNeeded(ctx context.Context, approval *database.AIApproval) error {
	if approval == nil || approval.ExpiresAt == nil {
		return nil
	}
	if approval.Status != database.AIApprovalStatusPending {
		return nil
	}
	if time.Now().After(*approval.ExpiresAt) {
		approval.Status = database.AIApprovalStatusExpired
		if err := database.UpdateAIApproval(ctx, approval); err != nil {
			return err
		}
		return fmt.Errorf("approval expired")
	}
	return nil
}

func isKnownApprovalAction(actionType string) bool {
	switch actionType {
	case database.AIActionStartRun,
		database.AIActionSaveWorkflowNormal,
		database.AIActionSaveWorkflowTemporary,
		database.AIActionPromoteWorkflow,
		database.AIActionOverwriteWorkflow:
		return true
	default:
		return false
	}
}
