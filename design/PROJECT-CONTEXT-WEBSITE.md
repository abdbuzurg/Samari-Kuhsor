# Samari Kuhsor — Project Context & Handoff

**Purpose of this document:** complete context for the Samari Kuhsor (Самари Кӯҳсор) website project so a new collaborator can pick it up without re-reading the chat history. Covers the brief, decisions made, design system, every page and section, all interactive behavior, assets, and known constraints.

**Last updated:** 17 August 2026

---

## 1. Project at a glance

| Item | Value |
|---|---|
| Client / brand | **Самари Кӯҳсор** (Samari Kuhsor) — "dары гор" / gifts of the mountains |
| Legal entity | **QOIM LLC** (parent company; Samari Kuhsor is its commercial brand) |
| Production site | Тем, город Хорог, ГБАО (Gorno-Badakhshan Autonomous Region), Republic of Tajikistan |
| Business | Food & beverage factory — juices, jams, tomato paste, drinking water |
| Site language | **Russian** (RU). A ТҶ / РУ / EN switcher exists in the header but is currently visual only — not wired to translations. |
| Deliverable | Informational corporate website (not e-commerce). Retail prices are deliberately **not** published. |
| Audience | Large retail buyers, distributors, wholesale partners |
| Status | Design prototype, approved direction, iterating on details |

### Files in the project

| File | Role |
|---|---|
| `Samari Kuhsor - C.dc.html` | **The source of truth.** The entire website — all pages, logic, styling. Edit this. |
| `Samari Kuhsor - website.html` | Compiled single-file standalone build (~1 MB). Generated from the source; **never edit directly.** Regenerate after every change. |
| `assets/` | Images used by the site (see §9) |
| `uploads/` | Raw client-supplied material (logos, map, pattern reference) |
| `spec_text.txt` | Original ToR / terms of reference document from the client |
| `support.js` | Runtime file. Do not edit. Note: it is an older runtime version and was deliberately left in place — the component was authored against it. |

**Workflow:** edit `Samari Kuhsor - C.dc.html` → recompile to `Samari Kuhsor - website.html` → deliver the standalone file to the client. The client reviews by opening the standalone HTML in a browser (works fully offline, all assets inlined).

---

## 2. How we got here (decision history)

1. **Three design directions** (A, B, C) were built from the client's ToR document. The client chose **Direction C**. A and B were deleted from the project.
2. **Palette went bright/eco.** The original C was dark; the client asked for brighter colors matching an "ECO FRIENDLY" agenda. Result: the green/cream palette in use today.
3. **Typography fixed for Tajik.** The Cyrillic letter **ҳ** rendered badly in the original font. We tested candidates against the letterforms used on Obi Zulol / Siyoma (Tajik brand sites) and selected **Golos Text**, which renders ҳ, ҷ, ӯ cleanly with a proper descender.
4. **Real brand logos integrated** (QOIM badge + Samari Kuhsor emblem) after the client supplied them.
5. **Pamir regional identity layer added** — mountain line-art, ridgeline dividers, chorkhona (Pamiri skylight) motif, "Roof of the World" framing.
6. **A brief experiment with a cool slate/turquoise "Pamir palette" was reverted** at the client's request — the warm green/cream palette was explicitly preferred and must be kept.
7. **Catalog animation went through three generations:**
   - v1: assembly line, products roll in and park, batch buttons ✅
   - v2: continuous endless loop with a split detail view ❌ (rejected)
   - v3: **reverted to v1** (current) — this is the approved behavior.
8. **Belt surface styling iterated:** a Pamiri textile pattern (taken from a traditional dress photo the client shared) was tried and **rejected** — client feedback: *"Узор был не очень... I would go with the first iteration, blank green line."* Current belt is a **plain green gradient**.
9. **Map replaced.** A hand-drawn SVG map was swapped for the client's supplied raster map of Tajikistan, with a true border-draw animation.

### Standing client preferences (do not violate)
- **Keep the warm green/cream palette.** A cooler slate/turquoise variant was tried and explicitly rejected.
- **Plain green conveyor belt.** No textile pattern on the belt.
- **Nothing cartoonish.** Refined, premium, trustworthy for large retail buyers.
- Ornament and regional motifs are **light accents only** — never heavy.
- Small, targeted edits when asked; don't redesign adjacent areas unprompted.

---

## 3. Design system

### Palette (warm green + cream — the approved one)

