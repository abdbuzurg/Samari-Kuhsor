// Package batches owns batch records and QR issuance.
//
// The batch is the traceability spine (docs/02-SCHEMA.md:196). Its `status` is
// changed ONLY by quality events — never by production or warehouse staff, and
// never from here. This package creates batches and issues their QR codes;
// releasing one out of quarantine belongs to `quality` (T18) and requires
// quality:approve.
package batches

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/qr"
	"github.com/qoim/samari/backend/internal/http/common"
)

// Resource: batches are part of Товары и цены for permission purposes at this
// stage — QR issuance is a catalogue task done before production exists (D11).
// Batch STATUS transitions live under `quality` and are gated separately.
const Resource = "items"

type Service struct {
	pool *pgxpool.Pool
	// publicBaseURL is the site a scanned code resolves to. It is configuration,
	// not a constant: the payload is printed onto wrappers months in advance, so
	// it must be the real production address from the very first export — a code
	// pointing at a staging host would be printed onto thousands of jars.
	publicBaseURL string
}

func NewService(pool *pgxpool.Pool, publicBaseURL string) *Service {
	return &Service{pool: pool, publicBaseURL: publicBaseURL}
}

type CreateInput struct {
	BatchNo    string
	ItemID     uuid.UUID
	ProducedOn pgtype.Date
	ExpiresOn  pgtype.Date
}

// Create records a planned batch.
//
// Note there is no status parameter: a new batch is `in_production` by column
// default and only quality events move it (docs/02-SCHEMA.md:197).
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (db.Batch, error) {
	batchNo := strings.TrimSpace(in.BatchNo)

	var details []common.FieldError
	if batchNo == "" {
		details = append(details, common.FieldError{
			Field: "batch_no", Code: "required", Message: "Укажите номер партии",
		})
	}
	// Mirrors the batches_expiry_after_production CHECK. Validated here so the
	// user gets a message naming the field, rather than a 500 from a constraint
	// violation that reaches them as "внутренняя ошибка сервера".
	if in.ProducedOn.Valid && in.ExpiresOn.Valid && in.ExpiresOn.Time.Before(in.ProducedOn.Time) {
		details = append(details, common.FieldError{
			Field: "expires_on", Code: "invalid",
			Message: "Срок годности раньше даты производства",
		})
	}
	if len(details) > 0 {
		return db.Batch{}, common.Validation(details...)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Batch{}, fmt.Errorf("batches: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	if _, err := q.GetItemByID(ctx, in.ItemID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Batch{}, common.Validation(common.FieldError{
				Field: "item_id", Code: "not_found", Message: "Товар не найден",
			})
		}
		return db.Batch{}, fmt.Errorf("batches: load item: %w", err)
	}

	batch, err := q.CreateBatch(ctx, db.CreateBatchParams{
		BatchNo:    batchNo,
		ItemID:     in.ItemID,
		ProducedOn: in.ProducedOn,
		ExpiresOn:  in.ExpiresOn,
		CreatedBy:  uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		// Belt and braces: the checks above should catch these first, but a
		// constraint violation must never surface as a 500 either way.
		switch {
		case constraintViolated(err, "batches_batch_no_key"):
			return db.Batch{}, common.Validation(common.FieldError{
				Field: "batch_no", Code: "already_exists",
				Message: fmt.Sprintf("Партия %s уже существует", batchNo),
			})
		case constraintViolated(err, "batches_expiry_after_production"):
			return db.Batch{}, common.Validation(common.FieldError{
				Field: "expires_on", Code: "invalid",
				Message: "Срок годности раньше даты производства",
			})
		}
		return db.Batch{}, fmt.Errorf("batches: create: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionCreate,
		Resource:   Resource,
		ResourceID: audit.Target(batch.ID),
		After:      map[string]any{"batch_no": batch.BatchNo, "status": batch.Status},
	}); err != nil {
		return db.Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Batch{}, fmt.Errorf("batches: commit: %w", err)
	}
	return batch, nil
}

// Get loads one batch.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.Batch, error) {
	batch, err := db.New(s.pool).GetBatchByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Batch{}, common.NotFound()
		}
		return db.Batch{}, fmt.Errorf("batches: get: %w", err)
	}
	return batch, nil
}

