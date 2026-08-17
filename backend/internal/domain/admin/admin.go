// Package admin implements role management and the audit viewer.
//
// docs/04-RBAC.md §6 flags this as "not one of the twelve modules, and easy to
// forget", which is exactly why it carries the guardrail that matters most:
//
//	The last user holding admin:manage cannot be deactivated or stripped of it.
//	Enforce server-side (docs/04-RBAC.md:147).
//
// Without it, one careless edit locks every person out of a system that has no
// other way in — and the fix would require direct database access to a production
// server in Dushanbe.
package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Admin

var SortSpec = common.SortSpec{
	Allowed:     []string{"full_name", "email"},
	Default:     "full_name",
	DefaultDesc: false,
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// ErrLastAdministrator is returned when a change would leave nobody able to
// administer the system.
var ErrLastAdministrator = errors.New("admin: last administrator")

// lastAdminError is the user-facing form. Phrased as an explanation rather than a
// refusal, because the person hitting it is usually trying to do something
// reasonable and needs to know what to do first.
func lastAdminError() error {
	return common.BusinessRule(
		"Это последний пользователь с правами администратора. Назначьте администратором другого пользователя, прежде чем изменять этого.")
}

// guardLastAdmin refuses a change that would remove the final administrator.
//
// Counts users OTHER than the one being changed who are active and hold
// admin:manage. Counting the subject too would let the last administrator strip
// their own rights, which is the exact scenario this prevents.
func (s *Service) guardLastAdmin(ctx context.Context, q *db.Queries, subject uuid.UUID) error {
	others, err := q.CountUsersHoldingPermission(ctx, db.CountUsersHoldingPermissionParams{
		Resource:  rbac.Admin,
		Action:    string(rbac.Manage),
		Excluding: uuid.NullUUID{UUID: subject, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("admin: count administrators: %w", err)
	}
	if others == 0 {
		return lastAdminError()
	}
	return nil
}

// holdsAdminManage reports whether a user currently holds admin:manage.
func (s *Service) holdsAdminManage(ctx context.Context, q *db.Queries, userID uuid.UUID) (bool, error) {
	perms, err := q.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("admin: permissions: %w", err)
	}
	flat := make([]string, 0, len(perms))
	for _, p := range perms {
		flat = append(flat, p.Resource+":"+p.Action)
	}
	return rbac.NewSet(flat).Can(rbac.Admin, rbac.Manage), nil
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

type RoleInput struct {
	Key                    string
	NameRU, NameTG, NameEN string
	// Permissions as "resource:action" strings, replacing the role's current set.
	Permissions []string
}

func (s *Service) CreateRole(ctx context.Context, actor uuid.UUID, in RoleInput) (db.Role, error) {
	if err := validateRole(in); err != nil {
		return db.Role{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Role{}, fmt.Errorf("admin: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	role, err := q.CreateRole(ctx, db.CreateRoleParams{
		Key: in.Key, NameRu: in.NameRU, NameTg: in.NameTG, NameEn: in.NameEN,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		if strings.Contains(err.Error(), "roles_key_key") {
			return db.Role{}, common.Validation(common.FieldError{
				Field: "key", Code: "already_exists", Message: "Роль с таким ключом уже существует",
			})
		}
		return db.Role{}, fmt.Errorf("admin: create role: %w", err)
	}

	if err := s.applyPermissions(ctx, q, actor, role.ID, in.Permissions); err != nil {
		return db.Role{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionCreate, Resource: Resource,
		ResourceID: audit.Target(role.ID),
		After:      map[string]any{"key": role.Key, "permissions": in.Permissions},
	}); err != nil {
		return db.Role{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Role{}, fmt.Errorf("admin: commit: %w", err)
	}
	return role, nil
}

// SetRolePermissions replaces a role's permission set.
//
// System roles ARE editable — they are a starting point, not a fixed matrix (D9).
// What they are not is deletable.
func (s *Service) SetRolePermissions(ctx context.Context, actor uuid.UUID, roleID uuid.UUID, perms []string) error {
	for _, p := range perms {
		if err := validatePermission(p); err != nil {
			return err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	role, err := q.GetRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.NotFound()
		}
		return fmt.Errorf("admin: load role: %w", err)
	}

	before, err := q.ListRolePermissions(ctx, roleID)
	if err != nil {
		return fmt.Errorf("admin: current permissions: %w", err)
	}

	// If this role currently grants admin:manage and the new set does not, check
	// that somebody else can still administer the system. Editing the admin role's
	// permissions is the subtler way to lock everyone out.
	grantedAdmin := false
	for _, p := range before {
		if p.Resource == rbac.Admin && p.Action == string(rbac.Manage) {
			grantedAdmin = true
		}
	}
	willGrantAdmin := false
	for _, p := range perms {
		if p == rbac.Admin+":"+string(rbac.Manage) {
			willGrantAdmin = true
		}
	}
	if grantedAdmin && !willGrantAdmin {
		if err := s.guardRoleRemovesLastAdmin(ctx, q, roleID); err != nil {
			return err
		}
	}

	if err := q.ClearRolePermissions(ctx, roleID); err != nil {
		return fmt.Errorf("admin: clear permissions: %w", err)
	}
	if err := s.applyPermissions(ctx, q, actor, roleID, perms); err != nil {
		return err
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate, Resource: Resource,
		ResourceID: audit.Target(roleID),
		Before:     map[string]any{"permissions": permStrings(before)},
		After:      map[string]any{"permissions": perms, "role": role.Key},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// guardRoleRemovesLastAdmin checks whether removing admin:manage from a role
// would leave nobody holding it.
func (s *Service) guardRoleRemovesLastAdmin(ctx context.Context, q *db.Queries, roleID uuid.UUID) error {
	// Count administrators who hold the permission through some OTHER role.
	rows, err := q.ListUsersWithRoles(ctx, db.ListUsersWithRolesParams{Limit: 1000, Offset: 0})
	if err != nil {
		return fmt.Errorf("admin: list users: %w", err)
	}
	role, err := q.GetRole(ctx, roleID)
	if err != nil {
		return fmt.Errorf("admin: load role: %w", err)
	}

	for _, u := range rows {
		if !u.IsActive {
			continue
		}
		// Does this user hold admin:manage via a role other than the one changing?
		otherRoles := false
		for _, key := range u.RoleKeys {
			if key != role.Key {
				otherRoles = true
			}
		}
		if !otherRoles {
			continue
		}
		holds, err := s.holdsAdminManage(ctx, q, u.ID)
		if err != nil {
			return err
		}
		if holds {
			return nil // somebody else can still administer
		}
	}
	return lastAdminError()
}

func (s *Service) applyPermissions(ctx context.Context, q *db.Queries, actor uuid.UUID, roleID uuid.UUID, perms []string) error {
	for _, raw := range perms {
		p, err := rbac.ParsePermission(raw)
		if err != nil {
			return common.Validation(common.FieldError{
				Field: "permissions", Code: "invalid", Message: "Некорректное разрешение: " + raw,
			})
		}
		if err := q.GrantPermission(ctx, db.GrantPermissionParams{
			RoleID: roleID, Resource: p.Resource, Action: string(p.Action),
			CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return fmt.Errorf("admin: grant %s: %w", raw, err)
		}
	}
	return nil
}

// DeleteRole tombstones a non-system role.
func (s *Service) DeleteRole(ctx context.Context, actor uuid.UUID, roleID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	role, err := q.GetRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.NotFound()
		}
		return fmt.Errorf("admin: load role: %w", err)
	}
	if role.IsSystem {
		return common.BusinessRule(
			"Системную роль нельзя удалить. Её разрешения можно изменить.")
	}

	if _, err := q.TombstoneRole(ctx, roleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.BusinessRule("Роль уже удалена или является системной.")
		}
		return fmt.Errorf("admin: delete role: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionDelete, Resource: Resource,
		ResourceID: audit.Target(roleID), Before: map[string]any{"key": role.Key},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// SetUserRoles replaces a user's roles.
func (s *Service) SetUserRoles(ctx context.Context, actor uuid.UUID, userID uuid.UUID, roleIDs []uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	// If this user currently administers and the new roles might not grant it,
	// make sure somebody else does.
	held, err := s.holdsAdminManage(ctx, q, userID)
	if err != nil {
		return err
	}
	if held {
		if err := s.guardLastAdmin(ctx, q, userID); err != nil {
			return err
		}
	}

	if err := q.ClearUserRoles(ctx, userID); err != nil {
		return fmt.Errorf("admin: clear roles: %w", err)
	}
	for _, rid := range roleIDs {
		if err := q.AssignRole(ctx, db.AssignRoleParams{
			UserID: userID, RoleID: rid, CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return fmt.Errorf("admin: assign role: %w", err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate, Resource: Resource,
		ResourceID: audit.Target(userID),
		After:      map[string]any{"roles": len(roleIDs)},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetUserActive activates or deactivates a user.
func (s *Service) SetUserActive(ctx context.Context, actor uuid.UUID, userID uuid.UUID, active bool) (db.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.User{}, fmt.Errorf("admin: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	if !active {
		held, err := s.holdsAdminManage(ctx, q, userID)
		if err != nil {
			return db.User{}, err
		}
		if held {
			if err := s.guardLastAdmin(ctx, q, userID); err != nil {
				return db.User{}, err
			}
		}
	}

	user, err := q.SetUserActive(ctx, db.SetUserActiveParams{ID: userID, IsActive: active})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, common.NotFound()
		}
		return db.User{}, fmt.Errorf("admin: set active: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate, Resource: Resource,
		ResourceID: audit.Target(userID),
		After:      map[string]any{"is_active": active, "email": user.Email},
	}); err != nil {
		return db.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.User{}, fmt.Errorf("admin: commit: %w", err)
	}
	return user, nil
}

// Reads.

func (s *Service) Roles(ctx context.Context) ([]db.ListRolesRow, error) {
	return db.New(s.pool).ListRoles(ctx)
}

func (s *Service) RolePermissions(ctx context.Context, roleID uuid.UUID) ([]string, error) {
	rows, err := db.New(s.pool).ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("admin: role permissions: %w", err)
	}
	return permStrings(rows), nil
}

func (s *Service) Users(ctx context.Context, p common.Params) ([]db.ListUsersWithRolesRow, int64, error) {
	q := db.New(s.pool)
	var search *string
	if p.Query != "" {
		search = &p.Query
	}
	rows, err := q.ListUsersWithRoles(ctx, db.ListUsersWithRolesParams{
		Q: search, Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("admin: users: %w", err)
	}
	total, err := q.CountUsers(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: count users: %w", err)
	}
	return rows, total, nil
}

func validateRole(in RoleInput) error {
	var details []common.FieldError
	if strings.TrimSpace(in.Key) == "" {
		details = append(details, common.FieldError{
			Field: "key", Code: "required", Message: "Укажите ключ роли",
		})
	}
	for _, f := range []struct{ name, value string }{
		{"name_ru", in.NameRU}, {"name_tg", in.NameTG}, {"name_en", in.NameEN},
	} {
		if strings.TrimSpace(f.value) == "" {
			details = append(details, common.FieldError{
				Field: f.name, Code: "required", Message: "Укажите название роли",
			})
		}
	}
	for _, p := range in.Permissions {
		if err := validatePermission(p); err != nil {
			details = append(details, common.FieldError{
				Field: "permissions", Code: "invalid", Message: "Некорректное разрешение: " + p,
			})
		}
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func validatePermission(raw string) error {
	p, err := rbac.ParsePermission(raw)
	if err != nil {
		return err
	}
	if !rbac.ValidResource(p.Resource) {
		return fmt.Errorf("unknown resource %q", p.Resource)
	}
	// approve is only defined for a handful of resources (docs/04-RBAC.md §3).
	// Granting it elsewhere creates a permission that can never be satisfied.
	if p.Action == rbac.Approve && !rbac.ApproveResources[p.Resource] {
		return fmt.Errorf("approve is not defined for %q", p.Resource)
	}
	return nil
}

func permStrings(rows []db.RolePermission) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Resource+":"+r.Action)
	}
	return out
}
