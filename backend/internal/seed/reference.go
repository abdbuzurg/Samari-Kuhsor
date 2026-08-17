// Package seed loads reference and demonstration data.
//
// Two commands, hard-separated (docs/07-IMPLEMENTATION-PLAN.md I22):
//
//	reference — production-safe and idempotent. The five real products, the seed
//	            roles and their permission matrix, one administrator.
//	demo      — suppliers, batches, movements, QC results and so on. REFUSES to
//	            run outside development, because demo batches with fabricated QC
//	            releases sitting in audit_log alongside real ones is not untidy
//	            data, it is a falsified regulatory record — and no-hard-delete
//	            means tombstoning them does not remove them.
package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/rbac"
)

// ---------------------------------------------------------------------------
// Roles — docs/04-RBAC.md §4
// ---------------------------------------------------------------------------

// SeedRole is one of the five roles shipped so QOIM is not configuring RBAC on
// opening day (D9). They are is_system: editable by an administrator, but not
// deletable.
type SeedRole struct {
	Key                    string
	NameRU, NameTG, NameEN string
	// Permissions is this role's row of the matrix in docs/04-RBAC.md §4.
	Permissions []string
}

// SeedRoles transcribes docs/04-RBAC.md §4 exactly.
//
// Note what Директор does NOT get: manage on any operational module. Management
// reads; the floor writes (docs/04-RBAC.md:95). That is a deliberate part of the
// original synchronisation design, not an oversight, and it must survive anyone
// later assuming a director should be able to edit everything.
var SeedRoles = []SeedRole{
	{
		Key: "admin", NameRU: "Администратор", NameTG: "Маъмур", NameEN: "Administrator",
		Permissions: []string{
			"dashboard:manage", "crm:manage", "inquiries:manage", "items:manage",
			"inventory:manage", "procurement:manage", "procurement:approve",
			"production:manage", "quality:manage", "quality:approve",
			"logistics:manage", "hr:manage", "equipment:manage",
			"documents:manage", "documents:approve",
			"cms:manage", "cms:approve", "admin:manage", "audit:read",
		},
	},
	{
		Key: "director", NameRU: "Директор", NameTG: "Директор", NameEN: "Director",
		Permissions: []string{
			"dashboard:read", "crm:read", "inquiries:read", "items:read",
			"inventory:read", "procurement:read", "procurement:approve",
			"production:read", "quality:read", "logistics:read", "hr:read",
			"equipment:read", "documents:read", "documents:approve",
			"cms:read", "cms:approve", "audit:read",
		},
	},
	{
		Key: "warehouse", NameRU: "Склад", NameTG: "Анбор", NameEN: "Warehouse",
		Permissions: []string{
			"dashboard:read", "items:read", "inventory:manage", "procurement:manage",
			"production:read", "quality:read", "logistics:read", "equipment:read",
			"documents:read",
		},
	},
	{
		Key: "production", NameRU: "Производство", NameTG: "Истеҳсолот", NameEN: "Production",
		Permissions: []string{
			"dashboard:read", "items:read", "inventory:read", "production:manage",
			"quality:read", "equipment:read", "documents:read",
		},
	},
	{
		Key: "quality", NameRU: "Качество", NameTG: "Сифат", NameEN: "Quality",
		Permissions: []string{
			"dashboard:read", "items:read", "inventory:read", "production:read",
			"quality:manage", "quality:approve", "documents:read",
		},
	},
}

// ---------------------------------------------------------------------------
// Products — docs/01-DECISIONS.md D8
// ---------------------------------------------------------------------------

// SeedItem is one of the five real finished goods. There are exactly five
// (D8). The CRM prototype's pomegranate juice, apricot juice, strawberry jam and
// 1.5 л water were design filler and must not appear anywhere in the system.
type SeedItem struct {
	SKU      string
	Category string
	BaseUOM  string
	NameRU   string
	// Packaging beyond the base consumer unit. Only the configurations the docs
	// actually name are seeded; inventing case sizes would be fabricating data
	// the client has to confirm.
	Cases []SeedCase
}

