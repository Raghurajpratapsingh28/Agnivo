# Frontend API Integration Guide

This document describes how a Next.js (or any browser) frontend integrates with the Agnivo dashboard API at `/api/v1`.

For route layout and server architecture, see [architecture.md](./architecture.md).

---

## Quick start

| Item | Value |
|------|-------|
| Base URL (local) | `http://localhost:8080` |
| Dashboard API prefix | `/api/v1` |
| Frontend env var | `NEXT_PUBLIC_API_URL` (defaults to `http://localhost:8080` in Docker) |
| Auth scheme | Bearer JWT (access token) |
| Content type | `application/json` |
| Max request body | 1 MiB |

All dashboard endpoints live under:

```
{NEXT_PUBLIC_API_URL}/api/v1
```

Example:

```
POST http://localhost:8080/api/v1/auth/login
```

Health and metrics run on a separate admin port (default `9090`), not on the public API port.

---

## Response envelope

Every `/api/v1` response uses the same JSON envelope defined in `packages/platform/dto`.

### Success

```json
{
  "data": { },
  "meta": {
    "request_id": "optional-uuid",
    "page": { "page": 1, "page_size": 20, "total_items": 100, "total_pages": 5 },
    "cursor": { "next_cursor": "abc", "has_more": true }
  }
}
```

- Always read the payload from `data`.
- `meta` is optional. Most list endpoints today return a full array in `data` without pagination metadata (see [Pagination](#pagination)).
- `204 No Content` responses have an empty body (logout, deletes).

### Error

```json
{
  "error": {
    "code": "validation",
    "message": "validation failed",
    "details": {
      "fields": [
        { "field": "email", "rule": "email", "message": "must be a valid email" }
      ]
    }
  }
}
```

### Error codes → HTTP status

| Code | HTTP | Typical cause |
|------|------|---------------|
| `invalid_argument` | 400 | Malformed JSON, unknown fields, oversized body |
| `validation` | 422 | Field validation failure |
| `not_found` | 404 | Resource missing |
| `already_exists` | 409 | Duplicate slug, etc. |
| `conflict` | 409 | State conflict |
| `unauthenticated` | 401 | Missing or invalid token |
| `permission_denied` | 403 | RBAC failure or not an org member |
| `rate_limited` | 429 | Login rate limit |
| `failed_precondition` | 412 | e.g. project not active |
| `not_implemented` | 501 | Stub routes (`/cli/v1`, `/stream/v1`) |
| `internal` | 500 | Server error |

Map `422` validation errors to form fields via `error.details.fields`.

---

## Request rules

1. Send `Content-Type: application/json` on POST/PATCH.
2. Bodies must be a **single JSON object**. Unknown fields are rejected.
3. Use **snake_case** field names in request bodies.
4. Path parameters are UUIDs: `orgID`, `projectID`, `deploymentID`, `envID`, `secretID`, `domainID`, `sessionID`, `keyID`, `userID`.

---

## Authentication

The dashboard uses **token-based auth** (no cookie sessions). Store tokens securely on the client.

### Headers

| Header | Required | Purpose |
|--------|----------|---------|
| `Authorization` | Protected routes | `Bearer <access_token>` |
| `Content-Type` | POST/PATCH | `application/json` |
| `X-Request-ID` | Optional | Echoed in `meta.request_id` if supported |
| `X-Correlation-ID` | Optional | Cross-service tracing |

CORS (development) allows origins `*` and headers `Authorization`, `Content-Type`, `X-Request-ID`, `X-Correlation-ID`.

### Token lifecycle

| Token | TTL (default) | Storage recommendation |
|-------|---------------|------------------------|
| Access token (JWT) | 15 minutes | Memory or short-lived storage |
| Refresh token | 30 days | `httpOnly` cookie or secure storage |

**Login**

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword12",
  "remember_me": false,
  "device_name": "Chrome on Mac",
  "org_id": "optional-org-uuid"
}
```

```json
{
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "raw-refresh-token",
    "expires_at": "2026-06-26T12:15:00Z",
    "token_type": "Bearer"
  }
}
```

**Refresh** (rotates the refresh token — always persist the new one)

```http
POST /api/v1/auth/refresh

{ "refresh_token": "..." }
```

**Logout**

```http
POST /api/v1/auth/logout

