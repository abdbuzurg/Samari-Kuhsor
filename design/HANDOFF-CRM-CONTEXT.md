# Samari Kuhsor — CRM/ERP Prototype · Engineering Handoff

Context document for Claude Cowork / any developer picking up `Samari-Kuhsor-Green.html`.
Everything below describes what exists **in the file today**, and what the source specification
requires that has **not** been built yet.

---

## 1. What this artifact is

| | |
|---|---|
| **File** | `review/Samari-Kuhsor-Green.html` |
| **Type** | Single self-contained HTML file. No build step, no bundler, no dependencies to install. |
| **Runs by** | Double-clicking the file, or serving the folder statically. |
| **Network needs** | Two CDN requests only: Google Fonts (Archivo) and `unpkg.com/lucide@latest` for icons. Everything else is inlined. Offline, the layout is intact but icons and the Archivo face fall back. |
| **Status** | **Design prototype.** Presentation layer only — no backend, no persistence, no auth. All data is hard-coded in JS arrays. |
| **Client decision** | Direction "Modernist" was chosen, then recoloured to a green palette per client feedback ("надо будет использовать другую палитру цветов, желательно зеленые оттенки"). |
| **Interface language** | Russian. A ТҶ / РУ / EN switcher is wired for all interface chrome. |

### Its purpose
This is the **visual and interaction contract** for the real build. It answers "what does the
system look like, what screens exist, how does a user move through it". It is intentionally not
an application: treat the markup, tokens, layout and status semantics as the spec, and replace
the data layer with real APIs.

---

## 2. Source specification traceability

Built from *Brief Technical Specification — Integrated CRM/ERP System for QOIM and the Samari
Kuhsor Brand*.

- **Legal entity:** QOIM · **Brand:** Samari Kuhsor / Самари Кӯҳсор
- **Business:** production and sale of fruit juices, jams, tomato paste and bottled drinking water.
- **System type:** integrated CRM + ERP with website integration.

All **12 functional modules** named in §2 of the spec exist as navigable screens. Coverage detail
is in §6 and the gap list is in §9 of this document.

---

## 3. File architecture

The document is four CSS layers plus one script, in this order. **Order matters** — later layers
intentionally override earlier ones.

```
<head>
  <style> …Modernist design-system stylesheet (inlined verbatim)…     ① tokens + components
  <style> …green palette override…                                     ② recolours ① 
  <style> …application layout CSS…                                     ③ app shell, grids, charts
  <style> …green chrome + semantic status palette…                     ④ dark chrome, status colors
  <script> window.__DS = {key:'modernist', cfg:{icons:'lucide'}} </script>
</head>
<body>
  <div id="app"></div>            ← entire UI is rendered into here
  <div class="sk-toast-wrap">     ← toast stack
  <script> …the application IIFE… </script>
</body>
```

### Layer ① — Modernist design system
Unmodified copy of the design system's `styles.css`. Provides `:root` tokens
(`--color-*`, `--font-*`, `--space-*`, `--radius-*`, `--shadow-*`) and component classes
(`.btn`, `.tag`, `.card`, `.input`, `.field`, `.seg`, `.table`, `.dialog`, `.nav`, `.hr`).
**Do not edit this layer.** Its character: flat, 0px radius, 2px rules, Archivo throughout,
flush-left labels, modular grid.

### Layer ② — Green palette override
Replaces only the accent ramps. Nothing else about Modernist changes.

```css
--color-accent: #1f7a3d;          /* "Bottle" green — the brand accent   */
--color-accent-2: #3d8a55;
--color-accent-100…900            /* #eff8f1 → #10301b                    */
--color-accent-2-100…900          /* #f0f7f2 → #1a3623                    */
```

Two alternative greens were built and are kept for reference:
`themes/modernist-green-orchard.css` (fresher, agricultural, `#3f9427`) and
`themes/modernist-green-eucalyptus.css` (cooler, `#0e7a63`). Swapping layer ② for either file
recolours the whole system with no other change.

### Layer ③ — Application layout
All classes are prefixed `sk-` to keep them clearly separate from design-system classes.
Covers the app shell grid, sidebar, top bar, popovers, page header, KPI cards, bar chart,
pipeline bars, list rows, toolbars, toasts and the dialog helpers.

### Layer ④ — Green chrome + semantic status
Two distinct jobs, both driven by client feedback:

1. **Branded chrome** — sidebar and top bar are filled `--sk-chrome: #124524` with white text.
   Everything inside those two regions gets its own light-on-dark treatment (inputs, buttons,
   segmented control, avatar, badge). Popovers explicitly revert to light surfaces.
