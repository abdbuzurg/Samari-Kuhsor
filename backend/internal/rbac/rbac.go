// Package rbac implements the permission model in docs/04-RBAC.md.
//
// The model is deliberately small: a user's effective permissions are the union
// across their roles. There is no denial precedence and no role hierarchy — union
// only, which is simpler to reason about, simpler to test, and sufficient for a
// company of this size (docs/04-RBAC.md §1).
//
// Authorization happens HERE, in Go middleware, and nowhere else. The BFF forwards
// the session and shapes payloads; React hides buttons for presentation. Neither
// is a control (CLAUDE.md §3).
package rbac

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Action is what may be done to a resource.
type Action string

const (
	// Read views lists, detail pages and reports.
	Read Action = "read"
	// Manage creates, edits and tombstones. Implies Read.
	Manage Action = "manage"
	// Approve authorises a state transition that carries authority rather than
	// data entry — releasing a batch, approving a purchase order, publishing
	// content. It does NOT imply Manage: signing something off is a different
	// authority from editing it (docs/04-RBAC.md §3).
	Approve Action = "approve"
)

func (a Action) Valid() bool {
	switch a {
	case Read, Manage, Approve:
		return true
	}
	return false
}

// Resource keys. One per module, plus the two cross-cutting ones. These strings
// are stable: they appear in role_permissions.resource, in audit_log.resource and
// in the permission list /auth/me returns (docs/04-RBAC.md §2).
const (
	Dashboard   = "dashboard"
	CRM         = "crm"
	Inquiries   = "inquiries"
	Items       = "items"
	Inventory   = "inventory"
	Procurement = "procurement"
	Production  = "production"
	Quality     = "quality"
	Logistics   = "logistics"
	HR          = "hr"
	Equipment   = "equipment"
	Documents   = "documents"
	Finance     = "finance" // deferred (D2); the key exists so it cannot be invented twice
	CMS         = "cms"
	Admin       = "admin"
	Audit       = "audit"
	// Auth is not a module and carries no permissions. Login must work before any
	// permission can be resolved, so /auth/* routes declare this instead.
	Auth = "auth"
)

// Resources is every valid resource key, for validation and for the role editor's
// permission matrix.
var Resources = []string{
	Dashboard, CRM, Inquiries, Items, Inventory, Procurement, Production,
	Quality, Logistics, HR, Equipment, Documents, Finance, CMS, Admin, Audit,
}

// ApproveResources are the only resources where Approve is meaningful
// (docs/04-RBAC.md §3). Granting it elsewhere is a configuration error, not a
// harmless no-op, so the role editor and the seed both validate against this.
var ApproveResources = map[string]bool{
	Quality:     true,
	Procurement: true,
	CMS:         true,
	Documents:   true,
	Finance:     true, // phase 2
}

func ValidResource(r string) bool { return slices.Contains(Resources, r) }

// Permission is a resource/action pair.
type Permission struct {
	Resource string
	Action   Action
}

func (p Permission) String() string { return p.Resource + ":" + string(p.Action) }

// ParsePermission reads the canonical "resource:action" form used everywhere —
// in code, in role_permissions, and in /auth/me (docs/03-API-CONTRACT.md:78).
func ParsePermission(s string) (Permission, error) {
	resource, action, found := strings.Cut(s, ":")
	if !found {
		return Permission{}, fmt.Errorf("rbac: %q is not resource:action", s)
	}
	// An empty resource is not merely odd, it is dangerous: it parses, it is
	// stored, and it can never match any real check — so it looks like a granted
	// permission while granting nothing.
	if resource == "" {
		return Permission{}, fmt.Errorf("rbac: %q has an empty resource", s)
	}
	p := Permission{Resource: resource, Action: Action(action)}
	if !p.Action.Valid() {
		return Permission{}, fmt.Errorf("rbac: %q is not a valid action", action)
	}
	// Deliberately NOT validated against Resources here: NewSet reads rows written
	// by an administrator through the role editor, and an older binary meeting a
	// resource key added by a newer migration must not silently drop the user's
	// other permissions. Unknown resources are rejected where they are declared —
	// at startup, in Registry.Guarded.
	return p, nil
}

// Set is a resolved permission set for one user.
type Set struct {
	granted map[Permission]struct{}
}

// NewSet builds a Set from the flat "resource:action" strings the auth layer
// resolves. Unparseable entries are ignored rather than fatal: a malformed row in
// role_permissions must not lock a user out of the entire system, and it will be
// caught by the seed validation and the role editor.
func NewSet(perms []string) Set {
	s := Set{granted: make(map[Permission]struct{}, len(perms))}
	for _, raw := range perms {
		if p, err := ParsePermission(raw); err == nil {
			s.granted[p] = struct{}{}
		}
	}
	return s
}

// Can reports whether the set satisfies the requirement.
//
// The only implication in the model: Manage satisfies Read. Approve does not
// satisfy Manage, and Manage does not satisfy Approve (docs/04-RBAC.md:116).
func (s Set) Can(resource string, action Action) bool {
	if _, ok := s.granted[Permission{resource, action}]; ok {
		return true
	}
	if action == Read {
		if _, ok := s.granted[Permission{resource, Manage}]; ok {
			return true
		}
	}
	return false
}

// CanRead is sugar for the commonest check: whether a module is visible at all.
func (s Set) CanRead(resource string) bool { return s.Can(resource, Read) }

// ReadableResources lists the modules the user may see, for building the nav.
// Presentation only — the server still enforces every request.
func (s Set) ReadableResources() []string {
	var out []string
	for _, r := range Resources {
		if s.CanRead(r) {
			out = append(out, r)
		}
	}
	return out
}

// Strings returns the flat, sorted, deduplicated list for /auth/me.
func (s Set) Strings() []string {
	out := make([]string, 0, len(s.granted))
	for p := range s.granted {
		out = append(out, p.String())
	}
	sort.Strings(out)
	return out
}

// IsEmpty reports whether the user can do nothing at all.
func (s Set) IsEmpty() bool { return len(s.granted) == 0 }
