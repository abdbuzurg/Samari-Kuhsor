// Package api holds every payload type the HTTP layer sends or receives.
//
// These Go types are the SINGLE SOURCE OF TRUTH for the API contract. The
// TypeScript in packages/types is generated from this package by tygo, and
// `make check` fails if the generated output is stale
// (docs/07-IMPLEMENTATION-PLAN.md I3).
//
// That matters because docs/03-API-CONTRACT.md:265 names drift between the Go and
// TypeScript payload shapes as "the most likely silent bug in this architecture".
// Generating one from the other makes the drift structurally impossible rather
// than merely discouraged.
//
// Rules for anything added here:
//   - Money and quantities are `common.Money` / `common.Quantity`, which
//     serialise as STRINGS. Never float64, never int cents (CLAUDE.md §4.6).
//   - Timestamps are strings in RFC 3339 UTC.
//   - Status fields are `common.Status`: the backend decides the semantic level,
//     never a React component (docs/03-API-CONTRACT.md:177).
//   - Every mutable resource carries `version` for optimistic concurrency (§7).
package api

import "github.com/qoim/samari/backend/internal/http/common"

// ---------------------------------------------------------------------------
// Envelope — docs/03-API-CONTRACT.md §4
// ---------------------------------------------------------------------------

// PageMeta is the metadata on every collection response.
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// FieldError is one entry in an error's details array. The frontend switches on
// Code, never on Message (docs/03-API-CONTRACT.md:116).
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError is the body of every error response, at every status code.
type APIError struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

// ErrorEnvelope wraps APIError, matching the wire shape exactly.
type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

// ---------------------------------------------------------------------------
// Status — docs/03-API-CONTRACT.md §8
// ---------------------------------------------------------------------------

// Status drives the prototype's coloured tags. Per docs/07 C3 the frontend
// renders the label from its i18n dictionary keyed on Key; Label is the Russian
// fallback for a missing key.
type Status struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Level string `json:"level"` // ok | warn | danger | info | neutral
}

// ---------------------------------------------------------------------------
// Auth — docs/03-API-CONTRACT.md §9
// ---------------------------------------------------------------------------

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse carries the opaque session token for the BFF to put in an
// httpOnly cookie. The API never sets a cookie itself (docs/07 I8).
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Role is one of the caller's roles.
//
// Names are carried in all three languages rather than pre-localised, because a
// role name is CONTENT, not UI chrome (CLAUDE.md §6) — administrators create
// roles and name them. docs/02-SCHEMA.md:56 makes `roles` one of two deliberate
// exceptions to the sibling-translation-table rule for exactly this reason, and
// the frontend picks the column matching the interface language.
type Role struct {
	Key    string `json:"key"`
	NameRU string `json:"name_ru"`
	NameTG string `json:"name_tg"`
	NameEN string `json:"name_en"`
}

// User is the caller's own identity, as returned by /auth/me.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Roles    []Role `json:"roles"`
	// Permissions is the flat "resource:action" list the CRM uses to hide nav
	// entries and buttons. Hiding is presentation only — the server still enforces
	// every request (docs/04-RBAC.md:120).
	Permissions []string `json:"permissions"`
}

// ---------------------------------------------------------------------------
// Items — docs/05-MODULES.md §4. The reference slice (T11).
// ---------------------------------------------------------------------------

// ItemTranslation is the per-locale content for an item. Null fields render
// «уточняется» until the client's recipes are lab-verified (docs/02-SCHEMA.md:176).
type ItemTranslation struct {
	Name              string  `json:"name"`
	Description       *string `json:"description" tstype:"string | null"`
	Ingredients       *string `json:"ingredients" tstype:"string | null"`
	Nutrition         *string `json:"nutrition" tstype:"string | null"`
	StorageConditions *string `json:"storage_conditions" tstype:"string | null"`
	AfterOpening      *string `json:"after_opening" tstype:"string | null"`
}

// PackagingUnit is a selling unit of an item. A case is a unit, not a product:
// WAT-500 × 24 is a packaging unit of WAT-500, not a separate SKU (D8).
type PackagingUnit struct {
	Code      string          `json:"code"`
	QtyInBase common.Quantity `json:"qty_in_base"`
	Barcode   *string         `json:"barcode" tstype:"string | null"` // EAN-13, null pending register Q4
}

