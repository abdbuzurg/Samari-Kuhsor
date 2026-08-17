import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import CMSPage from '@/app/cms/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { ContentBlock, ContentPage, NewsPost } from '@samari/types';

/**
 * Контент сайта.
 *
 * The ladder is the module, and the two things worth testing are the two ways it
 * can be undermined: rendering a button the server will refuse, and letting an
 * edit land on live content without passing a rung.
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

const DRAFT_PAGE: ContentPage = {
  id: '018f3c9e-0000-7000-8000-000000000001',
  key: 'home',
  block_count: 3,
  published_at: null,
  status: { key: 'draft', label: 'Черновик', level: 'neutral' },
  version: 1,
  allowed_transitions: ['technical_review'],
};

const PUBLISHED_PAGE: ContentPage = {
  ...DRAFT_PAGE,
  id: '018f3c9e-0000-7000-8000-000000000002',
  key: 'production',
  published_at: '2026-08-17T09:00:00Z',
  status: { key: 'published', label: 'Опубликовано', level: 'ok' },
  // With cms:approve, a published page may be pulled back. Without it, empty.
  allowed_transitions: ['approved', 'draft'],
};

const BLOCK: ContentBlock = {
  id: '018f3c9e-0000-7000-8000-0000000000b1',
  block_key: 'hero',
  sort_order: 0,
  locale: 'ru',
  heading: 'Фрукты Памира',
  body: 'Соки прямого отжима.',
  cta_label: 'Смотреть продукцию',
};

const READY_POST: NewsPost = {
  id: '018f3c9e-0000-7000-8000-0000000000c1',
  slug: 'zapusk-linii',
  title: 'Запуск линии',
  category: 'Запуск',
  published_on: '2026-08-17',
  status: { key: 'approved', label: 'Утверждено', level: 'info' },
  version: 1,
  missing_locales: [],
  allowed_transitions: ['published', 'draft', 'language_review'],
};

const HALF_TRANSLATED: NewsPost = {
  ...READY_POST,
  id: '018f3c9e-0000-7000-8000-0000000000c2',
  slug: 'montazh',
  title: 'Монтаж оборудования',
  missing_locales: ['tg', 'en'],
};

function cmsHandlers(
  opts: { pages?: ContentPage[]; blocks?: ContentBlock[]; news?: NewsPost[] } = {},
) {
  return [
    http.get('/api/cms/pages', () => HttpResponse.json({ data: opts.pages ?? [DRAFT_PAGE] })),
    http.get('/api/cms/pages/:id/blocks', () =>
      HttpResponse.json({ data: opts.blocks ?? [BLOCK] }),
    ),
    http.get('/api/cms/news', () =>
      HttpResponse.json({
        data: opts.news ?? [READY_POST],
        meta: { page: 1, per_page: 25, total: 1, total_pages: 1 },
      }),
    ),
  ];
}

// ---------------------------------------------------------------------------
// The four states
// ---------------------------------------------------------------------------

describe('states', () => {
  it('renders pages once loaded', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers());
    wrap(<CMSPage />);
    expect(await screen.findByRole('button', { name: 'home' })).toBeInTheDocument();
  });

  it('shows a loading state', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/cms/pages', async () => {
        await delay('infinite');
        return HttpResponse.json({ data: [] });
      }),
    );
    wrap(<CMSPage />);
    await waitFor(() => {
      expect(screen.getByTestId('pages-loading')).toBeInTheDocument();
    });
  });

  it('shows an empty state rather than an empty table', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers({ pages: [] }));
    wrap(<CMSPage />);
    expect(await screen.findByTestId('pages-empty')).toBeInTheDocument();
  });

  it('shows an error state', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/cms/pages', () =>
        HttpResponse.json(
          { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
          { status: 500 },
        ),
      ),
    );
    wrap(<CMSPage />);
    expect(await screen.findByTestId('pages-error')).toBeInTheDocument();
    expect(screen.queryByTestId('pages-empty')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The ladder
// ---------------------------------------------------------------------------

describe('the workflow ladder', () => {
  it('renders exactly the transitions the server allowed', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers());
    wrap(<CMSPage />);
    await screen.findByRole('button', { name: 'home' });

    const actions = screen.getByTestId('workflow-actions');
    // The server computed this list from the same matrix it enforces. Working it
    // out again here would be a second copy of the rules, and the first time
    // they disagreed the UI would offer a button that fails.
    expect(within(actions).getByRole('button', { name: 'На техпроверку' })).toBeInTheDocument();
    expect(within(actions).queryByRole('button', { name: 'Опубликовать' })).not.toBeInTheDocument();
    expect(within(actions).queryByRole('button', { name: 'Утвердить' })).not.toBeInTheDocument();
  });

  it('offers no publish button when the server did not allow it', async () => {
    const noApprove: ContentPage = {
      ...DRAFT_PAGE,
      status: { key: 'language_review', label: 'Языковая проверка', level: 'warn' },
      // A user without cms:approve gets only the downward moves.
      allowed_transitions: ['draft', 'technical_review'],
    };
    server.use(session.loaded(adminUser), ...cmsHandlers({ pages: [noApprove] }));
    wrap(<CMSPage />);
    await screen.findByRole('button', { name: 'home' });

    const actions = screen.getByTestId('workflow-actions');
    expect(within(actions).queryByRole('button', { name: 'Утвердить' })).not.toBeInTheDocument();
    expect(within(actions).getByRole('button', { name: 'В черновик' })).toBeInTheDocument();
  });

  it('sends the chosen transition', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      ...cmsHandlers(),
      http.post('/api/cms/pages/:id/transition', async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ data: { status: DRAFT_PAGE.status, allowed_transitions: [] } });
      }),
    );
    wrap(<CMSPage />);
    await screen.findByRole('button', { name: 'home' });
    await userEvent.click(screen.getByRole('button', { name: 'На техпроверку' }));

    await waitFor(() => {
      expect(sent).toEqual({ to: 'technical_review' });
    });
  });

  it('surfaces the server refusal, which names the actual rule', async () => {
    server.use(
      session.loaded(adminUser),
      ...cmsHandlers({ news: [HALF_TRANSLATED] }),
      http.post('/api/cms/news/:id/transition', () =>
        HttpResponse.json(
          {
            error: {
              code: 'business_rule',
              message: 'Нельзя опубликовать: нет перевода для языков: tg, en.',
            },
          },
          { status: 409 },
        ),
      ),
    );
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));
    await screen.findByText('Монтаж оборудования');
    await userEvent.click(screen.getByRole('button', { name: 'Опубликовать' }));

    const error = await screen.findByTestId('workflow-error');
    expect(error).toHaveTextContent('нет перевода для языков: tg, en');
  });

  it('shows a dash when nothing can be done from here', async () => {
    const stuck: ContentPage = { ...DRAFT_PAGE, allowed_transitions: [] };
    server.use(session.loaded(adminUser), ...cmsHandlers({ pages: [stuck] }));
    wrap(<CMSPage />);
    await screen.findByRole('button', { name: 'home' });
    expect(screen.getByTestId('no-transitions')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Published content is frozen
// ---------------------------------------------------------------------------

describe('published content', () => {
  it('disables the block editor and says how to proceed', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers({ pages: [PUBLISHED_PAGE] }));
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'production' }));

    expect(await screen.findByTestId('frozen-notice')).toBeInTheDocument();
    // Disabled rather than hidden: the editor should be able to READ what is
    // live, and a form that vanishes reads as a loading failure.
    const form = await screen.findByTestId('block-form');
    expect(within(form).getByLabelText('Заголовок')).toBeDisabled();
    expect(within(form).getByRole('button', { name: 'Сохранить' })).toBeDisabled();
  });

  it('leaves a draft page editable', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers());
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'home' }));

    const form = await screen.findByTestId('block-form');
    expect(within(form).getByLabelText('Заголовок')).not.toBeDisabled();
    expect(screen.queryByTestId('frozen-notice')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// The block editor
// ---------------------------------------------------------------------------

describe('block editor', () => {
  it('edits one locale at a time and refetches on switch', async () => {
    const asked: (string | null)[] = [];
    server.use(
      session.loaded(adminUser),
      http.get('/api/cms/pages', () => HttpResponse.json({ data: [DRAFT_PAGE] })),
      http.get('/api/cms/news', () =>
        HttpResponse.json({ data: [], meta: { page: 1, per_page: 25, total: 0, total_pages: 1 } }),
      ),
      http.get('/api/cms/pages/:id/blocks', ({ request }) => {
        const locale = new URL(request.url).searchParams.get('locale');
        asked.push(locale);
        return HttpResponse.json({
          data: [{ ...BLOCK, locale: locale ?? 'ru', heading: locale === 'tg' ? null : 'Фрукты Памира' }],
        });
      }),
    );
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'home' }));
    await screen.findByTestId('block-form');
    expect(asked).toContain('ru');

    // Scoped: the top bar also has a ТҶ button, for the INTERFACE language.
    // These two switchers are genuinely different — one changes what the editor
    // reads, the other changes which translation they are editing.
    const contentLocale = screen.getByRole('group', { name: 'Язык контента' });
    await userEvent.click(within(contentLocale).getByRole('button', { name: 'ТҶ' }));
    await waitFor(() => {
      expect(asked).toContain('tg');
    });
    // An untranslated block still appears, empty. An editor cannot fill in a gap
    // they cannot see.
    await waitFor(() => {
      expect(screen.getByLabelText('Заголовок')).toHaveValue('');
    });
  });

  it('sends the block with its locale', async () => {
    let sent: Record<string, unknown> | null = null;
    server.use(
      session.loaded(adminUser),
      ...cmsHandlers(),
      http.put('/api/cms/pages/:id/blocks', async ({ request }) => {
        sent = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ data: BLOCK });
      }),
    );
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'home' }));
    await screen.findByTestId('block-form');

    await userEvent.clear(screen.getByLabelText('Заголовок'));
    await userEvent.type(screen.getByLabelText('Заголовок'), 'Новый заголовок');
    await userEvent.click(screen.getByRole('button', { name: 'Сохранить' }));

    await waitFor(() => {
      expect(sent).toMatchObject({
        block_key: 'hero',
        locale: 'ru',
        heading: 'Новый заголовок',
      });
    });
  });

  it('shows the empty state when a page has no blocks', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers({ blocks: [] }));
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'home' }));
    expect(await screen.findByTestId('blocks-empty')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// News
// ---------------------------------------------------------------------------

describe('news', () => {
  it('names the missing translations in the list', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers({ news: [HALF_TRANSLATED] }));
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));

    const state = await screen.findByTestId('translations-missing');
    // Shown in the list rather than discovered when publication is refused: the
    // three locales ship together (D10), so a post with one cannot go live.
    expect(state).toHaveTextContent('ТҶ');
    expect(state).toHaveTextContent('EN');
  });

  it('marks a fully translated post as complete', async () => {
    server.use(session.loaded(adminUser), ...cmsHandlers());
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));
    expect(await screen.findByTestId('translations-complete')).toBeInTheDocument();
  });

  it('creates a post', async () => {
    let sent: Record<string, unknown> | null = null;
    server.use(
      session.loaded(adminUser),
      ...cmsHandlers(),
      http.post('/api/cms/news', async ({ request }) => {
        sent = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ data: READY_POST }, { status: 201 });
      }),
    );
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));

    await userEvent.type(screen.getByLabelText('Адрес (латиница)'), 'novaya-liniya');
    await userEvent.click(screen.getByRole('button', { name: 'Создать' }));

    await waitFor(() => {
      expect(sent).toMatchObject({ slug: 'novaya-liniya', category: null });
    });
  });

  it('surfaces a rejected slug against the form', async () => {
    server.use(
      session.loaded(adminUser),
      ...cmsHandlers(),
      http.post('/api/cms/news', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Адрес может содержать только латинские буквы, цифры и дефис',
            },
          },
          { status: 422 },
        ),
      ),
    );
    wrap(<CMSPage />);
    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));
    await userEvent.type(screen.getByLabelText('Адрес (латиница)'), 'запуск');
    await userEvent.click(screen.getByRole('button', { name: 'Создать' }));

    expect(await screen.findByTestId('news-form-error')).toHaveTextContent('латинские буквы');
  });
});

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

describe('permissions', () => {
  it('hides the write controls from a user who may only read', async () => {
    // The seed warehouse role has no cms grant at all; this gives read only.
    const reader = { ...warehouseUser, permissions: ['dashboard:read', 'cms:read'] };
    server.use(session.loaded(reader), ...cmsHandlers());
    wrap(<CMSPage />);
    await screen.findByRole('button', { name: 'home' });

    // Hiding is cosmetic — Go refuses regardless (docs/04-RBAC.md:120) — but a
    // control that always fails is its own defect.
    expect(screen.getByRole('button', { name: 'На техпроверку' })).toBeDisabled();

    await userEvent.click(await screen.findByRole('tab', { name: 'Новости' }));
    expect(screen.queryByLabelText('Адрес (латиница)')).not.toBeInTheDocument();
  });
});