type SeedCase struct {
	Code      string
	QtyInBase string
}

// SeedItems are the five approved products. Note that shelf life, composition and
// nutrition are deliberately absent: the client set the rule that the system must
// not publish unverified claims, so those fields stay null and render
// «уточняется» until the recipes are lab-verified (docs/02-SCHEMA.md:176).
var SeedItems = []SeedItem{
	{
		SKU: "APJ-1000", Category: "juice", BaseUOM: "bottle",
		NameRU: "Яблочный сок прямого отжима, 1 000 мл, стекло",
		// CASE12 is the configuration shown in docs/03-API-CONTRACT.md:216.
		Cases: []SeedCase{{Code: "CASE12", QtyInBase: "12.000"}},
	},
	{
		SKU: "APR-220", Category: "jam", BaseUOM: "jar",
		NameRU: "Абрикосовый джем, 212–228 мл, стекло",
	},
	{
		SKU: "TOM-500", Category: "paste", BaseUOM: "jar",
		NameRU: "Томатная паста, 500 мл, стекло",
	},
	{
		SKU: "WAT-500", Category: "water", BaseUOM: "bottle",
		NameRU: "Негазированная питьевая вода 0,5 л, ПЭТ",
		// CASE24 is named explicitly in D8: "WAT-500 × 24 is a selling unit of
		// WAT-500, not a separate SKU."
		Cases: []SeedCase{{Code: "CASE24", QtyInBase: "24.000"}},
	},
	{
		SKU: "WAT-1000", Category: "water", BaseUOM: "bottle",
		NameRU: "Негазированная питьевая вода 1 л, ПЭТ",
	},
}

// baseUnitCode is the packaging code for one consumer unit.
func baseUnitCode(uom string) string {
	switch uom {
	case "jar":
		return "JAR"
	default:
		return "BOTTLE"
	}
}

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

// Result reports what the run changed, so a second run visibly does nothing.
type Result struct {
	RolesCreated        int
	PermissionsCreated  int
	ItemsCreated        int
	TranslationsCreated int
	PackagingCreated    int
	AdminCreated        bool
	AdminID             uuid.UUID
}

