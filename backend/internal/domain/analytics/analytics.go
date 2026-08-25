// Package analytics implements the website's first-party statistics
// (docs/01-DECISIONS.md D12).
//
// It replaces Matomo. Three things make it different from a general-purpose
// tracker, and all three are deliberate:
//
//   - Identity is a SESSION, not a visitor. A random id held in sessionStorage
//     and gone when the tab closes. Products are ranked by distinct sessions, so
//     ten views in one sitting count once and one enthusiastic distributor does
//     not outrank thirty real people.
//   - Targets are validated against the real catalogue. A product_view naming a
//     SKU that does not exist is dropped. That is what stops the owner's ranking
//     being forged from an unauthenticated endpoint.
//   - Raw events live 90 days and are then HARD DELETED, with the counts
//     preserved in a rollup that carries no session id. The two departures from
//     CLAUDE.md §4 this requires are set out in D12 and in migration 00007.
//
// Ingestion writes no audit_log row — one per click would bury the trail that
// proves who released a batch. Maintenance writes one per run, so the deletion
// is provable and the clicks are not.
package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/audit"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

const Resource = rbac.Analytics

// Event kinds.
const (
	KindPageView    = "page_view"
	KindProductView = "product_view"
	KindLinkClick   = "link_click"
)

// product_view sources.
const (
	SourceProductPage = "product_page"
	SourceBeltModal   = "belt_modal"
)

// link_click categories. Everything is captured; the dashboard shows only
// CategoryCTA and CategoryOutbound.
const (
	CategoryCTA      = "cta"
	CategoryProduct  = "product"
	CategoryNav      = "nav"
	CategoryFooter   = "footer"
	CategoryOutbound = "outbound"
)

var (
	kinds      = []string{KindPageView, KindProductView, KindLinkClick}
	sources    = []string{SourceProductPage, SourceBeltModal}
	categories = []string{CategoryCTA, CategoryProduct, CategoryNav, CategoryFooter, CategoryOutbound}
	locales    = []string{"ru", "tg", "en"}
)

// Retention is the raw window. Past this an event is deleted outright; its
// counts survive in analytics_daily, which carries no session id.
const Retention = 90 * 24 * time.Hour

// Config bounds ingestion.
type Config struct {
	// IPSalt keeps the hash from being a rainbow-table lookup of the address
	// space. Rotating it resets rate-limit windows, which is harmless.
	IPSalt string
	// MaxPerWindow is generous for a person and useless for skewing a ranking.
	MaxPerWindow int
	Window       time.Duration
	// MaxBatch caps one request. The client flushes at 25.
	MaxBatch int
}

func DefaultConfig() Config {
	return Config{MaxPerWindow: 300, Window: time.Hour, MaxBatch: 50}
}

type Service struct {
	pool *pgxpool.Pool
	cfg  Config
	// now is injectable so retention can be tested against a clock rather than
	// by waiting ninety days.
	now func() time.Time
}

func NewService(pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{pool: pool, cfg: cfg, now: time.Now}
}

// WithClock returns a copy that reads time from now. Tests only.
func (s *Service) WithClock(now func() time.Time) *Service {
	clone := *s
	clone.now = now
	return &clone
}

// Event is one thing that happened in a browser, as submitted.
type Event struct {
	Kind     string
	Target   string
	Source   string
	Category string
	Locale   string
	// SKU is set for a product_view, and for a link_click attributed to a
	// product — the modal's «Запросить цену» and «Подробнее».
	SKU string
}

// Batch is one flush from one browser.
type Batch struct {
	SessionID string
	IP        string
	Events    []Event
}

