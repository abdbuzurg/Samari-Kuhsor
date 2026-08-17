package http

import (
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Authentication endpoints (docs/03-API-CONTRACT.md §9).
//
// The session cookie itself is set by the Next.js BFF, not here: the cookie lives
// between the browser and Next.js, and this API only ever sees a Bearer token
// (docs/07-IMPLEMENTATION-PLAN.md I8). So login returns the token in the body for
// the BFF to store; it never sets a Set-Cookie header of its own.

// Payload types live in internal/api, which is the single source of truth the
// TypeScript in packages/types is generated from (docs/07 I3). Defining them here
// as well is exactly the drift 03-API-CONTRACT.md:265 warns about.

func identityResponse(ident auth.Identity) api.User {
	roles := make([]string, 0, len(ident.Roles))
	for _, r := range ident.Roles {
		roles = append(roles, r.Key)
	}
	// Normalised through rbac.Set so the list is sorted and deduplicated, and so
	// a malformed row cannot reach the client (docs/03-API-CONTRACT.md:194).
	perms := rbac.NewSet(ident.Permissions).Strings()
	if perms == nil {
		perms = []string{}
	}
	return api.User{
		ID:          ident.User.ID.String(),
		Email:       ident.User.Email,
		FullName:    ident.User.FullName,
		Roles:       roles,
		Permissions: perms,
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}

	var details []common.FieldError
	if strings.TrimSpace(req.Email) == "" {
		details = append(details, common.FieldError{
			Field: "email", Code: "required", Message: "Укажите email",
		})
	}
	if req.Password == "" {
		details = append(details, common.FieldError{
			Field: "password", Code: "required", Message: "Укажите пароль",
		})
	}
	if len(details) > 0 {
		common.Fail(w, r, common.Validation(details...))
		return
	}

	token, ident, err := s.auth.Login(r.Context(), auth.LoginRequest{
		Email:     req.Email,
		Password:  req.Password,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		common.Fail(w, r, loginError(err))
		return
	}

	common.JSON(w, http.StatusOK, api.LoginResponse{Token: token, User: identityResponse(ident)})
}

// loginError collapses the domain's failure modes for the client.
//
// Invalid credentials, an unknown address and a deactivated account all return
// the same 401. Distinguishing them would tell an attacker which addresses exist
// and which are worth pursuing. A lockout is reported distinctly because the user
// needs to know why a correct password stopped working, and the account's
// existence is already established by that point.
func loginError(err error) error {
	switch {
	case errors.Is(err, auth.ErrAccountLocked):
		return common.Newf(common.CodeForbidden,
			"Учётная запись временно заблокирована из-за неудачных попыток входа")
	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrAccountInactive):
		return common.Newf(common.CodeUnauthenticated, "Неверный email или пароль")
	default:
		return err
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	if err := s.auth.Logout(r.Context(), ident, clientIP(r)); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

// handleMe returns the current user, roles and resolved permissions — the flat
// list the CRM uses to hide nav entries and buttons. Hiding is presentation only;
// the server still enforces every request (docs/04-RBAC.md:120).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	common.JSON(w, http.StatusOK, identityResponse(ident))
}

// clientIP reads the caller's address for the session record and audit trail.
//
// The BFF is the only client, and it reaches the API over the compose network, so
// RemoteAddr is the BFF's own address. X-Forwarded-For is honoured because the BFF
// sets it deliberately and nothing else can reach this port (I8, I18) — on a
// publicly reachable API this header would be attacker-controlled and unusable.
func clientIP(r *http.Request) *netip.Addr {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		if addr, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
			return &addr
		}
	}
	host := r.RemoteAddr
	if h, _, found := strings.Cut(host, ":"); found && strings.Count(host, ":") == 1 {
		host = h // host:port; bare IPv6 is left alone
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return &addr
	}
	return nil
}
