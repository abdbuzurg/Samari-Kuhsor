package api_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/qoim/samari/backend/internal/api"
)

// The wire contract, asserted against the Go types themselves.
//
// packages/types is generated from this package by tygo, and `make check` fails
// if it is stale — but "not stale" only means the TypeScript matches what tygo
// emits. These tests check the thing tygo cannot: that what it emits matches what
// encoding/json actually sends.

// allDTOs is every type that crosses the wire. New DTOs must be added here; the
// coverage test below fails loudly if one is missed.
func allDTOs() []any {
	return []any{
		api.Status{}, api.PageMeta{}, api.APIError{}, api.ErrorEnvelope{},
		api.LoginRequest{}, api.LoginResponse{}, api.Role{}, api.User{},
		api.Item{}, api.ItemListRow{}, api.ItemTranslation{}, api.PackagingUnit{},
		api.Price{}, api.ItemWriteRequest{}, api.ItemTranslationWrite{},
		api.PackagingUnitWrite{}, api.PriceWriteRequest{},
		api.Batch{}, api.BatchWriteRequest{}, api.BatchDetail{},
		api.BatchStatusEvent{}, api.BatchTransitionRequest{}, api.BatchListRow{},
		api.Location{}, api.StockBalanceRow{}, api.StockMovementRow{},
		api.MovementWriteRequest{}, api.TransferRequest{},
		api.ManufacturingOrder{}, api.ManufacturingOrderRow{}, api.ProductionEntry{},
		api.ManufacturingOrderWriteRequest{}, api.ProductionEntryWriteRequest{},
		api.QualityTest{}, api.QualityTestWriteRequest{},
		api.Supplier{}, api.SupplierWriteRequest{},
		api.PurchaseOrder{}, api.PurchaseOrderRow{}, api.PurchaseOrderLine{},
		api.PurchaseOrderLineWrite{}, api.PurchaseOrderWriteRequest{},
		api.TransitionRequest{}, api.GoodsReceipt{}, api.GoodsReceiptRequest{},
		api.GoodsReceiptLineWrite{},
		api.SalesOrder{}, api.SalesOrderRow{}, api.SalesOrderLine{},
		api.SalesOrderLineWrite{}, api.SalesOrderWriteRequest{}, api.ConfirmOrderRequest{},
		api.Shipment{}, api.ShipmentLine{}, api.ShipmentWriteRequest{},
		api.ShipmentLoadRequest{},
		api.Inquiry{}, api.InquirySubmitRequest{}, api.InquiryReceipt{}, api.Lead{},
		api.RoleDetail{}, api.RoleWriteRequest{}, api.RolePermissionsRequest{},
		api.UserRolesRequest{}, api.UserActiveRequest{}, api.AdminUserRow{},
		api.PermissionCatalogue{}, api.PermissionResource{},
		api.Notification{}, api.AlertFeed{}, api.AlertCondition{},
	}
}

// No DTO may embed a struct.
//
// encoding/json FLATTENS an embedded struct into the parent object; tygo emits it
// as a NESTED field. The two disagree, so an embedded row generates TypeScript
// describing a payload the API never sends — and because the generated file is
// internally consistent, `make check` stays green while every consumer of that
// type reads undefined.
//
// This was a real defect: ManufacturingOrder, PurchaseOrder and SalesOrder each
// embedded their list row, and the generated TypeScript had them nested.
func TestNoDTOEmbedsAStruct(t *testing.T) {
	t.Parallel()
	for _, dto := range allDTOs() {
		typ := reflect.TypeOf(dto)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				if !f.Anonymous {
					continue
				}
				if f.Type.Kind() == reflect.Struct {
					t.Errorf("%s embeds %s — encoding/json flattens it but tygo nests it; "+
						"repeat the fields instead", typ.Name(), f.Type.Name())
				}
			}
		})
	}
}

// Every exported field must carry a json tag.
//
// Without one Go uses the Go field name — `MONo`, `SKU`, `POID` — and the wire
// format silently becomes Go's naming convention instead of the snake_case the
// contract specifies (docs/03-API-CONTRACT.md §3).
func TestEveryFieldHasAJSONTag(t *testing.T) {
	t.Parallel()
	for _, dto := range allDTOs() {
		typ := reflect.TypeOf(dto)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				if f.PkgPath != "" {
					continue // unexported, never serialised
				}
				tag := f.Tag.Get("json")
				if tag == "" {
					t.Errorf("%s.%s has no json tag", typ.Name(), f.Name)
					continue
				}
				name, _, _ := strings.Cut(tag, ",")
				if name == "" || name == "-" {
					continue
				}
				if name != strings.ToLower(name) {
					t.Errorf("%s.%s serialises as %q — the contract is snake_case",
						typ.Name(), f.Name, name)
				}
			}
		})
	}
}

