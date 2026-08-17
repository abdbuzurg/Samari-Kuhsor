import type { Status } from '@samari/types';

/**
 * A status tag.
 *
 * The whole point of this component is what it does NOT do: it never decides a
 * colour. The backend sends `level`, and this maps level → the prototype's
 * verbatim `.tag-*` class (CLAUDE.md §5, docs/03-API-CONTRACT.md:177).
 *
 * That indirection exists because inside the content area green means *healthy*,
 * never merely *branded* — an explicit client instruction. A component that
 * mapped "active" or "released" to green directly would work until the first
 * status whose brand colour and health meaning disagree, and then it would be
 * wrong in a way nobody notices until the client does.
 */

const LEVEL_CLASS: Record<string, string> = {
  ok: 'tag-ok',
  warn: 'tag-warn',
  danger: 'tag-danger',
  info: 'tag-info',
  neutral: 'tag-neutral',
};

export function StatusTag({ status, label }: { status: Status; label?: string }) {
  // An unknown level falls back to neutral rather than rendering an unstyled tag:
  // a new status added server-side should look plain, not broken.
  const className = LEVEL_CLASS[status.level] ?? LEVEL_CLASS.neutral;

  return (
    <span
      className={`tag ${className}`}
      data-testid="status-tag"
      data-level={status.level}
      data-status={status.key}
    >
      {/* label comes from the caller's i18n dictionary when available; the
          payload's Russian label is the fallback (docs/07 C3). */}
      {label ?? status.label}
    </span>
  );
}
