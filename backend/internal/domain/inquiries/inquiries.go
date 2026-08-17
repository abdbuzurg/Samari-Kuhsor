// Package inquiries implements Интеграция с сайтом.
//
// This is the one module written by the PUBLIC website through the same backend
// (docs/02-SCHEMA.md:370), so it is the only place unauthenticated input reaches
// the database. Two consequences shape it:
//
//   - Every submission returns its reference number to the visitor. That is a ToR
//     requirement, and it is also the only receipt the visitor gets.
//   - The endpoint is rate-limited by IP and stores nothing a public reader can
//     retrieve: the public surface may write here and never read.
package inquiries

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Inquiries

// Types and their reference prefixes (docs/05-MODULES.md:160).
const (
	TypeWholesale   = "wholesale"
	TypeContact     = "contact"
	TypeDistributor = "distributor"
	TypeComplaint   = "complaint"
	TypeJob         = "job"
)

// Prefixes maps a type to its reference prefix. These appear on the visitor's
// receipt and in the client's own correspondence, so they are fixed.
var Prefixes = map[string]string{
	TypeWholesale:   "WR-",
	TypeContact:     "CF-",
	TypeDistributor: "DA-",
	TypeComplaint:   "CP-",
	TypeJob:         "JB-",
}

func Types() []string {
	return []string{TypeWholesale, TypeContact, TypeDistributor, TypeComplaint, TypeJob}
}

// Statuses: Новое → Создан лид → Закрыто.
const (
	StatusNew         = "new"
	StatusLeadCreated = "lead_created"
	StatusClosed      = "closed"
)

// RateLimit bounds public submissions per IP. Deliberately generous: a
// distributor filling in three enquiries about three products is normal, and a
// limit that catches them is worse than one that lets a nuisance through.
type RateLimit struct {
	Max      int
	Lookback time.Duration
}

func DefaultRateLimit() RateLimit { return RateLimit{Max: 10, Lookback: time.Hour} }

var SortSpec = common.SortSpec{
	Allowed:     []string{"created_at", "reference_no", "status"},
	Default:     "created_at",
	DefaultDesc: true,
}

type Service struct {
	pool  *pgxpool.Pool
	limit RateLimit
}

func NewService(pool *pgxpool.Pool, limit RateLimit) *Service {
	return &Service{pool: pool, limit: limit}
}

// SubmitInput is a public submission. Note what it does NOT accept: a status, a
// reference number, or an id. All three are the server's to decide.
type SubmitInput struct {
	Type    string
	Name    string
	Company *string
	Contact string
	Message *string
	// BatchID is required for a complaint, so the ToR's
	// complaint → batch traceability → investigation workflow is possible
	// (docs/05-MODULES.md:166).
	BatchID uuid.NullUUID
	IP      *netip.Addr
}

