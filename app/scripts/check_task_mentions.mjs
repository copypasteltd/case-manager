import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { recordViewport } from './task_mentions_recording.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..'), mobile = process.argv.includes('--mobile');
const group = 'task-mentions-' + (mobile ? 'mobile' : 'web'), evidence = path.join(app, '../docs/quality/evidence', group);
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.task_mentions_fixture, true); assert.equal(fixture.isolated_database, true);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) h.update(file + '\0').update(await readFile(path.join(app, file))).update('\0'); return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temp = path.join(app, '.local/browser-temp'); await mkdir(temp, { recursive: true }); process.env.TMP = temp; process.env.TEMP = temp;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], checks = [], layouts = [], screenshots = [], recordings = [], errors = [], serverErrors = [];
let browser, passed = false, failure, contextsClosed = 0;
const screen = page => mobile ? mobileScreen(page) : page.locator('body');
const escaped = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const button = (page, label) => screen(page).getByRole('button', { name: new RegExp('^' + escaped(label) + '(?: ↗)?$') });
const field = (page, label) => mobile ? screen(page).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
async function checkBox(page, label) { if (mobile) await screen(page).getByText(label, { exact: true }).last().click(); else await page.getByRole('checkbox', { name: label, exact: true }).check(); }
async function settle(page) { await stableMobile(page); await page.waitForTimeout(250); }
async function api(token, route, method = 'GET', body) {
  let r;
  if (typeof token === 'string') r = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method !== 'GET' ? { 'Idempotency-Key': 'task-mentions-browser:' + randomUUID() } : {}) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  else {
    const csrf = (await token.cookies(fixture.origin)).find(c => c.name === 'cp_csrf')?.value;
    const response = await token.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'Content-Type': 'application/json', ...(method !== 'GET' ? { 'Idempotency-Key': 'task-mentions-browser:' + randomUUID(), 'X-CSRF-Token': decodeURIComponent(csrf ?? '') } : {}) }, ...(body === undefined ? {} : { data: body }) });
    r = { ok: response.ok(), status: response.status(), json: () => response.json() };
  }
  const value = await r.json(); assert.ok(r.ok, route + ': ' + r.status + ' ' + value.error?.code); return value.data;
}
async function drain() { const r = await fetch(fixture.api_origin + '/__fixture/task-mentions/drain', { method: 'POST', headers: { Authorization: 'Bearer ' + fixture.stop_token } }); assert.equal(r.status, 204); }
async function login(account) {
  const context = await browser.newContext({ viewport: mobile ? { width: 390, height: 900 } : { width: 1365, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', e => errors.push(e.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ path: new URL(r.url()).pathname, status: r.status() }); });
  await page.goto(fixture.origin);
  if (mobile) { await field(page, '内部账号').fill(account.login); await field(page, '密码').fill(account.password); }
  else { await page.locator('input[name="login"]').fill(account.login); await page.locator('input[name="password"]').fill(account.password); }
  const logged = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/auth/login' && r.request().method() === 'POST'); await button(page, '登录工作空间').click(); let result = await (await logged).json();
  if (account.mfa_secret) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(300);
    const step = Math.floor(Date.now() / 30000), code = otp(account.mfa_secret, step);
    if (mobile) await screen(page).locator('input[maxlength="6"]').fill(code); else await page.locator('input[name="code"]').fill(code);
    const verified = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/auth/mfa/verify' && r.request().method() === 'POST'); await button(page, '验证并继续').click(); result = await (await verified).json(); account.last_step = step;
  }
  assert.equal(result.data?.step, 'authenticated'); await screen(page).getByText('工作从这里开始', { exact: false }).first().waitFor().catch(async () => { if (!mobile) await page.getByRole('main').waitFor(); else throw new Error('Mobile login did not reach Home'); });
  return { page, context, token: mobile ? result.data.access_token : context };
}
async function openTask(page) {
  if (mobile) await page.evaluate(id => { window.location.hash = '#/pages/detail/index?kind=task&id=' + id; }, fixture.task_id);
  else await page.goto(fixture.origin + '/projects/' + fixture.project_id + '?task=' + fixture.task_id);
  await button(page, '新增任务评论').waitFor(); await settle(page);
}
async function shot(page, suffix) {
  await settle(page);
  const layout = await page.evaluate(() => ({ width: innerWidth, document: document.documentElement.scrollWidth }));
  assert.ok(layout.document <= layout.width + 1, 'Discussion must fit its viewport'); layouts.push({ name: suffix, ...layout });
  const name = group + '-' + suffix + '.png'; await page.screenshot({ path: path.join(app, '../docs/media/screenshots', name), fullPage: false, caret: 'hide' }); screenshots.push(name);
}
async function prepare(page) { await button(page, '核对任务评论').click(); await checkBox(page, '已核对文字、固定附件及提及范围'); }
async function save(page, edit = false) {
  const response = page.waitForResponse(r => new URL(r.url()).pathname.startsWith('/v1/' + (edit ? 'comments/' : 'tasks/' + fixture.task_id + '/comments')) && r.request().method() === (edit ? 'PATCH' : 'POST'));
  await button(page, edit ? '保存任务评论新修订' : '保存任务评论').click(); const r = await response; assert.equal(r.status(), 201); const value = (await r.json()).data;
  await button(page, '核对任务评论').waitFor({ state: 'hidden' }); return value;
}
function card(page, body) { return mobile ? screen(page).getByText(body, { exact: true }).first().locator('..') : page.locator('article[data-task-comment]').filter({ has: page.getByText(body, { exact: true }) }).first(); }
async function edit(page, body) { await card(page, body).getByRole('button', { name: '修改我的任务评论', exact: true }).click(); await field(page, '任务评论文字').waitFor(); }
async function closeEditor(page) { if (mobile) await button(page, '关闭任务评论表单并核对讨论').click(); else await page.getByRole('dialog').last().getByRole('button', { name: /关闭/ }).click(); }

try {
  browser = await chromium.launch({ executablePath, headless: true, args: ['--no-first-run', '--disable-extensions'] });
  const alice = await login(fixture.alice), bob = await login(fixture.bob); await openTask(alice.page);
  const candidates = await api(alice.token, '/tasks/' + fixture.task_id + '/mention-candidates'); const recipient = candidates.items.find(c => c.id === fixture.bob_id); assert.ok(recipient);
  const file = await api(alice.token, '/files/' + fixture.file_id);
  let first;
  recordings.push(await recordViewport(alice.page, app, group + '-collaboration', async () => {
    await button(alice.page, '新增任务评论').click(); await field(alice.page, '任务评论文字').fill('请核对香港项目的固定依据与下一步安排。');
    await button(alice.page, '选择任务评论固定附件').click();
    if (mobile) { await screen(alice.page).locator('.cp-row-title').getByText(file.name, { exact: true }).click(); await button(alice.page, '引用固定版本 1').click(); await button(alice.page, '收起任务附件选择').click(); }
    else { await alice.page.locator('.reference-option').filter({ hasText: file.name }).first().click(); await alice.page.getByRole('button', { name: /^版本 1 ·/ }).click(); }
    await checkBox(alice.page, recipient.display_name); await prepare(alice.page); await shot(alice.page, 'preview'); await alice.page.waitForTimeout(1300);
    first = await save(alice.page); assert.deepEqual(first.mentioned_user_ids, [fixture.bob_id]); assert.deepEqual(first.attachment_versions, [fixture.file_version_id]);
    await card(alice.page, first.body).waitFor(); await card(alice.page, first.body).scrollIntoViewIfNeeded(); await shot(alice.page, 'discussion'); await alice.page.waitForTimeout(1500);
  }));
  checks.push('Actual UI creates a fixed historical-version comment with an explicitly selected eligible recipient; exact receipt and immutable-history readback pass.');
  await drain(); const notices = await api(bob.token, '/notifications?category=mention'); assert.equal(notices.items.length, 1); const noticeID = notices.items[0].id;
  await edit(alice.page, first.body); await field(alice.page, '任务评论文字').fill('补充说明：沿用原固定版本和提及成员。'); await prepare(alice.page); const second = await save(alice.page, true);
  assert.equal(second.replaces_id, first.id); assert.deepEqual(second.attachment_versions, [fixture.file_version_id]); assert.deepEqual(second.mentioned_user_ids, [fixture.bob_id]); await drain();
  let current = await api(bob.token, '/notifications?category=mention'); assert.equal(current.items.length, 1); assert.equal(current.items[0].id, noticeID); assert.equal(current.items[0].delivery_seq, 1); assert.equal(current.items[0].target.comment_id, second.id);
  checks.push('UI text-only revision preserves fixed associations and updates the current notification target without repeating delivery.');
  // Push the selected discussion beyond the first20 current comments using real, labelled setup writes.
  for (let i = 0; i < 22; i++) await api(alice.token, '/tasks/' + fixture.task_id + '/comments', 'POST', { body: '隔离分页样本 ' + (i + 1) });
  assert.equal((await api(bob.token, '/tasks/' + fixture.task_id + '/comments?limit=20')).items.some(c => c.id === second.id), false);
  if (mobile) await bob.page.evaluate(() => { window.location.hash = '#/pages/notifications/index'; }); else await bob.page.goto(fixture.origin + '/notifications');
  await button(bob.page, '打开相关资料').first().waitFor(); await shot(bob.page, 'notification');
  await button(bob.page, '打开相关资料').first().click(); await screen(bob.page).getByText('通知指定讨论', { exact: false }).first().waitFor(); await screen(bob.page).getByText(second.body, { exact: true }).first().waitFor();
  assert.ok(decodeURIComponent(bob.page.url()).includes(second.id));
  const focusedBody = screen(bob.page).getByText(second.body, { exact: true }).first();
  let focusedBounds;
  for (let i = 0; i < 30; i++) { focusedBounds = await focusedBody.boundingBox(); if (focusedBounds && focusedBounds.y >= 0 && focusedBounds.y + focusedBounds.height <= 900) break; await bob.page.waitForTimeout(100); }
  assert.ok(focusedBounds && focusedBounds.y >= 0 && focusedBounds.y + focusedBounds.height <= 900, 'Application must automatically position the exact comment in the viewport; the checker does not scroll it.');
  await shot(bob.page, 'exact-discussion');
  checks.push('Actual notification UI opens the exact current comment beyond the first20 rows for a non-assignee, with canonical task/comment routing.');
  await card(alice.page, second.body).getByRole('button', { name: '查看任务评论历史', exact: true }).click(); await screen(alice.page).getByText(first.body, { exact: true }).first().waitFor().catch(async () => { if (!mobile) throw new Error('Immutable history body missing'); await screen(alice.page).getByText('评论修订 1', { exact: true }).click(); await screen(alice.page).getByText(first.body, { exact: true }).first().waitFor(); });
  if (mobile) await button(alice.page, '收起任务评论历史').click(); else await alice.page.getByRole('dialog').last().getByRole('button', { name: /关闭/ }).click();
  checks.push('Actual discussion history retains the original body and fixed historical attachment.');
  // Separate immutable comments keep the two intentional transport faults independently reviewable.
  const fault = await api(alice.token, '/tasks/' + fixture.task_id + '/comments', 'POST', { body: '不完整回执前的独立评论' }); await button(alice.page, '重新读取任务讨论').click(); await edit(alice.page, fault.body);
  await field(alice.page, '任务评论文字').fill('服务器已提交，但此用例故意移除回执中的提及字段。'); await prepare(alice.page);
  let incomplete = 0; await alice.page.route('**/v1/comments/' + fault.id, async route => { if (route.request().method() !== 'PATCH') return route.continue(); const response = await route.fetch(); assert.equal(response.status(), 201); const data = await response.json(); delete data.data.mentioned_user_ids; delete data.data.mentions_filtered; incomplete++; await route.fulfill({ response, json: data }); });
  await button(alice.page, '保存任务评论新修订').click(); await screen(alice.page).getByText(/本次操作需要核对|结果尚未核对|回执不完整/).first().waitFor(); assert.equal(incomplete, 1); assert.equal(await button(alice.page, '保存任务评论新修订').isDisabled(), true);
  await alice.page.unroute('**/v1/comments/' + fault.id); await closeEditor(alice.page);
  checks.push('Labelled incomplete201 receipt is a real committed revision; both mention fields removed at browser transport, repeat submit blocked.');
  await button(alice.page, '新增任务评论').click(); await field(alice.page, '任务评论文字').fill('服务器已提交，但此用例故意丢失创建回执。'); await prepare(alice.page);
  let lost = 0; const routePattern = '**/v1/tasks/' + fixture.task_id + '/comments'; await alice.page.route(routePattern, async route => { if (route.request().method() !== 'POST') return route.continue(); const response = await route.fetch(); assert.equal(response.status(), 201); lost++; await route.abort('failed'); });
  await button(alice.page, '保存任务评论').click(); await screen(alice.page).getByText(/本次操作需要核对|结果尚未核对|尚未确认/).first().waitFor(); assert.equal(lost, 1); assert.equal(await button(alice.page, '保存任务评论').isDisabled(), true); assert.ok(await field(alice.page, '任务评论文字').inputValue());
  await alice.page.unroute(routePattern); await closeEditor(alice.page); await button(alice.page, '重新读取任务讨论').click();
  const all = await api(alice.token, '/tasks/' + fixture.task_id + '/comments?limit=100'); assert.equal(all.items.filter(c => c.body === '服务器已提交，但此用例故意丢失创建回执。').length, 1);
  checks.push('Labelled committed-response loss blocks repeat creation, retains editable text, and actual database readback contains exactly one created comment.');
  await button(alice.page, '新增任务评论').click(); assert.equal(await field(alice.page, '任务评论文字').inputValue(), '服务器已提交，但此用例故意丢失创建回执。'); await prepare(alice.page); const reconciled = await save(alice.page); assert.equal(reconciled.body, '服务器已提交，但此用例故意丢失创建回执。'); const afterReconcile = await api(alice.token, '/tasks/' + fixture.task_id + '/comments?limit=100'); assert.equal(afterReconcile.items.filter(c => c.body === reconciled.body).length, 1);
  checks.push('After explicit list review, reopening restores text only; reconfirming the identical intent reuses its original idempotency key and reconciles the one committed comment without creating another.');
  const p = await api(alice.token, '/projects/' + fixture.project_id); await api(alice.token, '/projects/' + fixture.project_id + '/members', 'POST', { expected_version: p.version, user_id: fixture.bob_id, action: 'remove', reason: '隔离浏览器验收：撤销讨论读取权限' });
  await bob.page.reload(); await (mobile ? field(bob.page, '内部账号') : bob.page.locator('input[name="login"]')).waitFor(); assert.equal(await screen(bob.page).getByText(second.body, { exact: true }).count(), 0);
  checks.push('Actual project-membership removal invalidates the other employee session and clears the open task discussion before returning to login.');
  passed = true;
} catch (error) {
  failure = error?.stack ?? String(error);
  for (const account of [fixture.alice, fixture.bob, fixture.administrator]) for (const key of ['password', 'mfa_secret']) if (account?.[key]) failure = failure.replaceAll(account[key], '[redacted]');
  process.stderr.write(failure + '\n');
} finally {
  for (const context of contexts) { try { await context.close(); contextsClosed++; } catch { /* report cleanup mismatch */ } }
  if (browser) await browser.close(); const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && contextsClosed === contexts.length && !errors.length && !serverErrors.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, screenshots, recordings, errors, server_errors: serverErrors, failure, owned_browser_closed: closed, browser_contexts_created: contexts.length, browser_contexts_closed: contextsClosed, real_native_api: true, real_cloud: false, real_model: false, real_device: false, real_telegram_network: false, client: mobile ? 'Taro H5' : 'React Web', controlled_response_faults: ['incomplete committed201 comment revision', 'committed comment creation response lost'] };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
