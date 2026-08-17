package http

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/admin"
	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Интеграция с сайтом, Уведомления, Администрирование —
// docs/05-MODULES.md §11, §17, §18.

// ---------------------------------------------------------------------------
// Интеграция с сайтом
// ---------------------------------------------------------------------------

func inquiryType(kind string) api.Status {
	switch kind {
	case inquiries.TypeWholesale:
		return api.Status{Key: kind, Label: "Оптовый запрос", Level: string(common.LevelInfo)}
	case inquiries.TypeDistributor:
		return api.Status{Key: kind, Label: "Дистрибуция", Level: string(common.LevelInfo)}
	case inquiries.TypeJob:
		return api.Status{Key: kind, Label: "Вакансия", Level: string(common.LevelNeutral)}
	// A complaint is danger, and it is the only enquiry type that is: it may mean
	// product in the field is wrong, and it is the entry point to the ToR's
	// traceability workflow.
	case inquiries.TypeComplaint:
		return api.Status{Key: kind, Label: "Жалоба", Level: string(common.LevelDanger)}
	default:
		return api.Status{Key: kind, Label: "Обращение", Level: string(common.LevelNeutral)}
	}
}

func inquiryStatus(status string) api.Status {
	switch status {
	case inquiries.StatusLeadCreated:
		return api.Status{Key: status, Label: "Создан лид", Level: string(common.LevelOK)}
	case inquiries.StatusClosed:
		return api.Status{Key: status, Label: "Закрыто", Level: string(common.LevelNeutral)}
	default:
		// `new` is amber, not green: an unanswered enquiry is work outstanding.
		return api.Status{Key: status, Label: "Новое", Level: string(common.LevelWarn)}
	}
}