2. **Semantic status colours** — because the green palette is mono, status had to be
   re-introduced as its own axis. See §5.

---

## 4. Application script — structure and state

One IIFE, vanilla JS, no framework. Rendering is string-templating into `innerHTML`.

### State
```js
var st = {
  mod:        'dashboard',  // active module key — drives the whole main pane
  lang:       'ru',         // 'ru' | 'tj' | 'en' — interface chrome only
  period:     'month',      // 'day' | 'week' | 'month' | 'quarter' — dashboard chart + revenue KPI
  notif:      false,        // notifications popover open
  menu:       false,        // user menu popover open
  notifCount: 5,            // unread badge
  query:      ''            // live search string, filters the visible table
};
```

### Render functions
| Function | Responsibility |
|---|---|
| `render()` | Full repaint: rebuilds shell + main pane. Used for navigation and language change. |
| `rerenderContent()` | Repaints `#sk-content` only. Used for period change and search typing (preserves focus + caret). |
| `rerenderTop()` | Repaints the top bar only. Used for popover open/close. |
| `shell(main)` | Sidebar + top bar + content wrapper + dialog markup. |
| `dashboard()` | The executive overview screen. |
| `moduleView(id)` | Generic module screen: page header → KPI row → toolbar → data table. |
| `table(cols, rows)` | Renders a table and applies the live `st.query` filter; shows an empty state when nothing matches. |
| `kpiCards(list)` | The KPI strip; column count adapts to the number of KPIs. |
| `icon(name, size)` | Returns an icon span. **Already includes its own `.sk-ic` wrapper — never wrap the call in another `.sk-ic`.** |
| `toast(msg)` | Transient bottom-right confirmation, auto-dismisses after 2.4s. |

### Boot sequence
The script polls until both the stylesheet and Lucide have loaded (or 1.6s elapses), then calls
`render()`. This exists because a blocking `document.write` of the icon CDN previously stalled
first paint.

---

## 5. Design contract — read before changing any colour

### Palette roles
| Token | Value | Used for |
|---|---|---|
| `--color-accent` | `#1f7a3d` | Brand accent: primary buttons, chart bars, pipeline fills, active marks. |
| `--sk-chrome` | `#124524` | Sidebar + top bar fill. |
| `--sk-chrome-mark` | `#b0dcbd` | Active nav indicator, status dot, count pill on active item. |
| `--sk-ok` / `--sk-ok-t` | `#1f7a3d` / `#175c2e` | Healthy: В норме, Готово, Оплачено, Выпущено, Активен, Действует. |
| `--sk-warn` / `--sk-warn-t` | `#b8791a` / `#8a5510` | Attention: Истекает, Контроль QC, На согласовании, Приёмка. |
| `--sk-danger` / `--sk-danger-t` | `#c0341c` / `#9a2814` | Problem: Просрочен, Карантин, Низкий остаток, Поломка, Задержка, Отклонено. |
| `--sk-info` | grey | Neutral in-flight states: Новый, В работе, В пути, Переговоры, Отгружен. |

**Rule:** green means *healthy*, never merely *branded*, anywhere inside the content area.
This was an explicit client decision ("зелёный «в норме», янтарный «внимание», красный «проблема»").
The chrome is the only place green is decorative.

### Status tag classes
`.tag` (from the design system) + one of `.tag-ok`, `.tag-warn`, `.tag-danger`, `.tag-info`,
`.tag-neutral`. In data, every status is authored as `{t:'label', v:'ok'|'warn'|'danger'|'info'|'neutral'}`.
Current distribution: 32 ok, 26 neutral, 20 info, 13 danger, 8 warn.

### Layout invariants
These were fixed in response to review and must not regress:
- Sidebar brand block and top bar are **both exactly 64px** so the divider is continuous across the seam.
- First sidebar group label aligns to the page kicker (both at y = 80).
- The page title takes remaining free space (`flex:1 1 auto; min-width:0`) so it never wraps behind the KPI row.
- Search icon: `.sk-search .sk-ic` is 16×16 absolutely positioned at `left:12px`; input has `padding-left:38px`. Do not nest a second `.sk-ic` around `icon()`.

---

## 6. Screen-by-screen detail

### Global shell (present on every screen)

**Sidebar (252px, fixed).** Brand block "СК / Самари Кӯҳсор / CRM-ERP платформа", then 13 nav
entries in 4 groups, then a status footer ("Система · рабочая среда"). Entries carry a count
pill where there is pending work.

