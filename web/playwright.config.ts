import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "tests",
  workers: 1,
  // GitHub runners sometimes reset a connection to the local app (ECONNRESET); one retry in CI.
  retries: process.env.CI ? 1 : 0,
  timeout: 60000,
  use: {
    headless: true,
    launchOptions: {
      executablePath:
        process.env.CHROME_PATH ||
        "C:/Program Files/Google/Chrome/Application/chrome.exe",
    },
  },
  outputDir: "../.build/playwright",
});
