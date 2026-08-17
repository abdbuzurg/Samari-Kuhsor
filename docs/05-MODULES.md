# 05 — Module Specifications

One section per module. Column sets, KPIs and status ladders come from the **approved prototype**
(`design/Samari-Kuhsor-Green-CRM.html`) — reproduce them, do not redesign them.

Every module gets: a list view, a detail view, an edit form. The list view is already designed; the
**detail view is new work** and is the largest remaining design task in the project. §2 defines one
pattern that all modules reuse.

---

## 1. Shared shell

**Sidebar**, 252px, brand block "СК / Самари Кӯҳсор / CRM-ERP платформа", then nav in four groups,
then a status footer. Count pills show pending work per module.

| Group | Modules |
|---|---|
| ОБЗОР | Панель управления |
| ПРОДАЖИ | CRM и продажи · Интеграция с сайтом · Товары и цены |
| ОПЕРАЦИИ | Склад и запасы · Закупки и поставщики · Производство · Качество и безопасность · Логистика |
| УПРАВЛЕНИЕ | Финансы и бюджет *(hidden at launch)* · Персонал · Оборудование и ТО · Документы |

Nav entries render only where the user has `read`. A user with no permission on a module never sees
it.

**Top bar**, 64px: global search · ТҶ/РУ/EN switcher · notifications with unread badge · settings ·
user chip with dropdown. Sidebar brand block and top bar are both exactly 64px so the divider is
continuous.

---

## 2. The shared detail-view pattern

Build once, reuse for every module. Get this right in the Товары slice.

```
┌ Breadcrumb: Модуль / Запись
├ Header:  title · key identifier · status tag · [Редактировать] [action buttons]
├ Field groups: 2-column definition list, grouped by theme
├ Related records: tabbed or stacked tables (batches, movements, tests, lines…)
├ Activity: audit_log entries for this record, newest first
└ Footer: created/updated by and when, version
```

Rules:

- Action buttons appear only when the user holds the permission (`manage`, or `approve` for
  transitions). Hiding is cosmetic; the server still enforces.
- State transitions are buttons that call the sub-resource endpoint, never a status dropdown in the
  edit form.
- The activity panel reads `audit_log` filtered to this resource and id.
- Edit is a separate route (`/items/{id}/edit`) or a full-height drawer — not the generic modal
  from the prototype, which was a placeholder.

---

## 3. Панель управления — `dashboard`

Read-only aggregate. Build **last**, once the modules feeding it exist.

**KPI cards (6):** Выручка · Заказы в работе · Дебиторка · Стоимость запасов · Партии на карантине ·
Просроченные поставки.

> **Finance dependency.** Выручка and Дебиторка, and the revenue series in the Выручка и заказы
> panel, all depend on the deferred finance module. At launch: hide the Дебиторка card entirely
> (there is no receivables data without finance), and source Выручка and the chart's revenue series
> from **confirmed `sales_orders` only**. If sales orders are empty on opening day — which they will
> be — render the empty state. Do not fabricate figures, and do not carry the prototype's sample
> numbers into production.

**Panels:** Выручка и заказы (dual bar chart, period switch день/неделя/месяц/квартал) ·
Воронка продаж · Запасы: остатки и сроки · Производство сегодня (plan-vs-actual bar) ·
Лента событий · Последние заказы.

The period switch re-plots the chart and the revenue KPI only.

---

## 4. Товары и цены — `items` · **first vertical slice**

The smallest module and the one everything else references. Its slice becomes the template.

**KPIs:** Всего SKU · Активных · Языки каталога · Средняя цена
**Columns:** SKU · Наименование · Категория · Упаковка · Цена · Срок годн. · Статус
**Statuses:** Черновик `neutral` → Активен `ok` → Архив `neutral`

**Detail sections:** identity (SKU, type, category, base UOM, `min_qty`) · translations ru/tg/en ·
packaging units and barcodes · price history · shelf life and storage · related batches ·
activity. Website publication is not a separate field — a finished good is on the public site when
`status = active`.

Seed with exactly the five approved products (`01-DECISIONS.md` D8). Composition, nutrition and
shelf-life fields stay null and render **`уточняется`** until the client's lab data arrives.

