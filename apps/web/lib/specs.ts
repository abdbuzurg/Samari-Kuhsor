import type { PublicProduct } from '@/lib/catalogue';

/**
 * The product specification table.
 *
 * One definition, used by the product page and by the quick-look modal. They
 * showed the same twelve rows in the design, and computing them in two places is
 * how the modal ends up claiming a different closure type from the page it links
 * to.
 *
 * Most rows read «Уточняется». That is the client's explicit content rule, not a
 * gap to fill in: compositions, nutritional values, shelf life and water
 * classification are not published until recipes are approved and lab-verified
 * (docs/02-SCHEMA.md:176). An invented value here would be a claim the company
 * cannot support to a regulator.
 */

export interface SpecRow {
  k: string;
  v: string;
}

const TBC = 'Уточняется';

export function specsFor(product: PublicProduct): SpecRow[] {
  const isWater = product.line === 'Вода';

  return [
    { k: 'Артикул / SKU', v: product.sku },
    { k: 'Объём / нетто', v: product.volume || TBC },
    { k: 'Упаковка', v: product.pack || TBC },
    { k: 'Материал упаковки', v: isWater ? 'ПЭТ' : 'Стекло' },
    {
      k: 'Тип укупорки',
      v: isWater
        ? 'Винтовая пробка'
        : product.sku === 'APJ-1000'
          ? 'Металлическая крышка'
          : 'Крышка Twist-off',
    },
    { k: 'Состав', v: 'Уточняется после утверждения рецептуры' },
    { k: 'Пищевая ценность', v: 'Уточняется после лабораторного контроля' },
    { k: 'Условия хранения', v: 'В сухом прохладном месте, вдали от света' },
    { k: 'Срок годности', v: 'Уточняется после испытаний' },
    { k: 'После вскрытия', v: 'Хранить в холодильнике' },
    { k: 'Адрес производства', v: 'Тем, Хорог, ГБАО, Республика Таджикистан' },
    { k: 'Статус сертификации', v: 'На согласовании' },
  ];
}
