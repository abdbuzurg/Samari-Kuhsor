import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { ReactNode } from 'react';

import { AssemblyLine } from '@/components/AssemblyLine';
import { CatalogueGrid } from '@/components/CatalogueGrid';
import { ContactForm } from '@/components/ContactForm';
import { ConsentBanner } from '@/components/ConsentBanner';
import { Analytics } from '@/components/Analytics';
import { server, http, HttpResponse } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The public site.
 *
 * These tests protect the things CLAUDE.md §5 says the client already rejected
 * alternatives for. Reproducing the approved design is the requirement; a
 * regression here is not a styling nit, it is shipping the variant the client
 * turned down.
 */

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      {node}
    </NextIntlClientProvider>,
  );
}

const PRODUCTS: PublicProduct[] = [
  {
    id: '1', sku: 'APJ-1000', idx: '01', name: 'Яблочный сок прямого отжима',
    short: 'Яблочный сок', line: 'Соки', accent: '#6FAE3E', tint: '#E8F1DA',
    volume: '1 000 мл', pack: 'Стеклянная бутылка', description: 'Сок прямого отжима.',
  },
  {
    id: '2', sku: 'APR-220', idx: '02', name: 'Абрикосовый джем',
    short: 'Абрикосовый джем', line: 'Джемы', accent: '#E79A3A', tint: '#FBEACB',
    volume: '212–228 мл', pack: 'Стеклянная банка', description: 'Абрикосовый джем.',
  },
  {
    id: '3', sku: 'TOM-500', idx: '03', name: 'Томатная паста',
    short: 'Томатная паста', line: 'Паста', accent: '#D6533B', tint: '#F7DCD3',
    volume: '500 мл', pack: 'Стеклянная банка', description: 'Томатная паста.',
  },
  {
    id: '4', sku: 'WAT-500', idx: '04', name: 'Негазированная питьевая вода 0,5 л',
    short: 'Питьевая вода 0,5 л', line: 'Вода', accent: '#3FA3C4', tint: '#DAEDF4',
    volume: '500 мл', pack: 'ПЭТ-бутылка', description: 'Питьевая вода 0,5 л.',
  },
  {
    id: '5', sku: 'WAT-1000', idx: '05', name: 'Негазированная питьевая вода 1 л',
    short: 'Питьевая вода 1 л', line: 'Вода', accent: '#3FA3C4', tint: '#DAEDF4',
    volume: '1 000 мл', pack: 'ПЭТ-бутылка', description: 'Питьевая вода 1 л.',
  },
];

/** jsdom has no IntersectionObserver. The component falls back to "parked", and
 *  these tests exercise that path; the animation itself is CSS. */
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
  window.localStorage.clear();
});

// ---------------------------------------------------------------------------
// The assembly line — v1, the approved animation
// ---------------------------------------------------------------------------

describe('Assembly line', () => {
  it('shows four products at a time and pages between batches', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);

    // Four per batch (CLAUDE.md §5 — "batch buttons page between sets of four").
    expect(await screen.findByText('Яблочный сок')).toBeInTheDocument();
    expect(screen.getByText('Питьевая вода 0,5 л')).toBeInTheDocument();
    expect(screen.queryByText('Питьевая вода 1 л')).not.toBeInTheDocument();
    expect(screen.getByText('01 / 02')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Следующая партия' }));
    await waitFor(() => {
      expect(screen.getByText('Питьевая вода 1 л')).toBeInTheDocument();
    });
    expect(screen.queryByText('Яблочный сок')).not.toBeInTheDocument();
    expect(screen.getByText('02 / 02')).toBeInTheDocument();
  });

  it('wraps around rather than dead-ending at the last batch', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await screen.findByText('Яблочный сок');

    await userEvent.click(screen.getByRole('button', { name: 'Предыдущая партия' }));
    await waitFor(() => {
      expect(screen.getByText('02 / 02')).toBeInTheDocument();
    });
  });

  it('disables the batch buttons when everything fits in one batch', async () => {
    wrap(<AssemblyLine products={PRODUCTS.slice(0, 3)} />);
    await screen.findByText('Яблочный сок');
    expect(screen.getByRole('button', { name: 'Следующая партия' })).toBeDisabled();
    expect(screen.getByText('01 / 01')).toBeInTheDocument();
  });

  it('rolls products in from the left, staggered ~150ms', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    const first = (await screen.findByText('Яблочный сок')).closest('li') as HTMLElement;
    const second = screen.getByText('Абрикосовый джем').closest('li') as HTMLElement;

    // skcRoll is the approved v1 keyframe: roll in from the left and park. A
    // continuous-loop v2 was built and rejected (CLAUDE.md §5).
    expect(first.style.animation).toContain('skcRoll');
    expect(first.style.animation).toContain('0.00s');
    expect(second.style.animation).toContain('0.15s');
  });

  it('degrades to a fade under prefers-reduced-motion', async () => {
    stubMatchMedia(true);
    wrap(<AssemblyLine products={PRODUCTS} />);
    const first = (await screen.findByText('Яблочный сок')).closest('li') as HTMLElement;

    await waitFor(() => {
      expect(first.style.animation).toContain('skcFade');
    });
    expect(first.style.animation).not.toContain('skcRoll');
  });

  it('stays horizontal — the slots are a row, never a vertical list', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    await screen.findByText('Яблочный сок');
    const list = screen.getByText('Яблочный сок').closest('ul') as HTMLElement;

    // CLAUDE.md §5: "The assembly line stays horizontal and swipeable on mobile.
    // It must never become a vertical list." The .skc-slots class carries the
    // prototype's own mobile rule, which turns this into a scroll-snapping row.
    expect(list.className).toContain('skc-slots');
    expect(list.style.display).toBe('grid');
    expect(list.style.gridTemplateColumns).toContain('repeat(4');
  });

  it('links each slot to its product page', async () => {
    wrap(<AssemblyLine products={PRODUCTS} />);
    const link = (await screen.findByText('Яблочный сок')).closest('a') as HTMLAnchorElement;
    expect(link.getAttribute('href')).toContain('/catalogue/APJ-1000');
  });
});

