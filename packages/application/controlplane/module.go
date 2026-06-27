package controlplane

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/controlplane/crypto"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpjobs"
	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	"github.com/agnivo/agnivo/packages/application/controlplane/domain"
	"github.com/agnivo/agnivo/packages/application/controlplane/envvar"
	cphttp "github.com/agnivo/agnivo/packages/application/controlplane/http"
	"github.com/agnivo/agnivo/packages/application/controlplane/gitrepo"
	"github.com/agnivo/agnivo/packages/application/controlplane/project"
	"github.com/agnivo/agnivo/packages/application/controlplane/secret"
	"github.com/agnivo/agnivo/packages/application/controlplane/webhook"
	"github.com/agnivo/agnivo/packages/application/identity"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"github.com/agnivo/agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
)

// Module is the control plane composition root.
type Module struct {
	Projects    *project.Service
	Deployments *deployment.Service
	Git         *gitrepo.Service
	EnvVars     *envvar.Service
	Secrets     *secret.Service
	Domains     *domain.Service
	Webhooks    *webhook.Service
	Events      *cpevents.Publisher
	Jobs        *cpjobs.Enqueuer
	HTTP        *cphttp.Handlers
	Bus         events.Bus
}

// Init wires the control plane module.
func Init(ctx context.Context, app *bootstrap.App, id *identity.Module) (*Module, error) {
	_ = id
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for control plane")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	vault, err := crypto.NewVault(app.Config)
	if err != nil {
		return nil, err
	}

	bus := events.NewInMemory(ctx, events.Config{})
	jobMetrics := jobs.NewMetrics(app.Config.App.Name)
	queue := jobs.NewQueue(app.DB, jobMetrics)
	enqueuer := cpjobs.NewEnqueuer(queue)
	publisher := cpevents.NewPublisher(bus, app.Config.App.Name)

	auditLog := audit.NewLogger(app.DB)
	projectRepo := project.NewRepository(app.DB)
	deploymentRepo := deployment.NewRepository(app.DB)
	gitStore := gitrepo.NewStore(app.DB)
	envRepo := envvar.NewRepository(app.DB)
	secretRepo := secret.NewRepository(app.DB)
	domainRepo := domain.NewRepository(app.DB)

	projectSvc := project.NewService(projectRepo, auditLog, publisher, app.Config.ControlPlane.DefaultRegion)
	deploymentSvc := deployment.NewService(deploymentRepo, projectRepo, enqueuer, publisher, auditLog)
	gitSvc := gitrepo.NewService(gitStore, vault, publisher, auditLog)
	envSvc := envvar.NewService(envRepo, vault, publisher, auditLog)
	secretSvc := secret.NewService(secretRepo, vault, publisher, auditLog)
	domainSvc := domain.NewService(domainRepo, enqueuer, publisher, auditLog)
	webhookSvc := webhook.NewService(app.DB, deploymentSvc, gitSvc, publisher, app.Config)

	h := cphttp.NewHandlers(cphttp.HandlersDeps{
		Projects: projectSvc, Deployments: deploymentSvc, Git: gitSvc,
		EnvVars: envSvc, Secrets: secretSvc, Domains: domainSvc,
	})

	app.AddHook(lifecycle.Hook{Name: "event-bus", Stop: bus.Close})

	return &Module{
		Projects: projectSvc, Deployments: deploymentSvc, Git: gitSvc,
		EnvVars: envSvc, Secrets: secretSvc, Domains: domainSvc,
		Webhooks: webhookSvc, Events: publisher, Jobs: enqueuer,
		HTTP: h, Bus: bus,
	}, nil
}

// MountRoutes registers control plane REST endpoints.
func (m *Module) MountRoutes(r chi.Router, id *identity.Module) {
	cphttp.Mount(r, m.HTTP, id.Middleware)
}
