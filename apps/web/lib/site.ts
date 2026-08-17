/**
 * The site's own public address.
 *
 * Used for canonical URLs, hreflang and the QR payload the CRM prints onto
 * wrappers. It is read from the environment rather than hard-coded because the
 * domain is not registered yet: the client tests over a bare IP first, and the
 * canonical tags must point at whatever address the site is actually reachable
 * at, not at a name that does not resolve.
 *
 * The default is the eventual production name, so a deployment that forgets to
 * set it produces the right answer for launch rather than localhost.
 */
export function siteUrl(): string {
  return (process.env.NEXT_PUBLIC_SITE_URL ?? 'https://samari-kuhsor.tj').replace(/\/+$/, '');
}
