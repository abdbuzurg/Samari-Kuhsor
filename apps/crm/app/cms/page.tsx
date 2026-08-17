'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { StatusTag } from '@/components/StatusTag';
import { WorkflowActions } from '@/components/WorkflowActions';
import {
  useContentBlocks,
  useContentPages,
  useCreateNewsPost,
  useNewsPosts,
  useSaveContentBlock,
} from '@/lib/operations';
import { useSession, can } from '@/lib/session';
import type { ContentBlock, ContentPage, NewsPost } from '@samari/types';

const LOCALES = ['ru', 'tg', 'en'] as const;
const LOCALE_LABELS: Record<string, string> = { ru: 'РУ', tg: 'ТҶ', en: 'EN' };

/**
 * Контент сайта — docs/05-MODULES.md §15.
 *
 * The ladder is the module: Черновик → Техническая проверка → Языковая проверка
 * → Утверждено → Опубликовано. Two rungs need cms:approve, and the server
 * decides which — this page renders the buttons the server said were available
 * rather than working them out again from the status.
 *
 * Published content is not editable here. That is not a UI choice: the server
 * refuses, because an edit landing live without review would make the ladder
 * decorative.
 */
export default function CMSPage() {
  const t = useTranslations();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'cms', 'manage');

  const [tab, setTab] = useState<'pages' | 'news'>('pages');

  return (
    <AppShell>
      <div className="mb-5">
        <div className="text-[11px] uppercase tracking-[0.18em] muted">{t('group.admin')}</div>
        <h1 className="text-[27px] leading-tight mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
          Контент сайта
        </h1>
      </div>

      <div className="flex gap-2 mb-4" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'pages'}
          className={`btn ${tab === 'pages' ? '' : 'btn-secondary'}`}
          onClick={() => setTab('pages')}
        >
          Страницы
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'news'}
          className={`btn ${tab === 'news' ? '' : 'btn-secondary'}`}
          onClick={() => setTab('news')}
        >
          Новости
        </button>
      </div>

      {tab === 'pages' ? <PagesTab mayManage={mayManage} /> : <NewsTab mayManage={mayManage} />}
    </AppShell>
  );
}

