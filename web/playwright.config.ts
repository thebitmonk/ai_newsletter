import { defineConfig, devices } from "@playwright/test";

// E2E config. Assumes the full stack is already running via scripts/e2e-up.sh
// (Postgres + NSQ + Redis + Firebase emulator + Go backend on :8080 +
// Nuxt dev on :3000). Vitest is unaffected — these specs live in e2e/.

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    headless: true,
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
});
