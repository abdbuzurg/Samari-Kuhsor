package rbac

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// docs/04-RBAC.md:123 — "Every route must declare a permission. A route with no
// rbac.Require is a bug; add a lint or startup check that fails if any registered
// route lacks one."
//
// The mechanism: routes are registered through Router, which records what each
// one declared. Verify then walks the real chi tree and fails if anything was
// registered without passing through here.
//
// Crucially, a route with no permission must be declared PUBLIC explicitly. That
// turns "this endpoint is unauthenticated" from an omission anyone can make by
// forgetting a middleware into a decision someone had to write down — which is
// the whole point of the check.

// Declaration is what a route declared about its own access.
type Declaration struct {
	Method  string
	Pattern string
	// Permission is the requirement, or nil for a deliberately public route.
	Permission *Permission
	// PublicReason documents why a public route is public. Required, so that
	// "public" is always accompanied by a justification.
	PublicReason string
}

func (d Declaration) key() string { return d.Method + " " + d.Pattern }

func (d Declaration) String() string {
	if d.Permission == nil {
		return fmt.Sprintf("%s %s  public — %s", d.Method, d.Pattern, d.PublicReason)
	}
	return fmt.Sprintf("%s %s  %s", d.Method, d.Pattern, d.Permission)
}

// Registry collects declarations as routes are mounted.
type Registry struct {
	declared map[string]Declaration
}

func NewRegistry() *Registry {
	return &Registry{declared: make(map[string]Declaration)}
}

func (reg *Registry) record(d Declaration) {
	if existing, ok := reg.declared[d.key()]; ok {
		panic(fmt.Sprintf("rbac: %s declared twice (%s, then %s)", d.key(), existing, d))
	}
	reg.declared[d.key()] = d
}

// Declarations returns every declaration, sorted — used by the startup log so the
// full access surface of the API is visible in one place on every boot.
func (reg *Registry) Declarations() []Declaration {
	out := make([]Declaration, 0, len(reg.declared))
	for _, d := range reg.declared {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// Guarded mounts a route behind a permission requirement.
func (reg *Registry) Guarded(r chi.Router, method, pattern, resource string, action Action, h http.HandlerFunc) {
	if !ValidResource(resource) {
		panic(fmt.Sprintf("rbac: %s %s declares unknown resource %q", method, pattern, resource))
	}
	if !action.Valid() {
		panic(fmt.Sprintf("rbac: %s %s declares unknown action %q", method, pattern, action))
	}
	// Approve is only meaningful on a handful of resources (docs/04-RBAC.md §3).
	// Requiring it elsewhere would create a permission no role can sensibly hold.
	if action == Approve && !ApproveResources[resource] {
		panic(fmt.Sprintf("rbac: %s %s requires %s:approve, but approve is not defined for that resource",
			method, pattern, resource))
	}

	p := Permission{Resource: resource, Action: action}
	reg.record(Declaration{Method: method, Pattern: pattern, Permission: &p})
	r.With(Require(resource, action)).Method(method, pattern, h)
}

// Public mounts a route with no permission requirement. The reason is mandatory.
func (reg *Registry) Public(r chi.Router, method, pattern, reason string, h http.HandlerFunc) {
	if strings.TrimSpace(reason) == "" {
		panic(fmt.Sprintf("rbac: %s %s is public but gives no reason", method, pattern))
	}
	reg.record(Declaration{Method: method, Pattern: pattern, PublicReason: reason})
	r.Method(method, pattern, h)
}

// Verify walks the mounted router and reports every route that was registered
// without a declaration. Call it at startup and refuse to serve on error.
func Verify(root chi.Router, reg *Registry) error {
	var undeclared []string

	err := chi.Walk(root, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// chi reports patterns with a trailing slash for subrouter roots.
		normalised := route
		if len(normalised) > 1 && strings.HasSuffix(normalised, "/") {
			normalised = strings.TrimSuffix(normalised, "/")
		}
		if _, ok := reg.declared[method+" "+normalised]; !ok {
			if _, ok := reg.declared[method+" "+route]; !ok {
				undeclared = append(undeclared, method+" "+route)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rbac: walk routes: %w", err)
	}

	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf(
			"rbac: %d route(s) registered without a permission declaration — "+
				"mount them with Registry.Guarded or, if genuinely unauthenticated, "+
				"Registry.Public with a reason:\n  %s",
			len(undeclared), strings.Join(undeclared, "\n  "))
	}
	return nil
}
