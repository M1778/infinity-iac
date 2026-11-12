const { defineConfig, devices } = require('@playwright/test');

const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;

module.exports = defineConfig({
    testDir: './tests/docs',
    fullyParallel: true,
    forbidOnly: Boolean(process.env.CI),
    retries: process.env.CI ? 1 : 0,
    workers: process.env.CI ? 2 : undefined,
    timeout: 30000,
    reporter: [['list'], ['html', { open: 'never' }]],
    use: {
        baseURL: process.env.DOCS_BASE_URL || 'http://127.0.0.1:4173',
        screenshot: 'only-on-failure',
        trace: 'retain-on-failure',
        actionTimeout: 10000,
        navigationTimeout: 30000,
        launchOptions: executablePath ? { executablePath } : {},
    },
    projects: [
        { name: 'desktop-light', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 }, colorScheme: 'light' } },
        { name: 'desktop-dark', use: { ...devices['Desktop Chrome'], viewport: { width: 1024, height: 768 }, colorScheme: 'dark' } },
        { name: 'mobile-light', use: { ...devices['Pixel 5'], viewport: { width: 375, height: 812 }, colorScheme: 'light' } },
        { name: 'mobile-dark', use: { ...devices['Pixel 5'], viewport: { width: 375, height: 812 }, colorScheme: 'dark' } },
    ],
    webServer: process.env.DOCS_BASE_URL ? undefined : {
        command: 'python3 -I -m http.server 4173 --bind 127.0.0.1 --directory docs-site',
        url: 'http://127.0.0.1:4173',
        reuseExistingServer: !process.env.CI,
        timeout: 30000,
    },
});
