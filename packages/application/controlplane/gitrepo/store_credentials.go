package gitrepo

import (
	"context"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

// GetWithCredentials returns a repository including encrypted credential blobs for worker use.
func (st *Store) GetWithCredentials(ctx context.Context, orgID, projectID string) (Repository, error) {
	const q = `SELECT id, project_id, org_id, provider, repo_url, clone_url, default_branch, is_private,
		access_token_enc, deploy_key_enc, metadata, connected_at, updated_at, disconnected_at
		FROM controlplane_git_repositories
		WHERE project_id=$1 AND org_id=$2 AND disconnected_at IS NULL LIMIT 1`
	row := st.db.Conn(ctx).QueryRow(ctx, q, projectID, orgID)
	var r Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.OrgID, &r.Provider, &r.RepoURL, &r.CloneURL,
		&r.DefaultBranch, &r.IsPrivate, &r.AccessTokenEnc, &r.DeployKeyEnc,
		&r.Metadata, &r.ConnectedAt, &r.UpdatedAt, &r.DisconnectedAt)
	if err != nil {
		return Repository{}, postgres.Translate(err, "gitrepo: get with credentials")
	}
	return r, nil
}
