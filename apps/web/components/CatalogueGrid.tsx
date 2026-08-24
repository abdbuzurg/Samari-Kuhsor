'use client';

import { useMemo, useState } from 'react';
import { useTranslations } from 'next-intl';

import { Link } from '@/i18n/routing';
import { palette } from '@/lib/palette';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The catalogue grid, with the prototype's line filter and live search.
 *
 * Filtering is client-side because the catalogue is five products: a round trip
 * to filter five rows would be slower than the keystroke that triggered it, and
 * the whole set is already on the page.
 */
export function CatalogueGrid({ products }: { products: PublicProduct[] }) {
  const t = useTranslations('catalogue');
  const [query, setQuery] = useState('');
  const [line, setLine] = useState<string>('');

  const lines = useMemo(() => {
    const seen: string[] = [];
    for (const p of products) if (!seen.includes(p.line)) seen.push(p.line);
    return seen;
  }, [products]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return products.filter((p) => {
      if (line && p.line !== line) return false;
      if (!needle) return true;
      return (
        p.name.toLowerCase().includes(needle) ||
        p.short.toLowerCase().includes(needle) ||
        p.sku.toLowerCase().includes(needle)
      );
    });
  }, [products, query, line]);

  return (
    <div>
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          gap: 10,
          alignItems: 'center',
          marginBottom: 26,
        }}
      >
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('searchPlaceholder')}
          aria-label={t('searchLabel')}
          style={{
            fontSize: 14,
            fontFamily: 'inherit',
            padding: '11px 16px',
            borderRadius: 10,
            border: `1px solid ${palette.hairlineStrong}`,
            background: '#fff',
            color: palette.deep,
            minWidth: 240,
          }}
        />
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <FilterChip active={line === ''} onClick={() => setLine('')}>
            {t('all')}
          </FilterChip>
          {lines.map((l) => (
            <FilterChip key={l} active={line === l} onClick={() => setLine(l)}>
              {l}
            </FilterChip>
          ))}
        </div>
      </div>

      {filtered.length === 0 ? (
        <div data-testid="catalogue-no-match" style={{ padding: '48px 0' }}>
          <p style={{ fontSize: 17, fontWeight: 700, margin: '0 0 6px' }}>{t('noMatch')}</p>
          <p style={{ fontSize: 14, color: palette.muted, margin: 0 }}>{t('noMatchBody')}</p>
        </div>
      ) : (
        <ul
          className="skc-2col"
          style={{
            listStyle: 'none',
            margin: 0,
            padding: 0,
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))',
            gap: 20,
          }}
        >
          {filtered.map((p) => (
            <li key={p.id} data-testid="catalogue-card">
              <Link
                href={`/catalogue/${p.sku}`}
                style={{
                  textDecoration: 'none',
                  display: 'block',
                  background: '#fff',
                  borderRadius: 16,
                  border: `1px solid ${palette.hairline}`,
                  overflow: 'hidden',
                  height: '100%',
                }}
              >
                <div
                  aria-hidden
                  style={{
                    background: p.tint,
                    height: 148,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                  }}
                >
                  <span
                    style={{
                      width: 54,
                      height: 96,
                      borderRadius: '8px 8px 10px 10px',
                      background: '#fff',
                      borderTop: `14px solid ${p.accent}`,
                      display: 'block',
                    }}
                  />
                </div>
                <div style={{ padding: '18px 20px 22px' }}>
                  <p
                    style={{
                      margin: '0 0 8px',
                      fontSize: 11.5,
                      fontWeight: 700,
                      letterSpacing: '.05em',
                      textTransform: 'uppercase',
                      color: p.accent,
                    }}
                  >
                    {p.line}
                  </p>
                  <h3 style={{ margin: 0, fontSize: 16, lineHeight: 1.35, fontWeight: 700 }}>
                    {p.name}
                  </h3>
                  <p style={{ margin: '8px 0 0', fontSize: 13, color: palette.muted }}>
                    {p.volume} · {p.pack}
                  </p>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      className="skc-tap"
      type="button"
      onClick={onClick}
      aria-pressed={active}
      style={{
        fontSize: 13,
        fontWeight: 700,
        fontFamily: 'inherit',
        padding: '9px 15px',
        borderRadius: 9,
        cursor: 'pointer',
        border: `1px solid ${active ? palette.primary : palette.hairlineStrong}`,
        background: active ? palette.primary : 'transparent',
        color: active ? '#fff' : palette.deep,
      }}
    >
      {children}
    </button>
  );
}
