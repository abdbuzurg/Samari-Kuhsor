'use client';

import { StatusTag } from '@/components/StatusTag';
import type { Dashboard } from '@samari/types';

/**
 * Воронка продаж.
 *
 * Every bar is scaled against the FIRST stage, not against the largest — the
 * prototype's `p.v / PIPE[0].v`. That is what makes it read as a funnel: each
 * track shows what fraction of the intake survived to that point, so the bars
 * shorten left to right and the drop-off is the shape you see. Scaling to the
 * maximum would produce the same picture only by accident, and would invert the
 * moment a later stage ever exceeded the first.
 *
 * The backend already computed this and shipped it; nothing rendered it until
 * now, so the panel was dead data travelling over the wire on every page load.
 */
export function PipelinePanel({ stages }: { stages: Dashboard['pipeline'] }) {
  if (stages.length === 0) {
    return (
      <p className="muted text-[13px]" data-testid="pipeline-empty">
        Сделок в работе нет.
      </p>
    );
  }

  const intake = stages[0].count || 1;

  return (
    <div className="flex flex-col gap-3" data-testid="pipeline">
      {stages.map((stage) => (
        <div
          key={stage.stage.key}
          className="grid items-center gap-2 text-[12.5px]"
          style={{ gridTemplateColumns: '118px 1fr 34px' }}
          data-testid="pipeline-row"
        >
          <StatusTag status={stage.stage} />
          <div
            className="rounded-full overflow-hidden"
            style={{
              height: 11,
              background: 'color-mix(in srgb, var(--color-text) 9%, transparent)',
            }}
          >
            <div
              data-testid="pipeline-fill"
              className="h-full rounded-full"
              style={{
                width: `${Math.min(100, Math.round((stage.count / intake) * 100))}%`,
                background: 'var(--color-accent)',
                transition: 'width .5s',
              }}
            />
          </div>
          <span className="text-right tabular-nums font-semibold">{stage.count}</span>
        </div>
      ))}
    </div>
  );
}
