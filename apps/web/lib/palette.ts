/**
 * The approved palette, as constants.
 *
 * CLAUDE.md §5: a cooler slate/turquoise variant was built and explicitly
 * rejected. These values are the visual contract — reproduce, do not improve.
 *
 * They exist in TypeScript as well as CSS because the port carries the
 * prototype's inline styles, and a hex literal repeated across forty components
 * is how a palette drifts one shade at a time.
 */
export const palette = {
  page: '#F5F7EE',
  section: '#EAF1DD',
  deep: '#23583A',
  primary: '#3E8E5A',
  primaryHover: '#2E7A4B',
  accent: '#E79A3A',
  muted: '#4E5D53',
  white: '#ffffff',
  hairline: 'rgba(35,88,58,.12)',
  hairlineStrong: 'rgba(35,88,58,.24)',
  beltTop: '#4E8F63',
  beltBottom: '#2C5A3C',
} as const;

/** Per-product accents, from the prototype's data array. */
export const productAccents: Record<string, { accent: string; tint: string }> = {
  juice: { accent: '#6FAE3E', tint: '#E8F1DA' },
  jam: { accent: '#E79A3A', tint: '#FBEACB' },
  paste: { accent: '#D6533B', tint: '#F7DCD3' },
  water: { accent: '#3FA3C4', tint: '#DAEDF4' },
};
