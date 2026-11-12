const { test, expect } = require('@playwright/test');

async function openDocs(page) {
    await page.goto('./');
    await expect(page.getByRole('searchbox', { name: 'Search recipes' })).toBeEnabled();
}

async function openSection(page, section, title) {
    if (await page.getByRole('button', { name: 'Open documentation menu' }).isVisible()) {
        await page.getByRole('button', { name: 'Open documentation menu' }).click();
    }
    await page.getByRole('navigation', { name: 'Documentation' }).getByRole('link', { name: title, exact: true }).click();
    await expect(page.locator(`#${section}`)).toBeInViewport();
}

test('documentation loads with complete navigation, safe links, and no failed resources', async ({ page, request }) => {
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
    page.on('requestfailed', (request) => errors.push(`${request.url()}: ${request.failure().errorText}`));
    page.on('response', (response) => { if (response.status() >= 400) errors.push(`${response.status()}: ${response.url()}`); });
    await openDocs(page);
    await expect(page).toHaveTitle('Infinity-IaC Documentation');
    await expect(page.getByRole('link', { name: 'GitHub', exact: true })).toHaveAttribute('href', 'https://github.com/M1778/infinity-iac');
    await expect(page.locator('.nav-links a')).toHaveCount(11);
    const report = await page.evaluate(() => {
        const links = [...document.querySelectorAll('a[href]')];
        const ids = [...document.querySelectorAll('[id]')].map((element) => element.id);
        return {
            badAnchors: links.filter((link) => link.getAttribute('href').startsWith('#') && !document.getElementById(link.hash.slice(1))).map((link) => link.getAttribute('href')),
            unsafeLinks: links.filter((link) => !link.getAttribute('href').startsWith('#') && !link.getAttribute('href').startsWith('https://') && link.getAttribute('href') !== 'data/catalog.json').map((link) => link.getAttribute('href')),
            duplicateIds: ids.filter((id, index) => ids.indexOf(id) !== index),
            scrollWidth: document.documentElement.scrollWidth,
            viewportWidth: window.innerWidth,
        };
    });
    expect(report.badAnchors).toEqual([]);
    expect(report.unsafeLinks).toEqual([]);
    expect(report.duplicateIds).toEqual([]);
    expect(report.scrollWidth).toBeLessThanOrEqual(report.viewportWidth);
    const metadata = await request.get('./data/catalog.json');
    expect(metadata.ok()).toBeTruthy();
    const catalog = await metadata.json();
    expect(catalog.schema).toBe('docs-catalog/v1');
    expect(catalog.recipes.length).toBeGreaterThanOrEqual(100);
    await expect(page.locator('.recipe-entry')).toHaveCount(catalog.recipes.length);
    for (const item of catalog.recipes) {
        for (const field of ['steps', 'outputs', 'health']) expect(Object.keys(item)).not.toContain(field);
        for (const input of item.inputs) expect(Object.keys(input).sort()).toEqual(['name', 'required', 'type']);
    }
    expect(errors).toEqual([]);
});

test('catalog search, status and kind filters handle real and empty states', async ({ page }) => {
    await openDocs(page);
    await openSection(page, 'recipes', 'Recipe catalog');
    const allEntries = await page.locator('.recipe-entry').count();
    await page.getByRole('searchbox', { name: 'Search recipes' }).fill('nginx');
    await expect(page.locator('#recipe-nginx')).toBeVisible();
    await page.locator('#recipe-nginx > summary').click();
    await expect(page.locator('#recipe-nginx .recipe-body')).toBeVisible();
    await expect(page.locator('#recipe-nginx .recipe-links a').first()).toHaveAttribute('href', /\/recipes\/nginx\/recipe\.yaml$/);
    await page.getByRole('button', { name: 'Reset filters' }).click();
    await page.getByLabel('Kind', { exact: true }).selectOption('script');
    const scriptEntries = page.locator('.recipe-entry[data-kind="script"]');
    const scriptCount = await scriptEntries.count();
    await expect(page.locator('.recipe-entry:not([hidden])')).toHaveCount(scriptCount);
    await page.getByRole('button', { name: 'Reset filters' }).click();
    const templateCount = await page.locator('.recipe-entry[data-status="template"]').count();
    await page.getByLabel('Status', { exact: true }).selectOption('template');
    await expect(page.locator('.recipe-entry:not([hidden])')).toHaveCount(templateCount);
    await page.getByRole('searchbox', { name: 'Search recipes' }).fill('definitely-no-such-recipe-123');
    await expect(page.getByText('No matching recipes.', { exact: true })).toBeVisible();
    await expect(page.locator('#catalog-result')).toHaveText(`Showing 0 of ${allEntries} recipes`);
    await page.getByRole('button', { name: 'Reset filters' }).click();
    await expect(page.locator('.recipe-entry:not([hidden])')).toHaveCount(allEntries);
    await expect(page.getByRole('button', { name: 'Reset filters' })).toBeDisabled();
});

test('catalog permalinks reveal matching details without executable metadata', async ({ page }) => {
    await page.goto('./#recipe-postgres');
    await expect(page.locator('#recipe-postgres')).toHaveAttribute('open', '');
    await expect(page.locator('#recipe-postgres')).toBeInViewport();
    await expect(page.locator('#recipe-postgres .recipe-note')).toContainText('not runtime certification');
    await expect(page.locator('#recipe-postgres')).not.toContainText('POSTGRES_PASSWORD=');
});

