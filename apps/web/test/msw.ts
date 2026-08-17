import { setupServer } from 'msw/node';
import { http, HttpResponse, delay } from 'msw';

/**
 * MSW server for the public site.
 *
 * Far smaller than the CRM's: the site has no session and one write, so there is
 * no identity to mock.
 */
export const server = setupServer();

export { http, HttpResponse, delay };
