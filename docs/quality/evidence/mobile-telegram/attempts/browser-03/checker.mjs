import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { recordViewport } from './telegram_access_recording.mjs';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-telegram-browser');
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) { h.update(file + '\0'); h.update(await readFile(path.join(app, file))); h.update('\0'); } return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], closedContexts = new Set(), checks = [], layouts = [], errors = [], serverErrors = [], cleanupErrors = [], recordings = [], screenshots = [];
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, label) => screen(page).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]');
let browser, mainPage, passed = false, failure;
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); }
async function navigate(page, url) { await page.evaluate(value => { window.location.hash = '#' + value; }, url); await stable(page); }
async function closeOwned(context) { if (!closedContexts.has(context)) { await context.close(); closedContexts.add(context); } }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 390, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ status: r.status(), path: new URL(r.url()).pathname }); });
  await page.goto(fixture.origin); await field(page, '内部账号').fill(account.login); await field(page, '密码').fill(account.password);
  const pending = page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST'); await button(page, '登录工作空间 ↗').click(); let result = await (await pending).json();
  if (account.mfa_secret) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
    const step = Math.floor(Date.now() / 30000); await screen(page).locator('input[maxlength="6"]').fill(otp(account.mfa_secret, step));
    const verified = page.waitForResponse(r => r.url().endsWith('/v1/auth/mfa/verify') && r.request().method() === 'POST'); await button(page, '验证并继续 ↗').click(); result = await (await verified).json(); account.last_step = step;
  }
  assert.equal(result.data?.step, 'authenticated'); await heading(page, '工作从这里开始'); return { page, context, token: result.data.access_token };
}
async function response(token, route, method = 'GET', body) {
  return fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'telegram-setup:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
}
async function request(token, route, method, body) { const r = await response(token, route, method, body), data = await r.json(); assert.ok(r.ok, `${method ?? 'GET'} ${route}: ${r.status} ${data.error?.code ?? ''}`); return data.data; }
async function submit(page, label, route, method, status = 200) {
  const [r] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route && r.request().method() === method), button(page, label).click()]);
  assert.equal(r.status(), status, route); return { body: r.request().postDataJSON(), value: (await r.json()).data };
}
async function layout(page, name) {
  for (const width of [320, 390, 768]) {
    await page.setViewportSize({ width, height: 900 }); await stable(page);
    const value = await screen(page).evaluate(node => ({ width: node.getBoundingClientRect().width, overflow: document.documentElement.scrollWidth > innerWidth + 1, small: [...node.querySelectorAll('.cp-button,.cp-row')].filter(n => { const r = n.getBoundingClientRect(); return r.width && r.height < 43.5; }).map(n => n.textContent) }));
    assert.equal(value.overflow, false, name + ' horizontal overflow'); assert.ok(Math.abs(value.width - width) <= 1); assert.deepEqual(value.small, []); layouts.push({ page: name, width, passed: true });
  }
  await page.setViewportSize({ width: 390, height: 900 }); await stable(page); await assertSafe(page); await page.screenshot({ path: path.join(evidence, name + '.png'), fullPage: true });
}


async function mfa(page, account) {
  while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
  const step = Math.floor(Date.now() / 30000); await field(page, '验证器六位验证码').fill(otp(account.mfa_secret, step));
  await submit(page, '确认本人操作', '/auth/mfa/step-up', 'POST'); account.last_step = step;
  await screen(page).getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  assert.equal(await field(page, '验证器六位验证码').count(), 0);
}

