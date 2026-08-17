import 'server-only';

import { callApi } from '@/lib/api';
import { productAccents } from '@/lib/palette';
import type { ItemListRow } from '@samari/types';

/**
 * The public catalogue.
 *
 * Content comes from the CRM (CLAUDE.md §2) — the five products are rows in
 * `items` with `item_translations`, not an array in this repository. The
 * prototype's hard-coded product list was a mock-up; carrying it forward would
 * mean the client edits a product in the CRM and the website does not change.
 *
 * The FALLBACK below is the exception, and it is deliberate. The public site
 * must render before the CRM has any data — the client tests the website first,
 * over an IP, with an empty database (docs/07-IMPLEMENTATION-PLAN.md). A
 * catalogue page that 500s because nobody has seeded the products yet would make
 * that test impossible. The fallback carries exactly the five SKUs the client
 * approved and nothing that could be mistaken for a claim: no composition, no
 * nutritional values, no shelf life (docs/02-SCHEMA.md:176).
 */

export interface PublicProduct {
  id: string;
  sku: string;
  idx: string;
  name: string;
  short: string;
  line: string;
  accent: string;
  tint: string;
  volume: string;
  pack: string;
  description: string;
}

/** The line each SKU belongs to, and the accent that goes with it. */
const LINES: Record<string, { line: string; palette: keyof typeof productAccents }> = {
  'APJ-1000': { line: 'Соки', palette: 'juice' },
  'APR-220': { line: 'Джемы', palette: 'jam' },
  'TOM-500': { line: 'Паста', palette: 'paste' },
  'WAT-500': { line: 'Вода', palette: 'water' },
  'WAT-1000': { line: 'Вода', palette: 'water' },
};

/**
 * The five approved SKUs, in catalogue order (docs/01-DECISIONS.md — the
 * catalogue is exactly five products).
 */
export const CATALOGUE_ORDER = ['APJ-1000', 'APR-220', 'TOM-500', 'WAT-500', 'WAT-1000'];

const FALLBACK: Omit<PublicProduct, 'accent' | 'tint' | 'idx' | 'line'>[] = [
  {
    id: 'APJ-1000',
    sku: 'APJ-1000',
    name: 'Яблочный сок прямого отжима',
    short: 'Яблочный сок',
    volume: '1 000 мл',
    pack: 'Стеклянная бутылка',
    description:
      'Сок прямого отжима в прозрачной стеклянной бутылке 1 000 мл. ' +
      'Финальные заявления публикуются после утверждения рецептуры.',
  },
  {
    id: 'APR-220',
    sku: 'APR-220',
    name: 'Абрикосовый джем',
    short: 'Абрикосовый джем',
    volume: '212–228 мл',
    pack: 'Стеклянная банка',
    description:
      'Абрикосовый джем в прозрачной стеклянной банке. ' +
      'Итоговый вес нетто указывается после испытаний фасовки.',
  },
  {
    id: 'TOM-500',
    sku: 'TOM-500',
    name: 'Томатная паста',
    short: 'Томатная паста',
    volume: '500 мл',
    pack: 'Стеклянная банка',
    description:
      'Томатная паста в стеклянной банке. Концентрация сухих веществ и вес ' +
      'нетто публикуются после утверждения рецептуры.',
  },
  {
    id: 'WAT-500',
    sku: 'WAT-500',
    name: 'Негазированная питьевая вода 0,5 л',
    short: 'Питьевая вода 0,5 л',
    volume: '500 мл',
    pack: 'ПЭТ-бутылка',
    description:
      'Негазированная питьевая вода в ПЭТ-бутылке 500 мл. Классификация воды ' +
      'не заявляется без документального подтверждения.',
  },
  {
    id: 'WAT-1000',
    sku: 'WAT-1000',
    name: 'Негазированная питьевая вода 1 л',
    short: 'Питьевая вода 1 л',
    volume: '1 000 мл',
    pack: 'ПЭТ-бутылка',
    description:
      'Негазированная питьевая вода в ПЭТ-бутылке 1 000 мл. Классификация воды ' +
      'не заявляется без документального подтверждения.',
  },
];

function decorate(
  base: Omit<PublicProduct, 'accent' | 'tint' | 'idx' | 'line'>,
  index: number,
): PublicProduct {
  const meta = LINES[base.sku] ?? { line: 'Продукция', palette: 'juice' as const };
  const colours = productAccents[meta.palette];
  return {
    ...base,
    idx: String(index + 1).padStart(2, '0'),
    line: meta.line,
    accent: colours.accent,
    tint: colours.tint,
  };
}

/**
 * Loads the catalogue for one locale.
 *
 * Falls back to the approved five when the API is unreachable or has no
 * products yet. It never throws: the public site rendering is more important
 * than the catalogue being live, and a visitor seeing the product range is a
 * better outcome than a 500.
 */
export async function loadCatalogue(locale: string): Promise<PublicProduct[]> {
  // Cached for five minutes. The catalogue is five products that change when
  // someone edits them in the CRM; a visitor seeing a name five minutes late is
  // not a problem, and a Go round trip on every page view is.
  const result = await callApi<ItemListRow[]>(
    `/public/products?locale=${encodeURIComponent(locale)}`,
    { revalidate: 300 },
  );

  if (!result.ok || !Array.isArray(result.data) || result.data.length === 0) {
    return FALLBACK.map(decorate);
  }

  const bySku = new Map(result.data.map((row) => [row.sku, row]));
  // Ordered by the approved catalogue order rather than by whatever the API
  // returns: the client agreed the order the products appear in.
  const ordered = CATALOGUE_ORDER.map((sku) => bySku.get(sku)).filter(
    (row): row is ItemListRow => !!row,
  );
  // Anything the CRM has that is not in the approved five still appears, after
  // them — a sixth product added later must not be invisible.
  for (const row of result.data) {
    if (!CATALOGUE_ORDER.includes(row.sku)) ordered.push(row);
  }

  return ordered.map((row, i) => {
    const fallback = FALLBACK.find((f) => f.sku === row.sku);
    return decorate(
      {
        id: row.id,
        sku: row.sku,
        name: row.name,
        short: fallback?.short ?? row.name,
        volume: fallback?.volume ?? '',
        pack: fallback?.pack ?? '',
        description: fallback?.description ?? '',
      },
      i,
    );
  });
}