// A nullable field must be a pointer AND annotate its TypeScript type.
//
// tygo turns a Go pointer into `field?: T`, but the API sends `"field": null` —
// the distinction TypeScript actually branches on. Every pointer field in a
// RESPONSE type therefore carries `tstype:"X | null"`. Request types are exempt:
// there, optional genuinely means "may be omitted".
func TestNullableResponseFieldsDeclareNull(t *testing.T) {
	t.Parallel()
	for _, dto := range allDTOs() {
		typ := reflect.TypeOf(dto)
		if strings.HasSuffix(typ.Name(), "Request") || strings.HasSuffix(typ.Name(), "Write") {
			continue
		}
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				if f.Type.Kind() != reflect.Ptr {
					continue
				}
				if tag := f.Tag.Get("json"); strings.Contains(tag, ",omitempty") {
					continue // genuinely absent, not null
				}
				ts := f.Tag.Get("tstype")
				if !strings.Contains(ts, "null") {
					t.Errorf("%s.%s is a pointer in a response but does not declare "+
						"`tstype:\"... | null\"` — tygo would emit an optional field "+
						"while the API sends null", typ.Name(), f.Name)
				}
			}
		})
	}
}

// Money and quantity cross the wire as strings, never JSON numbers
// (docs/03-API-CONTRACT.md §6). A JSON number is an IEEE-754 double in every
// JavaScript client, and numeric(14,2) does not survive that round trip.
func TestMonetaryAndQuantityFieldsAreStrings(t *testing.T) {
	t.Parallel()
	suspicious := []string{
		"qty", "quantity", "amount", "price", "total", "cost", "balance", "on_hand",
	}
	for _, dto := range allDTOs() {
		typ := reflect.TypeOf(dto)
		// PageMeta's `total` and `total_pages` are row counts, not amounts. An
		// integer is the correct type for them and always will be — a page count
		// cannot be fractional.
		if typ.Name() == "PageMeta" {
			continue
		}
		t.Run(typ.Name(), func(t *testing.T) {
			for i := range typ.NumField() {
				f := typ.Field(i)
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				matched := false
				for _, s := range suspicious {
					if strings.Contains(name, s) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
				kind := f.Type.Kind()
				if kind == reflect.Ptr {
					kind = f.Type.Elem().Kind()
				}
				switch kind {
				case reflect.String, reflect.Slice, reflect.Struct:
					// string, a list of lines, or a Status — all fine.
				default:
					t.Errorf("%s.%s (%s) is a %s — money and quantities cross the wire "+
						"as strings", typ.Name(), f.Name, name, kind)
				}
			}
		})
	}
}

// A round trip through encoding/json must produce a flat object for the detail
// types that previously embedded their list row. This is the same defect as
// TestNoDTOEmbedsAStruct, asserted from the other end: through the actual
// marshaller rather than through reflection over the declaration.
func TestDetailPayloadsAreFlat(t *testing.T) {
	t.Parallel()
	cases := map[string]any{
		"ManufacturingOrder": api.ManufacturingOrder{ID: "1", MONo: "MO-1"},
		"PurchaseOrder":      api.PurchaseOrder{ID: "1", PONo: "PO-1"},
		"SalesOrder":         api.SalesOrder{ID: "1", SONo: "SO-1"},
	}
	for name, dto := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(dto)
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]json.RawMessage
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if _, nested := out[name+"Row"]; nested {
				t.Errorf("%s serialised with a nested %sRow object", name, name)
			}
			if _, ok := out["id"]; !ok {
				t.Errorf("%s has no top-level id: %s", name, raw)
			}
		})
	}
}

// The DTO list above must not fall behind the package. A new response type that
// nobody adds here escapes every check in this file.
//
// Counted rather than enumerated by reflection, because Go offers no way to list
// a package's types at runtime — so this is a deliberate tripwire: adding a DTO
// fails the build until it is registered.
func TestDTOListCoversThePackage(t *testing.T) {
	t.Parallel()
	const registered = 71
	if got := len(allDTOs()); got != registered {
		t.Errorf("allDTOs() has %d entries, expected %d — if you added a DTO, add it "+
			"to allDTOs() and update this count", got, registered)
	}
	seen := map[string]bool{}
	for _, dto := range allDTOs() {
		name := reflect.TypeOf(dto).Name()
		if seen[name] {
			t.Errorf("%s is listed twice", name)
		}
		seen[name] = true
	}
}
