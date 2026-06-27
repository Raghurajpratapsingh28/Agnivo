package project_test

import (
	"context"
	"testing"

	"github.com/agnivo/agnivo/packages/application/controlplane"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/controlplane/project"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectCreateIntegration(t *testing.T) {
	ctx := context.Background()
	db := testkit.ConnectPostgres(t)
	testkit.RunMigrations(t, db, controlplane.Migrations()...)

	repo := project.NewRepository(db)
	bus := events.NewInMemory(ctx, events.Config{})
	pub := cpevents.NewPublisher(bus, "test")
	svc := project.NewService(repo, audit.NewLogger(db), pub, "us-east-1")

	orgID := testkit.RandomSlug()
	userID := testkit.RandomSlug()
	ctx = tenant.WithOrg(ctx, orgID)
	ctx = tenant.WithUser(ctx, userID)
	ctx = tenant.WithMemberRole(ctx, string(rbac.RoleOwner))

	p, err := svc.Create(ctx, orgID, userID, project.CreateInput{
		Name: "My App", Description: "test project",
	}, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, "my-app", p.Slug)

	list, err := svc.List(ctx, orgID, false)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
