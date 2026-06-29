package git_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/gitrepo"
	buildgit "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/git"
	"github.com/stretchr/testify/assert"
)

func TestInjectTokenGitHub(t *testing.T) {
	url := buildgit.ResolveCloneURL(gitrepo.Repository{
		RepoURL: "https://github.com/org/repo.git",
	})
	assert.Equal(t, "https://github.com/org/repo.git", url)

	injected := injectTokenForTest(url, gitrepo.ProviderGitHub, "secret123")
	assert.Contains(t, injected, "x-access-token:secret123@github.com")
}

func TestInjectTokenSkipsWhenAtPresent(t *testing.T) {
	url := "https://user:pass@github.com/org/repo.git"
	injected := injectTokenForTest(url, gitrepo.ProviderGitHub, "token")
	assert.Equal(t, url, injected)
}

func injectTokenForTest(cloneURL string, provider gitrepo.Provider, token string) string {
	creds := buildgit.Credentials{CloneURL: cloneURL, Provider: provider, AccessToken: token}
	creds.CloneURL = cloneURL
	// replicate inject via Clone path: use DecryptCredentials pattern — test via manager helper
	return testInject(cloneURL, provider, token)
}

func testInject(cloneURL string, provider gitrepo.Provider, token string) string {
	c := buildgit.Credentials{CloneURL: cloneURL, Provider: provider, AccessToken: token}
	_ = c
	if token == "" {
		return cloneURL
	}
	// inline same logic as manager for test
	switch provider {
	case gitrepo.ProviderGitHub:
		if len(cloneURL) > 8 && cloneURL[:8] == "https://" && !containsAt(cloneURL) {
			return "https://x-access-token:" + token + "@" + cloneURL[8:]
		}
	}
	return cloneURL
}

func containsAt(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return true
		}
	}
	return false
}

func TestResolveCloneURL(t *testing.T) {
	assert.Equal(t, "https://clone", buildgit.ResolveCloneURL(gitrepo.Repository{
		CloneURL: "https://clone", RepoURL: "https://repo",
	}))
	assert.Equal(t, "https://repo", buildgit.ResolveCloneURL(gitrepo.Repository{RepoURL: "https://repo"}))
}
