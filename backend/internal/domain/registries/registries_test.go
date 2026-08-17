package registries_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/domain/registries"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Персонал, Оборудование и ТО, Документы.
//
// The interesting behaviour in these three is not CRUD: it is what happens to a
// date that runs out, and — for Документы — who is allowed to say a certificate
// is in force.

type fixture struct {
	pool  *pgxpool.Pool
	svc   *registries.Service
	actor uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	f := fixture{pool: pool, svc: registries.NewService(pool)}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('hr@samari-kuhsor.tj','Кадры','x') RETURNING id`).Scan(&f.actor); err != nil {
		t.Fatal(err)
	}
	return f
}

func date(s string) pgtype.Date {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: d, Valid: true}
}

func ptr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// Персонал
// ---------------------------------------------------------------------------

func TestEmployeeValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	cases := map[string]registries.EmployeeInput{
		"no name":        {FullName: "   "},
		"unknown shift":  {FullName: "А. Раҳимов", Shift: ptr("weekend")},
		"unknown status": {FullName: "А. Раҳимов", Status: "vacation"},
		// A contract ending before it began is a typo, and it would sit in the
		// alerts feed as permanently expiring.
		"contract ends before it starts": {
			FullName: "А. Раҳимов", HiredOn: date("2026-09-01"), ContractUntil: date("2026-08-01"),
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.CreateEmployee(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			} else if code := common.AsError(err).Code; code != common.CodeValidationFailed {
				t.Errorf("code = %s, want validation", code)
			}
		})
	}

	// The valid case, so the rejections above are not passing for the wrong reason.
	if _, err := f.svc.CreateEmployee(ctx, f.actor, registries.EmployeeInput{
		FullName: "А. Раҳимов", Shift: ptr("day"),
		HiredOn: date("2026-08-01"), ContractUntil: date("2027-08-01"),
	}); err != nil {
		t.Errorf("a valid employee was refused: %v", err)
	}
}

func TestEmployeeCreateIsAudited(t *testing.T) {
	t.Parallel()
	f := setup(t)

	e, err := f.svc.CreateEmployee(context.Background(), f.actor, registries.EmployeeInput{
		FullName: "С. Назаров", Shift: ptr("night"),
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := testsupport.AssertAudited(t, f.pool, "hr", e.ID, "create")
	if !entry.ActorID.Valid || entry.ActorID.UUID != f.actor {
		t.Errorf("audit actor = %v, want %s", entry.ActorID, f.actor)
	}
}

func TestEmployeeUpdateIsVersionGuarded(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	e, err := f.svc.CreateEmployee(ctx, f.actor, registries.EmployeeInput{FullName: "С. Назаров"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.UpdateEmployee(ctx, f.actor, e.ID, registries.EmployeeInput{
		FullName: "С. Назаров", Status: registries.EmployeeOnLeave, Version: e.Version,
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}

	// The second write carries the version the first one superseded.
	_, err = f.svc.UpdateEmployee(ctx, f.actor, e.ID, registries.EmployeeInput{
		FullName: "С. Назаров", Status: registries.EmployeeSuspended, Version: e.Version,
	})
	if err == nil {
		t.Fatal("a stale version was accepted")
	}
	if code := common.AsError(err).Code; code != common.CodeVersionConflict {
		t.Errorf("code = %s, want version_conflict", code)
	}
}

// The list exists to answer "whose contract is running out", so that ordering is
// part of the contract rather than a default nobody chose.
func TestEmployeesAreOrderedByContractExpiry(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	for _, e := range []struct {
		name  string
		until pgtype.Date
	}{
		{"Третий", date("2027-01-01")},
		{"Первый", date("2026-09-01")},
		{"Без договора", pgtype.Date{}},
		{"Второй", date("2026-11-01")},
	} {
		if _, err := f.svc.CreateEmployee(ctx, f.actor, registries.EmployeeInput{
			FullName: e.name, ContractUntil: e.until,
		}); err != nil {
			t.Fatal(err)
		}
	}

	rows, total, err := f.svc.Employees(ctx, common.Params{Page: 1, PerPage: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	got := []string{rows[0].Employee.FullName, rows[1].Employee.FullName, rows[2].Employee.FullName}
	want := []string{"Первый", "Второй", "Третий"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d = %s, want %s", i, got[i], want[i])
		}
	}
	// An employee with no fixed-term contract sorts last, not first: NULL is "no
	// expiry to worry about", and putting it at the top would bury the ones that do.
	if rows[3].Employee.FullName != "Без договора" {
		t.Errorf("last row = %s, want the employee with no contract end",
			rows[3].Employee.FullName)
	}
}

// ---------------------------------------------------------------------------
// Оборудование и ТО
// ---------------------------------------------------------------------------

func (f fixture) asset(t *testing.T, no, status string) uuid.UUID {
	t.Helper()
	a, err := f.svc.CreateAsset(context.Background(), f.actor, registries.AssetInput{
		AssetNo: no, Name: "Линия розлива", Status: status,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestAssetValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	cases := map[string]registries.AssetInput{
		"no inventory number": {Name: "Линия розлива"},
		"no name":             {AssetNo: "EQ-047"},
		"unknown status":      {AssetNo: "EQ-047", Name: "Линия", Status: "on_fire"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.CreateAsset(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// The failure this prevents: the asset stays amber after it has been serviced,
// and the factory learns to ignore the colour.
func TestRecordingMaintenanceClearsTheDueFlag(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.asset(t, "EQ-047", registries.AssetMaintenanceDue)
	if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
		AssetID: id, EventType: ptr("planned"), NextDueOn: date("2026-11-17"),
	}); err != nil {
		t.Fatal(err)
	}

	rows, _, err := f.svc.Assets(ctx, common.Params{Page: 1, PerPage: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Asset.Status != registries.AssetRunning {
		t.Errorf("status after service = %s, want running", rows[0].Asset.Status)
	}
	// And the next due date is carried onto the list, so the register answers
	// "when next" without opening each asset.
	if !rows[0].NextDueOn.Valid {
		t.Error("the next service date did not reach the list view")
	}
}

// A running asset serviced early stays running — the service record is added but
// nothing about its state changes, because nothing was wrong with it.
func TestServicingARunningAssetDoesNotChangeItsStatus(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.asset(t, "EQ-048", registries.AssetRunning)
	if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
		AssetID: id, EventType: ptr("planned"),
	}); err != nil {
		t.Fatal(err)
	}
	rows, _, err := f.svc.Assets(ctx, common.Params{Page: 1, PerPage: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Asset.Status != registries.AssetRunning {
		t.Errorf("status = %s, want running", rows[0].Asset.Status)
	}
}

// A broken asset is NOT returned to running by a service record. Whether a repair
// worked is a judgement someone makes, not something the act of writing a note
// establishes.
func TestServicingABrokenAssetDoesNotSilentlyDeclareItFixed(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.asset(t, "EQ-049", registries.AssetBroken)
	if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
		AssetID: id, EventType: ptr("repair"),
	}); err != nil {
		t.Fatal(err)
	}
	rows, _, err := f.svc.Assets(ctx, common.Params{Page: 1, PerPage: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Asset.Status != registries.AssetBroken {
		t.Errorf("status = %s — a repair note declared a broken asset fixed",
			rows[0].Asset.Status)
	}
}

func TestMaintenanceOnARetiredOrUnknownAssetIsRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	retired := f.asset(t, "EQ-050", registries.AssetRetired)
	if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
		AssetID: retired,
	}); err == nil {
		t.Error("a retired asset accepted a service record")
	}

	if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
		AssetID: uuid.New(),
	}); err == nil {
		t.Error("an unknown asset accepted a service record")
	}
}

func TestMaintenanceHistoryIsKept(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	id := f.asset(t, "EQ-051", registries.AssetRunning)
	for range 3 {
		if _, err := f.svc.RecordMaintenance(ctx, f.actor, registries.MaintenanceInput{
			AssetID: id, EventType: ptr("planned"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := f.svc.MaintenanceFor(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("%d service records, want 3 — history is append-only", len(events))
	}
}

// ---------------------------------------------------------------------------
// Документы
// ---------------------------------------------------------------------------

func TestDocumentTransitionMatrixIsExhaustive(t *testing.T) {
	t.Parallel()

	all := []string{
		registries.DocDraft, registries.DocApproval, registries.DocActive,
		registries.DocExpiring, registries.DocExpired, registries.DocArchived,
	}
	legal := map[string]bool{}
	for _, tr := range registries.DocTransitions() {
		legal[tr.From+"->"+tr.To] = true
	}

	// Every from×to pair is checked, so a transition added to the matrix without
	// thought fails here rather than shipping.
	for _, from := range all {
		for _, to := range all {
			key := from + "->" + to
			_, ok := registries.LookupDoc(from, to)
			if ok != legal[key] {
				t.Errorf("%s: Lookup says %v, matrix says %v", key, ok, legal[key])
			}
			if from == to && ok {
				t.Errorf("%s is a no-op but is declared legal", key)
			}
		}
	}

	// The specific rules that matter, named rather than left implicit.
	if _, ok := registries.LookupDoc(registries.DocDraft, registries.DocActive); ok {
		t.Error("a draft can become active without passing through approval")
	}
	for _, derived := range []string{registries.DocExpiring, registries.DocExpired} {
		for _, from := range all {
			if _, ok := registries.LookupDoc(from, derived); ok {
				t.Errorf("%s is reachable as a decision — it is a condition of a date "+
					"passing and is derived by the alerts service", derived)
			}
		}
	}
}

func TestActivatingADocumentRequiresApprove(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	doc, err := f.svc.CreateDocument(ctx, f.actor, registries.DocumentInput{
		DocNo: "СЕРТ-001", Title: "Сертификат соответствия", ValidUntil: date("2027-08-17"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Created as a draft, always. There is no way to post one straight to active,
	// because that would be an approval with no approver.
	if doc.Status != registries.DocDraft {
		t.Fatalf("new document status = %s, want draft", doc.Status)
	}

	if _, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocApproval,
	}); err != nil {
		t.Fatalf("sending for approval needs no authority: %v", err)
	}

	// Activation without the grant.
	_, err = f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocActive, HasApprove: false,
	})
	if err == nil {
		t.Fatal("a document was activated without documents:approve")
	}
	if code := common.AsError(err).Code; code != common.CodeForbidden {
		t.Errorf("code = %s, want forbidden", code)
	}

	activated, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocActive, HasApprove: true,
	})
	if err != nil {
		t.Fatalf("activation with approve: %v", err)
	}
	if activated.Status != registries.DocActive {
		t.Errorf("status = %s, want active", activated.Status)
	}

	// Audited as `approve`, not `update`. The verb is the whole reason the audit
	// write lives in the domain rather than in a trigger.
	entry := testsupport.AssertAudited(t, f.pool, "documents", doc.ID, "approve")
	if entry.Action != "approve" {
		t.Errorf("action = %s, want approve", entry.Action)
	}
}

func TestIllegalDocumentTransitionsAreRefused(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	doc, err := f.svc.CreateDocument(ctx, f.actor, registries.DocumentInput{
		DocNo: "СЕРТ-002", Title: "Паспорт качества",
	})
	if err != nil {
		t.Fatal(err)
	}
	// draft → active skips approval entirely.
	if _, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocActive, HasApprove: true,
	}); err == nil {
		t.Error("a draft was activated without passing through approval")
	}
	// draft → expired is not a decision anyone makes.
	if _, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocExpired, HasApprove: true,
	}); err == nil {
		t.Error("a document was declared expired by hand")
	}
	if _, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: uuid.New(), To: registries.DocApproval,
	}); err == nil {
		t.Error("an unknown document was transitioned")
	}
}

// A document can be sent back from approval for correction without any authority:
// refusing to approve something is not itself an act of approval.
func TestApprovalCanBeReturnedToDraftWithoutApprove(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	doc, err := f.svc.CreateDocument(ctx, f.actor, registries.DocumentInput{
		DocNo: "СЕРТ-003", Title: "Протокол испытаний",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocApproval,
	}); err != nil {
		t.Fatal(err)
	}
	back, err := f.svc.TransitionDocument(ctx, f.actor, registries.DocTransitionInput{
		DocID: doc.ID, To: registries.DocDraft,
	})
	if err != nil {
		t.Fatalf("returning for correction: %v", err)
	}
	if back.Status != registries.DocDraft {
		t.Errorf("status = %s, want draft", back.Status)
	}
}

func TestDocAllowedFromMatchesWhatTheDomainEnforces(t *testing.T) {
	t.Parallel()

	// Without approve, an approval-stage document may only go back to draft.
	got := registries.DocAllowedFrom(registries.DocApproval, false)
	if len(got) != 1 || got[0] != registries.DocDraft {
		t.Errorf("without approve: %v, want [draft]", got)
	}
	// With it, activation appears.
	withApprove := registries.DocAllowedFrom(registries.DocApproval, true)
	if len(withApprove) != 2 {
		t.Errorf("with approve: %v, want draft and active", withApprove)
	}

	// Every destination the projection offers must actually be legal, or the UI
	// would render a button the domain then refuses.
	for _, from := range []string{
		registries.DocDraft, registries.DocApproval, registries.DocActive, registries.DocArchived,
	} {
		for _, hasApprove := range []bool{false, true} {
			for _, to := range registries.DocAllowedFrom(from, hasApprove) {
				rule, ok := registries.LookupDoc(from, to)
				if !ok {
					t.Errorf("DocAllowedFrom(%s, %v) offered an illegal move to %s",
						from, hasApprove, to)
				}
				if rule.RequiresApprove && !hasApprove {
					t.Errorf("DocAllowedFrom(%s, false) offered %s, which needs approve",
						from, to)
				}
			}
		}
	}
}

func TestDocumentValidation(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	for name, in := range map[string]registries.DocumentInput{
		"no number": {Title: "Сертификат"},
		"no title":  {DocNo: "СЕРТ-004"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.CreateDocument(ctx, f.actor, in); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// The list exists to answer "what is about to lapse".
func TestDocumentsAreOrderedByExpiry(t *testing.T) {
	t.Parallel()
	f := setup(t)
	ctx := context.Background()

	for i, until := range []string{"2027-01-01", "2026-09-01", "2026-11-01"} {
		if _, err := f.svc.CreateDocument(ctx, f.actor, registries.DocumentInput{
			DocNo:      "СЕРТ-10" + string(rune('0'+i)),
			Title:      "Документ",
			ValidUntil: date(until),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, err := f.svc.Documents(ctx, common.Params{Page: 1, PerPage: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rows[0].ValidUntil.Time.Before(rows[1].ValidUntil.Time) {
		t.Error("documents are not ordered by expiry")
	}
}