const checkbox = (page, name) => screen(page).getByRole('checkbox', { name, exact: true });
const channelAck = '我已核对准确Bot、群、话题、项目范围和全部允许任务';
function done(message) { checks.push(message); console.log('Completed actual flow ' + checks.length + ': ' + message); }
async function assertSafe(page) { assert.equal(await page.locator('input[type="password"]:visible,input[maxlength="6"]:visible,.cp-secret:visible').count(), 0); }
async function image(page, name, caption) {
  await stable(page); await assertSafe(page); const file = path.join(app, '../docs/media/screenshots/' + name + '.png');
  await page.screenshot({ path: file, fullPage: false });
  screenshots.push({ file: 'docs/media/screenshots/' + name + '.png', caption, sha256: createHash('sha256').update(await readFile(file)).digest('hex'), synthetic_data: true, real_device: false });
}
async function openAccess(page) {
  await screen(page).getByRole('button', { name: /^Telegram 接入/ }).click(); await heading(page, 'Telegram 接入'); await button(page, '重新读取TG权限与本人状态').waitFor(); await stable(page);
}
async function challenge(page, account, reason) {
  await button(page, '发起本人TG绑定').click(); await select(page, '选择绑定Bot', '@' + fixture.bot_username);
  await field(page, '身份操作原因').fill(reason); await checkbox(page, '我将仅在所选Bot私聊证明本人身份').check();
  await mfa(page, account); const issued = await submit(page, '生成本人私聊绑定链接', '/me/telegram/challenges', 'POST', 201);
  await button(page, '取消此次绑定流程').waitFor(); assert.ok(/^[A-Za-z0-9_-]{43}$/.test(issued.value.start_token)); return issued.value;
}
async function config(page, account, label, reason) {
  await field(page, 'TG入口名称').fill(label); await field(page, 'TG配置原因').fill(reason);
  await checkbox(page, channelAck).check(); await mfa(page, account);
}
function patchBody(c, label, reason) {
  return { expected_version: c.version, label, project_id: c.project_id, allowed_skill_ids: c.allowed_skill_ids,
    default_skill_id: c.default_skill_id, state: c.state, delivery_enabled: c.delivery_enabled, reason };
}

