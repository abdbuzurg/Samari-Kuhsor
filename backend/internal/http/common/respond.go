package common

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// The response envelope (docs/03-API-CONTRACT.md §4). Every successful response is
// {"data": …, "meta": …}; every error at every status is {"error": {…}}.

type envelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Error *Error `json:"error"`
}

// PageMeta is the collection metadata (docs/03-API-CONTRACT.md:98).
type PageMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// NewPageMeta computes the metadata for a page of results.
func NewPageMeta(p Params, total int64) PageMeta {
	// An empty collection is one empty page, not zero pages.
	pages := max(1, int((total+int64(p.PerPage)-1)/int64(p.PerPage)))
	return PageMeta{Page: p.Page, PerPage: p.PerPage, Total: total, TotalPages: pages}
}

// JSON writes a successful response.
func JSON(w http.ResponseWriter, status int, data any) {
	write(w, status, envelope{Data: data})
}

// List writes a collection response with pagination metadata.
//
// data must be a non-nil slice: docs/03-API-CONTRACT.md shows collections as JSON
// arrays, and a nil slice marshals to `null`, which breaks every client that maps
// over it. Callers get an empty array instead.
func List(w http.ResponseWriter, data any, meta PageMeta) {
	write(w, http.StatusOK, envelope{Data: data, Meta: meta})
}

// Created writes a 201 with the new resource.
func Created(w http.ResponseWriter, data any) {
	write(w, http.StatusCreated, envelope{Data: data})
}

// NoContent writes a 204 for a successful tombstone.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Fail writes an error response, deriving the status from the code.
//
// The cause, if any, is logged server-side and never sent to the client.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := AsError(err)

	if apiErr.cause != nil || apiErr.Code == CodeInternal {
		slog.ErrorContext(r.Context(), "request failed",
			"code", apiErr.Code,
			"method", r.Method,
			"path", r.URL.Path,
			"error", apiErr.Error(),
		)
	}

	// Defence in depth: an internal error's message is replaced with the generic
	// Russian text even if a caller built it with a leaky one.
	if apiErr.Code == CodeInternal {
		apiErr = &Error{Code: CodeInternal, Message: defaultMessages[CodeInternal]}
	}

	write(w, apiErr.Code.Status(), errorEnvelope{Error: apiErr})
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Nothing this API returns should ever be cached by an intermediary: it is all
	// per-user, permission-filtered data.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // Cyrillic must not be escaped into \u sequences
	if err := enc.Encode(body); err != nil {
		// The status is already written, so the response is beyond repair; log it.
		slog.Error("encoding response failed", "error", err)
	}
}
