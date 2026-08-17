package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/http/common"
)

// Shared request parsing.
//
// Every one of these returns a *common.Error naming the offending field, because
// docs/03-API-CONTRACT.md §4 requires validation failures to say which field was
// wrong — "Некорректный запрос" tells the operator nothing they can act on.

func parseUUIDField(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, common.Validation(common.FieldError{
			Field: field, Code: "invalid", Message: "Некорректный идентификатор",
		})
	}
	return id, nil
}

func parseNullUUIDField(raw *string, field string) (uuid.NullUUID, error) {
	if raw == nil || *raw == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := parseUUIDField(*raw, field)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}

// parseDecimalField parses a quantity or an amount.
//
// The wire format is a string precisely so this can be exact
// (docs/03-API-CONTRACT.md §6). Accepting a JSON number here would reintroduce
// the float that the string format exists to avoid.
func parseDecimalField(raw, field string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Decimal{}, common.Validation(common.FieldError{
			Field: field, Code: "invalid", Message: "Укажите число",
		})
	}
	return d, nil
}

func parseNullDecimalField(raw *string, field string) (decimal.Decimal, error) {
	if raw == nil || *raw == "" {
		return decimal.Zero, nil
	}
	return parseDecimalField(*raw, field)
}

// idFromPath reads the {id} URL parameter.
func idFromPath(r *http.Request) (uuid.UUID, error) {
	return parseUUIDField(chi.URLParam(r, "id"), "id")
}

// optionalQuery returns a pointer for a present-and-non-empty query parameter and
// nil otherwise, which is what the domain filters expect: absent means "do not
// filter", not "match the empty string".
func optionalQuery(r *http.Request, key string) *string {
	if v := r.URL.Query().Get(key); v != "" {
		return &v
	}
	return nil
}
