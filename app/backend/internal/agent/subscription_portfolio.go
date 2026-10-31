package agent

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/identity"
)

type SubscriptionPortfolioPreview struct {
	ConversationID         string                               `json:"conversation_id"`
	ConversationRevisionID string                               `json:"conversation_revision_id"`
	ContextSecurityVersion int64                                `json:"context_security_version"`
	ProjectID              *string                              `json:"project_id"`
	From                   string                               `json:"from"`
	To                     string                               `json:"to_exclusive"`
	CapturedAt             time.Time                            `json:"captured_at"`
	Summary                finance.SubscriptionPortfolioSummary `json:"summary"`
	ContentText            string                               `json:"content_text"`
	ContentSHA256          string                               `json:"content_sha256"`
	CoverageNotice         string                               `json:"coverage_notice"`
	UntrustedData          bool                                 `json:"untrusted_data"`
}

// A human preview is not an immutable model report or a financial command.
func (s *Service) SubscriptionPortfolio(ctx context.Context, a core.Actor, id string, in finance.SubscriptionPortfolioInput) (SubscriptionPortfolioPreview, error) {
	var out SubscriptionPortfolioPreview
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if c.State != "active" || c.Type == "telegram" || a.Channel == "telegram" {
		return out, core.Missing()
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return out, err
	}
	members, err := aiAudience(ctx, tx, a, c)
	if err != nil {
		return out, err
	}
	ai := a
	ai.RunID = "portfolio-preview"
	content, _, summary, err := finance.AgentSubscriptionPortfolio(ctx, tx, ai, finance.ReviewScope{ProjectID: c.ProjectID, Classification: c.Classification, ConversationID: c.ID, Audience: members}, in)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return out, err
	}
	out = SubscriptionPortfolioPreview{ConversationID: c.ID, ConversationRevisionID: c.CurrentRevisionID, ContextSecurityVersion: c.ContextSecurityVersion, ProjectID: c.ProjectID, From: in.From, To: in.To, CapturedAt: content["captured_at"].(time.Time), Summary: summary, ContentText: string(raw), ContentSHA256: core.Digest(raw), CoverageNotice: content["coverage_notice"].(string), UntrustedData: true}
	return out, tx.Commit(ctx)
}