function PagesTab({ mayManage }: { mayManage: boolean }) {
  const pages = useContentPages();
  const [open, setOpen] = useState<ContentPage | null>(null);

  if (pages.isLoading) {
    return (
      <p className="muted text-[13px]" role="status" data-testid="pages-loading">
        Загрузка…
      </p>
    );
  }
  if (pages.isError) {
    return (
      <p className="text-[13px]" role="alert" data-testid="pages-error">
        Не удалось загрузить страницы сайта.
      </p>
    );
  }
  const rows = pages.data ?? [];
  if (rows.length === 0) {
    return (
      <p className="muted text-[13px]" data-testid="pages-empty">
        Страниц нет. Они появятся после первичного наполнения контента.
      </p>
    );
  }

  return (
    <div className="card p-4">
      <table className="table w-full">
        <thead>
          <tr>
            <th>Страница</th>
            <th className="text-right">Блоков</th>
            <th>Статус</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {rows.map((page) => (
            <tr key={page.id} data-testid="page-row">
              <td>
                <button
                  type="button"
                  className="text-[13.5px]"
                  aria-expanded={open?.id === page.id}
                  onClick={() => setOpen(open?.id === page.id ? null : page)}
                >
                  {page.key}
                </button>
              </td>
              <td className="text-right tabular-nums">{page.block_count}</td>
              <td>
                <StatusTag status={page.status} />
              </td>
              <td className="text-right">
                <WorkflowActions
                  kind="pages"
                  id={page.id}
                  allowed={page.allowed_transitions}
                  disabled={!mayManage}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {open && <BlockEditor page={open} mayManage={mayManage} />}
    </div>
  );
}

/**
 * The block editor, one locale at a time.
 *
 * Tabs rather than three columns: the three languages are edited by different
 * people at different times, and a three-column form makes every field a third
 * of the width for no benefit to any of them.
 */
function BlockEditor({ page, mayManage }: { page: ContentPage; mayManage: boolean }) {
  const [locale, setLocale] = useState<string>('ru');
  const blocks = useContentBlocks(page.id, locale);
  const save = useSaveContentBlock(page.id);
  const [error, setError] = useState<string | null>(null);

  const frozen = page.status.key === 'published';

  return (
    <section className="mt-5 pt-4 border-t" style={{ borderColor: 'var(--color-divider)' }}>
      <div className="flex items-center gap-2 mb-3">
        <h2 className="text-[14px]" style={{ fontFamily: 'var(--font-heading)' }}>
          Блоки страницы «{page.key}»
        </h2>
        <div className="flex gap-1 ml-auto" role="group" aria-label="Язык контента">
          {LOCALES.map((l) => (
            <button
              key={l}
              type="button"
              className={`btn ${l === locale ? '' : 'btn-secondary'}`}
              aria-pressed={l === locale}
              onClick={() => setLocale(l)}
            >
              {LOCALE_LABELS[l]}
            </button>
          ))}
        </div>
      </div>

      {frozen && (
        <p className="muted text-[13px] mb-3" data-testid="frozen-notice">
          Страница опубликована. Чтобы изменить содержимое, верните её в черновик — этот шаг
          записывается в журнал.
        </p>
      )}
      {error && (
        <p className="text-[13px] mb-3" role="alert" data-testid="block-error">
          {error}
        </p>
      )}

      {blocks.isLoading && (
        <p className="muted text-[13px]" role="status" data-testid="blocks-loading">
          Загрузка…
        </p>
      )}
      {blocks.isError && (
        <p className="text-[13px]" role="alert" data-testid="blocks-error">
          Не удалось загрузить блоки.
        </p>
      )}
      {!blocks.isLoading && !blocks.isError && (blocks.data ?? []).length === 0 && (
        <p className="muted text-[13px]" data-testid="blocks-empty">
          На этой странице пока нет блоков.
        </p>
      )}

      <div className="space-y-4">
        {(blocks.data ?? []).map((block) => (
          <BlockForm
            key={block.id}
            block={block}
            locale={locale}
            disabled={!mayManage || frozen || save.isPending}
            onSave={async (payload) => {
              setError(null);
              try {
                await save.mutateAsync(payload);
              } catch (e) {
                setError(e instanceof Error ? e.message : 'Не удалось сохранить блок');
              }
            }}
          />
        ))}
      </div>
    </section>
  );
}

function BlockForm({
  block,
  locale,
  disabled,
  onSave,
}: {
  block: ContentBlock;
  locale: string;
  disabled: boolean;
  onSave: (payload: unknown) => Promise<void>;
}) {
  return (
    <form
      data-testid="block-form"
      onSubmit={(e) => {
        e.preventDefault();
        const form = new FormData(e.currentTarget);
        void onSave({
          block_key: block.block_key,
          sort_order: block.sort_order,
          locale,
          heading: emptyToNull(form.get('heading')),
          body: emptyToNull(form.get('body')),
          cta_label: emptyToNull(form.get('cta_label')),
        });
      }}
    >
      <fieldset disabled={disabled} className="grid gap-2">
        <legend className="text-[12px] muted mb-1">{block.block_key}</legend>
        <label className="text-[12px] muted" htmlFor={`${block.id}-heading`}>
          Заголовок
        </label>
        <input
          id={`${block.id}-heading`}
          name="heading"
          className="input"
          defaultValue={block.heading ?? ''}
          key={`${block.id}-${locale}-heading`}
        />
        <label className="text-[12px] muted" htmlFor={`${block.id}-body`}>
          Текст
        </label>
        <textarea
          id={`${block.id}-body`}
          name="body"
          rows={3}
          className="input"
          defaultValue={block.body ?? ''}
          key={`${block.id}-${locale}-body`}
        />
        <label className="text-[12px] muted" htmlFor={`${block.id}-cta`}>
          Текст кнопки
        </label>
        <input
          id={`${block.id}-cta`}
          name="cta_label"
          className="input"
          defaultValue={block.cta_label ?? ''}
          key={`${block.id}-${locale}-cta`}
        />
        <button type="submit" className="btn justify-self-start mt-1">
          Сохранить
        </button>
      </fieldset>
    </form>
  );
}

function NewsTab({ mayManage }: { mayManage: boolean }) {
  const news = useNewsPosts({});
  const create = useCreateNewsPost();
  const [error, setError] = useState<string | null>(null);

  if (news.isLoading) {
    return (
      <p className="muted text-[13px]" role="status" data-testid="news-loading">
        Загрузка…
      </p>
    );
  }
  if (news.isError) {
    return (
      <p className="text-[13px]" role="alert" data-testid="news-error">
        Не удалось загрузить новости.
      </p>
    );
  }
  const rows = news.data?.data ?? [];

  return (
    <div className="card p-4">
      {error && (
        <p className="text-[13px] mb-3" role="alert" data-testid="news-form-error">
          {error}
        </p>
      )}

      {mayManage && (
        <form
          className="flex flex-wrap items-end gap-2 mb-4"
          onSubmit={async (e) => {
            e.preventDefault();
            const form = new FormData(e.currentTarget);
            setError(null);
            try {
              await create.mutateAsync({
                slug: String(form.get('slug') ?? ''),
                category: emptyToNull(form.get('category')),
              });
              e.currentTarget.reset();
            } catch (err) {
              setError(err instanceof Error ? err.message : 'Не удалось создать новость');
            }
          }}
        >
          <div>
            <label className="block text-[12px] muted mb-1" htmlFor="new-slug">
              Адрес (латиница)
            </label>
            <input id="new-slug" name="slug" className="input" required />
          </div>
          <div>
            <label className="block text-[12px] muted mb-1" htmlFor="new-category">
              Рубрика
            </label>
            <input id="new-category" name="category" className="input" />
          </div>
          <button type="submit" className="btn" disabled={create.isPending}>
            Создать
          </button>
        </form>
      )}

      {rows.length === 0 ? (
        <p className="muted text-[13px]" data-testid="news-empty">
          Новостей нет.
        </p>
      ) : (
        <table className="table w-full">
          <thead>
            <tr>
              <th>Заголовок</th>
              <th>Адрес</th>
              <th>Переводы</th>
              <th>Статус</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rows.map((post) => (
              <tr key={post.id} data-testid="news-row">
                <td>{post.title ?? '—'}</td>
                <td className="tabular-nums text-[12.5px]">{post.slug}</td>
                <td>
                  <TranslationState post={post} />
                </td>
                <td>
                  <StatusTag status={post.status} />
                </td>
                <td className="text-right">
                  <WorkflowActions
                    kind="news"
                    id={post.id}
                    allowed={post.allowed_transitions}
                    disabled={!mayManage}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/**
 * Which translations are still missing.
 *
 * Shown in the list rather than left to be discovered when publication is
 * refused. The three locales ship together (D10), so a post with one translation
 * cannot go live — and an editor should learn that before they try.
 */
function TranslationState({ post }: { post: NewsPost }) {
  if (post.missing_locales.length === 0) {
    return (
      <span className="tag tag-ok" data-testid="translations-complete">
        Все языки
      </span>
    );
  }
  return (
    <span className="tag tag-warn" data-testid="translations-missing">
      Нет: {post.missing_locales.map((l) => LOCALE_LABELS[l] ?? l).join(', ')}
    </span>
  );
}

function emptyToNull(value: FormDataEntryValue | null): string | null {
  const s = typeof value === 'string' ? value.trim() : '';
  return s === '' ? null : s;
}