**QR generation** writes to `batches.qr_payload`, so it needs the `batches` table. Per D11 it must
work before 9 September — the wrapper printer needs the codes in advance. The `batches` migration is
therefore pulled forward into Phase 0 foundations, and QR generation and export are built as a
tail-end task of this slice. See `06-BUILD-PLAN.md`.

---

## 5. Склад и запасы — `inventory`

**KPIs:** Позиций на складе · Низкий остаток · Скоро истекает · Стоимость запасов
**Columns:** Артикул · Товар · Партия · Остаток · Срок годн. · Локация · Статус
**Statuses:** В норме `ok` · Истекает N дн `warn` · Низкий остаток `danger` · Ниже мин. `danger`

Every quantity shown is **derived from `stock_movements`** — see `02-SCHEMA.md` §5. There is no
balance column and the UI must never offer "set stock to X". Corrections are adjustment movements.

**Detail:** an item/batch/location position showing the full movement ledger, running balance,
expiry, and links to the source documents (goods receipt, manufacturing order, shipment).

Actions: приёмка, перемещение, списание, корректировка — each posting a signed movement.

---

## 6. Производство — `production`

**KPIs:** План на смену · Факт · Выход % · Простой
**Columns:** Заказ · Продукт · Партия · План · Факт · Брак · Статус
**Statuses:** Запланирован `neutral` → В работе `info` → На контроле QC `warn` → Готово `ok`

Manufacturing orders map 1:1 to batches (`MO-0612` → `B-2617`). Actual output, yield and downtime
are sums over `production_entries`, never stored columns.

Completing an order posts a `production_output` stock movement into a **quarantine** location and
moves the batch to `quarantine`. It does **not** make the batch sellable — only Качество can.

**Detail:** order header · batch · shift entries (append-only log of good/scrap/downtime) ·
material consumption · linked QC tests · activity.

---

## 7. Качество и безопасность — `quality`

The regulatory heart of the system. Highest test coverage in the codebase.

**KPIs:** Проверок · На карантине · Несоответствия · Выпущено
**Columns:** Партия · Продукт · Тест · Результат · Инспектор · Статус
**Statuses:** Карантин `danger` → Выпущено `ok` / Отклонено `danger`
**Tests:** pH · микробиология · Brix/вязкость · металлодетекция · органолептика

Release and rejection require `quality:approve` and write an immutable `batch_status_events` row
plus an audit entry naming the deciding user. Only `released` batches may be sold or shipped —
enforce in the sales and logistics domains, not just the UI.

**Detail:** batch header · all tests with results and inspectors · status history with who decided
and why · linked manufacturing order · linked stock positions · activity.

---

## 8. Интеграция с сайтом — `inquiries`

**KPIs:** Заявки сегодня · Оптовые запросы · Жалобы · Отклики (вакансии)
**Columns:** № · Тип обращения · Имя/компания · Контакт · Дата · Статус
**Statuses:** Новое `info` → Создан лид `ok` → Закрыто `neutral`

Written by the public website through `POST /public/inquiries`. Reference prefixes: `WR-` оптовый
запрос · `CF-` контакт · `DA-` дистрибьютор · `CP-` жалоба · `JB-` вакансия. Every submission
returns its reference number to the visitor — a ToR requirement.

Action: convert to a lead in `crm`, carrying the reference number across.

Complaints (`CP-`) must link to a batch so the ToR's complaint→traceability workflow is possible.

---

## 9. CRM и продажи — `crm`

**KPIs:** Новые лиды · Открытые сделки · Конверсия · Просроченные задачи
**Columns:** Клиент · Тип · Регион · Статус · Сумма · Менеджер
**Pipeline:** Новый лид `info` → Переговоры `info` → КП отправлено `warn` → Выиграно `ok` /
Проиграно `neutral`

Regions in the seed data are real: Душанбе, Худжанд, Хорог, Бохтар.

**Detail:** customer header · contacts · deals with stage history · linked inquiries · orders ·
activity.

> On 9 September the plant has produced nothing and has no customers. This module will be empty at
> launch. Build it, seed it lightly, do not invest detail-view effort here at the expense of the
> operational modules.

---

## 10. Закупки и поставщики — `procurement`

