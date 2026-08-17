package rbac

import (
	"context"
	"net/http"
)

// Identity is what the authentication middleware puts in the request context: the
// resolved caller and their permission set. rbac deliberately does not import the
// auth package — it needs only these two facts, and keeping the dependency one-way
// stops authorization logic drifting into the auth package.
type Identity struct {
	UserID      string
	Permissions Set
}

type contextKey struct{}

var identityKey = contextKey{}

// WithIdentity attaches a resolved identity to the request context.
func WithIdentity(ctx context.Context, ident Identity) context.Context {
	return context.WithValue(ctx, identityKey, ident)
}

// IdentityFrom returns the resolved identity, if the request was authenticated.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	ident, ok := ctx.Value(identityKey).(Identity)
	return ident, ok
}

// Unauthorized and Forbidden are set by the HTTP layer at startup so rbac can
// respond in the API's envelope format without importing the HTTP package (which
// would be a cycle). They are plain funcs rather than an interface because there
// is exactly one implementation and it is chosen once.
var (
	Unauthorized = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
	}
	Forbidden = func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}
)

// Require gates a route on a permission.
//
// Two distinct failures, deliberately never conflated (docs/04-RBAC.md:117):
//
//	no identity at all           → 401 unauthenticated
//	identity without the grant   → 403 forbidden
//
// A missing permission is never a 404 and never a silently empty list. Filtering
// results instead of refusing hides bugs: the caller cannot tell "you may not see
// this" from "there is nothing here", and neither can the developer.
func Require(resource string, action Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ident, ok := IdentityFrom(r.Context())
			if !ok {
				Unauthorized(w, r)
				return
			}
			if !ident.Permissions.Can(resource, action) {
				Forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
