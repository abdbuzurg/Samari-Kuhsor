/**
 * The website's approved copy, transcribed from the design.
 *
 * Source: `Samari Kuhsor - C.dc.html` in the client's design project — which its
 * own handoff document calls "the source of truth. The entire website." Every
 * string here is verbatim from it.
 *
 * Two rules govern this file.
 *
 * First: **do not write new copy here.** An earlier pass invented hero text
 * ("Фрукты Памира, бережно превращённые в продукт") rather than transcribing the
 * approved line, which is exactly what CLAUDE.md §5 forbids — reproduce the
 * prototype, do not improve it. The client approved these words; a developer
 * rewriting them is a change nobody asked for.
 *
 * Second: **do not add claims.** The client's standing content rule is that
 * compositions, nutritional values, shelf life and water classification stay
 * "уточняется" until recipes are approved and lab-tested. Nothing in this file
 * asserts anything about the product that is not already approved.
 *
 * Why constants rather than the CMS: these are the marketing sections
 * (hero, trust strip, eco, about, CTA), and the CMS has no seeded content. A
 * CMS-driven home page against an empty database renders a blank site, which is
 * the wrong default for the one page a visitor lands on. The CMS owns the
 * catalogue and the news, which is where content genuinely changes.
 */

export const HERO = {
  eyebrow: 'Крыша мира · Памир',
  // Split so the highlighted half can carry the accent colour, as in the design.
  titleLead: 'Продукты',
  titleAccentPrefix: 'с ',
  titleAccent: 'Крыши мира',
  lead:
    'Самари Кӯҳсор — таджикский бренд из Хорога, высокогорный Памир (Бадахшан). ' +
    'Соки, джемы, паста и вода, рождённые чистой водой и воздухом гор.',
} as const;

/** Four columns, not three. The design's trust strip is a 4-up grid. */
export const TRUST_ITEMS = [
  {
    title: 'Лабораторный контроль',
    text: 'Проверки на этапах производства до публикации заявлений.',
  },
  {
    title: 'Прослеживаемость',
    text: 'Контроль сырья от приёмки до готовой продукции.',
  },
  {
    title: 'Гигиена и санитария',
    text: 'Протоколы очистки и HACCP-подход на площадке.',
  },
  {
    title: 'Профессиональная упаковка',
    text: 'Стекло и ПЭТ, современные форматы и укупорка.',
  },
] as const;

/**
 * Retailer names for the trust marquee.
 *
 * These are PLACEHOLDERS and the design says so on the page itself
 * ("Логотипы приведены для примера оформления"). That caption is not decoration
 * — it is the thing that stops the strip reading as a claim about who actually
 * stocks the product, which for a factory that has not opened would be false.
 * Keep the caption whenever these names are shown.
 */
export const RETAILERS = [
  { name: 'ОРИЁН', color: '#23583A' },
  { name: 'ФАРОВОН', color: '#2E7A4B' },
  { name: 'ПАЙКАР', color: '#23583A' },
  { name: 'САДБАРГ', color: '#E79A3A' },
  { name: 'ЗАМИН', color: '#23583A' },
  { name: 'НАВРӮЗ', color: '#2E7A4B' },
  { name: 'БАҲОР', color: '#23583A' },
  { name: 'ОСИЁ МАРТ', color: '#E79A3A' },
] as const;

export const MAP_SECTION = {
  eyebrow: 'Сделано на Памире',
  titleLine1: 'Корни в Хороге,',
  titleLine2: 'сердце Бадахшана',
  body:
    'Наша площадка — в Теме, город Хорог, на юге Горно-Бадахшанской автономной ' +
    'области (ГБАО), у слияния рек Пяндж и Гунт. Высокогорье Памира даёт чистую ' +
    'воду и сырьё.',
  stats: [
    { value: '≈2 200 м', label: 'над уровнем моря' },
    { value: 'ГБАО', label: 'Республика Таджикистан' },
  ],
  caption: 'Таджикистан · местоположение Хорога, ГБАО',
} as const;

