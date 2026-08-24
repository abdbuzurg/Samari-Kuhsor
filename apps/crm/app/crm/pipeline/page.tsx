'use client';

import Link from 'next/link';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { StatusTag } from '@/components/StatusTag';
import { useDeals } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { DealRow } from '@samari/types';

/**
 * Воронка сделок — the pipeline board (docs/05-MODULES.md:179).
 *
 * Новый лид → Переговоры → КП отправлено → Выиграно / Проиграно.
 *
 * Columns are the five stages of the `deals.stage` CHECK constraint. The board
 * reads; moving a deal happens on its own screen, where the server's
 * `allowed_transitions` decides what may be offered. A drag-and-drop board that
 * recomputed legality client-side would be a second copy of the matrix.
 */

const STAGES: Array<{ key: string; label: string; level: string }> = [
  { key: 'new', label: 'Новый лид', level: 'info' },
  { key: 'negotiation', label: 'Переговоры', level: 'info' },
  { key: 'quoted', label: 'КП отправлено', level: 'warn' },
  { key: 'won', label: 'Выиграно', level: 'ok' },
  { key: 'lost', label: 'Проиграно', level: 'neutral' },
];

export default function PipelinePage() {
  const [search, setSearch] = useState('');
  const deals = useDeals({ q: search, page: 1 });
  const rows = deals.data?.data ?? [];

  return (
    <AppShell>
      <div className="flex items-center justify-between gap-3 mb-4 flex-wrap">
        <div>
          <div className="text-[12px] muted">CRM и продажи</div>
          <h1 className="text-[27px] leading-[1.05]" style={{ fontFamily: 'var(--font-heading)' }}>
            Воронка сделок
          </h1>
        </div>
        <div className="flex gap-2">
          <input
            className="input"
            placeholder="Поиск по клиенту и региону"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Поиск"
          />
          <Link href="/crm" className="btn btn-secondary">
            Клиенты
          </Link>
        </div>
      </div>

      {deals.isLoading && (
        <p className="muted text-[13px]" data-testid="pipeline-loading">
          Загрузка…
        </p>
      )}

      {deals.isError && (
        <p className="text-[13px]" role="alert" data-testid="pipeline-error">
          Не удалось загрузить сделки.
        </p>
      )}

      {!deals.isLoading && !deals.isError && rows.length === 0 && (
        <div className="card p-6" data-testid="pipeline-empty">
          <h2 className="text-[17px] mb-1" style={{ fontFamily: 'var(--font-heading)' }}>
            Сделок нет
          </h2>
          <p className="muted text-[13px]">
            Сделка создаётся на карточке клиента после конвертации обращения с сайта.
          </p>
        </div>
      )}

      {!deals.isLoading && !deals.isError && rows.length > 0 && (
        // Horizontal scroll rather than a wrap: five stages read as a funnel
        // left to right, and stacking them vertically loses the shape entirely.
        <div className="overflow-x-auto" data-testid="pipeline-board">
          <div className="flex gap-3 min-w-max pb-2">
            {STAGES.map((stage) => {
              const inStage = rows.filter((d) => d.stage.key === stage.key);
              const total = inStage.reduce((sum, d) => sum + Number(d.amount ?? 0), 0);
              return (
                <section
                  key={stage.key}
                  className="card p-3 flex flex-col gap-2"
                  style={{ width: 250 }}
                  data-testid={`stage-${stage.key}`}
                >
                  <header className="flex items-center justify-between gap-2">
                    <StatusTag status={{ key: stage.key, label: stage.label, level: stage.level }} />
                    <span className="text-[12px] muted tabular-nums">{inStage.length}</span>
                  </header>
                  <div className="text-[12px] muted tabular-nums">
                    {total > 0 ? `${total.toFixed(2)} с.` : '—'}
                  </div>

                  {inStage.length === 0 ? (
                    <p className="muted text-[12px]">Пусто</p>
                  ) : (
                    inStage.map((deal) => <DealCard key={deal.id} deal={deal} />)
                  )}
                </section>
              );
            })}
          </div>
        </div>
      )}
    </AppShell>
  );
}

function DealCard({ deal }: { deal: DealRow }) {
  return (
    <Link
      href={`/crm/deals/${deal.id}`}
      className="card p-2.5 hover:underline"
      data-testid="deal-card"
    >
      <div className="text-[13px]">{deal.customer_name}</div>
      <div className="text-[12px] muted">{orTBC(deal.region)}</div>
      <div className="flex items-center justify-between gap-2 mt-1">
        <span className="text-[12.5px] tabular-nums">
          {deal.amount ? `${deal.amount} с.` : '—'}
        </span>
        <span className="text-[11.5px] muted">{orTBC(deal.expected_close)}</span>
      </div>
    </Link>
  );
}
