package http

import (
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/go-chi/chi/v5"
)

// Mount registers all identity REST routes.
func Mount(r chi.Router, h *Handlers, mw *Middleware) {
	// Public auth routes.
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.Post("/verify-email", h.VerifyEmail)
		r.Post("/password-reset", h.RequestPasswordReset)
		r.Post("/password-reset/confirm", h.ResetPassword)

		r.Group(func(r chi.Router) {
			r.Use(mw.Authenticate)
			r.Use(mw.RequireAuth)
			r.Get("/me", h.Me)
			r.Patch("/me", h.UpdateProfile)
			r.Post("/change-password", h.ChangePassword)
			r.Post("/invitations/accept", h.AcceptInvitation)
			r.Get("/sessions", h.ListSessions)
			r.Delete("/sessions/{sessionID}", h.RevokeSession)
			r.Post("/logout-all", h.LogoutAll)
		})
	})

	// Organization routes.
	r.Route("/orgs", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(mw.Authenticate)
			r.Use(mw.RequireAuth)
			r.Post("/", h.CreateOrg)
			r.Get("/", h.ListOrgs)
		})

		r.Route("/{orgID}", func(r chi.Router) {
			r.Use(mw.Authenticate)
			r.Use(mw.RequireAuth)
			r.Use(mw.OrgContext)

			r.Get("/", h.GetOrg)
			r.With(mw.RequirePermission(rbac.PermOrgWrite)).Patch("/", h.UpdateOrg)
			r.With(mw.RequirePermission(rbac.PermOrgDelete)).Delete("/", h.DeleteOrg)

			r.Route("/members", func(r chi.Router) {
				r.With(mw.RequirePermission(rbac.PermMemberRead)).Get("/", h.ListMembers)
				r.With(mw.RequirePermission(rbac.PermMemberInvite)).Post("/", h.InviteMember)
				r.With(mw.RequirePermission(rbac.PermMemberRemove)).Delete("/{userID}", h.RemoveMember)
				r.With(mw.RequirePermission(rbac.PermMemberManage)).Patch("/{userID}", h.UpdateMemberRole)
			})

			r.Route("/api-keys", func(r chi.Router) {
				r.With(mw.RequirePermission(rbac.PermAPIKeyRead)).Get("/", h.ListAPIKeys)
				r.With(mw.RequirePermission(rbac.PermAPIKeyManage)).Post("/", h.CreateAPIKey)
				r.With(mw.RequirePermission(rbac.PermAPIKeyManage)).Post("/{keyID}/rotate", h.RotateAPIKey)
				r.With(mw.RequirePermission(rbac.PermAPIKeyManage)).Delete("/{keyID}", h.DeleteAPIKey)
			})
		})
	})
}
