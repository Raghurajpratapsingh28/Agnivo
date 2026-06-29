package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/gitrepo"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Credentials holds decrypted repository auth (never log these fields).
type Credentials struct {
	Provider    gitrepo.Provider
	CloneURL    string
	Branch      string
	CommitSHA   string
	AccessToken string
	DeployKey   string
	IsPrivate   bool
}

// CloneResult is the outcome of a repository checkout.
type CloneResult struct {
	WorkspaceDir string
	CommitSHA    string
	Branch       string
	Duration     time.Duration
}

// Manager handles git clone and checkout operations.
type Manager struct {
	workspaceRoot string
}

// NewManager constructs a git manager.
func NewManager(workspaceRoot string) *Manager {
	return &Manager{workspaceRoot: workspaceRoot}
}

// Clone checks out source code into an ephemeral workspace.
func (m *Manager) Clone(ctx context.Context, creds Credentials, deploymentID string) (CloneResult, error) {
	start := time.Now()
	dir := filepath.Join(m.workspaceRoot, deploymentID)
	if err := os.RemoveAll(dir); err != nil {
		return CloneResult{}, errors.Wrap(err, errors.CodeInternal, "git: cleanup workspace")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return CloneResult{}, errors.Wrap(err, errors.CodeInternal, "git: create workspace")
	}

	cloneURL := injectToken(creds.CloneURL, creds.Provider, creds.AccessToken)

	var cleanup []func()
	defer func() {
		for _, fn := range cleanup {
			fn()
		}
	}()

	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0")

	if creds.DeployKey != "" {
		f, err := writeTempKey(creds.DeployKey)
		if err != nil {
			return CloneResult{}, err
		}
		cleanup = append(cleanup, func() { _ = os.Remove(f) })
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", f))
	}

	depth := "--depth=1"
	args := []string{"clone", depth, "--single-branch"}
	if creds.Branch != "" {
		args = append(args, "--branch", creds.Branch)
	}
	args = append(args, cloneURL, dir)

	if err := runGit(ctx, env, args...); err != nil {
		return CloneResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "git: clone failed")
	}

	sha := creds.CommitSHA
	if sha != "" {
		if err := runGit(ctx, env, "-C", dir, "fetch", "origin", sha); err == nil {
			if err := runGit(ctx, env, "-C", dir, "checkout", sha); err != nil {
				return CloneResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "git: checkout commit failed")
			}
		} else if err := runGit(ctx, env, "-C", dir, "checkout", sha); err != nil {
			return CloneResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "git: checkout commit failed")
		}
	} else {
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
		if err == nil {
			sha = strings.TrimSpace(string(out))
		}
	}

	branch := creds.Branch
	if branch == "" {
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	return CloneResult{
		WorkspaceDir: dir,
		CommitSHA:    sha,
		Branch:       branch,
		Duration:     time.Since(start),
	}, nil
}

// Cleanup removes a workspace directory.
func (m *Manager) Cleanup(dir string) error {
	if dir == "" || !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(m.workspaceRoot)) {
		return nil
	}
	return os.RemoveAll(dir)
}

// DecryptCredentials decrypts stored repository credentials.
func DecryptCredentials(vault *crypto.Vault, orgID, projectID string, repo gitrepo.Repository) (Credentials, error) {
	aad := crypto.AAD(orgID, projectID)
	var token, key string
	if len(repo.AccessTokenEnc) > 0 {
		plain, err := vault.Decrypt(repo.AccessTokenEnc, aad)
		if err != nil {
			return Credentials{}, err
		}
		token = string(plain)
	}
	if len(repo.DeployKeyEnc) > 0 {
		plain, err := vault.Decrypt(repo.DeployKeyEnc, aad)
		if err != nil {
			return Credentials{}, err
		}
		key = string(plain)
	}
	cloneURL := repo.CloneURL
	if cloneURL == "" {
		cloneURL = repo.RepoURL
	}
	return Credentials{
		Provider: repo.Provider, CloneURL: cloneURL, Branch: repo.DefaultBranch,
		AccessToken: token, DeployKey: key, IsPrivate: repo.IsPrivate,
	}, nil
}

func injectToken(cloneURL string, provider gitrepo.Provider, token string) string {
	if token == "" || strings.Contains(cloneURL, "@") {
		return cloneURL
	}
	if !strings.HasPrefix(cloneURL, "https://") {
		return cloneURL
	}
	rest := strings.TrimPrefix(cloneURL, "https://")
	switch provider {
	case gitrepo.ProviderGitHub:
		return "https://x-access-token:" + token + "@" + rest
	case gitrepo.ProviderGitLab:
		return "https://oauth2:" + token + "@" + rest
	case gitrepo.ProviderBitbucket:
		return "https://x-token-auth:" + token + "@" + rest
	default:
		return "https://" + token + "@" + rest
	}
}

func writeTempKey(pem string) (string, error) {
	f, err := os.CreateTemp("", "agnivo-deploy-key-*")
	if err != nil {
		return "", errors.Wrap(err, errors.CodeInternal, "git: temp key")
	}
	if _, err := f.WriteString(pem); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	_ = f.Close()
	return f.Name(), nil
}

func runGit(ctx context.Context, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// ResolveCloneURL picks the best clone URL from repository metadata.
func ResolveCloneURL(repo gitrepo.Repository) string {
	if repo.CloneURL != "" {
		return repo.CloneURL
	}
	return repo.RepoURL
}
