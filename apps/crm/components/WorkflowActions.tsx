'use client';

import { useState } from 'react';

import { useCMSTransition } from '@/lib/operations';

const LABELS: Record<string, string> = {
  draft: 'В черновик',
  technical_review: 'На техпроверку',
  language_review: 'На языковую проверку',
  approved: 'Утвердить',
  published: 'Опубликовать',
};

/**
 * The ladder's buttons.
 *
 * Rendered from `allowed_transitions`, which the SERVER computed from the same
 * matrix it enforces. Working the list out again here from the status and the
 * user's permissions would be a second copy of the rules — and the first time
 * the two disagreed, the UI would offer a button that fails.
 */
export function WorkflowActions({
  kind,
  id,
  allowed,
  disabled,
}: {
  kind: 'pages' | 'news';
  id: string;
  allowed: string[];
  disabled: boolean;
}) {
  const transition = useCMSTransition(kind, id);
  const [error, setError] = useState<string | null>(null);

  if (allowed.length === 0) {
    return (
      <span className="muted text-[12px]" data-testid="no-transitions">
        —
      </span>
    );
  }

  return (
    <div className="flex flex-wrap gap-1.5 justify-end" data-testid="workflow-actions">
      {allowed.map((to) => (
        <button
          key={to}
          type="button"
          className="btn btn-secondary"
          disabled={disabled || transition.isPending}
          onClick={async () => {
            setError(null);
            try {
              await transition.mutateAsync({ to });
            } catch (e) {
              // The server's own message: it names the actual rule, which a
              // generic "не удалось" cannot.
              setError(e instanceof Error ? e.message : 'Не удалось изменить статус');
            }
          }}
        >
          {LABELS[to] ?? to}
        </button>
      ))}
      {error && (
        <span className="text-[12px] w-full text-right" role="alert" data-testid="workflow-error">
          {error}
        </span>
      )}
    </div>
  );
}