| Token | Hex | Use |
|---|---|---|
| Page background | `#F5F7EE` | Body base |
| Section tint | `#EAF1DD` | Alternating section backgrounds, hero gradient top |
| Deep green | `#23583A` | Headings, dark UI, footer background |
| Primary green | `#3E8E5A` | Buttons, links, primary accents |
| Deep primary | `#2E7A4B` | Gradient partner, link hover |
| Light green | `#5BA86B` | Eyebrow labels, secondary accents |
| Apricot accent | `#E79A3A` | Underlines, pulley centers, highlight accents |
| Body text | `#4E5D53` | Paragraphs |
| Muted text | `#7C8C7E` / `#85937F` / `#96a191` | Secondary/caption text |
| Belt dark green | `#26473A` → `#1F3A2C` | Conveyor housing |
| Belt surface | `#4E8F63` → `#2C5A3C` | The belt itself (plain green gradient) |

Product accent colors (per product, used for dots and card tints):
- Juice `#6FAE3E` / tint `#E8F1DA`
- Jam `#E79A3A` / tint `#FBEACB`
- Tomato paste `#D6533B` / tint `#F7DCD3`
- Water `#3FA3C4` / tint `#DAEDF4`

### Typography
- **Single family: Golos Text** (Google Fonts), weights 400–900.
- Chosen specifically for correct Tajik Cyrillic (ҳ, ҷ, ӯ).
- Display headings use a `.disp` class = `letter-spacing: -.02em`.
- Scale: hero h1 60px / section h2 32–38px / body 15–18px / eyebrow labels 11px uppercase with `.18em` tracking.

### Visual motifs
- **Mountain line-art** — hero background ridgelines, a layered snow-capped ridgeline divider between sections, faint ridge silhouette in the CTA block.
- **Chorkhona** (nested-squares Pamiri skylight motif) — used in the eco badge frames and as small diamond glyphs in retailer chips.
- **Rounded cards** — 16–22px radius, 1px `rgba(35,88,58,.12)` borders, soft shadows.
- Layout max width **1240px**, side padding 32px.

---

## 4. Site structure

Single-page application with client-side view switching (no page reloads). Five views:

1. **Главная** (Home) — the main landing page, most of the content
2. **Продукция** (Catalogue) — filterable product grid
3. **Product detail** — reached by clicking any product
4. **Производство и качество** (Production & Quality)
5. **Контакты** (Contacts)

Persistent across all views: **header** and **footer**.

---

## 5. Page-by-page detail

### 5.1 Header (persistent)
- Sticky, translucent background with blur, 1px bottom border.
- Left: **Samari Kuhsor emblem** (real logo, 46px tall) + wordmark "Самари Кӯҳсор" with "SAMARI KUHSOR · PAMIR" in small tracked caps beneath.
- Center-right: nav — Главная / Продукция / Производство и качество / Контакты. Active item gets a light green pill background.
- Language switcher: **ТҶ / РУ / EN** segmented control (РУ active). Visual only.
- Right: green CTA button **"Дистрибьюторам"** → goes to Контакты.

### 5.2 Footer (persistent)
Dark green (`#23583A`), four columns:
1. Samari Kuhsor emblem on a white chip, brand name, descriptor line, and a **QOIM LLC** company chip with the corporate badge.
2. **Разделы** — links to the four main views.
3. **Компания** — О компании, Таджикистан, Новости и медиа, Загрузки (labels only, not wired).
4. **Правовая информация** — Политика конфиденциальности, Условия использования, Cookie (labels only).

Bottom bar: `© 2026 QOIM. Все права защищены.` and the tagline `Тозагӣ аз табиат · Чистота от природы`.

---

### 5.3 HOME — section by section

#### a) Hero
- Background: gradient `#EAF1DD → #F5F7EE` with layered **mountain ridgeline SVG** silhouettes along the bottom.
- Eyebrow pill: **"Тозагӣ аз табиат"** (Purity from nature) with a small green dot.
- H1: **"Продукты с высоты Памира"** (green highlight on the last words).
- Body paragraph: Tajik brand, modern production in Тем, Хорог, ГБАО; juices, jams, paste, water; respect for the region and its clean resources.
- Two CTAs: **"Смотреть продукцию →"** (solid green) and **"О производстве"** (outlined).
- Right side: a white card listing the products as numbered rows (index, colored bar, name, volume · packaging, arrow). Each row is clickable → product detail.

#### b) Trust strip
White band, four equal columns, thin dividers:
1. **Лабораторный контроль** — checks at production stages before any claims are published.
2. **Прослеживаемость** — raw-material control from intake to finished product.
3. **Гигиена и санитария** — cleaning protocols and HACCP approach on site.
4. **Профессиональная упаковка** — glass and PET, modern formats and closures.