// ---------------------------------------------------------------------------
// Catalogue
// ---------------------------------------------------------------------------

describe('Catalogue', () => {
  it('renders every product', () => {
    wrap(<CatalogueGrid products={PRODUCTS} />);
    expect(screen.getAllByTestId('catalogue-card')).toHaveLength(5);
  });

  it('filters by line', async () => {
    wrap(<CatalogueGrid products={PRODUCTS} />);
    await userEvent.click(screen.getByRole('button', { name: 'Вода' }));
    expect(screen.getAllByTestId('catalogue-card')).toHaveLength(2);
    expect(screen.queryByText('Томатная паста')).not.toBeInTheDocument();
  });

  it('searches by name and by SKU', async () => {
    wrap(<CatalogueGrid products={PRODUCTS} />);
    const search = screen.getByLabelText(messages.catalogue.searchLabel);

    await userEvent.type(search, 'джем');
    expect(screen.getAllByTestId('catalogue-card')).toHaveLength(1);

    await userEvent.clear(search);
    // Wholesale buyers quote the SKU over the phone, so it is searchable.
    await userEvent.type(search, 'WAT-1000');
    expect(screen.getAllByTestId('catalogue-card')).toHaveLength(1);
    expect(screen.getByText('Негазированная питьевая вода 1 л')).toBeInTheDocument();
  });

  it('says nothing was found rather than showing an empty grid', async () => {
    wrap(<CatalogueGrid products={PRODUCTS} />);
    await userEvent.type(screen.getByLabelText(messages.catalogue.searchLabel), 'ничего');
    expect(screen.getByTestId('catalogue-no-match')).toBeInTheDocument();
    expect(screen.queryByTestId('catalogue-card')).not.toBeInTheDocument();
  });

  it('marks the active filter for assistive technology', async () => {
    wrap(<CatalogueGrid products={PRODUCTS} />);
    expect(screen.getByRole('button', { name: 'Все' })).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(screen.getByRole('button', { name: 'Соки' }));
    expect(screen.getByRole('button', { name: 'Соки' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Все' })).toHaveAttribute('aria-pressed', 'false');
  });
});

// ---------------------------------------------------------------------------
// The enquiry form
// ---------------------------------------------------------------------------

describe('Contact form', () => {
  it('shows the reference number after a successful submission', async () => {
    server.use(
      http.post('/api/inquiries', () =>
        HttpResponse.json({ data: { reference_no: 'WR-0001' } }, { status: 201 }),
      ),
    );
    wrap(<ContactForm />);

    await userEvent.type(screen.getByLabelText(/Имя/), 'Ориён Маркет');
    await userEvent.type(screen.getByLabelText(/Телефон или e-mail/), '+992 000 00 00');
    await userEvent.click(screen.getByRole('button', { name: messages.contact.submit }));

    const sent = await screen.findByTestId('contact-sent');
    // The reference number is the visitor's only receipt and the thing QOIM
    // will ask for when they call.
    expect(sent).toHaveTextContent('WR-0001');
    // And the form is gone, so nobody submits twice by accident.
    expect(screen.queryByRole('button', { name: messages.contact.submit })).not.toBeInTheDocument();
  });

  it('shows field errors against the fields the server named', async () => {
    server.use(
      http.post('/api/inquiries', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Проверьте форму',
              details: [{ field: 'contact', code: 'required', message: 'Укажите контакт' }],
            },
          },
          { status: 422 },
        ),
      ),
    );
    wrap(<ContactForm />);

    await userEvent.type(screen.getByLabelText(/Имя/), 'Тест');
    await userEvent.click(screen.getByRole('button', { name: messages.contact.submit }));

    expect(await screen.findByTestId('error-contact')).toHaveTextContent('Укажите контакт');
    // The form stays on screen so the visitor can fix it.
    expect(screen.getByRole('button', { name: messages.contact.submit })).toBeInTheDocument();
  });

  it('shows one general message when the server names no field', async () => {
    server.use(
      http.post('/api/inquiries', () =>
        HttpResponse.json(
          { error: { code: 'rate_limited', message: 'Слишком много обращений' } },
          { status: 429 },
        ),
      ),
    );
    wrap(<ContactForm />);
    await userEvent.type(screen.getByLabelText(/Имя/), 'Тест');
    await userEvent.type(screen.getByLabelText(/Телефон или e-mail/), 'x');
    await userEvent.click(screen.getByRole('button', { name: messages.contact.submit }));

    expect(await screen.findByTestId('contact-error')).toBeInTheDocument();
    // It does not invent a per-field error for a rate limit.
    expect(screen.queryByTestId('error-name')).not.toBeInTheDocument();
  });

  it('offers every enquiry type the backend accepts', () => {
    wrap(<ContactForm />);
    const select = screen.getByLabelText(messages.contact.type) as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    // The five types in docs/05-MODULES.md:160. A type the frontend cannot send
    // is a type the client cannot receive.
    expect(values).toEqual(['wholesale', 'distributor', 'contact', 'complaint', 'job']);
  });

  it('sends the trimmed payload and omits empty optional fields', async () => {
    let body: Record<string, unknown> | null = null;
    server.use(
      http.post('/api/inquiries', async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ data: { reference_no: 'CF-0001' } }, { status: 201 });
      }),
    );
    wrap(<ContactForm />);

    await userEvent.type(screen.getByLabelText(/Имя/), '  Гость  ');
    await userEvent.type(screen.getByLabelText(/Телефон или e-mail/), 'x@example.tj');
    await userEvent.click(screen.getByRole('button', { name: messages.contact.submit }));
    await screen.findByTestId('contact-sent');

    // An empty company is null, not "" — the column is nullable and an empty
    // string would look like a company whose name is blank.
    expect(body).toMatchObject({ company: null, message: null, type: 'wholesale' });
  });
});

