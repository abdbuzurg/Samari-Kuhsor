package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DemoResult reports what the demonstration seed created.
type DemoResult struct {
	Customers    int
	Contacts     int
	Leads        int
	Deals        int
	Tasks        int
	Suppliers    int
	Employees    int
	Assets       int
	Documents    int
	Batches      int
	Movements    int
	QualityTests int
	Orders       int
	Inquiries    int
	Shipments    int
}

// Demo loads demonstration data across every module.
//
// It exists because an empty system reads as a broken one to a client who has
// not been told it is a new one, and T36 puts this in front of QOIM before a
// single real record exists. Every module that has a screen gets rows.
//
// It is NOT idempotent in the way Reference is, and deliberately so: it refuses
// to run twice rather than quietly doubling every figure on the dashboard. The
// guard is the presence of demo customers, because that is the first thing it
// writes and the cheapest thing to check.
//
// The caller has already refused to run this with APP_ENV=production
// (cmd/seed/main.go:43). That guard is not a convention: demo batches carrying
// fabricated QC releases would sit in audit_log beside real ones, and
// no-hard-delete means tombstoning them afterwards does not remove them. A
// falsified regulatory record cannot be tidied up later.
func Demo(ctx context.Context, pool *pgxpool.Pool) (DemoResult, error) {
	var res DemoResult

	tx, err := pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("demo: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// The probe looks at what the demo itself writes and nothing else writes
	// during normal operation. Customers alone is not enough: an enquiry that
	// nobody converted leaves no customer behind, so a database carrying real
	// enquiries would still look empty here.
	var existing int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM customers WHERE deleted_at IS NULL)
		     + (SELECT count(*) FROM deals WHERE deleted_at IS NULL)
		     + (SELECT count(*) FROM suppliers WHERE deleted_at IS NULL)`).Scan(&existing); err != nil {
		return res, fmt.Errorf("demo: probe: %w", err)
	}
	if existing > 0 {
		return res, fmt.Errorf("demo data is already present (%d customers, deals and suppliers); "+
			"reset the database rather than seeding twice", existing)
	}

	ref, err := loadReferenceIDs(ctx, tx)
	if err != nil {
		return res, err
	}

	if err := demoCRM(ctx, tx, ref, &res); err != nil {
		return res, err
	}
	if err := demoRegistries(ctx, tx, ref, &res); err != nil {
		return res, err
	}
	if err := demoOperations(ctx, tx, ref, &res); err != nil {
		return res, err
	}
	if err := demoInquiries(ctx, tx, ref, &res); err != nil {
		return res, err
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("demo: commit: %w", err)
	}
	return res, nil
}

// referenceIDs are the rows the reference seed already created. The demo seed
// attaches to them rather than inventing a second catalogue: the five products
// are the real ones (D8) and demo data must not add a sixth.
type referenceIDs struct {
	admin      uuid.UUID
	items      map[string]uuid.UUID // sku → id
	locations  map[string]uuid.UUID // code → id
	quarantine uuid.UUID
	finished   uuid.UUID
	raw        uuid.UUID
}

func loadReferenceIDs(ctx context.Context, tx pgx.Tx) (referenceIDs, error) {
	ref := referenceIDs{
		items:     map[string]uuid.UUID{},
		locations: map[string]uuid.UUID{},
	}

	if err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE deleted_at IS NULL ORDER BY created_at LIMIT 1`,
	).Scan(&ref.admin); err != nil {
		return ref, fmt.Errorf("demo: no user exists — run `seed reference` first: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT sku, id FROM items WHERE deleted_at IS NULL`)
	if err != nil {
		return ref, fmt.Errorf("demo: items: %w", err)
	}
	for rows.Next() {
		var sku string
		var id uuid.UUID
		if err := rows.Scan(&sku, &id); err != nil {
			rows.Close()
			return ref, err
		}
		ref.items[sku] = id
	}
	rows.Close()
	if len(ref.items) == 0 {
		return ref, fmt.Errorf("demo: no items — run `seed reference` first")
	}

	locs, err := tx.Query(ctx, `SELECT code, id, zone FROM locations WHERE deleted_at IS NULL`)
	if err != nil {
		return ref, fmt.Errorf("demo: locations: %w", err)
	}
	for locs.Next() {
		var code, zone string
		var id uuid.UUID
		if err := locs.Scan(&code, &id, &zone); err != nil {
			locs.Close()
			return ref, err
		}
		ref.locations[code] = id
		switch zone {
		case "quarantine":
			ref.quarantine = id
		case "finished_goods":
			ref.finished = id
		case "raw":
			ref.raw = id
		}
	}
	locs.Close()
	if ref.quarantine == uuid.Nil || ref.finished == uuid.Nil {
		return ref, fmt.Errorf("demo: warehouse zones missing — run `seed reference` first")
	}
	return ref, nil
}

// ---------------------------------------------------------------------------
// CRM и продажи
// ---------------------------------------------------------------------------

// demoCustomers span the four real regions named in docs/05-MODULES.md:179.
// Invented company names, real places — a demo that shows "Регион 1" teaches
// nobody what the field is for.
var demoCustomers = []struct {
	name, kind, region, contact string
}{
	{"ООО «Ориён Савдо»", "distributor", "Душанбе", "+992 900 100 200"},
	{"Сеть «Пайкар»", "retail", "Худжанд", "+992 927 310 455"},
	{"Магазин «Лаъли Бадахшон»", "retail", "Хорог", "+992 935 118 700"},
	{"ООО «Вахш Трейд»", "wholesale", "Бохтар", "+992 918 442 019"},
}

func demoCRM(ctx context.Context, tx pgx.Tx, ref referenceIDs, res *DemoResult) error {
	customerIDs := make([]uuid.UUID, 0, len(demoCustomers))

	for _, c := range demoCustomers {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO customers (name, customer_type, region, contact, created_by)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			c.name, c.kind, c.region, c.contact, ref.admin).Scan(&id); err != nil {
			return fmt.Errorf("demo: customer %s: %w", c.name, err)
		}
		customerIDs = append(customerIDs, id)
		res.Customers++

		if _, err := tx.Exec(ctx, `
			INSERT INTO contacts (customer_id, full_name, role, phone, created_by)
			VALUES ($1, $2, $3, $4, $5)`,
			id, "Контактное лицо — "+c.region, "Менеджер", c.contact, ref.admin); err != nil {
			return fmt.Errorf("demo: contact: %w", err)
		}
		res.Contacts++
	}

	// Deals across all five stages, so the dashboard's Воронка продаж renders a
	// funnel rather than the empty state it has always shown.
	stages := []struct {
		stage  string
		amount string
	}{
		{"new", "12000.00"},
		{"negotiation", "48000.00"},
		{"quoted", "31500.00"},
		{"won", "27000.00"},
		{"lost", "9000.00"},
	}
	for i, s := range stages {
		customer := customerIDs[i%len(customerIDs)]
		var dealID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO deals (customer_id, amount, stage, owner_id, expected_close, created_by)
			VALUES ($1, $2, $3, $4, CURRENT_DATE + ($5 * INTERVAL '1 day'), $6)
			RETURNING id`,
			customer, s.amount, s.stage, ref.admin, (i+1)*10, ref.admin).Scan(&dealID); err != nil {
			return fmt.Errorf("demo: deal %s: %w", s.stage, err)
		}
		res.Deals++

		// The stage chain, so a deal's history reads as a progression rather than
		// appearing fully formed at whatever stage it happens to sit in.
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stage_events (deal_id, from_stage, to_stage, changed_by, note)
			VALUES ($1, NULL, 'new', $2, 'Создана из обращения с сайта')`,
			dealID, ref.admin); err != nil {
			return fmt.Errorf("demo: deal event: %w", err)
		}
		if s.stage != "new" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO deal_stage_events (deal_id, from_stage, to_stage, changed_by, note)
				VALUES ($1, 'new', $2, $3, NULL)`,
				dealID, s.stage, ref.admin); err != nil {
				return fmt.Errorf("demo: deal event: %w", err)
			}
		}
	}

	for i, c := range customerIDs[:2] {
		if _, err := tx.Exec(ctx, `
			INSERT INTO leads (customer_id, source, status, created_by)
			VALUES ($1, $2, $3, $4)`,
			c, "Сайт", []string{"new", "negotiation"}[i], ref.admin); err != nil {
			return fmt.Errorf("demo: lead: %w", err)
		}
		res.Leads++
	}

	// One overdue task, because "Просроченные задачи" showing 0 on a demo tells
	// the viewer nothing about what the KPI means.
	tasks := []struct {
		title  string
		offset int
	}{
		{"Перезвонить в «Ориён Савдо»", -3},
		{"Подготовить КП для «Пайкар»", 2},
		{"Согласовать отгрузку в Хорог", 5},
	}
	for _, t := range tasks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tasks (title, assignee_id, due_on, status, created_by)
			VALUES ($1, $2, CURRENT_DATE + ($3 * INTERVAL '1 day'), 'open', $4)`,
			t.title, ref.admin, t.offset, ref.admin); err != nil {
			return fmt.Errorf("demo: task: %w", err)
		}
		res.Tasks++
	}

	return nil
}

