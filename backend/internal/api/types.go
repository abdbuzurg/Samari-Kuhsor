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

// User is the caller's own identity, as returned by /auth/me.
type User struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Roles    []string `json:"roles"`
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
	Description       *string `json:"description"`
	Ingredients       *string `json:"ingredients"`
	Nutrition         *string `json:"nutrition"`
	StorageConditions *string `json:"storage_conditions"`
	AfterOpening      *string `json:"after_opening"`
}

// PackagingUnit is a selling unit of an item. A case is a unit, not a product:
// WAT-500 × 24 is a packaging unit of WAT-500, not a separate SKU (D8).
type PackagingUnit struct {
	Code      string          `json:"code"`
	QtyInBase common.Quantity `json:"qty_in_base"`
	Barcode   *string         `json:"barcode"` // EAN-13, null pending register Q4
}

type Price struct {
	Amount    common.Money `json:"amount"`
	Currency  string       `json:"currency"`
	ValidFrom string       `json:"valid_from"`
	ValidTo   *string      `json:"valid_to"`
}

// Item is a product-master record. status = "active" IS the website publication
// state; there is no separate publication flag (docs/02-SCHEMA.md:141).
type Item struct {
	ID             string                     `json:"id"`
	SKU            string                     `json:"sku"`
	ItemType       string                     `json:"item_type"` // finished_good | raw_material | packaging
	Category       *string                    `json:"category"`
	BaseUOM        string                     `json:"base_uom"`
	ShelfLifeDays  *int32                     `json:"shelf_life_days"`
	MinQty         *common.Quantity           `json:"min_qty"`
	Translations   map[string]ItemTranslation `json:"translations"`
	PackagingUnits []PackagingUnit            `json:"packaging_units"`
	CurrentPrice   *Price                     `json:"current_price"`
	Status         Status                     `json:"status"`
	Version        int32                      `json:"version"`
	CreatedAt      string                     `json:"created_at"`
	UpdatedAt      string                     `json:"updated_at"`
}
