package items

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
)

// CLAUDE.md §7 requires unit tests for every domain function, not only
// integration coverage through HTTP. Validation is pure logic, so it is tested
// directly: a table here runs in microseconds where the same cases through the
// API cost a database round trip each.

func ru(name string) []TranslationInput {
	return []TranslationInput{{Locale: "ru", Name: name}}
}

func validCreate() CreateInput {
	return CreateInput{
		SKU:          "APJ-1000",
		ItemType:     "finished_good",
		BaseUOM:      "bottle",
		Status:       "active",
		Translations: ru("Яблочный сок прямого отжима"),
		Packaging:    []PackagingInput{{Code: "BOTTLE", QtyInBase: decimal.RequireFromString("1")}},
	}
}

// fieldsOf returns the field names an error reported, so a test can assert which
// field was blamed rather than only that something failed.
func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	apiErr := common.AsError(err)
	if apiErr.Code != common.CodeValidationFailed {
		t.Fatalf("expected validation_failed, got %s", apiErr.Code)
	}
	out := make([]string, 0, len(apiErr.Details))
	for _, d := range apiErr.Details {
		out = append(out, d.Field)
	}
	return out
}

func TestValidateCreateAcceptsAValidProduct(t *testing.T) {
	t.Parallel()
	if err := validateCreate(validCreate()); err != nil {
		t.Fatalf("a valid product was rejected: %v", err)
	}
}

func TestValidateCreateBlamesTheRightField(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate    func(*CreateInput)
		wantField string
	}{
		"empty sku": {
			func(in *CreateInput) { in.SKU = "  " }, "sku",
		},
		"unknown item type": {
			func(in *CreateInput) { in.ItemType = "widget" }, "item_type",
		},
		"empty base uom": {
			func(in *CreateInput) { in.BaseUOM = "" }, "base_uom",
		},
		"unknown status": {
			func(in *CreateInput) { in.Status = "published" }, "status",
		},
		"no translations at all": {
			func(in *CreateInput) { in.Translations = nil }, "translations",
		},
		"blank name": {
			func(in *CreateInput) { in.Translations = ru("   ") }, "translations.ru.name",
		},
		// docs/07 C2 — the prototype's `tj` is not a locale in this system.
		"tj locale": {
			func(in *CreateInput) { in.Translations = []TranslationInput{{Locale: "tj", Name: "Оби себ"}} },
			"translations.tj",
		},
		"duplicate locale": {
			func(in *CreateInput) {
				in.Translations = []TranslationInput{
					{Locale: "ru", Name: "Сок"}, {Locale: "ru", Name: "Сок ещё раз"},
				}
			},
			"translations.ru",
		},
		"zero packaging quantity": {
			func(in *CreateInput) {
				in.Packaging = []PackagingInput{{Code: "BOTTLE", QtyInBase: decimal.Zero}}
			},
			"packaging_units.BOTTLE.qty_in_base",
		},
		"duplicate packaging code": {
			func(in *CreateInput) {
				in.Packaging = []PackagingInput{
					{Code: "CASE12", QtyInBase: decimal.RequireFromString("12")},
					{Code: "CASE12", QtyInBase: decimal.RequireFromString("24")},
				}
			},
			"packaging_units.CASE12",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := validCreate()
			c.mutate(&in)

			fields := fieldsOf(t, validateCreate(in))
			if len(fields) == 0 {
				t.Fatal("expected a validation error")
			}
			found := false
			for _, f := range fields {
				if f == c.wantField {
					found = true
				}
			}
			if !found {
				t.Errorf("blamed %v, expected %q — a message pointing at the wrong field is worse than none",
					fields, c.wantField)
			}
		})
	}
}

// D8 — raw materials are RAW-, packaging is PKG-. Finished goods keep the five
// approved codes, which share no prefix, so their format is unconstrained.
func TestSKUPrefixRule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sku, itemType string
		valid         bool
	}{
		{"RAW-SUG-50", "raw_material", true},
		{"SUG-50", "raw_material", false},
		{"PKG-CAP-82", "packaging", true},
		{"CAP-82", "packaging", false},
		{"PKG-SUG-50", "raw_material", false}, // right shape, wrong type
		{"RAW-CAP-82", "packaging", false},

		// The five approved finished goods, none sharing a prefix (D8).
		{"APJ-1000", "finished_good", true},
		{"APR-220", "finished_good", true},
		{"TOM-500", "finished_good", true},
		{"WAT-500", "finished_good", true},
		{"WAT-1000", "finished_good", true},
	}

	for _, c := range cases {
		t.Run(c.itemType+"/"+c.sku, func(t *testing.T) {
			in := validCreate()
			in.SKU, in.ItemType = c.sku, c.itemType

			err := validateCreate(in)
			if c.valid && err != nil {
				t.Errorf("%s should be valid for %s: %v", c.sku, c.itemType, err)
			}
			if !c.valid {
				fields := fieldsOf(t, err)
				if len(fields) == 0 {
					t.Fatalf("%s should be refused for %s", c.sku, c.itemType)
				}
				if !contains(fields, "sku") {
					t.Errorf("blamed %v, expected sku", fields)
				}
			}
		})
	}
}

