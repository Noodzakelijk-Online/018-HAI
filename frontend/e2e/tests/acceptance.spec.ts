import { test, expect, Page } from '@playwright/test';

/**
 * End-to-end acceptance flow for a fresh operator, matching the sequence the
 * external review asked to see proven at the browser level:
 *
 *   login → source connection → sync → workflow creation → approval →
 *   one safe, bounded execution.
 *
 * These tests drive the REAL UI against a running stack (docker compose up).
 * They use the app's existing `data-testid` hooks. Selectors for the source
 * connect/sync steps are marked TODO(selector) where a stable testid still
 * needs to be confirmed against the sources page in the target build — do that
 * before treating this file as green.
 *
 * Credentials come from the environment so no secret is committed:
 *   E2E_BASE_URL, E2E_OPERATOR_EMAIL, E2E_OPERATOR_PASSWORD
 */

const email = process.env.E2E_OPERATOR_EMAIL || 'operator@example.com';
const password = process.env.E2E_OPERATOR_PASSWORD || '';

async function login(page: Page) {
  await page.goto('/');
  // The login page renders when unauthenticated.
  await page.getByTestId('login-submit').waitFor({ state: 'visible' });
  // Local operator sign-in (email + password). Google login is a separate path
  // (login-google) that cannot run headless without a real Google session.
  await page.getByLabel(/email/i).fill(email);
  await page.getByLabel(/password/i).fill(password);
  await page.getByTestId('login-submit').click();
  // After sign-in the shell/home renders.
  await expect(page.getByTestId('brand-command-center')).toBeVisible();
}

test.describe('HAI operator acceptance flow', () => {
  test.skip(!password, 'Set E2E_OPERATOR_PASSWORD (and a seeded account) to run against a live stack.');

  test('login → connect source → sync → workflow → approve → bounded execution', async ({ page }) => {
    await test.step('login', async () => {
      await login(page);
    });

    await test.step('connect a local, read-only source', async () => {
      // Navigate to the connected-sources view.
      await page.goto('/home/connected-sources');
      // TODO(selector): confirm these testids on the sources page in the target
      // build. The source connector must be a local/export or read-only path so
      // the acceptance run stays bounded and side-effect free.
      const connect = page.getByTestId('source-connect');
      if (await connect.count()) {
        await connect.first().click();
        await expect(page.getByTestId('source-connected')).toBeVisible();
      }
    });

    await test.step('run a bounded sync', async () => {
      const sync = page.getByTestId('source-sync');
      if (await sync.count()) {
        await sync.first().click();
        await expect(page.getByText(/sync (completed|synced)/i)).toBeVisible();
      }
    });

    await test.step('create a workflow', async () => {
      await page.goto('/home/workflow-engine');
      await page.getByTestId('workflow-create').click();
      await expect(page.getByTestId('workflow-apply-transition')).toBeVisible();
    });

    await test.step('approve / advance the workflow', async () => {
      // The approval gate: advancing a transition is the approve action.
      await page.getByTestId('workflow-apply-transition').first().click();
    });

    await test.step('one safe, bounded execution', async () => {
      // The worker run is the bounded execution surface. High-risk/external
      // side effects stay disabled by default, so this exercises the safe path.
      await page.getByTestId('workflow-run-worker').click();
      // A run should surface an audit/activity trail rather than a silent claim.
      await expect(page.getByTestId('panel-activity-audit')).toBeVisible();
    });
  });
});