type Price struct {
	Amount    common.Money `json:"amount"`
	Currency  string       `json:"currency"`
	ValidFrom string       `json:"valid_from"`
	ValidTo   *string      `json:"valid_to" tstype:"string | null"`
}

// Item is a product-master record. status = "active" IS the website publication
// state; there is no separate publication flag (docs/02-SCHEMA.md:141).
type Item struct {
	ID             string                     `json:"id"`
	SKU            string                     `json:"sku"`
	ItemType       string                     `json:"item_type"` // finished_good | raw_material | packaging
	Category       *string                    `json:"category" tstype:"string | null"`
	BaseUOM        string                     `json:"base_uom"`
	ShelfLifeDays  *int32                     `json:"shelf_life_days" tstype:"number | null"`
	MinQty         *common.Quantity           `json:"min_qty" tstype:"string | null"`
	Translations   map[string]ItemTranslation `json:"translations"`
	PackagingUnits []PackagingUnit            `json:"packaging_units"`
	CurrentPrice   *Price                     `json:"current_price" tstype:"Price | null"`
	PriceHistory   []Price                    `json:"price_history"`
	Status         Status                     `json:"status"`
	Version        int32                      `json:"version"`
	CreatedAt      string                     `json:"created_at"`
	UpdatedAt      string                     `json:"updated_at"`
}

// ItemListRow is one row of the Товары list. Deliberately narrower than Item:
// the columns are SKU · Наименование · Категория · Упаковка · Цена · Срок годн. ·
// Статус (docs/05-MODULES.md:85), and sending full translation sets and price
// history for 50 rows would be wasted bytes on a Khorog connection.
type ItemListRow struct {
	ID  string `json:"id"`
	SKU string `json:"sku"`
	// Name is already resolved to the requested locale, falling back to Russian
	// and then to the SKU. A list needs one label, not a translation set.
	Name           string   `json:"name"`
	ItemType       string   `json:"item_type"`
	Category       *string  `json:"category" tstype:"string | null"`
	BaseUOM        string   `json:"base_uom"`
	PackagingCodes []string `json:"packaging_codes"`
	ShelfLifeDays  *int32   `json:"shelf_life_days" tstype:"number | null"`
	CurrentPrice   *Price   `json:"current_price" tstype:"Price | null"`
	Status         Status   `json:"status"`
	Version        int32    `json:"version"`
}

// ItemTranslationWrite is the per-locale content on create and update.
//
// Ingredients, Nutrition and the rest may be omitted: docs/02-SCHEMA.md:176
// requires them to stay null until the client's recipes are lab-verified, and
// the UI renders «уточняется». The system must not publish unverified claims.
type ItemTranslationWrite struct {
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	Ingredients       *string `json:"ingredients"`
	Nutrition         *string `json:"nutrition"`
	StorageConditions *string `json:"storage_conditions"`
	AfterOpening      *string `json:"after_opening"`
}

// PackagingUnitWrite defines a selling unit. QtyInBase is a STRING for the same
// reason every quantity is (docs/03-API-CONTRACT.md:147).
type PackagingUnitWrite struct {
	Code      string  `json:"code"`
	QtyInBase string  `json:"qty_in_base"`
	Barcode   *string `json:"barcode"`
}

// ItemWriteRequest is the create and update payload.
//
// SKU and ItemType are ignored on update: changing either once batches and stock
// movements reference the item would rewrite history other records point at.
type ItemWriteRequest struct {
	// Version is required on PATCH and absent on POST
	// (docs/03-API-CONTRACT.md §7).
	Version        *int32                          `json:"version"`
	SKU            string                          `json:"sku"`
	ItemType       string                          `json:"item_type"`
	Category       *string                         `json:"category"`
	BaseUOM        string                          `json:"base_uom"`
	ShelfLifeDays  *int32                          `json:"shelf_life_days"`
	MinQty         *string                         `json:"min_qty"`
	Status         string                          `json:"status"`
	Translations   map[string]ItemTranslationWrite `json:"translations"`
	PackagingUnits []PackagingUnitWrite            `json:"packaging_units"`
}

// PriceWriteRequest adds a price. Prices are never overwritten: the new one
// supersedes the open one and the history is kept.
type PriceWriteRequest struct {
	Amount    string  `json:"amount"`
	Currency  string  `json:"currency"`
	ValidFrom string  `json:"valid_from"`
	ValidTo   *string `json:"valid_to"`
}
