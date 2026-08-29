'use client';

import Image from 'next/image';
import { useEffect, useRef, useState } from 'react';

import { MAP_SECTION } from '@/lib/content';
import { palette } from '@/lib/palette';
import { TAJIKISTAN_BORDER_PATH, TAJIKISTAN_VIEWBOX } from '@/lib/tajikistan-path';

/**
 * The animated map of Tajikistan.
 *
 * Three stages, in order, when the section scrolls into view:
 *
 *   1. the border draws itself (~2s)
 *   2. the raster map fades in behind the completed border (~0.8s)
 *   3. the heart marker pops in at Khorog, then the arrow draws out to its label
 *
 * The order is the point. Drawing the outline first and filling it afterwards
 * reads as "this is the country, and here is where we are"; showing the filled
 * map immediately and animating a pin onto it says nothing.
 *
 * The «Хорог» label sits OUTSIDE the map rather than on it, joined to the marker
 * by an arrow. On the map it had to overlap the country to stay near the heart,
 * which put a filled pill over the terrain it was pointing at.
 *
 * The draw is `stroke-dasharray: 1` with `pathLength="1"`, so the dash maths is
 * independent of the path's true length — the path is 6 000 characters of traced
 * coordinates and computing its length in JS to animate it would be both slower
 * and wrong the moment anyone edits it.
 *
 * Under `prefers-reduced-motion` every stage is applied at once with no
 * transition: the finished map, immediately (CLAUDE.md §5).
 */

type Phase = 0 | 1 | 2 | 3;

/**
 * Arrow geometry, in the border path's own viewBox units (0 0 710 446).
 *
 * Sharing the border's coordinate system is what keeps the arrow on Khorog at
 * every width: the marker is placed by percentage against the same raster, so
 * expressing the arrow in pixels — or in a second viewBox — would need the two
 * to be reconciled on every resize.
 *
 * The heart occupies (386,337)–(434,385) there. The shaft leaves from the right
 * of the frame, below the country's eastern lobe, so it crosses empty map
 * instead of the terrain it is pointing at, and stops short of the marker.
 */
const ARROW = {
  tailX: 745,
  tailY: 418,
  tipX: 446,
  tipY: 381,
  /** Where the shaft stops and the head begins, along the tail→tip direction. */
  baseX: 460,
  baseY: 383,
  head: '446,381 460.6,377.7 459.4,387.8',
} as const;

/**
 * The same arrow for the stacked layout, where the label sits below the map.
 *
 * Slanted rather than vertical: the label is centred under the card at x=355,
 * but Khorog is at x=410. A vertical shaft would have to choose between sitting
 * under the label and pointing at the marker.
 *
 * The tail at y=490 is 44 units below the 446-unit frame, which is why
 * `.skc-map-callout` sets its gap as a PERCENTAGE — a fixed px margin would
 * drift away from this tail as the card resized.
 */
const ARROW_STACKED = {
  tailX: 355,
  tailY: 490,
  tipX: 410,
  tipY: 395,
  baseX: 402.5,
  baseY: 408,
  head: '410,395 407.3,410.8 397.7,405.2',
} as const;

