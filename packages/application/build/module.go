package build

import (
	"context"
	"fmt"

	buildhttp "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/http"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildkit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildstore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cancel"
	buildevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/ecr"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/executor"
	buildgit "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/git"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/logs"
	buildmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/metrics"
	buildworker "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/worker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/deployment"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/envvar"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/gitrepo"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/project"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the builder composition root.
type Module struct {
	Worker   *jobs.Worker
	Handler  *buildworker.Handler
	HTTP     *buildhttp.Handlers
	Metrics  *buildmetrics.Metrics
	Pipeline *executor.Pipeline
	Cancels  *cancel.Registry
	Queue    *jobs.Queue
}

// Init wires the builder module.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for builder")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	vault, err := crypto.NewVault(app.Config)
	if err != nil {
		return nil, err
	}

	bus := events.NewInMemory(ctx, events.Config{})
	app.AddHook(lifecycle.Hook{Name: "builder-event-bus", Stop: bus.Close})

	jobMetrics := jobs.NewMetrics(app.Config.App.Name)
	app.Metrics.MustRegister(jobMetrics.Collectors()...)
	buildMetrics := buildmetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(buildMetrics.Collectors()...)

	queue := jobs.NewQueue(app.DB, jobMetrics)
	buildRepo := buildstore.NewRepository(app.DB)
	artifactRepo := buildstore.NewArtifactRepository(app.DB)
	logRepo := buildstore.NewLogRepository(app.DB)
	eventRepo := buildevents.NewRepository(app.DB)
	publisher := buildevents.NewPublisher(bus, app.Config.App.Name, eventRepo)

	var logStreamer *logs.Streamer
	if app.Redis != nil {
		logStreamer = logs.NewStreamer(logRepo, app.Redis)
	} else {
		logStreamer = logs.NewStreamer(logRepo, nil)
	}

	gitMgr := buildgit.NewManager(app.Config.Builder.WorkspaceDir)
	bkit := buildkit.NewBuilder(app.Config.Builder)
	ecrClient := ecr.NewClient(app.Config.Builder.ECR, app.Config.Builder.DockerCLI)
	cancels := cancel.NewRegistry()

	workerID := fmt.Sprintf("builder-%s", app.Config.App.Name)
	pipeline := executor.NewPipeline(executor.Deps{
		Config: app.Config.Builder, Vault: vault,
		Projects: project.NewRepository(app.DB),
		Deployments: deployment.NewRepository(app.DB),
		GitStore: gitrepo.NewStore(app.DB),
		EnvRepo: envvar.NewRepository(app.DB),
		Builds: buildRepo, Artifacts: artifactRepo,
		Events: publisher, Logs: logStreamer, Git: gitMgr,
		Builder: bkit, ECR: ecrClient, Metrics: buildMetrics,
		Cancels: cancels, WorkerID: workerID,
	})

	handler := buildworker.NewHandler(pipeline, cancels)
	httpHandlers := buildhttp.NewHandlers(buildRepo, logRepo, queue, handler, cancels)

	workerCfg := jobs.WorkerConfig{
		Queue: cpjobs.QueueBuilds,
		Concurrency: app.Config.Builder.Concurrency,
		BatchSize: app.Config.Builder.Concurrency,
		PollInterval: app.Config.Builder.PollInterval,
		Visibility: app.Config.Builder.Visibility,
		Logger: app.Log,
	}
	jw := jobs.NewWorker(queue, workerCfg)
	jw.Handle(cpjobs.TypeBuild, handler.Handle)

	app.AddRunner("build-worker", jw.Run)

	if app.Config.Builder.InternalPort > 0 {
		app.RegisterInternalServer("builder-internal", app.Config.Builder.InternalPort, func(r chi.Router) {
			buildhttp.Mount(r, httpHandlers)
		})
	}

	app.Log.Info("builder module initialized",
		zap.Int("concurrency", app.Config.Builder.Concurrency),
		zap.String("workspace", app.Config.Builder.WorkspaceDir),
		zap.Bool("ecr_enabled", app.Config.Builder.ECR.Enabled),
	)

	return &Module{
		Worker: jw, Handler: handler, HTTP: httpHandlers,
		Metrics: buildMetrics, Pipeline: pipeline, Cancels: cancels, Queue: queue,
	}, nil
}

