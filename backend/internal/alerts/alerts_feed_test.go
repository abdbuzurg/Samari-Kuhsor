package alerts_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
	"github.com/qoim/samari/backend/internal/testsupport"
)

// Уведомления. docs/05-MODULES.md §17 lists ten triggers; I15 splits them into
// three persisted events and seven derived conditions. These tests hold that
// split in place — in particular that nothing here writes a row for a condition,
// because a stored condition needs retraction logic and retraction logic is where
// stale alarms come from.

type feedFixture struct {
	pool *pgxpool.Pool
	svc  *alerts.Service
	user uuid.UUID
}

func newFeed(t *testing.T) feedFixture {
	t.Helper()
	pool := testsupport.NewDB(t)
	f := feedFixture{pool: pool, svc: alerts.NewService(pool)}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('qa@samari-kuhsor.tj','Контролёр','x') RETURNING id`).Scan(&f.user); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f feedFixture) emit(t *testing.T, kind alerts.Kind, resource string, level common.Level, title string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	target := uuid.New()
	if err := alerts.Emit(ctx, tx, uuid.NullUUID{}, kind, resource, target, level, title, ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return target
}

// docs/05-MODULES.md:294 — "users see only notifications for resources they can
// read". Otherwise a warehouse operator learns from the bell that a batch was
// rejected, which is precisely the module they were denied.
func TestFeedIsFilteredByReadableResources(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()

	f.emit(t, alerts.KindBatchRejected, rbac.Quality, common.LevelDanger, "B-1")
	f.emit(t, alerts.KindInquiryReceived, rbac.Inquiries, common.LevelInfo, "CF-1")

	qaOnly := rbac.NewSet([]string{"quality:read"})
	got, err := f.svc.Feed(ctx, f.user, qaOnly, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d notifications for a quality-only viewer, want 1", len(got))
	}
	if got[0].Resource != rbac.Quality {
		t.Errorf("leaked a %s notification", got[0].Resource)
	}

	both := rbac.NewSet([]string{"quality:read", "inquiries:read"})
	all, err := f.svc.Feed(ctx, f.user, both, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("%d notifications for a viewer of both, want 2", len(all))
	}

	// A viewer with no read permission at all sees nothing, and the query is not
	// even issued — `resource = ANY('{}')` would be a correct but wasteful trip.
	none, err := f.svc.Feed(ctx, f.user, rbac.Set{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("%d notifications for a viewer with no permissions", len(none))
	}
}

// The badge counts what the bell would show. If they disagree the user clicks a
// bell showing 3 and finds one item.
func TestUnreadCountMatchesTheFeedAndClearsOnRead(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	perms := rbac.NewSet([]string{"quality:read", "inquiries:read"})

	f.emit(t, alerts.KindBatchQuarantined, rbac.Quality, common.LevelWarn, "B-1")
	f.emit(t, alerts.KindInquiryReceived, rbac.Inquiries, common.LevelInfo, "CF-1")

	n, err := f.svc.Unread(ctx, f.user, perms)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("unread = %d, want 2", n)
	}

	if err := f.svc.MarkAllRead(ctx, f.user, perms); err != nil {
		t.Fatal(err)
	}
	if n, err = f.svc.Unread(ctx, f.user, perms); err != nil || n != 0 {
		t.Errorf("unread after marking read = %d (err %v), want 0", n, err)
	}

	// Read state is per user, so one manager clearing the bell does not clear it
	// for the whole factory.
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO users (email, full_name, password_hash)
		VALUES ('other@samari-kuhsor.tj','Другой','x') RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if n, err = f.svc.Unread(ctx, other, perms); err != nil || n != 2 {
		t.Errorf("another user's unread = %d (err %v), want 2", n, err)
	}

	// And the feed reports read state rather than dropping the row — the bell
	// keeps its history.
	feed, err := f.svc.Feed(ctx, f.user, perms, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 2 {
		t.Fatalf("feed dropped read notifications: %d rows", len(feed))
	}
	for _, row := range feed {
		if !row.IsRead {
			t.Errorf("%s still reads as unread", row.Title)
		}
	}
}

// MarkAllRead must not acknowledge what the user was never shown, or a permission
// grant later would surface notifications already marked read.
func TestMarkAllReadIsScopedToWhatTheViewerCanSee(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()

	f.emit(t, alerts.KindBatchRejected, rbac.Quality, common.LevelDanger, "B-1")
	f.emit(t, alerts.KindInquiryReceived, rbac.Inquiries, common.LevelInfo, "CF-1")

	if err := f.svc.MarkAllRead(ctx, f.user, rbac.NewSet([]string{"quality:read"})); err != nil {
		t.Fatal(err)
	}
	n, err := f.svc.Unread(ctx, f.user, rbac.NewSet([]string{"quality:read", "inquiries:read"}))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("unread = %d, want 1 — the inquiries notification was acknowledged unseen", n)
	}
}

// The point of I15: a standing condition is a live count, so fixing the data
// clears the alert with no retraction step.
func TestDerivedConditionsAppearAndSelfHeal(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	perms := rbac.NewSet([]string{"inventory:read"})

	var item, loc uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom, min_qty)
		VALUES ('SUG-25','raw_material','kg', 100) RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO locations (code, name, zone) VALUES ('WH-1','Склад сырья','raw')
		RETURNING id`).Scan(&loc); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO stock_movements (item_id, location_id, qty_delta, reason)
		VALUES ($1,$2,40,'goods_receipt')`, item, loc); err != nil {
		t.Fatal(err)
	}

	below := func() int {
		t.Helper()
		got, err := f.svc.Counts(ctx, perms)
		if err != nil {
			t.Fatal(err)
		}
		return got[rbac.Inventory]
	}
	if below() != 1 {
		t.Fatalf("40 kg against a 100 kg minimum did not raise the low-stock condition")
	}

	// A goods receipt takes it above the minimum. Nothing retracts the alert —
	// there is nothing to retract.
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO stock_movements (item_id, location_id, qty_delta, reason)
		VALUES ($1,$2,80,'goods_receipt')`, item, loc); err != nil {
		t.Fatal(err)
	}
	if below() != 0 {
		t.Errorf("the low-stock condition survived the receipt that fixed it")
	}
}

