'use client';

import { useState } from 'react';

import { useTransition } from '@/lib/operations';

/**
 * The ladder's buttons, for every module that has a ladder.
 *
 * Rendered from `allowed_transitions`, which the SERVER computed from the same
 * matrix it enforces. Working the list out again here from the status and the
 * user's permissions would be a second copy of the rules — and the first time
 * the two disagreed, the UI would offer a button that fails.
 *
 * Generalised from the CMS-only original in R01. The mutation stays INSIDE this
 * component rather than being passed in, because the CMS renders one of these
 * per table row: a caller cannot call a hook per row without fixing the row
 * count. So the caller passes the endpoint, not the hook.
 *
 * `reasonFor` exists because some transitions carry a mandatory reason — recall
 * (`released → rejected`) is refused by the domain without one. Collecting it
 * inline means the refusal never has to be shown at all.
 */
export function WorkflowActions({
  endpoint,
  invalidate,
  allowed,
  labels,
  disabled,
  reasonFor,
}: {
  /** The transition sub-resource, e.g. `/api/batches/{id}/transition`. */
  endpoint: string;
  /** Query-key prefix(es) to invalidate on success. A transition usually
   *  changes both the record and the register it came from. */
  invalidate: string | string[];
  allowed: string[];
  labels: Record<string, string>;
  disabled: boolean;
  /** Target states that must collect a reason before the request is sent. */
  reasonFor?: (to: string) => boolean;
}) {
  const transition = useTransition(endpoint, invalidate);
  const [error, setError] = useState<string | null>(null);
  const [pendingTarget, setPendingTarget] = useState<string | null>(null);
  const [reason, setReason] = useState('');

  if (allowed.length === 0) {
    return (
      <span className="muted text-[12px]" data-testid="no-transitions">
        —
      </span>
    );
  }

  async function send(to: string, note?: string) {
    setError(null);
    try {
      await transition.mutateAsync(note ? { to, reason: note } : { to });
      setPendingTarget(null);
      setReason('');
    } catch (e) {
      // The server's own message: it names the actual rule, which a generic
      // "не удалось" cannot.
      setError(e instanceof Error ? e.message : 'Не удалось изменить статус');
    }
  }

  return (
    <div className="flex flex-wrap gap-1.5 justify-end" data-testid="workflow-actions">
      {allowed.map((to) => (
        <button
          key={to}
          type="button"
          className="btn btn-secondary"
          disabled={disabled || transition.isPending}
          onClick={() => {
            if (reasonFor?.(to)) {
              setPendingTarget(to);
              setError(null);
              return;
            }
            void send(to);
          }}
        >
          {labels[to] ?? to}
        </button>
      ))}

      {pendingTarget && (
        <div className="w-full flex flex-col gap-1.5 mt-1" data-testid="transition-reason">
          <label className="text-[12px] muted text-left" htmlFor="transition-reason-input">
            Причина для «{labels[pendingTarget] ?? pendingTarget}»
          </label>
          <textarea
            id="transition-reason-input"
            className="input"
            rows={2}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <div className="flex gap-1.5 justify-end">
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => {
                setPendingTarget(null);
                setReason('');
                setError(null);
              }}
            >
              Отмена
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={reason.trim().length === 0 || transition.isPending}
              onClick={() => void send(pendingTarget, reason.trim())}
              data-testid="confirm-transition"
            >
              Подтвердить
            </button>
          </div>
        </div>
      )}

      {error && (
        <span className="text-[12px] w-full text-right" role="alert" data-testid="workflow-error">
          {error}
        </span>
      )}
    </div>
  );
}