test('theme switching persists as a per-viewer convenience and follows system mode', async ({ page }, testInfo) => {
    await openDocs(page);
    const theme = page.getByRole('combobox', { name: 'Color theme' });
    await theme.selectOption('dark');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toBe('rgb(25, 25, 24)');
    await page.reload();
    await expect(theme).toHaveValue('dark');
    await theme.selectOption('light');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toBe('rgb(250, 249, 248)');
    await theme.selectOption('system');
    await expect(page.locator('html')).not.toHaveAttribute('data-theme');
    const expected = testInfo.project.use.colorScheme === 'dark' ? 'rgb(25, 25, 24)' : 'rgb(250, 249, 248)';
    expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toBe(expected);
});

test('blocked browser storage does not prevent navigation, theme, or catalog rendering', async ({ page }) => {
    await page.addInitScript(() => {
        Object.defineProperty(window, 'localStorage', { get() { throw new Error('Storage blocked'); } });
    });
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await openDocs(page);
    await page.getByRole('combobox', { name: 'Color theme' }).selectOption('dark');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await page.getByRole('searchbox', { name: 'Search recipes' }).fill('nginx');
    await expect(page.locator('#recipe-nginx')).toBeVisible();
    expect(errors).toEqual([]);
});

test('keyboard skip link and active section navigation are available', async ({ page }) => {
    await openDocs(page);
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Skip to documentation' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.locator('#main')).toBeFocused();
    await openSection(page, 'installation', 'Installation');
    await expect(page.locator('.nav-links a[href="#installation"]')).toHaveAttribute('aria-current', 'location');
    await expect(page.locator('#current-section')).toHaveText('Installation');
});

test('mobile menu traps focus, closes with Escape, and returns focus', async ({ page }, testInfo) => {
    test.skip(!testInfo.project.name.startsWith('mobile'), 'Mobile menu only');
    await openDocs(page);
    const opener = page.getByRole('button', { name: 'Open documentation menu' });
    await opener.click();
    await expect(page.locator('html')).toHaveAttribute('data-menu-open', '');
    await expect(page.locator('.brand')).toBeFocused();
    await page.keyboard.press('Shift+Tab');
    await expect(page.locator('.sidebar-footer a').last()).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('.brand')).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(page.locator('html')).not.toHaveAttribute('data-menu-open');
    await expect(opener).toBeFocused();
    await openSection(page, 'safety', 'Access & safety');
    await expect(page.locator('html')).not.toHaveAttribute('data-menu-open');
});

test('copy command succeeds and offers manual selection when clipboard is unavailable', async ({ page }) => {
    await page.addInitScript(() => {
        window.copiedText = '';
        Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (text) => { window.copiedText = text; } } });
    });
    await openDocs(page);
    const button = page.getByRole('button', { name: 'Copy offline inspection commands' });
    await button.click();
    await expect(button).toHaveText('Copied');
    expect(await page.evaluate(() => window.copiedText)).toContain('infinity-iac recipe render --name nginx --port 8080');
    await page.evaluate(() => {
        Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async () => { throw new Error('Denied'); } } });
    });
    await button.click();
    await expect(button).toHaveText('Select & copy');
    expect(await page.evaluate(() => window.getSelection().toString())).toContain('infinity-iac list recipes --output json');
    await expect(page.locator('#copy-status')).toContainText('Clipboard unavailable');
});

test('content has readable theme contrast and no horizontal page overflow at narrow widths', async ({ page }) => {
    await openDocs(page);
    const pairs = await page.evaluate(() => {
        const probe = document.createElement('span');
        document.body.append(probe);
        const color = (value) => {
            probe.style.color = value;
            return getComputedStyle(probe).color.match(/[\d.]+/g).slice(0, 3).map(Number);
        };
        const contrast = (foreground, background) => {
            const luminance = (rgb) => rgb.map((component) => component / 255).map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
            const a = luminance(color(foreground));
            const b = luminance(color(background));
            return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
        };
        const tokens = getComputedStyle(document.documentElement);
        return ['--fluent-text', '--fluent-text-secondary', '--fluent-primary'].map((token) => ({ token, ratio: contrast(tokens.getPropertyValue(token), tokens.getPropertyValue('--fluent-bg')) }));
    });
    for (const pair of pairs) expect(pair.ratio, pair.token).toBeGreaterThanOrEqual(4.5);
    for (const width of [320, 375, 768, 1024]) {
        await page.setViewportSize({ width, height: 812 });
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
    }
});

test('generated documentation remains readable without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 375, height: 812 } });
    try {
        const page = await context.newPage();
        await page.goto(`${baseURL}/`);
        await expect(page.getByRole('navigation', { name: 'Documentation' })).toBeVisible();
        await page.getByRole('navigation', { name: 'Documentation' }).getByRole('link', { name: 'Recipe catalog', exact: true }).click();
        await expect(page.locator('#recipes')).toBeInViewport();
        await expect(page.locator('.recipe-entry')).toHaveCount((await (await page.request.get(`${baseURL}/data/catalog.json`)).json()).recipes.length);
        await page.locator('#recipe-nginx > summary').click();
        await expect(page.locator('#recipe-nginx .recipe-body')).toBeVisible();
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
    } finally {
        await context.close();
    }
});