// Not a row anywhere. If a future change starts persisting conditions, this fails.
func TestStandingConditionsAreNeverPersisted(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()

	for _, kind := range alerts.Kinds() {
		if alerts.IsPersisted(kind) {
			continue
		}
		var n int
		if err := f.pool.QueryRow(ctx,
			`SELECT count(*) FROM notifications WHERE kind = $1`, string(kind)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s is a standing condition but has %d persisted rows", kind, n)
		}
	}
}

// The two ends of the notification contract: the domain that causes the event
// writes it, in the same transaction, and a rollback takes it with it.
func TestQuarantineAndRejectionNotifyFromTheQualityDomain(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	qs := quality.NewService(f.pool)

	var item, batch uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id, status) VALUES ('B-2617',$1,'in_production')
		RETURNING id`, item).Scan(&batch); err != nil {
		t.Fatal(err)
	}

	if _, err := qs.Transition(ctx, f.user, quality.TransitionInput{
		BatchID: batch, To: quality.StatusQuarantine,
	}); err != nil {
		t.Fatal(err)
	}
	feed, err := f.svc.Feed(ctx, f.user, rbac.NewSet([]string{"quality:read"}), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 1 || feed[0].Kind != string(alerts.KindBatchQuarantined) {
		t.Fatalf("quarantine produced %d notifications: %+v", len(feed), feed)
	}
	if feed[0].Level != string(common.LevelWarn) {
		t.Errorf("quarantine level = %s, want warn", feed[0].Level)
	}

	reason := "Отзыв по жалобе CP-000001"
	if _, err := qs.Transition(ctx, f.user, quality.TransitionInput{
		BatchID: batch, To: quality.StatusRejected, HasApprove: true, Reason: &reason,
	}); err != nil {
		t.Fatal(err)
	}
	feed, err = f.svc.Feed(ctx, f.user, rbac.NewSet([]string{"quality:read"}), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 2 {
		t.Fatalf("rejection did not notify: %d rows", len(feed))
	}
	if feed[0].Level != string(common.LevelDanger) {
		t.Errorf("rejection level = %s, want danger", feed[0].Level)
	}
	if feed[0].Body == nil || *feed[0].Body != reason {
		t.Errorf("the rejection reason did not reach the notification: %v", feed[0].Body)
	}
}

// A release is good news the approver already has. Notifying on it would train the
// factory to dismiss the bell unread, which costs them the two that matter.
func TestReleaseDoesNotNotify(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	qs := quality.NewService(f.pool)

	var item, batch uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id, status) VALUES ('B-2618',$1,'quarantine')
		RETURNING id`, item).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Transition(ctx, f.user, quality.TransitionInput{
		BatchID: batch, To: quality.StatusReleased, HasApprove: true,
	}); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a release wrote %d notifications", n)
	}
}