try {
  assert.equal(fixture.telegram_ingress_fixture, true);
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), admin = await login(fixture.administrator);
  // Policy administration alone grants no business visibility. Give this isolated
  // account an explicit business role and membership before testing the picker.
  const adminMe = await request(admin.token, '/me'), aliceMe = await request(alice.token, '/me');
  const roles = await request(admin.token, '/roles'), manager = roles.items.find(v => v.key === 'project_manager'); assert.ok(manager);
  await request(admin.token, '/role-bindings', 'POST', { user_id: adminMe.user.id, role_id: manager.id, scope_type: 'org', scope_id: null,
    classification_ceiling: 'project_restricted', reason: 'Synthetic explicit project visibility for mobile picker acceptance' });
  await request(alice.token, '/projects', 'POST', { name: 'TG入口合成可见项目', scope: '仅用于隔离移动入口范围验收',
    owner_id: aliceMe.user.id, member_ids: [adminMe.user.id], classification: 'internal' });
  mainPage = alice.page; const page = alice.page, control = admin.page;
  await openAccess(page); assert.equal(await button(page, '新增暂停的TG群入口').count(), 0);
  await button(page, '查看本人TG请求与通知回执').click(); await heading(page, '我的TG请求');
  await button(page, '本人绑定与TG入口管理').click(); await heading(page, 'Telegram 接入'); await stable(page);
  done('The actual employee workspace and existing request page open the mobile access page; a non-administrator has no channel-configuration controls.');

  let issued = await challenge(page, fixture.alice, '本人取消流程验收');
  assert.equal(await screen(page).locator('.cp-secret').count(), 1);
  await button(page, '重新读取TG权限与本人状态').click(); await stable(page); assert.equal(await screen(page).locator('.cp-secret').count(), 0);
  await button(page, '取消此次绑定流程').click(); await field(page, '身份操作原因').fill('取消准确的未证明流程');
  await checkbox(page, '我已核对准确对象与此次取消或撤销的影响').check();
  await submit(page, '确认取消此次绑定', '/me/telegram/challenges/' + issued.id + '/cancel', 'POST');
  await button(page, '发起本人TG绑定').waitFor(); await stable(page);
  assert.equal((await request(alice.token, '/me/telegram')).challenges.some(c => c.id === issued.id), false);
  done('A real recent-MFA private challenge is created and explicitly cancelled; refresh clears its capability instead of persisting it in a draft.');

  issued = await challenge(page, fixture.alice, '本人绑定数字身份验收'); const telegramID = 987650082;
  const r = await fetch(fixture.api_origin + '/v1/integrations/telegram/' + fixture.bot_id + '/webhook', { method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Telegram-Bot-Api-Secret-Token': fixture.webhook_secret },
    body: JSON.stringify({ update_id: 820001, message: { message_id: 820001, date: Math.floor(Date.now() / 1000),
      from: { id: telegramID, is_bot: false }, chat: { id: telegramID, type: 'private' }, text: '/start ' + issued.start_token,
      entities: [{ type: 'bot_command', offset: 0, length: 6 }] } }) }); assert.equal(r.status, 200);
  await button(page, '重新读取TG权限与本人状态').click(); await stable(page);
  assert.equal(await screen(page).locator('.cp-secret').count(), 0);
  await button(page, '核对并确认本人数字身份').click(); await field(page, '身份操作原因').fill('原登录核对本人准确数字身份');
  await checkbox(page, '我已核对该数字身份属于本人').check(); await mfa(page, fixture.alice);
  const binding = (await submit(page, '确认绑定这个数字身份', '/me/telegram/challenges/' + issued.id + '/confirm', 'POST', 201)).value;
  assert.equal(binding.telegram_user_id, String(telegramID)); await button(page, '撤销此本人绑定').waitFor(); await stable(page);
  await layout(page, 'telegram-own-binding'); await image(page, 'mobile-telegram-own-binding', 'Own numeric identity confirmed through the original mobile login; synthetic loopback webhook.');
  await button(page, '撤销此本人绑定').click(); await field(page, '身份操作原因').fill('结束本地合成本人绑定验收');
  await checkbox(page, '我已核对准确对象与此次取消或撤销的影响').check();
  await submit(page, '确认撤销此本人绑定', '/me/telegram/bindings/' + binding.id + '/revoke', 'POST'); await stable(page);
  assert.equal((await request(alice.token, '/me/telegram')).bindings.some(b => b.id === binding.id), false);
  done('An actual loopback webhook proves a numeric Telegram identity; original-session mobile confirmation and explicit revocation are corroborated by authoritative own-status reads. No Telegram network is used.');

  mainPage = control; await openAccess(control); await button(control, '新增暂停的TG群入口').click();
  await select(control, '接收Bot', '@' + fixture.bot_username);
  await field(control, '准确数字群号').fill('-1008200000001'); await field(control, '准确数字话题号').fill('42');
  await button(control, '选择TG项目范围').click();
  const projects = await request(admin.token, '/projects?limit=20'); assert.ok(projects.items.length);
  await button(control, '选择项目：' + projects.items[0].name).click();
  await checkbox(control, '允许任务：服务事件摘要').check(); await select(control, 'TG默认任务', '服务事件摘要');
  await checkbox(control, '允许向准确群与话题发送阶段和登录链接').check();
  await config(control, fixture.administrator, '移动端合成群入口', '完整核对Bot、群、话题及项目范围');
  const created = await submit(control, '创建暂停的TG群入口', '/telegram/channels', 'POST', 201);
  let channel = created.value; assert.equal(channel.state, 'paused'); assert.equal(channel.disclosure_mode, 'portal_only');
  assert.equal(channel.project_id, projects.items[0].id); assert.equal(channel.delivery_enabled, true); assert.equal(channel.default_skill_id, 'incident-summary');
  await button(control, '编辑入口：' + channel.label).waitFor(); await stable(control);
  done('An administrator creates an exact Bot/chat/topic/project channel through the actual project picker; two allowed Skills, explicit default and link-only delivery are persisted and the new channel remains paused.');

  await button(control, '编辑入口：' + channel.label).click();
  assert.equal(await field(control, '准确数字群号').count(), 0); await select(control, 'TG接收状态', '启用接收');
  await config(control, fixture.administrator, channel.label, '核对完整范围后启用接收');
  channel = (await submit(control, '保存准确TG入口配置', '/telegram/channels/' + channel.id, 'PATCH')).value;
  assert.equal(channel.state, 'active'); assert.equal(channel.version, 2); await button(control, '编辑入口：' + channel.label).waitFor(); await stable(control);
  await image(control, 'mobile-telegram-channel-config', 'Administrator channel configuration with precise location, complete allowed tasks and portal-only disclosure.');
  await layout(control, 'telegram-channel-config');
  done('An explicit fresh-MFA edit activates the exact immutable location, advances one version and is corroborated against the latest complete immutable history snapshot.');

  await button(control, '编辑入口：' + channel.label).click();
  channel = await request(admin.token, '/telegram/channels/' + channel.id, 'PATCH', patchBody(channel, '另一窗口的新入口版本', '隔离数据库并发配置验收'));
  await config(control, fixture.administrator, '此窗口保留的冲突文字', '不覆盖另一窗口的新版本');
  let writes = 0; const countWrites = req => { if (req.method() === 'PATCH' && new URL(req.url()).pathname === '/v1/telegram/channels/' + channel.id) writes++; };
  control.on('request', countWrites);
  await submit(control, '保存准确TG入口配置', '/telegram/channels/' + channel.id, 'PATCH', 409); await stable(control);
  assert.equal(await field(control, 'TG入口名称').inputValue(), '此窗口保留的冲突文字'); assert.equal(await button(control, '保存准确TG入口配置').isDisabled(), true);
  await control.waitForTimeout(300); assert.equal(writes, 1); control.off('request', countWrites);
  await button(control, '关闭表单并读取最新TG入口').click(); await button(control, '编辑入口：' + channel.label).waitFor();
  done('A real second-writer version conflict returns 409, retains entered text and blocks repeat submission; closing the form rereads the new authoritative version.');

  await button(control, '编辑入口：' + channel.label).click(); await config(control, fixture.administrator, '回执不完整仍需核对', '隔离代理删除一个回执字段');
  let mutations = 0;
  await control.route('**/v1/telegram/channels/' + channel.id, async route => {
    if (route.request().method() !== 'PATCH') return route.continue(); mutations++;
    const original = await route.fetch(), data = await original.json(); assert.equal(original.status(), 200); delete data.data.delivery_enabled;
    await route.fulfill({ response: original, json: data });
  });
  await submit(control, '保存准确TG入口配置', '/telegram/channels/' + channel.id, 'PATCH'); await stable(control);
  assert.equal(await button(control, '保存准确TG入口配置').isDisabled(), true); assert.equal(await field(control, 'TG入口名称').inputValue(), '回执不完整仍需核对');
  await control.waitForTimeout(300); assert.equal(mutations, 1); await control.unroute('**/v1/telegram/channels/' + channel.id);
  await button(control, '关闭表单并读取最新TG入口').click(); await button(control, '编辑入口：回执不完整仍需核对').waitFor();
  channel = (await request(admin.token, '/telegram/channels?limit=100')).items.find(c => c.id === channel.id);
  done('A controlled proxy removes one field after a real committed PATCH: the mobile form rejects the incomplete receipt, retains input and sends no duplicate write before explicit authoritative reread.');

  for (let n = 0; n < 21; n++) channel = await request(admin.token, '/telegram/channels/' + channel.id, 'PATCH', patchBody(channel, n === 20 ? '移动端完整历史入口' : '合成历史入口第' + n + '次', '合成历史分页准备第' + n + '次'));
  await button(control, '重新读取TG群入口列表').click(); await button(control, '查看历史：' + channel.label).click(); await stable(control);
  await button(control, '加载更早TG入口历史').click(); await screen(control).getByText('完整核对Bot、群、话题及项目范围', { exact: true }).waitFor();
  assert.equal(await button(control, '加载更早TG入口历史').count(), 0);
  await layout(control, 'telegram-channel-history');
  await image(control, 'mobile-telegram-channel-history', 'Complete paginated immutable channel snapshots; synthetic history was prepared through the real API.');
  recordings.push(await recordViewport(control, app, 'mobile-telegram-access', async () => {
    await control.mouse.wheel(0, 650); await control.waitForTimeout(900); await control.mouse.wheel(0, -650); await control.waitForTimeout(900);
    await button(control, '返回TG入口列表').click(); await button(control, '查看历史：' + channel.label).click(); await stable(control); await control.waitForTimeout(1000);
  }));
  await button(control, '返回TG入口列表').click();
  done('The actual mobile history follows an opaque cursor to the first full revision and renders immutable configuration and reasons; safe viewport frames capture the history/list interaction.');

  for (let n = 0; n < 21; n++) await request(admin.token, '/telegram/channels', 'POST', {
    bot_id: fixture.bot_id, chat_id: String(-1008200000100 - n), topic_id: 0, label: '合成分页入口' + String(n).padStart(2, '0'),
    project_id: null, allowed_skill_ids: ['project-briefing'], default_skill_id: 'project-briefing', delivery_enabled: false, reason: '入口列表分页合成准备' });
  await button(control, '重新读取TG群入口列表').click(); await button(control, '加载更多TG群入口').waitFor(); await button(control, '加载更多TG群入口').click(); await stable(control);
  assert.equal(await button(control, '加载更多TG群入口').count(), 0); assert.ok(await screen(control).getByRole('button', { name: /^编辑入口：/ }).count() >= 23);
  await control.route('**/v1/telegram/channels?*', route => route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: { code: 'forbidden', message: 'Synthetic permission-change response', request_id: 'synthetic' } }) }));
  await button(control, '重新读取TG群入口列表').click(); await stable(control);
  assert.equal(await screen(control).getByRole('button', { name: /^编辑入口：/ }).count(), 0); assert.equal(await button(control, '新增暂停的TG群入口').isDisabled(), true);
  await control.unroute('**/v1/telegram/channels?*');
  done('The real channel list loads its next opaque cursor; a separately labelled synthetic 403 clears cached controls and disables creation instead of exposing stale configuration.');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  failure = String(error.stack ?? error);
  if (mainPage && !mainPage.isClosed()) {
    try { failure += '\nActive page: ' + JSON.stringify({ route: new URL(mainPage.url()).hash, titles: await screen(mainPage).locator('.cp-title').allTextContents(), actions: await screen(mainPage).getByRole('button').allTextContents() });
      await mainPage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true, mask: [mainPage.locator('input[type="password"],input[maxlength="6"],.cp-secret,.cp-code')] }); } catch { /* Keep original failure. */ }
  }
  for (const account of [fixture.alice, fixture.bob, fixture.administrator]) for (const key of ['password', 'mfa_secret']) if (account?.[key]) failure = failure.replaceAll(account[key], '[redacted]');
  if (fixture.webhook_secret) failure = failure.replaceAll(fixture.webhook_secret, '[redacted]');
  process.stderr.write(failure + '\n');
} finally {
  for (const context of contexts) { try { await closeOwned(context); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && !cleanupErrors.length && closedContexts.size === contexts.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, recordings, screenshots, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, failure, owned_browser_closed: closed, browser_contexts_created: contexts.length, browser_contexts_closed: closedContexts.size, real_native_api: true, real_model: false, real_cloud: false, real_device: false, real_telegram_network: false, controlled_response_faults: ['incomplete committed channel receipt', 'list permission 403'] };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