| Group | Entries (count pill) |
|---|---|
| ОБЗОР | Панель управления |
| ПРОДАЖИ | CRM и продажи (34) · Интеграция с сайтом (12) · Товары и цены |
| ОПЕРАЦИИ | Склад и запасы (7) · Закупки и поставщики (6) · Производство (5) · Качество и безопасность (3) · Логистика (8) |
| УПРАВЛЕНИЕ | Финансы и бюджет (5) · Персонал · Оборудование и ТО · Документы |

**Top bar (64px).** Global search · ТҶ/РУ/EN switcher · notifications bell with unread badge ·
settings · user chip (А. Раҳимов, Администратор) with dropdown.

---

### 6.1 Панель управления — executive overview *(landing screen)*

Six KPI cards: Выручка (2 480 000 c., +12.4%) · Заказы в работе (47, +6) · Дебиторка
(318 500 c., 5 счетов) · Стоимость запасов (1.24 млн c., −2.1%) · Партии на карантине (3) ·
Просроч. поставки (2).

Then four panels:
- **Выручка и заказы** — dual bar chart, revenue + order count, re-plots on period change.
  Data exists for all four periods (day / week / month / quarter).
- **Воронка продаж** — Заявки с сайта 64 → Лиды 38 → КП 24 → Заказы 15 → Оплачено 11.
- **Запасы: остатки и сроки** — 5 rows with status tags.
- **Производство сегодня** — 72% plan-vs-actual bar (12 000 план / 8 640 факт) + 3 batch rows.
- **Лента событий** — 4 most recent events, severity-coloured.
- **Последние заказы** — 6-column table, SO-0908…SO-0912.

Covers spec §6 "Management Dashboards" as a single cross-module view.

### 6.2 CRM и продажи
KPIs: Новые лиды 34 · Открытые сделки 47 · Конверсия 22% · Просроч. задачи 5.
Table: Клиент · Тип · Регион · Статус · Сумма · Менеджер. 6 customers across Душанбе, Худжанд,
Хорог, Бохтар; pipeline stages Новый лид → Переговоры → КП отправлено → Выиграно.

### 6.3 Интеграция с сайтом
KPIs: Заявки сегодня 12 · Оптовые запросы 4 · Жалобы 1 · Отклики (вакансии) 2.
Table: № · Тип обращения · Имя/компания · Контакт · Дата · Статус. Reference numbers follow the
submission type (`WR-` wholesale, `CF-` contact, `DA-` distributor, `CP-` complaint, `JB-` job),
matching the spec's requirement that every website submission returns a reference number.
Statuses show the CRM hand-off: Новое → Создан лид.

### 6.4 Товары и цены
KPIs: Всего SKU 38 · Активных 34 · Языки каталога 3 · Средняя цена 42 c.
Table: SKU · Наименование · Категория · Упаковка · Цена · Срок годн. · Статус.
Real product lines: juices (apple, pomegranate, apricot), jams (apricot), tomato paste, bottled
water. SKU convention `JUS-APL-100`, `JAM-APR-035`, `PST-TOM-050`, `WAT-050-24`.

### 6.5 Склад и запасы
KPIs: Позиций 212 · Низкий остаток 7 · Скоро истекает 4 · Стоимость запасов 1.24 млн c.
Table: Артикул · Товар · Партия · Остаток · Срок годн. · Локация · Статус.
Demonstrates batch numbers (`B-2588`), expiry dates, bin locations (`Склад ГП · A-12`,
`Матер. · C-07`, `Сырьё · D-01`) and low-stock/expiry alerts.

### 6.6 Закупки и поставщики
KPIs: Заявки 9 · Открытые PO 6 · Ожидают приёмки 3 · Поставщики 24.
Table: PO № · Поставщик · Позиции · Сумма · Ожидается · Статус.
Status ladder: Черновик → Согласование → Подтверждён → В пути → Приёмка.

### 6.7 Производство
KPIs: План на смену 12 000 ед · Факт 8 640 ед (72%) · Выход 96.4% · Простой 34 мин.
Table: Заказ · Продукт · Партия · План · Факт · Брак · Статус. Manufacturing orders `MO-06xx`
map 1:1 to batches `B-26xx`; status ladder В работе → На контроле QC → Готово.

### 6.8 Качество и безопасность
KPIs: Проверок 18 · На карантине 3 · Несоответствия 2 · Выпущено 15.
Table: Партия · Продукт · Тест · Результат · Инспектор · Статус.
Tests include pH, microbiology, Brix/viscosity, metal detection, organoleptics.
Status ladder: Карантин → Выпущено / Отклонено — the quarantine-and-release capability the spec
lists as an acceptance condition.

