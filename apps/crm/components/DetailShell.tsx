"use client";

import Link from "next/link";
import type { ReactNode } from "react";

/**
 * Loading, error and not-found for a detail route.
 *
 * Ten detail views land in R03–R13. Without this each would invent its own
 * three states, and the not-found case is the one that matters: before R00 a
 * row click produced a raw 404 because the route did not exist. A record that
 * genuinely is not there must not look like the same defect.
 *
 * Renders inside AppShell rather than wrapping it: the chrome should stay put
 * while the record loads, and swapping the whole shell for a spinner makes the
 * sidebar flicker on every navigation.
 *
 * A 403 is deliberately given its own text. "Нет доступа" and "Не найдено" are
 * different facts, and telling a warehouse user that a batch does not exist when
 * really they may not read it sends them to look for a data problem that is not
 * there.
 */
export function DetailShell({
  moduleLabel,
  moduleHref,
  isLoading,
  error,
  children,
}: {
  moduleLabel: string;
  moduleHref: string;
  isLoading: boolean;
  error?: { status?: number } | null;
  children: ReactNode;
}) {
  if (isLoading) {
    return (
      <p className="muted text-[13px]" data-testid="detail-loading">
        Загрузка…
      </p>
    );
  }

  if (error) {
    const notFound = error.status === 404;
    const forbidden = error.status === 403;
    return (
      <div
        className="card p-6 flex flex-col items-start gap-2"
        data-testid="detail-error"
      >
        <h1
          className="text-[19px]"
          style={{ fontFamily: "var(--font-heading)" }}
        >
          {notFound
            ? "Запись не найдена"
            : forbidden
              ? "Нет доступа"
              : "Не удалось загрузить"}
        </h1>
        <p className="muted text-[13px]">
          {notFound
            ? "Возможно, она была удалена или ссылка устарела."
            : forbidden
              ? "У вас нет прав на просмотр этой записи."
              : "Попробуйте обновить страницу."}
        </p>
        <Link href={moduleHref} className="btn btn-secondary mt-1">
          {`← ${moduleLabel}`}
        </Link>
      </div>
    );
  }

  return <>{children}</>;
}
