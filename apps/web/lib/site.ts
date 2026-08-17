import 'server-only';

/**
 * The site's own public address.
 *
 * Used for canonical URLs, hreflang, the sitemap and robots.txt — all of which
 * are generated on the SERVER, which is why this reads a plain environment
 * variable rather than a NEXT_PUBLIC_ one.
 *
 * That distinction is not cosmetic. Next inlines every NEXT_PUBLIC_ value into
 * the bundle at BUILD time, so a container that sets it at runtime changes
 * nothing: the image was built with whatever the builder had. This was a real
 * defect — robots.txt served `Allow: /` while running under a bare IP, because
 * it was reading a value frozen at build time to the production domain. The
 * whole point of the IP-first deploy sequence is that the address is not known
 * when the image is built.
 *
 * `import 'server-only'` is the enforcement: if a client component ever imports
 * this, the build fails rather than silently reading a stale constant.
 *
 * The default is the eventual production name, so a deployment that forgets to
 * set it produces the right answer for launch rather than localhost.
 */
export function siteUrl(): string {
  const configured =
    process.env.PUBLIC_SITE_URL ??
    // Kept as a fallback for `next dev`, where a .env.local commonly carries the
    // NEXT_PUBLIC_ form. Never relied on in a container.
    process.env.NEXT_PUBLIC_SITE_URL ??
    'https://samari-kuhsor.tj';
  return configured.replace(/\/+$/, '');
}