### 6.9 Логистика
KPIs: Рейсов 8 · В пути 5 · Задержки 1 · Транс. расходы 96 400 c.
Table: Рейс · Маршрут · Водитель · Машина · Груз · Статус. Real Tajik routes
(Душанбе → Худжанд / Бохтар / Куляб, Худжанд → Хорог) with Tajik plate formats.

### 6.10 Финансы и бюджет
KPIs: Бюджет 3.10 млн c. · Исполнено 2.42 млн c. (78%) · Дебиторка 318 500 c. · Кредиторка 204 000 c.
Table: Документ · Отдел · Назначение · Сумма · Статус · Дата.
Two document classes: `ER-` expense requests (На согласовании → Одобрено → Оплачено) and
`INV-` invoices (Оплачено / Просрочен).

### 6.11 Персонал
KPIs: Сотрудников 148 · На смене 96 · В отпуске 7 · Истекают договоры 3.
Table: ФИО · Отдел · Должность · Смена · Статус · Договор до. Shifts (Дневная / Ночная /
Сменная) and contract-expiry tracking.

### 6.12 Оборудование и ТО
KPIs: Активов 64 · В работе 58 · ТО скоро 5 · Поломки 1.
Table: Актив · Тип · Линия · След. ТО · Гарантия до · Статус. Assets `EQ-0xx` — filling lines,
pasteuriser, labeller, cold room.

### 6.13 Документы
KPIs: Документов 320 · Действующих 298 · Истекают 6 · На согласовании 4.
Table: Документ · Тип · Владелец · Версия · Действует до · Статус. Versioned controlled
documents: ISO 22000 certificate, sanitation SOP, supplier contract, sanitary permit, operator manual.

---

## 7. Interaction inventory

Every interactive element is delegated through one document-level click handler keyed on
`data-act`. To add behaviour, add a `data-act` value and a branch — do not attach inline handlers.

| `data-act` | Trigger | Behaviour |
|---|---|---|
| `nav` | Sidebar entry, "Все →" link | Sets `st.mod`, clears search, full re-render |
| `lang` | ТҶ / РУ / EN | Sets `st.lang`, re-renders all chrome strings |
| `period` | Сегодня / Неделя / Месяц / Квартал | Re-plots chart + revenue KPI |
| `notif` | Bell | Toggles notifications popover |
| `menu` | User chip | Toggles user dropdown |
| `markall` | "Прочитать всё" | Zeroes the unread badge, toasts |
| `notif-go` | A notification row | Closes popover, jumps to Интеграция с сайтом |
| `create` | «Создать» | Opens the new-record dialog |
| `dlg-save` / `dlg-close` | Dialog footer | Closes dialog; save toasts "Запись сохранена" |
| `export` | «Экспорт» | Toasts "Экспорт в Excel подготовлен" |
| `toast-filter` | «Фильтры» | Placeholder toast |
| `toast-settings` / `toast-profile` / `toast-logout` | Gear / menu items | Placeholder toasts |

Additional behaviour: clicking outside an open popover closes it. Typing in either search field
filters the visible table live and preserves focus and caret position.

**Placeholder actions** (`export`, `toast-filter`, `toast-settings`, `toast-profile`,
`toast-logout`, `dlg-save`) intentionally only toast. They are the seams where real
functionality attaches.

---

## 8. Internationalisation model

`T.ru`, `T.tj`, `T.en` hold every chrome string: module names, group headings, button labels,
KPI labels, dialog copy, table column headers for the dashboard order table.

**Important:** only the *chrome* is translated. Row data (customer names, product names, status
labels) stays Russian in all three languages, because it represents database content, not UI. In
the real system those come from the API — product names specifically must be multilingual per
spec §2 (Products and Pricing: "multilingual product catalogue").

Adding a language = adding a fourth key to `T` with the same shape, and a fourth radio in `topHTML()`.

---

## 9. What is NOT built — gap list against the spec

This prototype covers screens and navigation. The following are specified but absent:

**Not implemented at all**
- Authentication, sessions, HTTPS/2FA, session timeout (spec §3).
- **Role-based access.** Only the Administrator view exists. Spec §3 names 12 roles
  (Administrator, Director, Sales, Finance, Procurement, Warehouse, Production, Quality,
  Logistics, HR, Driver, Read-only) with restricted access to financial, HR and quality data.
- Audit trail; archive-instead-of-delete semantics; daily backups.
- The website→CRM API itself (the Интеграция screen shows results, not the integration).
- Excel import/export, PDF report generation (the button is a placeholder).
- Barcode / QR support.
- Real notification triggers (the feed is static sample data).
- Offline tolerance / autosave / retry.

