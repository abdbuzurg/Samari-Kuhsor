package http

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qoim/samari/backend/internal/http/common"
	"github.com/qoim/samari/backend/internal/rbac"
)

// Export — ToR §4 and §8 acceptance condition 7, "Reports can be exported".
//
// One exporter over the existing list handlers rather than a writer per module.
// A per-module exporter would be thirteen places for the permission check to be
// forgotten, and the first one forgotten is a data leak wearing a download
// button.
//
// The mechanism: re-enter the module's own list handler with the caller's
// request, capture the JSON it produces, and flatten it to CSV. That means an
// export is BY CONSTRUCTION the same rows, the same filter and the same RBAC as
// the screen it was launched from — there is no second query to drift.

// exportable maps an export key to the collection handler behind it and the
// permission that handler is already guarded on. The permission is repeated here
// only so the export route can be declared; the real check still happens in the
// middleware that wraps the inner handler.
type exportable struct {
	Key     string
	Module  string
	handler func(http.ResponseWriter, *http.Request)
	// columns is the field order for the CSV. Explicit rather than derived from
	// the JSON, because map iteration order is random and a report whose columns
	// move between runs is not a report.
	columns []exportColumn
}

type exportColumn struct {
	// path is a dotted field path into the row, e.g. "status.label".
	path   string
	header string
}

// exportRoutes lists the collections and their modules, without needing a Server.
// server.go declares one guarded route per entry at boot.
func exportRoutes() []struct {
	Key    string
	Module string
} {
	return []struct {
		Key    string
		Module string
	}{
		{"items", rbac.Items}, {"stock", rbac.Inventory}, {"quality", rbac.Quality},
		{"production", rbac.Production}, {"procurement", rbac.Procurement},
		{"sales-orders", rbac.CRM}, {"customers", rbac.CRM}, {"deals", rbac.CRM},
		{"logistics", rbac.Logistics}, {"hr", rbac.HR}, {"equipment", rbac.Equipment},
		{"documents", rbac.Documents}, {"inquiries", rbac.Inquiries}, {"audit", rbac.Audit},
	}
}

func (s *Server) exports() map[string]exportable {
	return map[string]exportable{
		"items": {handler: s.handleListItems, columns: []exportColumn{
			{"sku", "Артикул"}, {"name", "Наименование"}, {"item_type", "Тип"},
			{"category", "Категория"}, {"base_uom", "Ед. изм."},
			{"current_price.amount", "Цена"}, {"status.label", "Статус"},
		}},
		"stock": {handler: s.handleListStock, columns: []exportColumn{
			{"sku", "Артикул"}, {"item_name", "Наименование"},
			{"batch_no", "Партия"}, {"location_code", "Локация"},
			{"on_hand", "Остаток"}, {"base_uom", "Ед. изм."},
			{"expires_on", "Годен до"}, {"status.label", "Статус"},
		}},
		"quality": {handler: s.handleListBatches, columns: []exportColumn{
			{"batch_no", "Партия"}, {"sku", "Артикул"}, {"item_name", "Товар"},
			{"produced_on", "Произведена"}, {"expires_on", "Годен до"},
			{"test_count", "Проверок"}, {"failed_count", "Не пройдено"},
			{"status.label", "Статус"},
		}},
		"production": {handler: s.handleListManufacturingOrders, columns: []exportColumn{
			{"mo_no", "Заказ"}, {"sku", "Артикул"}, {"item_name", "Товар"},
			{"line", "Линия"}, {"scheduled_for", "Дата"},
			{"planned_qty", "План"}, {"good_qty", "Годных"}, {"scrap_qty", "Брак"},
			{"status.label", "Статус"},
		}},
		"procurement": {handler: s.handleListPurchaseOrders, columns: []exportColumn{
			{"po_no", "Заказ"}, {"supplier_name", "Поставщик"},
			{"expected_at", "Ожидается"}, {"total", "Сумма"}, {"status.label", "Статус"},
		}},
		"sales-orders": {handler: s.handleListSalesOrders, columns: []exportColumn{
			{"so_no", "Заказ"}, {"customer_name", "Клиент"},
			{"ordered_on", "Дата"}, {"total", "Сумма"}, {"status.label", "Статус"},
		}},
		"customers": {handler: s.handleListCustomers, columns: []exportColumn{
			{"name", "Клиент"}, {"customer_type", "Тип"}, {"region", "Регион"},
			{"contact", "Контакт"}, {"open_deals", "Сделок"}, {"open_amount", "Сумма"},
		}},
		"deals": {handler: s.handleListDeals, columns: []exportColumn{
			{"customer_name", "Клиент"}, {"region", "Регион"},
			{"stage.label", "Стадия"}, {"amount", "Сумма"},
			{"owner_name", "Менеджер"}, {"expected_close", "Закрытие"},
		}},
		"logistics": {handler: s.handleListShipments, columns: []exportColumn{
			{"trip_no", "Рейс"}, {"route_from", "Откуда"}, {"route_to", "Куда"},
			{"driver_name", "Водитель"}, {"vehicle_plate", "Транспорт"},
			{"transport_cost", "Стоимость"}, {"status.label", "Статус"},
		}},
		"hr": {handler: s.handleListEmployees, columns: []exportColumn{
			{"full_name", "ФИО"}, {"position_title", "Должность"},
			{"department", "Подразделение"}, {"shift", "Смена"},
			{"hired_on", "Принят"}, {"contract_until", "Договор до"},
			{"status.label", "Статус"},
		}},
		"equipment": {handler: s.handleListAssets, columns: []exportColumn{
			{"asset_no", "Инв. номер"}, {"name", "Наименование"},
			{"line", "Линия"}, {"next_due_on", "Следующее ТО"},
			{"warranty_until", "Гарантия до"}, {"status.label", "Статус"},
		}},
		"documents": {handler: s.handleListDocuments, columns: []exportColumn{
			{"doc_no", "Номер"}, {"title", "Название"}, {"doc_type", "Тип"},
			{"owner_name", "Ответственный"}, {"valid_until", "Действует до"},
			{"status.label", "Статус"},
		}},
		"inquiries": {handler: s.handleListInquiries, columns: []exportColumn{
			{"reference_no", "Номер"}, {"type.label", "Тип"}, {"name", "Имя"},
			{"company", "Компания"}, {"contact", "Контакт"},
			{"batch_no", "Партия"}, {"status.label", "Статус"},
		}},
		"audit": {handler: s.handleAuditLog, columns: []exportColumn{
			{"occurred_at", "Когда"}, {"actor_name", "Кто"},
			{"action", "Действие"}, {"resource", "Модуль"},
			{"resource_id", "Объект"}, {"ip", "IP"},
		}},
	}
}

