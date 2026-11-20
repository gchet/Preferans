import { expect, type Locator, type Page } from "@playwright/test";

type Gesture = "click" | "tap";

// The app drops user actions while a previous one is still running (body.busy), so a click that
// lands during that window is silently lost. Wait for idle before acting.
export async function clickIdle(page: Page, target: Locator, gesture: Gesture = "click") {
  await expect(async () => {
    await expect(page.locator("body")).not.toHaveClass(/\bbusy\b/, { timeout: 2000 });
    await target[gesture]({ timeout: 2000 });
  }).toPass({ timeout: 15000 });
}

// Clicking "Start" right after "Ready" can happen before the page has applied the new ready state.
// In that case the app only shows the waiting state and does not start the game, so retry the click
// until the lobby is gone.
export async function startGame(page: Page, start: Locator, gesture: Gesture = "click") {
  await expect(async () => {
    await expect(page.locator("body")).not.toHaveClass(/\bbusy\b/, { timeout: 2000 });
    await start[gesture]({ timeout: 2000 });
    await expect(page.locator(".center.lobby")).toBeHidden({ timeout: 3000 });
  }).toPass({ timeout: 30000 });
}

// Changing the language is ignored while the app is busy; reselect until it is applied.
export async function selectLanguage(page: Page, picker: Locator, lang: string) {
  await expect(async () => {
    await expect(page.locator("body")).not.toHaveClass(/\bbusy\b/, { timeout: 2000 });
    await picker.selectOption(lang);
    await expect(page.locator("html")).toHaveAttribute("lang", lang, { timeout: 3000 });
  }).toPass({ timeout: 20000 });
}

// "Save" exists only on the Rules and AI tabs; the other settings tabs save on change. Finish editing settings
// either way and close the panel.
export async function saveSettings(page: Page) {
  const save = page.locator("#settings-form > button.primary");
  if (await save.isVisible()) {
    await save.click();
    return;
  }
  await page.waitForTimeout(400);
  await expect(page.locator("body")).not.toHaveClass(/\bbusy\b/, { timeout: 5000 });
  await page.keyboard.press("Escape");
}
