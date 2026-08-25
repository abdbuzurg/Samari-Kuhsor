package seed_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/rbac"
	"github.com/qoim/samari/backend/internal/seed"
	"github.com/qoim/samari/backend/internal/testsupport"
)

func load(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testsupport.NewDB(t)
	if _, err := seed.Reference(context.Background(), pool,
		"admin@samari-kuhsor.tj", "$argon2id$fake", "Администратор"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return pool
}

// docs/07-IMPLEMENTATION-PLAN.md I22 — the reference seed runs on production, so
// running it twice must be a no-op rather than a duplicate-key failure or, worse,
// a second set of roles.
func TestReferenceIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := load(t)
	ctx := context.Background()

	second, err := seed.Reference(ctx, pool, "admin@samari-kuhsor.tj", "$argon2id$fake", "Администратор")
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if second.RolesCreated != 0 || second.ItemsCreated != 0 ||
		second.PermissionsCreated != 0 || second.PackagingCreated != 0 ||
		second.TranslationsCreated != 0 || second.AdminCreated {
		t.Errorf("second run created rows: %+v", second)
	}

	counts := map[string]int{}
	for _, table := range []string{"roles", "role_permissions", "items", "item_translations",
		"packaging_units", "users", "user_roles"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE deleted_at IS NULL`).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	if counts["roles"] != 5 {
		t.Errorf("%d roles after two runs, expected 5", counts["roles"])
	}
	if counts["items"] != 5 {
		t.Errorf("%d items after two runs, expected 5", counts["items"])
	}
	if counts["users"] != 1 {
		t.Errorf("%d users after two runs, expected 1", counts["users"])
	}
}

// docs/04-RBAC.md §4 is transcribed cell for cell. This test is the transcription
// check: it restates the matrix independently, so a typo in the seed shows up as
// a disagreement rather than as a role quietly missing a permission.
func TestPermissionMatrixMatchesTheSpec(t *testing.T) {
	t.Parallel()
	pool := load(t)

	// docs/04-RBAC.md §4, read straight off the table.
	want := map[string]map[string][]string{
		"admin": {
			"dashboard": {"manage"}, "crm": {"manage"}, "inquiries": {"manage"},
			"items": {"manage"}, "inventory": {"manage"},
			"procurement": {"approve", "manage"}, "production": {"manage"},
			"quality": {"approve", "manage"}, "logistics": {"manage"}, "hr": {"manage"},
			"equipment": {"manage"}, "documents": {"approve", "manage"},
			"cms": {"approve", "manage"}, "admin": {"manage"}, "audit": {"read"},
			// analytics is read-only for everyone: website statistics are written
			// by visitors, not staff (docs/01-DECISIONS.md D12).
			"analytics": {"read"},
		},
		"director": {
			"dashboard": {"read"}, "crm": {"read"}, "inquiries": {"read"},
			"items": {"read"}, "inventory": {"read"},
			"procurement": {"approve", "read"}, "production": {"read"},
			"quality": {"read"}, "logistics": {"read"}, "hr": {"read"},
			"equipment": {"read"}, "documents": {"approve", "read"},
			"cms": {"approve", "read"}, "audit": {"read"}, "analytics": {"read"},
		},
		"warehouse": {
			"dashboard": {"read"}, "items": {"read"}, "inventory": {"manage"},
			"procurement": {"manage"}, "production": {"read"}, "quality": {"read"},
			"logistics": {"read"}, "equipment": {"read"}, "documents": {"read"},
		},
		"production": {
			"dashboard": {"read"}, "items": {"read"}, "inventory": {"read"},
			"production": {"manage"}, "quality": {"read"}, "equipment": {"read"},
			"documents": {"read"},
		},
		"quality": {
			"dashboard": {"read"}, "items": {"read"}, "inventory": {"read"},
			"production": {"read"}, "quality": {"approve", "manage"}, "documents": {"read"},
		},
	}

	for roleKey, wantPerms := range want {
		rows, err := pool.Query(context.Background(), `
			SELECT rp.resource, rp.action
			FROM roles r JOIN role_permissions rp ON rp.role_id = r.id
			WHERE r.key = $1 AND r.deleted_at IS NULL AND rp.deleted_at IS NULL
			ORDER BY rp.resource, rp.action`, roleKey)
		if err != nil {
			t.Fatalf("query %s: %v", roleKey, err)
		}
		got := map[string][]string{}
		for rows.Next() {
			var resource, action string
			if err := rows.Scan(&resource, &action); err != nil {
				t.Fatal(err)
			}
			got[resource] = append(got[resource], action)
		}
		rows.Close()

		for resource, actions := range wantPerms {
			slices.Sort(actions)
			gotActions := got[resource]
			slices.Sort(gotActions)
			if !slices.Equal(actions, gotActions) {
				t.Errorf("%s / %s: got %v, docs/04-RBAC.md §4 says %v",
					roleKey, resource, gotActions, actions)
			}
		}
		for resource := range got {
			if _, expected := wantPerms[resource]; !expected {
				t.Errorf("%s has an unexpected grant on %s: %v", roleKey, resource, got[resource])
			}
		}
	}
}

// docs/04-RBAC.md:95 — "Note what Директор does not get: manage on operational
// modules. Management reads; the floor writes." This is a deliberate part of the
// synchronisation design and must survive someone later assuming a director
// should be able to edit everything.
func TestDirectorCannotManageOperationalModules(t *testing.T) {
	t.Parallel()
	pool := load(t)

	rows, err := pool.Query(context.Background(), `
		SELECT rp.resource FROM roles r JOIN role_permissions rp ON rp.role_id = r.id
		WHERE r.key = 'director' AND rp.action = 'manage'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var resource string
		if err := rows.Scan(&resource); err != nil {
			t.Fatal(err)
		}
		t.Errorf("Директор has manage on %s; management reads, the floor writes", resource)
	}
}

