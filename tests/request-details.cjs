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
            await page.route(`${baseURL}/?**`, async route => {
                const id = new URL(route.request().url()).searchParams.get('view_id');
                if (rejectDetails && id !== null) {
                    return route.fulfill({ status: 503, body: 'Unavailable' });
                }
                const response = await route.fetch();
                if (id === delayId) await new Promise(resolve => setTimeout(resolve, 250));
                const html = (await response.text()).replace(
                    "    <script>\n        document.addEventListener",
                    '    <script src="/__test-delay.js"></script>\n    <script>\n        document.addEventListener'
                );
                await route.fulfill({ response, body: html });
            });
            const capture = async path => {
                const response = await context.request.post(`${baseURL}${path}`, {
                    data: '{"message":"regression"}',
                    headers: { 'Content-Type': 'application/json' }
                });
                return (await response.text()).match(/view_id=(\d+)/)[1];
            };
            const first = await capture('/language-flash-first?test=1');
            const second = await capture('/language-flash-second');
            const listReady = page.waitForResponse(r => r.url().includes('/api/requests?'));
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
            // Slow old response must not overwrite the latest selection.
            delayId = first;
            const oldRequest = page.waitForRequest(r => new URL(r.url()).searchParams.get('view_id') === first);
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
            assert.deepEqual(errors, []);
            console.log(`PASS ${language}: no language flash/reload; selection, history, race, collapse, copy, failure`);
            await context.close();
        }
    } finally {
        await browser.close();
    }
})().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
