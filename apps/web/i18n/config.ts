/**
 * Locale configuration for the public site.
 *
 * Routed, unlike the CRM: `/ru/...`, `/tg/...`, `/en/...`. Search engines need a
 * distinct URL per language and an hreflang set pointing at real addresses, and
 * a cookie provides neither (docs/07-IMPLEMENTATION-PLAN.md I14).
 *
 * The Tajik code is `tg`, never `tj`. The prototype used `tj` — a country TLD,
 * not a language code — and the schema's CHECK constraint enforces `tg`
 * (docs/07-IMPLEMENTATION-PLAN.md C2). Only the LABEL is ТҶ.
 */

/** Order matters: the approved prototype's switcher reads ТҶ / РУ / EN. */
export const locales = ['tg', 'ru', 'en'] as const;
export type Locale = (typeof locales)[number];

/** Russian is the default (D10, CLAUDE.md §6). */
export const defaultLocale: Locale = 'ru';

export const localeLabels: Record<Locale, string> = {
  tg: 'ТҶ',
  ru: 'РУ',
  en: 'EN',
};

/**
 * BCP-47 tags for `hreflang` and the `lang` attribute.
 *
 * `tg-TJ` rather than bare `tg`: Tajik is written in Cyrillic in Tajikistan and
 * in Perso-Arabic elsewhere, and the region subtag is what tells a crawler which
 * audience the page is for.
 */
export const htmlLang: Record<Locale, string> = {
  tg: 'tg-TJ',
  ru: 'ru-RU',
  en: 'en',
};

export function isLocale(value: string | undefined): value is Locale {
  return !!value && (locales as readonly string[]).includes(value);
}
