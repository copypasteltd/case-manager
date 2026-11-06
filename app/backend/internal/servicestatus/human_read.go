package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
)

// Public human reads hold original-session and organization locks for the whole
// snapshot. Shared AgentSummary and worker recipient checks never enter here.
func (s *Service) humanReader(ctx context.Context, a core.Actor) (*Service, func(), error) {
	if s.readTx != nil {
		return s, func() {}, nil
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return nil, func() {}, err
	}
	reader := *s
	reader.readTx = tx
	return &reader, func() { _ = tx.Rollback(ctx) }, nil
}
func (s *Service) readDB() platform.DBTX {
	if s.readTx != nil {
		return s.readTx
	}
	return s.DB
}
