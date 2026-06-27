// Package http exposes REST handlers for the control plane.
package http

import (
	"net/http"

	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	"github.com/agnivo/agnivo/packages/application/controlplane/domain"
	"github.com/agnivo/agnivo/packages/application/controlplane/envvar"
	"github.com/agnivo/agnivo/packages/application/controlplane/gitrepo"
	"github.com/agnivo/agnivo/packages/application/controlplane/project"
	"github.com/agnivo/agnivo/packages/application/controlplane/secret"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/dto"
	"github.com/agnivo/agnivo/packages/platform/httpx"
)

// HandlersDeps configures control plane HTTP handlers.
type HandlersDeps struct {
	Projects    *project.Service
	Deployments *deployment.Service
	Git         *gitrepo.Service
	EnvVars     *envvar.Service
	Secrets     *secret.Service
	Domains     *domain.Service
}

// Handlers exposes control plane REST handlers.
type Handlers struct {
	projects    *project.Service
	deployments *deployment.Service
	git         *gitrepo.Service
	envVars     *envvar.Service
	secrets     *secret.Service
	domains     *domain.Service
}

// NewHandlers constructs control plane handlers.
func NewHandlers(d HandlersDeps) *Handlers {
	return &Handlers{
		projects: d.Projects, deployments: d.Deployments, git: d.Git,
		envVars: d.EnvVars, secrets: d.Secrets, domains: d.Domains,
	}
}

func clientMeta(r *http.Request) (ip, ua string) {
	return httpx.ClientIP(r), httpx.Header(r, "User-Agent")
}

func orgID(r *http.Request) (string, error) { return httpx.RequirePathParam(r, "orgID") }
func projectID(r *http.Request) (string, error) { return httpx.RequirePathParam(r, "projectID") }

// ListProjects GET /orgs/{orgID}/projects
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	org, err := orgID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	includeArchived, _ := httpx.QueryBool(r, "include_archived")
	items, err := h.projects.List(r.Context(), org, includeArchived)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, items)
}

// CreateProject POST /orgs/{orgID}/projects
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	org, err := orgID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in project.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	p, err := h.projects.Create(r.Context(), org, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, p)
}

// GetProject GET /orgs/{orgID}/projects/{projectID}
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	org, err := orgID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	pid, err := projectID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	p, err := h.projects.Get(r.Context(), org, pid)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, p)
}

// UpdateProject PATCH /orgs/{orgID}/projects/{projectID}
func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	org, err := orgID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	pid, err := projectID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, _ := tenant.RequireUser(r.Context())
	var in project.UpdateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	p, err := h.projects.Update(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, p)
}

// DeleteProject DELETE /orgs/{orgID}/projects/{projectID}
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	org, err := orgID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	pid, err := projectID(r)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.projects.Delete(r.Context(), org, pid, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ArchiveProject POST /orgs/{orgID}/projects/{projectID}/archive
func (h *Handlers) ArchiveProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	p, err := h.projects.Archive(r.Context(), org, pid, userID, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, p)
}

// RestoreProject POST /orgs/{orgID}/projects/{projectID}/restore
func (h *Handlers) RestoreProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	p, err := h.projects.Restore(r.Context(), org, pid, userID, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, p)
}

// DuplicateProject POST /orgs/{orgID}/projects/{projectID}/duplicate
func (h *Handlers) DuplicateProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var body struct {
		Name string `json:"name" validate:"omitempty,min=1,max=100"`
	}
	if err := dto.DecodeValidate(w, r, &body); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	p, err := h.projects.Duplicate(r.Context(), org, pid, userID, body.Name, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, p)
}

// Deploy POST /orgs/{orgID}/projects/{projectID}/deploy
func (h *Handlers) Deploy(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in deployment.DeployInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	d, err := h.deployments.Deploy(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, d)
}

// Rollback POST /orgs/{orgID}/projects/{projectID}/rollback
func (h *Handlers) Rollback(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var body struct {
		DeploymentID string `json:"deployment_id" validate:"required,uuid"`
	}
	if err := dto.DecodeValidate(w, r, &body); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	d, err := h.deployments.Rollback(r.Context(), org, pid, body.DeploymentID, userID, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, d)
}

// ListDeployments GET /orgs/{orgID}/projects/{projectID}/deployments
func (h *Handlers) ListDeployments(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	limit := 50
	if n, ok := httpx.QueryInt(r, "limit"); ok {
		limit = n
	}
	items, err := h.deployments.ListHistory(r.Context(), org, pid, limit)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, items)
}

// GetDeployment GET /orgs/{orgID}/deployments/{deploymentID}
func (h *Handlers) GetDeployment(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	d, err := h.deployments.Get(r.Context(), org, depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, d)
}

// GetLatestDeployment GET /orgs/{orgID}/projects/{projectID}/deployments/latest
func (h *Handlers) GetLatestDeployment(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	d, err := h.deployments.GetLatest(r.Context(), org, pid)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, d)
}