#### c) Catalog — the animated assembly line ⭐ (signature element)
Heading **"Наш каталог"** (eyebrow: КАТАЛОГ), intro line: *"Продукция сходит прямо с линии. Наведите на продукт для эффекта, нажмите — чтобы открыть подробности."* Link at right: **"Вся продукция →"**.

The belt itself sits in a dark green rounded housing:
- **Plain green gradient belt surface** (`#4E8F63 → #2C5A3C`), inset shadows top and bottom, rounded 14px. **No pattern** — this is the client-approved look.
- A **pulley/roller** at each end: dark radial-gradient circle with an apricot hub.
- **4 product slots per batch**, evenly spaced on the belt. Each product = a colored card (with a diagonal-stripe placeholder standing in for the real product photo), an accent dot in the corner, a dark wooden tray/pallet beneath it, and the product name below.
- Top-right: batch counter (`01 / 02`) and **▲ / ▼ buttons** to page between batches.

**Behavior:**
- **Roll-in on scroll:** when the section enters the viewport, products roll in from the left, staggered ~150 ms apart, easing to a stop and parking in their slots. Fires once per mount.
- **Batch switching:** ▲/▼ swap in the previous/next batch of 4; the belt re-runs the roll-in animation. With 5 products there are 2 batches.
- **Hover:** the item lifts and scales slightly, gains a soft glow, and reveals a small **"Просмотр →"** cue over the card.
- **Click:** opens the product modal (below).
- **Replay:** navigating away to another view and back re-arms the animation so it plays again.
- **Reduced motion:** `prefers-reduced-motion` users get a static placement with simple fades, no roll-in.

#### d) Product modal
Centered overlay, dark blurred backdrop, click-outside or × to close. Contents:
- Large product image area (tinted, with accent dot)
- Product line eyebrow, name, description
- Spec table (striped rows)
- Two CTAs: **"Запросить цену"** → Контакты, and **"Подробнее →"** → full product detail page

#### e) Retailer trust belt
- Heading: **"Нам доверяют ведущие ритейлеры"**
- A continuously scrolling horizontal marquee of retailer chips (name + small chorkhona diamond glyph), with fade masks at both edges.
- Chips are muted by default and brighten on hover. No pause, no click, no modal — this strip is deliberately quieter than the product belt so the catalog stays the hero.
- Caption below: *"Логотипы приведены для примера оформления."* — **these are placeholder retailer names**; real logos need to be supplied by the client.

#### f) Animated Tajikistan map — "Корни в Хороге, сердце Бадахшана"
Two-column card. Left: eyebrow **СДЕЛАНО НА ПАМИРЕ**, heading, a paragraph about the site being in Тем, Хорог, southern GBAO at the confluence of the Panj and Gunt rivers, and two stats — **≈2 200 м** (above sea level) and **ГБАО** (Republic of Tajikistan).

Right: the map, which animates in three stages when it scrolls into view:
1. **Border draws** — the exact outline of Tajikistan is traced as an animated stroke (~2 s). The path was extracted pixel-by-pixel from the client's map image, so it matches perfectly.
2. **Interior fills in** — the raster map fades in behind the completed border (~0.8 s).
3. **Heart appears** — the maroon heart marker pops in at Khorog's true location in southern GBAO, followed by a green **"Хорог"** label pill.

Caption: *"Таджикистан · местоположение Хорога, ГБАО"*.

Reduced-motion users see the finished map immediately. Like the belt, this replays when you navigate away and return.

#### g) Eco / purity section — "Чистый регион — чистый продукт"
- Eyebrow **ЧИСТОТА ПАМИРА**, centered heading, and a supporting line tying purity to the untouched high-altitude environment: clean meltwater and mountain air as the natural basis of quality.
- Three badges, each in a distinct **chorkhona-style line-art frame** with a custom drawn icon:
  1. **Без консервантов** (No preservatives)
  2. **Без добавок** (No additives)
  3. **Экологичное производство** (Eco-friendly production)

#### h) Ridgeline divider
A layered mountain range graphic (back range, front range, snow caps on the tallest peaks, crisp ridgeline stroke) used as a section separator. Replaced an earlier abstract zigzag that read as a squiggle.

#### i) About the brand
- Eyebrow **О БРЕНДЕ**, pull-quote: **«Самари Кӯҳсор» — дары гор.**
- Paragraph: commercial brand of QOIM; modern production plus the clean resources of the Pamir region and professional quality control; trust through process transparency.
- **QOIM LLC** badge chip with the corporate logo.
- Two cards: the slogan card (**Тозагӣ аз табиат** / Чистота от природы) and a region card (**Тем, Хорог** / ГБАО, Республика Таджикистан).

