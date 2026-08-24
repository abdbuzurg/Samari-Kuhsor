'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { useCreateCustomer } from '@/lib/operations';

/**
 * Новый клиент.
 *
 * Region is a closed list, not free text: the four regions drive the CRM's own
 * Регион column, and a free-text field makes that column meaningless within a
 * month. Go refuses anything else regardless.
 */
const REGIONS = ['Душанбе', 'Худжанд', 'Хорог', 'Бохтар'];
const TYPES: Array<{ value: string; label: string }> = [
  { value: 'distributor', label: 'Дистрибьютор' },
  { value: 'wholesale', label: 'Опт' },
  { value: 'retail', label: 'Розница' },
];

export default function NewCustomerPage() {
  const router = useRouter();
  const create = useCreateCustomer();
  const [name, setName] = useState('');
  const [type, setType] = useState(TYPES[0].value);
  const [region, setRegion] = useState(REGIONS[0]);
  const [contact, setContact] = useState('');
  const [error, setError] = useState<string | null>(null);

  return (
    <AppShell>
      <h1 className="text-[27px] leading-[1.05] mb-4" style={{ fontFamily: 'var(--font-heading)' }}>
        Новый клиент
      </h1>

      <section className="card p-5 flex flex-col gap-3 max-w-2xl" data-testid="customer-form">
        <label className="flex flex-col gap-1 text-[12px] muted">
          Название
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-label="Название"
          />
        </label>

        <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-[12px] muted">
            Тип
            <select
              className="input"
              value={type}
              onChange={(e) => setType(e.target.value)}
              aria-label="Тип"
            >
              {TYPES.map((x) => (
                <option key={x.value} value={x.value}>
                  {x.label}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[12px] muted">
            Регион
            <select
              className="input"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              aria-label="Регион"
            >
              {REGIONS.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </label>
        </div>

        <label className="flex flex-col gap-1 text-[12px] muted">
          Контакт
          <input
            className="input"
            value={contact}
            onChange={(e) => setContact(e.target.value)}
            aria-label="Контакт"
          />
        </label>

        {error && (
          <p className="text-[12px]" role="alert" data-testid="customer-error">
            {error}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-secondary" onClick={() => router.push('/crm')}>
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={create.isPending || !name.trim()}
            data-testid="save-customer"
            onClick={async () => {
              setError(null);
              try {
                const created = (await create.mutateAsync({
                  name: name.trim(),
                  customer_type: type,
                  region,
                  contact: contact.trim() || undefined,
                })) as { id?: string };
                router.push(created?.id ? `/crm/${created.id}` : '/crm');
              } catch (e) {
                setError(e instanceof Error ? e.message : 'Не удалось сохранить клиента');
              }
            }}
          >
            Сохранить
          </button>
        </div>
      </section>
    </AppShell>
  );
}
