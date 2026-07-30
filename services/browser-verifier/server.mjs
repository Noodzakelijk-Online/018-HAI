import http from 'node:http';
import { chromium } from 'playwright';

const token = String(process.env.HAI_PLAYWRIGHT_RUNNER_TOKEN || '').trim();
const allowedOrigins = new Set(String(process.env.HAI_PLAYWRIGHT_ALLOWED_ORIGINS || '').split(',').map((value) => value.trim()).filter(Boolean));
const maxBody = 8 * 1024;

function send(response, code, body) {
  response.writeHead(code, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' });
  response.end(JSON.stringify(body));
}

function trustedProfile(value) {
  if (!value || typeof value !== 'object' || typeof value.id !== 'string' || typeof value.name !== 'string' || typeof value.url !== 'string') return null;
  try {
    const target = new URL(value.url);
    if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password || target.search || target.hash || !allowedOrigins.has(target.origin)) return null;
    if (value.expectedPath && (typeof value.expectedPath !== 'string' || !value.expectedPath.startsWith('/'))) return null;
    return { id: value.id.slice(0, 80), name: value.name.slice(0, 120), url: target.toString(), expectedPath: value.expectedPath || '' };
  } catch { return null; }
}

async function verify(profile) {
  const target = new URL(profile.url);
  let browser;
  try {
    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ acceptDownloads: false, serviceWorkers: 'block' });
    await context.route('**/*', (route) => {
      try { return allowedOrigins.has(new URL(route.request().url()).origin) ? route.continue() : route.abort(); } catch { return route.abort(); }
    });
    const page = await context.newPage();
    await page.goto(target.toString(), { waitUntil: 'domcontentloaded', timeout: 15000 });
    const finalURL = new URL(page.url());
    if (!allowedOrigins.has(finalURL.origin)) return { status: 'failed', summary: 'verification redirected outside the configured local origin' };
    if (profile.expectedPath && finalURL.pathname !== profile.expectedPath) return { status: 'failed', finalPath: finalURL.pathname, summary: 'verification reached an unexpected local route' };
    const title = (await page.title()).trim().slice(0, 160);
    return { status: 'passed', finalPath: finalURL.pathname.slice(0, 240), pageTitle: title, summary: 'named local route reached without browser interaction' };
  } catch { return { status: 'failed', summary: 'named local route could not be verified' }; }
  finally { if (browser) await browser.close(); }
}

http.createServer(async (request, response) => {
  if (request.method === 'GET' && request.url === '/healthz') return send(response, 200, { status: 'ok', scope: 'read-only named local browser verification' });
  if (request.method !== 'POST' || request.url !== '/verify' || !token || request.headers['x-hai-browser-token'] !== token) return send(response, 404, { error: 'not found' });
  let body = '';
  request.on('data', (chunk) => { body += chunk; if (body.length > maxBody) request.destroy(); });
  request.on('end', async () => {
    let profile;
    try { profile = trustedProfile(JSON.parse(body)); } catch { profile = null; }
    if (!profile) return send(response, 400, { error: 'invalid named local verification profile' });
    return send(response, 200, await verify(profile));
  });
}).listen(8080, '0.0.0.0');
