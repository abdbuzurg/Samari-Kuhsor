import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import ItemsPage from '@/app/items/page';
import { ItemForm, emptyValues, toPayload, valuesFromItem } from '@/components/ItemForm';
import { StatusTag } from '@/components/StatusTag';
import { ApiError } from '@/lib/session';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { Item, ItemListRow } from '@samari/types';

/**
 * CLAUDE.md §7 — every React data component is tested in four states: loading,
 * empty, error and populated. The list, the status tag and the edit form are the
 * three pieces the next eleven modules copy, so each is covered here.
 */

let client: QueryClient;

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
});

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </NextIntlClientProvider>,
  );
}

const APPLE: ItemListRow = {
  id: '018f3c9e-0000-7000-8000-000000000001',
  sku: 'APJ-1000',
  name: 'Яблочный сок прямого отжима',
  item_type: 'finished_good',
  category: 'juice',
  base_uom: 'bottle',
  packaging_codes: ['BOTTLE', 'CASE12'],
  shelf_life_days: null,
  current_price: { amount: '18.50', currency: 'TJS', valid_from: '2026-09-09', valid_to: null },
  status: { key: 'active', label: 'Активен', level: 'ok' },
  version: 1,
};

const WATER: ItemListRow = {
  ...APPLE,
  id: '018f3c9e-0000-7000-8000-000000000002',
  sku: 'WAT-500',
  name: 'Негазированная питьевая вода 0,5 л',
  category: 'water',
  packaging_codes: [],
  current_price: null,
  status: { key: 'draft', label: 'Черновик', level: 'neutral' },
};

const items = {
  loaded: (rows: ItemListRow[] = [APPLE, WATER]) =>
    http.get('/api/items', () =>
      HttpResponse.json({
        data: rows,
        meta: { page: 1, per_page: 50, total: rows.length, total_pages: 1 },
      }),
    ),
  loading: () =>
    http.get('/api/items', async () => {
      await delay('infinite');
      return HttpResponse.json({ data: [] });
    }),
  failing: () =>
    http.get('/api/items', () =>
      HttpResponse.json(
        { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
        { status: 500 },
      ),
    ),
};

// ---------------------------------------------------------------------------
// The list, in all four states
// ---------------------------------------------------------------------------

describe('Товары list — the four required states', () => {
  it('loading: shows a status region, not an empty table', async () => {
    server.use(session.loaded(adminUser), items.loading());
    wrap(<ItemsPage />);
    expect(await screen.findByText('Загрузка…')).toBeInTheDocument();
  });

  it('populated: renders the prototype columns and the rows', async () => {
    server.use(session.loaded(adminUser), items.loaded());
    wrap(<ItemsPage />);

    // Columns from docs/05-MODULES.md:85.
    for (const header of ['SKU', 'Наименование', 'Категория', 'Упаковка', 'Цена', 'Срок годн.', 'Статус']) {
      expect(await screen.findByRole('columnheader', { name: header })).toBeInTheDocument();
    }
    expect(await screen.findByText('APJ-1000')).toBeInTheDocument();
    expect(screen.getByText('Яблочный сок прямого отжима')).toBeInTheDocument();
    expect(screen.getAllByTestId('list-row')).toHaveLength(2);
  });

  it('error: reports the failure instead of pretending the catalogue is empty', async () => {
    server.use(session.loaded(adminUser), items.failing());
    wrap(<ItemsPage />);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Не удалось загрузить/);
  });

  it('empty: invites the first product rather than showing a blank table', async () => {
    server.use(session.loaded(adminUser), items.loaded([]));
    wrap(<ItemsPage />);
    expect(await screen.findByText('Товаров пока нет')).toBeInTheDocument();
  });
});

// An empty collection and a filtered-to-nothing one are different situations.
// "Add your first product" is unhelpful when the answer is "clear the search box".
it('distinguishes "no products" from "no matches"', async () => {
  server.use(
    session.loaded(adminUser),
    http.get('/api/items', ({ request }) => {
      const q = new URL(request.url).searchParams.get('q');
      const data = q ? [] : [APPLE, WATER];
      return HttpResponse.json({
        data,
        meta: { page: 1, per_page: 50, total: data.length, total_pages: 1 },
      });
    }),
  );
  wrap(<ItemsPage />);
  await screen.findByText('APJ-1000');

  await userEvent.type(screen.getByLabelText('Поиск по артикулу и наименованию'), 'нетнет');

  expect(await screen.findByText(/Ничего не найдено по запросу/)).toBeInTheDocument();
  expect(screen.queryByText('Товаров пока нет')).not.toBeInTheDocument();
});

