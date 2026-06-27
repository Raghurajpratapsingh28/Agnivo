package member

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Status is membership state.
type Status string

const (
	StatusActive  Status = "active"
	StatusInvited Status = "invited"
	StatusRemoved Status = "removed"
)

// Member is an organization membership.
type Member struct {
	ID        string     `db:"id" json:"id"`
	OrgID     string     `db:"org_id" json:"org_id"`
	UserID    string     `db:"user_id" json:"user_id"`
	Role      rbac.Role  `db:"role" json:"role"`
	Status    Status     `db:"status" json:"status"`
	InvitedBy *string    `db:"invited_by" json:"invited_by,omitempty"`
	InvitedAt *time.Time `db:"invited_at" json:"invited_at,omitempty"`
	JoinedAt  *time.Time `db:"joined_at" json:"joined_at,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
}

// Invitation is a pending org invite.
type Invitation struct {
	ID         string     `db:"id"`
	OrgID      string     `db:"org_id"`
	Email      string     `db:"email"`
	Role       rbac.Role  `db:"role"`
	TokenHash  string     `db:"token_hash"`
	InvitedBy  string     `db:"invited_by"`
	ExpiresAt  time.Time  `db:"expires_at"`
	AcceptedAt *time.Time `db:"accepted_at"`
	RejectedAt *time.Time `db:"rejected_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

// Repository persists memberships and invitations.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a member repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// AddOwner creates the founding owner membership.
func (r *Repository) AddOwner(ctx context.Context, orgID, userID string) (Member, error) {
	now := time.Now().UTC()
	const q = `INSERT INTO identity_members (id, org_id, user_id, role, status, joined_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'active',$5,$5,$5)
		RETURNING id, org_id, user_id, role, status, invited_by, invited_at, joined_at, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q, idx.NewUUID(), orgID, userID, rbac.RoleOwner, now)
	return scanMember(row)
}

// GetActive fetches an active membership for user in org.
func (r *Repository) GetActive(ctx context.Context, orgID, userID string) (Member, error) {
	const q = `SELECT id, org_id, user_id, role, status, invited_by, invited_at, joined_at, created_at, updated_at
		FROM identity_members WHERE org_id=$1 AND user_id=$2 AND status='active' LIMIT 1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, orgID, userID)
	m, err := scanMember(row)
	if err != nil {
		return Member{}, postgres.Translate(err, "member: get")
	}
	return m, nil
}

// ListByOrg returns active members of an organization.
func (r *Repository) ListByOrg(ctx context.Context, orgID string) ([]Member, error) {
	const q = `SELECT id, org_id, user_id, role, status, invited_by, invited_at, joined_at, created_at, updated_at
		FROM identity_members WHERE org_id=$1 AND status='active' ORDER BY joined_at`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, postgres.Translate(err, "member: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Member])
}

// UpdateRole changes a member's role.
func (r *Repository) UpdateRole(ctx context.Context, orgID, userID string, role rbac.Role) error {
	const q = `UPDATE identity_members SET role=$3, updated_at=now()
		WHERE org_id=$1 AND user_id=$2 AND status='active'`
	tag, err := r.db.Conn(ctx).Exec(ctx, q, orgID, userID, role)
	if err != nil {
		return postgres.Translate(err, "member: update role")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("member not found")
	}
	return nil
}

// Remove soft-removes a member.
func (r *Repository) Remove(ctx context.Context, orgID, userID string) error {
	const q = `UPDATE identity_members SET status='removed', updated_at=now()
		WHERE org_id=$1 AND user_id=$2 AND status='active'`
	tag, err := r.db.Conn(ctx).Exec(ctx, q, orgID, userID)
	if err != nil {
		return postgres.Translate(err, "member: remove")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("member not found")
	}
	return nil
}

// CreateInvitation stores a pending invitation.
func (r *Repository) CreateInvitation(ctx context.Context, orgID, email string, role rbac.Role, tokenHash, invitedBy string, expiresAt time.Time) (Invitation, error) {
	const q = `INSERT INTO identity_invitations (id, org_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		RETURNING id, org_id, email, role, token_hash, invited_by, expires_at, accepted_at, rejected_at, created_at`
	var inv Invitation
	row := r.db.Conn(ctx).QueryRow(ctx, q, idx.NewUUID(), orgID, email, role, tokenHash, invitedBy, expiresAt)
	err := row.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.RejectedAt, &inv.CreatedAt)
	if err != nil {
		return Invitation{}, postgres.Translate(err, "member: create invitation")
	}
	return inv, nil
}

// AcceptInvitation marks invitation accepted and creates membership.
func (r *Repository) AcceptInvitation(ctx context.Context, inv Invitation, userID string) (Member, error) {
	var member Member
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		now := time.Now().UTC()
		tag, err := r.db.Conn(ctx).Exec(ctx,
			`UPDATE identity_invitations SET accepted_at=$2 WHERE id=$1 AND accepted_at IS NULL`, inv.ID, now)
		if err != nil {
			return postgres.Translate(err, "member: accept invitation")
		}
		if tag.RowsAffected() == 0 {
			return errors.Conflict("invitation already used")
		}
		const q = `INSERT INTO identity_members (id, org_id, user_id, role, status, invited_by, invited_at, joined_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,'active',$5,$6,$6,$6,$6)
			ON CONFLICT (org_id, user_id) DO UPDATE SET role=EXCLUDED.role, status='active', updated_at=$6
			RETURNING id, org_id, user_id, role, status, invited_by, invited_at, joined_at, created_at, updated_at`
		row := r.db.Conn(ctx).QueryRow(ctx, q, idx.NewUUID(), inv.OrgID, userID, inv.Role, inv.InvitedBy, now)
		member, err = scanMember(row)
		return postgres.Translate(err, "member: accept invitation")
	})
	return member, err
}

// GetInvitationByTokenHash finds a pending invitation by token hash.
func (r *Repository) GetInvitationByTokenHash(ctx context.Context, hash string) (Invitation, error) {
	const q = `SELECT id, org_id, email, role, token_hash, invited_by, expires_at, accepted_at, rejected_at, created_at
		FROM identity_invitations
		WHERE token_hash=$1 AND accepted_at IS NULL AND rejected_at IS NULL AND expires_at > now() LIMIT 1`
	var inv Invitation
	row := r.db.Conn(ctx).QueryRow(ctx, q, hash)
	err := row.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.RejectedAt, &inv.CreatedAt)
	if err != nil {
		return Invitation{}, postgres.Translate(err, "member: get invitation")
	}
	return inv, nil
}

// RejectInvitation marks an invitation rejected.
func (r *Repository) RejectInvitation(ctx context.Context, invID string) error {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE identity_invitations SET rejected_at=now() WHERE id=$1 AND accepted_at IS NULL AND rejected_at IS NULL`, invID)
	if err != nil {
		return postgres.Translate(err, "member: reject invitation")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("invitation not found")
	}
	return nil
}

// ListPendingInvitations returns pending invitations for an org.
func (r *Repository) ListPendingInvitations(ctx context.Context, orgID string) ([]Invitation, error) {
	const q = `SELECT id, org_id, email, role, token_hash, invited_by, expires_at, accepted_at, rejected_at, created_at
		FROM identity_invitations
		WHERE org_id=$1 AND accepted_at IS NULL AND rejected_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, postgres.Translate(err, "member: list invitations")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Invitation])
}

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.InvitedAt, &m.JoinedAt, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}