**Partially represented — screens exist, flows do not**
The five end-to-end workflows in spec §5 are visible as *states in tables* but are not walkable
as multi-step flows. None of these transitions can be performed in the prototype:
1. **Sales:** inquiry → lead → quotation → order → stock check → invoice → delivery → payment.
2. **Procurement:** request → approval → quotations → PO → delivery → inspection → receipt → payment.
3. **Production:** plan → material reservation → batch → testing → release → stock.
4. **Complaint:** complaint → identification → batch traceability → investigation → corrective action → closure.
5. **Budget:** expense request → budget check → approval → payment → document → actual update.

**Detail views do not exist.** Every module is a list. There is no record detail page, no edit
form beyond the generic dialog, no drill-down from a table row. This is the single largest piece
of remaining design work.

**Responsive.** Designed desktop-first at ~1440px. The shell is a fixed two-column grid; it has
not been adapted for tablet or mobile, which spec §4 requires.

---

## 10. How to extend

### Add a module
1. Add its key to the relevant group in `GROUPS`.
2. Add an icon in `MODICON` (Lucide name).
3. Add its translations to `T.ru/tj/en` → `mod`.
4. Add a `MODULES.<key>` entry with `kpis`, `cols`, `rows`.
5. Optionally add a `NAVCOUNT` badge.

No other change is needed — `moduleView()` is generic.

### Add a status
Author it in data as `{t:'Label', v:'ok'|'warn'|'danger'|'info'|'neutral'}`. Never hard-code a
colour on a tag.

### Replace mock data with an API
All data lives in top-level constants: `MODULES`, `NOTIFS`, `CHART`, `PIPE`, `STOCK`, `BATCH`,
`ORDERS`. Each is read only at render time, so the cleanest migration is to make the render
functions async (or hydrate these constants from a fetch before `boot()`), keeping the exact
same shapes. The table renderer expects `cols: string[]` and `rows: (string | {t,v})[][]`.

### Change the green
Swap layer ② for one of the two alternative theme files. Do not edit accent values inline —
they are ramps, and the light/dark steps are used for tints, hovers and pressed states.

---

## 11. Gotchas

- **`icon()` is self-wrapping.** It returns `<span class="sk-ic">…</span>`. Wrapping it again
  shifts the glyph 12px right — this was a real defect found in review.
- **Chrome regions invert text colour.** Anything placed inside `.sk-top` or `.sk-side`
  inherits white text. Popovers rendered inside `.sk-top` explicitly reset to `var(--color-text)`;
  any new light surface in the chrome needs the same reset.
- **`innerHTML` re-render loses focus.** `rerenderContent()` deliberately restores focus and
  caret for the search fields. Any new input that survives a re-render needs the same treatment.
- **Icons repaint after every render.** `paintIcons()` must be called after any `innerHTML`
  assignment, or Lucide placeholders stay empty.
- **`:has()` is used** for the segmented control's checked state. Fine in current browsers; note
  it if a legacy target appears.
- **Layer order is load-bearing.** Moving layer ④ above ③, or ② above ①, breaks the chrome and
  the palette respectively.

---

## 12. Related files in the project

| Path | What it is |
|---|---|
| `review/Samari-Kuhsor-Green.html` | **The deliverable.** Chosen direction, green palette. |
| `frames/dashboard.html` | Themeable source used by the 4-direction comparison; takes `?ds=modernist\|classical\|organic\|nocturne`. Kept in sync with the deliverable's layout fixes. |
| `Samari Kuhsor CRM-ERP.dc.html` | The pannable canvas comparing all four original directions. |
| `review/Samari-Kuhsor-{Modernist,Classical,Organic,Nocturne}.html` | The four original standalone review builds (red Modernist, etc.). |
| `review/Samari-Kuhsor-Modernist-{Bottle,Orchard,Eucalyptus}.html` | The three green palette candidates. |
| `themes/modernist-green-*.css` | Palette override layers for the three greens. |
| `uploads/_extracted_spec.txt` | Plain-text extraction of the client's Word specification. |

---

## 13. Recommended next steps

In the order that de-risks the build:

1. **Design the record detail view** — one pattern reused by all 12 modules (header, field
   groups, related records, activity log, actions). Currently the biggest unknown.
2. **Build one end-to-end flow as a clickable prototype** — Sales is the right first choice; it
   is the spec's Phase 1 and exercises website intake, CRM, stock and finance in one path.
3. **Add the role switcher** so restricted views can be reviewed by the client before backend work.
4. **Define the API contract** from the table shapes in §6 — they are already close to the
   resource shapes the backend needs.
5. **Responsive pass** for tablet/mobile per spec §4.
6. Phase against the spec's own plan: Phase 1 Core MVP → Phase 2 Operations → Phase 3 Expansion.