func (s *Server) handleListInquiries(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, inquiries.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Inquiries.List(r.Context(), params, inquiries.ListFilter{
		Status: optionalQuery(r, "status"), Type: optionalQuery(r, "type"),
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Inquiry, 0, len(rows))
	for _, row := range rows {
		inq := api.Inquiry{
			ID:          row.ID.String(),
			ReferenceNo: row.ReferenceNo,
			Type:        inquiryType(row.InquiryType),
			Name:        row.Name,
			Company:     row.Company,
			Contact:     row.Contact,
			Message:     row.Message,
			BatchNo:     row.BatchNo,
			Status:      inquiryStatus(row.Status),
			SubmittedAt: common.Timestamp(row.CreatedAt),
			Version:     row.Version,
		}
		if row.BatchID.Valid {
			id := row.BatchID.UUID.String()
			inq.BatchID = &id
		}
		out = append(out, inq)
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// handleSubmitInquiry is the ONLY unauthenticated write in the system.
//
// It still travels browser → website BFF → Go: the service key proves the caller
// is our own website rather than a script, and the domain rate-limits by the
// visitor's IP on top of that (docs/03-API-CONTRACT.md:249).
func (s *Server) handleSubmitInquiry(w http.ResponseWriter, r *http.Request) {
	var req api.InquirySubmitRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	batchID, err := parseNullUUIDField(req.BatchID, "batch_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ip := clientIP(r)
	inquiry, err := s.svc.Inquiries.Submit(r.Context(), inquiries.SubmitInput{
		Type: req.Type, Name: req.Name, Company: req.Company,
		Contact: req.Contact, Message: req.Message, BatchID: batchID, IP: ip,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	// The reference number and nothing else. A fuller echo would confirm to an
	// anonymous caller what the system stored, and there is no reason to.
	common.Created(w, api.InquiryReceipt{ReferenceNo: inquiry.ReferenceNo})
}

func (s *Server) handleConvertInquiry(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lead, err := s.svc.Inquiries.ConvertToLead(r.Context(), ident.User.ID, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.Lead{
		ID: lead.ID.String(), Source: lead.Source,
		Status: api.Status{Key: lead.Status, Label: "Новый", Level: string(common.LevelInfo)},
	})
}

// ---------------------------------------------------------------------------
// Уведомления
// ---------------------------------------------------------------------------

// handleAlerts serves the bell and the sidebar pills from one call, so they can
// never disagree about how many open items a module has.
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	ctx := r.Context()
	perms := rbac.NewSet(ident.Permissions)

	feed, err := s.svc.Alerts.Feed(ctx, ident.User.ID, perms, 50)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	conditions, err := s.svc.Alerts.Open(ctx, perms)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	counts, err := s.svc.Alerts.Counts(ctx, perms)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	unread, err := s.svc.Alerts.Unread(ctx, ident.User.ID, perms)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	out := api.AlertFeed{
		Notifications: make([]api.Notification, 0, len(feed)),
		Conditions:    make([]api.AlertCondition, 0, len(conditions)),
		Unread:        unread,
		Counts:        counts,
	}
	for _, n := range feed {
		item := api.Notification{
			ID: n.ID.String(), Kind: n.Kind, Resource: n.Resource,
			Level: n.Level, Title: n.Title, Body: n.Body,
			OccurredAt: common.Timestamp(n.OccurredAt), IsRead: n.IsRead,
		}
		if n.ResourceID.Valid {
			id := n.ResourceID.UUID.String()
			item.ResourceID = &id
		}
		out.Notifications = append(out.Notifications, item)
	}
	for _, c := range conditions {
		out.Conditions = append(out.Conditions, api.AlertCondition{
			Kind: string(c.Kind), Resource: c.Resource,
			Level: string(c.Level), Count: c.Count,
		})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handleMarkAlertsRead(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	if err := s.svc.Alerts.MarkAllRead(r.Context(), ident.User.ID,
		rbac.NewSet(ident.Permissions)); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

// ---------------------------------------------------------------------------
// Администрирование
// ---------------------------------------------------------------------------

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roles, err := s.svc.Admin.Roles(ctx)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.RoleDetail, 0, len(roles))
	for _, role := range roles {
		perms, err := s.svc.Admin.RolePermissions(ctx, role.ID)
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		out = append(out, api.RoleDetail{
			ID: role.ID.String(), Key: role.Key, Name: role.NameRu,
			Permissions: perms, UserCount: int64(role.UserCount), Version: role.Version,
		})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.RoleWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	role, err := s.svc.Admin.CreateRole(r.Context(), ident.User.ID,
		admin.RoleInput{Key: req.Key, NameRU: req.Name})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.RoleDetail{
		ID: role.ID.String(), Key: role.Key, Name: role.NameRu,
		Permissions: []string{}, Version: role.Version,
	})
}

func (s *Server) handleSetRolePermissions(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.RolePermissionsRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	if err := s.svc.Admin.SetRolePermissions(r.Context(), ident.User.ID, id, req.Permissions); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	if err := s.svc.Admin.DeleteRole(r.Context(), ident.User.ID, id); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

func adminUserResponse(u db.ListUsersWithRolesRow) api.AdminUserRow {
	status := api.Status{Key: "active", Label: "Активен", Level: string(common.LevelOK)}
	if !u.IsActive {
		status = api.Status{Key: "inactive", Label: "Отключён", Level: string(common.LevelNeutral)}
	}
	roles := u.RoleKeys
	if roles == nil {
		roles = []string{}
	}
	return api.AdminUserRow{
		ID: u.ID.String(), Email: u.Email, FullName: u.FullName,
		IsActive: u.IsActive, Status: status, Roles: roles,
	}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, admin.UserSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Admin.Users(r.Context(), params)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.AdminUserRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminUserResponse(row))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleSetUserRoles(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.UserRolesRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	roleIDs := make([]uuid.UUID, 0, len(req.RoleIDs))
	for i, raw := range req.RoleIDs {
		roleID, err := parseUUIDField(raw, indexedField("role_ids", i, ""))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err := s.svc.Admin.SetUserRoles(r.Context(), ident.User.ID, id, roleIDs); err != nil {
		common.Fail(w, r, err)
		return
	}
	common.NoContent(w)
}

func (s *Server) handleSetUserActive(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var req api.UserActiveRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	user, err := s.svc.Admin.SetUserActive(r.Context(), ident.User.ID, id, req.Active)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	status := api.Status{Key: "active", Label: "Активен", Level: string(common.LevelOK)}
	if !user.IsActive {
		status = api.Status{Key: "inactive", Label: "Отключён", Level: string(common.LevelNeutral)}
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": api.AdminUserRow{
		ID: user.ID.String(), Email: user.Email, FullName: user.FullName,
		IsActive: user.IsActive, Status: status, Roles: []string{}, Version: user.Version,
	}})
}

// handlePermissionCatalogue serves the role editor its checkbox list.
//
// Generated from rbac's own tables, so the editor cannot offer a permission the
// middleware does not recognise, and a new module's permissions appear here the
// moment they are declared.
func (s *Server) handlePermissionCatalogue(w http.ResponseWriter, r *http.Request) {
	actions := make([]string, 0, len(rbac.AllActions))
	for _, a := range rbac.AllActions {
		actions = append(actions, string(a))
	}
	out := api.PermissionCatalogue{
		Resources: make([]api.PermissionResource, 0, len(rbac.Resources)),
	}
	for _, res := range rbac.Resources {
		out.Resources = append(out.Resources, api.PermissionResource{Key: res, Actions: actions})
	}
	common.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// handleAuditLog serves the audit viewer (docs/04-RBAC.md §6).
//
// Read-only, and not by convention: audit_log has no UPDATE and no DELETE query
// anywhere in this repository and no deleted_at column to tombstone with. There
// is no route that could edit an entry because there is no query that could.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, admin.AuditSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	filter := admin.AuditFilter{Resource: optionalQuery(r, "resource")}
	if raw := r.URL.Query().Get("actor_id"); raw != "" {
		id, err := parseUUIDField(raw, "actor_id")
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		filter.ActorID = uuid.NullUUID{UUID: id, Valid: true}
	}
	if raw := r.URL.Query().Get("resource_id"); raw != "" {
		id, err := parseUUIDField(raw, "resource_id")
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		filter.ResourceID = uuid.NullUUID{UUID: id, Valid: true}
	}

	rows, total, err := s.svc.Admin.Audit(r.Context(), params, filter)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.AuditEntry, 0, len(rows))
	for _, row := range rows {
		entry := api.AuditEntry{
			ID: row.ID.String(), Action: row.Action, Resource: row.Resource,
			ActorName:  row.ActorName,
			OccurredAt: common.Timestamp(row.OccurredAt),
			Before:     rawJSON(row.Before),
			After:      rawJSON(row.After),
		}
		if row.ResourceID.Valid {
			id := row.ResourceID.UUID.String()
			entry.ResourceID = &id
		}
		if row.ActorID.Valid {
			id := row.ActorID.UUID.String()
			entry.ActorID = &id
		}
		if row.Ip != nil {
			ip := row.Ip.String()
			entry.IP = &ip
		}
		out = append(out, entry)
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

// rawJSON passes a jsonb column through as JSON rather than as a base64 string.
//
// Without this the before/after payloads reach the browser as the raw []byte
// Go marshals them from, which renders as base64 and is useless to a reader.
func rawJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}