// DeploymentTimeline GET /orgs/{orgID}/deployments/{deploymentID}/timeline
func (h *Handlers) DeploymentTimeline(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	depID, _ := httpx.RequirePathParam(r, "deploymentID")
	events, err := h.deployments.Timeline(r.Context(), org, depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, events)
}

// CancelDeployment POST /orgs/{orgID}/deployments/{deploymentID}/cancel
func (h *Handlers) CancelDeployment(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	depID, _ := httpx.RequirePathParam(r, "deploymentID")
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	d, err := h.deployments.Cancel(r.Context(), org, depID, userID, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, d)
}

// ConnectRepository POST /orgs/{orgID}/projects/{projectID}/repository
func (h *Handlers) ConnectRepository(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in gitrepo.ConnectInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	repo, err := h.git.Connect(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, repo)
}

// GetRepository GET /orgs/{orgID}/projects/{projectID}/repository
func (h *Handlers) GetRepository(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	repo, err := h.git.Get(r.Context(), org, pid)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, repo)
}

// UpdateRepository PATCH /orgs/{orgID}/projects/{projectID}/repository
func (h *Handlers) UpdateRepository(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in gitrepo.UpdateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	repo, err := h.git.Update(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, repo)
}

// DisconnectRepository DELETE /orgs/{orgID}/projects/{projectID}/repository
func (h *Handlers) DisconnectRepository(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.git.Disconnect(r.Context(), org, pid, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListEnv GET /orgs/{orgID}/projects/{projectID}/env
func (h *Handlers) ListEnv(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	env := envvar.Scope(httpx.QueryString(r, "environment"))
	items, err := h.envVars.List(r.Context(), org, pid, env)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, items)
}

// CreateEnv POST /orgs/{orgID}/projects/{projectID}/env
func (h *Handlers) CreateEnv(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in envvar.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	v, err := h.envVars.Create(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, v)
}

// UpdateEnv PATCH /orgs/{orgID}/projects/{projectID}/env/{envID}
func (h *Handlers) UpdateEnv(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	envID, _ := httpx.RequirePathParam(r, "envID")
	userID, _ := tenant.RequireUser(r.Context())
	var in envvar.UpdateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	v, err := h.envVars.Update(r.Context(), org, pid, envID, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, v)
}

// DeleteEnv DELETE /orgs/{orgID}/projects/{projectID}/env/{envID}
func (h *Handlers) DeleteEnv(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	envID, _ := httpx.RequirePathParam(r, "envID")
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.envVars.Delete(r.Context(), org, pid, envID, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListSecrets GET /orgs/{orgID}/projects/{projectID}/secrets
func (h *Handlers) ListSecrets(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	items, err := h.secrets.List(r.Context(), org, pid)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, items)
}

// CreateSecret POST /orgs/{orgID}/projects/{projectID}/secrets
func (h *Handlers) CreateSecret(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in secret.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	s, err := h.secrets.Create(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, s)
}

// RotateSecret POST /orgs/{orgID}/projects/{projectID}/secrets/{secretID}/rotate
func (h *Handlers) RotateSecret(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	secretID, _ := httpx.RequirePathParam(r, "secretID")
	userID, _ := tenant.RequireUser(r.Context())
	var body struct {
		Value string `json:"value" validate:"required,secret"`
	}
	if err := dto.DecodeValidate(w, r, &body); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	s, err := h.secrets.Rotate(r.Context(), org, pid, secretID, userID, body.Value, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, s)
}

// DeleteSecret DELETE /orgs/{orgID}/projects/{projectID}/secrets/{secretID}
func (h *Handlers) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	secretID, _ := httpx.RequirePathParam(r, "secretID")
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.secrets.Delete(r.Context(), org, pid, secretID, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListDomains GET /orgs/{orgID}/projects/{projectID}/domains
func (h *Handlers) ListDomains(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	items, err := h.domains.List(r.Context(), org, pid)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, items)
}

// CreateDomain POST /orgs/{orgID}/projects/{projectID}/domains
func (h *Handlers) CreateDomain(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	var in domain.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	d, err := h.domains.Create(r.Context(), org, pid, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, d)
}

// DeleteDomain DELETE /orgs/{orgID}/projects/{projectID}/domains/{domainID}
func (h *Handlers) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	domainID, _ := httpx.RequirePathParam(r, "domainID")
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.domains.Delete(r.Context(), org, pid, domainID, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// PauseProject POST /orgs/{orgID}/projects/{projectID}/pause
func (h *Handlers) PauseProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.deployments.Pause(r.Context(), org, pid, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ResumeProject POST /orgs/{orgID}/projects/{projectID}/resume
func (h *Handlers) ResumeProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.deployments.Resume(r.Context(), org, pid, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// RestartProject POST /orgs/{orgID}/projects/{projectID}/restart
func (h *Handlers) RestartProject(w http.ResponseWriter, r *http.Request) {
	org, _ := orgID(r)
	pid, _ := projectID(r)
	userID, _ := tenant.RequireUser(r.Context())
	ip, ua := clientMeta(r)
	if err := h.deployments.Restart(r.Context(), org, pid, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}
