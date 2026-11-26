// Сквозные тесты по пользовательским сценариям и сценарий видео (video.spec.ts).
// Нужны поднятые API 127.0.0.1:8088 и эмбеддер 8089 (make dev / make ml). Vite на 5177 поднимается сам,
// если не запущен (reuseExistingServer), и гасится Playwright по окончании — по своему процессу, не по порту.
import { defineConfig, devices } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL ?? 'http://localhost:5177'
const destructive = process.env.E2E_DESTRUCTIVE === '1'

export default defineConfig({
  testDir: './tests',
  globalSetup: './tests/global-setup.ts',
  // Демо-база одна: тесты читают её параллельно без вреда, но LLM-кэш и SSE лучше не дёргать гурьбой.
  fullyParallel: false,
  workers: 2,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: [['list'], ['html', { open: 'never' }]],
  timeout: 60_000,
  expect: { timeout: 15_000 },
  // @destructive меняет демо-состояние (принятие «акад»): только при E2E_DESTRUCTIVE=1.
  grepInvert: destructive ? undefined : /@destructive/,
  use: {
    baseURL: BASE_URL,
    locale: 'ru-RU',
    timezoneId: 'Europe/Moscow',
    colorScheme: 'dark',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // Без лимита клик по пропавшему элементу ждёт до таймаута теста (6 мин у видео).
    actionTimeout: 15_000,
  },
  projects: [
    {
      name: 'flows',
      testIgnore: /video\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
    {
      // Сценарий видео: 1920×1080, одним дублем, без звука (Playwright звук не пишет).
      name: 'video',
      testMatch: /video\.spec\.ts/,
      outputDir: '../artifacts/video/run',
      timeout: 6 * 60_000,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
        deviceScaleFactor: 1,
        video: { mode: 'on', size: { width: 1920, height: 1080 } },
        trace: 'off',
        screenshot: 'off',
      },
    },
  ],
  webServer: {
    command: 'pnpm --dir ../frontend dev',
    url: BASE_URL,
    reuseExistingServer: true,
    timeout: 60_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
})