// Submit records a public enquiry and returns it, including the reference number
// the visitor is shown.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (db.Inquiry, error) {
	if err := validate(in); err != nil {
		return db.Inquiry{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Inquiry{}, fmt.Errorf("inquiries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)

	if in.IP != nil && s.limit.Max > 0 {
		recent, err := q.CountInquiriesSince(ctx, db.CountInquiriesSinceParams{
			SourceIp: in.IP,
			Lookback: pgtype.Interval{Microseconds: s.limit.Lookback.Microseconds(), Valid: true},
		})
		if err != nil {
			return db.Inquiry{}, fmt.Errorf("inquiries: rate check: %w", err)
		}
		if int(recent) >= s.limit.Max {
			return db.Inquiry{}, common.New(common.CodeRateLimited)
		}
	}

	// A complaint must name a batch, or the traceability workflow has no entry
	// point and the complaint is just a message.
	if in.Type == TypeComplaint {
		if !in.BatchID.Valid {
			return db.Inquiry{}, common.Validation(common.FieldError{
				Field: "batch_id", Code: "required",
				Message: "Для жалобы укажите партию, указанную на упаковке",
			})
		}
		if _, err := q.GetBatchByID(ctx, in.BatchID.UUID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.Inquiry{}, common.Validation(common.FieldError{
					Field: "batch_id", Code: "not_found", Message: "Партия не найдена",
				})
			}
			return db.Inquiry{}, fmt.Errorf("inquiries: load batch: %w", err)
		}
	}

	// The reference number comes from a per-type sequence (migration 00006).
	//
	// It replaced a retry-on-collision loop over MAX(reference_no)+1. That could
	// not work under concurrency for a reason the retry count could not fix: MAX
	// reads only committed rows, so N concurrent submissions all read the same
	// maximum and then walk the same candidates in lockstep. The collisions are
	// systematic, not random. A sequence never hands the same value to two
	// callers, so there is nothing left to retry.
	//
	// Sequences leave gaps when a transaction rolls back. A reference number is
	// an identifier the visitor quotes back to QOIM, not a count of anything, so
	// a gap costs nothing.
	ref, err := q.NextInquiryReference(ctx, Prefixes[in.Type])
	if err != nil {
		return db.Inquiry{}, fmt.Errorf("inquiries: reference: %w", err)
	}

	inquiry, err := q.CreateInquiry(ctx, db.CreateInquiryParams{
		ReferenceNo: ref, InquiryType: in.Type, Name: strings.TrimSpace(in.Name),
		Company: in.Company, Contact: strings.TrimSpace(in.Contact), Message: in.Message,
		BatchID: in.BatchID, SourceIp: in.IP,
	})
	if err != nil {
		return db.Inquiry{}, fmt.Errorf("inquiries: create: %w", err)
	}

	// Audited with a nil actor: the submitter is a member of the public, not a
	// user of this system. Recording no actor is truthful; inventing one is not.
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: audit.ActionCreate, Resource: Resource,
		ResourceID: audit.Target(inquiry.ID),
		IP:         in.IP,
		After:      map[string]any{"reference_no": inquiry.ReferenceNo, "type": in.Type},
	}); err != nil {
		return db.Inquiry{}, err
	}

	// The third persisted notification (docs/05-MODULES.md §17).
	//
	// The title is the reference number, not a rendered sentence: the enquiry type
	// is already carried by `kind`, and per C3 the label for it is chosen by the
	// frontend in the reader's locale. Baking "Оптовый запрос" into the row would
	// pin every notification to the language of whoever's submission created it.
	//
	// A complaint is
	// danger, not warn: it is the entry point to the ToR's traceability workflow
	// and the only enquiry type that can mean a product problem is in the field.
	level := common.LevelInfo
	if in.Type == TypeComplaint {
		level = common.LevelDanger
	}
	if err := alerts.Emit(ctx, tx, uuid.NullUUID{}, alerts.KindInquiryReceived,
		Resource, inquiry.ID, level,
		inquiry.ReferenceNo, in.Name); err != nil {
		return db.Inquiry{}, fmt.Errorf("inquiries: notify: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Inquiry{}, fmt.Errorf("inquiries: commit: %w", err)
	}
	return inquiry, nil
}

