'use client';

import { useEffect, useRef, useState } from 'react';
import { useTranslations } from 'next-intl';

import { ProductModal } from '@/components/ProductModal';
import { Link } from '@/i18n/routing';
import { palette } from '@/lib/palette';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The catalogue assembly line — v1, the approved animation.
 *
 * CLAUDE.md §5 is explicit about every detail here, because a v2 was built and
 * rejected:
 *   - products roll in FROM THE LEFT and PARK in slots. Not a continuous loop.
 *   - the stagger is ~150ms.
 *   - batch buttons page between sets of four.
 *   - the belt is a plain green gradient (#4E8F63 → #2C5A3C). A Pamiri textile
 *     pattern was tried and rejected.
 *   - horizontal and swipeable on mobile. It must NEVER become a vertical list.
 *   - under prefers-reduced-motion it degrades to static placement plus a fade.
 *
 * The roll and fade keyframes (skcRoll, skcFade) ship verbatim from the
 * prototype's CSS; this component only decides when to apply them.
 */

const BATCH_SIZE = 4;

export function AssemblyLine({ products }: { products: PublicProduct[] }) {
  const t = useTranslations();
  const beltRef = useRef<HTMLDivElement | null>(null);

  const [batch, setBatch] = useState(0);
  const [started, setStarted] = useState(false);
  const [reduced, setReduced] = useState(false);
  // The quick-look. Clicking a slot opens it rather than navigating away: the
  // visitor came to browse, and a full page load for a five-product catalogue is
  // a heavier answer than the question deserved.
  const [selected, setSelected] = useState<PublicProduct | null>(null);

  const totalBatches = Math.max(1, Math.ceil(products.length / BATCH_SIZE));
  const multiBatch = totalBatches > 1;
  const visible = products.slice(batch * BATCH_SIZE, batch * BATCH_SIZE + BATCH_SIZE);

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return;
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    setReduced(mq.matches);
    const onChange = () => setReduced(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  // The line starts when it scrolls into view, not on mount. Products that
  // rolled in while the visitor was reading the hero would have finished before
  // they arrived, and the animation is the point.
  useEffect(() => {
    const el = beltRef.current;
    if (!el) return;
    if (typeof IntersectionObserver === 'undefined') {
      // jsdom and very old browsers. Render parked rather than invisible: a
      // catalogue nobody can see is a worse failure than a missing animation.
      setStarted(true);
      return;
    }
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setStarted(true);
        }
      },
      { threshold: 0.3 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  function page(direction: -1 | 1) {
    if (!multiBatch) return;
    setBatch((current) => (current + direction + totalBatches) % totalBatches);
    // Re-arm so the new set rolls in rather than appearing.
    setStarted(false);
    requestAnimationFrame(() => setStarted(true));
  }

  const label = `${String(batch + 1).padStart(2, '0')} / ${String(totalBatches).padStart(2, '0')}`;

  return (
    <div ref={beltRef} style={{ position: 'relative', padding: '30px 0 42px' }}>
      <div
        style={{
          position: 'relative',
          background: palette.deep,
          borderRadius: 20,
          padding: '26px 0 34px',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '0 26px 18px',
          }}
        >
          <span style={{ color: '#EAF1DD', fontSize: 12.5, fontWeight: 700, opacity: 0.8 }}>
            {label}
          </span>
          <div style={{ display: 'flex', gap: 6, marginLeft: 'auto' }}>
            <button
              type="button"
              onClick={() => page(-1)}
              disabled={!multiBatch}
              aria-label="Предыдущая партия"
              style={batchButtonStyle(multiBatch)}
            >
              ▲
            </button>
            <button
              type="button"
              onClick={() => page(1)}
              disabled={!multiBatch}
              aria-label="Следующая партия"
              style={batchButtonStyle(multiBatch)}
            >
              ▼
            </button>
          </div>
        </div>

        <div style={{ position: 'relative', minHeight: 250 }}>
          {/* The belt. A plain green gradient — a Pamiri textile pattern was
              tried and rejected (CLAUDE.md §5). */}
          <div
            aria-hidden
            style={{
              position: 'absolute',
              left: 44,
              right: 44,
              top: '50%',
              transform: 'translateY(-50%)',
              height: 100,
              borderRadius: 14,
              overflow: 'hidden',
              background: `linear-gradient(180deg,${palette.beltTop},#3E7A52 46%,#356B47 72%,${palette.beltBottom})`,
              boxShadow:
                'inset 0 9px 18px rgba(0,0,0,.26),inset 0 -9px 18px rgba(0,0,0,.22),inset 0 0 0 1px rgba(0,0,0,.12)',
            }}
          />

          <ul
            className="skc-slots"
            style={{
              position: 'relative',
              display: 'grid',
              gridTemplateColumns: `repeat(${Math.max(visible.length, 1)}, 1fr)`,
              gap: 12,
              padding: '0 44px',
              listStyle: 'none',
              margin: 0,
            }}
          >
            {visible.map((product, i) => (
              <li
                key={product.id}
                className="skc-slot"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  transition: 'transform .35s cubic-bezier(.34,0,.24,1)',
                  ...slotAnimation(started, reduced, i),
                }}
              >
                <button
                  type="button"
                  onClick={() => setSelected(product)}
                  className="skc-slot-button"
                  style={{
                    background: 'none',
                    border: 'none',
                    padding: 0,
                    font: 'inherit',
                    color: 'inherit',
                    cursor: 'pointer',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    gap: 12,
                  }}
                >
                  <span
                    aria-hidden
                    style={{
                      width: 74,
                      height: 132,
                      borderRadius: '10px 10px 14px 14px',
                      background: product.tint,
                      borderTop: `18px solid ${product.accent}`,
                      display: 'block',
                    }}
                  />
                  <span
                    style={{
                      color: '#EAF1DD',
                      fontSize: 13,
                      fontWeight: 700,
                      textAlign: 'center',
                      maxWidth: 150,
                    }}
                  >
                    {product.short}
                  </span>
                  <span style={{ color: '#EAF1DD', fontSize: 11.5, opacity: 0.7 }}>
                    {product.volume}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>

      {selected && (
        <ProductModal
          product={selected}
          onClose={() => setSelected(null)}
          labels={{
            requestPrice: t('cta.requestPrice'),
            learnMore: t('cta.learnMore'),
            close: t('a11y.close'),
          }}
        />
      )}

      <div style={{ marginTop: 18, textAlign: 'right' }}>
        <Link
          href="/catalogue"
          style={{
            textDecoration: 'none',
            fontSize: 14,
            fontWeight: 700,
            color: palette.primaryHover,
            borderBottom: `2px solid ${palette.accent}`,
            paddingBottom: 3,
          }}
        >
          {t('cta.allProducts')}
        </Link>
      </div>
    </div>
  );
}

function batchButtonStyle(enabled: boolean): React.CSSProperties {
  return {
    width: 30,
    height: 24,
    borderRadius: 6,
    border: '1px solid rgba(255,255,255,.22)',
    background: 'rgba(255,255,255,.08)',
    color: '#EAF1DD',
    fontSize: 11,
    fontFamily: 'inherit',
    cursor: enabled ? 'pointer' : 'not-allowed',
    opacity: enabled ? 1 : 0.32,
  };
}

/**
 * The roll-in, staggered ~150ms per slot.
 *
 * Under reduced motion it becomes a fade at 100ms intervals — static placement
 * plus a fade, which is what CLAUDE.md §5 requires. Before the line has scrolled
 * into view the slots are transparent; they are still in the DOM and still
 * readable by a screen reader, which is why opacity is used rather than
 * `display:none`.
 */
function slotAnimation(started: boolean, reduced: boolean, index: number): React.CSSProperties {
  if (!started) return { opacity: 0 };
  if (reduced) {
    return { animation: `skcFade .5s ease ${(index * 0.1).toFixed(2)}s backwards` };
  }
  return {
    animation: `skcRoll .95s cubic-bezier(.34,0,.24,1) ${(index * 0.15).toFixed(2)}s backwards`,
  };
}
