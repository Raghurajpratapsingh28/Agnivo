package auth_test

import (
	"context"
	"testing"

	"github.com/agnivo/agnivo/packages/application/identity"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/auth"
	"github.com/agnivo/agnivo/packages/application/identity/jwt"
	"github.com/agnivo/agnivo/packages/application/identity/member"
	"github.com/agnivo/agnivo/packages/application/identity/organization"
	"github.com/agnivo/agnivo/packages/application/identity/password"
	"github.com/agnivo/agnivo/packages/application/identity/session"
	"github.com/agnivo/agnivo/packages/application/identity/user"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRegisterLoginRefreshLogout(t *testing.T) {
	ctx := context.Background()
	db := testkit.ConnectPostgres(t)
	testkit.RunMigrations(t, db, identity.Migrations()...)

	priv, pub, err := jwt.GenerateKeyPair()
	require.NoError(t, err)
	jwtMgr := jwt.NewManager(jwt.Config{PrivateKey: priv, PublicKey: pub, Issuer: "test", Audience: "test"})
	hasher, err := password.NewHasher(password.DefaultParams)
	require.NoError(t, err)

	userRepo := user.NewRepository(db)
	orgRepo := organization.NewRepository(db)
	memberRepo := member.NewRepository(db)
	sessionRepo := session.NewRepository(db)
	auditLog := audit.NewLogger(db)

	svc := auth.NewService(auth.Deps{
		DB: db, Users: userRepo, Orgs: orgRepo, Members: memberRepo,
		Sessions: sessionRepo, Revocation: session.NewRevocationStore(nil),
		JWT: jwtMgr, Hasher: hasher, Audit: auditLog,
		Config: &config.Config{Identity: config.Identity{JWT: config.JWT{RefreshTTL: 0}}},
	})

	u, err := svc.Register(ctx, auth.RegisterInput{
		Email: "test@example.com", Password: "secure-password-12", DisplayName: "Test User", OrgName: "Acme",
	}, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.NotEmpty(t, u.ID)

	tokens, err := svc.Login(ctx, auth.LoginInput{Email: "test@example.com", Password: "secure-password-12"}, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.NotEmpty(t, tokens.AccessToken)
	assert.NotEmpty(t, tokens.RefreshToken)

	refreshed, err := svc.Refresh(ctx, tokens.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.NotEmpty(t, refreshed.AccessToken)

	require.NoError(t, svc.Logout(ctx, refreshed.RefreshToken, "127.0.0.1", "test"))
}