// A public submission notifies sales, and it carries no actor: the submitter is a
// member of the public, not a user of this system.
func TestPublicSubmissionNotifiesWithNoActor(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	svc := inquiries.NewService(f.pool, inquiries.DefaultRateLimit())

	inq, err := svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeContact, Name: "Гость", Contact: "+992 000 00 00",
	})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := f.svc.Feed(ctx, f.user, rbac.NewSet([]string{"inquiries:read"}), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 1 {
		t.Fatalf("%d notifications for one submission", len(feed))
	}
	// The title is the reference number, not a rendered Russian sentence — the
	// label for the type is the frontend's job in the reader's locale (C3).
	if feed[0].Title != inq.ReferenceNo {
		t.Errorf("title = %q, want the reference number %q", feed[0].Title, inq.ReferenceNo)
	}
	if feed[0].CreatedBy.Valid {
		t.Errorf("a public submission was attributed to user %s", feed[0].CreatedBy.UUID)
	}

	// A complaint is danger — it is the entry point to the traceability workflow.
	var item, batch uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id) VALUES ('B-2617',$1) RETURNING id`,
		item).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, inquiries.SubmitInput{
		Type: inquiries.TypeComplaint, Name: "Покупатель", Contact: "x",
		BatchID: uuid.NullUUID{UUID: batch, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	feed, err = f.svc.Feed(ctx, f.user, rbac.NewSet([]string{"inquiries:read"}), 50)
	if err != nil {
		t.Fatal(err)
	}
	if feed[0].Level != string(common.LevelDanger) {
		t.Errorf("a complaint notified at %s, want danger", feed[0].Level)
	}
}

// A failed transition must leave no notification. The whole reason Emit takes the
// caller's transaction rather than the pool.
func TestNotificationRollsBackWithItsTransaction(t *testing.T) {
	t.Parallel()
	f := newFeed(t)
	ctx := context.Background()
	qs := quality.NewService(f.pool)

	var item, batch uuid.UUID
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO items (sku, item_type, base_uom) VALUES ('APJ-1000','finished_good','bottle')
		RETURNING id`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO batches (batch_no, item_id, status) VALUES ('B-2619',$1,'quarantine')
		RETURNING id`, item).Scan(&batch); err != nil {
		t.Fatal(err)
	}

	// Rejection from quarantine needs approve. Without it the transition fails
	// after the batch row would otherwise have moved.
	if _, err := qs.Transition(ctx, f.user, quality.TransitionInput{
		BatchID: batch, To: quality.StatusRejected,
	}); err == nil {
		t.Fatal("a rejection without approve succeeded")
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a refused transition left %d notifications behind", n)
	}
}