#### j) Production preview
Heading **"Производство и качество"** + "Подробнее →" link. A 3×2 grid of the six numbered process steps (titles only — full text on the Production page).

#### k) News
Heading **"Новости"**, three cards with tinted image placeholders, a category · date line in apricot, and a headline:
1. **Строительство** · Июнь 2026 — Строительство производственной площадки в Теме
2. **Оборудование** · Июль 2026 — Монтаж технологического оборудования
3. **Запуск** · Август 2026 — Подготовка к запуску продуктовых линий

#### l) Closing CTA
Green gradient block with a faint ridge silhouette. Heading **"Опт, дистрибуция и партнёрство"**, a line about formats, specifications and terms being available on request once product is ready to ship, and a white **"Отправить запрос →"** button. Three stat tiles alongside: product lines count, **ГБАО** (Хорог, Таджикистан), and export/wholesale packaging note (glass and PET).

---

### 5.4 PRODUCTS (Продукция)
- Breadcrumb, H1 **Продукция**, and a note that retail prices are not published at the informational-site stage.
- **Filter chips:** Все / Соки / Джемы / Паста / Вода — active chip is solid green.
- **Search field** — filters by product name as you type.
- Responsive card grid. Each card: tinted image area with accent dot, product line label, name, volume · packaging. Click → product detail.

### 5.5 PRODUCT DETAIL
- Breadcrumb: Главная / Продукция / product name.
- Left: large tinted product image plus a row of four thumbnails (placeholders — real photography needed).
- Right: line eyebrow with accent dot, product name, description, and a **12-row spec table**:

  Артикул / SKU · Объём / нетто · Упаковка · Материал упаковки · Тип укупорки · Состав · Пищевая ценность · Условия хранения · Срок годности · После вскрытия · Адрес производства · Статус сертификации

- An apricot **caution note**: compositions, nutritional values and claims are published only after recipes are approved and lab-verified.
- CTAs: **"Запрос для дистрибьюторов"** → Контакты, and **"↓ Технический лист (PDF)"** (not wired to a real file).

### 5.6 PRODUCTION & QUALITY (Производство и качество)
- Tinted header band with breadcrumb, H1 **"Производство и качество"**, and an intro about a controlled, traceable process from raw-material intake to packaging at the Тем, Хорог site.
- **Six process steps** in a 3×2 grid, each numbered with a title and description:
  1. **Приёмка и контроль сырья** — inspection of fruit, vegetables and water on arrival
  2. **Переработка фруктов и овощей** — washing, preparation, processing
  3. **Водоподготовка и выдув ПЭТ** — water treatment and PET bottle forming
  4. **Пастеризация и розлив** — heat treatment and filling
  5. **Укупорка, охлаждение, маркировка** — sealing, cooling, labelling
  6. **Упаковка и складирование** — grouped packaging, formats, storage
- Closing card: **"Гигиена, прослеживаемость и лаборатория"** — sanitary protocols, control at every stage, lab checks; quality claims published only with supporting documents. Three tags: **HACCP-подход**, **Прослеживаемость**, **Лаборатория**. Paired with a photo placeholder for the production hall / equipment.

### 5.7 CONTACTS (Контакты)
Two columns.

**Left — enquiry form.** Fields:
- ФИО * (required)
- Компания
- Email * (required)
- Телефон / WhatsApp * (required)
- Страна * (required)
- **Instagram (URL)** — added at client request
- **Facebook (URL)** — added at client request
- Причина обращения * — dropdown: Общий вопрос / Опт / Дистрибуция / Поставщик / Трудоустройство / СМИ
- Сообщение * (textarea, required)
- Consent checkbox * — personal-data processing per the privacy policy
- Submit: **"Отправить запрос"**

On submit the form is replaced by a green checkmark and **"Заявка отправлена"** with a note that it is a demo form — real submission is configured during development. **No backend is wired.**

**Right — company details card** (dark green): QOIM badge and **QOIM LLC**, "Юридическое лицо · бренд «Самари Кӯҳсор»", the address (Тем, Хорог, ГБАО, Республика Таджикистан), then `info@samari-kuhsor.tj`, `+992 —` / WhatsApp, and hours Пн–Пт 09:00–18:00. **Phone number is a placeholder — client must supply.**

Below it: a static map card showing Tajikistan with Khorog marked.

---

## 6. Product catalogue (the five products)

Do not invent products. This is the full list.