{ "refresh_token": "..." }
```

Returns `204`.

### 401 handling pattern

1. On `401`, attempt one refresh with the stored refresh token.
2. Retry the original request with the new access token.
3. If refresh fails, clear tokens and redirect to login.

### Optional login context

Pass `org_id` at login to embed an org in the JWT claims. Membership is always re-validated server-side on org-scoped routes.

### Other token types (automation)

The same `Authorization: Bearer` header accepts:

| Prefix | Type | Use |
|--------|------|-----|
| (JWT) | Access token | Dashboard sessions |
| `agn_` | Org API key | Programmatic access |
| `pat_` | Personal access token | User-scoped automation |

The dashboard frontend should use JWT access tokens only.

---

## TypeScript client skeleton

Minimal fetch wrapper for a Next.js app:

```typescript
const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const API_V1 = `${API_BASE}/api/v1`;

type ApiResponse<T> = {
  data?: T;
  meta?: { request_id?: string; page?: unknown; cursor?: unknown };
  error?: { code: string; message: string; details?: unknown };
};

type TokenPair = {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  token_type: "Bearer";
};

class ApiClient {
  private accessToken: string | null = null;
  private refreshToken: string | null = null;

  setTokens(tokens: TokenPair) {
    this.accessToken = tokens.access_token;
    this.refreshToken = tokens.refresh_token;
  }

  clearTokens() {
    this.accessToken = null;
    this.refreshToken = null;
  }

  async request<T>(
    path: string,
    init: RequestInit = {},
    retry = true
  ): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set("Content-Type", "application/json");
    if (this.accessToken) {
      headers.set("Authorization", `Bearer ${this.accessToken}`);
    }

    const res = await fetch(`${API_V1}${path}`, { ...init, headers });
    if (res.status === 204) return undefined as T;

    const body = (await res.json()) as ApiResponse<T>;

    if (res.status === 401 && retry && this.refreshToken) {
      await this.refresh();
      return this.request(path, init, false);
    }

    if (!res.ok || body.error) {
      throw body.error ?? { code: "unknown", message: res.statusText };
    }

    return body.data as T;
  }

  async login(email: string, password: string) {
    const tokens = await this.request<TokenPair>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password, device_name: navigator.userAgent }),
    });
    this.setTokens(tokens);
    return tokens;
  }

  async refresh() {
    const tokens = await this.request<TokenPair>(
      "/auth/refresh",
      {
        method: "POST",
        body: JSON.stringify({ refresh_token: this.refreshToken }),
      },
      false
    );
    this.setTokens(tokens);
  }

  async me() {
    return this.request<PublicUser>("/auth/me");
  }
}

type PublicUser = {
  id: string;
  email: string;
  display_name: string;
  avatar_url: string;
  timezone: string;
  locale: string;
  status: string;
  email_verified: boolean;
  last_login_at: string | null;
  created_at: string;
};
```

For production, persist refresh tokens in an `httpOnly` cookie via a Next.js Route Handler rather than localStorage.

---

## RBAC

Org-scoped routes require active membership. The user's role determines available actions.

### Roles (highest → lowest privilege)

`owner` · `admin` · `developer` · `operator` · `billing` · `viewer`

### Permissions used by the dashboard API

| Permission | Used for |
|------------|----------|
| `org:read` / `org:write` / `org:delete` | Organization CRUD |
| `member:read` / `member:invite` / `member:manage` / `member:remove` | Team management |
| `apikey:read` / `apikey:manage` | API keys |
| `project:read` / `project:write` | Projects, env, secrets, domains, repo |
| `deploy:read` / `deploy:write` | Deployments, pause/resume/restart |

403 responses use code `permission_denied`. Gate UI actions by role or handle 403 gracefully.

**Role → permission summary**

| Role | Projects & deploys | Members | API keys | Org settings |
|------|-------------------|---------|----------|--------------|
| owner | full | full | full | full |
| admin | full | full | full | read/write |
| developer | full | read | read | read |
| operator | read + deploy write | read | — | read |
| billing | — | — | — | billing only |
| viewer | read only | read | — | read |

---

## Endpoints

All paths are relative to `/api/v1`. 🔒 = requires `Authorization: Bearer`.

### Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/register` | — | Create account |
| POST | `/auth/login` | — | Get token pair |
| POST | `/auth/refresh` | — | Rotate tokens |
| POST | `/auth/logout` | — | Revoke session |
| POST | `/auth/verify-email` | — | Verify email token |
| POST | `/auth/password-reset` | — | Request reset (always 204) |
| POST | `/auth/password-reset/confirm` | — | Complete reset |
| GET | `/auth/me` | 🔒 | Current user |
| PATCH | `/auth/me` | 🔒 | Update profile |
| POST | `/auth/change-password` | 🔒 | Change password |
| POST | `/auth/invitations/accept` | 🔒 | Accept org invite |
| GET | `/auth/sessions` | 🔒 | List sessions |
| DELETE | `/auth/sessions/{sessionID}` | 🔒 | Revoke session |
| POST | `/auth/logout-all` | 🔒 | Revoke all sessions |

