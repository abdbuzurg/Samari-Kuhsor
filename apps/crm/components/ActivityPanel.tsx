'use client';

import { actionLabel } from '@/lib/labels';
import { useAuditLog } from '@/lib/operations';

/**
 * The activity row of the shared detail pattern (docs/05-MODULES.md §2):
 * "audit_log entries for this record, newest first".
 *
 * Reads `/audit?resource=&resource_id=`. Both filters already existed in Go
 * (`audit.sql:32-33`) and `ListAuditForResource` was already written; nothing
 * had ever sent the second one. Building this once is what gives every module
 * its activity panel rather than each detail view inventing a feed.
 *
 * A user without `audit:read` gets 403 from Go. That is not an error worth
 * shouting about on someone else's screen — the panel simply says the trail is
 * unavailable, and the rest of the record still renders.
 */
export function ActivityPanel({ resource, resourceId }: { resource: string; resourceId: string }) {
  const audit = useAuditLog({ resource, resourceId });
  const rows = audit.data?.data ?? [];

  return (
    <section className="card p-4" data-testid="activity-panel">
      <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
        История
      </h2>

      {audit.isLoading && (
        <p className="muted text-[12px]" data-testid="activity-loading">
          Загрузка…
        </p>
      )}

      {audit.isError && (
        <p className="muted text-[12px]" data-testid="activity-error">
          История недоступна.
        </p>
      )}

      {!audit.isLoading && !audit.isError && rows.length === 0 && (
        <p className="muted text-[12px]" data-testid="activity-empty">
          Изменений не было.
        </p>
      )}

      {!audit.isLoading && !audit.isError && rows.length > 0 && (
        <ol className="flex flex-col gap-2.5" data-testid="activity-list">
          {rows.map((entry) => (
            <li key={entry.id} className="text-[12.5px]" data-testid="activity-entry">
              <div className="flex items-baseline justify-between gap-2">
                <span>{actionLabel(entry.action)}</span>
                <span className="muted tabular-nums text-[11.5px] shrink-0">
                  {new Date(entry.occurred_at).toLocaleString('ru-RU')}
                </span>
              </div>
              {/* A public enquiry has no actor. «Система» is truthful; a blank
                  reads as missing data, and naming a user would put a person
                  against an action nobody in the company took. */}
              <div className="muted text-[11.5px]">{entry.actor_name ?? 'Система'}</div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
