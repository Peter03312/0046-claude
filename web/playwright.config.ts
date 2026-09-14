import { defineConfig, devices } from '@playwright/test'

// Browser tests exercise the real Vite dev server backed by a real Go API.
// The API process (and its temp SQLite file) is started once by
// scripts/e2e-api.sh; here we only launch the Vite dev server which proxies
// /api to VITE_API_TARGET.
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'npm run dev -- --port 5173',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
})