// Reference loads production-safe reference data. It is idempotent: running it
// twice creates nothing the second time and returns no error.
//
// Everything happens in one transaction. A half-seeded system — roles without
// their permissions, say — would grant access nobody intended.
func Reference(ctx context.Context, pool *pgxpool.Pool, adminEmail, adminPasswordHash, adminName string) (Result, error) {
	var res Result

	tx, err := pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("seed: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := seedRoles(ctx, tx, &res); err != nil {
		return res, err
	}
	if err := seedItems(ctx, tx, &res); err != nil {
		return res, err
	}
	if adminEmail != "" {
		if err := seedAdmin(ctx, tx, adminEmail, adminPasswordHash, adminName, &res); err != nil {
			return res, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("seed: commit: %w", err)
	}
	return res, nil
}

func seedRoles(ctx context.Context, tx pgx.Tx, res *Result) error {
	for _, r := range SeedRoles {
		var roleID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM roles WHERE key = $1 AND deleted_at IS NULL`, r.Key).Scan(&roleID)
		switch {
		case err == pgx.ErrNoRows:
			if err := tx.QueryRow(ctx, `
				INSERT INTO roles (key, name_ru, name_tg, name_en, is_system)
				VALUES ($1, $2, $3, $4, true) RETURNING id`,
				r.Key, r.NameRU, r.NameTG, r.NameEN).Scan(&roleID); err != nil {
				return fmt.Errorf("seed: create role %s: %w", r.Key, err)
			}
			res.RolesCreated++
		case err != nil:
			return fmt.Errorf("seed: look up role %s: %w", r.Key, err)
		}

		for _, p := range r.Permissions {
			perm, err := rbac.ParsePermission(p)
			if err != nil {
				return fmt.Errorf("seed: role %s: %w", r.Key, err)
			}
			// Validated against the resource list, so a typo in the matrix is a
			// startup failure rather than a permission nobody notices is missing.
			if !rbac.ValidResource(perm.Resource) {
				return fmt.Errorf("seed: role %s grants unknown resource %q", r.Key, perm.Resource)
			}
			if perm.Action == rbac.Approve && !rbac.ApproveResources[perm.Resource] {
				return fmt.Errorf("seed: role %s grants %s, but approve is not defined for that resource",
					r.Key, p)
			}

			tag, err := tx.Exec(ctx, `
				INSERT INTO role_permissions (role_id, resource, action)
				VALUES ($1, $2, $3)
				ON CONFLICT DO NOTHING`, roleID, perm.Resource, string(perm.Action))
			if err != nil {
				return fmt.Errorf("seed: grant %s to %s: %w", p, r.Key, err)
			}
			res.PermissionsCreated += int(tag.RowsAffected())
		}
	}
	return nil
}

func seedItems(ctx context.Context, tx pgx.Tx, res *Result) error {
	for _, it := range SeedItems {
		var itemID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM items WHERE sku = $1 AND deleted_at IS NULL`, it.SKU).Scan(&itemID)
		switch {
		case err == pgx.ErrNoRows:
			// status starts 'active': for a finished good that IS the website
			// publication state (docs/02-SCHEMA.md:141).
			if err := tx.QueryRow(ctx, `
				INSERT INTO items (sku, item_type, category, base_uom, status)
				VALUES ($1, 'finished_good', $2, $3, 'active') RETURNING id`,
				it.SKU, it.Category, it.BaseUOM).Scan(&itemID); err != nil {
				return fmt.Errorf("seed: create item %s: %w", it.SKU, err)
			}
			res.ItemsCreated++
		case err != nil:
			return fmt.Errorf("seed: look up item %s: %w", it.SKU, err)
		}

		// Russian only. Tajik and English product names come from the translation
		// vendor (D10); inventing them here would publish unreviewed content, and
		// a missing locale row correctly falls back to ru in the frontend
		// (docs/02-SCHEMA.md:53).
		tag, err := tx.Exec(ctx, `
			INSERT INTO item_translations (item_id, locale, name)
			VALUES ($1, 'ru', $2)
			ON CONFLICT DO NOTHING`, itemID, it.NameRU)
		if err != nil {
			return fmt.Errorf("seed: translate %s: %w", it.SKU, err)
		}
		res.TranslationsCreated += int(tag.RowsAffected())

		units := append([]SeedCase{{Code: baseUnitCode(it.BaseUOM), QtyInBase: "1.000"}}, it.Cases...)
		for _, u := range units {
			tag, err := tx.Exec(ctx, `
				INSERT INTO packaging_units (item_id, code, qty_in_base)
				VALUES ($1, $2, $3::numeric)
				ON CONFLICT DO NOTHING`, itemID, u.Code, u.QtyInBase)
			if err != nil {
				return fmt.Errorf("seed: packaging %s/%s: %w", it.SKU, u.Code, err)
			}
			res.PackagingCreated += int(tag.RowsAffected())
		}
	}
	return nil
}

func seedAdmin(ctx context.Context, tx pgx.Tx, email, passwordHash, name string, res *Result) error {
	var userID uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, email).Scan(&userID)
	switch {
	case err == pgx.ErrNoRows:
		if passwordHash == "" {
			return fmt.Errorf("seed: admin password is required to create %s", email)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, full_name, password_hash)
			VALUES ($1, $2, $3) RETURNING id`, email, name, passwordHash).Scan(&userID); err != nil {
			return fmt.Errorf("seed: create admin: %w", err)
		}
		res.AdminCreated = true
	case err != nil:
		return fmt.Errorf("seed: look up admin: %w", err)
	}
	res.AdminID = userID

	var roleID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM roles WHERE key = 'admin' AND deleted_at IS NULL`).Scan(&roleID); err != nil {
		return fmt.Errorf("seed: find admin role: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
		return fmt.Errorf("seed: assign admin role: %w", err)
	}
	return nil
}
