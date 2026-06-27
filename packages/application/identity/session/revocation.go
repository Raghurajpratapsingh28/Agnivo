package session

import (
	"context"
	"fmt"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
)

// RevocationStore tracks revoked session and token JTIs in Redis for fast
// access-token rejection without hitting PostgreSQL on every request.
type RevocationStore struct {
	redis *redis.Client
}

// NewRevocationStore constructs a revocation store. redis may be nil (no-op).
func NewRevocationStore(r *redis.Client) *RevocationStore { return &RevocationStore{redis: r} }

func keySession(sessionID string) string { return "identity:revoke:session:" + sessionID }

// RevokeSession marks sessionID as revoked until exp.
func (s *RevocationStore) RevokeSession(ctx context.Context, sessionID string, exp time.Time) error {
	if s.redis == nil {
		return nil
	}
	ttl := time.Until(exp)
	if ttl <= 0 {
		ttl = time.Minute
	}
	return s.redis.SetTTL(ctx, keySession(sessionID), "1", ttl)
}

// IsSessionRevoked reports whether sessionID is revoked.
func (s *RevocationStore) IsSessionRevoked(ctx context.Context, sessionID string) (bool, error) {
	if s.redis == nil || sessionID == "" {
		return false, nil
	}
	_, ok, err := s.redis.GetString(ctx, keySession(sessionID))
	return ok, err
}

// RevokeAccessJTI revokes a specific access token JTI until exp.
func (s *RevocationStore) RevokeAccessJTI(ctx context.Context, jti string, exp time.Time) error {
	if s.redis == nil {
		return nil
	}
	ttl := time.Until(exp)
	if ttl <= 0 {
		ttl = time.Minute
	}
	return s.redis.SetTTL(ctx, fmt.Sprintf("identity:revoke:jti:%s", jti), "1", ttl)
}
