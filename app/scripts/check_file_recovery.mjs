// Real synthetic browser input reaches the owned native API and PostgreSQL.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { recoveryBrowser, waitOTP } from './file_recovery_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './file_recovery_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
await recoveryBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  assert.ok(fixture.file_recovery_fixture && fixture.isolated_database);
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture));
  await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = async () => mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': (await context.cookies()).find(c => c.name === 'cp_csrf')?.value ?? '' };
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, { headers: await headers() }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  await page.goto(fixture.origin + (mobile ? '/#/pages/secretary/index?kind=conversation&id=' : '/conversations/') + fixture.conversation_id);
  await field('本次消息内容').waitFor();
  const original = await get('/runs/' + fixture.original_run_id);
  assert.equal(original.state, 'cancelled'); assert.equal(original.file_update.operation_id, null);
  if (mobile) await selectMobile(page, '本次秘书任务', '新建或更新文件 · controlled-test'); else await field('本次秘书任务').selectOption(fixture.profile_id);
  await button('选择旧草稿继续处理').click();
  if (mobile) {
    await root().getByText(new RegExp(fixture.original_run_id.slice(0, 8))).click();
  } else { await field('选择原文件请求').selectOption(fixture.original_run_id); }
  await root().getByText('文件更新实际结果', { exact: true }).waitFor();
  const index = original.file_update.drafts.findIndex(d => d.id === fixture.original_draft_id);
  assert.ok(index >= 0);
  const choice = '草稿 ' + (index + 1) + ' · ' + fixture.original_draft_id.slice(0, 8) + ' · ' + original.file_update.drafts[index].size_bytes + ' 字节';
  if (mobile) await selectMobile(page, '本次恢复的准确草稿', choice); else await field('本次恢复的准确草稿').selectOption(fixture.original_draft_id);
  assert.equal(await button('将准确草稿恢复加入请求').isEnabled(), false);
  checks.push('The real original stopped run and exact saved diff load through the application; selecting a draft alone does not authorize or send recovery.');
  const review = '我已核对选定草稿的差异、内容摘要及原目标';
  if (mobile) await root().locator('.cp-check').filter({ hasText: review }).click(); else await field(review).check();
  assert.equal((await get('/runs/' + fixture.original_run_id)).state, 'cancelled');
  checks.push('Explicit human review enables selection while the old run and its committed state stay fixed.');
  const directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  const prefix = 'file-recovery-' + (mobile ? 'mobile' : 'web'), shots = [], layouts = [];
  async function shot(suffix) { await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png'; await page.screenshot({ path: path.join(directory, file) }); shots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) }); }
  await button('将准确草稿恢复加入请求').scrollIntoViewIfNeeded();
  const recording = await recordViewport(page, app, prefix, async () => {
    await shot('review');
    await button('将准确草稿恢复加入请求').click(); await button('移除本次草稿恢复').waitFor();
    await button('移除本次草稿恢复').scrollIntoViewIfNeeded(); await shot('selected');
    await field('本次消息内容').fill('恢复已核对的准确草稿；重新验证与扫描后提交，保持原目标和基础版本。');
    const accepted = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/conversations/' + fixture.conversation_id + '/runs' && r.request().method() === 'POST');
    await button('确认发送本次请求').click(); const response = await accepted; assert.equal(response.status(), 202);
    const request = response.request().postDataJSON(), run = (await response.json()).data;
    assert.deepEqual(request.file_recovery, { source_run_id: fixture.original_run_id, draft_id: fixture.original_draft_id, content_sha256: fixture.content_sha256, reviewed: true });
    assert.ok(!request.file_update && !request.file_create); assert.notEqual(run.id, original.id);
    assert.equal(run.file_update.recovery.source_run_id, original.id); assert.equal(run.file_update.recovery.draft_id, fixture.original_draft_id);
    assert.equal(run.file_update.recovery.content_sha256, fixture.content_sha256); assert.equal(run.file_update.recovery.depth, 1);
    assert.equal(run.file_update.base_version_id, original.file_update.base_version_id); assert.deepEqual(run.file_update.target, original.file_update.target);
    assert.equal(run.file_update.status, 'authorized'); assert.equal(run.file_update.operation_id, null); assert.deepEqual(run.file_update.drafts, []);
    const persisted = await get('/runs/' + run.id); assert.deepEqual(persisted.file_update.recovery, run.file_update.recovery);
    await root().getByText('恢复来源', { exact: false }).first().waitFor();
    await root().getByText('恢复来源', { exact: false }).first().evaluate(node => node.scrollIntoView({ block: 'center' })); await shot('accepted');
    assert.equal((await get('/runs/' + original.id)).state, original.state);
    checks.push('An actual UI submission persists a new queued run with immutable exact recovery provenance, original target/base and no claimed upload, model output or formal commit.');
    const stopped = await context.request.post(origin + '/v1/runs/' + run.id + '/cancel', { headers: { ...await headers(), 'Idempotency-Key': randomUUID() }, data: { expected_version: persisted.version, action: 'cancel', reason: '关闭本轮合成页面检查创建的待执行任务' } });
    assert.equal(stopped.status(), 200);
    const conflict = await context.request.post(origin + '/v1/conversations/' + fixture.conversation_id + '/runs', { headers: { ...await headers(), 'Idempotency-Key': randomUUID() }, data: { ...request, client_message_id: randomUUID(), file_recovery: { ...request.file_recovery, content_sha256: 'f'.repeat(64) } } });
    assert.equal(conflict.status(), 409); assert.equal((await conflict.json()).error.code, 'FILE_RECOVERY_UNAVAILABLE');
    assert.equal((await get('/runs/' + original.id)).file_update.operation_id, null);
    checks.push('A changed content digest cannot continue the original run or create a formal file commit.');
    if (!mobile) { await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await shot('dark'); await page.getByLabel('外观偏好', { exact: true }).selectOption('light'); }
  });
  for (const width of mobile ? [390] : [1440, 390]) { await page.setViewportSize({ width, height: 900 }); const size = await (mobile ? root() : page.locator('html')).evaluate(node => ({ width: node.clientWidth, scroll: node.scrollWidth })); assert.ok(size.scroll <= size.width + 1); layouts.push({ width, ...size, passed: true }); }
  checks.push('The actual Web and shared mobile recovery view stays within the viewport; the owned synthetic API fixture is separate from real model, cloud and production acceptance.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ recordings: [recording], layouts, screenshots: shots, synthetic_data: true, real_browser_pixels: true, audio: false }, null, 2) + '\n');
}, 'file-recovery');
