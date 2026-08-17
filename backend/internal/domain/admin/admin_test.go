package admin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/admin"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
	"github.com/qoim/samari/backend/internal/seed"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// docs/04-RBAC.md §6. The guardrail under test is the one that can brick the
// system: if the last administrator is deactivated or stripped of admin:manage,
// nobody can administer anything and the only fix is direct database access to a
// production server in Dushanbe.

type fixture struct {
	pool  *pgxpool.Pool
	svc   *admin.Service
	actor uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	ctx := context.Background()

	// Real seed roles and matrix, so the guard is tested against the permissions
	// QOIM will actually have.
	if _, err := seed.Reference(ctx, pool, "admin@samari-kuhsor.tj", "$argon2id$fake", "Администратор"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	f := fixture{pool: pool, svc: admin.NewService(pool)}
	if err := pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email='admin@samari-kuhsor.tj'`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) roleID(t *testing.T, key string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM roles WHERE key=$1`, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) newUser(t *testing.T, email string, roleKeys ...string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ($1, $1, 'x') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, k := range roleKeys {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, id, f.roleID(t, k)); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (f fixture) isActive(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	var active bool
	if err := f.pool.QueryRow(context.Background(),
		`SELECT is_active FROM users WHERE id=$1`, id).Scan(&active); err != nil {
		t.Fatal(err)
	}
	return active
}

// ---------------------------------------------------------------------------
// THE guardrail
// ---------------------------------------------------------------------------

func TestLastAdministratorCannotBeDeactivated(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	// The seeded administrator is the only one.
	err := func() error {
		_, err := f.svc.SetUserActive(ctx, f.actor, f.actor, false)
		return err
	}()
	if err == nil {
		t.Fatal("the last administrator was deactivated; nobody could administer the system")
	}
	if code := common.AsError(err).Code; code != common.CodeBusinessRule {
		t.Errorf("code = %s, want business_rule", code)
	}
	// The message must say what to do, not merely refuse: the person hitting this
	// is usually doing something reasonable.
	if msg := common.AsError(err).Message; !strings.Contains(msg, "администратор") {
		t.Errorf("unhelpful message: %q", msg)
	}
	if !f.isActive(t, f.actor) {
		t.Error("the account was deactivated despite the refusal")
	}
}

func TestLastAdministratorCannotBeStrippedOfTheirRole(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	// Reassigning the sole administrator to a non-admin role.
	if err := f.svc.SetUserRoles(ctx, f.actor, f.actor,
		[]uuid.UUID{f.roleID(t, "warehouse")}); err == nil {
		t.Fatal("the last administrator was stripped of admin:manage")
	}

	// Their roles are unchanged.
	var n int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id
		WHERE ur.user_id=$1 AND ur.deleted_at IS NULL AND r.key='admin'`, f.actor).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("the administrator lost their role despite the refusal")
	}
}

// The subtler route to the same disaster: leave the user alone, but remove
// admin:manage from the role itself.
func TestRemovingAdminManageFromTheLastAdminRoleIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	// A permission set with everything except admin:manage.
	reduced := []string{"dashboard:manage", "items:manage", "audit:read"}

	if err := f.svc.SetRolePermissions(ctx, f.actor, f.roleID(t, "admin"), reduced); err == nil {
		t.Fatal("admin:manage was removed from the only role granting it")
	}

	// The permission survives.
	var n int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id
		WHERE r.key='admin' AND rp.resource='admin' AND rp.action='manage'
		  AND rp.deleted_at IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("admin:manage was removed despite the refusal")
	}
}

// With a second administrator in place, every one of those operations is allowed
// again. A guard that never lets go is as broken as one that never holds.
func TestWithASecondAdministratorTheGuardReleases(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	second := f.newUser(t, "second-admin@samari-kuhsor.tj", "admin")

	if _, err := f.svc.SetUserActive(ctx, second, f.actor, false); err != nil {
		t.Fatalf("deactivation was still refused with two administrators: %v", err)
	}
	if f.isActive(t, f.actor) {
		t.Error("the account was not deactivated")
	}
}

// A DEACTIVATED administrator does not count: they cannot log in, so they cannot
// rescue anyone.
func TestADeactivatedAdministratorDoesNotCount(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	second := f.newUser(t, "second-admin@samari-kuhsor.tj", "admin")

	// Deactivate the second one (allowed — the first is still active).
	if _, err := f.svc.SetUserActive(ctx, f.actor, second, false); err != nil {
		t.Fatalf("deactivating the second administrator: %v", err)
	}

	// Now the first is once again the only ACTIVE administrator.
	if _, err := f.svc.SetUserActive(ctx, f.actor, f.actor, false); err == nil {
		t.Fatal("the last ACTIVE administrator was deactivated; a deactivated one cannot rescue anyone")
	}
}

// ---------------------------------------------------------------------------
// System roles
// ---------------------------------------------------------------------------

// D9 — seed roles are a starting point, not a fixed matrix: editable, but not
// deletable.
func TestSystemRolesAreEditableButNotDeletable(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	warehouse := f.roleID(t, "warehouse")

	// Editable.
	if err := f.svc.SetRolePermissions(ctx, f.actor, warehouse,
		[]string{"dashboard:read", "inventory:manage", "logistics:manage"}); err != nil {
		t.Fatalf("a system role could not be edited: %v", err)
	}
	perms, err := f.svc.RolePermissions(ctx, warehouse)
	if err != nil {
		t.Fatal(err)
	}
	if len(perms) != 3 {
		t.Errorf("permissions = %v, want 3", perms)
	}

	// Not deletable.
	if err := f.svc.DeleteRole(ctx, f.actor, warehouse); err == nil {
		t.Fatal("a system role was deleted")
	}
	var n int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM roles WHERE key='warehouse' AND deleted_at IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("the system role was deleted despite the refusal")
	}
}

func TestCustomRolesCanBeCreatedAndDeleted(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	role, err := f.svc.CreateRole(ctx, f.actor, admin.RoleInput{
		Key: "driver", NameRU: "Водитель", NameTG: "Ронанда", NameEN: "Driver",
		Permissions: []string{"logistics:read", "dashboard:read"},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.IsSystem {
		t.Error("a user-created role was marked is_system")
	}
	testsupport.AssertAudited(t, f.pool, "admin", role.ID, "create")

	if err := f.svc.DeleteRole(ctx, f.actor, role.ID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	testsupport.AssertAudited(t, f.pool, "admin", role.ID, "delete")
}

// ---------------------------------------------------------------------------
// Permission validation
// ---------------------------------------------------------------------------

func TestRoleValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	base := admin.RoleInput{Key: "x", NameRU: "X", NameTG: "X", NameEN: "X"}

	cases := map[string]admin.RoleInput{
		"no key":           {NameRU: "X", NameTG: "X", NameEN: "X"},
		"no Russian name":  {Key: "x", NameTG: "X", NameEN: "X"},
		"unknown resource": withPerms(base, "widgets:read"),
		"unknown action":   withPerms(base, "items:destroy"),
		// approve is defined for five resources only (docs/04-RBAC.md §3).
		"approve where undefined": withPerms(base, "items:approve"),
		"malformed":               withPerms(base, "itemsread"),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.CreateRole(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			}
		})
	}

	// approve IS accepted where it is defined.
	for _, resource := range []string{"quality", "procurement", "cms", "documents"} {
		in := base
		in.Key = "role-" + resource
		in.Permissions = []string{resource + ":approve"}
		if _, err := f.svc.CreateRole(ctx, f.actor, in); err != nil {
			t.Errorf("%s:approve was refused: %v", resource, err)
		}
	}
}

// docs/04-RBAC.md:149 — permission changes take effect on the affected user's
// NEXT request. Nothing is cached beyond the request, so this is a property of
// reading them fresh every time.
func TestPermissionChangesTakeEffectImmediately(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	user := f.newUser(t, "ops@samari-kuhsor.tj", "warehouse")

	read := func() rbac.Set {
		t.Helper()
		rows, err := f.pool.Query(ctx, `
			SELECT rp.resource, rp.action FROM user_roles ur
			JOIN roles r ON r.id=ur.role_id AND r.deleted_at IS NULL
			JOIN role_permissions rp ON rp.role_id=r.id AND rp.deleted_at IS NULL
			WHERE ur.user_id=$1 AND ur.deleted_at IS NULL`, user)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var flat []string
		for rows.Next() {
			var res, act string
			if err := rows.Scan(&res, &act); err != nil {
				t.Fatal(err)
			}
			flat = append(flat, res+":"+act)
		}
		return rbac.NewSet(flat)
	}

	if !read().Can(rbac.Inventory, rbac.Manage) {
		t.Fatal("the warehouse role does not grant inventory:manage")
	}

	// Strip it.
	if err := f.svc.SetRolePermissions(ctx, f.actor, f.roleID(t, "warehouse"),
		[]string{"dashboard:read"}); err != nil {
		t.Fatal(err)
	}

	// The very next read reflects it — no cache to invalidate.
	if read().Can(rbac.Inventory, rbac.Manage) {
		t.Error("a revoked permission was still in effect")
	}
}

func TestRoleChangesAreAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	warehouse := f.roleID(t, "warehouse")
	if err := f.svc.SetRolePermissions(ctx, f.actor, warehouse, []string{"dashboard:read"}); err != nil {
		t.Fatal(err)
	}
	entry := testsupport.AssertAudited(t, f.pool, "admin", warehouse, "update")

	// The before/after must show what changed, or the trail cannot answer
	// "who removed my access".
	if !strings.Contains(string(entry.Before), "inventory:manage") {
		t.Errorf("the audit entry does not record the removed permission: %s", entry.Before)
	}
	if !strings.Contains(string(entry.After), "dashboard:read") {
		t.Errorf("the audit entry does not record the new set: %s", entry.After)
	}
}

func withPerms(in admin.RoleInput, perms ...string) admin.RoleInput {
	in.Permissions = perms
	return in
}