#### Auth request bodies

**Register**

```json
{
  "email": "user@example.com",
  "password": "securepassword12",
  "display_name": "Jane Doe",
  "org_name": "My Team"
}
```

Password minimum length: 12 characters.

**Update profile** (PATCH — only include fields to change)

```json
{
  "display_name": "Jane Doe",
  "avatar_url": "https://example.com/avatar.png",
  "timezone": "America/New_York",
  "locale": "en-US"
}
```

**Change password**

```json
{
  "current_password": "...",
  "new_password": "newsecurepass12"
}
```

**Verify email / accept invitation**

```json
{ "token": "raw-token-from-email-link" }
```

**Password reset confirm**

```json
{
  "token": "raw-token-from-email-link",
  "new_password": "newsecurepass12"
}
```

### Organizations

Base: `/orgs`

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| POST | `/orgs` | 🔒 | Create org |
| GET | `/orgs` | 🔒 | List user's orgs |
| GET | `/orgs/{orgID}` | member | Get org |
| PATCH | `/orgs/{orgID}` | `org:write` | Update org |
| DELETE | `/orgs/{orgID}` | `org:delete` | Delete org |

**Create org**

```json
{ "name": "Acme Inc", "slug": "acme" }
```

> **Note:** The `Organization` model currently serializes with PascalCase Go field names (`ID`, `Name`, `Slug`) because it lacks `json` tags. Most other resources use snake_case. Handle both or normalize on the client until the API is updated.

### Members & API keys

Base: `/orgs/{orgID}`

| Method | Path | Permission |
|--------|------|------------|
| GET | `/members` | `member:read` |
| POST | `/members` | `member:invite` |
| DELETE | `/members/{userID}` | `member:remove` |
| PATCH | `/members/{userID}` | `member:manage` |
| GET | `/api-keys` | `apikey:read` |
| POST | `/api-keys` | `apikey:manage` |
| POST | `/api-keys/{keyID}/rotate` | `apikey:manage` |
| DELETE | `/api-keys/{keyID}` | `apikey:manage` |

**Invite member**

```json
{ "email": "dev@example.com", "role": "developer" }
```

Valid roles: `owner`, `admin`, `developer`, `operator`, `billing`, `viewer`.

Response includes a one-time invite token:

```json
{
  "data": {
    "invitation": { },
    "token": "raw-invite-token"
  }
}
```

**Create API key**

```json
{
  "name": "CI deploy key",
  "scopes": ["deploy:write"],
  "expires_at": "2027-01-01T00:00:00Z"
}
```

Response includes a one-time secret:

```json
{
  "data": {
    "key": { "id": "...", "prefix": "...", "scopes": ["..."] },
    "secret": "agn_..."
  }
}
```

Show the secret once; it cannot be retrieved again.

### Projects

