// Actual authenticated UI input and native API/PG; no substituted responses.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './workbench_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence, openPage }) => {
  const root = (p = page) => mobile ? mobileScreen(p) : p;
  const field = (label, p = page) => mobile ? root(p).locator('input[aria-label="' + label + '"]') : p.getByLabel(label, { exact: true });
  const button = (label, p = page) => root(p).getByRole('button', { name: label, exact: true });
  let bearer, summaryReads = 0;
  page.on('request', request => {
    if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization;
    if (new URL(request.url()).pathname === '/v1/dashboard/business') summaryReads++;
  });
  async function login(p, who, mfa) {
    await p.goto(fixture.origin);
    await field('内部账号', p).fill(who.login); await field('密码', p).fill(who.password);
    await button(mobile ? '登录工作空间 ↗' : '登录工作空间', p).click();
    if (mfa) { await field('六位验证码', p).fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续', p).click(); }
    await root(p).getByText('业务待办', { exact: true }).waitFor();
  }
  const home = async () => {
    if (mobile) {
      if (new URL(page.url()).hash !== '#/pages/home/index') {
        await page.goBack();
        await page.waitForURL(url => url.hash === '#/pages/home/index');
      }
    } else await page.goto(fixture.origin + '/');
  };
  const panel = () => mobile ? root().locator('.cp-section').filter({ hasText: '我负责的合同到期 · ' }) : page.getByRole('region', { name: '业务待办汇总' });
  const ready = () => root().getByText(mobile ? '待我审批 · 6' : '业务待办', { exact: true }).waitFor();
  await login(page, fixture.alice, true);
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = () => mobile ? { Authorization: bearer } : { Origin: fixture.origin };
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, { headers: headers() }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  const current = await get('/dashboard/business');
  assert.equal(current.scope, 'my_readable_work'); assert.equal(current.groups.length, 3);
  assert.deepEqual(current.groups.map(g => [g.kind, g.count, g.items.length, g.has_more]), [['approvals', 6, 5, true], ['contracts', 2, 2, false], ['subscription_periods', 1, 1, false]]);
  const approval = current.groups[0].items[0], contract = current.groups[1].items[0], period = current.groups[2].items[0];
  await ready(); await root().getByText(approval.title, { exact: true }).first().waitFor();
  assert.equal(await page.locator('.business-workbench-script-marker').count(), 0);
  checks.push({ name: 'Genuine login and MFA expose exact current-owner counts with bounded samples and literal titles', passed: true });

  const prefix = mobile ? 'workbench-mobile' : 'workbench-web', directory = path.join(app, '../docs/media/screenshots');
  await mkdir(directory, { recursive: true }); const shots = [];
  async function shot(suffix, p = page) {
    await assertNoCredentials(p); const file = prefix + '-' + suffix + '.png';
    await p.screenshot({ path: path.join(directory, file), fullPage: false });
    shots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) });
  }
  if (mobile) await root().getByText('业务待办', { exact: true }).scrollIntoViewIfNeeded(); else await panel().scrollIntoViewIfNeeded();
  await shot('summary');
  const recordingViewport = page.viewportSize();
  if (!mobile) await page.setViewportSize({ width: 760, height: 1000 });
  const recording = await recordViewport(page, app, prefix, async () => {
    for (const label of ['我负责的合同到期' + (mobile ? ' · 2' : ''), '我负责的订阅待核对' + (mobile ? ' · 1' : '')]) {
      await root().getByText(label, { exact: true }).scrollIntoViewIfNeeded();
      await page.waitForTimeout(1000);
    }
    await root().getByText('业务待办', { exact: true }).scrollIntoViewIfNeeded();
    await page.waitForTimeout(800);
    const [refreshed] = await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/v1/dashboard/business'),
      button(mobile ? '刷新工作台' : '刷新业务待办').click(),
    ]);
    assert.equal(refreshed.status(), 200); await ready();
    await root().getByText('业务待办', { exact: true }).scrollIntoViewIfNeeded();
    await page.waitForTimeout(1000);
  });
  if (!mobile) await page.setViewportSize(recordingViewport);
  checks.push({ name: 'Authenticated synthetic recording contains real workbench pixels without credentials', passed: true });

  const targets = [
    [approval, '/requests/' + approval.id, 'kind=request&id=' + approval.id],
    [contract, '/contracts/' + contract.id, 'kind=contract&id=' + contract.id],
    [period, '/subscription-periods/' + period.id, '/pages/subscription-period/index?id=' + period.id],
  ];
  for (const [item, webPath, mobilePath] of targets) {
    await home(); await ready();
    const [response] = await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + webPath && r.request().method() === 'GET'),
      root().getByText(item.title, { exact: true }).first().click().then(() => page.waitForURL(url => mobile ? url.hash.includes(mobilePath) : url.pathname === webPath)),
    ]);
    assert.equal(response.status(), 200); assert.equal((await response.json()).data.id, item.id);
  }
  checks.push({ name: 'All three actual UI links open ordinary currently authorized exact business resources', passed: true });

  await home(); await ready(); const beforeRefresh = summaryReads;
  const [refreshed] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/v1/dashboard/business'),
    button(mobile ? '刷新工作台' : '刷新业务待办').click(),
  ]);
  assert.equal(refreshed.status(), 200);
  await ready(); assert.ok(summaryReads > beforeRefresh);
  const narrow = mobile ? 360 : 760; await page.setViewportSize({ width: narrow, height: 1000 });
  await root().getByText('业务待办', { exact: true }).scrollIntoViewIfNeeded();
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2), 'Workbench must not force a wider page.');
  await shot('narrow'); checks.push({ name: 'Explicit user refresh reaches the backend and responsive layouts fit the current viewport', passed: true });

  const member = await openPage({ viewport: mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 } });
  await login(member.page, { login: 'member', password: 'member-test-password-long' }, false);
  if (mobile) {
    await root(member.page).getByText('待我审批 · 0', { exact: true }).waitFor();
    await root(member.page).getByText('我负责的合同到期 · 0', { exact: true }).waitFor();
    await root(member.page).getByText('我负责的订阅待核对 · 0', { exact: true }).waitFor();
  } else {
    for (const label of ['待我审批', '我负责的合同到期', '我负责的订阅待核对']) assert.equal(await member.page.getByLabel(label + '数量', { exact: true }).textContent(), '0');
  }
  assert.equal(await root(member.page).getByText(approval.title, { exact: true }).count(), 0);
  await root(member.page).getByText('业务待办', { exact: true }).scrollIntoViewIfNeeded(); await shot('other-member', member.page);
  checks.push({ name: 'A separate genuine member session cannot see another user\'s counts or business titles', passed: true });

  const revoked = await context.request.post(fixture.api_origin + '/__fixture/workbench/revoke-approval', { headers: { Authorization: 'Bearer ' + fixture.stop_token } });
  assert.equal(revoked.status(), 204);
  const [updatedResponse] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/v1/dashboard/business'),
    button(mobile ? '刷新工作台' : '刷新业务待办').click(),
  ]);
  assert.equal(updatedResponse.status(), 200);
  assert.equal((await updatedResponse.json()).data.groups[0].count, 0);
  if (mobile) await root().getByText('待我审批 · 0', { exact: true }).waitFor();
  else assert.equal(await page.getByLabel('待我审批数量', { exact: true }).textContent(), '0');
  assert.equal(await root().getByText(approval.title, { exact: true }).count(), 0);
  checks.push({ name: 'Actual committed role revocation removes approval counts and cached titles after human refresh', passed: true });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots: shots, recording, synthetic_data: true, real_device: false, api_responses_mocked: false }, null, 2) + '\n');
});
