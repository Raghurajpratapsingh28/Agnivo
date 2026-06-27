package http

import (
	idhttp "github.com/agnivo/agnivo/packages/application/identity/http"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/go-chi/chi/v5"
)

// Mount registers control plane routes under /orgs/{orgID}.
func Mount(r chi.Router, h *Handlers, mw *idhttp.Middleware) {
	r.Route("/orgs/{orgID}", func(r chi.Router) {
		r.Use(mw.Authenticate)
		r.Use(mw.RequireAuth)
		r.Use(mw.OrgContext)

		// Projects
		r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/projects", h.ListProjects)
		r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/projects", h.CreateProject)

		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/", h.GetProject)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Patch("/", h.UpdateProject)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Delete("/", h.DeleteProject)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/archive", h.ArchiveProject)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/restore", h.RestoreProject)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/duplicate", h.DuplicateProject)
			r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/pause", h.PauseProject)
			r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/resume", h.ResumeProject)
			r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/restart", h.RestartProject)

			// Deployments
			r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/deploy", h.Deploy)
			r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/rollback", h.Rollback)
			r.With(mw.RequirePermission(rbac.PermDeployRead)).Get("/deployments", h.ListDeployments)
			r.With(mw.RequirePermission(rbac.PermDeployRead)).Get("/deployments/latest", h.GetLatestDeployment)

			// Repository
			r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/repository", h.GetRepository)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/repository", h.ConnectRepository)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Patch("/repository", h.UpdateRepository)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Delete("/repository", h.DisconnectRepository)

			// Environment variables
			r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/env", h.ListEnv)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/env", h.CreateEnv)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Patch("/env/{envID}", h.UpdateEnv)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Delete("/env/{envID}", h.DeleteEnv)

			// Secrets
			r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/secrets", h.ListSecrets)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/secrets", h.CreateSecret)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/secrets/{secretID}/rotate", h.RotateSecret)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Delete("/secrets/{secretID}", h.DeleteSecret)

			// Domains
			r.With(mw.RequirePermission(rbac.PermProjectRead)).Get("/domains", h.ListDomains)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Post("/domains", h.CreateDomain)
			r.With(mw.RequirePermission(rbac.PermProjectWrite)).Delete("/domains/{domainID}", h.DeleteDomain)
		})

		// Org-level deployment routes
		r.With(mw.RequirePermission(rbac.PermDeployRead)).Get("/deployments/{deploymentID}", h.GetDeployment)
		r.With(mw.RequirePermission(rbac.PermDeployRead)).Get("/deployments/{deploymentID}/timeline", h.DeploymentTimeline)
		r.With(mw.RequirePermission(rbac.PermDeployWrite)).Post("/deployments/{deploymentID}/cancel", h.CancelDeployment)
	})
}
