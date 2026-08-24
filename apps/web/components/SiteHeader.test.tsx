import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect } from 'vitest';
import type { ReactNode } from 'react';

import { SiteHeader } from '@/components/SiteHeader';
import messages from '@/messages/ru.json';

/**
 * The header's mobile drawer.
 *
 * The header row needs about 1080px — logo, four nav items, three locale links
 * and the CTA. At 393px it was 1066px wide and forced EVERY page on the site to
 * scroll sideways. That was the single largest mobile defect and it was present
 * on every route.
 *
 * What must not change is the contract (CLAUDE.md §5): four nav items, and the
 * language switcher as ТҶ / РУ / EN in that order. The drawer moves them; it
 * does not reorder or drop any of them.
 *
 * Which of the two is VISIBLE is decided by a CSS media query, not by
 * JavaScript — so it works before hydration and never flashes the wrong one.
 * jsdom does not evaluate media queries, so these tests assert the markup and
 * behaviour; the widths themselves are covered by the Playwright mobile spec.
 */

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      {node}
    </NextIntlClientProvider>,
  );
}

describe('SiteHeader — the mobile drawer', () => {
  it('renders no drawer until it is asked for', () => {
    wrap(<SiteHeader />);
    expect(screen.queryByTestId('site-drawer')).not.toBeInTheDocument();
    expect(screen.getByTestId('site-burger')).toHaveAttribute('aria-expanded', 'false');
  });

  it('opens and closes from the same control', async () => {
    wrap(<SiteHeader />);
    const burger = screen.getByTestId('site-burger');

    await userEvent.click(burger);
    expect(screen.getByTestId('site-drawer')).toBeInTheDocument();
    expect(burger).toHaveAttribute('aria-expanded', 'true');

    await userEvent.click(burger);
    expect(screen.queryByTestId('site-drawer')).not.toBeInTheDocument();
  });

  it('carries all four nav items, in the order the design fixes', async () => {
    wrap(<SiteHeader />);
    await userEvent.click(screen.getByTestId('site-burger'));

    const drawer = within(screen.getByTestId('site-drawer'));
    const labels = drawer.getAllByRole('link').map((a) => a.textContent?.trim());
    expect(labels.slice(0, 4)).toEqual([
      messages.nav.home,
      messages.nav.catalogue,
      messages.nav.production,
      messages.nav.contact,
    ]);
  });

  it('carries the language switcher as ТҶ / РУ / EN', async () => {
    wrap(<SiteHeader />);
    await userEvent.click(screen.getByTestId('site-burger'));

    const drawer = within(screen.getByTestId('site-drawer'));
    const group = within(drawer.getByRole('group'));
    // The order is part of the visual contract, not a preference.
    expect(group.getAllByRole('link').map((a) => a.textContent?.trim())).toEqual([
      'ТҶ',
      'РУ',
      'EN',
    ]);
  });

  it('keeps the distributor CTA rather than dropping it on small screens', async () => {
    wrap(<SiteHeader />);
    await userEvent.click(screen.getByTestId('site-burger'));

    const drawer = within(screen.getByTestId('site-drawer'));
    expect(drawer.getByText(messages.cta.distributors)).toBeInTheDocument();
  });

  it('closes when a module is chosen, so it never covers the page it opened', async () => {
    wrap(<SiteHeader />);
    await userEvent.click(screen.getByTestId('site-burger'));

    const drawer = within(screen.getByTestId('site-drawer'));
    await userEvent.click(drawer.getByText(messages.nav.catalogue));

    // The most common mobile-menu defect is a drawer left sitting over the
    // destination.
    expect(screen.queryByTestId('site-drawer')).not.toBeInTheDocument();
  });

  it('closes when the language is switched', async () => {
    wrap(<SiteHeader />);
    await userEvent.click(screen.getByTestId('site-burger'));

    const drawer = within(screen.getByTestId('site-drawer'));
    await userEvent.click(within(drawer.getByRole('group')).getByText('ТҶ'));

    expect(screen.queryByTestId('site-drawer')).not.toBeInTheDocument();
  });

  it('still renders the desktop nav — the drawer is additive, not a replacement', () => {
    wrap(<SiteHeader />);
    // Both exist in the markup; CSS decides which one is shown. Removing the
    // desktop nav from the DOM would break the approved layout above 1024px.
    expect(document.querySelector('.site-nav-desktop')).toBeTruthy();
    expect(document.querySelector('.site-locale-desktop')).toBeTruthy();
    expect(document.querySelector('.site-cta-desktop')).toBeTruthy();
  });
});
