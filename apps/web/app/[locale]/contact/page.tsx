import type { Metadata } from 'next';
import Image from 'next/image';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { ContactForm } from '@/components/ContactForm';
import { Section, SectionHead } from '@/components/Section';
import { palette } from '@/lib/palette';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'contact' });
  return { title: t('title'), description: t('lead') };
}

/**
 * Контакты.
 *
 * Rebuilt against the approved design (PROJECT-CONTEXT.md §5.7) after a
 * comparison found the page had been reduced to a heading, an address line and
 * the form. The design's right-hand column is a dark green «Реквизиты» card
 * carrying the QOIM badge, the legal line, the address, email, phone and opening
 * hours, with a static map card beneath it. None of that was on the page.
 *
 * Column order is the design's too: the form leads, the details sit beside it.
 * The built version had them the other way round, so the first thing on the page
 * was an address rather than the thing a distributor came to do.
 *
 * The phone stays «+992 —». The client has not supplied a number
 * (PROJECT-CONTEXT.md §10) and a plausible-looking placeholder on a contact page
 * is worse than an obviously incomplete one.
 */
export default async function ContactPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('contact');

  return (
    <Section>
      <SectionHead title={t('heading')} lead={t('lead')} />

      <div
        className="skc-2col"
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0, 1.1fr) minmax(0, .9fr)',
          gap: 40,
          alignItems: 'start',
        }}
      >
        <ContactForm />

        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <div
            style={{
              background: palette.deep,
              color: '#EAF1DD',
              borderRadius: 20,
              padding: '30px 28px 32px',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 18 }}>
              <span
                style={{
                  background: '#fff',
                  borderRadius: 12,
                  padding: 7,
                  display: 'inline-flex',
                  flex: 'none',
                }}
              >
                <Image
                  src="/assets/logo-qoim.png"
                  alt=""
                  width={30}
                  height={30}
                  style={{ display: 'block', height: 30, width: 'auto' }}
                />
              </span>
              <span>
                <span className="disp" style={{ display: 'block', fontSize: 16, fontWeight: 800 }}>
                  QOIM LLC
                </span>
                <span style={{ display: 'block', fontSize: 11.5, opacity: 0.72, marginTop: 2 }}>
                  {t('legalLine')}
                </span>
              </span>
            </div>

            <div
              style={{
                fontSize: 11,
                letterSpacing: '.18em',
                textTransform: 'uppercase',
                opacity: 0.6,
                marginBottom: 12,
              }}
            >
              {t('reqDetails')}
            </div>

            <address
              style={{
                fontStyle: 'normal',
                fontSize: 14,
                lineHeight: 1.75,
                margin: 0,
                display: 'flex',
                flexDirection: 'column',
                gap: 10,
              }}
            >
              <span>{t('address')}</span>
              <a
                href={`mailto:${t('emailValue')}`}
                style={{ color: '#EAF1DD', textDecoration: 'none', fontWeight: 700 }}
              >
                {t('emailValue')}
              </a>
              <span style={{ opacity: 0.86 }}>{t('phoneValue')}</span>
              <span style={{ opacity: 0.7, fontSize: 13 }}>{t('hours')}</span>
            </address>
          </div>

          {/* The client's own map of Tajikistan, static here — the animated
              three-stage version belongs to the home page. */}
          <figure
            style={{
              margin: 0,
              background: '#fff',
              border: `1px solid ${palette.hairline}`,
              borderRadius: 20,
              overflow: 'hidden',
            }}
          >
            <Image
              src="/assets/map-full.jpg"
              alt={t('mapCaption')}
              width={720}
              height={460}
              style={{ display: 'block', width: '100%', height: 'auto' }}
            />
            <figcaption
              style={{
                fontSize: 11.5,
                color: palette.muted,
                textAlign: 'center',
                padding: '12px 14px 14px',
              }}
            >
              {t('mapCaption')}
            </figcaption>
          </figure>
        </div>
      </div>
    </Section>
  );
}
