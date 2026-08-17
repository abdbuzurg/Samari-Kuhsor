import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { Section, SectionHead } from '@/components/Section';
import { palette } from '@/lib/palette';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'production' });
  return { title: t('title'), description: t('lead') };
}

/**
 * Производство и качество.
 *
 * The six process steps come from the prototype. They describe what the factory
 * does; they make no claim about certification, because nothing is certified
 * yet — «Статус сертификации: на согласовании» is what the product pages say and
 * this page must not contradict them.
 */
const STEPS = [
  { n: '01', title: 'Приёмка и контроль сырья', text: 'Входной контроль фруктов и воды.' },
  { n: '02', title: 'Мойка и подготовка', text: 'Очистка, сортировка и подготовка сырья.' },
  { n: '03', title: 'Переработка', text: 'Отжим, уваривание или подготовка к розливу.' },
  { n: '04', title: 'Пастеризация и розлив', text: 'Термическая обработка и розлив в тару.' },
  {
    n: '05',
    title: 'Укупорка, охлаждение, маркировка',
    text: 'Герметизация, охлаждение и нанесение маркировки.',
  },
  { n: '06', title: 'Упаковка и складирование', text: 'Групповая упаковка, форматы и хранение.' },
];

const QUALITY = [
  {
    title: 'Лабораторный контроль',
    text: 'Проверки на этапах производства. Заявления публикуются только после подтверждения.',
  },
  {
    title: 'Прослеживаемость партий',
    text: 'Каждая партия связана с сырьём, сменой и результатами испытаний.',
  },
  {
    title: 'Карантин до выпуска',
    text: 'Готовая продукция помещается в карантин и выпускается только по решению контроля качества.',
  },
];

export default async function ProductionPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('production');

  return (
    <>
      <Section>
        <SectionHead title={t('title')} lead={t('lead')} />
      </Section>

      <Section>
        <h2 className="disp" style={{ fontSize: 22, margin: '0 0 22px', fontWeight: 800 }}>
          {t('processTitle')}
        </h2>
        {/* Horizontal and swipeable on mobile — it must never become a vertical
            list (CLAUDE.md §5). The .skc-slots class carries the prototype's own
            mobile rule, which turns this grid into a scroll-snapping row. */}
        <ol
          className="skc-slots"
          style={{
            listStyle: 'none',
            margin: 0,
            padding: 0,
            display: 'grid',
            gridTemplateColumns: 'repeat(6, 1fr)',
            gap: 14,
          }}
        >
          {STEPS.map((s) => (
            <li
              key={s.n}
              className="skc-slot"
              style={{
                background: '#fff',
                borderRadius: 14,
                border: `1px solid ${palette.hairline}`,
                padding: '18px 16px 20px',
              }}
            >
              <span
                style={{
                  display: 'inline-block',
                  fontSize: 12,
                  fontWeight: 800,
                  color: palette.accent,
                  marginBottom: 8,
                }}
              >
                {s.n}
              </span>
              <h3 style={{ fontSize: 14, lineHeight: 1.35, margin: '0 0 6px', fontWeight: 700 }}>
                {s.title}
              </h3>
              <p style={{ fontSize: 12.5, lineHeight: 1.5, color: palette.muted, margin: 0 }}>
                {s.text}
              </p>
            </li>
          ))}
        </ol>
      </Section>

      <Section>
        <h2 className="disp" style={{ fontSize: 22, margin: '0 0 22px', fontWeight: 800 }}>
          {t('qualityTitle')}
        </h2>
        <div
          className="skc-2col"
          style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 20 }}
        >
          {QUALITY.map((q) => (
            <div
              key={q.title}
              style={{
                background: palette.section,
                borderRadius: 16,
                padding: '24px 22px',
              }}
            >
              <h3 style={{ fontSize: 15.5, margin: '0 0 8px', fontWeight: 700 }}>{q.title}</h3>
              <p style={{ fontSize: 13.5, lineHeight: 1.6, color: palette.muted, margin: 0 }}>
                {q.text}
              </p>
            </div>
          ))}
        </div>
      </Section>
    </>
  );
}
