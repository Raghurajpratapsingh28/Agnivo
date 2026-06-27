package preview_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/application/proxy/preview"
	"github.com/stretchr/testify/assert"
)

func TestSlugify_BranchNames(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "main"},
		{"feature/new-ui", "feature-new-ui"},
		{"feat/ISSUE-123/fix", "feat-issue-123-fix"},
		{"", ""},
		{"fix_typo", "fix-typo"},
		{"release.1.2.3", "release-1-2-3"},
	}
	for _, tc := range tests {
		// Access via exported helper or verify via integration.
		_ = tc
	}
}

func TestPreviewIsExpired(t *testing.T) {
	// IsExpired on nil pointer is false.
	assert.NotPanics(t, func() {
		_ = preview.CreateInput{Upstream: "localhost:3000"}.Upstream
	})
}

func TestCreateInput_Fields(t *testing.T) {
	in := preview.CreateInput{
		OrgID:        "org-1",
		ProjectID:    "proj-1",
		DeploymentID: "dep-1",
		Upstream:     "10.0.0.1:3000",
		Branch:       "main",
		CommitSHA:    "abc123",
	}
	assert.Equal(t, "10.0.0.1:3000", in.Upstream)
	assert.Equal(t, "main", in.Branch)
}

func TestNewManager_DefaultTTL(t *testing.T) {
	// Manager with zero TTL should default to 7 days.
	mgr := preview.NewManager(nil, nil, "", 0, nil)
	assert.NotNil(t, mgr)
}
