// Package http wires the API: middleware, routing and handlers.
//
// Topology (docs/03-API-CONTRACT.md §1):
//
//	Browser --httpOnly cookie--> Next.js BFF --Bearer + service key--> Go --> Postgres
//
// The browser never reaches this package. In production the api container
// publishes no host port at all, so it is reachable only by the two Next.js
// containers on the compose network (docs/07-IMPLEMENTATION-PLAN.md I8/I18).
package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/qoim/samari/backend/internal/alerts"
	"github.com/qoim/samari/backend/internal/auth"
	"github.com/qoim/samari/backend/internal/domain/admin"
	"github.com/qoim/samari/backend/internal/domain/analytics"
	"github.com/qoim/samari/backend/internal/domain/batches"
	"github.com/qoim/samari/backend/internal/domain/cms"
	"github.com/qoim/samari/backend/internal/domain/crm"
	"github.com/qoim/samari/backend/internal/domain/dashboard"
	"github.com/qoim/samari/backend/internal/domain/inquiries"
	"github.com/qoim/samari/backend/internal/domain/inventory"
	"github.com/qoim/samari/backend/internal/domain/items"
	"github.com/qoim/samari/backend/internal/domain/procurement"
	"github.com/qoim/samari/backend/internal/domain/production"
	"github.com/qoim/samari/backend/internal/domain/quality"
	"github.com/qoim/samari/backend/internal/domain/registries"
	"github.com/qoim/samari/backend/internal/domain/sales"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Config is the server's runtime configuration.
type Config struct {
	// ServiceKey proves the caller is one of our BFFs. It is NOT an identity:
	// it is the same for every request and says nothing about the user. Defence
	// in depth behind the unpublished port, never the only lock.
	ServiceKey string
}

// Services is every domain the API serves.
//
// A struct rather than a parameter list: with a dozen modules a positional
// signature is a standing invitation to swap two services of the same type, and
// the compiler would not notice.
type Services struct {
	Auth        *auth.Service
	Items       *items.Service
	Batches     *batches.Service
	Inventory   *inventory.Service
	Production  *production.Service
	Quality     *quality.Service
	Procurement *procurement.Service
	Sales       *sales.Service
	CRM         *crm.Service
	Inquiries   *inquiries.Service
	Registries  *registries.Service
	CMS         *cms.Service
	Dashboard   *dashboard.Service
	Admin       *admin.Service
	Alerts      *alerts.Service
	Analytics   *analytics.Service
}

type Server struct {
	svc    Services
	cfg    Config
	router chi.Router
	reg    *rbac.Registry
}

