import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { Section, SectionHead } from '@/components/Section';
import { ConsentReset } from '@/components/ConsentReset';
import { LegalBody } from '@/components/LegalBody';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'legal' });
  return { title: t('privacyTitle'), robots: { index: true, follow: true } };
}

/**
 * Политика конфиденциальности.
 *
 * Written against what the system ACTUALLY does, not from a template. Every
 * claim here is checkable in the code: the enquiry form stores what it says it
 * stores, the enquiry IP is recorded for the rate limit and the audit trail, and
 * the statistics section describes the first-party collector that replaced
 * Matomo (docs/01-DECISIONS.md D12) — including the 90-day window, which is
 * enforced by a job rather than asserted here.
 *
 * This is deliberately not legal advice and does not pretend to be a lawyer's
 * document — it is an accurate description of the data flows, which is what the
 * client needs before a lawyer reviews it.
 */
export default async function PrivacyPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('legal');

  return (
    <Section>
      <SectionHead title={t('privacyTitle')} />
      <LegalBody>
        <h2>Какие данные мы собираем</h2>
        <p>
          Через форму обратной связи мы получаем имя, контактные данные (телефон или
          электронную почту), название компании и текст сообщения — то, что вы вводите
          сами. При жалобе на продукцию дополнительно сохраняется номер партии.
        </p>
        <p>
          Вместе с отправкой формы сохраняется IP-адрес отправителя. Он используется
          только для ограничения частоты отправок и для журнала действий; он не
          используется для профилирования и не передаётся третьим сторонам.
        </p>

        <h2>Зачем мы их используем</h2>
        <p>
          Чтобы ответить на ваше обращение и, если речь идёт о поставках, продолжить
          работу по заявке. Каждому обращению присваивается номер, который вы получаете
          сразу после отправки — по нему мы находим вашу заявку.
        </p>

        <h2>Статистика посещений</h2>
        <p>
          Мы ведём собственную статистику и храним её на своём сервере. Сторонних
          систем аналитики — Google Analytics, Matomo или любых других — на сайте нет,
          и никакие данные не передаются третьим сторонам.
        </p>
        <p>Мы записываем:</p>
        <ul>
          <li>какие страницы открывают;</li>
          <li>какие товары открывают — на странице товара и в карточке на линии;</li>
          <li>какие ссылки и кнопки нажимают.</li>
        </ul>
        <p>
          Чтобы отличить один визит от другого, действия связываются случайным номером.
          Он создаётся в браузере, хранится только до закрытия вкладки и не связан ни с
          вашим именем, ни с учётной записью. Мы не используем файлы cookie для статистики
          и не отслеживаем вас между визитами.
        </p>
        <p>
          Статистика включается только после того, как вы нажали «Принять». Пока вы этого
          не сделали или если вы отказались, ничего не отправляется — ни одного запроса.
        </p>
        <p>
          Подробные записи хранятся <strong>90 дней</strong>, после чего удаляются
          безвозвратно. Остаются только обезличенные суточные итоги — сколько раз открыли
          ту или иную страницу, — в которых нет номера визита и ничего, что связано с
          конкретным человеком.
        </p>

        <ConsentReset />

        <h2>Сколько мы храним</h2>
        <p>
          Обращения хранятся в нашей учётной системе. Записи не удаляются безвозвратно:
          они помечаются как удалённые и остаются доступными для внутренних проверок, как
          того требует учёт производства пищевой продукции.
        </p>

        <h2>Ваши права</h2>
        <p>
          Вы можете запросить сведения о хранимых данных или их исправление, написав нам
          через форму обратной связи или по адресу, указанному в разделе «Контакты».
          Укажите номер обращения, если он у вас есть.
        </p>

        <h2>Контакты</h2>
        <p>QOIM LLC, Тем, Хорог, ГБАО, Республика Таджикистан.</p>
      </LegalBody>
    </Section>
  );
}