**KPIs:** Заявки на закупку *(count of POs in `draft` + `approval`)* · Открытые PO ·
Ожидают приёмки · Поставщики
**Columns:** PO № · Поставщик · Позиции · Сумма · Ожидается · Статус
**Statuses:** Черновик `neutral` → Согласование `warn` → Подтверждён `info` → В пути `info` →
Приёмка `warn` → Закрыт `ok`

Moving out of Согласование requires `procurement:approve`. Goods receipt posts `goods_receipt`
stock movements — this is how raw material enters inventory.

**Detail:** PO header · supplier · line items · receipt history · linked stock movements · activity.

---

## 11. Логистика — `logistics`

**KPIs:** Рейсов · В пути · Задержки · Транспортные расходы
**Columns:** Рейс · Маршрут · Водитель · Машина · Груз · Статус
**Statuses:** Запланирован `neutral` → Загрузка `info` → В пути `info` → Доставлен `ok` ·
Задержка `danger`

Real routes: Душанбе → Худжанд / Бохтар / Куляб, Худжанд → Хорог. Tajik plate formats.

**Loading a shipment line must reject any batch not in `released` status.** Enforce server-side.

---

## 12. Персонал — `hr`

**KPIs:** Сотрудников · На смене · В отпуске · Истекают договоры
**Columns:** ФИО · Отдел · Должность · Смена · Статус · Договор до
**Shifts:** Дневная · Ночная · Сменная

Contract expiry drives a warning at 30 days. Contains personal data — visible only with `hr:read`,
and it must not be exposed through any public endpoint. No payroll at launch.

---

## 13. Оборудование и ТО — `equipment`

**KPIs:** Активов · В работе · ТО скоро · Поломки
**Columns:** Актив · Тип · Линия · След. ТО · Гарантия до · Статус
**Statuses:** В работе `ok` · ТО скоро `warn` · Поломка `danger` · Выведен `neutral`

Assets `EQ-0xx`: filling lines, pasteuriser, labeller, cold room. Detail carries the maintenance
history and links to production lines.

---

## 14. Документы — `documents`

**KPIs:** Документов · Действующих · Истекают · На согласовании
**Columns:** Документ · Тип · Владелец · Версия · Действует до · Статус
**Statuses:** Черновик `neutral` → Согласование `warn` → Действует `ok` → Истекает `warn` →
Истёк `danger` → Архив `neutral`

Versioned controlled documents: ISO 22000 certificate, sanitation SOPs, supplier contracts,
sanitary permits, operator manuals. Superseded versions are retained, never deleted — this is the
archive-rather-than-delete rule from the ToR.

---

## 15. CMS — `cms`

Not in the prototype. New build, and the reason the website is a Next.js application.

**Screens:** page list with workflow status · block editor per page with ru/tg/en tabs ·
news post editor · media library with multilingual alt text · preview · publish.

**Workflow:** Черновик → Техническая проверка → Языковая проверка → Утверждено → Опубликовано.
Moving to Утверждено or Опубликовано requires `cms:approve`. Every transition writes
`content_workflow_events` and an audit entry.

The public site renders only `published`. The CRM can preview any state.

Editable surfaces on the website: hero, trust strip, eco section, about, production steps, news,
CTA, contact details, and the product copy that comes from `items` translations.

---

## 16. Финансы и бюджет — `finance` · deferred

Hidden from navigation at launch. Schema stubs may exist. See `01-DECISIONS.md` D2 and register
question Q2 — until the client answers whether statutory Tajik books are required, this module's
scope is unbounded and nothing should be built.

---

## 17. Notifications

The prototype's notification feed is static sample data. Real triggers for launch:

| Trigger | Level |
|---|---|
| New website inquiry | `info` |
| Batch entered quarantine | `warn` |
| Batch rejected | `danger` |
| Stock below minimum | `danger` |
| Batch expiring within 30 days | `warn` |
| Purchase order awaiting approval | `warn` |
| Delivery overdue | `danger` |
| Document expiring within 30 days | `warn` |
| Employment contract expiring within 30 days | `warn` |
| Equipment maintenance due | `warn` |

Each notification links to the record. Users see only notifications for resources they can `read`.
