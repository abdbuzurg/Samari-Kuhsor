package common

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// Wire types (docs/03-API-CONTRACT.md §6).
//
// The rule that matters most: money and quantities are JSON STRINGS, never
// numbers. "Never a JSON number — floats corrupt currency" (:147). A JSON number
// becomes a float64 in every JavaScript client on the planet, and 0.1 + 0.2 does
// not equal 0.3 in a factory's stock ledger any more than it does anywhere else.

// Money is numeric(14,2) on the wire: "18.50".
type Money struct{ decimal.Decimal }

func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.StringFixed(2))
}

func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		// Accept a number on input so a client that sends 18.5 is not silently
		// rejected — but parse it through decimal, never through float64.
		return m.Decimal.UnmarshalJSON(b)
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}
	m.Decimal = d
	return nil
}

// NewMoney wraps a decimal for output.
func NewMoney(d decimal.Decimal) Money { return Money{d} }

// Quantity is numeric(14,3) on the wire: "8640.000".
type Quantity struct{ decimal.Decimal }

func (q Quantity) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.StringFixed(3))
}

func (q *Quantity) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return q.Decimal.UnmarshalJSON(b)
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}
	q.Decimal = d
	return nil
}

func NewQuantity(d decimal.Decimal) Quantity { return Quantity{d} }

// NullMoney and NullQuantity serialise as null when absent.
func NullMoney(d decimal.NullDecimal) *Money {
	if !d.Valid {
		return nil
	}
	m := NewMoney(d.Decimal)
	return &m
}

func NullQuantity(d decimal.NullDecimal) *Quantity {
	if !d.Valid {
		return nil
	}
	q := NewQuantity(d.Decimal)
	return &q
}

// Timestamp renders RFC 3339 in UTC: "2026-09-09T06:30:00Z" (:145). Formatting for
// Dushanbe time (UTC+5) is the frontend's job — the API has exactly one timezone.
func Timestamp(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

// NullTimestamp renders null rather than an empty string when absent.
func NullTimestamp(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

// Date renders a date without time: "2026-09-09" (:148).
func Date(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.UTC().Format("2006-01-02")
	return &s
}

// ---------------------------------------------------------------------------
// Status payloads — docs/03-API-CONTRACT.md §8
// ---------------------------------------------------------------------------

// Level is the semantic severity of a status. The BACKEND decides this, never a
// React component (docs/03-API-CONTRACT.md:177). Green means healthy, never merely
// branded (CLAUDE.md §5) — so the colour axis is data, resolved here.
type Level string

const (
	LevelOK      Level = "ok"      // #1f7a3d — В норме, Готово, Выпущено, Активен
	LevelWarn    Level = "warn"    // #b8791a — Истекает, Контроль QC, На согласовании
	LevelDanger  Level = "danger"  // #c0341c — Карантин, Низкий остаток, Отклонено
	LevelInfo    Level = "info"    // grey    — Новый, В работе, В пути
	LevelNeutral Level = "neutral" // Черновик, Архив, Проиграно
)

// Status is the payload shape the prototype's coloured tags are driven by.
//
// Per docs/07-IMPLEMENTATION-PLAN.md C3, the frontend renders the label from its
// i18n dictionary keyed on Key — the CRM ships in three languages and a Russian
// label baked into the payload would show Russian tags in a Tajik interface.
// Label is kept as the Russian fallback for a missing dictionary key.
type Status struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Level Level  `json:"level"`
}

// NewStatus builds a status payload.
func NewStatus(key, russianLabel string, level Level) Status {
	return Status{Key: key, Label: russianLabel, Level: level}
}