// docs/02-SCHEMA.md:176 — the client's rule is that the system must not publish
// unverified claims. A blank cell reads as "no shelf life"; «уточняется» reads as
// "not yet determined", which is the truth.
it('renders «уточняется» for values that are not lab-verified', async () => {
  server.use(session.loaded(adminUser), items.loaded([WATER]));
  wrap(<ItemsPage />);

  await screen.findByText('WAT-500');
  const row = screen.getByTestId('list-row');
  // shelf_life_days, packaging and price are all absent on this fixture.
  expect(row).toHaveTextContent('уточняется');
});

it('formats money as the string it received, never through a float', async () => {
  server.use(session.loaded(adminUser), items.loaded([APPLE]));
  wrap(<ItemsPage />);

  await screen.findByText('APJ-1000');
  // Asserted on the row's text rather than a single node: JSX splits
  // `{amount} c.` across two text nodes, and matching the concatenation is what
  // actually proves the string survived unrounded.
  expect(screen.getByTestId('list-row')).toHaveTextContent('18.50 c.');
  // The trailing zero is the point: 18.5 would mean it went through a Number.
  expect(screen.getByTestId('list-row').textContent).toContain('18.50');
});

// docs/04-RBAC.md:120 — React hides the button; the server still enforces.
describe('Товары list — permission-driven affordances', () => {
  it('shows Создать to a user with items:manage', async () => {
    server.use(session.loaded(adminUser), items.loaded());
    wrap(<ItemsPage />);
    expect(await screen.findByRole('button', { name: /Создать/ })).toBeInTheDocument();
  });

  it('hides Создать from a user with items:read only', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['items:read'] }),
      items.loaded(),
    );
    wrap(<ItemsPage />);
    await screen.findByText('APJ-1000');
    expect(screen.queryByRole('button', { name: /Создать/ })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// StatusTag — the design contract
// ---------------------------------------------------------------------------

describe('StatusTag', () => {
  // CLAUDE.md §5: green means healthy, never merely branded. The backend decides
  // the level; this component must never map a status key to a colour itself.
  it('takes its colour from level, not from the status key', () => {
    render(<StatusTag status={{ key: 'released', label: 'Выпущено', level: 'ok' }} />);
    expect(screen.getByTestId('status-tag')).toHaveClass('tag-ok');

    render(<StatusTag status={{ key: 'quarantine', label: 'Карантин', level: 'danger' }} />);
    expect(screen.getAllByTestId('status-tag')[1]).toHaveClass('tag-danger');
  });

  it('maps every documented level to the prototype class', () => {
    const expected: Record<string, string> = {
      ok: 'tag-ok',
      warn: 'tag-warn',
      danger: 'tag-danger',
      info: 'tag-info',
      neutral: 'tag-neutral',
    };
    for (const [level, className] of Object.entries(expected)) {
      const { unmount } = render(<StatusTag status={{ key: 'x', label: 'X', level }} />);
      expect(screen.getByTestId('status-tag')).toHaveClass(className);
      unmount();
    }
  });

  // A status added server-side should look plain, not broken.
  it('falls back to neutral for an unknown level', () => {
    render(<StatusTag status={{ key: 'x', label: 'X', level: 'chartreuse' }} />);
    expect(screen.getByTestId('status-tag')).toHaveClass('tag-neutral');
  });

  it('prefers a caller-supplied label over the payload fallback', () => {
    render(<StatusTag status={{ key: 'active', label: 'Активен', level: 'ok' }} label="Фаъол" />);
    expect(screen.getByTestId('status-tag')).toHaveTextContent('Фаъол');
  });
});

// ---------------------------------------------------------------------------
// The edit form
// ---------------------------------------------------------------------------

const FULL_ITEM: Item = {
  id: APPLE.id,
  sku: 'APJ-1000',
  item_type: 'finished_good',
  category: 'juice',
  base_uom: 'bottle',
  shelf_life_days: null,
  min_qty: null,
  translations: { ru: { name: 'Яблочный сок', description: null, ingredients: null, nutrition: null, storage_conditions: null, after_opening: null } },
  packaging_units: [],
  current_price: null,
  price_history: [],
  status: { key: 'active', label: 'Активен', level: 'ok' },
  version: 4,
  created_at: '2026-08-17T09:14:22Z',
  updated_at: '2026-08-17T09:14:22Z',
};

describe('ItemForm', () => {
  // docs/03-API-CONTRACT.md §7 — a PATCH carries the version it read.
  it('sends the version it was given, and omits it when creating', () => {
    const create = toPayload(emptyValues());
    expect(create.version).toBeUndefined();
    expect(create.sku).toBeDefined();

    const update = toPayload(valuesFromItem(FULL_ITEM), FULL_ITEM.version);
    expect(update.version).toBe(4);
    // SKU and item_type are absent on update: changing either would rewrite
    // history that batches and stock movements point at.
    expect(update.sku).toBeUndefined();
    expect(update.item_type).toBeUndefined();
  });

  // «уточняется» depends on null. An empty string would render as a blank value
  // that reads like a verified absence.
  it('turns empty optional fields into null, not empty strings', () => {
    const payload = toPayload({ ...emptyValues(), sku: 'APJ-1000', names: { ru: 'Сок', tg: '', en: '' } });
    expect(payload.category).toBeNull();
    expect(payload.shelf_life_days).toBeNull();
    expect(payload.min_qty).toBeNull();
    // Only the locale that was filled in is sent.
    expect(payload.translations).toEqual({ ru: { name: 'Сок' } });
  });

  it('keeps quantities as strings all the way to the server', () => {
    const payload = toPayload({ ...emptyValues(), sku: 'X', min_qty: '12.345', names: { ru: 'X', tg: '', en: '' } });
    expect(payload.min_qty).toBe('12.345');
    expect(typeof payload.min_qty).toBe('string');
  });

  // A version conflict means someone else's change is already saved. Retrying
  // would overwrite exactly what the guard protected, so the form must stop and
  // say so rather than fail generically.
  it('surfaces a 409 as an actionable conflict and disables saving', () => {
    render(
      <ItemForm
        initial={valuesFromItem(FULL_ITEM)}
        version={4}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={new ApiError(409, 'version_conflict', 'Запись была изменена другим пользователем')}
        submitLabel="Сохранить"
      />,
    );

    expect(screen.getByTestId('version-conflict')).toBeInTheDocument();
    expect(screen.getByText(/изменена другим пользователем/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Сохранить' })).toBeDisabled();
  });

  // Field errors belong against their input, not in a banner: the API returns
  // stable field codes precisely so the form can place them.
  it('renders a per-field error against the field it blames', () => {
    render(
      <ItemForm
        initial={emptyValues()}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={
          new ApiError(400, 'validation_failed', 'Проверьте заполненные поля', [
            { field: 'sku', code: 'already_exists', message: 'Артикул APJ-1000 уже используется' },
          ])
        }
        submitLabel="Создать"
      />,
    );
    expect(screen.getByText('Артикул APJ-1000 уже используется')).toBeInTheDocument();
  });

  it('locks SKU and item type when editing, and explains why', () => {
    render(
      <ItemForm
        initial={valuesFromItem(FULL_ITEM)}
        version={4}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={null}
        submitLabel="Сохранить"
      />,
    );
    expect(screen.getByLabelText(/SKU/)).toBeDisabled();
    expect(screen.getByText(/ссылаются партии и движения склада/)).toBeInTheDocument();
  });

  // D8 — telling the user the prefix rule before submission beats a 400 after.
  it('states the SKU prefix rule when a non-finished-good type is chosen', async () => {
    render(
      <ItemForm
        initial={emptyValues()}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={null}
        submitLabel="Создать"
      />,
    );
    await userEvent.selectOptions(screen.getByLabelText(/Тип позиции/), 'raw_material');
    expect(await screen.findByText(/RAW-/)).toBeInTheDocument();
  });

  // The form must not offer fields the client forbade publishing unverified.
  it('does not offer composition or nutrition inputs', () => {
    render(
      <ItemForm
        initial={emptyValues()}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={null}
        submitLabel="Создать"
      />,
    );
    expect(screen.queryByLabelText(/Состав/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Пищевая ценность/)).not.toBeInTheDocument();
    expect(screen.getByText(/лабораторной проверки/)).toBeInTheDocument();
  });

  it('submits the entered values', async () => {
    const onSubmit = vi.fn();
    render(
      <ItemForm
        initial={emptyValues()}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        isSubmitting={false}
        error={null}
        submitLabel="Создать"
      />,
    );

    await userEvent.type(screen.getByLabelText(/SKU/), 'APJ-1000');
    await userEvent.type(screen.getByLabelText('Русский'), 'Яблочный сок');
    await userEvent.click(screen.getByRole('button', { name: 'Создать' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      sku: 'APJ-1000',
      names: expect.objectContaining({ ru: 'Яблочный сок' }),
    });
  });
});
