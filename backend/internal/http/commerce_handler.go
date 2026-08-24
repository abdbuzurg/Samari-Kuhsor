package http

import (
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/qoim/samari/backend/internal/api"
	"github.com/qoim/samari/backend/internal/db"
	"github.com/qoim/samari/backend/internal/domain/procurement"
	"github.com/qoim/samari/backend/internal/domain/sales"
	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Закупки, Продажи, Логистика — docs/05-MODULES.md §8, §9, §10.

// poStatus — docs/05-MODULES.md:181. Only `closed` is green: an order is not
// healthy because it exists, it is healthy because the goods arrived. `approval`
// is amber because it is blocking someone.
func poStatus(status string) api.Status {
	switch status {
	case procurement.StatusApproval:
		return api.Status{Key: status, Label: "На согласовании", Level: string(common.LevelWarn)}
	case procurement.StatusConfirmed:
		return api.Status{Key: status, Label: "Подтверждён", Level: string(common.LevelInfo)}
	case procurement.StatusInTransit:
		return api.Status{Key: status, Label: "В пути", Level: string(common.LevelInfo)}
	case procurement.StatusReceiving:
		return api.Status{Key: status, Label: "Приёмка", Level: string(common.LevelInfo)}
	case procurement.StatusClosed:
		return api.Status{Key: status, Label: "Закрыт", Level: string(common.LevelOK)}
	case procurement.StatusCancelled:
		return api.Status{Key: status, Label: "Отменён", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Черновик", Level: string(common.LevelNeutral)}
	}
}

func soStatus(status string) api.Status {
	switch status {
	case sales.SOStatusConfirmed:
		return api.Status{Key: status, Label: "Подтверждён", Level: string(common.LevelInfo)}
	case sales.SOStatusPicking:
		return api.Status{Key: status, Label: "Комплектуется", Level: string(common.LevelInfo)}
	case sales.SOStatusShipped:
		return api.Status{Key: status, Label: "Отгружен", Level: string(common.LevelInfo)}
	case sales.SOStatusClosed:
		return api.Status{Key: status, Label: "Закрыт", Level: string(common.LevelOK)}
	case sales.SOStatusCancelled:
		return api.Status{Key: status, Label: "Отменён", Level: string(common.LevelNeutral)}
	default:
		return api.Status{Key: status, Label: "Черновик", Level: string(common.LevelNeutral)}
	}
}

func shipStatus(status string) api.Status {
	switch status {
	case sales.ShipLoading:
		return api.Status{Key: status, Label: "Погрузка", Level: string(common.LevelInfo)}
	case sales.ShipDelivered:
		return api.Status{Key: status, Label: "Доставлено", Level: string(common.LevelOK)}
	default:
		return api.Status{Key: status, Label: "В пути", Level: string(common.LevelInfo)}
	}
}

// ---------------------------------------------------------------------------
// Поставщики
// ---------------------------------------------------------------------------

// handleGetSupplier serves the supplier record behind Закупки.
func (s *Server) handleGetSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	row, err := s.svc.Procurement.Supplier(r.Context(), id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, supplierResponse(row))
}

func supplierResponse(s db.Supplier) api.Supplier {
	return api.Supplier{
		ID: s.ID.String(), Name: s.Name, TaxID: s.TaxID,
		Contact: s.Contact, Region: s.Region, Rating: s.Rating, Version: s.Version,
	}
}

func (s *Server) handleListSuppliers(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, procurement.SupplierSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Procurement.Suppliers(r.Context(), params)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Supplier, 0, len(rows))
	for _, row := range rows {
		out = append(out, supplierResponse(row))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleCreateSupplier(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.SupplierWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	supplier, err := s.svc.Procurement.CreateSupplier(r.Context(), ident.User.ID,
		procurement.SupplierInput{
			Name: req.Name, TaxID: req.TaxID, Contact: req.Contact,
			Region: req.Region, Rating: req.Rating,
		})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, supplierResponse(supplier))
}

// ---------------------------------------------------------------------------
// Заказы поставщикам
// ---------------------------------------------------------------------------

func (s *Server) handleListPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, procurement.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Procurement.List(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.PurchaseOrderRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.PurchaseOrderRow{
			ID:           row.PurchaseOrder.ID.String(),
			PONo:         row.PurchaseOrder.PoNo,
			SupplierID:   row.PurchaseOrder.SupplierID.String(),
			SupplierName: row.SupplierName,
			ExpectedAt:   common.Date(row.PurchaseOrder.ExpectedAt),
			Total:        row.TotalAmount.StringFixed(2),
			Status:       poStatus(row.PurchaseOrder.Status),
			Version:      row.PurchaseOrder.Version,
			CreatedAt:    common.Timestamp(row.PurchaseOrder.CreatedAt),
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleGetPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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
	ctx := r.Context()
	po, err := s.svc.Procurement.Get(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines, err := s.svc.Procurement.Lines(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	total := decimal.Zero
	out := api.PurchaseOrder{Lines: make([]api.PurchaseOrderLine, 0, len(lines))}
	for _, l := range lines {
		lineTotal := l.Qty.Mul(l.UnitPrice)
		total = total.Add(lineTotal)
		out.Lines = append(out.Lines, api.PurchaseOrderLine{
			ID:          l.ID.String(),
			ItemID:      l.ItemID.String(),
			SKU:         l.Sku,
			ItemName:    l.ItemName,
			Qty:         l.Qty.String(),
			ReceivedQty: l.ReceivedQty.String(),
			UnitPrice:   l.UnitPrice.StringFixed(2),
			LineTotal:   lineTotal.StringFixed(2),
		})
	}
	out.ID = po.ID.String()
	out.PONo = po.PoNo
	out.SupplierID = po.SupplierID.String()
	out.ExpectedAt = common.Date(po.ExpectedAt)
	out.Total = total.StringFixed(2)
	out.Status = poStatus(po.Status)
	out.Version = po.Version
	out.CreatedAt = common.Timestamp(po.CreatedAt)
	out.AllowedTransitions = procurement.AllowedFrom(po.Status,
		rbac.NewSet(ident.Permissions).Can(rbac.Procurement, rbac.Approve))
	common.JSON(w, http.StatusOK, out)
}

func (s *Server) handleCreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.PurchaseOrderWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	supplierID, err := parseUUIDField(req.SupplierID, "supplier_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	expectedAt, err := parseDate(deref(req.ExpectedAt), "expected_at", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines := make([]procurement.LineInput, 0, len(req.Lines))
	for i, l := range req.Lines {
		itemID, err := parseUUIDField(l.ItemID, indexedField("lines", i, "item_id"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		qty, err := parseDecimalField(l.Qty, indexedField("lines", i, "qty"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		price, err := parseDecimalField(l.UnitPrice, indexedField("lines", i, "unit_price"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		lines = append(lines, procurement.LineInput{ItemID: itemID, Qty: qty, UnitPrice: price})
	}

	po, err := s.svc.Procurement.CreateOrder(r.Context(), ident.User.ID, procurement.OrderInput{
		PONo: req.PONo, SupplierID: supplierID, ExpectedAt: expectedAt, Lines: lines,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.PurchaseOrderRow{
		ID: po.ID.String(), PONo: po.PoNo, SupplierID: po.SupplierID.String(),
		ExpectedAt: common.Date(po.ExpectedAt), Status: poStatus(po.Status),
		Version: po.Version, CreatedAt: common.Timestamp(po.CreatedAt),
	})
}

func (s *Server) handleTransitionPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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
	var req api.TransitionRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	po, err := s.svc.Procurement.Transition(r.Context(), ident.User.ID, procurement.TransitionInput{
		POID: id, To: req.To,
		HasApprove: rbac.NewSet(ident.Permissions).Can(rbac.Procurement, rbac.Approve),
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.PurchaseOrderRow{
		ID: po.ID.String(), PONo: po.PoNo, SupplierID: po.SupplierID.String(),
		Status: poStatus(po.Status), Version: po.Version,
		CreatedAt: common.Timestamp(po.CreatedAt),
	})
}

// handleReceivePurchaseOrder records a receipt and posts the stock in one
// transaction — the receipt and the ledger entries are the same event.
func (s *Server) handleReceivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
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
	var req api.GoodsReceiptRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	locationID, err := parseUUIDField(req.LocationID, "location_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines := make([]procurement.ReceiptLineInput, 0, len(req.Lines))
	for i, l := range req.Lines {
		lineID, err := parseUUIDField(l.POLineID, indexedField("lines", i, "po_line_id"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		qty, err := parseDecimalField(l.Qty, indexedField("lines", i, "qty"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		batchID, err := parseNullUUIDField(l.BatchID, indexedField("lines", i, "batch_id"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		lines = append(lines, procurement.ReceiptLineInput{POLineID: lineID, Qty: qty, BatchID: batchID})
	}

	receipt, err := s.svc.Procurement.Receive(r.Context(), ident.User.ID, procurement.ReceiptInput{
		POID: id, LocationID: locationID, Lines: lines, Note: req.Note,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.GoodsReceipt{
		ID: receipt.ID.String(), POID: receipt.PoID.String(),
		ReceivedAt: common.Timestamp(receipt.CreatedAt),
	})
}

// ---------------------------------------------------------------------------
// Заказы клиентов
// ---------------------------------------------------------------------------

func (s *Server) handleListSalesOrders(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, sales.SortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Sales.ListOrders(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.SalesOrderRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.SalesOrderRow{
			ID:           row.SalesOrder.ID.String(),
			SONo:         row.SalesOrder.SoNo,
			CustomerID:   row.SalesOrder.CustomerID.String(),
			CustomerName: row.CustomerName,
			OrderedOn:    common.Date(row.SalesOrder.OrderedOn),
			Total:        row.TotalAmount.StringFixed(2),
			Status:       soStatus(row.SalesOrder.Status),
			Version:      row.SalesOrder.Version,
			CreatedAt:    common.Timestamp(row.SalesOrder.CreatedAt),
		})
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func (s *Server) handleGetSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	so, err := s.svc.Sales.GetOrder(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines, err := s.svc.Sales.OrderLines(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}

	total := decimal.Zero
	out := api.SalesOrder{Lines: make([]api.SalesOrderLine, 0, len(lines))}
	for _, l := range lines {
		lineTotal := l.Qty.Mul(l.UnitPrice)
		total = total.Add(lineTotal)
		line := api.SalesOrderLine{
			ID: l.ID.String(), ItemID: l.ItemID.String(), SKU: l.Sku, ItemName: l.ItemName,
			BatchNo: l.BatchNo, Qty: l.Qty.String(),
			UnitPrice: l.UnitPrice.StringFixed(2), LineTotal: lineTotal.StringFixed(2),
		}
		if l.BatchID.Valid {
			b := l.BatchID.UUID.String()
			line.BatchID = &b
		}
		out.Lines = append(out.Lines, line)
	}
	out.ID = so.ID.String()
	out.SONo = so.SoNo
	out.CustomerID = so.CustomerID.String()
	out.OrderedOn = common.Date(so.OrderedOn)
	out.Total = total.StringFixed(2)
	out.Status = soStatus(so.Status)
	out.Version = so.Version
	out.CreatedAt = common.Timestamp(so.CreatedAt)
	common.JSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateSalesOrder(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.SalesOrderWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	customerID, err := parseUUIDField(req.CustomerID, "customer_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	orderedOn, err := parseDate(deref(req.OrderedOn), "ordered_on", false)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines := make([]sales.OrderLineInput, 0, len(req.Lines))
	for i, l := range req.Lines {
		itemID, err := parseUUIDField(l.ItemID, indexedField("lines", i, "item_id"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		batchID, err := parseNullUUIDField(l.BatchID, indexedField("lines", i, "batch_id"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		qty, err := parseDecimalField(l.Qty, indexedField("lines", i, "qty"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		price, err := parseDecimalField(l.UnitPrice, indexedField("lines", i, "unit_price"))
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		lines = append(lines, sales.OrderLineInput{
			ItemID: itemID, BatchID: batchID, Qty: qty, UnitPrice: price,
		})
	}

	so, err := s.svc.Sales.CreateOrder(r.Context(), ident.User.ID, sales.OrderInput{
		SONo: req.SONo, CustomerID: customerID, OrderedOn: orderedOn, Lines: lines,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.SalesOrderRow{
		ID: so.ID.String(), SONo: so.SoNo, CustomerID: so.CustomerID.String(),
		OrderedOn: common.Date(so.OrderedOn), Status: soStatus(so.Status),
		Version: so.Version, CreatedAt: common.Timestamp(so.CreatedAt),
	})
}

// handleConfirmSalesOrder is the gate. A draft may quote anything; confirming
// checks every line's batch is released and reserves the stock. Both rules live
// in the domain — this handler only names the location they apply at.
func (s *Server) handleConfirmSalesOrder(w http.ResponseWriter, r *http.Request) {
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
	var req api.ConfirmOrderRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	locationID, err := parseUUIDField(req.LocationID, "location_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	so, err := s.svc.Sales.ConfirmOrder(r.Context(), ident.User.ID, id, locationID)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.JSON(w, http.StatusOK, api.SalesOrderRow{
		ID: so.ID.String(), SONo: so.SoNo, CustomerID: so.CustomerID.String(),
		Status: soStatus(so.Status), Version: so.Version,
		CreatedAt: common.Timestamp(so.CreatedAt),
	})
}

// ---------------------------------------------------------------------------
// Логистика
// ---------------------------------------------------------------------------

func (s *Server) handleListShipments(w http.ResponseWriter, r *http.Request) {
	params, err := common.ParseParams(r, sales.ShipmentSortSpec)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	rows, total, err := s.svc.Sales.Shipments(r.Context(), params, optionalQuery(r, "status"))
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.Shipment, 0, len(rows))
	for _, row := range rows {
		out = append(out, shipmentResponse(row.Shipment, row.DriverName, row.VehiclePlate, nil))
	}
	common.List(w, out, common.NewPageMeta(params, total))
}

func shipmentResponse(sh db.Shipment, driver, plate *string, lines []api.ShipmentLine) api.Shipment {
	out := api.Shipment{
		ID: sh.ID.String(), TripNo: sh.TripNo,
		RouteFrom: sh.RouteFrom, RouteTo: sh.RouteTo,
		DriverName: driver, VehiclePlate: plate,
		Status: shipStatus(sh.Status), Version: sh.Version,
		CreatedAt: common.Timestamp(sh.CreatedAt),
		Lines:     lines,
	}
	if out.Lines == nil {
		out.Lines = []api.ShipmentLine{}
	}
	if sh.DriverID.Valid {
		id := sh.DriverID.UUID.String()
		out.DriverID = &id
	}
	if sh.VehicleID.Valid {
		id := sh.VehicleID.UUID.String()
		out.VehicleID = &id
	}
	if sh.TransportCost.Valid {
		cost := sh.TransportCost.Decimal.StringFixed(2)
		out.TransportCost = &cost
	}
	return out
}

func (s *Server) handleGetShipment(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	sh, err := s.svc.Sales.GetShipment(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	lines, err := s.svc.Sales.ShipmentLines(ctx, id)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	out := make([]api.ShipmentLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, api.ShipmentLine{
			ID: l.ID.String(), ItemID: l.ItemID.String(), SKU: l.Sku, ItemName: l.ItemName,
			BatchID: l.BatchID.String(), BatchNo: l.BatchNo, Qty: l.Qty.String(),
		})
	}
	// common.JSON already builds the envelope (respond.go:38). This handler was
	// wrapping it a second time and shipping {"data":{"data":…}} — one of the 27
	// T34 found, and the only one that survived. It survived because the envelope
	// guard skips any route that 404s, and every /{id} case in its table uses a
	// fabricated id. Found by driving the browser (R18).
	common.JSON(w, http.StatusOK, shipmentResponse(sh, nil, nil, out))
}

func (s *Server) handleCreateShipment(w http.ResponseWriter, r *http.Request) {
	ident, ok := identityFrom(r)
	if !ok {
		common.Fail(w, r, common.Unauthenticated())
		return
	}
	var req api.ShipmentWriteRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	driverID, err := parseNullUUIDField(req.DriverID, "driver_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	vehicleID, err := parseNullUUIDField(req.VehicleID, "vehicle_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	var cost decimal.NullDecimal
	if req.TransportCost != nil && *req.TransportCost != "" {
		d, err := parseDecimalField(*req.TransportCost, "transport_cost")
		if err != nil {
			common.Fail(w, r, err)
			return
		}
		cost = decimal.NullDecimal{Decimal: d, Valid: true}
	}

	sh, err := s.svc.Sales.CreateShipment(r.Context(), ident.User.ID, sales.ShipmentInput{
		TripNo: req.TripNo, RouteFrom: req.RouteFrom, RouteTo: req.RouteTo,
		DriverID: driverID, VehicleID: vehicleID, TransportCost: cost,
	})
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, shipmentResponse(sh, nil, nil, nil))
}

// handleLoadShipment puts one batch on the lorry. The released-batch check is in
// the domain: a lorry leaving Хорог with quarantined product is the failure the
// whole quality chain exists to prevent, so it is enforced at the last step too.
func (s *Server) handleLoadShipment(w http.ResponseWriter, r *http.Request) {
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
	var req api.ShipmentLoadRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.Fail(w, r, err)
		return
	}
	itemID, err := parseUUIDField(req.ItemID, "item_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	batchID, err := parseUUIDField(req.BatchID, "batch_id")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	qty, err := parseDecimalField(req.Qty, "qty")
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	line, err := s.svc.Sales.LoadLine(r.Context(), ident.User.ID, id, itemID, batchID, qty)
	if err != nil {
		common.Fail(w, r, err)
		return
	}
	common.Created(w, api.ShipmentLine{
		ID: line.ID.String(), ItemID: line.ItemID.String(),
		BatchID: line.BatchID.String(), Qty: line.Qty.String(),
	})
}

// indexedField names a field inside an array, so a validation error on the third
// line says `lines[2].qty` rather than `qty` — with ten lines on screen, the
// unqualified name is not actionable.
func indexedField(array string, i int, field string) string {
	return array + "[" + strconv.Itoa(i) + "]." + field
}