// Multiple problems must be reported together. Fixing one field, resubmitting
// and being told about the next is a miserable way to enter a product.
func TestValidationReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()
	in := validCreate()
	in.SKU = ""
	in.BaseUOM = ""
	in.ItemType = "widget"

	fields := fieldsOf(t, validateCreate(in))
	if len(fields) < 3 {
		t.Errorf("reported %v; all three problems should come back together", fields)
	}
}

func TestValidateUpdate(t *testing.T) {
	t.Parallel()

	valid := UpdateInput{Version: 1, BaseUOM: "bottle", Status: "draft", Translations: ru("Сок")}
	if err := validateUpdate(valid); err != nil {
		t.Fatalf("a valid update was rejected: %v", err)
	}

	// Unlike create, an update need not carry translations — a status change is a
	// legitimate edit on its own.
	noTranslations := valid
	noTranslations.Translations = nil
	if err := validateUpdate(noTranslations); err != nil {
		t.Errorf("an update with no translations should be allowed: %v", err)
	}

	bad := valid
	bad.Status = "published"
	if fields := fieldsOf(t, validateUpdate(bad)); !contains(fields, "status") {
		t.Errorf("blamed %v, expected status", fields)
	}
}

// The audit projection must identify the record and carry the fields that
// change, without dragging in every column.
func TestAuditSnapshot(t *testing.T) {
	t.Parallel()

	category := "juice"
	shelfLife := int32(365)
	snap := auditSnapshot(db.Item{
		Sku:           "APJ-1000",
		ItemType:      "finished_good",
		BaseUom:       "bottle",
		Status:        "active",
		Version:       3,
		Category:      &category,
		ShelfLifeDays: &shelfLife,
		MinQty:        decimal.NullDecimal{Decimal: decimal.RequireFromString("12.5"), Valid: true},
	})

	for _, key := range []string{"sku", "item_type", "base_uom", "status", "version", "category"} {
		if _, ok := snap[key]; !ok {
			t.Errorf("audit snapshot is missing %q", key)
		}
	}
	// Quantities keep their scale even in the audit trail — never a float.
	if snap["min_qty"] != "12.500" {
		t.Errorf("min_qty = %v, want the exact decimal 12.500", snap["min_qty"])
	}

	// Absent optional fields are omitted rather than recorded as zero values: an
	// audit entry claiming shelf_life_days = 0 would be a fabricated reading.
	sparse := auditSnapshot(db.Item{Sku: "WAT-500", ItemType: "finished_good", BaseUom: "bottle", Status: "draft"})
	for _, key := range []string{"category", "shelf_life_days", "min_qty"} {
		if _, ok := sparse[key]; ok {
			t.Errorf("audit snapshot invented a value for absent field %q", key)
		}
	}
}

// The sort whitelist and the CASE branches in ListItems must agree. A field
// allowed here but missing there would fall through to the id tiebreaker and
// appear to do nothing — a bug with no error message.
func TestSortSpecMatchesTheQuery(t *testing.T) {
	t.Parallel()

	if err := SortSpec.Validate(); err != nil {
		t.Fatalf("sort spec is invalid: %v", err)
	}
	// Read straight from queries/items.sql.
	inQuery := []string{"sku", "category", "status", "created_at"}
	for _, f := range SortSpec.Allowed {
		if !contains(inQuery, f) {
			t.Errorf("%q is whitelisted but has no CASE branch in ListItems — sorting by it would silently do nothing", f)
		}
	}
	for _, f := range inQuery {
		if !contains(SortSpec.Allowed, f) {
			t.Errorf("%q has a CASE branch but is not whitelisted — it can never be requested", f)
		}
	}
}

func TestNilIfEmpty(t *testing.T) {
	t.Parallel()
	if nilIfEmpty("") != nil || nilIfEmpty("   ") != nil {
		t.Error("blank search strings must become nil, or the query filters on whitespace")
	}
	if got := nilIfEmpty("сок"); got == nil || *got != "сок" {
		t.Errorf("got %v", got)
	}
}

func TestEnumerationsMatchTheSchema(t *testing.T) {
	t.Parallel()
	// These mirror the CHECK constraints in migration 00001. If they drift, a
	// valid value is rejected with a 400 or an invalid one reaches the database
	// and returns a 500.
	for _, want := range []string{"finished_good", "raw_material", "packaging"} {
		if !contains(ItemTypes, want) {
			t.Errorf("item type %q is missing", want)
		}
	}
	for _, want := range []string{"draft", "active", "archived"} {
		if !contains(Statuses, want) {
			t.Errorf("status %q is missing", want)
		}
	}
	if strings.Join(Locales, ",") != "ru,tg,en" {
		t.Errorf("locales = %v; the schema's CHECK is (ru, tg, en) and `tj` is not one (C2)", Locales)
	}
}