Base: `/orgs/{orgID}/projects`

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects?include_archived=` | `project:read` |
| POST | `/projects` | `project:write` |
| GET | `/projects/{projectID}` | `project:read` |
| PATCH | `/projects/{projectID}` | `project:write` |
| DELETE | `/projects/{projectID}` | `project:write` |
| POST | `/projects/{projectID}/archive` | `project:write` |
| POST | `/projects/{projectID}/restore` | `project:write` |
| POST | `/projects/{projectID}/duplicate` | `project:write` |
| POST | `/projects/{projectID}/pause` | `deploy:write` |
| POST | `/projects/{projectID}/resume` | `deploy:write` |
| POST | `/projects/{projectID}/restart` | `deploy:write` |

**Create project**

```json
{
  "name": "my-app",
  "slug": "my-app",
  "description": "Production web app",
  "repo_url": "https://github.com/org/repo",
  "repo_provider": "github",
  "branch": "main",
  "default_runtime": "node20",
  "framework": "nextjs",
  "build_method": "dockerfile",
  "region": "us-east-1",
  "visibility": "private",
  "tags": ["production"]
}
```

| Field | Values |
|-------|--------|
| `repo_provider` | `github`, `gitlab`, `bitbucket`, `generic`, `""` |
| `build_method` | `dockerfile` (default), `buildpack`, `nixpacks` |
| `visibility` | `private`, `public` |

**Project status:** `active`, `archived`, `deleted`

### Deployments

| Method | Path | Permission |
|--------|------|------------|
| POST | `/projects/{projectID}/deploy` | `deploy:write` |
| POST | `/projects/{projectID}/rollback` | `deploy:write` |
| GET | `/projects/{projectID}/deployments?limit=` | `deploy:read` |
| GET | `/projects/{projectID}/deployments/latest` | `deploy:read` |
| GET | `/deployments/{deploymentID}` | `deploy:read` |
| GET | `/deployments/{deploymentID}/timeline` | `deploy:read` |
| POST | `/deployments/{deploymentID}/cancel` | `deploy:write` |

**Trigger deploy**

```json
{
  "commit_sha": "abc123",
  "commit_message": "feat: launch",
  "branch": "main",
  "author": "Jane",
  "environment": "production"
}
```

`environment` defaults to `production`. Allowed: `development`, `preview`, `production`.

**Rollback**

```json
{ "deployment_id": "uuid-of-target-deployment" }
```

**Deployment status values**

`pending` → `queued` → `building` → `built` → `scheduling` → `deploying` → `live`

Terminal: `live`, `failed`, `cancelled`, `rolled_back`

Intermediate rollback: `rolling_back`

**Polling for deploy progress** (streaming not yet available):

```typescript
async function pollDeployment(
  client: ApiClient,
  orgId: string,
  deploymentId: string,
  intervalMs = 3000
) {
  const terminal = new Set(["live", "failed", "cancelled", "rolled_back"]);

  for (;;) {
    const deployment = await client.request<Deployment>(
      `/orgs/${orgId}/deployments/${deploymentId}`
    );
    if (terminal.has(deployment.status)) return deployment;
    await new Promise((r) => setTimeout(r, intervalMs));
  }
}
```

Use `GET .../timeline` for a status event log.

List deployments accepts `?limit=N` (default 50). No cursor/page metadata yet.

### Repository

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/repository` | `project:read` |
| POST | `/projects/{projectID}/repository` | `project:write` |
| PATCH | `/projects/{projectID}/repository` | `project:write` |
| DELETE | `/projects/{projectID}/repository` | `project:write` |

**Connect repository**

```json
{
  "provider": "github",
  "repo_url": "https://github.com/org/repo",
  "clone_url": "https://github.com/org/repo.git",
  "default_branch": "main",
  "is_private": true,
  "access_token": "ghp_...",
  "deploy_key": "optional-deploy-key"
}
```

Providers: `github`, `gitlab`, `bitbucket`, `generic`.

### Environment variables

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/env?environment=` | `project:read` |
| POST | `/projects/{projectID}/env` | `project:write` |
| PATCH | `/projects/{projectID}/env/{envID}` | `project:write` |
| DELETE | `/projects/{projectID}/env/{envID}` | `project:write` |

**Create env var**

```json
{
  "key": "DATABASE_URL",
  "value": "postgres://...",
  "environment": "production",
  "is_secret": true
}
```

Secret values are masked in list responses (`value` omitted or redacted).

Filter: `?environment=development|preview|production`

### Secrets

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/secrets` | `project:read` |
| POST | `/projects/{projectID}/secrets` | `project:write` |
| POST | `/projects/{projectID}/secrets/{secretID}/rotate` | `project:write` |
| DELETE | `/projects/{projectID}/secrets/{secretID}` | `project:write` |

**Create secret**

```json
{
  "name": "stripe-key",
  "value": "sk_live_...",
  "environment": "production",
  "metadata": {}
}
```

Plaintext is never returned after creation — only metadata (`id`, `name`, `environment`, `version`, `disabled`).

### Domains

