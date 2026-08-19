import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { ReactNode } from 'react';

import { AssemblyLine } from '@/components/AssemblyLine';
import { EcoBadges } from '@/components/home/EcoBadges';
import { RetailerMarquee } from '@/components/home/RetailerMarquee';
import { TajikistanMap } from '@/components/home/TajikistanMap';
import { TrustStrip } from '@/components/home/TrustStrip';
import { ECO_BADGES, RETAILERS, TRUST_ITEMS } from '@/lib/content';
import messages from '@/messages/ru.json';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The home sections added from the approved design.
 *
 * These tests are mostly about the design contract, not about React. The client
 * has already rejected alternatives for several of these — a regression here is
 * not a styling nit, it is shipping the variant they turned down.
 */

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      {node}
    </NextIntlClientProvider>,
  );
}

function stubMatchMedia(reducedMotion: boolean) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: reducedMotion && query.includes('prefers-reduced-motion'),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    onchange: null,
    dispatchEvent: () => false,
  }));
}

beforeEach(() => {
  vi.unstubAllGlobals();
  stubMatchMedia(false);
});

const PRODUCTS: PublicProduct[] = [
  {
    id: '1', sku: 'APJ-1000', idx: '01', name: 'Яблочный сок прямого отжима',
    short: 'Яблочный сок', line: 'Соки', accent: '#6FAE3E', tint: '#E8F1DA',
    volume: '1 000 мл', pack: 'Стеклянная бутылка',
    description: 'Сок прямого отжима в прозрачной стеклянной бутылке 1 000 мл.',
  },
  {
    id: '2', sku: 'APR-220', idx: '02', name: 'Абрикосовый джем',
    short: 'Абрикосовый джем', line: 'Джемы', accent: '#E79A3A', tint: '#FBEACB',
    volume: '212–228 мл', pack: 'Стеклянная банка', description: 'Абрикосовый джем.',
  },
  {
    id: '3', sku: 'WAT-500', idx: '03', name: 'Негазированная питьевая вода 0,5 л',
    short: 'Питьевая вода 0,5 л', line: 'Вода', accent: '#3FA3C4', tint: '#DAEDF4',
    volume: '500 мл', pack: 'ПЭТ-бутылка', description: 'Питьевая вода.',
  },
];

// ---------------------------------------------------------------------------
// Trust strip
// ---------------------------------------------------------------------------