// ConvertToLead creates a lead from an enquiry, carrying the reference across
// (docs/05-MODULES.md:164).
func (s *Service) ConvertToLead(ctx context.Context, actor uuid.UUID, inquiryID uuid.UUID) (db.Lead, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Lead{}, fmt.Errorf("inquiries: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := db.New(tx)
	inq, err := q.GetInquiry(ctx, inquiryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Lead{}, common.NotFound()
		}
		return db.Lead{}, fmt.Errorf("inquiries: load: %w", err)
	}
	if inq.Status != StatusNew {
		// Converting twice would create two leads chasing one enquiry.
		return db.Lead{}, common.BusinessRule(fmt.Sprintf(
			"Обращение %s уже обработано (статус «%s»).", inq.ReferenceNo, inq.Status))
	}

	// The customer is created from what the enquiry gave us. A distributor who
	// already exists gets a duplicate record — deduplication is a judgement the
	// sales team makes, not one to guess at on their behalf.
	name := inq.Name
	if inq.Company != nil && strings.TrimSpace(*inq.Company) != "" {
		name = *inq.Company
	}
	customer, err := q.CreateCustomer(ctx, db.CreateCustomerParams{
		Name: name, Contact: &inq.Contact,
		CreatedBy: uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.Lead{}, fmt.Errorf("inquiries: create customer: %w", err)
	}

	source := inq.ReferenceNo // the reference carried across
	lead, err := q.CreateLead(ctx, db.CreateLeadParams{
		CustomerID: uuid.NullUUID{UUID: customer.ID, Valid: true},
		InquiryID:  uuid.NullUUID{UUID: inquiryID, Valid: true},
		Source:     &source,
		CreatedBy:  uuid.NullUUID{UUID: actor, Valid: true},
	})
	if err != nil {
		return db.Lead{}, fmt.Errorf("inquiries: create lead: %w", err)
	}

	if _, err := q.SetInquiryStatus(ctx, db.SetInquiryStatusParams{
		ID: inquiryID, Status: StatusLeadCreated,
	}); err != nil {
		return db.Lead{}, fmt.Errorf("inquiries: set status: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: audit.Actor(actor), Action: audit.ActionUpdate, Resource: Resource,
		ResourceID: audit.Target(inquiryID),
		Before:     map[string]any{"status": StatusNew},
		After: map[string]any{
			"status": StatusLeadCreated, "lead_id": lead.ID.String(),
			"reference_no": inq.ReferenceNo,
		},
	}); err != nil {
		return db.Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Lead{}, fmt.Errorf("inquiries: commit: %w", err)
	}
	return lead, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (db.Inquiry, error) {
	inq, err := db.New(s.pool).GetInquiry(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Inquiry{}, common.NotFound()
		}
		return db.Inquiry{}, fmt.Errorf("inquiries: get: %w", err)
	}
	return inq, nil
}

type ListFilter struct {
	Status *string
	Type   *string
}

func (s *Service) List(ctx context.Context, p common.Params, f ListFilter) ([]db.ListInquiriesRow, int64, error) {
	q := db.New(s.pool)
	var search *string
	if p.Query != "" {
		search = &p.Query
	}
	rows, err := q.ListInquiries(ctx, db.ListInquiriesParams{
		Status: f.Status, InquiryType: f.Type, Q: search, Limit: p.Limit(), Offset: p.Offset(),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inquiries: list: %w", err)
	}
	total, err := q.CountInquiries(ctx, db.CountInquiriesParams{
		Status: f.Status, InquiryType: f.Type, Q: search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("inquiries: count: %w", err)
	}
	return rows, total, nil
}

func validate(in SubmitInput) error {
	var details []common.FieldError

	if _, ok := Prefixes[in.Type]; !ok {
		details = append(details, common.FieldError{
			Field: "inquiry_type", Code: "invalid", Message: "Неизвестный тип обращения",
		})
	}
	if strings.TrimSpace(in.Name) == "" {
		details = append(details, common.FieldError{
			Field: "name", Code: "required", Message: "Укажите имя",
		})
	}
	if strings.TrimSpace(in.Contact) == "" {
		details = append(details, common.FieldError{
			Field: "contact", Code: "required", Message: "Укажите контакт для связи",
		})
	}
	// Bounded so a public endpoint cannot be used to store arbitrary volumes of
	// text in the database.
	if in.Message != nil && len(*in.Message) > 5000 {
		details = append(details, common.FieldError{
			Field: "message", Code: "too_long", Message: "Сообщение слишком длинное",
		})
	}
	if len(details) > 0 {
		return common.Validation(details...)
	}
	return nil
}
