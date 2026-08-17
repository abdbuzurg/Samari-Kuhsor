package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/items"
	"github.com/qoim/samari/backend/internal/http/common"
)

// Товары и цены. docs/05-MODULES.md §4, docs/03-API-CONTRACT.md §10.
//
// Handlers stay thin on purpose: parse, delegate, present. Business rules live in
// internal/domain/items so they are testable without HTTP and so the next eleven
// modules copy a shape that keeps them there.

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	p, err := common.ParseParams(r, items.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	q := r.URL.Query()
	filter := items.ListFilter{
		ItemType: optionalParam(q.Get("item_type")),
		Status:   optionalParam(q.Get("status")),
		Category: optionalParam(q.Get("category")),
		Locale:   localeOf(r),
	}

	rows, total, err := s.svc.Items.List(r.Context(), p, filter)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	out := make([]api.ItemListRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, listRowResponse(row))
	}
	common.List(w, out, common.NewPageMeta(p, total))
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	detail, err := s.svc.Items.Get(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, itemResponse(detail))
}

func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}

	var req api.ItemWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}

	minQty, err := optionalDecimal(req.MinQty, "min_qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	packaging, err := packagingInputs(req.PackagingUnits)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	status := req.Status
	if status == "" {
		status = "draft" // matches the column default; a new product is not published by accident
	}

	detail, err := s.svc.Items.Create(r.Context(), ident.User.ID, items.CreateInput{
		SKU:           req.SKU,
		ItemType:      req.ItemType,
		Category:      req.Category,
		BaseUOM:       req.BaseUOM,
		ShelfLifeDays: req.ShelfLifeDays,
		MinQty:        minQty,
		Status:        status,
		Translations:  translationInputs(req.Translations),
		Packaging:     packaging,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, itemResponse(detail))
}

func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	var req api.ItemWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	version, err := common.RequireVersion(common.Versioned{Version: req.Version})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	minQty, err := optionalDecimal(req.MinQty, "min_qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	detail, err := s.svc.Items.Update(r.Context(), ident.User.ID, id, items.UpdateInput{
		Version:       version,
		Category:      req.Category,
		BaseUOM:       req.BaseUOM,
		ShelfLifeDays: req.ShelfLifeDays,
		MinQty:        minQty,
		Status:        req.Status,
		Translations:  translationInputs(req.Translations),
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, itemResponse(detail))
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	// A tombstone is a mutation and carries the same optimistic-concurrency
	// guard as an edit: deleting a record someone else just changed must fail
	// the same way editing it would.
	var req common.Versioned
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	version, err := common.RequireVersion(req)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	if err := s.svc.Items.Delete(r.Context(), ident.User.ID, id, version); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

func (s *Server) handleAddItemPrice(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	var req api.PriceWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		common.Fail(w, r, common.Validation(common.FieldError{
			Field: "amount", Code: "invalid", Message: "Некорректная сумма",
		}))
		return
	}
	validFrom, err := parseDate(req.ValidFrom, "valid_from", true)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	validTo, err := parseDate(deref(req.ValidTo), "valid_to", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	price, err := s.svc.Items.AddPrice(r.Context(), ident.User.ID, id, items.PriceInput{
		Amount:    amount,
		Currency:  req.Currency,
		ValidFrom: validFrom,
		ValidTo:   validTo,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, priceResponse(price))
}

// ---------------------------------------------------------------------------
// Presentation
// ---------------------------------------------------------------------------

// itemStatus maps the stored status to its semantic level.
//
// The BACKEND decides the level, never a React component
// (docs/03-API-CONTRACT.md:177). Statuses from docs/05-MODULES.md:86:
// Черновик neutral → Активен ok → Архив neutral.
func itemStatus(status string) api.Status {
	switch status {
	case "active":
		return api.Status{Key: status, Label: "Активен", Level: string(common.LevelOK)}
	case "archived":
		return api.Status{Key: status, Label: "Архив", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Черновик", Level: string(common.LevelNeutral)}
	}
}

func itemResponse(d items.Detail) api.Item {
	translations := map[string]api.ItemTranslation{}
	for _, t := range d.Translations {
		translations[t.Locale] = api.ItemTranslation{
			Name:              t.Name,
			Description:       t.Description,
			Ingredients:       t.Ingredients,
			Nutrition:         t.Nutrition,
			StorageConditions: t.StorageConditions,
			AfterOpening:      t.AfterOpening,
		}
	}

	units := make([]api.PackagingUnit, 0, len(d.Packaging))
	for _, u := range d.Packaging {
		units = append(units, api.PackagingUnit{
			Code:      u.Code,
			QtyInBase: common.NewQuantity(u.QtyInBase),
			Barcode:   u.Barcode, // null until register question Q4 is answered
		})
	}

	history := make([]api.Price, 0, len(d.Prices))
	for _, p := range d.Prices {
		history = append(history, priceResponse(p))
	}

	item := api.Item{
		ID:             d.Item.ID.String(),
		SKU:            d.Item.Sku,
		ItemType:       d.Item.ItemType,
		Category:       d.Item.Category,
		BaseUOM:        d.Item.BaseUom,
		ShelfLifeDays:  d.Item.ShelfLifeDays,
		MinQty:         common.NullQuantity(d.Item.MinQty),
		Translations:   translations,
		PackagingUnits: units,
		PriceHistory:   history,
		Status:         itemStatus(d.Item.Status),
		Version:        d.Item.Version,
		CreatedAt:      common.Timestamp(d.Item.CreatedAt),
		UpdatedAt:      common.Timestamp(d.Item.UpdatedAt),
	}
	if d.CurrentPrice != nil {
		p := priceResponse(*d.CurrentPrice)
		item.CurrentPrice = &p
	}
	return item
}

func listRowResponse(row items.ListRow) api.ItemListRow {
	codes := row.PackagingCodes
	if codes == nil {
		codes = []string{} // an array, never null — the client maps over it
	}
	out := api.ItemListRow{
		ID:             row.Item.ID.String(),
		SKU:            row.Item.Sku,
		Name:           row.DisplayName,
		ItemType:       row.Item.ItemType,
		Category:       row.Item.Category,
		BaseUOM:        row.Item.BaseUom,
		PackagingCodes: codes,
		ShelfLifeDays:  row.Item.ShelfLifeDays,
		Status:         itemStatus(row.Item.Status),
		Version:        row.Item.Version,
	}
	if row.CurrentPrice != nil {
		p := priceResponse(*row.CurrentPrice)
		out.CurrentPrice = &p
	}
	return out
}

func priceResponse(p db.ItemPrice) api.Price {
	return api.Price{
		Amount:    common.NewMoney(p.Amount),
		Currency:  p.Currency,
		ValidFrom: *common.Date(p.ValidFrom),
		ValidTo:   common.Date(p.ValidTo),
	}
}

// ---------------------------------------------------------------------------
// Parsing helpers
// ---------------------------------------------------------------------------

func translationInputs(in map[string]api.ItemTranslationWrite) []items.TranslationInput {
	out := make([]items.TranslationInput, 0, len(in))
	for locale, t := range in {
		out = append(out, items.TranslationInput{
			Locale:            locale,
			Name:              t.Name,
			Description:       t.Description,
			Ingredients:       t.Ingredients,
			Nutrition:         t.Nutrition,
			StorageConditions: t.StorageConditions,
			AfterOpening:      t.AfterOpening,
		})
	}
	return out
}

func packagingInputs(in []api.PackagingUnitWrite) ([]items.PackagingInput, error) {
	out := make([]items.PackagingInput, 0, len(in))
	for _, u := range in {
		// Quantities arrive as strings and are parsed through decimal, never
		// through float64 (docs/03-API-CONTRACT.md:147).
		qty, err := decimal.NewFromString(u.QtyInBase)
		if err != nil {
			return nil, common.Validation(common.FieldError{
				Field:   "packaging_units." + u.Code + ".qty_in_base",
				Code:    "invalid",
				Message: "Некорректное количество",
			})
		}
		out = append(out, items.PackagingInput{Code: u.Code, QtyInBase: qty, Barcode: u.Barcode})
	}
	return out, nil
}

func optionalDecimal(s *string, field string) (*decimal.Decimal, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil, common.Validation(common.FieldError{
			Field: field, Code: "invalid", Message: "Некорректное числовое значение",
		})
	}
	return &d, nil
}

func parseDate(s, field string, required bool) (pgtype.Date, error) {
	if s == "" {
		if required {
			return pgtype.Date{}, common.Validation(common.FieldError{
				Field: field, Code: "required", Message: "Укажите дату",
			})
		}
		return pgtype.Date{}, nil
	}
	var d pgtype.Date
	if err := d.Scan(s); err != nil {
		return pgtype.Date{}, common.Validation(common.FieldError{
			Field: field, Code: "invalid", Message: "Некорректная дата, ожидается ГГГГ-ММ-ДД",
		})
	}
	return d, nil
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	raw := chiURLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		// A malformed id is not found, not a server error — and reporting it as
		// 404 avoids confirming whether any particular id shape exists.
		return uuid.Nil, common.NotFound()
	}
	return id, nil
}

func optionalParam(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// localeOf reads the display locale for list labels. Defaults to Russian
// (CLAUDE.md §6); an unknown value falls back rather than erroring, because a
// stale bookmark must not break the page.
func localeOf(r *http.Request) string {
	switch r.URL.Query().Get("locale") {
	case "tg":
		return "tg"
	case "en":
		return "en"
	default:
		return "ru"
	}
}

// chiURLParam is a thin wrapper so handlers do not import chi directly.
func chiURLParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}
