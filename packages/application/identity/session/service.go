package session

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Service handles session management.
type Service struct {
	repo       *Repository
	revocation *RevocationStore
	audit      *audit.Logger
}

// NewService constructs a session service.
func NewService(repo *Repository, revocation *RevocationStore, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, revocation: revocation, audit: auditLog}
}

// List returns active sessions for the authenticated user.
func (s *Service) List(ctx context.Context, userID string) ([]Session, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Revoke revokes a single session.
func (s *Service) Revoke(ctx context.Context, userID, sessionID, ip, ua string) error {
	sess, err := s.getOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if err := s.repo.Revoke(ctx, sessionID, userID); err != nil {
		return err
	}
	_ = s.revocation.RevokeSession(ctx, sessionID, sess.ExpiresAt)
	uid := userID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionSessionRevoke, ResourceType: "session", ResourceID: sessionID, IPAddress: ip, UserAgent: ua})
}

// RevokeAll revokes all sessions except the current one.
func (s *Service) RevokeAll(ctx context.Context, userID, exceptSessionID, ip, ua string) (int64, error) {
	n, err := s.repo.RevokeAll(ctx, userID, exceptSessionID)
	if err != nil {
		return 0, err
	}
	uid := userID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionSessionRevoke, IPAddress: ip, UserAgent: ua})
	return n, nil
}

func (s *Service) getOwnedSession(ctx context.Context, userID, sessionID string) (Session, error) {
	sessions, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return Session{}, err
	}
	for _, sess := range sessions {
		if sess.ID == sessionID {
			return sess, nil
		}
	}
	return Session{}, errors.NotFound("session not found")
}