// Ingest stores what is valid and silently discards what is not.
//
// It returns a count rather than an error for anything but a rate limit or a
// database fault, because the endpoint answers 204 regardless: validation
// feedback on an anonymous endpoint is a probing oracle, and a beacon must never
// surface an error into the page.
func (s *Service) Ingest(ctx context.Context, in Batch) (accepted int, err error) {
	if strings.TrimSpace(in.SessionID) == "" || len(in.Events) == 0 {
		return 0, nil
	}
	if l := len(in.SessionID); l < 8 || l > 64 {
		return 0, nil
	}
	if len(in.Events) > s.cfg.MaxBatch {
		in.Events = in.Events[:s.cfg.MaxBatch]
	}

	hash := s.hashIP(in.IP)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("analytics: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)

	if s.cfg.MaxPerWindow > 0 {
		recent, err := q.CountAnalyticsEventsSince(ctx, db.CountAnalyticsEventsSinceParams{
			IpHash:   hash,
			Lookback: pgtype.Interval{Microseconds: s.cfg.Window.Microseconds(), Valid: true},
		})
		if err != nil {
			return 0, fmt.Errorf("analytics: rate check: %w", err)
		}
		if int(recent) >= s.cfg.MaxPerWindow {
			return 0, common.New(common.CodeRateLimited)
		}
	}

	// One lookup per distinct SKU rather than per event: a batch is usually the
	// same product several times.
	items := map[string]uuid.UUID{}

	for _, e := range in.Events {
		itemID, ok := s.resolve(ctx, q, e, items)
		if !ok {
			continue
		}
		if err := q.InsertAnalyticsEvent(ctx, db.InsertAnalyticsEventParams{
			SessionID: in.SessionID,
			Kind:      e.Kind,
			Target:    e.Target,
			ItemID:    itemID,
			Source:    nilIfEmpty(e.Source),
			Category:  nilIfEmpty(e.Category),
			Locale:    e.Locale,
			IpHash:    hash,
		}); err != nil {
			return 0, fmt.Errorf("analytics: insert: %w", err)
		}
		accepted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("analytics: commit: %w", err)
	}
	return accepted, nil
}

// resolve validates one event and returns the item it names, if any.
//
// Anything not understood is dropped rather than corrected. The target space is
// five products and a handful of routes; a caller that sends something else is
// either broken or probing, and neither deserves a stored row.
func (s *Service) resolve(
	ctx context.Context, q *db.Queries, e Event, cache map[string]uuid.UUID,
) (uuid.NullUUID, bool) {
	if !contains(kinds, e.Kind) || !contains(locales, e.Locale) {
		return uuid.NullUUID{}, false
	}
	if t := strings.TrimSpace(e.Target); t == "" || len(t) > 512 {
		return uuid.NullUUID{}, false
	}
	if e.Source != "" && !contains(sources, e.Source) {
		return uuid.NullUUID{}, false
	}
	if e.Category != "" && !contains(categories, e.Category) {
		return uuid.NullUUID{}, false
	}

	switch e.Kind {
	case KindPageView:
		// A path, not a URL: no host, no query, no fragment. Anything else is
		// either a mistake or an attempt to smuggle content into `target`.
		if !strings.HasPrefix(e.Target, "/") || strings.ContainsAny(e.Target, "?#") {
			return uuid.NullUUID{}, false
		}
		return uuid.NullUUID{}, true

	case KindProductView:
		// A product_view MUST name a real SKU. This is the anti-forgery
		// guarantee: the ranking cannot be inflated with products that do not
		// exist, and the FK means it cannot be inserted even if it slipped past.
		if e.Source == "" {
			return uuid.NullUUID{}, false
		}
		id, ok := s.lookupSKU(ctx, q, e.Target, cache)
		if !ok {
			return uuid.NullUUID{}, false
		}
		return uuid.NullUUID{UUID: id, Valid: true}, true

	case KindLinkClick:
		if e.Category == "" {
			return uuid.NullUUID{}, false
		}
		// A product-attributed click carries a SKU; an unknown one is dropped
		// rather than stored unattributed, so the conversion figure stays honest.
		if e.SKU != "" {
			id, ok := s.lookupSKU(ctx, q, e.SKU, cache)
			if !ok {
				return uuid.NullUUID{}, false
			}
			return uuid.NullUUID{UUID: id, Valid: true}, true
		}
		return uuid.NullUUID{}, true
	}
	return uuid.NullUUID{}, false
}

func (s *Service) lookupSKU(
	ctx context.Context, q *db.Queries, sku string, cache map[string]uuid.UUID,
) (uuid.UUID, bool) {
	if id, seen := cache[sku]; seen {
		return id, id != uuid.Nil
	}
	id, err := q.ItemIDBySKU(ctx, sku)
	if err != nil {
		// Cache the miss too: a batch full of one forged SKU should cost one
		// query, not twenty-five.
		cache[sku] = uuid.Nil
		return uuid.Nil, false
	}
	cache[sku] = id
	return id, true
}

// hashIP salts and hashes. The raw address is never stored, and never logged.
func (s *Service) hashIP(ip string) string {
	sum := sha256.Sum256([]byte(s.cfg.IPSalt + "|" + ip))
	return hex.EncodeToString(sum[:])
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

var _ = errors.Is
var _ = pgx.ErrNoRows
var _ = audit.Record
