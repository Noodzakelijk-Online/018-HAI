import { expect, Page, test } from '@playwright/test';

/**
 * Real operator acceptance path:
 *
 * login -> local source registration -> bounded sync -> governed workflow
 * intake -> durable human approval -> one exact controlled workflow run.
 *
 * The source is the owner-scoped, read-only connected-sources mount. This test
 * never authorizes an external provider or requests an irreversible action.
 */

const email = process.env.E2E_OPERATOR_EMAIL || 'operator@example.com';
const password = process.env.E2E_OPERATOR_PASSWORD || '';
const allowMutation = process.env.E2E_ALLOW_MUTATION === 'true';

async function login(page: Page) {
  await page.goto('/');
  await page.getByTestId('login-submit').waitFor({ state: 'visible' });
  await page.getByTestId('login-email').fill(email);
  await page.getByTestId('login-password').fill(password);
  await page.getByTestId('login-submit').click();
  await expect(page).toHaveURL(/\/control-center(?:\?|$)/);
  await expect(page.getByRole('heading', { name: 'Command Center', exact: true }).first()).toBeVisible();
}

test.describe('HAI operator acceptance flow', () => {
  test.skip(
    !password || !allowMutation,
    'Set E2E_OPERATOR_PASSWORD and E2E_ALLOW_MUTATION=true only for an isolated acceptance stack.',
  );

  test('login -> source -> sync -> workflow -> approval -> exact bounded execution', async ({ page }) => {
    let sourceName = '';
    const runId = Date.now();
    const pursuitName = `E2E governed pursuit ${runId}`;
    const projectKey = `e2e-governed-${runId}`;
    const capabilityMarker = `probe-${runId}`;

    await test.step('login', async () => {
      await login(page);
    });

    await test.step('register a unique read-only execution capability', async () => {
      const response = await page.request.post('/api/v1/automation/', {
        multipart: {
          name: `E2E backend readiness ${capabilityMarker}`,
          host: 'backend',
          port: '80',
          position: '0',
          removeImage: 'false',
          launchType: 'api',
		  launchTarget: 'GET http://backend/readyz',
          dependencyNotes: `Read-only HAI backend readiness check for ${capabilityMarker}.`,
          healthCheckType: 'http',
		  healthCheckUrl: 'http://backend/readyz',
          expectedHttpStatus: '200',
        },
      });
      expect(response.status(), await response.text()).toBe(201);
      const automation = await response.json();
      expect(automation.id).toBeTruthy();
    });

    await test.step('connect a local, read-only source', async () => {
      await page.goto('/connected-sources');
      await expect(page.getByTestId('source-connect-form')).toBeVisible();
      sourceName = `E2E local source ${Date.now()}`;
      await page.getByTestId('source-name').fill(sourceName);
      await page.getByTestId('source-target').fill('.');
      await page.getByTestId('source-connect').click();

      const sourceRow = page.getByTestId('source-row').filter({ hasText: sourceName });
      await expect(sourceRow).toBeVisible();
      await sourceRow.click();
      await expect(page.getByText(sourceName, { exact: true }).last()).toBeVisible();
    });

    await test.step('run a bounded source sync', async () => {
      const sourceRow = page.getByTestId('source-row').filter({ hasText: sourceName });
      await sourceRow.getByTestId('source-sync').click();
      await expect(page.getByText(/source sync/i).first()).toBeVisible();
    });

    await test.step('create an explicit pursuit for governed work', async () => {
      await page.goto('/pursuits');
      await page.getByTestId('pursuit-new').click();
      await expect(page.getByTestId('pursuit-create-form')).toBeVisible();
      await page.getByTestId('pursuit-title').fill(pursuitName);
      await page.getByTestId('pursuit-project-key').fill(projectKey);
      await page.getByTestId('pursuit-create').click();
      await expect(page.getByTestId('pursuit-row').filter({ hasText: pursuitName })).toBeVisible();
    });

    await test.step('create an explicitly approval-gated workflow', async () => {
      await page.goto('/workflow-engine');
      await page.locator('#workflow-intake-input').fill(
        `Run the HAI backend readiness ${capabilityMarker} only after explicit human approval and attach the source-grounded result.`
      );
      await page.getByTestId('workflow-project-key').fill(projectKey);
      await page.getByRole('button', { name: 'Basic', exact: true }).click();
      await expect(page.getByRole('button', { name: 'Advanced', exact: true })).toBeVisible();
      await page.locator('details.advanced-block > summary').click();
      await page.getByTestId('workflow-source-id').fill(`e2e-source-${runId}`);
      await page.getByTestId('workflow-source-uri').fill(`local://e2e/source/${runId}`);
      await page.getByTestId('workflow-source-label').fill(sourceName);
      await page.getByTestId('workflow-create').click();
      await expect(
        page.getByTestId('workflow-approval-controls').or(page.getByTestId('workflow-runtime-selection'))
      ).toBeVisible();
    });

    await test.step('select the exact runtime and approve through the durable boundary', async () => {
      const runtimeSelection = page.getByTestId('workflow-runtime-selection');
      if (await runtimeSelection.isVisible()) {
        const exactRuntime = runtimeSelection.getByRole('button', {
          name: `E2E backend readiness ${capabilityMarker}`,
          exact: true,
        });
        await expect(exactRuntime).toBeVisible();
        const proposalResponse = page.waitForResponse((response) =>
          response.request().method() === 'POST'
          && /\/api\/v1\/workflow\/[^/]+\/proposals\/[^/]+\/resolve$/.test(new URL(response.url()).pathname)
        );
        await exactRuntime.click();
        const response = await proposalResponse;
        expect(response.ok(), await response.text()).toBeTruthy();
      } else {
        await page.getByTestId('workflow-approve').click();
      }
      await expect(page.getByTestId('workflow-approval-controls')).toHaveCount(0);
      await expect(page.getByTestId('workflow-runtime-selection')).toHaveCount(0);
      await expect(page.getByText(/approved/i).first()).toBeVisible();
    });

    await test.step('run only the selected safe, approved workflow', async () => {
      const exactRun = page.getByTestId('workflow-run-selected');
      await expect(exactRun).toBeVisible();
      await exactRun.click();
      const confirmation = page.getByRole('dialog');
      await expect(confirmation.getByText('Run this approved workflow?')).toBeVisible();
      const runResponse = page.waitForResponse((response) =>
        response.request().method() === 'POST'
        && /\/api\/v1\/workflow\/[^/]+\/run$/.test(new URL(response.url()).pathname)
      );
      await confirmation.getByRole('button', { name: 'Run this workflow', exact: true }).click();
      const response = await runResponse;
      expect(response.ok(), await response.text()).toBeTruthy();
      const result = await response.json();
      expect(result.status).toBe('completed');
      expect(result.state).toBe('completed');
      await expect(page.getByText(/workflow completed/i).first()).toBeVisible();
      await expect(page.getByText(/last operation/i).first()).toBeVisible();
      await expect(page.getByTestId('workflow-selected-state')).toHaveText('completed');
      await expect(page.getByTestId('workflow-selected-verification')).not.toHaveText('-');
    });
  });
});
