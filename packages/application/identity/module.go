package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/agnivo/agnivo/packages/application/identity/apikey"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/auth"
	idhttp "github.com/agnivo/agnivo/packages/application/identity/http"
	"github.com/agnivo/agnivo/packages/application/identity/jwt"
	"github.com/agnivo/agnivo/packages/application/identity/member"
	"github.com/agnivo/agnivo/packages/application/identity/organization"
	"github.com/agnivo/agnivo/packages/application/identity/password"
	"github.com/agnivo/agnivo/packages/application/identity/pat"
	"github.com/agnivo/agnivo/packages/application/identity/session"
	"github.com/agnivo/agnivo/packages/application/identity/user"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the Identity & Access Platform composition root.
type Module struct {
	Auth         *auth.Service
	Users        *user.Service
	Orgs         *organization.Service
	Members      *member.Service
	APIKeys      *apikey.Service
	Sessions     *session.Service
	PATs         *pat.Repository
	JWT          *jwt.Manager
	Revocation   *session.RevocationStore
	MemberRepo   *member.Repository
	HTTP         *idhttp.Handlers
	Middleware   *idhttp.Middleware
}

// Init wires the identity module onto the bootstrap App.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for identity module")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	jwtMgr, err := buildJWTManager(app.Config, app.Log)
	if err != nil {
		return nil, err
	}
	hasher, err := password.NewHasher(password.DefaultParams)
	if err != nil {
		return nil, err
	}

	userRepo := user.NewRepository(app.DB)
	orgRepo := organization.NewRepository(app.DB)
	memberRepo := member.NewRepository(app.DB)
	sessionRepo := session.NewRepository(app.DB)
	apiKeyRepo := apikey.NewRepository(app.DB)
	patRepo := pat.NewRepository(app.DB)
	auditLog := audit.NewLogger(app.DB)
	revocation := session.NewRevocationStore(app.Redis)

	authSvc := auth.NewService(auth.Deps{
		DB: app.DB, Users: userRepo, Orgs: orgRepo, Members: memberRepo,
		Sessions: sessionRepo, Revocation: revocation, JWT: jwtMgr,
		Hasher: hasher, Audit: auditLog, Config: app.Config, Redis: app.Redis,
	})
	userSvc := user.NewService(userRepo, auditLog)
	orgSvc := organization.NewService(orgRepo, memberRepo, auditLog)
	memberSvc := member.NewService(app.DB, memberRepo, userRepo, auditLog)
	apiKeySvc := apikey.NewService(apiKeyRepo, auditLog)
	sessionSvc := session.NewService(sessionRepo, revocation, auditLog)

	mw := idhttp.NewMiddleware(idhttp.MiddlewareDeps{
		JWT: jwtMgr, Revocation: revocation, Members: memberRepo,
		APIKeys: apiKeySvc, PATs: patRepo,
	})
	h := idhttp.NewHandlers(idhttp.HandlersDeps{
		Auth: authSvc, Users: userSvc, Orgs: orgSvc, Members: memberSvc,
		APIKeys: apiKeySvc, Sessions: sessionSvc,
	})

	mod := &Module{
		Auth: authSvc, Users: userSvc, Orgs: orgSvc, Members: memberSvc,
		APIKeys: apiKeySvc, Sessions: sessionSvc, PATs: patRepo,
		JWT: jwtMgr, Revocation: revocation, MemberRepo: memberRepo,
		HTTP: h, Middleware: mw,
	}
	return mod, nil
}

// MountRoutes registers identity REST endpoints on the router.
func (m *Module) MountRoutes(r chi.Router) {
	idhttp.Mount(r, m.HTTP, m.Middleware)
}

func buildJWTManager(cfg *config.Config, log *zap.Logger) (*jwt.Manager, error) {
	jcfg := cfg.Identity.JWT
	privPEM, pubPEM := jcfg.PrivateKeyPEM, jcfg.PublicKeyPEM

	if privPEM == "" || pubPEM == "" {
		log.Warn("identity JWT keys not configured; generating ephemeral development keys")
		priv, pub, err := jwt.GenerateKeyPair()
		if err != nil {
			return nil, err
		}
		privPEM = encodePrivateKeyPEM(priv)
		pubPEM = encodePublicKeyPEM(pub)
	}

	priv, pub, err := jwt.LoadKeysFromPEM(privPEM, pubPEM)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "identity: load jwt keys")
	}
	return jwt.NewManager(jwt.Config{
		PrivateKey: priv, PublicKey: pub,
		Issuer: jcfg.Issuer, Audience: jcfg.Audience,
		AccessTTL: jcfg.AccessTTL, ClockSkew: jcfg.ClockSkew,
	}), nil
}

func encodePrivateKeyPEM(key *rsa.PrivateKey) string {
	b, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

func encodePublicKeyPEM(key *rsa.PublicKey) string {
	b, _ := x509.MarshalPKIXPublicKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}))
}

// GenerateDevKeyPairPEM returns PEM-encoded RSA keys for local development.
func GenerateDevKeyPairPEM() (privatePEM, publicPEM string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}
	return encodePrivateKeyPEM(priv), encodePublicKeyPEM(&priv.PublicKey), nil
}
