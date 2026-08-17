// Package common holds the HTTP conventions every module inherits: the response
// envelope, the error format, collection parameters and the optimistic-concurrency
// guard.
//
// docs/03-API-CONTRACT.md:5 — "Follow these exactly; consistency here is what lets
// twelve modules be built from one template." Get it right once.
package common

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is the stable machine string in every error response. The frontend
// switches on it, never on the message (docs/03-API-CONTRACT.md:116).
type Code string

const (
	CodeValidationFailed Code = "validation_failed"
	CodeUnauthenticated  Code = "unauthenticated"
	CodeForbidden        Code = "forbidden"
	CodeNotFound         Code = "not_found"
	CodeConflict         Code = "conflict"
	CodeVersionConflict  Code = "version_conflict"
	CodeRateLimited      Code = "rate_limited"
	CodeInternal         Code = "internal_error"

	// CodeBusinessRule fills a gap in the contract: docs/03-API-CONTRACT.md:123
	// reserves 422 for "business-rule violation" but the code list at :120 names
	// no code for it. Used for refusals like shipping a batch that is not
	// released — the request is well-formed and permitted, but the domain says no.
	CodeBusinessRule Code = "business_rule"
)

// Status maps a code to its HTTP status (docs/03-API-CONTRACT.md:123).
func (c Code) Status() int {
	switch c {
	case CodeValidationFailed:
		return http.StatusBadRequest
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeVersionConflict:
		return http.StatusConflict
	case CodeBusinessRule:
		return http.StatusUnprocessableEntity
	case CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// FieldError is one entry in an error's details array.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error is an API error. It implements error so domain code can return it
// directly and the HTTP layer can unwrap it.
type Error struct {
	Code Code `json:"code"`
	// Message is Russian and safe to display. Since docs/07 C3 the frontend
	// renders from Code and treats this as a fallback, but it must still never
	// leak SQL, stack traces or internal identifiers (docs/03-API-CONTRACT.md:118).
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`

	// cause is the underlying error. Never serialised — it is for server logs.
	cause error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches an internal error for logging. The cause never reaches the
// client.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// Default Russian messages. docs/03-API-CONTRACT.md:117 requires `message` to be
// Russian and user-facing; per docs/07 C3 the frontend prefers its own dictionary
// keyed on Code, so these are the fallback when a key is missing.
var defaultMessages = map[Code]string{
	CodeValidationFailed: "Проверьте заполненные поля",
	CodeUnauthenticated:  "Требуется вход в систему",
	CodeForbidden:        "Недостаточно прав",
	CodeNotFound:         "Запись не найдена",
	CodeConflict:         "Конфликт данных",
	CodeVersionConflict:  "Запись была изменена другим пользователем",
	CodeRateLimited:      "Слишком много запросов, попробуйте позже",
	CodeBusinessRule:     "Действие недопустимо",
	CodeInternal:         "Внутренняя ошибка сервера",
}

// New builds an error with the default Russian message for the code.
func New(c Code) *Error {
	return &Error{Code: c, Message: defaultMessages[c]}
}

// Newf builds an error with a custom Russian message.
func Newf(c Code, message string, args ...any) *Error {
	return &Error{Code: c, Message: fmt.Sprintf(message, args...)}
}

// Validation builds a validation error from field problems.
func Validation(details ...FieldError) *Error {
	e := New(CodeValidationFailed)
	e.Details = details
	return e
}

// Convenience constructors for the errors domain code raises most.
func NotFound() *Error        { return New(CodeNotFound) }
func Forbidden() *Error       { return New(CodeForbidden) }
func Unauthenticated() *Error { return New(CodeUnauthenticated) }

// VersionConflict is returned when a PATCH carries a stale version
// (docs/03-API-CONTRACT.md §7). The current version is included so the client can
// show what it is up against without a second round trip.
func VersionConflict(current int32) *Error {
	e := New(CodeVersionConflict)
	e.Details = []FieldError{{
		Field:   "version",
		Code:    "stale",
		Message: fmt.Sprintf("Текущая версия записи: %d", current),
	}}
	return e
}

// BusinessRule refuses a well-formed, permitted request on domain grounds — for
// example loading a shipment line with a batch that is not released.
func BusinessRule(message string) *Error {
	return &Error{Code: CodeBusinessRule, Message: message}
}

// AsError extracts an *Error from an error chain, or wraps an unknown error as an
// internal error. Unknown errors never surface their text to the client: an
// unexpected error is exactly the one most likely to contain a SQL fragment or a
// file path.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr
	}
	return New(CodeInternal).WithCause(err)
}
