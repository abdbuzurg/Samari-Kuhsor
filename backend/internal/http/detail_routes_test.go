package http_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// R02 — the five detail endpoints that had no route.
//
// Every list view in the CRM linked to a detail page that did not exist. Five of
// the ten had no GET-by-id endpoint behind them either, even though the SQL had
// been written months earlier: GetEmployee, GetAsset, GetDocument, GetInquiry and
// GetSupplier were all sitting in queries/ unreferenced.
//
// The permission cases (403 without the grant, 401 anonymous, admitted with it)
// come from everyGuardedRoute in routes_test.go, which these routes are now
// registered in. What that table cannot check is that the handler returns the
// record it was asked for, so that is what this file does: create through the
// API, read it back by id, and confirm an unknown id is 404 rather than 500 or
// an empty 200.
func TestDetailEndpointsReturnTheRecordAndNotFoundForAnUnknownID(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "detail@samari-kuhsor.tj",
		"hr:manage", "equipment:manage", "documents:manage",
		"procurement:manage", "inquiries:manage")
	token := loginAs(t, handler, "detail@samari-kuhsor.tj")

	cases := []struct {
		name       string
		collection string
		create     string
	}{
		{"employee", "/api/v1/employees", `{"full_name":"А. Раҳимов","version":0}`},
		{"asset", "/api/v1/assets", `{"asset_no":"EQ-047","name":"Линия розлива","version":0}`},
		{"document", "/api/v1/documents", `{"doc_no":"D-1","title":"ISO 22000","version":0}`},
		{"supplier", "/api/v1/suppliers", `{"name":"Памир Агро"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := do(t, handler, http.MethodPost, tc.collection, token, bodyOf(tc.create))
			if res.Code != http.StatusCreated && res.Code != http.StatusOK {
				t.Fatalf("create → %d %s", res.Code, truncate(res.Body.String()))
			}
			id := idFromEnvelope(t, res.Body.Bytes())

			got := do(t, handler, http.MethodGet, tc.collection+"/"+id, token, nil)
			if got.Code != http.StatusOK {
				t.Fatalf("GET by id → %d %s", got.Code, truncate(got.Body.String()))
			}
			if back := idFromEnvelope(t, got.Body.Bytes()); back != id {
				t.Errorf("returned id = %s, want %s", back, id)
			}

			missing := do(t, handler, http.MethodGet, tc.collection+"/"+zeroUUID, token, nil)
			if missing.Code != http.StatusNotFound {
				t.Errorf("unknown id → %d, want 404", missing.Code)
			}
		})
	}
}

// The enquiry route is separate because an enquiry is the one record no
// authenticated user creates: it arrives through the public submission endpoint
// with no session at all. Reading it back is what makes the ToR's
// complaint → traceability workflow openable from Обращения.
func TestInquiryDetailIsReadableAfterAPublicSubmission(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "sales@samari-kuhsor.tj", "inquiries:manage")
	token := loginAs(t, handler, "sales@samari-kuhsor.tj")

	submitted := do(t, handler, http.MethodPost, "/api/v1/public/inquiries", "",
		bodyOf(`{"type":"wholesale","name":"ООО Ориён","contact":"+992 900 000 000"}`))
	if submitted.Code != http.StatusCreated && submitted.Code != http.StatusOK {
		t.Fatalf("submit → %d %s", submitted.Code, truncate(submitted.Body.String()))
	}
	// The public response carries the reference number and NOT the internal id:
	// a visitor has no account and no business holding a primary key. So the
	// enquiry is found the way a sales user finds it — through the register.
	listed := do(t, handler, http.MethodGet, "/api/v1/inquiries", token, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list → %d %s", listed.Code, truncate(listed.Body.String()))
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Data) == 0 {
		t.Fatalf("submitted enquiry did not reach the register: %s", truncate(listed.Body.String()))
	}
	id := page.Data[0].ID

	got := do(t, handler, http.MethodGet, "/api/v1/inquiries/"+id, token, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("GET by id → %d %s", got.Code, truncate(got.Body.String()))
	}

	var env struct {
		Data struct {
			ID          string `json:"id"`
			ReferenceNo string `json:"reference_no"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &env); err != nil {
		t.Fatalf("body was not the data envelope: %s", truncate(got.Body.String()))
	}
	if env.Data.ID != id {
		t.Errorf("returned id = %s, want %s", env.Data.ID, id)
	}
	// The reference number is what the visitor holds; a detail view that does not
	// carry it cannot answer the phone call it exists to answer.
	if env.Data.ReferenceNo == "" {
		t.Error("reference_no is empty on the detail payload")
	}

	missing := do(t, handler, http.MethodGet, "/api/v1/inquiries/"+zeroUUID, token, nil)
	if missing.Code != http.StatusNotFound {
		t.Errorf("unknown id → %d, want 404", missing.Code)
	}
}

