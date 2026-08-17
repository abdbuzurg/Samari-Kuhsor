// Package items implements Товары и цены — the product master.
//
// docs/05-MODULES.md §4. This is the reference slice: the shapes here are copied
// by every module after it, so they are chosen deliberately rather than
// conveniently.
//
// Two rules from the decision record are enforced here rather than trusted:
//
//   - SKU prefixes (D8). Raw materials are RAW-, packaging is PKG-. Finished
//     goods keep the five approved codes, which share no prefix, so their format
//     is not constrained.
//   - Unverified claims (docs/02-SCHEMA.md:176). Composition, nutrition and shelf
//     life may be left null and render «уточняется». The client set that rule
//     explicitly: the system must not publish claims that are not lab-verified.
package items

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Resource is the module key used in permissions and in the audit trail.
const Resource = rbac.Items

// Valid enumerations, mirroring the CHECK constraints in migration 00001. Kept
// here too so a bad value is a 400 with a useful message rather than a 500 from
// a constraint violation.
var (
	ItemTypes = []string{"finished_good", "raw_material", "packaging"}
	Statuses  = []string{"draft", "active", "archived"}
	Locales   = []string{"ru", "tg", "en"}
)

// SortSpec is the whitelist of sortable fields. Must match the CASE branches in
// ListItems: a field allowed here but absent there would silently fall through
// to the id tiebreaker and appear to do nothing.
var SortSpec = common.SortSpec{
	Allowed:     []string{"sku", "category", "status", "created_at"},
	Default:     "created_at",
	DefaultDesc: true,
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListFilter carries the resource-specific query parameters.
type ListFilter struct {
	ItemType *string
	Status   *string
	Category *string
	Locale   string
}

// ListRow is one row of the list view, with the extras the columns need.
type ListRow struct {
	Item           db.Item
	DisplayName    string
	PackagingCodes []string
	CurrentPrice   *db.ItemPrice
}

func (s *Service) List(ctx context.Context, p common.Params, f ListFilter) ([]ListRow, int64, error) {
	q := db.New(s.pool)

	locale := f.Locale
	if locale == "" {
		locale = "ru"
	}

	rows, err := q.ListItems(ctx, db.ListItemsParams{
		Locale:    locale,
		ItemType:  f.ItemType,
		Status:    f.Status,
		Category:  f.Category,
		Q:         nilIfEmpty(p.Query),
		SortField: p.SortField,
		SortDesc:  p.SortDesc,
		Off:       p.Offset(),
		Lim:       p.Limit(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("items: list: %w", err)
	}

	total, err := q.CountListItems(ctx, db.CountListItemsParams{
		ItemType: f.ItemType,
		Status:   f.Status,
		Category: f.Category,
		Q:        nilIfEmpty(p.Query),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("items: count: %w", err)
	}

	out := make([]ListRow, 0, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, ListRow{Item: r.Item, DisplayName: r.DisplayName})
		ids = append(ids, r.Item.ID)
	}
	if len(ids) == 0 {
		return out, total, nil
	}

	// Two batch queries for the whole page rather than two per row.
	units, err := q.ListPackagingUnitsForItems(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("items: packaging: %w", err)
	}
	byItem := map[uuid.UUID][]string{}
	for _, u := range units {
		byItem[u.ItemID] = append(byItem[u.ItemID], u.Code)
	}

	prices, err := q.ListCurrentPricesForItems(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("items: prices: %w", err)
	}
	priceByItem := map[uuid.UUID]db.ItemPrice{}
	for _, pr := range prices {
		priceByItem[pr.ItemID] = pr
	}

	for i := range out {
		out[i].PackagingCodes = byItem[out[i].Item.ID]
		if pr, ok := priceByItem[out[i].Item.ID]; ok {
			out[i].CurrentPrice = &pr
		}
	}
	return out, total, nil
}

// Detail is everything the detail view needs (docs/05-MODULES.md §2 and §4).
type Detail struct {
	Item         db.Item
	Translations []db.ItemTranslation
	Packaging    []db.PackagingUnit
	Prices       []db.ItemPrice
	CurrentPrice *db.ItemPrice
	Batches      []db.Batch
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Detail, error) {
	q := db.New(s.pool)

	item, err := q.GetItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, common.NotFound()
		}
		return Detail{}, fmt.Errorf("items: get: %w", err)
	}

	d := Detail{Item: item}
	if d.Translations, err = q.ListItemTranslations(ctx, id); err != nil {
		return Detail{}, fmt.Errorf("items: translations: %w", err)
	}
	if d.Packaging, err = q.ListPackagingUnits(ctx, id); err != nil {
		return Detail{}, fmt.Errorf("items: packaging: %w", err)
	}
	if d.Prices, err = q.ListItemPrices(ctx, id); err != nil {
		return Detail{}, fmt.Errorf("items: prices: %w", err)
	}
	// The batch list is capped: the detail view shows recent batches, and a SKU
	// in production for a year should not return thousands of rows to render ten.
	if d.Batches, err = q.ListBatchesForItem(ctx, db.ListBatchesForItemParams{ItemID: id, Limit: 20}); err != nil {
		return Detail{}, fmt.Errorf("items: batches: %w", err)
	}

	if price, err := q.GetCurrentItemPrice(ctx, id); err == nil {
		d.CurrentPrice = &price
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, fmt.Errorf("items: current price: %w", err)
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Writes
// ---------------------------------------------------------------------------

// TranslationInput is the per-locale content for an item.
type TranslationInput struct {
	Locale            string
	Name              string
	Description       *string
	Ingredients       *string
	Nutrition         *string
	StorageConditions *string
	AfterOpening      *string
}

// PackagingInput is a selling unit. A case is a unit, not a product (D8).
type PackagingInput struct {
	Code      string
	QtyInBase decimal.Decimal
	Barcode   *string
}

// CreateInput is a new product.
type CreateInput struct {
	SKU           string
	ItemType      string
	Category      *string
	BaseUOM       string
	ShelfLifeDays *int32
	MinQty        *decimal.Decimal
	Status        string
	Translations  []TranslationInput
	Packaging     []PackagingInput
}

func (s *Service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (Detail, error) {
	if err := validateCreate(in); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("items: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	item, err := q.CreateItem(ctx, db.CreateItemParams{
		Sku:           in.SKU,
		ItemType:      in.ItemType,
		Category:      in.Category,
		BaseUom:       in.BaseUOM,
		ShelfLifeDays: in.ShelfLifeDays,
		MinQty:        nullDecimal(in.MinQty),
		Status:        in.Status,
		CreatedBy:     uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return Detail{}, mapWriteError(err, in.SKU)
	}

	for _, t := range in.Translations {
		if _, err := q.UpsertItemTranslation(ctx, db.UpsertItemTranslationParams{
			ItemID:            item.ID,
			Locale:            t.Locale,
			Name:              t.Name,
			Description:       t.Description,
			Ingredients:       t.Ingredients,
			Nutrition:         t.Nutrition,
			StorageConditions: t.StorageConditions,
			AfterOpening:      t.AfterOpening,
			CreatedBy:         uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return Detail{}, fmt.Errorf("items: translation %s: %w", t.Locale, err)
		}
	}

	for _, p := range in.Packaging {
		if _, err := q.CreatePackagingUnit(ctx, db.CreatePackagingUnitParams{
			ItemID:    item.ID,
			Code:      p.Code,
			QtyInBase: p.QtyInBase,
			Barcode:   p.Barcode,
			CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return Detail{}, mapPackagingError(err, p.Code)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(item.ID),
		After:      auditSnapshot(item),
	}); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("items: commit: %w", err)
	}
	return s.Get(ctx, item.ID)
}

// UpdateInput is a partial update. SKU and item_type are deliberately absent:
// changing either after batches and stock movements reference the item would
// rewrite history that other records point at.
type UpdateInput struct {
	Version       int32
	Category      *string
	BaseUOM       string
	ShelfLifeDays *int32
	MinQty        *decimal.Decimal
	Status        string
	Translations  []TranslationInput
}

func (s *Service) Update(ctx context.Context, actor uuid.UUID, id uuid.UUID, in UpdateInput) (Detail, error) {
	if err := validateUpdate(in); err != nil {
		return Detail{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, fmt.Errorf("items: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	before, err := q.GetItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, common.NotFound()
		}
		return Detail{}, fmt.Errorf("items: load: %w", err)
	}

	after, err := q.UpdateItem(ctx, db.UpdateItemParams{
		ID:              id,
		Category:        in.Category,
		BaseUom:         in.BaseUOM,
		ShelfLifeDays:   in.ShelfLifeDays,
		MinQty:          nullDecimal(in.MinQty),
		Status:          in.Status,
		ExpectedVersion: in.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The row exists (we just read it) but the version guard rejected the
			// write, so someone else committed in between.
			return Detail{}, common.VersionConflict(before.Version)
		}
		return Detail{}, fmt.Errorf("items: update: %w", err)
	}

	for _, t := range in.Translations {
		if _, err := q.UpsertItemTranslation(ctx, db.UpsertItemTranslationParams{
			ItemID:            id,
			Locale:            t.Locale,
			Name:              t.Name,
			Description:       t.Description,
			Ingredients:       t.Ingredients,
			Nutrition:         t.Nutrition,
			StorageConditions: t.StorageConditions,
			AfterOpening:      t.AfterOpening,
			CreatedBy:         uuid.NullUUID{UUID: actor, Valid: true},
		}); err != nil {
			return Detail{}, fmt.Errorf("items: translation %s: %w", t.Locale, err)
		}
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionUpdate,
		Resource:   Resource,
		ResourceID: audit.Target(id),
		Before:     auditSnapshot(before),
		After:      auditSnapshot(after),
	}); err != nil {
		return Detail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Detail{}, fmt.Errorf("items: commit: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete tombstones an item. There are no hard deletes (CLAUDE.md §4.3).
func (s *Service) Delete(ctx context.Context, actor uuid.UUID, id uuid.UUID, version int32) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("items: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	before, err := q.GetItemByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.NotFound()
		}
		return fmt.Errorf("items: load: %w", err)
	}

	if _, err := q.TombstoneItem(ctx, db.TombstoneItemParams{ID: id, ExpectedVersion: version}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return common.VersionConflict(before.Version)
		}
		return fmt.Errorf("items: tombstone: %w", err)
	}

	return commitWithAudit(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionDelete,
		Resource:   Resource,
		ResourceID: audit.Target(id),
		Before:     auditSnapshot(before),
	})
}

// PriceInput adds a price to the history.
type PriceInput struct {
	Amount    decimal.Decimal
	Currency  string
	ValidFrom pgtype.Date
	ValidTo   pgtype.Date
}

// AddPrice records a new price, closing the currently open one.
//
// Prices are never overwritten. The price history is shown on the detail view
// and is the record of what a product cost when an order was placed.
func (s *Service) AddPrice(ctx context.Context, actor uuid.UUID, itemID uuid.UUID, in PriceInput) (db.ItemPrice, error) {
	if in.Amount.IsNegative() {
		return db.ItemPrice{}, common.Validation(common.FieldError{
			Field: "amount", Code: "invalid", Message: "Цена не может быть отрицательной",
		})
	}
	if !in.ValidFrom.Valid {
		return db.ItemPrice{}, common.Validation(common.FieldError{
			Field: "valid_from", Code: "required", Message: "Укажите дату начала действия цены",
		})
	}
	if in.ValidTo.Valid && in.ValidTo.Time.Before(in.ValidFrom.Time) {
		return db.ItemPrice{}, common.Validation(common.FieldError{
			Field: "valid_to", Code: "invalid", Message: "Дата окончания раньше даты начала",
		})
	}
	currency := in.Currency
	if currency == "" {
		currency = "TJS" // base currency (CLAUDE.md §4.6)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.ItemPrice{}, fmt.Errorf("items: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if _, err := q.GetItemByID(ctx, itemID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ItemPrice{}, common.NotFound()
		}
		return db.ItemPrice{}, fmt.Errorf("items: load: %w", err)
	}

	if err := q.CloseOpenItemPrices(ctx, db.CloseOpenItemPricesParams{
		ItemID: itemID, NewValidFrom: in.ValidFrom,
	}); err != nil {
		return db.ItemPrice{}, fmt.Errorf("items: close prices: %w", err)
	}

	price, err := q.CreateItemPrice(ctx, db.CreateItemPriceParams{
		ItemID:    itemID,
		Currency:  currency,
		Amount:    in.Amount,
		ValidFrom: in.ValidFrom,
		ValidTo:   in.ValidTo,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.ItemPrice{}, fmt.Errorf("items: create price: %w", err)
	}

	if err := commitWithAudit(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionUpdate,
		Resource:   Resource,
		ResourceID: audit.Target(itemID),
		After: map[string]any{
			"price_added": price.Amount.StringFixed(2),
			"currency":    price.Currency,
			"valid_from":  in.ValidFrom.Time.Format("2006-01-02"),
		},
	}); err != nil {
		return db.ItemPrice{}, err
	}
	return price, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func validateCreate(in CreateInput) error {
	var details []common.FieldError

	sku := strings.TrimSpace(in.SKU)
	if sku == "" {
		details = append(details, common.FieldError{
			Field: "sku", Code: "required", Message: "Укажите артикул",
		})
	}
	if !contains(ItemTypes, in.ItemType) {
		details = append(details, common.FieldError{
			Field: "item_type", Code: "invalid", Message: "Неизвестный тип позиции",
		})
	}
	// D8: raw materials and packaging carry their own prefixes. Finished goods
	// keep the five approved codes, which share no prefix, so they are exempt.
	if prefix, ok := requiredPrefix[in.ItemType]; ok && !strings.HasPrefix(sku, prefix) {
		details = append(details, common.FieldError{
			Field: "sku", Code: "prefix_required",
			Message: fmt.Sprintf("Артикул должен начинаться с %s", prefix),
		})
	}
	if strings.TrimSpace(in.BaseUOM) == "" {
		details = append(details, common.FieldError{
			Field: "base_uom", Code: "required", Message: "Укажите базовую единицу",
		})
	}
	if in.Status != "" && !contains(Statuses, in.Status) {
		details = append(details, common.FieldError{
			Field: "status", Code: "invalid", Message: "Неизвестный статус",
		})
	}
	details = append(details, validateTranslations(in.Translations)...)
	details = append(details, validatePackaging(in.Packaging)...)

	// A product with no name at all would render as its SKU everywhere.
	if len(in.Translations) == 0 {
		details = append(details, common.FieldError{
			Field: "translations", Code: "required", Message: "Укажите наименование хотя бы на одном языке",
		})
	}

	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func validateUpdate(in UpdateInput) error {
	var details []common.FieldError
	if strings.TrimSpace(in.BaseUOM) == "" {
		details = append(details, common.FieldError{
			Field: "base_uom", Code: "required", Message: "Укажите базовую единицу",
		})
	}
	if !contains(Statuses, in.Status) {
		details = append(details, common.FieldError{
			Field: "status", Code: "invalid", Message: "Неизвестный статус",
		})
	}
	details = append(details, validateTranslations(in.Translations)...)
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}

func validateTranslations(ts []TranslationInput) []common.FieldError {
	var details []common.FieldError
	seen := map[string]bool{}
	for _, t := range ts {
		if !contains(Locales, t.Locale) {
			// Catches the `tj` / `tg` confusion at the API boundary with a clear
			// message, instead of as a CHECK-constraint 500 (docs/07 C2).
			details = append(details, common.FieldError{
				Field: "translations." + t.Locale, Code: "invalid",
				Message: "Поддерживаются языки ru, tg, en",
			})
			continue
		}
		if seen[t.Locale] {
			details = append(details, common.FieldError{
				Field: "translations." + t.Locale, Code: "duplicate",
				Message: "Язык указан дважды",
			})
		}
		seen[t.Locale] = true
		if strings.TrimSpace(t.Name) == "" {
			details = append(details, common.FieldError{
				Field: "translations." + t.Locale + ".name", Code: "required",
				Message: "Укажите наименование",
			})
		}
	}
	return details
}

func validatePackaging(ps []PackagingInput) []common.FieldError {
	var details []common.FieldError
	seen := map[string]bool{}
	for _, p := range ps {
		code := strings.TrimSpace(p.Code)
		if code == "" {
			details = append(details, common.FieldError{
				Field: "packaging_units.code", Code: "required", Message: "Укажите код упаковки",
			})
			continue
		}
		if seen[code] {
			details = append(details, common.FieldError{
				Field: "packaging_units." + code, Code: "duplicate", Message: "Код упаковки указан дважды",
			})
		}
		seen[code] = true
		// A packaging unit of zero or negative base units is not a unit; it would
		// make any conversion through it produce nonsense quantities.
		if !p.QtyInBase.IsPositive() {
			details = append(details, common.FieldError{
				Field: "packaging_units." + code + ".qty_in_base", Code: "invalid",
				Message: "Количество в базовых единицах должно быть больше нуля",
			})
		}
	}
	return details
}

// requiredPrefix per D8. Finished goods are absent on purpose: APJ-, APR-, TOM-
// and WAT- share no prefix and the client has already approved those codes.
var requiredPrefix = map[string]string{
	"raw_material": "RAW-",
	"packaging":    "PKG-",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mapWriteError turns a constraint violation into the contract's error shape.
// A duplicate SKU is the user's mistake, not a server fault, so it must not
// surface as a 500 with a Postgres message.
func mapWriteError(err error, sku string) error {
	if isUniqueViolation(err, "items_sku_key") {
		return common.Validation(common.FieldError{
			Field: "sku", Code: "already_exists",
			Message: fmt.Sprintf("Артикул %s уже используется", sku),
		})
	}
	return fmt.Errorf("items: create: %w", err)
}

func mapPackagingError(err error, code string) error {
	if isUniqueViolation(err, "packaging_units_key") {
		return common.Validation(common.FieldError{
			Field: "packaging_units." + code, Code: "already_exists",
			Message: fmt.Sprintf("Упаковка %s уже определена", code),
		})
	}
	return fmt.Errorf("items: packaging: %w", err)
}

func isUniqueViolation(err error, constraint string) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == pgerrcode.UniqueViolation &&
			strings.Contains(pgErr.ConstraintName, constraint)
	}
	return false
}

// auditSnapshot is the before/after payload. Deliberately a projection rather
// than the whole row: audit entries are read by humans investigating a change,
// and burying the three fields that moved under fifteen that did not makes the
// trail harder to use, not more complete.
func auditSnapshot(i db.Item) map[string]any {
	snap := map[string]any{
		"sku":       i.Sku,
		"item_type": i.ItemType,
		"base_uom":  i.BaseUom,
		"status":    i.Status,
		"version":   i.Version,
	}
	if i.Category != nil {
		snap["category"] = *i.Category
	}
	if i.ShelfLifeDays != nil {
		snap["shelf_life_days"] = *i.ShelfLifeDays
	}
	if i.MinQty.Valid {
		snap["min_qty"] = i.MinQty.Decimal.StringFixed(3)
	}
	return snap
}

func commitWithAudit(ctx context.Context, tx pgx.Tx, e audit.Entry) error {
	if err := audit.Record(ctx, tx, e); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("items: commit: %w", err)
	}
	return nil
}

func contains(set []string, v string) bool { return slices.Contains(set, v) }

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func nullDecimal(d *decimal.Decimal) decimal.NullDecimal {
	if d == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *d, Valid: true}
}

// ---------------------------------------------------------------------------
// Public site
// ---------------------------------------------------------------------------

// PublicList returns the active finished goods an anonymous visitor may see.
func (s *Service) PublicList(ctx context.Context, locale string) ([]db.ListPublicProductsRow, error) {
	rows, err := db.New(s.pool).ListPublicProducts(ctx, locale)
	if err != nil {
		return nil, fmt.Errorf("items: public list: %w", err)
	}
	return rows, nil
}

// PublicOne returns one product by SKU.
//
// Keyed on SKU rather than id because the public URL is /catalogue/APJ-1000: a
// UUID in a customer-facing address is unreadable, unmemorable, and impossible
// to quote over the phone — which visitors to a wholesale supplier do.
func (s *Service) PublicOne(ctx context.Context, locale, sku string) (db.GetPublicProductRow, error) {
	row, err := db.New(s.pool).GetPublicProduct(ctx, db.GetPublicProductParams{
		Locale: locale, Sku: sku,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetPublicProductRow{}, common.NotFound()
		}
		return db.GetPublicProductRow{}, fmt.Errorf("items: public product: %w", err)
	}
	return row, nil
}

// PublicNews returns published news posts.
func (s *Service) PublicNews(ctx context.Context, locale string, limit int32) ([]db.ListPublicNewsRow, error) {
	rows, err := db.New(s.pool).ListPublicNews(ctx, db.ListPublicNewsParams{
		Locale: locale, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("items: public news: %w", err)
	}
	return rows, nil
}