| # | ID | Line | Name | Volume | Packaging | SKU |
|---|---|---|---|---|---|---|
| 01 | `apple` | Соки | Яблочный сок прямого отжима | 1 000 мл | Стеклянная бутылка | APJ-1000 |
| 02 | `apricot` | Джемы | Абрикосовый джем | 212–228 мл | Стеклянная банка | APR-220 |
| 03 | `tomato` | Паста | Томатная паста | 500 мл | Стеклянная банка | TOM-500 |
| 04 | `water` | Вода | Негазированная питьевая вода 0,5 л | 500 мл | ПЭТ-бутылка | WAT-500 |
| 05 | `water1l` | Вода | Негазированная питьевая вода 1 л | 1 000 мл | ПЭТ-бутылка | WAT-1000 |

**Important content rule the client set:** the site must not make unverified claims. Compositions, nutritional values, shelf life and water classification are all marked "уточняется" (to be confirmed) until recipes are approved and lab-tested. Keep this posture.

---

## 7. Interactive behavior summary

| Feature | Behavior |
|---|---|
| Navigation | Client-side view switching, scrolls to top on change |
| Language switcher | ТҶ / РУ / EN — visual state only, **not wired to translations** |
| Assembly line roll-in | Triggers when the section scrolls into view; staggered ~150 ms; replays on batch change and on re-entering the Home view |
| Batch buttons ▲▼ | Cycle through batches of 4 products; re-runs roll-in |
| Product hover | Lift + scale + glow + "Просмотр →" cue |
| Product click | Opens modal with specs and CTAs |
| Retailer marquee | Continuous scroll, hover brightens, no interaction |
| Map animation | 3-stage: border draws → fill appears → heart + label; replays on re-entry |
| Catalogue filter | Category chips + live name search |
| Contact form | Client-side validation, demo success state, no backend |
| Reduced motion | All major animations degrade to static placement + fades |
| Belt style | Switchable variant setting: **green** (default, approved), steel, slats, stone, minimal, textile |

---

## 8. Responsive notes
- Desktop-first, built at 1240px max width.
- Two-column layouts collapse to single column on narrow screens.
- The assembly line stays **horizontal and swipeable** on mobile — it must never collapse into a vertical list.
- Product cards are slightly smaller on mobile.

---

## 9. Assets

| File | What it is |
|---|---|
| `assets/logo-samari-mark-web.png` | Samari Kuhsor emblem, cropped from the client's full logo, transparent, web-optimised. Used in header + footer. |
| `assets/logo-qoim-web.png` | QOIM LLC corporate badge, 512px. Used in About, Contacts, footer. |
| `assets/map-base-v3.png` | The client's Tajikistan map with the heart marker cleanly removed (so the heart can be animated separately). |
| `assets/map-heart-t.png` | The heart marker alone, background made transparent. |
| `assets/tj-outline.txt` | SVG path data for Tajikistan's border, traced pixel-accurately from the map image. Drives the draw animation. |
| `assets/map-full.jpg` | The client's original map, used as the static map on Contacts. |
| `assets/belt-pattern.png` | Pamiri textile pattern derived from a dress photo. **Currently unused** — the textile belt was rejected. Kept in case it's wanted elsewhere. |

Images are downscaled for web so the standalone build stays reasonable (~1 MB).

---

## 10. Known gaps / next steps

**Needs client input:**
- **Real product photography** — every product image is currently a striped placeholder.
- **Real retailer logos** — the marquee uses placeholder names.
- **Phone number** (`+992 —` is incomplete), and confirmation of the email address.
- **Pamir landscape photos** — were requested as premium section backdrops but never supplied; those areas use placeholders.
- News article content and images.
- Technical datasheet PDFs for the product pages.

**Needs development work:**
- Contact form has **no backend** — needs a real submission endpoint.
- Language switcher needs actual **ТҶ and EN translations**; only Russian copy exists.
- Footer links (Компания, Правовая информация) are labels, not links — legal pages need to be written.

**Note on imagery:** all diagrams, icons, map graphics and the ridgeline art on this site are hand-drawn vector work. No AI-generated photography is available in this workflow — real photos have to come from the client.

---

## 11. Working conventions

- Edit the source file, then recompile the standalone. Never hand-edit the compiled build.
- The client reviews by opening the standalone HTML directly in a browser, so **every asset must be inlined** — external file references will break for them. If something doesn't appear in the standalone build but works in preview, the asset reference is the first thing to check.
- When the client asks for a small change, change only that. They have pushed back on unrequested redesigns.
- Client feedback often arrives as an annotated screenshot with a red circle around the element in question.
