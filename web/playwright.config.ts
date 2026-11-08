import { defineConfig, devices } from "@playwright/test";

// Two servers: the e2e API (real app and pipeline on throwaway _test databases, with the
// extractor, model and ranker faked; api/test/e2e-server.ts), and the built site under
// `vite preview`, which sends the production CSP and proxies /api to it. The API needs the
// environment from scripts/test-env.sh, as the integration tests do.
export default defineConfig({
  testDir: "e2e",
  reporter: "line",
  workers: 1,
  // Whole journeys: sign in, upload, parse, several pages, each checked with axe twice.
  timeout: 120_000,
  use: { baseURL: "http://localhost:5173", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "npm --prefix ../api run e2e:server",
      url: "http://127.0.0.1:8080/api/health",
      reuseExistingServer: false,
      timeout: 180_000,
      gracefulShutdown: { signal: "SIGTERM", timeout: 15_000 },
    },
    {
      command: "npm run preview",
      url: "http://localhost:5173",
      reuseExistingServer: false,
      timeout: 30_000,
    },
  ],
});