// constraintViolated reports whether err is a Postgres constraint violation on
// the named constraint. Matching on the constraint name rather than the message
// keeps this working when the server locale changes.
func constraintViolated(err error, name string) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return strings.Contains(pgErr.ConstraintName, name)
	}
	return false
}

// ErrQRAlreadyIssued means a code was already generated for this batch.
var ErrQRAlreadyIssued = errors.New("batches: QR already issued")

// IssueQR generates and stores the batch's QR payload.
//
// Re-issuing is REFUSED, not silently overwritten. Wrappers are ordered against
// the issued code in advance (D11), so a second, different payload would
// invalidate wrappers that may already be printed and on their way — and a
// printed wrapper cannot be corrected. Refusing is recoverable; overwriting is not.
func (s *Service) IssueQR(ctx context.Context, actor uuid.UUID, batchID uuid.UUID) (db.Batch, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Batch{}, fmt.Errorf("batches: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	batch, err := q.GetBatchByID(ctx, batchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Batch{}, common.NotFound()
		}
		return db.Batch{}, fmt.Errorf("batches: load: %w", err)
	}
	if batch.QrPayload != nil {
		return db.Batch{}, common.BusinessRule(
			"QR-код для этой партии уже сгенерирован. Повторная генерация сделала бы недействительными уже заказанные этикетки.")
	}

	payload, err := qr.Payload(s.publicBaseURL, batch.BatchNo)
	if err != nil {
		return db.Batch{}, fmt.Errorf("batches: payload: %w", err)
	}

	issued, err := q.IssueBatchQR(ctx, db.IssueBatchQRParams{ID: batchID, QrPayload: &payload})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Another request issued one between the read and the write.
			return db.Batch{}, common.BusinessRule("QR-код для этой партии уже сгенерирован.")
		}
		return db.Batch{}, fmt.Errorf("batches: issue: %w", err)
	}

	// Audited as `approve`, not `update`: issuing a code commits the company to
	// ordering wrappers against it, and the trail should show who decided that.
	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID:    audit.Actor(actor),
		Action:     audit.ActionApprove,
		Resource:   Resource,
		ResourceID: audit.Target(batchID),
		After:      map[string]any{"qr_payload": payload, "batch_no": batch.BatchNo},
	}); err != nil {
		return db.Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Batch{}, fmt.Errorf("batches: commit: %w", err)
	}
	return issued, nil
}

// ExportQR writes the printer handoff: one SVG per issued batch plus a manifest.
func (s *Service) ExportQR(ctx context.Context, w io.Writer, itemID uuid.NullUUID) (int, error) {
	q := db.New(s.pool)

	rows, err := q.ListBatchesForQRExport(ctx, db.ListBatchesForQRExportParams{ItemID: itemID})
	if err != nil {
		return 0, fmt.Errorf("batches: export query: %w", err)
	}
	if len(rows) == 0 {
		return 0, common.BusinessRule("Нет партий со сгенерированными QR-кодами для экспорта.")
	}

	out := make([]qr.ExportRow, 0, len(rows))
	for _, r := range rows {
		payload := ""
		if r.Batch.QrPayload != nil {
			payload = *r.Batch.QrPayload
		}
		out = append(out, qr.ExportRow{
			BatchNo:  r.Batch.BatchNo,
			ItemSKU:  r.Sku,
			ItemName: r.ItemName,
			Payload:  payload,
		})
	}
	if err := qr.Export(w, out, 8); err != nil {
		return 0, err
	}
	return len(out), nil
}

// SVGFor renders a single batch's code for on-screen display.
func (s *Service) SVGFor(ctx context.Context, batchID uuid.UUID) ([]byte, error) {
	q := db.New(s.pool)
	batch, err := q.GetBatchByID(ctx, batchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, common.NotFound()
		}
		return nil, fmt.Errorf("batches: load: %w", err)
	}
	if batch.QrPayload == nil {
		return nil, common.NotFound()
	}
	return qr.SVG(*batch.QrPayload, 8)
}
