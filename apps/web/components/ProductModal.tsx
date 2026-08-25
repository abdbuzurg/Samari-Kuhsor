'use client';

import { useEffect, useRef } from 'react';

import { Link } from '@/i18n/routing';
import { ProductViewTracker } from '@/components/ProductViewTracker';
import { palette } from '@/lib/palette';
import { specsFor } from '@/lib/specs';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The product quick-look, opened from the assembly line.
 *
 * It exists so clicking a product on the belt does not throw the visitor off the
 * home page: they came to browse, and a full navigation to a detail page for a
 * five-product catalogue is a heavier answer than the question deserved. The
 * full page is one click further, from the modal's own "Подробнее" link.
 *
 * The accessibility work here is not in the design, which is a prototype. A
 * modal that traps nothing, announces nothing and cannot be closed from the
 * keyboard is unusable for anyone not using a mouse — that is a defect, not a
 * design decision to reproduce:
 *
 *   - `role="dialog"` + `aria-modal` + a label, so it is announced as a dialog
 *   - focus moves in on open and returns to the opener on close
 *   - Escape closes it
 *   - Tab is trapped inside while it is open
 *   - the page behind does not scroll
 */
export function ProductModal({
  product,
  onClose,
  labels,
}: {
  product: PublicProduct;
  onClose: () => void;
  labels: { requestPrice: string; learnMore: string; close: string };
}) {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    // Remember who opened it, so focus can go back there rather than to the top
    // of the document — which would lose the visitor's place on a long page.
    const opener = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== 'Tab') return;

      const focusable = panelRef.current?.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
      );
      if (!focusable || focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.body.style.overflow = previousOverflow;
      opener?.focus?.();
    };
  }, [onClose]);

  const specs = specsFor(product);

  return (
    <>
      {/* One of the two surfaces that SHOWS a product — the other is the product
          page. The belt click that opened this modal changes no URL, so without
          this the site's signature element would be invisible to the ranking
          (docs/01-DECISIONS.md D12). */}
      <ProductViewTracker sku={product.sku} source="belt_modal" />
    <div
      onClick={onClose}
      data-testid="product-modal-backdrop"
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(20,28,34,.62)',
        backdropFilter: 'blur(5px)',
        zIndex: 90,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
        animation: 'skcFadeIn .26s ease-out backwards',
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="product-modal-title"
        data-testid="product-modal"
        // Stops a click inside the panel from reaching the backdrop's close
        // handler. Without it, selecting text in the description closes the modal.
        onClick={(event) => event.stopPropagation()}
        className="skc-2col"
        style={{
          position: 'relative',
          width: '100%',
          maxWidth: 900,
          maxHeight: '90vh',
          overflowY: 'auto',
          background: palette.page,
          borderRadius: 22,
          padding: 40,
          animation: 'skcModalUp .42s cubic-bezier(.34,0,.24,1) backwards',
          display: 'grid',
          gridTemplateColumns: '.82fr 1.18fr',
          gap: 36,
        }}
      >
        <button
          ref={closeRef}
          type="button"
          onClick={onClose}
          aria-label={labels.close}
          style={{
            position: 'absolute',
            top: 16,
            right: 16,
            width: 34,
            height: 34,
            borderRadius: '50%',
            border: `1px solid rgba(35,88,58,.18)`,
            background: '#fff',
            color: palette.deep,
            fontSize: 15,
            lineHeight: 1,
            cursor: 'pointer',
            fontFamily: 'inherit',
          }}
        >
          ×
        </button>

        {/* The striped block stands in for product photography, which the client
            has not supplied. Deliberately obvious rather than a stock photo of
            someone else's juice. */}
        <div
          aria-hidden
          style={{
            alignSelf: 'start',
            aspectRatio: '1/1',
            borderRadius: 16,
            background: product.tint,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            position: 'relative',
          }}
        >
          <div
            style={{
              width: '42%',
              height: '72%',
              borderRadius: 11,
              background:
                'repeating-linear-gradient(45deg,rgba(255,255,255,.78),rgba(255,255,255,.78) 10px,rgba(255,255,255,.35) 10px,rgba(255,255,255,.35) 20px)',
              border: '1px solid rgba(0,0,0,.06)',
              boxShadow: '0 14px 30px rgba(0,0,0,.12)',
            }}
          />
          <span
            style={{
              position: 'absolute',
              top: 16,
              left: 16,
              width: 10,
              height: 10,
              borderRadius: '50%',
              background: product.accent,
            }}
          />
        </div>

        <div>
          <span
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 8,
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '.1em',
              textTransform: 'uppercase',
              color: '#7C8C7E',
              marginBottom: 14,
            }}
          >
            <span
              aria-hidden
              style={{
                width: 9,
                height: 9,
                borderRadius: '50%',
                background: product.accent,
              }}
            />
            {product.line}
          </span>

          <h3
            id="product-modal-title"
            className="disp"
            style={{
              fontSize: 27,
              fontWeight: 800,
              color: palette.deep,
              margin: '0 0 12px',
              lineHeight: 1.15,
            }}
          >
            {product.name}
          </h3>
          <p style={{ fontSize: 15, lineHeight: 1.6, color: palette.muted, margin: '0 0 18px' }}>
            {product.description}
          </p>

          <div
            style={{
              border: `1px solid rgba(35,88,58,.13)`,
              borderRadius: 12,
              overflow: 'hidden',
              marginBottom: 20,
            }}
          >
            {/* The first five rows only. The rest — composition, nutrition, shelf
                life — are on the full page, and most of them read «уточняется»
                until the lab has verified them. */}
            {specs.slice(0, 5).map((row, i) => (
              <div
                key={row.k}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '160px 1fr',
                  gap: 12,
                  padding: '10px 14px',
                  borderBottom: '1px solid rgba(35,88,58,.07)',
                  background: i % 2 ? palette.page : '#fff',
                }}
              >
                <div style={{ fontSize: 12, color: '#7C8C7E' }}>{row.k}</div>
                <div style={{ fontSize: 12.5, fontWeight: 600, color: palette.deep }}>
                  {row.v}
                </div>
              </div>
            ))}
          </div>

          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
            <Link
              href="/contact"
              data-sk-category="cta"
              data-sk-sku={product.sku}
              style={{
                textDecoration: 'none',
                fontSize: 14.5,
                fontWeight: 700,
                background: palette.primary,
                color: '#fff',
                padding: '13px 22px',
                borderRadius: 11,
              }}
            >
              {labels.requestPrice}
            </Link>
            <Link
              href={`/catalogue/${product.sku}`}
              data-sk-category="product"
              data-sk-sku={product.sku}
              style={{
                textDecoration: 'none',
                fontSize: 14.5,
                fontWeight: 700,
                color: palette.deep,
                padding: '13px 22px',
                borderRadius: 11,
                border: `1.5px solid ${palette.hairlineStrong}`,
              }}
            >
              {labels.learnMore}
            </Link>
          </div>
        </div>
      </div>
    </div>
    </>
  );
}
