package common

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Collection parameters (docs/03-API-CONTRACT.md §5):
//
//	GET /api/v1/items?page=1&per_page=50&sort=-created_at&q=сок&status=active

const (
	DefaultPage    = 1
	DefaultPerPage = 50
	MaxPerPage     = 200
)

// Params is a parsed, validated collection request.
type Params struct {
	Page    int
	PerPage int
	// SortField is a whitelisted column name; SortDesc is the `-` prefix.
	SortField string
	SortDesc  bool
	// Query is the live search string the prototype's toolbar sends.
	Query string
}

// Offset is the SQL OFFSET for this page.
func (p Params) Offset() int32 { return int32((p.Page - 1) * p.PerPage) }

// Limit is the SQL LIMIT for this page.
func (p Params) Limit() int32 { return int32(p.PerPage) }

// OrderBy renders the ORDER BY clause, always with a tiebreaker on id so that
// every collection response is deterministic (docs/03-API-CONTRACT.md:139).
// Two rows with equal sort keys must not swap places between pages, or the client
// silently sees one twice and misses another.
func (p Params) OrderBy() string {
	dir := "ASC"
	if p.SortDesc {
		dir = "DESC"
	}
	return p.SortField + " " + dir + ", id " + dir
}

// SortSpec declares which fields a resource may be sorted by, and the default.
//
// A whitelist, not an escape function: sort values reach an ORDER BY clause, and
// docs/03-API-CONTRACT.md:138 is explicit that arbitrary SQL fragments are never
// accepted. An unknown field is rejected, never passed through.
type SortSpec struct {
	Allowed []string
	Default string
	// DefaultDesc sorts the default field descending — usually what you want for
	// created_at.
	DefaultDesc bool
}

// ParseParams reads and validates collection parameters from the query string.
func ParseParams(r *http.Request, spec SortSpec) (Params, error) {
	q := r.URL.Query()
	p := Params{Page: DefaultPage, PerPage: DefaultPerPage}

	if raw := q.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Params{}, Validation(FieldError{
				Field: "page", Code: "invalid", Message: "Номер страницы должен быть положительным числом",
			})
		}
		p.Page = n
	}

	if raw := q.Get("per_page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Params{}, Validation(FieldError{
				Field: "per_page", Code: "invalid", Message: "Размер страницы должен быть положительным числом",
			})
		}
		// Clamped rather than rejected: a client asking for too much gets the
		// maximum, which is friendlier than an error and still bounds the query.
		p.PerPage = min(n, MaxPerPage)
	}

	p.SortField, p.SortDesc = spec.Default, spec.DefaultDesc
	if raw := q.Get("sort"); raw != "" {
		field, desc := strings.TrimPrefix(raw, "-"), strings.HasPrefix(raw, "-")
		if !slices.Contains(spec.Allowed, field) {
			return Params{}, Validation(FieldError{
				Field: "sort", Code: "not_sortable",
				Message: "Сортировка по этому полю недоступна",
			})
		}
		p.SortField, p.SortDesc = field, desc
	}

	p.Query = strings.TrimSpace(q.Get("q"))
	return p, nil
}

// Validate checks a SortSpec at startup. A default that is not in the whitelist
// would produce an ORDER BY on an unvetted column name.
func (s SortSpec) Validate() error {
	if s.Default == "" {
		return Newf(CodeInternal, "sort spec has no default field")
	}
	if !slices.Contains(s.Allowed, s.Default) {
		return Newf(CodeInternal, "sort spec default %q is not in its own whitelist", s.Default)
	}
	return nil
}