func idFromEnvelope(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("body was not the data envelope: %s", truncate(string(raw)))
	}
	if env.Data.ID == "" {
		t.Fatalf("no id in envelope: %s", truncate(string(raw)))
	}
	return env.Data.ID
}

// The envelope guard's blind spot, closed.
//
// TestEverySuccessfulResponseIsASingleDataEnvelope returns early on any non-200,
// and every /{id} case in everyGuardedRoute uses zeroUUID — so it has never
// checked a single detail response. R02 shipped five double-wrapped handlers
// past it, and R18 then found a SIXTH that had been live since T34:
// handleGetShipment.
//
// This creates a real record for each detail route and asserts the shape of what
// comes back, which is the only way this class of defect is visible.
func TestEveryDetailRouteReturnsASingleDataEnvelope(t *testing.T) {
	t.Parallel()
	handler, pool := newServer(t)

	seedUser(t, pool, "shapes-detail@samari-kuhsor.tj",
		"items:manage", "crm:manage", "hr:manage", "equipment:manage",
		"documents:manage", "procurement:manage", "logistics:manage", "inquiries:manage")
	token := loginAs(t, handler, "shapes-detail@samari-kuhsor.tj")

	// Each entry creates a record and then reads it back by id.
	cases := []struct {
		name       string
		collection string
		create     string
	}{
		{"employee", "/api/v1/employees", `{"full_name":"Тест","version":0}`},
		{"asset", "/api/v1/assets", `{"asset_no":"EQ-9","name":"Тест","version":0}`},
		{"document", "/api/v1/documents", `{"doc_no":"D-9","title":"Тест","version":0}`},
		{"supplier", "/api/v1/suppliers", `{"name":"Тест"}`},
		{"customer", "/api/v1/customers", `{"name":"Тест"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := do(t, handler, http.MethodPost, tc.collection, token, bodyOf(tc.create))
			if created.Code != http.StatusCreated && created.Code != http.StatusOK {
				t.Fatalf("create → %d %s", created.Code, truncate(created.Body.String()))
			}
			id := idFromEnvelope(t, created.Body.Bytes())

			got := do(t, handler, http.MethodGet, tc.collection+"/"+id, token, nil)
			assertSingleEnvelope(t, got.Body.Bytes())
		})
	}

	// Shipments are separate: the detail payload has no top-level id to create
	// against generically, and this is the route the defect actually lived in.
	trip := do(t, handler, http.MethodPost, "/api/v1/shipments", token,
		bodyOf(`{"trip_no":"TR-9","route_from":"Хорог","route_to":"Душанбе"}`))
	if trip.Code != http.StatusCreated && trip.Code != http.StatusOK {
		t.Fatalf("create trip → %d %s", trip.Code, truncate(trip.Body.String()))
	}
	tripID := idFromEnvelope(t, trip.Body.Bytes())
	assertSingleEnvelope(t, do(t, handler, http.MethodGet, "/api/v1/shipments/"+tripID, token, nil).Body.Bytes())
}

func assertSingleEnvelope(t *testing.T, raw []byte) {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("not a JSON object: %s", truncate(string(raw)))
	}
	data, ok := env["data"]
	if !ok {
		t.Fatalf("no `data` key: %s", truncate(string(raw)))
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(data, &nested); err == nil {
		if _, doubled := nested["data"]; doubled && len(nested) == 1 {
			t.Errorf("double-wrapped: data.data is the only key. common.JSON "+
				"already builds the envelope — pass the payload directly. Body: %s",
				truncate(string(raw)))
		}
	}
}
