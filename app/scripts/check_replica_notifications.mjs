import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './replica_notifications_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './replica_notifications_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.goto(fixture.origin + '/notifications');
  const shots = [], layouts = [], prefix = 'replica-notifications-web';
  async function get(route) {
    const response = await context.request.get(fixture.origin + '/v1' + route);
    assert.equal(response.status(), 200); return (await response.json()).data;
  }
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    shots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  const row = () => page.locator('[data-notification-id="' + fixture.notification_id + '"]');
  await row().getByRole('heading', { name: '文件副本已恢复并校验', exact: true }).waitFor();
  const before = await get('/notifications/' + fixture.notification_id);
  assert.equal(before.current_state, 'verified'); assert.equal(before.attention_required, false);
  assert.equal(before.delivery_seq, 2); assert.equal(before.read_seq, 1); assert.equal(before.read, false);
  assert.equal(before.target.version_id, fixture.version_id); assert.equal(before.target.replica_id, fixture.replica_id);
  for (const width of [390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1100 });
    await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
    const size = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, width: document.documentElement.scrollWidth }));
    layouts.push({ width, ...size });
    await writeFile(path.join(evidence, 'layout-probe.json'), JSON.stringify(layouts, null, 2) + '\n');
    assert.ok(size.width <= width + 2, JSON.stringify(size));
  }
  await page.setViewportSize({ width: 1440, height: 1100 });
  await row().scrollIntoViewIfNeeded(); await shot('recovery');
  checks.push('Actual worker failure/read/retry/verified events produce the same unread recovered thread, current exact version/replica and responsive authenticated Web at three widths.');
  const recording = await recordViewport(page, app, prefix, async () => {
    await row().getByRole('button', { name: '打开相关资料', exact: true }).click();
    await page.getByRole('heading', { name: '本次引用的内容版本', exact: true }).waitFor();
    const url = new URL(page.url());
    assert.equal(url.pathname, '/files/' + fixture.file_id);
    assert.equal(url.searchParams.get('version'), fixture.version_id);
    assert.equal(url.searchParams.get('replica'), fixture.replica_id);
    const panel = page.getByRole('region', { name: '通知对应版本', exact: true });
    await panel.getByText('通知对应副本', { exact: true }).waitFor();
    await panel.scrollIntoViewIfNeeded(); await shot('version');
    const version = await get('/files/' + fixture.file_id + '/versions/' + fixture.version_id);
    assert.equal(version.replicas.find(r => r.id === fixture.replica_id)?.state, 'verified');
    checks.push('Opening the actual notification selects the exact version through the ordinary current-authority API and highlights the matching verified replica.');
    await page.goto(fixture.origin + '/notifications');
    await row().getByRole('heading', { name: '文件副本已恢复并校验', exact: true }).waitFor();
    const pending = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/notifications/' + fixture.notification_id + '/read');
    await row().getByRole('button', { name: '标记已读', exact: true }).click();
    const response = await pending; assert.equal(response.status(), 200);
    assert.equal(response.request().postDataJSON().delivery_seq, 2);
    await row().getByText('已读', { exact: true }).waitFor();
    await row().scrollIntoViewIfNeeded(); await shot('read');
  });
  const after = await get('/notifications/' + fixture.notification_id);
  assert.equal(after.read, true); assert.equal(after.delivery_seq, 2); assert.equal(after.read_seq, 2);
  assert.equal(after.attention_required, false); assert.equal(after.business_event_id, before.business_event_id);
  checks.push('The actual sequence2 acknowledgement reads only the delivered recovery; business event, verified state and exact target remain fixed.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ recording, screenshots: shots, layouts, synthetic_data: true, scope: 'Real Web/native HTTP/PG with actual isolated replica worker failure and recovery. No real cloud, model, device or production acceptance.' }, null, 2) + '\n');
}, 'replica-notifications');
