'use client';

import { useParams } from 'next/navigation';
import Link from 'next/link';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import {
  useCreateContact,
  useCreateDeal,
  useCustomerDetail,
  useUpdateCustomer,
} from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type {
  Contact,
  CustomerDetail,
  CustomerInquiry,
  CustomerLead,
  CustomerOrder,
  DealRow,
} from '@samari/types';

/**
 * Клиент — the detail view specified at docs/05-MODULES.md:179:
 *
 *   "customer header · contacts · deals with stage history · linked inquiries ·
 *    orders · activity"
 *
 * Every band is a real relation. The linked-enquiries band reaches back through
 * the lead a conversion created, which is what makes an enquiry converted six
 * months ago still traceable to the customer it became.
 */
const TYPES: Record<string, string> = {
  distributor: 'Дистрибьютор',
  wholesale: 'Опт',
  retail: 'Розница',
};

export default function CustomerDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'crm', 'manage');
  const customer = useCustomerDetail(params.id);
  const detail = customer.data;
  const [editing, setEditing] = useState(false);

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.crm')}
        moduleHref="/crm"
        isLoading={customer.isLoading}
        error={
          customer.isError
            ? { status: customer.error instanceof ApiError ? customer.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.crm')}
            moduleHref="/crm"
            recordLabel={detail.customer.name}
            title={detail.customer.name}
            identifier={orTBC(detail.customer.region)}
            actions={
              mayManage ? (
                <button
                  type="button"
                  className="btn btn-secondary"
                  data-testid="toggle-customer-edit"
                  onClick={() => setEditing((v) => !v)}
                >
                  {editing ? 'Отмена' : 'Редактировать'}
                </button>
              ) : undefined
            }
            groups={groupsFor(detail)}
            related={
              <>
                {editing && (
                  <CustomerForm
                    customer={detail.customer}
                    onDone={() => setEditing(false)}
                  />
                )}
                <Contacts
                  customerId={detail.customer.id}
                  contacts={detail.contacts}
                  mayManage={mayManage}
                />
                <Deals
                  customerId={detail.customer.id}
                  deals={detail.deals}
                  mayManage={mayManage}
                />
                <Orders orders={detail.orders} />
                <Enquiries inquiries={detail.inquiries} />
                <Leads leads={detail.leads} />
              </>
            }
            activity={<ActivityPanel resource="crm" resourceId={detail.customer.id} />}
            footer={{
              createdAt: formatDateTime(detail.customer.created_at),
              updatedAt: formatDateTime(detail.customer.created_at),
              version: detail.customer.version,
            }}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(d: CustomerDetail): FieldGroup[] {
  const c = d.customer;
  return [
    {
      title: 'Клиент',
      fields: [
        { label: 'Название', value: c.name },
        {
          label: 'Тип',
          value: c.customer_type ? (TYPES[c.customer_type] ?? c.customer_type) : 'уточняется',
        },
        { label: 'Регион', value: orTBC(c.region) },
        { label: 'Контакт', value: orTBC(c.contact) },
      ],
    },
  ];
}

function Contacts({
  customerId,
  contacts,
  mayManage,
}: {
  customerId: string;
  contacts: Contact[];
  mayManage: boolean;
}) {
  const create = useCreateContact(customerId);
  const [open, setOpen] = useState(false);
  const [fullName, setFullName] = useState('');
  const [role, setRole] = useState('');
  const [phone, setPhone] = useState('');
  const [error, setError] = useState<string | null>(null);

  const columns: RelatedColumn<Contact>[] = [
    { key: 'name', header: 'Имя', render: (r) => r.full_name },
    { key: 'role', header: 'Должность', render: (r) => orTBC(r.role) },
    { key: 'phone', header: 'Телефон', render: (r) => orTBC(r.phone) },
    { key: 'email', header: 'Email', render: (r) => orTBC(r.email) },
  ];

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<Contact>
        title="Контактные лица"
        columns={columns}
        rows={contacts}
        rowKey={(r) => r.id}
        emptyLabel="Контактных лиц нет."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-contact-form"
            >
              {open ? 'Отмена' : 'Добавить контакт'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="contact-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Имя
              <input
                className="input"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                aria-label="Имя контакта"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Должность
              <input
                className="input"
                value={role}
                onChange={(e) => setRole(e.target.value)}
                aria-label="Должность"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Телефон
              <input
                className="input"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                aria-label="Телефон"
              />
            </label>
          </div>
          {error && (
            <p className="text-[12px]" role="alert" data-testid="contact-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={create.isPending || !fullName.trim()}
              data-testid="save-contact"
              onClick={async () => {
                setError(null);
                try {
                  await create.mutateAsync({
                    full_name: fullName.trim(),
                    role: role.trim() || undefined,
                    phone: phone.trim() || undefined,
                  });
                  setOpen(false);
                  setFullName('');
                  setRole('');
                  setPhone('');
                } catch (e) {
                  setError(e instanceof Error ? e.message : 'Не удалось сохранить контакт');
                }
              }}
            >
              Сохранить
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

function Deals({
  customerId,
  deals,
  mayManage,
}: {
  customerId: string;
  deals: DealRow[];
  mayManage: boolean;
}) {
  const create = useCreateDeal();
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState('');
  const [expected, setExpected] = useState('');
  const [error, setError] = useState<string | null>(null);

  const columns: RelatedColumn<DealRow>[] = [
    {
      key: 'stage',
      header: 'Стадия',
      render: (r) => (
        <Link href={`/crm/deals/${r.id}`} className="hover:underline">
          <StatusTag status={r.stage} />
        </Link>
      ),
    },
    {
      key: 'amount',
      header: 'Сумма',
      numeric: true,
      render: (r) => (r.amount ? `${r.amount} с.` : '—'),
    },
    { key: 'owner', header: 'Менеджер', render: (r) => orTBC(r.owner_name) },
    { key: 'close', header: 'Закрытие', render: (r) => orTBC(r.expected_close) },
  ];

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<DealRow>
        title="Сделки"
        columns={columns}
        rows={deals}
        rowKey={(r) => r.id}
        emptyLabel="Сделок нет."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-deal-form"
            >
              {open ? 'Отмена' : 'Создать сделку'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="deal-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Сумма
              <input
                className="input"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                aria-label="Сумма сделки"
                inputMode="decimal"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Ожидаемое закрытие
              <input
                className="input"
                type="date"
                value={expected}
                onChange={(e) => setExpected(e.target.value)}
                aria-label="Ожидаемое закрытие"
              />
            </label>
          </div>
          {error && (
            <p className="text-[12px]" role="alert" data-testid="deal-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={create.isPending}
              data-testid="save-deal"
              onClick={async () => {
                setError(null);
                try {
                  await create.mutateAsync({
                    customer_id: customerId,
                    amount: amount.trim() || undefined,
                    expected_close: expected || undefined,
                  });
                  setOpen(false);
                  setAmount('');
                  setExpected('');
                } catch (e) {
                  setError(e instanceof Error ? e.message : 'Не удалось создать сделку');
                }
              }}
            >
              Создать
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

function Orders({ orders }: { orders: CustomerOrder[] }) {
  const columns: RelatedColumn<CustomerOrder>[] = [
    {
      key: 'no',
      header: 'Заказ',
      render: (r) => (
        <Link href={`/crm/orders/${r.id}`} className="hover:underline tabular-nums">
          {r.so_no}
        </Link>
      ),
    },
    { key: 'date', header: 'Дата', render: (r) => orTBC(r.ordered_on) },
    { key: 'total', header: 'Сумма', numeric: true, render: (r) => `${r.total} с.` },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <RelatedTable<CustomerOrder>
      title="Заказы"
      columns={columns}
      rows={orders}
      rowKey={(r) => r.id}
      emptyLabel="Заказов нет."
    />
  );
}

/** Enquiries reach a customer through the lead their conversion created. */
function Enquiries({ inquiries }: { inquiries: CustomerInquiry[] }) {
  const columns: RelatedColumn<CustomerInquiry>[] = [
    {
      key: 'ref',
      header: 'Обращение',
      render: (r) => (
        <Link href={`/inquiries/${r.id}`} className="hover:underline tabular-nums">
          {r.reference_no}
        </Link>
      ),
    },
    { key: 'type', header: 'Тип', render: (r) => <StatusTag status={r.type} /> },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
    {
      key: 'when',
      header: 'Получено',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.submitted_at)}</span>,
    },
  ];

  return (
    <RelatedTable<CustomerInquiry>
      title="Обращения с сайта"
      columns={columns}
      rows={inquiries}
      rowKey={(r) => r.id}
      emptyLabel="Обращений с сайта не было."
    />
  );
}

function Leads({ leads }: { leads: CustomerLead[] }) {
  const columns: RelatedColumn<CustomerLead>[] = [
    { key: 'source', header: 'Источник', render: (r) => orTBC(r.source) },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
    {
      key: 'when',
      header: 'Создан',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.created_at)}</span>,
    },
  ];

  return (
    <RelatedTable<CustomerLead>
      title="Лиды"
      columns={columns}
      rows={leads}
      rowKey={(r) => r.id}
      emptyLabel="Лидов нет."
    />
  );
}


/**
 * Editing a customer.
 *
 * The region list is closed for the same reason it is on the create form: it
 * drives the register's Регион column, and free text makes that column
 * meaningless within a month. Go refuses anything else regardless.
 */
const REGIONS = ['Душанбе', 'Худжанд', 'Хорог', 'Бохтар'];

function CustomerForm({
  customer,
  onDone,
}: {
  customer: CustomerDetail['customer'];
  onDone: () => void;
}) {
  const update = useUpdateCustomer(customer.id);
  const [name, setName] = useState(customer.name);
  const [type, setType] = useState(customer.customer_type ?? '');
  const [region, setRegion] = useState(customer.region ?? '');
  const [contact, setContact] = useState(customer.contact ?? '');
  const [error, setError] = useState<string | null>(null);

  return (
    <section className="card p-4 flex flex-col gap-3" data-testid="customer-edit-form">
      <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
        Редактирование
      </h2>

      <label className="flex flex-col gap-1 text-[12px] muted">
        Название
        <input
          className="input"
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label="Название клиента"
        />
      </label>

      <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
        <label className="flex flex-col gap-1 text-[12px] muted">
          Тип
          <select
            className="input"
            value={type}
            onChange={(e) => setType(e.target.value)}
            aria-label="Тип клиента"
          >
            <option value="">— не указан —</option>
            {Object.entries(TYPES).map(([k, label]) => (
              <option key={k} value={k}>
                {label}
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
            aria-label="Регион клиента"
          >
            <option value="">— не указан —</option>
            {REGIONS.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-[12px] muted">
          Контакт
          <input
            className="input"
            value={contact}
            onChange={(e) => setContact(e.target.value)}
            aria-label="Контакт клиента"
          />
        </label>
      </div>

      {error && (
        <p className="text-[12px]" role="alert" data-testid="customer-edit-error">
          {error}
        </p>
      )}

      <div className="flex justify-end">
        <button
          type="button"
          className="btn btn-primary"
          disabled={update.isPending || !name.trim()}
          data-testid="save-customer-edit"
          onClick={async () => {
            setError(null);
            try {
              await update.mutateAsync({
                name: name.trim(),
                customer_type: type || undefined,
                region: region || undefined,
                contact: contact.trim() || undefined,
                // Optimistic concurrency: the server refuses a stale version
                // rather than overwriting whoever saved first.
                version: customer.version,
              });
              onDone();
            } catch (e) {
              setError(e instanceof Error ? e.message : 'Не удалось сохранить');
            }
          }}
        >
          Сохранить
        </button>
      </div>
    </section>
  );
}
