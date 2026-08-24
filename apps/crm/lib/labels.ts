/**
 * Russian labels for keys the API stores in English.
 *
 * Per docs/07-IMPLEMENTATION-PLAN.md C3 the backend stores keys and the reader
 * gets Russian, chosen in the frontend. These lived inside the audit page until
 * the activity panel needed the same map; two copies would drift the first time
 * a resource was added.
 */

export function actionLabel(action: string): string {
  const labels: Record<string, string> = {
    create: 'Создание',
    update: 'Изменение',
    delete: 'Удаление',
    approve: 'Согласование',
    login: 'Вход',
    logout: 'Выход',
  };
  return labels[action] ?? action;
}

export function resourceLabel(resource: string): string {
  const labels: Record<string, string> = {
    crm: 'CRM и продажи',
    inquiries: 'Интеграция с сайтом',
    items: 'Товары и цены',
    inventory: 'Склад и запасы',
    procurement: 'Закупки',
    production: 'Производство',
    quality: 'Качество',
    logistics: 'Логистика',
    hr: 'Персонал',
    equipment: 'Оборудование',
    documents: 'Документы',
    admin: 'Администрирование',
    auth: 'Аутентификация',
    cms: 'Контент сайта',
  };
  return labels[resource] ?? resource;
}

/** The first segment of a UUID — enough to correlate two entries at a glance. */
export function shortId(id: string | null | undefined): string {
  if (!id) return '—';
  return id.split('-')[0];
}
