// Browser regression check against a running local instance (default: port 8282).
// Requires Playwright for development only; the Go binary has no extra dependencies.
// Optional environment: REQUESTBIN_URL, PLAYWRIGHT_MODULE, BROWSER_EXECUTABLE_PATH.
const assert = require('node:assert/strict');
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const baseURL = process.env.REQUESTBIN_URL || 'http://127.0.0.1:8282';

(async () => {
    const browser = await chromium.launch({
        headless: true,
        ...(process.env.BROWSER_EXECUTABLE_PATH
            ? { executablePath: process.env.BROWSER_EXECUTABLE_PATH } : {})
    });
    try {
        for (const language of ['zh-CN', 'en']) {
            const context = await browser.newContext({ locale: language });
            await context.grantPermissions(['clipboard-read', 'clipboard-write']);
            const page = await context.newPage();
            const errors = [];
            page.on('pageerror', error => errors.push(error.message));
            const detailFetches = [];
            const oldAPIFetches = [];
            page.on('request', request => {
                const url = new URL(request.url());
                if (request.resourceType() === 'fetch' && url.pathname.startsWith('/api/requests')) {
                    oldAPIFetches.push(url.pathname);
                }
                if (request.resourceType() === 'fetch' && (url.searchParams.has('view_id')
                    || /^\/__tiny_requestbin\/api\/requests\/\d+$/.test(url.pathname))) detailFetches.push(url.pathname);
            });
            const samples = [];
            await page.exposeFunction('sampleLanguage', text => samples.push(text));
            await page.addInitScript(() => {
                localStorage.setItem('autoRefreshEnabled', 'false');
                const sample = () => {
                    const prefix = document.getElementById('subtitle-prefix');
                    if (prefix) window.sampleLanguage(prefix.textContent.trim());
                    requestAnimationFrame(sample);
                };
                requestAnimationFrame(sample);
            });
            // Avoid CDN availability affecting a local behavior check.
            await page.route('https://cdn.tailwindcss.com/**', route => route.fulfill({ body: '' }));
            // A parser pause makes the old pre-translation paint deterministic.
            await page.route('**/__test-delay.js', async route => {
                await new Promise(resolve => setTimeout(resolve, 150));
                await route.fulfill({ body: '' });
            });
            let rejectDetails = false;
            let delayId = null;
            await page.route(`${baseURL}/__tiny_requestbin/api/requests/*`, async route => {
                const id = new URL(route.request().url()).pathname.split('/').pop();
                if (!/^\d+$/.test(id)) return route.continue();
                if (rejectDetails) {
                    return route.fulfill({ status: 503, body: 'Unavailable' });
                }
                const response = await route.fetch();
                if (id === delayId) await new Promise(resolve => setTimeout(resolve, 250));
                if (response.status() === 200) assert.match(response.headers()['content-type'], /application\/json/);
                await route.fulfill({ response });
            });
            await page.route(`${baseURL}/?**`, async route => {
                const response = await route.fetch();
                const html = (await response.text()).replace(
                    "    <script>\n        document.addEventListener",
                    '    <script src="/__test-delay.js"></script>\n    <script>\n        document.addEventListener'
                );
                await route.fulfill({ response, body: html });
            });
            const body = JSON.stringify({
                message: 'regression', integer: '__integer__',
                html: '<img src=x onerror="window.bodyExecuted=true">',
                escaped: 'quote: " backslash: \\ newline: \n', empty: {}, values: [[], {}, true, null]
            }).replace('"__integer__"', '9007199254740993');
            const capture = async (path, data = body) => {
                const response = await context.request.post(`${baseURL}${path}`, {
                    data: Buffer.from(data),
                    headers: { 'Content-Type': 'application/json', 'X-Test': '<b>literal header</b>' }
                });
                return (await response.text()).match(/view_id=(\d+)/)[1];
            };
            const first = await capture('/language-flash-first?test=1&tag=one&tag=two');
            const second = await capture('/language-flash-second');
            const listReady = page.waitForResponse(r => r.url().includes('/__tiny_requestbin/api/requests?'));
            await page.goto(`${baseURL}/?lang=${language}&path=language-flash`);
            await listReady;
            await page.waitForFunction(() => document.querySelector('main') && typeof currentLang !== 'undefined');
            await page.evaluate(() => {
                window.originalHeader = document.querySelector('header');
                window.originalList = document.querySelector('aside');
                window.originalMain = document.querySelector('main');
            });
            const waitForDetails = id => page.waitForFunction(id =>
                document.querySelector('main h2')?.textContent.includes(`#${id}`)
                && !document.querySelector('main').hasAttribute('aria-busy'), id);
            const click = id => page.locator(`a[data-request-id="${id}"]`).click();
            samples.length = 0;
            await click(first);
            await waitForDetails(first);
            assert.equal(detailFetches.at(-1), `/__tiny_requestbin/api/requests/${first}`,
                'Selecting a request must fetch only its JSON details, not the full HTML page');
            const renderedBody = await page.locator('#body-text').textContent();
            assert.match(renderedBody, /9007199254740993/, 'JSON integer precision was lost');
            assert.deepEqual(JSON.parse(renderedBody), JSON.parse(body));
            assert.match(renderedBody, /\n  "message":/, 'JSON body was not formatted');
            assert.equal(await page.locator('#body-text img').count(), 0);
            assert.equal(await page.evaluate(() => window.bodyExecuted), undefined);
            assert.match(await page.locator('#headers-content').textContent(), /<b>literal header<\/b>/);
            assert.match(await page.locator('#query-content').textContent(), /one, two/);
            assert.equal(await page.locator('#query-content').getAttribute('data-has-content'), 'true');
            await page.waitForTimeout(200);
            const expectedPrefix = language === 'zh-CN' ? '发送请求到' : 'Send requests to';
            assert(samples.length > 0);
            assert(samples.every(text => text === expectedPrefix), `Language flash: ${[...new Set(samples)]}`);
            assert(await page.evaluate(() => window.originalHeader === document.querySelector('header')
                && window.originalList === document.querySelector('aside')
                && window.originalMain === document.querySelector('main')), 'Page was reloaded');
            assert.equal(new URL(page.url()).searchParams.get('path'), 'language-flash');
            assert.equal(new URL(page.url()).searchParams.get('lang'), language);
            assert(await page.locator(`a[data-request-id="${first}"]`).evaluate(el => el.classList.contains('bg-indigo-50')));
            await page.locator('#headers-toggle').click();
            await click(second);
            await waitForDetails(second);
            assert(await page.locator('#headers-content').evaluate(el => el.classList.contains('hidden')));
            await page.locator('#copy-body-btn').click();
            await page.waitForFunction(() => document.querySelector('#copy-body-btn').classList.contains('bg-green-600'));
            assert.match(await page.evaluate(() => navigator.clipboard.readText()), /regression/);
            await page.goBack();
            await waitForDetails(first);
            await page.goForward();
            await waitForDetails(second);
            await page.goBack();
            await waitForDetails(first);
            await page.goBack();
            await waitForDetails(second);
            assert.equal(new URL(page.url()).searchParams.get('view_id'), null);
            await page.goForward();
            await waitForDetails(first);
            await page.goForward();
            await waitForDetails(second);
            // Slow old response must not overwrite the latest selection.
            delayId = first;
            const oldRequest = page.waitForRequest(r => new URL(r.url()).pathname === `/__tiny_requestbin/api/requests/${first}`);
            await click(first);
            await oldRequest;
            await click(second);
            await waitForDetails(second);
            await page.waitForTimeout(350);
            assert.equal(new URL(page.url()).searchParams.get('view_id'), second);
            assert.match(await page.locator('main h2').first().textContent(), new RegExp(`#${second}`));
            rejectDetails = true;
            const oldURL = page.url();
            await click(first);
            await page.waitForFunction(() => !document.getElementById('detailsLoadError').classList.contains('hidden'));
            assert.equal(page.url(), oldURL, 'Failed load changed the URL');
            assert.match(await page.locator('main h2').first().textContent(), new RegExp(`#${second}`));
            rejectDetails = false;
            await context.request.post(`${baseURL}/__tiny_requestbin/api/requests/clear`);
            const missingResponse = page.waitForResponse(r => new URL(r.url()).pathname === `/__tiny_requestbin/api/requests/${first}`);
            await click(first);
            assert.equal((await missingResponse).status(), 404);
            await page.waitForFunction(() => !document.getElementById('detailsLoadError').classList.contains('hidden'));
            assert.equal(page.url(), oldURL, 'Missing request changed the URL');
            assert.match(await page.locator('main h2').first().textContent(), new RegExp(`#${second}`));

            // An initially empty page can render details from the shared template.
            await page.goto(`${baseURL}/?lang=${language}&path=initially-empty`);
            await page.waitForFunction(() => document.querySelector('main [data-i18n="welcome_title"]'));
            const empty = await capture('/initially-empty', '');
            await page.locator('#listRefreshButton').click();
            await page.locator(`a[data-request-id="${empty}"]`).waitFor();
            await click(empty);
            await waitForDetails(empty);
            assert(await page.locator('[data-detail="body-wrapper"]').evaluate(el => el.classList.contains('hidden')));
            assert(await page.locator('main [data-i18n="no_body"]').isVisible());
            assert.equal(await page.locator('#query-content').getAttribute('data-has-content'), 'false');
            await context.request.post(`${baseURL}/__tiny_requestbin/api/requests/clear`);
            await page.goBack();
            await page.waitForFunction(() => document.querySelector('main [data-i18n="welcome_title"]')
                && !document.querySelector('main').hasAttribute('aria-busy'));
            const matching = await capture('/search-scope/match');
            await capture('/search-other/skip');
            const searchResponse = page.waitForResponse(response => {
                const url = new URL(response.url());
                return url.pathname === '/__tiny_requestbin/api/requests'
                    && url.searchParams.get('path') === 'search-scope';
            });
            await page.locator('#pathSearchInput').fill('search-scope');
            assert.equal((await (await searchResponse).json()).total, 1);
            await page.locator(`a[data-request-id="${matching}"]`).waitFor();
            assert.equal(await page.locator('.request-link').count(), 1);
            assert.deepEqual(oldAPIFetches, [], 'The UI still fetches an old API path');
            assert.deepEqual(errors, []);
            console.log(`PASS ${language}: namespaced APIs/search; JSON details, language, history, race, collapse, copy, errors, empty state, safe text/integers`);
            await context.close();
        }
    } finally {
        await browser.close();
    }
})().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