// D8 — the catalogue is exactly five products, and the prototype's filler
// products "must not appear anywhere in the system".
func TestExactlyTheFiveApprovedProducts(t *testing.T) {
	t.Parallel()
	pool := load(t)

	rows, err := pool.Query(context.Background(), `
		SELECT sku, category, base_uom, status FROM items
		WHERE item_type = 'finished_good' AND deleted_at IS NULL ORDER BY sku`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type row struct{ sku, category, uom, status string }
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.sku, &r.category, &r.uom, &r.status); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}

	want := []row{
		{"APJ-1000", "juice", "bottle", "active"},
		{"APR-220", "jam", "jar", "active"},
		{"TOM-500", "paste", "jar", "active"},
		{"WAT-500", "water", "bottle", "active"},
		{"WAT-1000", "water", "bottle", "active"},
	}
	slices.SortFunc(want, func(a, b row) int { return strings.Compare(a.sku, b.sku) })

	if len(got) != len(want) {
		t.Fatalf("%d finished goods, D8 says exactly 5: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("product %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The CRM prototype's filler products were design placeholders, not real products.
func TestNoPrototypeFillerProducts(t *testing.T) {
	t.Parallel()
	pool := load(t)

	for _, forbidden := range []string{
		"JUS-APL-100", "JAM-APR-035", "PST-TOM-050", "WAT-050-24", // prototype SKU grammar
		"WAT-1500", // the 1.5 л water that does not exist
	} {
		var n int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM items WHERE sku = $1`, forbidden).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("prototype filler SKU %s was seeded; D8 says it is not real", forbidden)
		}
	}
	// And no pomegranate, apricot juice or strawberry jam by name.
	for _, forbidden := range []string{"Гранат", "Клубнич", "1,5 л"} {
		var n int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM item_translations WHERE name ILIKE '%' || $1 || '%'`,
			forbidden).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("a product matching %q was seeded; it is prototype filler", forbidden)
		}
	}
}

