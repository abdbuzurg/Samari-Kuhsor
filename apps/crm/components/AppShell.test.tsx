import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import type { ReactNode } from 'react';

import { AppShell } from './AppShell';
import { server, session, warehouseUser, adminUser, noRoleUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * CLAUDE.md §7 requires every React data component to be tested in four states:
 * loading, empty, error and populated. The shell is the first such component and
 * the one every screen depends on, so all four are covered here.
 */

let client: QueryClient;

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
});

function renderShell(children: ReactNode = <p>содержимое</p>) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <QueryClientProvider client={client}>
        <AppShell>{children}</AppShell>
      </QueryClientProvider>
    </NextIntlClientProvider>,
  );
}

describe('AppShell — the four required states', () => {
  it('loading: shows a status region while the session resolves', () => {
    server.use(session.loading());
    renderShell();
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.queryByTestId('sidebar')).not.toBeInTheDocument();
  });

  it('populated: renders the shell and the page content', async () => {
    server.use(session.loaded());
    renderShell();

    expect(await screen.findByTestId('sidebar')).toBeInTheDocument();
    expect(screen.getByTestId('topbar')).toBeInTheDocument();
    expect(screen.getByText('содержимое')).toBeInTheDocument();
    expect(screen.getByText(warehouseUser.full_name)).toBeInTheDocument();
    // Role names are content: the chip shows the Russian display name, not the key.
    expect(screen.getByText('Склад')).toBeInTheDocument();
    expect(screen.queryByText('warehouse')).not.toBeInTheDocument();
  });

  it('error: an unauthenticated session offers the login page instead of an error', async () => {
    server.use(session.unauthenticated());
    renderShell();

    expect(await screen.findByRole('button', { name: /Войти/ })).toBeInTheDocument();
    expect(screen.queryByTestId('sidebar')).not.toBeInTheDocument();
  });

  it('error: a server failure does not render the shell', async () => {
    server.use(session.serverError());
    renderShell();

    expect(await screen.findByRole('button', { name: /Войти/ })).toBeInTheDocument();
  });

  // Not theoretical: an administrator can create a user and forget to assign a
  // role. That user must get an explanation, not an empty sidebar and a blank page.
  it('empty: a user with no roles is told why they can see nothing', async () => {
    server.use(session.loaded(noRoleUser));
    renderShell();

    expect(await screen.findByText(/Доступ не настроен/)).toBeInTheDocument();
    expect(screen.queryByText('содержимое')).not.toBeInTheDocument();
  });
});

describe('AppShell — permission-driven navigation', () => {
  // docs/05-MODULES.md:25 — "A user with no permission on a module never sees it."
  it('hides modules the user cannot read', async () => {
    server.use(session.loaded(warehouseUser));
    renderShell();

    await screen.findByTestId('sidebar');
    const nav = screen.getByTestId('sidebar');

    // Склад may read these.
    expect(within(nav, 'Склад и запасы')).toBe(true);
    expect(within(nav, 'Товары и цены')).toBe(true);
    expect(within(nav, 'Качество и безопасность')).toBe(true);

    // And not these: no crm, hr or inquiries grant in the seed matrix.
    expect(within(nav, 'CRM и продажи')).toBe(false);
    expect(within(nav, 'Персонал')).toBe(false);
    expect(within(nav, 'Интеграция с сайтом')).toBe(false);
  });

  it('shows an administrator everything except the deferred finance module', async () => {
    server.use(session.loaded(adminUser));
    renderShell();

    await screen.findByTestId('sidebar');
    const nav = screen.getByTestId('sidebar');

    expect(within(nav, 'CRM и продажи')).toBe(true);
    expect(within(nav, 'Персонал')).toBe(true);

    // D2: Финансы и бюджет is deferred pending register question Q2 and must be
    // hidden even from an administrator (docs/05-MODULES.md:22).
    expect(within(nav, 'Финансы и бюджет')).toBe(false);
  });

  it('never renders a nav entry for finance, whatever the permissions say', async () => {
    server.use(
      session.loaded({ ...adminUser, permissions: [...adminUser.permissions, 'finance:manage'] }),
    );
    renderShell();

    await screen.findByTestId('sidebar');
    expect(within(screen.getByTestId('sidebar'), 'Финансы и бюджет')).toBe(false);
  });
});

describe('AppShell — layout invariants', () => {
  // CLAUDE.md §5: the sidebar is 252px and its brand block and the top bar are
  // both exactly 64px, so the divider is continuous across the seam. These were
  // fixed in response to review and must not regress
  // (HANDOFF-CRM-CONTEXT.md:163).
  it('keeps the sidebar at 252px and the chrome blocks at 64px', async () => {
    server.use(session.loaded());
    renderShell();

    const sidebar = await screen.findByTestId('sidebar');
    expect(sidebar.className).toContain('w-[252px]');

    const brand = sidebar.firstElementChild!;
    expect(brand.className).toContain('h-16'); // 64px

    const topbar = screen.getByTestId('topbar');
    expect(topbar.className).toContain('h-16'); // 64px — same seam
  });

  it('fills the chrome with the branded green, the only decorative use', async () => {
    server.use(session.loaded());
    renderShell();

    const sidebar = await screen.findByTestId('sidebar');
    expect(sidebar.getAttribute('style')).toContain('var(--sk-chrome)');
    expect(screen.getByTestId('topbar').getAttribute('style')).toContain('var(--sk-chrome)');
  });
});

/** Whether a nav label is present inside an element. */
function within(container: HTMLElement, label: string): boolean {
  return Array.from(container.querySelectorAll('a')).some(
    (a) => a.textContent?.includes(label) ?? false,
  );
}