export function TajikistanMap() {
  const sectionRef = useRef<HTMLElement | null>(null);
  const [phase, setPhase] = useState<Phase>(0);
  const [reduced, setReduced] = useState(false);
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return;
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    setReduced(mq.matches);
    const onChange = () => setReduced(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) return;

    const start = () => {
      if (reduced) {
        setPhase(3);
        return;
      }
      // The delays match the transition durations below, so each stage begins as
      // the previous one finishes rather than overlapping it.
      setPhase(1);
      timers.current.push(setTimeout(() => setPhase(2), 2200));
      timers.current.push(setTimeout(() => setPhase(3), 3000));
    };

    if (typeof IntersectionObserver === 'undefined') {
      // jsdom, and browsers old enough not to matter. Render the finished map
      // rather than an invisible one: a map nobody can see is a worse failure
      // than a missing animation.
      setPhase(3);
      return;
    }

    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) {
            start();
            io.disconnect();
          }
        }
      },
      { threshold: 0.3 },
    );
    io.observe(el);

    const pending = timers.current;
    return () => {
      io.disconnect();
      for (const t of pending) clearTimeout(t);
      pending.length = 0;
    };
  }, [reduced]);

  const instant = reduced ? '0s' : undefined;

  return (
    <section
      ref={sectionRef}
      data-testid="map-section"
      data-phase={phase}
      style={{ maxWidth: 1240, margin: '0 auto', padding: '64px 32px 24px' }}
    >
      <div
        className="skc-2col"
        style={{
          background: `linear-gradient(180deg,${palette.section},${palette.page})`,
          border: `1px solid ${palette.hairline}`,
          borderRadius: 22,
          padding: 44,
          display: 'grid',
          gridTemplateColumns: '.92fr 1.08fr',
          gap: 44,
          alignItems: 'center',
        }}
      >
        <div>
          <p
            style={{
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '.18em',
              textTransform: 'uppercase',
              color: palette.primaryHover,
              margin: '0 0 16px',
            }}
          >
            {MAP_SECTION.eyebrow}
          </p>
          <h2
            className="disp"
            style={{
              fontSize: 34,
              lineHeight: 1.1,
              fontWeight: 800,
              margin: '0 0 16px',
              color: palette.deep,
            }}
          >
            {MAP_SECTION.titleLine1}
            <br />
            {MAP_SECTION.titleLine2}
          </h2>
          <p
            style={{ fontSize: 16, lineHeight: 1.6, color: palette.muted, margin: '0 0 22px' }}
          >
            {MAP_SECTION.body}
          </p>
          <div style={{ display: 'flex', gap: 22, flexWrap: 'wrap' }}>
            {MAP_SECTION.stats.map((s) => (
              <div key={s.label}>
                <div
                  className="disp"
                  style={{ fontSize: 26, fontWeight: 800, color: palette.primary }}
                >
                  {s.value}
                </div>
                <div style={{ fontSize: 12.5, color: '#7C8C7E' }}>{s.label}</div>
              </div>
            ))}
          </div>
        </div>

        <div
          style={{
            background: '#fff',
            border: `1px solid rgba(35,88,58,.1)`,
            borderRadius: 16,
            padding: 22,
          }}
        >
          {/* The right gutter the label lives in. Percentage padding, so the
              gutter keeps its ratio to the map and the arrow — expressed in map
              units — lands in it at every width. Collapsed under 760px by
              `.skc-map-figure`, where there is no horizontal room to give up. */}
          <div className="skc-map-figure" style={{ position: 'relative', width: '100%' }}>
            <div style={{ position: 'relative', width: '100%' }}>
            {/* Stage 2. `unoptimized` because this is a hand-prepared raster whose
                coordinates the traced border depends on — re-encoding it at a
                different size is exactly the transformation that would slide the
                outline off the country. */}
            <Image
              src="/assets/map-base.png"
              alt="Карта Таджикистана"
              width={710}
              height={446}
              unoptimized
              data-testid="map-fill"
              style={{
                display: 'block',
                width: '100%',
                height: 'auto',
                opacity: phase >= 2 ? 1 : 0,
                transition: `opacity ${instant ?? '.8s'} ease`,
              }}
            />

            {/* Stage 1. */}
            <svg
              viewBox={TAJIKISTAN_VIEWBOX}
              preserveAspectRatio="xMidYMid meet"
              aria-hidden
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                overflow: 'visible',
              }}
            >
              <path
                pathLength={1}
                d={TAJIKISTAN_BORDER_PATH}
                data-testid="map-border"
                style={{
                  fill: 'none',
                  stroke: palette.primaryHover,
                  strokeWidth: 2.2,
                  strokeLinejoin: 'round',
                  strokeLinecap: 'round',
                  strokeDasharray: 1,
                  strokeDashoffset: phase >= 1 ? 0 : 1,
                  transition: `stroke-dashoffset ${instant ?? '2s'} ease-in-out`,
                }}
              />
            </svg>

            {/* Stage 3. Positioned as a percentage of the image, so it stays on
                Khorog at every width. */}
            <Image
              src="/assets/map-heart.png"
              alt=""
              width={48}
              height={48}
              unoptimized
              data-testid="map-heart"
              style={{
                position: 'absolute',
                left: '54.37%',
                top: '75.56%',
                width: '6.76%',
                height: 'auto',
                opacity: phase >= 3 ? 1 : 0,
                transform: `translateY(${phase >= 3 ? '0' : '-18px'}) scale(${phase >= 3 ? 1 : 0.72})`,
                transformOrigin: '50% 100%',
                transition: instant
                  ? 'none'
                  : 'opacity .45s ease, transform .65s cubic-bezier(.34,1.45,.5,1)',
                pointerEvents: 'none',
              }}
            />
            {/* The arrow, drawn in the border's viewBox so it tracks the marker.
                `overflow: visible` is load-bearing: the tail sits at x=745,
                outside the 710-unit frame, which is the whole point. */}
            <svg
              viewBox={TAJIKISTAN_VIEWBOX}
              preserveAspectRatio="xMidYMid meet"
              aria-hidden
              data-testid="map-arrow"
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                overflow: 'visible',
                pointerEvents: 'none',
              }}
            >
              {[
                { a: ARROW, cls: 'skc-map-arrow-wide' },
                { a: ARROW_STACKED, cls: 'skc-map-arrow-stacked' },
              ].map(({ a, cls }) => (
                <g key={cls} className={cls}>
                  <path
                    pathLength={1}
                    d={`M${a.tailX},${a.tailY} L${a.baseX},${a.baseY}`}
                    style={{
                      fill: 'none',
                      stroke: palette.deep,
                      strokeWidth: 2.2,
                      strokeLinecap: 'round',
                      strokeDasharray: 1,
                      // Drawn from the label towards the marker, so the eye is
                      // led to Khorog rather than away from it.
                      strokeDashoffset: phase >= 3 ? 0 : 1,
                      transition: instant
                        ? 'none'
                        : 'stroke-dashoffset .55s ease .1s',
                    }}
                  />
                  <polygon
                    points={a.head}
                    style={{
                      fill: palette.deep,
                      opacity: phase >= 3 ? 1 : 0,
                      // Lands only once the shaft has finished drawing.
                      transition: instant ? 'none' : 'opacity .25s ease .6s',
                    }}
                  />
                </g>
              ))}
            </svg>

            {/* The label. Anchored to the arrow's tail — 418/446 of the way down
                the map — so the two stay joined however the card is sized. */}
            <div
              className="skc-map-callout"
              data-testid="map-callout"
              style={{
                // The entrance offset travels as a custom property, NOT as an
                // inline `transform`: the wide layout also needs translateY(-50%)
                // to sit on the arrow, and an inline transform would silently
                // win over the stylesheet and drop the centring.
                ['--skc-enter' as string]: phase >= 3 ? '0px' : '6px',
                opacity: phase >= 3 ? 1 : 0,
                transition: instant
                  ? 'none'
                  : 'opacity .5s ease .55s, transform .5s ease .55s',
                pointerEvents: 'none',
              }}
            >
              <span
                style={{
                  display: 'inline-block',
                  background: palette.deep,
                  color: '#fff',
                  fontSize: 13,
                  fontWeight: 700,
                  padding: '6px 13px',
                  borderRadius: 15,
                  whiteSpace: 'nowrap',
                  boxShadow: '0 6px 16px rgba(35,88,58,.28)',
                }}
              >
                Хорог
              </span>
            </div>
            </div>
          </div>
          <p
            style={{ fontSize: 11, color: '#96a191', textAlign: 'center', margin: '10px 0 0' }}
          >
            {MAP_SECTION.caption}
          </p>
        </div>
      </div>
    </section>
  );
}