| Method | Path | Permission |
|--------|------|------------|
| GET | `/projects/{projectID}/domains` | `project:read` |
| POST | `/projects/{projectID}/domains` | `project:write` |
| DELETE | `/projects/{projectID}/domains/{domainID}` | `project:write` |

**Add domain**

```json
{
  "hostname": "app.example.com",
  "domain_type": "custom",
  "is_primary": true,
  "is_preview": false,
  "redirect_to": ""
}
```

`domain_type`: `platform`, `custom` (default), `wildcard`, `preview`

---

## Pagination

Platform support exists for offset (`?page=1&page_size=20`) and cursor (`?cursor=&limit=20`) pagination, but **most list handlers return the full array today**.

| Endpoint | Pagination today |
|----------|------------------|
| `GET /orgs` | Full list |
| `GET /orgs/{orgID}/members` | Full list |
| `GET /orgs/{orgID}/api-keys` | Full list |
| `GET /orgs/{orgID}/projects` | Full list; filter `?include_archived=true` |
| `GET .../env`, `.../secrets`, `.../domains` | Full list; env filter `?environment=` |
| `GET .../deployments` | `?limit=N` only (default 50) |

When pagination metadata is added, expect it under `meta.page` or `meta.cursor`.

---

## Live updates (not yet available)

These routes exist but return `501 not_implemented`:

| Path | Intended use |
|------|--------------|
| `GET /stream/v1/logs` | Live build/deploy logs (SSE) |
| `GET /stream/v1/builds/{buildID}` | Build stream |
| `GET /stream/v1/deployments/{deploymentID}` | Deployment progress |
| `GET /stream/v1/metrics` | Live metrics |
| `GET /stream/v1/notifications/ws` | WebSocket notifications |

Until streaming ships, poll deployment status and timeline endpoints.

---

## Suggested app structure

```
apps/web/
├── src/
│   ├── lib/
│   │   ├── api-client.ts      # fetch wrapper + token refresh
│   │   ├── api-types.ts       # TypeScript interfaces from this doc
│   │   └── auth-storage.ts    # refresh token persistence
│   ├── hooks/
│   │   ├── use-auth.ts
│   │   └── use-org.ts         # org context + role checks
│   └── app/
│       ├── (auth)/login/page.tsx
│       └── (dashboard)/orgs/[orgId]/projects/[projectId]/page.tsx
```

### Org context in the UI

1. After login, call `GET /orgs` to populate an org switcher.
2. Store the selected `orgID` in URL (`/orgs/{orgId}/...`) or app state.
3. Prefix all control-plane calls with `/orgs/{orgID}`.
4. Optionally pass `org_id` on login to embed org in the JWT.

### One-time secrets checklist

These values are shown **once** — copy to secure storage immediately:

- API key create/rotate → `data.secret`
- Member invite → `data.token`
- (Email flows return tokens via links, not API list endpoints)

---

## Local development

**Start the API**

```bash
AGNIVO_HTTP_ENABLED=true AGNIVO_DATABASE_ENABLED=true go run ./apps/api/cmd/api
```

PostgreSQL and Redis are required for identity and sessions.

**Frontend env**

```bash
# apps/web/.env.local
NEXT_PUBLIC_API_URL=http://localhost:8080
```

**Docker dev stack**

```bash
cp .env.example .env
pnpm docker:dev:up
```

Compose sets `NEXT_PUBLIC_API_URL=http://localhost:8080` for the web service.

---

## Integration checklist

- [ ] Configure `NEXT_PUBLIC_API_URL`
- [ ] Unwrap all responses from `data`
- [ ] Attach `Authorization: Bearer` on protected routes
- [ ] Implement refresh-on-401 with single retry
- [ ] Map `422` field errors to form validation
- [ ] Handle `403 permission_denied` in UI
- [ ] Use snake_case in request bodies
- [ ] Store refresh token securely (prefer `httpOnly` cookie via Route Handler)
- [ ] Persist one-time secrets (API keys, invite tokens) immediately
- [ ] Poll deployments until terminal status (no streaming yet)
- [ ] Gate actions by org role or tolerate 403 from the API

---

## Related docs

- [API architecture](./architecture.md) — route map and server layout
- [Backend README](../../README.md) — executables, config, platform packages
- [Root README](../../../README.md) — monorepo setup and Docker