describe('Trust strip', () => {
  it('shows four columns, not three', () => {
    wrap(<TrustStrip />);
    // An earlier pass shipped three with invented copy. The design's strip is a
    // 4-up grid and the fourth item is the packaging claim, which is the one a
    // wholesale buyer actually reads.
    expect(screen.getAllByTestId('trust-item')).toHaveLength(4);
    expect(TRUST_ITEMS).toHaveLength(4);
  });

  it('uses the approved copy verbatim', () => {
    wrap(<TrustStrip />);
    expect(screen.getByText('Гигиена и санитария')).toBeInTheDocument();
    expect(screen.getByText('Профессиональная упаковка')).toBeInTheDocument();
    expect(screen.getByText('Стекло и ПЭТ, современные форматы и укупорка.')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Retailer marquee
// ---------------------------------------------------------------------------

describe('Retailer marquee', () => {
  it('always carries the placeholder caption', () => {
    wrap(
      <RetailerMarquee
        heading={messages.home.retailersTitle}
        caption={messages.home.retailersCaption}
      />,
    );
    // These are invented retailer names for a factory that has not opened. Без
    // caption the strip reads as a claim about who stocks the product.
    expect(screen.getByTestId('retailer-caption')).toHaveTextContent(
      'Логотипы приведены для примера оформления',
    );
  });

  it('duplicates the list for a seamless loop but announces it once', () => {
    wrap(
      <RetailerMarquee
        heading={messages.home.retailersTitle}
        caption={messages.home.retailersCaption}
      />,
    );
    const marquee = screen.getByTestId('retailer-marquee');
    // Both copies are in the DOM — the CSS translate needs them — but the second
    // is aria-hidden so a screen reader does not read eight names twice.
    expect(marquee.children).toHaveLength(RETAILERS.length * 2);
    // Direct children only: the chorkhona glyph inside each chip is aria-hidden
    // too, and an unscoped query would count those as well.
    const hiddenChips = Array.from(marquee.children).filter(
      (el) => el.getAttribute('aria-hidden') === 'true',
    );
    expect(hiddenChips).toHaveLength(RETAILERS.length);
  });
});

// ---------------------------------------------------------------------------
// Eco badges
// ---------------------------------------------------------------------------

describe('Eco badges', () => {
  it('renders the three approved badges', () => {
    wrap(<EcoBadges />);
    expect(screen.getAllByTestId('eco-badge')).toHaveLength(3);
    for (const badge of ECO_BADGES) {
      expect(screen.getByText(badge.title)).toBeInTheDocument();
    }
  });

  it('makes no claim the client has not approved', () => {
    wrap(<EcoBadges />);
    // The badges say what the product does NOT contain, which the client stands
    // behind. Nothing here asserts a composition, a nutritional value or a
    // certification — those stay «уточняется» until the lab has verified them.
    const text = document.body.textContent ?? '';
    expect(text).not.toMatch(/сертифицирован|ГОСТ|органик|ISO/i);
  });
});

// ---------------------------------------------------------------------------
// The animated map
// ---------------------------------------------------------------------------

describe('Tajikistan map', () => {
  it('renders the finished map where IntersectionObserver is unavailable', async () => {
    wrap(<TajikistanMap />);
    // jsdom has no IntersectionObserver. The component falls through to the
    // final phase rather than leaving an invisible map — a map nobody can see is
    // a worse failure than a missing animation.
    await waitFor(() => {
      expect(screen.getByTestId('map-section')).toHaveAttribute('data-phase', '3');
    });
  });

  it('draws the border with a normalised path length', async () => {
    wrap(<TajikistanMap />);
    const border = await screen.findByTestId('map-border');
    // pathLength="1" makes the dash maths independent of the traced path's real
    // length, which is 6 000 characters of coordinates.
    expect(border).toHaveAttribute('pathLength', '1');
    expect(border.getAttribute('d')?.startsWith('M277,48')).toBe(true);
  });

  it('positions the marker on Khorog as a percentage, so it holds at any width', async () => {
    wrap(<TajikistanMap />);
    const heart = await screen.findByTestId('map-heart');
    expect(heart.style.left).toBe('54.37%');
    expect(heart.style.top).toBe('75.56%');
  });

  it('shows the region facts alongside the map', () => {
    wrap(<TajikistanMap />);
    expect(screen.getByText('≈2 200 м')).toBeInTheDocument();
    expect(screen.getByText('над уровнем моря')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The product quick-look
// ---------------------------------------------------------------------------

describe('Product quick-look', () => {
  it('opens from a slot on the belt rather than navigating away', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await userEvent.click(await screen.findByText('Яблочный сок'));

    const modal = await screen.findByTestId('product-modal');
    expect(within(modal).getByText('Яблочный сок прямого отжима')).toBeInTheDocument();
    // The full page is one click further, not the first thing that happens.
    expect(within(modal).getByRole('link', { name: messages.cta.learnMore })).toHaveAttribute(
      'href',
      expect.stringContaining('/catalogue/APJ-1000'),
    );
  });

  it('is announced as a dialog and takes focus', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await userEvent.click(await screen.findByText('Абрикосовый джем'));

    const modal = await screen.findByTestId('product-modal');
    expect(modal).toHaveAttribute('aria-modal', 'true');
    expect(modal).toHaveAttribute('aria-labelledby', 'product-modal-title');
    // Focus lands on the close button, so the first Tab is inside the dialog.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: messages.a11y.close })).toHaveFocus();
    });
  });

  it('closes on Escape and returns focus to the slot that opened it', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    const opener = await screen.findByText('Яблочный сок');
    await userEvent.click(opener);
    await screen.findByTestId('product-modal');

    await userEvent.keyboard('{Escape}');
    await waitFor(() => {
      expect(screen.queryByTestId('product-modal')).not.toBeInTheDocument();
    });
    // Back where they were, not at the top of a long page.
    await waitFor(() => {
      expect(opener.closest('button')).toHaveFocus();
    });
  });

  it('closes on a backdrop click but not on a click inside the panel', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await userEvent.click(await screen.findByText('Яблочный сок'));
    const modal = await screen.findByTestId('product-modal');

    // Selecting text in the description must not dismiss it.
    await userEvent.click(within(modal).getByText(/Сок прямого отжима/));
    expect(screen.getByTestId('product-modal')).toBeInTheDocument();

    await userEvent.click(screen.getByTestId('product-modal-backdrop'));
    await waitFor(() => {
      expect(screen.queryByTestId('product-modal')).not.toBeInTheDocument();
    });
  });

  it('shows specifications without inventing any', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await userEvent.click(await screen.findByText('Питьевая вода 0,5 л'));
    const modal = await screen.findByTestId('product-modal');

    expect(within(modal).getByText('WAT-500')).toBeInTheDocument();
    // Water is PET with a screw closure; the design derives both from the line
    // rather than storing them, and the modal must agree with the page.
    expect(within(modal).getByText('ПЭТ')).toBeInTheDocument();
    expect(within(modal).getByText('Винтовая пробка')).toBeInTheDocument();
  });

  it('locks the page behind it and releases the lock on close', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await userEvent.click(await screen.findByText('Яблочный сок'));
    await screen.findByTestId('product-modal');
    expect(document.body.style.overflow).toBe('hidden');

    await userEvent.keyboard('{Escape}');
    await waitFor(() => {
      expect(document.body.style.overflow).not.toBe('hidden');
    });
  });
});
