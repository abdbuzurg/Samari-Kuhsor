import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { Section, SectionHead } from '@/components/Section';
import { LegalBody } from '@/components/LegalBody';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'legal' });
  return { title: t('termsTitle') };
}

/**
 * Условия использования.
 *
 * The important paragraph is the one about product information: the site says
 * «уточняется» for composition, nutritional values and shelf life because those
 * are not approved yet, and this page states plainly that nothing on the site is
 * a certification claim. That protects the client, and it is true.
 */
export default async function TermsPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('legal');

  return (
    <Section>
      <SectionHead title={t('termsTitle')} />
      <LegalBody>
        <h2>О сайте</h2>
        <p>
          Сайт принадлежит QOIM LLC и представляет продукцию под маркой «Самари Кӯҳсор».
          Он носит информационный характер и не является публичной офертой.
        </p>

        <h2>Сведения о продукции</h2>
        <p>
          Часть характеристик — состав, пищевая ценность, срок годности, классификация
          воды — отмечена как «уточняется». Это означает, что рецептуры ещё не утверждены
          и лабораторные испытания не завершены. Мы не публикуем такие сведения до
          документального подтверждения, и ничто на этом сайте не следует считать
          заявлением о сертификации.
        </p>

        <h2>Обращения</h2>
        <p>
          Отправляя форму, вы подтверждаете, что указанные вами данные достоверны и что вы
          вправе их предоставить. Мы отвечаем на обращения в разумный срок, но не
          гарантируем конкретных сроков ответа.
        </p>

        <h2>Интеллектуальная собственность</h2>
        <p>
          Название, логотипы и материалы сайта принадлежат QOIM LLC. Их использование без
          письменного разрешения не допускается.
        </p>

        <h2>Изменения</h2>
        <p>
          Мы можем изменять содержание сайта и настоящие условия. Актуальная редакция
          всегда доступна на этой странице.
        </p>
      </LegalBody>
    </Section>
  );
}
