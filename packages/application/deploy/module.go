package deploy

import (
	"context"
	"fmt"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildstore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpevents"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/deployment"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/envvar"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/project"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/secret"
	deploycancel "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/deploystore"
	deployecr "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/ecr"
	deployevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/executor"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/health"
	deployhttp "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/http"
	deploymetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/recovery"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/rollback"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/runtime"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/scheduler"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/secrets"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/strategy"
	deployworker "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/worker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	pevents "github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the deployer composition root.
type Module struct {
	Worker   *jobs.Worker
	Handler  *deployworker.Handler
	HTTP     *deployhttp.Handlers
	Metrics  *deploymetrics.Metrics
	Pipeline *executor.Pipeline
	Cancels  *deploycancel.Registry
	Queue    *jobs.Queue
}

// Init wires the deployer module.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for deployer")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	vault, err := crypto.NewVault(app.Config)
	if err != nil {
		return nil, err
	}

	bus := pevents.NewInMemory(ctx, pevents.Config{})
	app.AddHook(lifecycle.Hook{Name: "deployer-event-bus", Stop: bus.Close})

	jobMetrics := jobs.NewMetrics(app.Config.App.Name)
	app.Metrics.MustRegister(jobMetrics.Collectors()...)
	deployMetrics := deploymetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(deployMetrics.Collectors()...)

	queue := jobs.NewQueue(app.DB, jobMetrics)
	execRepo := deploystore.NewRepository(app.DB)
	containerRepo := deploystore.NewContainerRepository(app.DB)
	healthRepo := deploystore.NewHealthRepository(app.DB)
	eventRepo := deployevents.NewRepository(app.DB)
	publisher := deployevents.NewPublisher(bus, app.Config.App.Name, eventRepo)
	cpPublisher := cpevents.NewPublisher(bus, app.Config.App.Name)

	secretLoader := secrets.NewLoader(envvar.NewRepository(app.DB), secret.NewRepository(app.DB), vault)
	puller := deployecr.NewPuller(app.Config.Deployer.ECR, app.Config.Deployer.DockerCLI)
	sched := scheduler.NewClient(app.Config.Deployer)
	rt := runtime.NewAgentDriver(app.Config.Deployer)
	healthChecker := health.NewChecker(app.Config.Deployer.Health)
	strategies := strategy.NewRegistry(app.Config.Deployer.DefaultStrategy)
	cancels := deploycancel.NewRegistry()

	deploymentRepo := deployment.NewRepository(app.DB)
	rollbackEngine := rollback.NewEngine(deploymentRepo, rt, publisher)

	workerID := fmt.Sprintf("deployer-%s", app.Config.App.Name)
	pipeline := executor.NewPipeline(executor.Deps{
		Config: app.Config.Deployer, Deployments: deploymentRepo,
		Projects:   project.NewRepository(app.DB),
		Artifacts:  buildstore.NewArtifactRepository(app.DB),
		Executions: execRepo, Containers: containerRepo, HealthRepo: healthRepo,
		Events: publisher, CPEvents: cpPublisher, Secrets: secretLoader,
		Puller: puller, Scheduler: sched, Runtime: rt, Health: healthChecker,
		Strategies: strategies, Rollback: rollbackEngine, Metrics: deployMetrics,
		Cancels: cancels, WorkerID: workerID,
	})

	handler := deployworker.NewHandler(pipeline, cancels)
	httpHandlers := deployhttp.NewHandlers(execRepo, healthRepo, queue, handler, cancels)

	workerCfg := jobs.WorkerConfig{
		Queue:        cpjobs.QueueDeployments,
		Concurrency:  app.Config.Deployer.Concurrency,
		BatchSize:    app.Config.Deployer.Concurrency,
		PollInterval: app.Config.Deployer.PollInterval,
		Visibility:   app.Config.Deployer.Visibility,
		Logger:       app.Log,
	}
	jw := jobs.NewWorker(queue, workerCfg)
	jw.Handle(cpjobs.TypeDeploy, handler.Handle)
	jw.Handle(cpjobs.TypeRollback, handler.Handle)
	jw.Handle(cpjobs.TypeDeleteDeployment, handler.Handle)
	jw.Handle(cpjobs.TypeSleep, handler.Handle)
	jw.Handle(cpjobs.TypeWake, handler.Handle)

	app.AddRunner("deploy-worker", jw.Run)

	_ = recovery.NewMonitor(execRepo, app.Config.Deployer.Visibility)

	if app.Config.Deployer.InternalPort > 0 {
		app.RegisterInternalServer("deployer-internal", app.Config.Deployer.InternalPort, func(r chi.Router) {
			deployhttp.Mount(r, httpHandlers)
		})
	}

	app.Log.Info("deployer module initialized",
		zap.Int("concurrency", app.Config.Deployer.Concurrency),
		zap.String("default_strategy", app.Config.Deployer.DefaultStrategy),
	)

	return &Module{
		Worker: jw, Handler: handler, HTTP: httpHandlers,
		Metrics: deployMetrics, Pipeline: pipeline, Cancels: cancels, Queue: queue,
	}, nil
}
