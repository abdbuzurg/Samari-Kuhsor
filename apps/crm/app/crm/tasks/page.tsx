'use client';

import Link from 'next/link';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useCreateTask, useSetTaskStatus, useTasks } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { Task } from '@samari/types';

/**
 * Задачи — the register behind the «Просроченные задачи» KPI.
 *
 * A task overdue is the only one of the four CRM KPIs that names work somebody
 * has to do, so the number has to lead somewhere.
 */
export default function TasksPage() {
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'crm', 'manage');
  const create = useCreateTask();

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState('');
  const [dueOn, setDueOn] = useState('');
  const [error, setError] = useState<string | null>(null);

  const tasks = useTasks({ q: search, page });
  const rows = tasks.data?.data ?? [];
  const meta = tasks.data?.meta;

  const today = new Date().toISOString().slice(0, 10);
  const overdue = rows.filter((r) => r.status.key === 'open' && r.due_on && r.due_on < today).length;

  const kpis: KPI[] = [
    { label: 'Задач', value: meta ? String(meta.total) : '—' },
    { label: 'Просрочено', value: String(overdue) },
  ];

  const columns: Column<Task>[] = [
    { key: 'title', header: 'Задача', render: (r) => r.title },
    { key: 'assignee', header: 'Исполнитель', render: (r) => orTBC(r.assignee_name) },
    {
      key: 'due',
      header: 'Срок',
      render: (r) =>
        // Overdue is a property of the date, not a status anybody sets.
        r.status.key === 'open' && r.due_on && r.due_on < today ? (
          <strong className="tabular-nums">{r.due_on}</strong>
        ) : (
          <span className="tabular-nums">{orTBC(r.due_on)}</span>
        ),
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
    {
      key: 'action',
      header: '',
      render: (r) => (mayManage && r.status.key === 'open' ? <CloseButton task={r} /> : null),
    },
  ];

  return (
    <AppShell>
      <div className="flex gap-2 mb-4">
        <Link href="/crm" className="btn btn-secondary">
          Клиенты
        </Link>
      </div>

      {open && (
        <section className="card p-4 flex flex-col gap-3 mb-4 max-w-2xl" data-testid="task-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Задача
              <input
                className="input"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                aria-label="Задача"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Срок
              <input
                className="input"
                type="date"
                value={dueOn}
                onChange={(e) => setDueOn(e.target.value)}
                aria-label="Срок"
              />
            </label>
          </div>
          {error && (
            <p className="text-[12px]" role="alert" data-testid="task-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={create.isPending || !title.trim()}
              data-testid="save-task"
              onClick={async () => {
                setError(null);
                try {
                  await create.mutateAsync({ title: title.trim(), due_on: dueOn || undefined });
                  setOpen(false);
                  setTitle('');
                  setDueOn('');
                } catch (e) {
                  setError(e instanceof Error ? e.message : 'Не удалось создать задачу');
                }
              }}
            >
              Сохранить
            </button>
          </div>
        </section>
      )}

      <ListView<Task>
        kicker="Продажи"
        title="Задачи"
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по названию"
        onCreate={mayManage ? () => setOpen((v) => !v) : undefined}
        createLabel="Новая задача"
        isLoading={tasks.isLoading}
        error={tasks.isError ? { message: 'Не удалось загрузить задачи' } : null}
        emptyTitle="Задач нет"
        emptyBody="Создайте задачу, чтобы не потерять договорённость с клиентом."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

function CloseButton({ task }: { task: Task }) {
  const setStatus = useSetTaskStatus(task.id);
  return (
    <button
      type="button"
      className="btn btn-secondary"
      disabled={setStatus.isPending}
      data-testid="close-task"
      onClick={() => setStatus.mutate({ status: 'done' })}
    >
      Выполнена
    </button>
  );
}
