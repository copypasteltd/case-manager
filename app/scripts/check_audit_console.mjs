// Real synthetic API/browser journeys. No response substitution or business state injection.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './audit_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './audit_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = value => createHash('sha256').update(value).digest('hex');
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = (web, portable = '审计' + web) => mobile ? root().locator('input[aria-label="' + portable + '"]') : page.getByLabel(web, { exact: true });
  const actor = fixture.administrator;
  assert.ok(actor && fixture.isolated_database);
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin);
  await field('内部账号', '内部账号').fill(actor.login); await field('密码', '密码').fill(actor.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  await field('六位验证码', '六位验证码').fill(await waitOTP({ alice: actor }));
  await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = async () => mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': (await context.cookies()).find(c => c.name === 'cp_csrf')?.value ?? '' };
  async function get(route) {
    const response = await context.request.get(origin + '/v1' + route, { headers: await headers() });
    assert.equal(response.status(), 200, route); return (await response.json()).data;
  }
  const initial = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/audit/events' && r.status() === 200);
  await page.goto(fixture.origin + (mobile ? '/#/pages/settings/index?section=audit' : '/audit'));
  const initialPage = (await (await initial).json()).data;
  await root().getByText('查询条件', { exact: true }).waitFor();
  assert.equal(initialPage.disclosure, 'organization_metadata_v1');
  assert.ok(initialPage.items.length > 0);
  for (const event of initialPage.items) for (const key of ['reason', 'session_id', 'payload']) assert.ok(!(key in event));
  const me = await get('/me'); assert.ok(me.actions.includes('audit.read') && me.actions.includes('audit.export'));
  const inaccessible = await context.request.get(origin + '/v1/projects/' + fixture.project_id, { headers: await headers() });
  assert.equal(inaccessible.status(), 404);
  checks.push('An explicitly authorized system administrator queries organization audit metadata without gaining business project access or free-text/session disclosure.');
  // Committed, isolated export commands create enough real audit rows and history for both pagination controls.
  for (let index = 0; index < 21; index++) {
    const response = await context.request.post(origin + '/v1/audit/exports', { headers: { ...await headers(), 'Idempotency-Key': randomUUID() }, data: { filters: initialPage.filters, reason: 'Synthetic pagination preparation ' + index } });
    assert.equal(response.status(), 201);
  }
  await button('刷新当前结果').click();
  await button('下一页记录').waitFor({ state: 'visible' });
  await assertNoCredentials(page);
  const prefix = 'audit-console-' + (mobile ? 'mobile' : 'web'), shots = [], layouts = [];
  const screenshots = path.join(app, '../docs/media/screenshots'); await mkdir(screenshots, { recursive: true });
  async function shot(suffix) {
    await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png';
    await page.screenshot({ path: path.join(screenshots, file) });
    shots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(screenshots, file))) });
  }
  const recording = await recordViewport(page, app, prefix, async () => {
    await button('下一页记录').click(); await button('上一页记录').click();
    await button('查看详情').first().click(); await root().getByText('审计记录详情', { exact: true }).waitFor();
    await shot('detail'); await button('关闭详情').click();
    checks.push('Event pagination and details use actual immutable event records and server-bound cursors.');
    await field('动作').fill('synthetic.no-match');
    await root().getByText('筛选尚未应用，列表仍显示上次查询结果。', { exact: true }).waitFor();
    await button('应用查询').click(); await root().getByText(mobile ? '没有匹配的审计记录。' : '没有匹配的审计记录', { exact: true }).waitFor();
    await shot('empty');
    if (mobile) await button('调整查询条件').click();
    await field('动作').fill('audit.export.created');
    const applied = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/audit/events' && new URL(r.url()).searchParams.get('action') === 'audit.export.created' && r.status() === 200);
    await button('应用查询').click();
    const filtered = (await (await applied).json()).data;
    assert.ok(filtered.items.length > 0 && filtered.items.every(e => e.action === 'audit.export.created'));
    checks.push('Unapplied draft filters are visible; explicit application resets the page and supports both empty results and exact-action matches.');
    await field('导出用途').fill('Synthetic human audit review and sealed-file verification');
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/audit/exports' && r.request().method() === 'POST' && r.status() === 201);
    await button('按当前条件生成 CSV').click();
    const record = (await (await created).json()).data;
    assert.equal(record.filters.action, 'audit.export.created'); assert.equal(record.state, 'ready');
    await root().getByText(record.id, { exact: mobile }).first().waitFor();
    await button('刷新我的导出').click();
    await button('下一页导出').click(); await button('上一页导出').click();
    await button(mobile ? '校验并打开 CSV' : '校验并下载 CSV').first().scrollIntoViewIfNeeded(); await shot('export');
    const completed = page.waitForEvent('download');
    await button(mobile ? '校验并打开 CSV' : '校验并下载 CSV').first().click();
    const download = await completed;
    assert.equal(download.suggestedFilename(), 'audit-' + record.id + '.csv');
    const stream = await download.createReadStream(), pieces = []; for await (const chunk of stream) pieces.push(chunk);
    const bytes = Buffer.concat(pieces); assert.equal(bytes.length, record.csv_bytes); assert.equal(sha(bytes), record.csv_sha256);
    const text = bytes.toString('utf8'); assert.ok(text.startsWith('\ufeffevent_id,occurred_at_utc,'));
    assert.ok(!text.split('\r\n')[0].includes('reason') && !text.includes(actor.password) && !text.includes(actor.mfa_secret));
    assert.equal(text.split('\r\n').filter(Boolean).length - 1, record.row_count);
    const fixed = await get('/audit/exports/' + record.id); assert.equal(fixed.csv_sha256, record.csv_sha256); assert.equal(fixed.row_count, record.row_count);
    checks.push('Human export creation, owner history pagination and client download verify exactly the sealed row count, byte count, original filters and SHA-256 despite later audit writes.');
    if (!mobile) {
      await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await shot('dark');
      await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
    }
  });
  for (const width of mobile ? [390] : [1440, 900, 390]) {
    await page.setViewportSize({ width, height: mobile ? 900 : 1100 }); await page.waitForTimeout(200);
    const layout = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    assert.ok(layout.scroll <= layout.client + 2, JSON.stringify(layout)); layouts.push({ width, ...layout });
  }
  checks.push('Audit layouts fit the reviewed viewports; any wide record table scrolls within its container.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ recordings: [recording], layouts, screenshots: shots, synthetic_data: true, real_browser_pixels: true, audio: false }, null, 2) + '\n');
}, 'audit-console');
