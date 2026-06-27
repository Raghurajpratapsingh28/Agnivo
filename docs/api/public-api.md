# Public API Reference

The public API is served by the `api` executable on port 8080 (configurable).

## Base URLs

| Surface | Prefix | Auth |
|---------|--------|------|
| Dashboard | `/api/v1` | JWT Bearer or API key |
| CLI | `/cli/v1` | Planned |
| Streaming | `/stream/v1` | JWT Bearer (required) |
| Webhooks | `/webhooks` | Provider HMAC |
| Internal (API host) | `/internal/v1` | Service bearer token |

Admin endpoints (health, metrics) run on port **9090**, not 8080.

## Response Format

All `/api/v1` responses use the standard envelope:

```json
{ "data": { }, "meta": { } }
```

Errors:

```json
{ "error": { "code": "validation", "message": "...", "details": { } } }
```

See [Frontend Integration Guide](../apps/api/docs/frontend-integration.md) for full error codes and TypeScript examples.

---

## Authentication (`/api/v1/auth`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/register` | Public | Create account |
| POST | `/auth/login` | Public | Login, receive tokens |
| POST | `/auth/refresh` | Public | Refresh access token |
| POST | `/auth/logout` | Public | Invalidate session |
| POST | `/auth/verify-email` | Public | Verify email address |
| POST | `/auth/password-reset` | Public | Request password reset |
| POST | `/auth/password-reset/confirm` | Public | Confirm password reset |
| GET | `/auth/me` | JWT | Current user profile |
| PATCH | `/auth/me` | JWT | Update profile |
| POST | `/auth/change-password` | JWT | Change password |
| POST | `/auth/invitations/accept` | JWT | Accept org invitation |
| GET | `/auth/sessions` | JWT | List active sessions |
| DELETE | `/auth/sessions/{sessionID}` | JWT | Revoke session |
| POST | `/auth/logout-all` | JWT | Revoke all sessions |

### Auth Headers

```
Authorization: Bearer <access_token>
Content-Type: application/json
```

API keys and PATs also use `Authorization: Bearer agn_...` or `pat_...`.

---

## Organizations (`/api/v1/orgs`)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| POST | `/orgs` | Authenticated | Create organization |
| GET | `/orgs` | Authenticated | List user's organizations |
| GET | `/orgs/{orgID}` | Member | Get organization |
| PATCH | `/orgs/{orgID}` | `org:write` | Update organization |
| DELETE | `/orgs/{orgID}` | `org:delete` | Delete organization |

### Members

| Method | Path | Permission |
|--------|------|------------|
| GET | `/orgs/{orgID}/members` | `member:read` |
| POST | `/orgs/{orgID}/members` | `member:invite` |
| DELETE | `/orgs/{orgID}/members/{userID}` | `member:remove` |
| PATCH | `/orgs/{orgID}/members/{userID}` | `member:manage` |

### API Keys

| Method | Path | Permission |
|--------|------|------------|
| GET | `/orgs/{orgID}/api-keys` | `api_key:read` |
| POST | `/orgs/{orgID}/api-keys` | `api_key:manage` |
| POST | `/orgs/{orgID}/api-keys/{keyID}/rotate` | `api_key:manage` |
| DELETE | `/orgs/{orgID}/api-keys/{keyID}` | `api_key:manage` |

---

## Projects (`/api/v1/orgs/{orgID}/projects`)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| GET | `/projects` | `project:read` | List projects |
| POST | `/projects` | `project:write` | Create project |
| GET | `/projects/{projectID}` | `project:read` | Get project |
| PATCH | `/projects/{projectID}` | `project:write` | Update project |
| DELETE | `/projects/{projectID}` | `project:write` | Delete project |
| POST | `/projects/{projectID}/archive` | `project:write` | Archive project |
| POST | `/projects/{projectID}/restore` | `project:write` | Restore archived project |
| POST | `/projects/{projectID}/duplicate` | `project:write` | Duplicate project |
| POST | `/projects/{projectID}/pause` | `deploy:write` | Pause deployments |
| POST | `/projects/{projectID}/resume` | `deploy:write` | Resume deployments |
| POST | `/projects/{projectID}/restart` | `deploy:write` | Restart latest deployment |

---

## Deployments

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| POST | `/projects/{projectID}/deploy` | `deploy:write` | Trigger deployment |
| POST | `/projects/{projectID}/rollback` | `deploy:write` | Rollback deployment |
| GET | `/projects/{projectID}/deployments` | `deploy:read` | List deployments |
| GET | `/projects/{projectID}/deployments/latest` | `deploy:read` | Latest deployment |
| GET | `/deployments/{deploymentID}` | `deploy:read` | Get deployment |
| GET | `/deployments/{deploymentID}/timeline` | `deploy:read` | Deployment events |
| POST | `/deployments/{deploymentID}/cancel` | `deploy:write` | Cancel deployment |

---

## Repository

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/repository` | `project:read` |
| POST | `/projects/{projectID}/repository` | `project:write` |
| PATCH | `/projects/{projectID}/repository` | `project:write` |
| DELETE | `/projects/{projectID}/repository` | `project:write` |

---

## Environment Variables

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/env` | `project:read` |
| POST | `/projects/{projectID}/env` | `project:write` |
| PATCH | `/projects/{projectID}/env/{envID}` | `project:write` |
| DELETE | `/projects/{projectID}/env/{envID}` | `project:write` |

---

## Secrets

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/secrets` | `project:read` |
| POST | `/projects/{projectID}/secrets` | `project:write` |
| POST | `/projects/{projectID}/secrets/{secretID}/rotate` | `project:write` |
| DELETE | `/projects/{projectID}/secrets/{secretID}` | `project:write` |

---

## Domains

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/domains` | `project:read` |
| POST | `/projects/{projectID}/domains` | `project:write` |
| DELETE | `/projects/{projectID}/domains/{domainID}` | `project:write` |

---

## Webhooks (`/webhooks`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/webhooks/github` | HMAC-SHA256 | GitHub push events |
| POST | `/webhooks/gitlab` | Token header | GitLab push events |
| POST | `/webhooks/bitbucket` | HMAC-SHA256 | Bitbucket push events |

---

## Streaming (`/stream/v1`)

Requires JWT authentication. Planned endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/stream/v1/logs` | SSE log stream |
| GET | `/stream/v1/builds/{buildID}` | Build progress stream |
| GET | `/stream/v1/deployments/{deploymentID}` | Deployment events |
| GET | `/stream/v1/metrics` | Runtime metrics stream |
| GET | `/stream/v1/notifications/ws` | WebSocket notifications |

---

## RBAC Permissions

| Permission | Scope |
|------------|-------|
| `org:write`, `org:delete` | Organization management |
| `member:read`, `member:invite`, `member:remove`, `member:manage` | Membership |
| `api_key:read`, `api_key:manage` | API keys |
| `project:read`, `project:write` | Projects |
| `deploy:read`, `deploy:write` | Deployments |

Roles (`owner`, `admin`, `member`, `viewer`) map to permission sets in `packages/application/identity/rbac`.

## OpenAPI

Machine-readable spec: [openapi.yaml](openapi.yaml)
