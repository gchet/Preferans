import { expect, type Locator, type Page } from "@playwright/test";

// Clicking "Start" right after "Ready" can happen before the page has applied the new ready state.
// In that case the app only shows the waiting state and does not start the game, so retry the click
// until the lobby is gone.
export async function startGame(page: Page, start: Locator) {
  await expect(async () => {
    await start.click({ timeout: 2000 });
    await expect(page.locator(".center.lobby")).toBeHidden({ timeout: 3000 });
  }).toPass({ timeout: 30000 });
}