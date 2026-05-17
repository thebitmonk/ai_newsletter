// Golden onboarding flow: signup → emulator-verify → land on / → create
// publication → see it on the dashboard.
//
// Requires the full stack running (scripts/e2e-up.sh) with the Firebase
// Auth emulator on :9099. The spec uses Firebase's emulator-only HTTP
// admin endpoint to auto-verify the user instead of clicking a real email
// link.

import { expect, test } from "@playwright/test";

const EMULATOR = process.env.FIREBASE_AUTH_EMULATOR_HOST ?? "localhost:9099";
const PROJECT_ID = process.env.FIREBASE_PROJECT_ID ?? "ai-newsletter-dev";

async function autoVerifyEmail(uid: string) {
  // Emulator admin: update user. https://firebase.google.com/docs/emulator-suite/connect_auth
  const url = `http://${EMULATOR}/emulator/v1/projects/${PROJECT_ID}/accounts:update`;
  const res = await fetch(url, {
    method: "POST",
    headers: { Authorization: "Bearer owner", "Content-Type": "application/json" },
    body: JSON.stringify({ localId: uid, emailVerified: true }),
  });
  if (!res.ok) throw new Error(`emulator verify failed: ${res.status} ${await res.text()}`);
}

async function deleteUser(uid: string) {
  // Best-effort cleanup; ignore failures (test still passed if assertions did).
  try {
    await fetch(
      `http://${EMULATOR}/emulator/v1/projects/${PROJECT_ID}/accounts/${uid}`,
      { method: "DELETE", headers: { Authorization: "Bearer owner" } },
    );
  } catch {/* swallow */}
}

test("signup → verify → login → create publication", async ({ page }) => {
  const email = `e2e-${Date.now()}@example.com`;
  const password = "playwright-test-12345";

  // 1. Signup.
  await page.goto("/signup");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password (8+ chars)").fill(password);
  await page.getByRole("button", { name: /^Sign up$/ }).click();

  // 2. Lands on /verify-email.
  await page.waitForURL("**/verify-email", { timeout: 15_000 });

  // 3. Read the Firebase UID from emulator and flip emailVerified.
  const listRes = await fetch(
    `http://${EMULATOR}/emulator/v1/projects/${PROJECT_ID}/accounts:query`,
    {
      method: "POST",
      headers: { Authorization: "Bearer owner", "Content-Type": "application/json" },
      body: JSON.stringify({ tenantId: "" }),
    },
  );
  const users = (await listRes.json()) as { userInfo?: { localId: string; email: string }[] };
  const probe = (users.userInfo ?? []).find((u) => u.email === email);
  if (!probe) throw new Error("user not found in emulator after signup");
  await autoVerifyEmail(probe.localId);

  // 4. Verify-email page polls every 3s; wait until it redirects to /.
  await page.waitForURL("**/", { timeout: 15_000 });
  await expect(page.getByText("You're signed in")).toBeVisible();

  // 5. Navigate to publications and create one.
  await page.goto("/publications");
  await page.getByRole("link", { name: "New publication" }).first().click();
  await page.waitForURL("**/publications/new");

  const name = `e2e pub ${Date.now()}`;
  await page.getByLabel("Name").fill(name);
  await page.getByLabel(/Brief/).fill("e2e test brief");
  await page.getByLabel(/Timezone/).fill("UTC");
  await page.getByLabel(/lead time/).fill("24h");
  await page.getByRole("button", { name: /^Create$/ }).click();

  // 6. Should land on the new pub's calendar.
  await page.waitForURL(/\/publications\/[0-9a-f-]+\/calendar/, { timeout: 15_000 });

  // 7. Back on the list, the new publication is visible.
  await page.goto("/publications");
  await expect(page.getByRole("heading", { name })).toBeVisible({ timeout: 5_000 });

  // Cleanup.
  await deleteUser(probe.localId);
});