// ---------------------------------------------------------------------------
// Consent and analytics
// ---------------------------------------------------------------------------

describe('Analytics consent', () => {
  it('shows the banner on a first visit', () => {
    wrap(<ConsentBanner />);
    expect(screen.getByTestId('consent-banner')).toBeInTheDocument();
  });

  it('offers accept and decline as equal choices', () => {
    wrap(<ConsentBanner />);
    const banner = screen.getByTestId('consent-banner');
    expect(within(banner).getByRole('button', { name: messages.consent.accept })).toBeInTheDocument();
    // A banner with only "accept" is not consent. Both must be present.
    expect(within(banner).getByRole('button', { name: messages.consent.decline })).toBeInTheDocument();
  });

  it('stops showing once a choice is made, in either direction', async () => {
    const { unmount } = wrap(<ConsentBanner />);
    await userEvent.click(screen.getByRole('button', { name: messages.consent.decline }));
    await waitFor(() => {
      expect(screen.queryByTestId('consent-banner')).not.toBeInTheDocument();
    });
    unmount();

    // And it stays gone on the next page load — "declined" is remembered, not
    // re-asked, which is the difference between consent and nagging.
    wrap(<ConsentBanner />);
    await waitFor(() => {
      expect(screen.queryByTestId('consent-banner')).not.toBeInTheDocument();
    });
  });

  it('loads no analytics script before consent is granted', async () => {
    vi.stubEnv('NEXT_PUBLIC_MATOMO_URL', 'https://analytics.samari-kuhsor.tj');
    vi.stubEnv('NEXT_PUBLIC_MATOMO_SITE_ID', '1');

    const { unmount } = wrap(<Analytics />);
    // Nothing rendered: no request to the analytics host at all, which is
    // stronger than loading the tracker with an opt-out flag set.
    expect(document.querySelector('[data-testid="matomo"]')).toBeNull();
    unmount();

    window.localStorage.setItem('samari_analytics_consent', 'denied');
    wrap(<Analytics />);
    await waitFor(() => {
      expect(document.querySelector('[data-testid="matomo"]')).toBeNull();
    });
  });

  it('renders nothing when no Matomo host is configured, even with consent', async () => {
    vi.stubEnv('NEXT_PUBLIC_MATOMO_URL', '');
    window.localStorage.setItem('samari_analytics_consent', 'granted');
    wrap(<Analytics />);
    await waitFor(() => {
      expect(document.querySelector('[data-testid="matomo"]')).toBeNull();
    });
  });
});