// ---------------------------------------------------------------------------
// Поставщики, персонал, оборудование, документы
// ---------------------------------------------------------------------------

func demoRegistries(ctx context.Context, tx pgx.Tx, ref referenceIDs, res *DemoResult) error {
	suppliers := []struct{ name, region, contact string }{
		{"«Памир Агро»", "Хорог", "+992 935 200 100"},
		{"«Стекло Тоҷик»", "Душанбе", "+992 900 555 010"},
		{"«ПЭТ Пак»", "Худжанд", "+992 927 700 330"},
	}
	for _, s := range suppliers {
		if _, err := tx.Exec(ctx, `
			INSERT INTO suppliers (name, region, contact, created_by)
			VALUES ($1, $2, $3, $4)`, s.name, s.region, s.contact, ref.admin); err != nil {
			return fmt.Errorf("demo: supplier %s: %w", s.name, err)
		}
		res.Suppliers++
	}

	// Contract expiry is the question the Персонал register exists to answer, so
	// one contract expires inside the 30-day warning window and one does not.
	employees := []struct {
		name, shift string
		contractIn  int
	}{
		{"А. Раҳимов", "day", 400},
		{"М. Назарова", "day", 21},
		{"Н. Шоев", "rotating", 200},
		{"С. Одинаев", "night", 640},
		{"З. Мирзоева", "day", 95},
	}
	for _, e := range employees {
		if _, err := tx.Exec(ctx, `
			INSERT INTO employees (full_name, shift, hired_on, contract_until, status, created_by)
			VALUES ($1, $2, CURRENT_DATE - INTERVAL '60 days',
			        CURRENT_DATE + ($3 * INTERVAL '1 day'), 'active', $4)`,
			e.name, e.shift, e.contractIn, ref.admin); err != nil {
			return fmt.Errorf("demo: employee %s: %w", e.name, err)
		}
		res.Employees++
	}

	assets := []struct {
		no, name, kind, line, status string
		dueIn                        int
	}{
		{"EQ-001", "Линия розлива сока", "filling", "Линия 1", "running", 90},
		{"EQ-002", "Пастеризатор", "thermal", "Линия 1", "maintenance_due", 3},
		{"EQ-003", "Линия розлива воды", "filling", "Линия 2", "running", 150},
		{"EQ-004", "Этикетировочная машина", "labelling", "Линия 2", "broken", 0},
	}
	for _, a := range assets {
		var assetID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO assets (asset_no, name, asset_type, line, commissioned_on,
			                    warranty_until, status, created_by)
			VALUES ($1, $2, $3, $4, CURRENT_DATE - INTERVAL '90 days',
			        CURRENT_DATE + INTERVAL '2 years', $5, $6) RETURNING id`,
			a.no, a.name, a.kind, a.line, a.status, ref.admin).Scan(&assetID); err != nil {
			return fmt.Errorf("demo: asset %s: %w", a.no, err)
		}
		res.Assets++

		if a.dueIn > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO maintenance_events (asset_id, event_type, performed_at,
				                                next_due_on, notes, created_by)
				VALUES ($1, 'planned', now() - INTERVAL '30 days',
				        CURRENT_DATE + ($2 * INTERVAL '1 day'), 'Плановое обслуживание', $3)`,
				assetID, a.dueIn, ref.admin); err != nil {
				return fmt.Errorf("demo: maintenance: %w", err)
			}
		}
	}

	// One expiring certificate, because an expiring certificate is a regulatory
	// exposure and the register is ordered by exactly that.
	documents := []struct {
		no, title, kind, status string
		validIn                 int
	}{
		{"DOC-001", "Сертификат ISO 22000", "certificate", "active", 25},
		{"DOC-002", "Санитарный регламент линии розлива", "sop", "active", 700},
		{"DOC-003", "Договор поставки — «Памир Агро»", "contract", "approval", 365},
		{"DOC-004", "Руководство по эксплуатации пастеризатора", "manual", "draft", 0},
	}
	for _, d := range documents {
		valid := "NULL"
		args := []any{d.no, d.title, d.kind, d.status, ref.admin}
		if d.validIn > 0 {
			valid = "CURRENT_DATE + ($6 * INTERVAL '1 day')"
			args = append(args, d.validIn)
		}
		q := fmt.Sprintf(`
			INSERT INTO documents (doc_no, title, doc_type, status, owner_id, valid_until)
			VALUES ($1, $2, $3, $4, $5, %s)`, valid)
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("demo: document %s: %w", d.no, err)
		}
		res.Documents++
	}

	return nil
}