// docs/02-SCHEMA.md:176 — composition, nutrition and shelf life stay null until
// the client's recipes are lab-verified, so the UI renders «уточняется». Seeding
// a plausible-looking value would publish an unverified claim, which the client
// explicitly forbade.
func TestUnverifiedClaimsAreLeftNull(t *testing.T) {
	t.Parallel()
	pool := load(t)

	var n int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM items i
		LEFT JOIN item_translations t ON t.item_id = i.id
		WHERE i.deleted_at IS NULL AND (
			i.shelf_life_days IS NOT NULL OR
			t.ingredients IS NOT NULL OR
			t.nutrition IS NOT NULL
		)`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d seeded rows carry composition, nutrition or shelf life; these must stay null until lab-verified", n)
	}
}

// D8 — a case is a unit, not a product. WAT-500 × 24 is a packaging unit of
// WAT-500, never a separate SKU.
func TestCasesArePackagingUnitsNotProducts(t *testing.T) {
	t.Parallel()
	pool := load(t)

	var qty string
	err := pool.QueryRow(context.Background(), `
		SELECT pu.qty_in_base::text FROM packaging_units pu
		JOIN items i ON i.id = pu.item_id
		WHERE i.sku = 'WAT-500' AND pu.code = 'CASE24'`).Scan(&qty)
	if err != nil {
		t.Fatalf("WAT-500 CASE24 packaging unit missing: %v", err)
	}
	if qty != "24.000" {
		t.Errorf("CASE24 qty_in_base = %s, want 24.000", qty)
	}

	// Every item has a base consumer unit of exactly 1.
	var missing int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM items i WHERE i.deleted_at IS NULL AND NOT EXISTS (
			SELECT 1 FROM packaging_units pu
			WHERE pu.item_id = i.id AND pu.qty_in_base = 1 AND pu.deleted_at IS NULL)`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Errorf("%d items have no base consumer unit", missing)
	}
}

// D10 — Tajik and English product names come from the translation vendor.
// Inventing them here would publish unreviewed content; a missing locale row
// correctly falls back to ru (docs/02-SCHEMA.md:53).
func TestOnlyRussianProductNamesAreSeeded(t *testing.T) {
	t.Parallel()
	pool := load(t)

	rows, err := pool.Query(context.Background(),
		`SELECT DISTINCT locale FROM item_translations WHERE deleted_at IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var locale string
		if err := rows.Scan(&locale); err != nil {
			t.Fatal(err)
		}
		if locale != "ru" {
			t.Errorf("locale %q was seeded; tg and en must come from the translators", locale)
		}
	}
}

func TestAdminGetsTheAdminRole(t *testing.T) {
	t.Parallel()
	pool := load(t)

	rows, err := pool.Query(context.Background(), `
		SELECT rp.resource || ':' || rp.action
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE u.email = 'admin@samari-kuhsor.tj'`)
	if err != nil {
		t.Fatal(err)
	}
	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		perms = append(perms, p)
	}
	rows.Close()

	set := rbac.NewSet(perms)
	if !set.Can(rbac.Admin, rbac.Manage) {
		t.Error("the seeded administrator cannot manage admin — nobody could configure the system")
	}
	if !set.Can(rbac.Quality, rbac.Approve) {
		t.Error("the seeded administrator cannot approve quality")
	}
}

// The seed roles are is_system: an administrator may edit their permissions but
// must not be able to delete them (D9).
func TestSeedRolesAreSystemRoles(t *testing.T) {
	t.Parallel()
	pool := load(t)

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM roles WHERE NOT is_system AND deleted_at IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d seeded roles are not marked is_system", n)
	}
}

// A typo in the matrix must fail the seed, not create a permission nobody can hold.
func TestSeedRoleDefinitionsAreValid(t *testing.T) {
	t.Parallel()
	for _, r := range seed.SeedRoles {
		for _, p := range r.Permissions {
			perm, err := rbac.ParsePermission(p)
			if err != nil {
				t.Errorf("role %s: %v", r.Key, err)
				continue
			}
			if !rbac.ValidResource(perm.Resource) {
				t.Errorf("role %s grants unknown resource %q", r.Key, perm.Resource)
			}
			if perm.Action == rbac.Approve && !rbac.ApproveResources[perm.Resource] {
				t.Errorf("role %s grants %s, but approve is not defined for that resource", r.Key, p)
			}
		}
	}
}