// NewServer builds the API. It returns an error rather than a Server if any route
// was registered without declaring its permission — the process must refuse to
// serve rather than expose an ungoverned endpoint (docs/04-RBAC.md:123).
func NewServer(svc Services, cfg Config) (*Server, error) {
	s := &Server{svc: svc, cfg: cfg, reg: rbac.NewRegistry()}

	// Sort whitelists are validated at startup: a default outside its own
	// whitelist would put an unvetted column name into an ORDER BY.
	if err := items.SortSpec.Validate(); err != nil {
		return nil, fmt.Errorf("items sort spec: %w", err)
	}

	// rbac cannot import this package (that would be a cycle), so it is given the
	// envelope-shaped responders here. Without this its 401/403 would be plain
	// text and would break the contract for exactly the two statuses every client
	// must branch on.
	rbac.Unauthorized = func(w http.ResponseWriter, r *http.Request) {
		common.Fail(w, r, common.Unauthenticated())
	}
	rbac.Forbidden = func(w http.ResponseWriter, r *http.Request) {
		common.Fail(w, r, common.Forbidden())
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Deliberately NOT middleware.RealIP: it is deprecated for IP spoofing because
	// it rewrites RemoteAddr from X-Forwarded-For / X-Real-IP whether or not the
	// infrastructure actually sets them. clientIP() reads the header explicitly,
	// and only because nothing but the BFF can reach this port (I8/I18).
	r.Use(requestLogger)
	r.Use(recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(s.requireServiceKey)
		api.Use(s.resolveIdentity)

		// chi.Walk reports full paths, so declarations are recorded under the
		// same mount prefix (docs/04-RBAC.md:123).
		v1 := s.reg.Scope("/api/v1")

		v1.Public(api, http.MethodPost, "/auth/login",
			"login must succeed before any permission can be resolved", s.handleLogin)
		v1.Public(api, http.MethodPost, "/auth/logout",
			"revoking your own session needs no module permission", s.handleLogout)
		v1.Public(api, http.MethodGet, "/auth/me",
			"returns the caller's own identity and permission list", s.handleMe)
		v1.Public(api, http.MethodGet, "/health",
			"liveness probe; returns no data", handleHealth)

		// Товары и цены. Reads need items:read; writes need items:manage, which
		// implies read (docs/04-RBAC.md §3).
		v1.Guarded(api, http.MethodGet, "/items",
			rbac.Items, rbac.Read, s.handleListItems)
		v1.Guarded(api, http.MethodGet, "/items/{id}",
			rbac.Items, rbac.Read, s.handleGetItem)
		v1.Guarded(api, http.MethodPost, "/items",
			rbac.Items, rbac.Manage, s.handleCreateItem)
		v1.Guarded(api, http.MethodPatch, "/items/{id}",
			rbac.Items, rbac.Manage, s.handleUpdateItem)
		v1.Guarded(api, http.MethodDelete, "/items/{id}",
			rbac.Items, rbac.Manage, s.handleDeleteItem)
		v1.Guarded(api, http.MethodPost, "/items/{id}/prices",
			rbac.Items, rbac.Manage, s.handleAddItemPrice)

		// Batches and QR issuance (D11). QR generation is needed BEFORE the plant
		// produces anything, because wrappers are ordered in advance.
		v1.Guarded(api, http.MethodPost, "/batches",
			rbac.Items, rbac.Manage, s.handleCreateBatch)
		v1.Guarded(api, http.MethodGet, "/batches/{id}",
			rbac.Items, rbac.Read, s.handleGetBatch)
		v1.Guarded(api, http.MethodPost, "/batches/{id}/qr",
			rbac.Items, rbac.Manage, s.handleIssueBatchQR)
		v1.Guarded(api, http.MethodGet, "/batches/{id}/qr.svg",
			rbac.Items, rbac.Read, s.handleBatchQRSVG)
		v1.Guarded(api, http.MethodGet, "/batches/qr-export",
			rbac.Items, rbac.Read, s.handleExportQR)

		// Панель управления. Guarded on dashboard:read; each PANEL is additionally
		// filtered by the module it summarises, inside the service — the landing
		// page is the one screen that would otherwise leak every module at once.
		v1.Guarded(api, http.MethodGet, "/dashboard",
			rbac.Dashboard, rbac.Read, s.handleDashboard)

		// Склад и запасы. Everything read here is a SUM computed at read time; the
		// only write is an append to the ledger.
		v1.Guarded(api, http.MethodGet, "/stock",
			rbac.Inventory, rbac.Read, s.handleListStock)
		v1.Guarded(api, http.MethodGet, "/stock/ledger",
			rbac.Inventory, rbac.Read, s.handleStockLedger)
		v1.Guarded(api, http.MethodGet, "/locations",
			rbac.Inventory, rbac.Read, s.handleListLocations)
		v1.Guarded(api, http.MethodPost, "/stock/movements",
			rbac.Inventory, rbac.Manage, s.handlePostMovement)
		v1.Guarded(api, http.MethodPost, "/stock/transfers",
			rbac.Inventory, rbac.Manage, s.handleTransfer)

		// Производство. Completion posts stock to quarantine and moves the batch,
		// so it is guarded as `manage` on production — the quality decision that
		// releases it afterwards is a separate authority.
		v1.Guarded(api, http.MethodGet, "/manufacturing-orders",
			rbac.Production, rbac.Read, s.handleListManufacturingOrders)
		v1.Guarded(api, http.MethodGet, "/manufacturing-orders/{id}",
			rbac.Production, rbac.Read, s.handleGetManufacturingOrder)
		v1.Guarded(api, http.MethodPost, "/manufacturing-orders",
			rbac.Production, rbac.Manage, s.handleCreateManufacturingOrder)
		v1.Guarded(api, http.MethodPost, "/manufacturing-orders/{id}/entries",
			rbac.Production, rbac.Manage, s.handleRecordProductionEntry)
		v1.Guarded(api, http.MethodPost, "/manufacturing-orders/{id}/complete",
			rbac.Production, rbac.Manage, s.handleCompleteManufacturingOrder)

		// Качество. The transition endpoint is `manage`, not `approve`: some moves
		// need approve and some do not, and the matrix decides which. Guarding the
		// whole route on approve would block the automatic move into quarantine.
		v1.Guarded(api, http.MethodGet, "/quality/batches",
			rbac.Quality, rbac.Read, s.handleListBatches)
		v1.Guarded(api, http.MethodGet, "/batches/{id}/detail",
			rbac.Quality, rbac.Read, s.handleBatchDetail)
		v1.Guarded(api, http.MethodPost, "/batches/{id}/tests",
			rbac.Quality, rbac.Manage, s.handleRecordQualityTest)
		v1.Guarded(api, http.MethodPost, "/batches/{id}/transition",
			rbac.Quality, rbac.Manage, s.handleTransitionBatch)

		// Закупки.
		v1.Guarded(api, http.MethodGet, "/suppliers",
			rbac.Procurement, rbac.Read, s.handleListSuppliers)
		v1.Guarded(api, http.MethodGet, "/suppliers/{id}",
			rbac.Procurement, rbac.Read, s.handleGetSupplier)
		v1.Guarded(api, http.MethodPost, "/suppliers",
			rbac.Procurement, rbac.Manage, s.handleCreateSupplier)
		v1.Guarded(api, http.MethodGet, "/purchase-orders",
			rbac.Procurement, rbac.Read, s.handleListPurchaseOrders)
		v1.Guarded(api, http.MethodGet, "/purchase-orders/{id}",
			rbac.Procurement, rbac.Read, s.handleGetPurchaseOrder)
		v1.Guarded(api, http.MethodPost, "/purchase-orders",
			rbac.Procurement, rbac.Manage, s.handleCreatePurchaseOrder)
		v1.Guarded(api, http.MethodPost, "/purchase-orders/{id}/transition",
			rbac.Procurement, rbac.Manage, s.handleTransitionPurchaseOrder)
		v1.Guarded(api, http.MethodPost, "/purchase-orders/{id}/receipts",
			rbac.Procurement, rbac.Manage, s.handleReceivePurchaseOrder)

		// CRM и продажи — the customer side. Sales orders follow below.
		//
		// Every one of these tables has existed since migration 00003 and had no
		// route until R12: `CreateCustomer` and `CreateLead` were reachable only
		// as a side effect of converting an enquiry, producing records no screen
		// could open.
		v1.Guarded(api, http.MethodGet, "/customers",
			rbac.CRM, rbac.Read, s.handleListCustomers)
		v1.Guarded(api, http.MethodGet, "/customers/{id}",
			rbac.CRM, rbac.Read, s.handleGetCustomer)
		v1.Guarded(api, http.MethodPost, "/customers",
			rbac.CRM, rbac.Manage, s.handleCreateCustomer)
		v1.Guarded(api, http.MethodPatch, "/customers/{id}",
			rbac.CRM, rbac.Manage, s.handleUpdateCustomer)
		v1.Guarded(api, http.MethodPost, "/customers/{id}/contacts",
			rbac.CRM, rbac.Manage, s.handleCreateContact)

		v1.Guarded(api, http.MethodGet, "/deals",
			rbac.CRM, rbac.Read, s.handleListDeals)
		v1.Guarded(api, http.MethodGet, "/deals/{id}",
			rbac.CRM, rbac.Read, s.handleGetDeal)
		v1.Guarded(api, http.MethodPost, "/deals",
			rbac.CRM, rbac.Manage, s.handleCreateDeal)
		// A POST to an action, not a PATCH of `stage`: the move is guarded by a
		// matrix and writes an immutable event, neither of which a field update
		// could express.
		v1.Guarded(api, http.MethodPost, "/deals/{id}/stage",
			rbac.CRM, rbac.Manage, s.handleMoveDealStage)

		v1.Guarded(api, http.MethodGet, "/tasks",
			rbac.CRM, rbac.Read, s.handleListTasks)
		v1.Guarded(api, http.MethodPost, "/tasks",
			rbac.CRM, rbac.Manage, s.handleCreateTask)
		v1.Guarded(api, http.MethodPut, "/tasks/{id}/status",
			rbac.CRM, rbac.Manage, s.handleSetTaskStatus)

		v1.Guarded(api, http.MethodGet, "/crm/kpis",
			rbac.CRM, rbac.Read, s.handleCRMKPIs)

		// Продажи.
		v1.Guarded(api, http.MethodGet, "/sales-orders",
			rbac.CRM, rbac.Read, s.handleListSalesOrders)
		v1.Guarded(api, http.MethodGet, "/sales-orders/{id}",
			rbac.CRM, rbac.Read, s.handleGetSalesOrder)
		v1.Guarded(api, http.MethodPost, "/sales-orders",
			rbac.CRM, rbac.Manage, s.handleCreateSalesOrder)
		v1.Guarded(api, http.MethodPost, "/sales-orders/{id}/confirm",
			rbac.CRM, rbac.Manage, s.handleConfirmSalesOrder)

		// Логистика.
		v1.Guarded(api, http.MethodGet, "/shipments",
			rbac.Logistics, rbac.Read, s.handleListShipments)
		v1.Guarded(api, http.MethodGet, "/shipments/{id}",
			rbac.Logistics, rbac.Read, s.handleGetShipment)
		v1.Guarded(api, http.MethodPost, "/shipments",
			rbac.Logistics, rbac.Manage, s.handleCreateShipment)
		v1.Guarded(api, http.MethodPost, "/shipments/{id}/lines",
			rbac.Logistics, rbac.Manage, s.handleLoadShipment)

		// Интеграция с сайтом. The submit endpoint is the one unauthenticated
		// write in the system: the website has no user to act as. It is still
		// behind the service key, and rate-limited by visitor IP in the domain.
		// The public read surface. Guarded by the service key like everything
		// else — "public" means no user session, not open to the internet — and
		// deliberately narrow: active finished goods and published news, with no
		// cost, supplier, stock or internal status anywhere in the payload.
		v1.Public(api, http.MethodGet, "/public/products",
			"the catalogue a visitor sees; no visitor has an account",
			s.handlePublicProducts)
		v1.Public(api, http.MethodGet, "/public/products/{sku}",
			"one product page; no visitor has an account",
			s.handlePublicProduct)
		v1.Public(api, http.MethodGet, "/public/news",
			"published news; no visitor has an account",
			s.handlePublicNews)
		// Аналитика сайта (D12). The second unauthenticated write, and unlike
		// the enquiry form it takes traffic on every click. It answers 204 for
		// everything: a caller who learns which of their forged SKUs was
		// rejected has been handed a catalogue enumeration tool.
		v1.Public(api, http.MethodPost, "/public/analytics",
			"website statistics from visitors who have no account; validated and rate-limited inside",
			s.handleAnalyticsCollect)
		v1.Public(api, http.MethodPost, "/public/inquiries",
			"public website form; no visitor has an account to authenticate with",
			s.handleSubmitInquiry)
		v1.Guarded(api, http.MethodGet, "/inquiries",
			rbac.Inquiries, rbac.Read, s.handleListInquiries)
		v1.Guarded(api, http.MethodGet, "/inquiries/{id}",
			rbac.Inquiries, rbac.Read, s.handleGetInquiry)
		v1.Guarded(api, http.MethodPost, "/inquiries/{id}/convert",
			rbac.Inquiries, rbac.Manage, s.handleConvertInquiry)

		// Уведомления. Not guarded on a module: the feed is filtered per-resource
		// inside the service against the caller's own permissions, so any
		// authenticated user may ask and sees only what they may read.
		v1.Public(api, http.MethodGet, "/alerts",
			"filtered per-resource inside the service against the caller's own grants",
			s.handleAlerts)
		v1.Public(api, http.MethodPost, "/alerts/read",
			"marks read only what this caller was already permitted to see",
			s.handleMarkAlertsRead)

		// Персонал.
		v1.Guarded(api, http.MethodGet, "/employees",
			rbac.HR, rbac.Read, s.handleListEmployees)
		v1.Guarded(api, http.MethodGet, "/employees/{id}",
			rbac.HR, rbac.Read, s.handleGetEmployee)
		v1.Guarded(api, http.MethodPost, "/employees",
			rbac.HR, rbac.Manage, s.handleCreateEmployee)
		v1.Guarded(api, http.MethodPatch, "/employees/{id}",
			rbac.HR, rbac.Manage, s.handleUpdateEmployee)

		// Оборудование и ТО.
		v1.Guarded(api, http.MethodGet, "/assets",
			rbac.Equipment, rbac.Read, s.handleListAssets)
		v1.Guarded(api, http.MethodGet, "/assets/{id}",
			rbac.Equipment, rbac.Read, s.handleGetAsset)
		v1.Guarded(api, http.MethodPost, "/assets",
			rbac.Equipment, rbac.Manage, s.handleCreateAsset)
		v1.Guarded(api, http.MethodGet, "/assets/{id}/maintenance",
			rbac.Equipment, rbac.Read, s.handleAssetMaintenance)
		v1.Guarded(api, http.MethodPost, "/assets/{id}/maintenance",
			rbac.Equipment, rbac.Manage, s.handleRecordMaintenance)

		// Документы. The transition route is `manage`, not `approve`: sending a
		// draft for review needs no authority, and only activation does — which
		// the transition matrix enforces inside the domain.
		v1.Guarded(api, http.MethodGet, "/documents",
			rbac.Documents, rbac.Read, s.handleListDocuments)
		v1.Guarded(api, http.MethodGet, "/documents/{id}",
			rbac.Documents, rbac.Read, s.handleGetDocument)
		v1.Guarded(api, http.MethodPost, "/documents",
			rbac.Documents, rbac.Manage, s.handleCreateDocument)
		v1.Guarded(api, http.MethodPost, "/documents/{id}/transition",
			rbac.Documents, rbac.Manage, s.handleTransitionDocument)

		// CMS. The transition route is `manage`, not `approve`: moving a draft
		// into review needs no authority, and only approval and publication do —
		// which the ladder enforces inside the domain.
		v1.Guarded(api, http.MethodGet, "/cms/pages",
			rbac.CMS, rbac.Read, s.handleListContentPages)
		v1.Guarded(api, http.MethodGet, "/cms/pages/{id}/blocks",
			rbac.CMS, rbac.Read, s.handleListContentBlocks)
		v1.Guarded(api, http.MethodPut, "/cms/pages/{id}/blocks",
			rbac.CMS, rbac.Manage, s.handleSaveContentBlock)
		v1.Guarded(api, http.MethodPost, "/cms/pages/{id}/transition",
			rbac.CMS, rbac.Manage, s.handleTransitionContentPage)
		v1.Guarded(api, http.MethodGet, "/cms/pages/{id}/history",
			rbac.CMS, rbac.Read, s.handleContentHistory)

		v1.Guarded(api, http.MethodGet, "/cms/news",
			rbac.CMS, rbac.Read, s.handleListNewsPosts)
		v1.Guarded(api, http.MethodPost, "/cms/news",
			rbac.CMS, rbac.Manage, s.handleCreateNewsPost)
		v1.Guarded(api, http.MethodGet, "/cms/news/{id}/translations",
			rbac.CMS, rbac.Read, s.handleListNewsTranslations)
		v1.Guarded(api, http.MethodPut, "/cms/news/{id}/translations",
			rbac.CMS, rbac.Manage, s.handleSaveNewsTranslation)
		v1.Guarded(api, http.MethodPost, "/cms/news/{id}/transition",
			rbac.CMS, rbac.Manage, s.handleTransitionNewsPost)
		v1.Guarded(api, http.MethodGet, "/cms/news/{id}/history",
			rbac.CMS, rbac.Read, s.handleContentHistory)

		v1.Guarded(api, http.MethodGet, "/cms/media",
			rbac.CMS, rbac.Read, s.handleListMedia)
		v1.Guarded(api, http.MethodPut, "/cms/media/{id}/alt",
			rbac.CMS, rbac.Manage, s.handleSetMediaAlt)

		// Экспорт. ToR §4 and §8 condition 7.
		//
		// ONE ROUTE PER COLLECTION, each guarded on that collection's own module.
		// A single /export/{collection} route would have to be declared public,
		// because the permission is not known until the path parameter is read —
		// and a public route that reads any module's data is exactly what
		// docs/04-RBAC.md:123's boot-time verification exists to prevent. Static
		// declarations mean the guard is checked before we serve, not after.
		for _, e := range exportRoutes() {
			v1.Guarded(api, http.MethodGet, "/export/"+e.Key,
				e.Module, rbac.Read, s.handleExport)
		}

		// Администрирование. Everything here is admin:manage — there is no
		// read-only view of the permission system that is worth the surface.
		v1.Guarded(api, http.MethodGet, "/admin/roles",
			rbac.Admin, rbac.Read, s.handleListRoles)
		v1.Guarded(api, http.MethodPost, "/admin/roles",
			rbac.Admin, rbac.Manage, s.handleCreateRole)
		v1.Guarded(api, http.MethodPut, "/admin/roles/{id}/permissions",
			rbac.Admin, rbac.Manage, s.handleSetRolePermissions)
		v1.Guarded(api, http.MethodDelete, "/admin/roles/{id}",
			rbac.Admin, rbac.Manage, s.handleDeleteRole)
		v1.Guarded(api, http.MethodGet, "/admin/users",
			rbac.Admin, rbac.Read, s.handleListUsers)
		v1.Guarded(api, http.MethodPut, "/admin/users/{id}/roles",
			rbac.Admin, rbac.Manage, s.handleSetUserRoles)
		v1.Guarded(api, http.MethodPut, "/admin/users/{id}/active",
			rbac.Admin, rbac.Manage, s.handleSetUserActive)
		// The audit viewer. audit:read, not admin:manage — reading the trail and
		// changing who may do what are different authorities, and an auditor
		// should not need the power to grant themselves anything.
		v1.Guarded(api, http.MethodGet, "/audit",
			rbac.Audit, rbac.Read, s.handleAuditLog)
		// The two dashboard panels. analytics:read is held by admin and
		// director only — there is no `manage`, because website statistics are
		// written by visitors rather than staff.
		v1.Guarded(api, http.MethodGet, "/analytics/report",
			rbac.Analytics, rbac.Read, s.handleAnalyticsReport)
		v1.Guarded(api, http.MethodGet, "/admin/permissions",
			rbac.Admin, rbac.Read, s.handlePermissionCatalogue)
	})

	if err := rbac.Verify(r, s.reg); err != nil {
		return nil, err
	}

	s.router = r
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.router }

