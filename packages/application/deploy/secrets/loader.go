package secrets

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/controlplane/crypto"
	"github.com/agnivo/agnivo/packages/application/controlplane/envvar"
	"github.com/agnivo/agnivo/packages/application/controlplane/secret"
	"github.com/agnivo/agnivo/packages/application/deploy/model"
)

// Loader decrypts env vars and secrets for runtime injection.
type Loader struct {
	envRepo    *envvar.Repository
	secretRepo *secret.Repository
	vault      *crypto.Vault
}

// NewLoader constructs a secret loader.
func NewLoader(envRepo *envvar.Repository, secretRepo *secret.Repository, vault *crypto.Vault) *Loader {
	return &Loader{envRepo: envRepo, secretRepo: secretRepo, vault: vault}
}

// LoadRuntimeConfig loads env and secrets for a deployment environment.
func (l *Loader) LoadRuntimeConfig(ctx context.Context, orgID, projectID, environment string) (model.RuntimeConfig, error) {
	cfg := model.RuntimeConfig{
		Env: make(map[string]string), Secrets: make(map[string]string),
		Labels: make(map[string]string), Annotations: make(map[string]string),
	}
	if l.vault == nil {
		return cfg, nil
	}
	aad := crypto.AAD(orgID, projectID)

	if l.envRepo != nil {
		vars, err := l.envRepo.List(ctx, orgID, projectID, envvar.Scope(environment))
		if err != nil {
			return cfg, err
		}
		for _, v := range vars {
			plain, err := l.vault.Decrypt(v.ValueEnc, aad)
			if err != nil {
				continue
			}
			val := string(plain)
			if v.IsSecret {
				cfg.Secrets[v.Key] = val
			} else {
				cfg.Env[v.Key] = val
			}
		}
	}

	if l.secretRepo != nil {
		secrets, err := l.secretRepo.List(ctx, orgID, projectID)
		if err != nil {
			return cfg, err
		}
		for _, s := range secrets {
			if s.Environment != environment && s.Environment != "" {
				continue
			}
			if s.DisabledAt != nil {
				continue
			}
			plain, err := l.vault.Decrypt(s.ValueEnc, aad)
			if err != nil {
				continue
			}
			cfg.Secrets[s.Name] = string(plain)
		}
	}

	cfg.Labels["agnivo.org_id"] = orgID
	cfg.Labels["agnivo.project_id"] = projectID
	cfg.Annotations["agnivo.managed"] = "true"
	return cfg, nil
}

// MaskEnv returns env map safe for logging (secrets redacted).
func MaskEnv(cfg model.RuntimeConfig) map[string]string {
	out := make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		out[k] = v
	}
	for k := range cfg.Secrets {
		out[k] = "[REDACTED]"
	}
	return out
}
