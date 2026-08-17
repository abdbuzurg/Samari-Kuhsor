package http

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/batches"
	"github.com/qoim/samari/backend/internal/http/common"
)

// batchStatus maps the stored status to its semantic level.
//
// docs/05-MODULES.md:141 — Карантин is `danger`, Выпущено is `ok`, Отклонено is
// `danger`. Green means healthy, so `released` is the only green one: a batch in
// quarantine is not "in progress", it is a problem until someone qualified says
// otherwise.
func batchStatus(status string) api.Status {
	switch status {
	case "released":
		return api.Status{Key: status, Label: "Выпущено", Level: string(common.LevelOK)}
	case "quarantine":
		return api.Status{Key: status, Label: "Карантин", Level: string(common.LevelDanger)}
	case "rejected":
		return api.Status{Key: status, Label: "Отклонено", Level: string(common.LevelDanger)}
	default:
		return api.Status{Key: status, Label: "В производстве", Level: string(common.LevelInfo)}
	}
}

func batchResponse(b db.Batch) api.Batch {
	return api.Batch{
		ID:         b.ID.String(),
		BatchNo:    b.BatchNo,
		ItemID:     b.ItemID.String(),
		ProducedOn: common.Date(b.ProducedOn),
		ExpiresOn:  common.Date(b.ExpiresOn),
		QRPayload:  b.QrPayload,
		QRIssuedAt: common.NullTimestamp(b.QrIssuedAt),
		Status:     batchStatus(b.Status),
		Version:    b.Version,
		CreatedAt:  common.Timestamp(b.CreatedAt),
	}
}

// Batches and QR issuance — docs/01-DECISIONS.md D11.

func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}

	var req api.BatchWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	itemID, err := uuid.Parse(req.ItemID)
	if err != nil {
		common.Fail(w, r, common.Validation(common.FieldError{
			Field: "item_id", Code: "invalid", Message: "Некорректный идентификатор товара",
		}))
		return
	}
	producedOn, err := parseDate(deref(req.ProducedOn), "produced_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	expiresOn, err := parseDate(deref(req.ExpiresOn), "expires_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	batch, err := s.svc.Batches.Create(r.Context(), ident.User.ID, batches.CreateInput{
		BatchNo:    req.BatchNo,
		ItemID:     itemID,
		ProducedOn: producedOn,
		ExpiresOn:  expiresOn,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, batchResponse(batch))
}

func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	batch, err := s.svc.Batches.Get(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, batchResponse(batch))
}

// handleIssueBatchQR is a sub-resource, not a PATCH: issuing a code is a
// decision that commits the company to ordering wrappers, and state transitions
// are sub-resources precisely so permissions and audit entries stay precise
// (docs/03-API-CONTRACT.md:74).
func (s *Server) handleIssueBatchQR(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	batch, err := s.svc.Batches.IssueQR(r.Context(), ident.User.ID, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, batchResponse(batch))
}

// handleBatchQRSVG renders one code for on-screen display.
func (s *Server) handleBatchQRSVG(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	svg, err := s.svc.Batches.SVGFor(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(svg)
}

// handleExportQR produces the printer handoff.
func (s *Server) handleExportQR(w http.ResponseWriter, r *http.Request) {
	var filter uuid.NullUUID
	if raw := r.URL.Query().Get("item_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			common.Fail(w, r, common.Validation(common.FieldError{
				Field: "item_id", Code: "invalid", Message: "Некорректный идентификатор товара",
			}))
			return
		}
		filter = uuid.NullUUID{UUID: id, Valid: true}
	}

	// Buffered rather than streamed: a mid-stream failure after the status line
	// is written would hand the printer a truncated ZIP that looks complete, and
	// a short export means a batch ships with the wrong wrapper.
	var buf bytes.Buffer
	count, err := s.svc.Batches.ExportQR(r.Context(), &buf, filter)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="samari-qr-codes.zip"`)
	w.Header().Set("X-Batch-Count", fmt.Sprint(count))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