// ---------------------------------------------------------------------------
// The operating chain: batches, stock, production, quality, orders, trips
// ---------------------------------------------------------------------------

func demoOperations(ctx context.Context, tx pgx.Tx, ref referenceIDs, res *DemoResult) error {
	apple, ok := ref.items["APJ-1000"]
	if !ok {
		return fmt.Errorf("demo: APJ-1000 missing from the catalogue")
	}
	water := ref.items["WAT-500"]
	jam := ref.items["APR-220"]

	// Three batches in three different states, so Качество shows a decision
	// waiting, a decision taken, and one still on the line.
	batches := []struct {
		no     string
		item   uuid.UUID
		status string
		qty    string
		loc    uuid.UUID
	}{
		{"B-2617", apple, "released", "4800.000", ref.finished},
		{"B-2618", water, "quarantine", "9600.000", ref.quarantine},
		{"B-2619", jam, "in_production", "0.000", uuid.Nil},
	}

	batchIDs := map[string]uuid.UUID{}
	for _, b := range batches {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO batches (batch_no, item_id, produced_on, expires_on, status, created_by)
			VALUES ($1, $2, CURRENT_DATE - INTERVAL '5 days',
			        CURRENT_DATE + INTERVAL '300 days', $3, $4) RETURNING id`,
			b.no, b.item, b.status, ref.admin).Scan(&id); err != nil {
			return fmt.Errorf("demo: batch %s: %w", b.no, err)
		}
		batchIDs[b.no] = id
		res.Batches++

		// The decision chain behind the status, so the traceability view has a
		// history rather than a batch that simply appeared released.
		if _, err := tx.Exec(ctx, `
			INSERT INTO batch_status_events (batch_id, from_status, to_status, decided_by, reason)
			VALUES ($1, NULL, 'in_production', $2, NULL)`, id, ref.admin); err != nil {
			return fmt.Errorf("demo: batch event: %w", err)
		}
		if b.status != "in_production" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO batch_status_events (batch_id, from_status, to_status, decided_by, reason)
				VALUES ($1, 'in_production', 'quarantine', $2, NULL)`, id, ref.admin); err != nil {
				return fmt.Errorf("demo: batch event: %w", err)
			}
		}
		if b.status == "released" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO batch_status_events (batch_id, from_status, to_status, decided_by, reason)
				VALUES ($1, 'quarantine', 'released', $2, 'Все проверки пройдены')`,
				id, ref.admin); err != nil {
				return fmt.Errorf("demo: batch event: %w", err)
			}
		}

		// Stock is posted as MOVEMENTS, never as a balance. There is no column to
		// set: the register sums these rows at read time (CLAUDE.md §4.2).
		if b.loc != uuid.Nil {
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_movements (item_id, batch_id, location_id, qty_delta,
				                             reason, note, created_by)
				VALUES ($1, $2, $3, $4, 'production_output', 'Выпуск партии', $5)`,
				b.item, id, b.loc, b.qty, ref.admin); err != nil {
				return fmt.Errorf("demo: movement: %w", err)
			}
			res.Movements++
		}
	}

	// Raw material on hand, so Склад is not only finished goods and so a low
	// stock alert has something to fire against.
	if ref.raw != uuid.Nil {
		for sku, qty := range map[string]string{"APJ-1000": "150.000", "WAT-500": "80.000"} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_movements (item_id, location_id, qty_delta, reason, note, created_by)
				VALUES ($1, $2, $3, 'goods_receipt', 'Приёмка от поставщика', $4)`,
				ref.items[sku], ref.raw, qty, ref.admin); err != nil {
				return fmt.Errorf("demo: raw movement: %w", err)
			}
			res.Movements++
		}
	}

	// Lab results, including one failure — a Качество screen where everything
	// passed does not show what the module is for.
	tests := []struct {
		batch, kind, value string
		passed             bool
	}{
		{"B-2617", "ph", "3.6", true},
		{"B-2617", "microbiology", "<10 КОЕ/г", true},
		{"B-2617", "organoleptic", "соответствует", true},
		{"B-2618", "ph", "7.2", true},
		{"B-2618", "microbiology", "обнаружено", false},
	}
	for _, t := range tests {
		if _, err := tx.Exec(ctx, `
			INSERT INTO quality_tests (batch_id, test_type, result_value, passed,
			                           tested_at, inspector_id, created_by)
			VALUES ($1, $2, $3, $4, now() - INTERVAL '2 days', $5, $5)`,
			batchIDs[t.batch], t.kind, t.value, t.passed, ref.admin); err != nil {
			return fmt.Errorf("demo: quality test: %w", err)
		}
		res.QualityTests++
	}

	// A manufacturing order matched 1:1 to the in-production batch.
	var moID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO manufacturing_orders (mo_no, item_id, batch_id, line, scheduled_for,
		                                  planned_qty, status, created_by)
		VALUES ('MO-1041', $1, $2, 'Линия 1', CURRENT_DATE, '6000.000', 'in_progress', $3)
		RETURNING id`, jam, batchIDs["B-2619"], ref.admin).Scan(&moID); err != nil {
		return fmt.Errorf("demo: manufacturing order: %w", err)
	}
	for i, e := range []struct {
		good, scrap string
		downtime    int
	}{{"1800.000", "60.000", 15}, {"2100.000", "40.000", 0}} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO production_entries (mo_id, good_qty, scrap_qty, downtime_min,
			                                note, recorded_at, recorded_by, created_by)
			VALUES ($1, $2, $3, $4, $5, now() - ($6 * INTERVAL '1 hour'), $7, $7)`,
			moID, e.good, e.scrap, e.downtime,
			fmt.Sprintf("Смена %d", i+1), 8-i*4, ref.admin); err != nil {
			return fmt.Errorf("demo: production entry: %w", err)
		}
	}

	// Purchase orders across the approval ladder.
	var supplierID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM suppliers WHERE deleted_at IS NULL ORDER BY name LIMIT 1`,
	).Scan(&supplierID); err != nil {
		return fmt.Errorf("demo: supplier lookup: %w", err)
	}
	for i, po := range []struct{ no, status string }{
		{"PO-0031", "approval"},
		{"PO-0032", "confirmed"},
		{"PO-0033", "closed"},
	} {
		var poID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO purchase_orders (po_no, supplier_id, expected_at, status, created_by)
			VALUES ($1, $2, CURRENT_DATE + ($3 * INTERVAL '1 day'), $4, $5) RETURNING id`,
			po.no, supplierID, (i+1)*7, po.status, ref.admin).Scan(&poID); err != nil {
			return fmt.Errorf("demo: purchase order %s: %w", po.no, err)
		}
		res.Orders++
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_order_lines (po_id, item_id, qty, unit_price, created_by)
			VALUES ($1, $2, '1000.000', '15.00', $3)`, poID, apple, ref.admin); err != nil {
			return fmt.Errorf("demo: po line: %w", err)
		}
	}

	// A sales order and a trip carrying the released batch, so the chain from
	// production through release to a loaded lorry is visible end to end.
	var customerID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM customers WHERE deleted_at IS NULL ORDER BY name LIMIT 1`,
	).Scan(&customerID); err != nil {
		return fmt.Errorf("demo: customer lookup: %w", err)
	}
	for i, so := range []struct{ no, status string }{
		{"SO-0101", "draft"},
		{"SO-0102", "confirmed"},
	} {
		var soID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO sales_orders (so_no, customer_id, ordered_on, status, created_by)
			VALUES ($1, $2, CURRENT_DATE - ($3 * INTERVAL '1 day'), $4, $5) RETURNING id`,
			so.no, customerID, i+1, so.status, ref.admin).Scan(&soID); err != nil {
			return fmt.Errorf("demo: sales order %s: %w", so.no, err)
		}
		res.Orders++
		if _, err := tx.Exec(ctx, `
			INSERT INTO sales_order_lines (sales_order_id, item_id, batch_id, qty,
			                               unit_price, created_by)
			VALUES ($1, $2, $3, '480.000', '18.50', $4)`,
			soID, apple, batchIDs["B-2617"], ref.admin); err != nil {
			return fmt.Errorf("demo: so line: %w", err)
		}
	}

	var vehicleID, driverID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO vehicles (plate, model, capacity, created_by)
		VALUES ('01 AA 123 AA', 'ГАЗель Next', '1500.000', $1) RETURNING id`,
		ref.admin).Scan(&vehicleID); err != nil {
		return fmt.Errorf("demo: vehicle: %w", err)
	}
	// A driver IS an employee — `drivers.employee_id` is NOT NULL. Modelling them
	// as a separate person with their own name would put the same human in the
	// system twice and break the HR register's answer to "who works here".
	var driverEmployee uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM employees WHERE full_name = 'Н. Шоев' AND deleted_at IS NULL`,
	).Scan(&driverEmployee); err != nil {
		return fmt.Errorf("demo: driver employee lookup: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO drivers (employee_id, licence_no, created_by)
		VALUES ($1, 'AB 4471982', $2) RETURNING id`,
		driverEmployee, ref.admin).Scan(&driverID); err != nil {
		return fmt.Errorf("demo: driver: %w", err)
	}

	for i, trip := range []struct{ no, from, to, status, cost string }{
		{"TR-0077", "Хорог", "Душанбе", "in_transit", "1200.00"},
		{"TR-0078", "Хорог", "Худжанд", "planned", "1850.00"},
	} {
		var shipID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO shipments (trip_no, route_from, route_to, driver_id, vehicle_id,
			                       transport_cost, status, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			trip.no, trip.from, trip.to, driverID, vehicleID,
			trip.cost, trip.status, ref.admin).Scan(&shipID); err != nil {
			return fmt.Errorf("demo: shipment %s: %w", trip.no, err)
		}
		res.Shipments++

		// Only the released batch may be loaded — the same rule Go enforces.
		if i == 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO shipment_lines (shipment_id, item_id, batch_id, qty, created_by)
				VALUES ($1, $2, $3, '480.000', $4)`,
				shipID, apple, batchIDs["B-2617"], ref.admin); err != nil {
				return fmt.Errorf("demo: shipment line: %w", err)
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Обращения с сайта
// ---------------------------------------------------------------------------

func demoInquiries(ctx context.Context, tx pgx.Tx, ref referenceIDs, res *DemoResult) error {
	var releasedBatch uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM batches WHERE batch_no = 'B-2617' AND deleted_at IS NULL`,
	).Scan(&releasedBatch); err != nil {
		return fmt.Errorf("demo: batch lookup: %w", err)
	}

	// All five types, because the per-type reference prefix is a client-facing
	// feature and a demo showing one type does not demonstrate it.
	//
	// Reference numbers come from the SAME per-type sequences the public
	// submission path uses (migration 00006), never hard-coded. An earlier draft
	// wrote 'WR-0001' and friends directly and collided the first time it met a
	// database that had already received a real submission — which a fresh test
	// database can never reproduce. Drawing from the sequence also means a demo
	// enquiry is indistinguishable in shape from a real one.
	inquiries := []struct {
		kind, prefix, name, company, contact, message, status string
		withBatch                                             bool
	}{
		{"wholesale", "WR-", "Ф. Юсупов", "ООО «Ориён Савдо»", "+992 900 100 200",
			"Интересуют оптовые поставки сока в Душанбе.", "lead_created", false},
		{"contact", "CF-", "Г. Сафарова", "", "+992 918 220 415",
			"Где можно купить вашу продукцию в Худжанде?", "new", false},
		{"distributor", "DA-", "Р. Холов", "«Вахш Трейд»", "+992 918 442 019",
			"Заявка на дистрибуцию по Хатлонской области.", "new", false},
		{"complaint", "CP-", "А. Каримов", "", "+992 900 111 222",
			"Посторонний привкус в соке, партия указана на упаковке.", "new", true},
		{"job", "JB-", "З. Мирзоева", "", "+992 935 660 118",
			"Резюме на позицию лаборанта.", "closed", false},
	}

	for _, i := range inquiries {
		var reference string
		if err := tx.QueryRow(ctx,
			`SELECT next_inquiry_reference($1)`, i.prefix).Scan(&reference); err != nil {
			return fmt.Errorf("demo: reference for %s: %w", i.kind, err)
		}

		var batch any
		if i.withBatch {
			batch = releasedBatch
		}
		var company any
		if i.company != "" {
			company = i.company
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO inquiries (inquiry_type, reference_no, name, company, contact,
			                       message, batch_id, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now() - INTERVAL '1 day')`,
			i.kind, reference, i.name, company, i.contact, i.message, batch, i.status); err != nil {
			return fmt.Errorf("demo: inquiry %s: %w", reference, err)
		}
		res.Inquiries++
	}

	_ = ref
	return nil
}
