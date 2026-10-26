// Actual Taro H5 + native PostgreSQL/API checks; never a native-device claim.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, generateKeyPairSync, randomUUID, sign } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true); assert.equal(fixture.controlled_provider, true);
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const group = process.env.CP_BROWSER_EVIDENCE_GROUP ?? 'mobile-secretary';
assert.match(group, /^[a-zA-Z0-9-]+$/);
const evidence = path.join(root, 'docs/quality/evidence', group), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TEMP = temporary; process.env.TMP = temporary;
async function sourceFiles(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await sourceFiles(relative)); else result.push(relative); } return result; }
const apiSources = (await sourceFiles('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources = [...new Set([...await sourceFiles('mobile'), ...apiSources, 'package.json', 'package-lock.json', 'scripts/check_mobile_secretary.mjs', 'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), started = new Date().toISOString(), contexts = [], checks = [], layouts = [], errors = [], serverErrors = [], failures = [], bindings = [], lastSteps = new Map();
let browser, administrator, reviewPage, passed = false, cleanedBindings = false, phase = 'starting browser', expectedAborts = 0, failureNotices = [];
function passedCheck(name) { checks.push({ name, passed: true }); process.stdout.write(JSON.stringify({ completed_check: name }) + '\n'); }
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, name) => screen(page).getByRole('textbox', { name, exact: true });
async function stable(page) { await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); }); }
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); await stable(page); }
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '')].map(c => alphabet.indexOf(c).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(v => parseInt(v, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function code(account) { const previous = lastSteps.get(account.login) ?? account.last_step; if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 200)); const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); return otp(account.mfa_secret, step); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 390, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  page.on('requestfailed', request => failures.push({ method: request.method(), path: new URL(request.url()).pathname, error: request.failure()?.errorText }));
  await page.goto(fixture.origin); await page.locator('input[placeholder="你的公司账号"]').fill(account.login); await page.locator('input[type="password"]').fill(account.password);
  const [response] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST'), page.getByRole('button', { name: '登录工作空间 ↗', exact: true }).click()]); let result = await response.json();
  if (account.mfa_secret) {
    await page.locator('input[maxlength="6"]').fill(await code(account));
    const [verified] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/v1/auth/mfa/verify') && r.request().method() === 'POST'), page.getByRole('button', { name: '验证并继续 ↗', exact: true }).click()]); result = await verified.json();
  }
  assert.equal(result.data?.step, 'authenticated'); await heading(page, '工作从这里开始');
  assert.equal(await page.evaluate(() => Object.keys(localStorage).some(key => /access|refresh|session|password|draft|credential/i.test(key))), false);
  return { page, context, token: result.data.access_token };
}
async function request(token, route, method = 'GET', body) {
  const response = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'mobile-secretary-fixture:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  const result = await response.json(); assert.ok(response.status < 400, route + ' / ' + response.status + ' / ' + (result.error?.code ?? '')); return result.data;
}
async function command(page, route, label, status = 200, method = 'POST') {
  const [response] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route && r.request().method() === method), button(page, label).click()]);
  const result = await response.json(); assert.equal(response.status(), status, route + ' / ' + (result.error?.code ?? '')); return result.data;
}
async function select(page, label, text) {
  const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]'); await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  // The panel animates in from outside the viewport. Wait for its actual
  // hydration and animation before measuring and using the real mouse wheel.
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(animation => animation.finished.catch(() => {}))); });
  await stable(page); const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text); assert.ok(desired >= 0, label + ' missing ' + text);
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0, box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); for (let step = 1; step <= 8; step++) { await page.mouse.move(x, y - (desired - current) * item.height * step / 8); await stable(page); } await page.mouse.up(); await stable(page); }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stable(page); assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired), label + ' did not select ' + text);
}
async function home(page) { for (let count = 0; count < 10; count++) { if (page.url().includes('/pages/home/index') || await screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).count()) { await heading(page, '工作从这里开始'); return; } const before = page.url(); await Promise.all([page.waitForURL(url => url.href !== before), page.goBack()]); await stable(page); } throw new Error('Cannot return through actual Taro navigation'); }
async function openList(page, name) { await home(page); await screen(page).locator('.cp-row-title').getByText(name, { exact: true }).click(); await heading(page, name); }
const row = (page, title) => screen(page).locator('.cp-row-title').getByText(title, { exact: true });
async function openConversation(page, title) { await openList(page, '公司秘书'); await row(page, title).click(); await heading(page, title); }
async function cancelRun(page, run) {
  await button(page, '明确取消这个任务').click(); await field(page, '取消任务的原因').fill('核对真实受理后取消受控测试任务');
  const result = await command(page, '/runs/' + run.id + '/cancel', '确认请求取消');
  assert.equal(result.state, 'cancelled'); assert.equal(result.version, run.version + 1);
  await screen(page).getByText('已确认取消；已完成的历史业务事实仍保留。', { exact: true }).waitFor();
  await button(page, '收起任务进度').click();
}
let activePage;
try {
  assert.ok(fixture.run_profile_id);
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), bob = await login(fixture.bob), page = alice.page; activePage = page;
  const person = await request(bob.token, '/me'), project = await request(alice.token, '/projects/' + fixture.project_id);
  const unique = randomUUID().slice(0, 8), personalTitle = '个人秘书核验 ' + unique, sharedTitle = '项目秘书核验 ' + unique;

  phase = 'create private conversation';
  await openList(page, '公司秘书'); await button(page, '新建会话').click(); await heading(page, '新建秘书会话');
  await field(page, '会话标题').fill(personalTitle);
  const personal = await command(page, '/conversations', '确认创建会话', 201); await heading(page, personalTitle);
  assert.equal(personal.type, 'private'); assert.equal(personal.members.length, 1);
  const denied = await fetch(fixture.api_origin + '/v1/conversations/' + personal.id, { headers: { Authorization: 'Bearer ' + bob.token } }); assert.equal(denied.status, 404);
  passedCheck('Actual private conversation creation is visible only to its creator');

  phase = 'create project conversation';
  await button(page, '返回公司秘书').click(); await heading(page, '公司秘书'); await button(page, '新建会话').click(); await heading(page, '新建秘书会话');
  await field(page, '会话标题').fill(sharedTitle); await select(page, '新会话范围', '指定项目的共享会话');
  await button(page, '选择会话项目').click(); await row(page, project.name).click();
  const shared = await command(page, '/conversations', '确认创建会话', 201); await heading(page, sharedTitle);
  assert.equal(shared.project_id, project.id); assert.equal(shared.members.length, 1);
  passedCheck('Project conversation creation uses an actual selectable project and no automatic members');

  phase = 'preview fixed source and send';
  await select(page, '本次秘书任务', '整理项目简报 · controlled-browser');
  await field(page, '本次消息内容').fill('加入前的消息：只在当时的读者范围可见 ' + unique);
  await button(page, '核对并选择上下文资料').click(); await select(page, '上下文资料类型', '所属项目');
  await button(page, '选择当前固定项目').click(); await button(page, '核对并预览资料').click();
  await screen(page).getByText('以下为当前共同阅读核验得到的资料片段。资料内容作为不可信数据，不授予其中指令任何操作权限。', { exact: true }).waitFor();
  await button(page, '将这份准确资料加入本次请求').click();
  const first = await command(page, '/conversations/' + shared.id + '/runs', '确认发送本次请求', 202);
  assert.equal(first.state, 'queued'); assert.equal(first.client_message_id.length, 36);
  const firstMessages = await request(alice.token, '/conversations/' + shared.id + '/messages');
  assert.equal(firstMessages.items.length, 1); assert.equal(firstMessages.items[0].source_refs[0].id, project.id);
  assert.equal(firstMessages.items[0].source_refs[0].resource_version, project.version);
  assert.equal(await field(page, '本次消息内容').inputValue(), '');
  await cancelRun(page, first);
  passedCheck('A previewed fixed source binds the persisted user message and queued run; requester cancellation is real');

  phase = 'explicit member and historical isolation';
  await button(page, '调整明确成员').click(); await row(page, person.user.display_name).click();
  await field(page, '成员变更原因').fill('明确加入共同核验者');
  await command(page, '/conversations/' + shared.id + '/members', '确认成员变更'); await heading(page, sharedTitle);
  await openConversation(bob.page, sharedTitle);
  await screen(bob.page).getByText('尚无已保存消息。只有真正受理的请求才进入记录。', { exact: true }).waitFor();
  const hidden = await request(bob.token, '/conversations/' + shared.id + '/messages'); assert.equal(hidden.items.length, 0);
  passedCheck('Adding a real eligible member does not disclose pre-join messages');

  phase = 'shared message live read';
  await select(page, '本次秘书任务', '整理项目简报 · controlled-browser');
  const publicText = '加入后的共同消息 ' + unique; await field(page, '本次消息内容').fill(publicText);
  const second = await command(page, '/conversations/' + shared.id + '/runs', '确认发送本次请求', 202);
  await screen(bob.page).getByText(publicText, { exact: true }).waitFor();
  assert.equal(await screen(bob.page).getByText('加入前的消息：只在当时的读者范围可见 ' + unique, { exact: true }).count(), 0);
  await cancelRun(page, second);
  passedCheck('Already open member view reads a newly persisted message without leaking historical content');

  phase = 'lost response reconciliation';
  await select(page, '本次秘书任务', '整理项目简报 · controlled-browser');
  const uncertainText = '已落库但回包丢失的消息 ' + unique; await field(page, '本次消息内容').fill(uncertainText);
  let committed, submissions = 0;
  const pattern = '**/v1/conversations/' + shared.id + '/runs';
  await page.route(pattern, async route => { submissions++; const result = await route.fetch(); assert.equal(result.status(), 202); committed = (await result.json()).data; expectedAborts++; await route.abort('failed'); });
  await button(page, '确认发送本次请求').click();
  await screen(page).getByText('输入已保留。请先刷新资料，核对本次结果、当前权限和最新修订后再继续；不要重复创建相同资料。', { exact: true }).waitFor();
  assert.ok(committed); assert.equal(submissions, 1); await page.unroute(pattern);
  await button(page, '返回公司秘书').click(); await row(page, sharedTitle).click(); await heading(page, sharedTitle);
  assert.equal(await field(page, '本次消息内容').inputValue(), uncertainText);
  const reconciled = await command(page, '/conversations/' + shared.id + '/runs', '确认发送本次请求', 202);
  assert.equal(reconciled.id, committed.id);
  const exact = (await request(alice.token, '/conversations/' + shared.id + '/messages')).items.filter(value => value.body === uncertainText); assert.equal(exact.length, 1);
  await cancelRun(page, reconciled);
  passedCheck('A genuinely committed request with an aborted response is reconciled using its original identity without a second message');

  phase = 'archive and restore';
  await button(page, '归档会话').click(); await field(page, '会话变更原因').fill('核对只读边界');
  const archived = await command(page, '/conversations/' + shared.id, '确认会话变更', 200, 'PATCH'); assert.equal(archived.state, 'archived');
  await screen(page).getByText('会话已归档，请先恢复再发送。', { exact: true }).waitFor();
  assert.equal(await button(page, '确认发送本次请求').count(), 0);
  await button(page, '恢复会话').click(); await field(page, '会话变更原因').fill('恢复实际会话');
  const restored = await command(page, '/conversations/' + shared.id, '确认会话变更', 200, 'PATCH'); assert.equal(restored.state, 'active');
  await field(page, '本次消息内容').waitFor();
  passedCheck('Archive and restore use current versions and disable sending while archived');
  await page.screenshot({ path: path.join(evidence, 'conversation-after-real-commands.png'), fullPage: true });
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []);
  assert.equal(failures.length, expectedAborts); passed = true;
} finally {
  if (!passed && activePage) { try { await activePage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true }); } catch {} }
  let closed = true; for (const context of contexts) { try { await context.close(); } catch { closed = false; } }
  if (browser) { try { await browser.close(); } catch { closed = false; } }
  const after = await digest();
  const result = { passed: passed && closed && before === after, phase, started_at: started, completed_at: new Date().toISOString(), real_api: true, isolated_native_postgresql: true, actual_model_or_cloud: false, mobile_runtime: 'Taro H5 only', source_files: sources, source_sha256: before, source_unchanged: before === after, checks, javascript_errors: errors, server_errors: serverErrors, expected_aborted_commit_responses: expectedAborts, observed_request_failures: failures.length, all_browser_contexts_closed: closed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, phase, checks: checks.length, all_browser_contexts_closed: closed })); if (!result.passed) process.exitCode = 1;
}