// Declarations exposes the full access surface for the startup log, so every boot
// prints exactly which routes are guarded and which are deliberately public.
func (s *Server) Declarations() []rbac.Declaration { return s.reg.Declarations() }

func handleHealth(w http.ResponseWriter, r *http.Request) {
	common.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requireServiceKey rejects anything that is not one of our BFFs.
func (s *Server) requireServiceKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.ServiceKey == "" { // not configured: development only
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-Service-Key")
		// Constant-time: a byte-by-byte comparison leaks the key one byte at a
		// time to anything that can measure response latency.
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.ServiceKey)) != 1 {
			common.Fail(w, r, common.Unauthenticated())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// resolveIdentity turns a Bearer token into a request-scoped identity.
//
// It does NOT reject anonymous requests: /auth/login and the public website
// endpoints are legitimately unauthenticated. Rejection is rbac.Require's job,
// which is what makes 401-versus-403 precise.
func (s *Server) resolveIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		ident, err := s.svc.Auth.Authenticate(r.Context(), token)
		if err != nil {
			// A bad token is anonymous, not an error: the route decides whether
			// anonymous is acceptable. Anything unexpected is still surfaced.
			if isSessionRejection(err) {
				next.ServeHTTP(w, r)
				return
			}
			common.Fail(w, r, err)
			return
		}

		ctx := withIdentity(r.Context(), ident)
		ctx = rbac.WithIdentity(ctx, rbac.Identity{
			UserID:      ident.User.ID.String(),
			Permissions: rbac.NewSet(ident.Permissions),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isSessionRejection(err error) bool {
	return errors.Is(err, auth.ErrSessionUnknown) ||
		errors.Is(err, auth.ErrSessionExpired) ||
		errors.Is(err, auth.ErrSessionRevoked) ||
		errors.Is(err, auth.ErrAccountInactive)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// Request-scoped identity, kept separate from rbac.Identity: rbac needs only the
// permission set, while handlers need the full user record.
type identityCtxKey struct{}

func withIdentity(ctx context.Context, ident auth.Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, ident)
}

func identityFrom(r *http.Request) (auth.Identity, bool) {
	ident, ok := r.Context().Value(identityCtxKey{}).(auth.Identity)
	return ident, ok
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// recoverer turns a panic into a 500 in the API's envelope. chi's own recoverer
// writes a bare status, which would be the one response in the system not shaped
// like every other.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", rec, "path", r.URL.Path)
				common.Fail(w, r, common.New(common.CodeInternal))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