// handleExport renders one collection as CSV.
//
// UTF-8 with a BOM: Excel on Windows reads a BOM-less UTF-8 CSV as the system
// codepage and renders every Cyrillic and Tajik character as mojibake. The BOM
// is three bytes that make the difference between a usable report and a support
// call.
//
// Semicolon-delimited for the same reason: a comma-delimited file opened in a
// Russian Windows locale lands entirely in column A.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	// The route is /export/<key>, declared statically per collection so the
	// guard is a real declaration rather than a runtime lookup.
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/export/")
	spec, ok := s.exports()[key]
	if !ok {
		common.Fail(w, r, common.NotFound())
		return
	}

	// Re-enter the module's own handler with an untouched request, so the export
	// inherits its filters, its sort and — critically — its permission check.
	rec := &captureWriter{header: http.Header{}}
	inner := r.Clone(r.Context())
	// Export the whole filtered set rather than one screen of it: a report that
	// silently stops at 25 rows is worse than no report.
	q := inner.URL.Query()
	q.Set("per_page", "1000")
	inner.URL.RawQuery = q.Encode()

	spec.handler(rec, inner)

	if rec.status != 0 && rec.status != http.StatusOK {
		// The inner handler refused — a 403 from RBAC, most importantly. Pass its
		// own response through rather than inventing one.
		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body)
		return
	}

	rows, err := decodeCollection(rec.body)
	if err != nil {
		common.Fail(w, r, fmt.Errorf("export: %s: %w", key, err))
		return
	}

	filename := fmt.Sprintf("samari-%s-%s.csv", key, time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
			filename, url.PathEscape(filename)))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM
	cw := csv.NewWriter(w)
	cw.Comma = ';'

	headers := make([]string, len(spec.columns))
	for i, c := range spec.columns {
		headers[i] = c.header
	}
	_ = cw.Write(headers)

	for _, row := range rows {
		line := make([]string, len(spec.columns))
		for i, c := range spec.columns {
			line[i] = lookupPath(row, c.path)
		}
		_ = cw.Write(line)
	}
	cw.Flush()
}

// lookupPath resolves a dotted path into a decoded JSON row, rendering the leaf
// as the plain string a spreadsheet expects. A null becomes an empty cell rather
// than the text "null".
func lookupPath(row map[string]any, path string) string {
	var cur any = row
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = m[part]
		if !ok || cur == nil {
			return ""
		}
	}
	switch v := cur.(type) {
	case string:
		return v
	case bool:
		if v {
			return "да"
		}
		return "нет"
	case float64:
		// Counts arrive as JSON numbers. Money and quantities are already strings
		// (CLAUDE.md §4.6) and never reach this branch.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// captureWriter buffers a handler's response so the exporter can reshape it.
//
// It deliberately implements only what a JSON handler uses. Anything that
// streams would need more, and nothing in the export set streams.
type captureWriter struct {
	header http.Header
	body   []byte
	status int
}

func (c *captureWriter) Header() http.Header { return c.header }

func (c *captureWriter) Write(b []byte) (int, error) {
	c.body = append(c.body, b...)
	return len(b), nil
}

func (c *captureWriter) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// decodeCollection unwraps the {data, meta} envelope every list handler produces.
func decodeCollection(body []byte) ([]map[string]any, error) {
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("response was not a data envelope: %w", err)
	}
	return env.Data, nil
}
