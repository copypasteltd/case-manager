import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './storage_operations_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './storage_operations_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.getByRole('link', { name: '文件复制运维', exact: true }).click();
  const shots = [], layouts = [], prefix = 'storage-operations-web';
  const button = name => page.getByRole('button', { name, exact: true });
  async function get(route) {
    const r = await context.request.get(fixture.origin + '/v1' + route);
    assert.equal(r.status(), 200); return (await r.json()).data;
  }
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    shots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  const row = page.locator('[data-replica-id="' + fixture.replica_id + '"]');
  await row.getByRole('button', { name: '核对副本与人工记录', exact: true }).click();
  const before = await get('/storage/replicas/' + fixture.replica_id);
  assert.equal(before.item.state, 'failed'); assert.equal(before.can_retry, true);
  assert.equal(before.item.file_id, fixture.file_id);
  const detail = page.locator('section.panel').filter({ has: page.getByRole('heading', { name: '副本详情与人工重试', exact: true }) });
  for (const width of [390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1100 });
    await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
    const size = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, width: document.documentElement.scrollWidth }));
    layouts.push({ width, ...size });
    await writeFile(path.join(evidence, 'layout-probe.json'), JSON.stringify(layouts, null, 2) + '\n');
    assert.ok(size.width <= width + 2, JSON.stringify(size));
  }
  await page.setViewportSize({ width: 1440, height: 1100 });
  await detail.scrollIntoViewIfNeeded(); await shot('failure');
  checks.push('Real authenticated Web metadata inventory and failed synthetic copy are visible at three viewport widths; no provider coordinates or credentials are exposed.');
  await button('核对并安排复制重试').click();
  const review = page.getByRole('dialog', { name: '核对文件复制重试', exact: true });
  await review.waitFor();
  assert.equal(await button('确认安排复制重试').isDisabled(), true);
  await shot('review');
  let saved;
  const recording = await recordViewport(page, app, prefix, async () => {
    await page.getByLabel('复制重试原因', { exact: true }).fill('本人已核对准确版本、可核验来源与停止任务，安排复制重试');
    assert.equal(await button('确认安排复制重试').isDisabled(), true);
    await page.getByLabel('已核对准确文件版本、副本及当前任务', { exact: true }).check();
    const pending = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/storage/replicas/' + fixture.replica_id + '/retry');
    await button('确认安排复制重试').click();
    const response = await pending; assert.equal(response.status(), 200);
    assert.equal(response.request().postDataJSON().basis_sha256, before.item.basis_sha256);
    saved = (await response.json()).data;
    assert.equal(saved.replica_id, fixture.replica_id);
    assert.equal(saved.after.state, 'queued'); assert.equal(saved.result_version, before.item.version + 1);
    assert.ok(saved.business_operation_id); assert.equal(saved.after.job, null);
    await page.locator('[data-retry-receipt="' + saved.id + '"]').waitFor();
    await detail.scrollIntoViewIfNeeded(); await shot('queued');
  });
  checks.push('Actual explicit review submits the fixed version and basis, creates one ordinary retry and immutable queue receipt and keeps queue status distinct from verified replication.');
  const current = await get('/storage/replicas/' + fixture.replica_id);
  assert.equal(current.item.state, 'queued');
  const history = await get('/storage/replica-retries?replica_id=' + fixture.replica_id + '&limit=20');
  assert.equal(history.items.length, 1); assert.equal(history.items[0].id, saved.id);
  assert.equal(history.items[0].before.state, 'failed'); assert.equal(history.items[0].after.state, 'queued');
  await detail.getByText('固定人工重试记录', { exact: true }).waitFor();
  checks.push('Real API/PG readback retains the exact original failure and queue snapshots in one receipt; source content and successful-cloud acceptance are not fabricated.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ recording, screenshots: shots, layouts, synthetic_data: true, scope: 'Real Web/native HTTP/PG using an isolated synthetic object-store fault; no real cloud, device, model or production acceptance.' }, null, 2) + '\n');
}, 'storage-operations');
