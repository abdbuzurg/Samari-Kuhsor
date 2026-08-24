'use client';

import { useParams } from 'next/navigation';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { StatusTag } from '@/components/StatusTag';
import { useEmployee, useUpdateEmployee } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import type { Employee } from '@samari/types';

/**
 * Персонал — employee file.
 *
 * This is the most sensitive payload in the system. It is reachable only through
 * a route guarded on `hr:read`, and there is no public counterpart — the T23 gate
 * asserts personal data is unreachable through every public endpoint, and adding
 * this detail route must not have changed that.
 */
const SHIFTS: Record<string, string> = {
  day: 'Дневная',
  night: 'Ночная',
  rotating: 'Сменный график',
};

export default function EmployeeDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'hr', 'manage');
  const employee = useEmployee(params.id);
  const detail = employee.data;
  const [editing, setEditing] = useState(false);

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.hr')}
        moduleHref="/hr"
        isLoading={employee.isLoading}
        error={
          employee.isError
            ? { status: employee.error instanceof ApiError ? employee.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.hr')}
            moduleHref="/hr"
            recordLabel={detail.full_name}
            title={detail.full_name}
            identifier={orTBC(detail.position_title)}
            status={<StatusTag status={detail.status} />}
            actions={
              mayManage ? (
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setEditing((v) => !v)}
                  data-testid="toggle-edit"
                >
                  {editing ? 'Отмена' : 'Редактировать'}
                </button>
              ) : undefined
            }
            groups={groupsFor(detail)}
            related={editing ? <EmployeeForm employee={detail} onDone={() => setEditing(false)} /> : undefined}
            activity={<ActivityPanel resource="hr" resourceId={detail.id} />}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(e: Employee): FieldGroup[] {
  return [
    {
      title: 'Сотрудник',
      fields: [
        { label: 'ФИО', value: e.full_name },
        { label: 'Должность', value: orTBC(e.position_title) },
        { label: 'Подразделение', value: orTBC(e.department) },
        { label: 'Смена', value: e.shift ? (SHIFTS[e.shift] ?? e.shift) : 'уточняется' },
        { label: 'Принят', value: orTBC(e.hired_on) },
        // Contract expiry is the question this register exists to answer, which
        // is why the list is ordered by it and why it is never hidden here.
        { label: 'Договор до', value: orTBC(e.contract_until) },
      ],
    },
  ];
}

function EmployeeForm({ employee, onDone }: { employee: Employee; onDone: () => void }) {
  const update = useUpdateEmployee(employee.id);
  const [fullName, setFullName] = useState(employee.full_name);
  const [shift, setShift] = useState(employee.shift ?? '');
  const [hiredOn, setHiredOn] = useState(employee.hired_on ?? '');
  const [contractUntil, setContractUntil] = useState(employee.contract_until ?? '');
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setError(null);
    try {
      await update.mutateAsync({
        full_name: fullName.trim(),
        shift: shift || undefined,
        hired_on: hiredOn || undefined,
        contract_until: contractUntil || undefined,
        status: employee.status.key,
        // Optimistic concurrency: the server refuses a stale version rather than
        // overwriting whatever the record now holds. useUpdate reseeds the
        // detail cache from the response, so the next edit sends a fresh one.
        version: employee.version,
      });
      onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить');
    }
  }

  return (
    <section className="card p-4 flex flex-col gap-3" data-testid="employee-form">
      <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
        Редактирование
      </h2>

      <label className="flex flex-col gap-1 text-[12px] muted">
        ФИО
        <input
          className="input"
          value={fullName}
          onChange={(e) => setFullName(e.target.value)}
          aria-label="ФИО"
        />
      </label>

      <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
        <label className="flex flex-col gap-1 text-[12px] muted">
          Смена
          <select
            className="input"
            value={shift}
            onChange={(e) => setShift(e.target.value)}
            aria-label="Смена"
          >
            <option value="">— не назначена —</option>
            {Object.entries(SHIFTS).map(([k, label]) => (
              <option key={k} value={k}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-[12px] muted">
          Принят
          <input
            className="input"
            type="date"
            value={hiredOn}
            onChange={(e) => setHiredOn(e.target.value)}
            aria-label="Принят"
          />
        </label>
        <label className="flex flex-col gap-1 text-[12px] muted">
          Договор до
          <input
            className="input"
            type="date"
            value={contractUntil}
            onChange={(e) => setContractUntil(e.target.value)}
            aria-label="Договор до"
          />
        </label>
      </div>

      {error && (
        <p className="text-[12px]" role="alert" data-testid="employee-error">
          {error}
        </p>
      )}

      <div className="flex justify-end">
        <button
          type="button"
          className="btn btn-primary"
          disabled={update.isPending || !fullName.trim()}
          onClick={() => void submit()}
          data-testid="save-employee"
        >
          Сохранить
        </button>
      </div>
    </section>
  );
}
