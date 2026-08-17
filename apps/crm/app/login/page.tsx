'use client';

import { useRouter } from 'next/navigation';
import { useState, type FormEvent } from 'react';

import { useLogin, ApiError } from '@/lib/session';

/**
 * Login.
 *
 * Error text is rendered from the stable `code`, not from the server's message
 * (docs/03-API-CONTRACT.md:116, docs/07-IMPLEMENTATION-PLAN.md C3) — the CRM
 * ships in three languages and a Russian message baked into the payload would
 * show Russian errors in a Tajik interface. The server message is the fallback.
 */
const ERROR_TEXT: Record<string, string> = {
  unauthenticated: 'Неверный email или пароль',
  validation_failed: 'Проверьте заполненные поля',
  forbidden: 'Учётная запись временно заблокирована',
  rate_limited: 'Слишком много попыток, попробуйте позже',
  internal_error: 'Сервис временно недоступен',
};

export default function LoginPage() {
  const router = useRouter();
  const login = useLogin();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate({ email, password }, { onSuccess: () => router.replace('/') });
  };

  const message =
    login.error instanceof ApiError
      ? (ERROR_TEXT[login.error.code] ?? login.error.message ?? ERROR_TEXT.internal_error)
      : null;

  return (
    <div className="min-h-screen grid place-items-center p-6" style={{ background: 'var(--color-surface)' }}>
      <form onSubmit={onSubmit} className="card p-8 w-full max-w-sm" noValidate>
        <div className="flex items-center gap-3 mb-6">
          <div
            className="w-10 h-10 grid place-items-center rounded-sm text-[15px]"
            style={{
              background: 'var(--color-accent)',
              color: 'var(--color-bg)',
              fontFamily: 'var(--font-heading)',
              fontWeight: 'var(--font-heading-weight)',
            }}
          >
            СК
          </div>
          <div>
            <div className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
              Самари Кӯҳсор
            </div>
            <div className="text-[11px] muted">CRM / ERP платформа</div>
          </div>
        </div>

        <label className="block mb-3">
          <span className="block text-[12px] mb-1">Email</span>
          <input
            className="input w-full"
            type="email"
            value={email}
            autoComplete="username"
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </label>

        <label className="block mb-5">
          <span className="block text-[12px] mb-1">Пароль</span>
          <input
            className="input w-full"
            type="password"
            value={password}
            autoComplete="current-password"
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </label>

        {message && (
          <div role="alert" className="tag tag-danger mb-4 w-full justify-center py-2">
            {message}
          </div>
        )}

        <button
          type="submit"
          disabled={login.isPending}
          className="btn w-full justify-center"
          style={{ background: 'var(--color-accent)', color: 'var(--color-bg)' }}
        >
          {login.isPending ? 'Вход…' : 'Войти'}
        </button>
      </form>
    </div>
  );
}