export const ECO_SECTION = {
  eyebrow: 'Чистота Памира',
  title: 'Чистый регион — чистый продукт',
  lead:
    'Нетронутая природа высокогорья, чистая талая вода и горный воздух Памира — ' +
    'естественная основа качества, которую невозможно повторить где-либо ещё.',
} as const;

export const ECO_BADGES = [
  {
    icon: 'leaf',
    title: 'Без консервантов',
    text: 'Только природная основа — без искусственных консервантов.',
  },
  {
    icon: 'drop',
    title: 'Без добавок',
    text: 'Чистый состав без красителей и усилителей вкуса.',
  },
  {
    icon: 'mtn',
    title: 'Эко-производство',
    text: 'Бережный процесс на чистой воде и воздухе Памира.',
  },
] as const;

export const ABOUT = {
  eyebrow: 'О бренде',
  quoteLead: '«Самари Кӯҳсор» — ',
  quoteAccent: 'дары гор',
  body:
    'Коммерческий бренд компании QOIM. Мы соединяем современное производство с ' +
    'чистыми ресурсами Памира и профессиональным контролем качества — доверие ' +
    'через прозрачность.',
  entityLabel: 'Юридическое лицо',
  entity: 'QOIM LLC',
  sloganLabel: 'Слоган',
  sloganTg: 'Тозагӣ аз табиат',
  sloganRu: 'Чистота от природы',
  regionLabel: 'Регион',
  regionCity: 'Тем, Хорог',
  regionArea: 'ГБАО, Республика Таджикистан',
} as const;

/**
 * The six process steps.
 *
 * The home page shows titles only; the Производство page adds the descriptions.
 * One list so the two cannot disagree about how many steps there are or what
 * they are called.
 */
export const PROCESS_STEPS = [
  {
    n: '01',
    title: 'Приёмка и контроль сырья',
    text: 'Входной контроль фруктов, овощей и воды при поступлении.',
  },
  {
    n: '02',
    title: 'Переработка фруктов и овощей',
    text: 'Мойка, подготовка и переработка сырья.',
  },
  {
    n: '03',
    title: 'Водоподготовка и выдув ПЭТ',
    text: 'Подготовка воды и формование ПЭТ-бутылки.',
  },
  {
    n: '04',
    title: 'Пастеризация и розлив',
    text: 'Термическая обработка и розлив в тару.',
  },
  {
    n: '05',
    title: 'Укупорка, охлаждение, маркировка',
    text: 'Герметизация, охлаждение и нанесение маркировки.',
  },
  {
    n: '06',
    title: 'Упаковка и складирование',
    text: 'Групповая упаковка, форматы и хранение.',
  },
] as const;

export const CTA = {
  title: 'Опт, дистрибуция и партнёрство',
  body:
    'Форматы упаковки, спецификации и условия — по запросу, как только продукция ' +
    'будет готова к отгрузке.',
  button: 'Отправить запрос →',
  tiles: [
    { value: '4', label: 'продуктовые линии' },
    { value: 'ГБАО', label: 'Хорог, Таджикистан' },
  ],
  wideTile: {
    label: 'Экспорт и опт',
    value: 'Стекло и ПЭТ · современная упаковка',
  },
} as const;

export const FOOTER = {
  brand: 'Самари Кӯҳсор',
  tagline: 'Roof of the World',
  blurb:
    'Самари Кӯҳсор — коммерческий бренд компании QOIM. Производственная площадка: ' +
    'Тем, Хорог, ГБАО, Республика Таджикистан.',
  companyChip: 'Компания',
  entity: 'QOIM LLC',
  sectionsLabel: 'Разделы',
  companyLabel: 'Компания',
  companyLinks: ['О компании', 'Таджикистан', 'Новости и медиа', 'Загрузки'],
  legalLabel: 'Правовая информация',
  copyright: '© 2026 QOIM. Все права защищены.',
  slogan: 'Тозагӣ аз табиат · Чистота от природы',
} as const;

export const BRAND = {
  name: 'Самари Кӯҳсор',
  subtitle: 'Roof of the World · Pamir',
} as const;
