/**
 * Locale configuration, safe for both server and client.
 *
 * Kept separate from request.ts on purpose: request.ts imports next/headers,
 * which is server-only, and the top bar's language switcher is a client
 * component. Exporting these constants from the same module would pull
 * next/headers into the browser bundle and fail the build.
 *
 * The Tajik code is `tg`, never `tj`. The prototype used `tj` (a country TLD);
 * the schema uses `tg` (ISO 639-1) and the database CHECK constraint enforces it.
 * See docs/07-IMPLEMENTATION-PLAN.md C2. Only the LABEL is ТҶ.
 */
/**
 * Order matters: the approved prototype's switcher reads ТҶ / РУ / EN, and the
 * order is part of the visual contract (CLAUDE.md §5). `ru` is still the default
 * locale — that is a separate concern from display order.
 */
export const locales = ['tg', 'ru', 'en'] as const;
export type Locale = (typeof locales)[number];

/** Russian is the default: docs/01-DECISIONS.md D10 and CLAUDE.md §6. */
export const defaultLocale: Locale = 'ru';

export const LOCALE_COOKIE = 'samari_locale';

/** Display labels for the ТҶ / РУ / EN switcher in the top bar. */
export const localeLabels: Record<Locale, string> = {
  tg: 'ТҶ',
  ru: 'РУ',
  en: 'EN',
};

export function isLocale(value: string | undefined): value is Locale {
  return !!value && (locales as readonly string[]).includes(value);
}
