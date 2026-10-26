// Real API/UI checks using a synthetic project and a temporary scoped role.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { waitForAPI } from './wait_for_api.mjs';

const run = promisify(execFile), app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development'); assert.equal(new URL(config.CP_PUBLIC_ORIGIN).hostname, 'localhost');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const mfaFile = path.join(app, '.local/development-mfa.json'), mfa = JSON.parse(await readFile(mfaFile, 'utf8'));
assert(mfa.secret, 'The owned existing administrator MFA record is required; do not reseed accounts');
const manager = accounts.find(user => user.login === 'project.manager'), reader = accounts.find(user => user.login === 'project.member'), admin = accounts.find(user => user.login === 'system.admin');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/parties'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [];
let browser, passed = false; const publications = [];
function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2)));
  const counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest();
  return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function nextCode() {
  if (Math.floor(Date.now() / 30_000) <= mfa.last_step) await new Promise(resolve => setTimeout(resolve, (mfa.last_step + 1) * 30_000 - Date.now() + 300));
  const step = Math.floor(Date.now() / 30_000); mfa.last_step = step; await writeFile(mfaFile, JSON.stringify(mfa, null, 2) + '\n'); return otp(mfa.secret, step);
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/v1/workflow-definitions') publications.push(request.url()); });
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  if (account === admin) {
    await page.getByLabel('六位验证码').waitFor(); await page.getByLabel('六位验证码').fill(await nextCode());
    await page.getByRole('button', { name: '验证并继续' }).click();
  }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  return { context, page };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(value => value.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, { method, headers: {
    Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID(),
  }, ...(body ? { data: body } : {}) });
  const result = await response.json();
  assert(response.ok(), 'Fixture API request failed: ' + route + ' ' + response.status() + ' ' + result.error?.code);
  return result.data;
}


let administrator, binding, bindingRevoked = false, partyID;
try {
  browser = await chromium.launch({ executablePath, headless: true });
  let owner = await login(manager); administrator = await login(admin);
  const me = await request(owner.context, '/me'), stamp = Date.now();
  const project = await request(owner.context, '/projects', 'POST', { name: '往来验证项目 ' + stamp, scope: '往来资料界面验证，全部联系资料为虚构样本。', owner_id: me.user.id, member_ids: [], classification: 'project_restricted' });
  const roles = await request(administrator.context, '/roles?limit=100'), role = roles.items.find(item => item.key === 'party_manager'); assert(role);
  binding = await request(administrator.context, '/role-bindings', 'POST', { user_id: me.user.id, role_id: role.id, scope_type: 'project', scope_id: project.id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3_600_000).toISOString(), reason: '本地往来资料界面验证，仅限此虚构项目，结束即撤销。' });
  await owner.context.close(); owner = await login(manager);
  const { page, context } = owner, name = '界面验证供应方 ' + stamp;
  await page.getByRole('link', { name: '往来资料', exact: true }).click();
  await page.getByRole('button', { name: '新增往来对象', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '新增往来对象', exact: true });
  await dialog.getByLabel('对象名称', { exact: true }).fill(name);
  await dialog.getByLabel('供应方', { exact: true }).check();
  await dialog.getByLabel('所属项目', { exact: true }).selectOption(project.id);
  assert.equal(await dialog.getByLabel('资料分类', { exact: true }).inputValue(), 'project_restricted');
  await dialog.getByLabel('联系人', { exact: true }).fill('虚构联系人甲');
  await dialog.getByLabel('联系邮箱', { exact: true }).fill('first@example.invalid');
  await dialog.getByLabel('补充说明', { exact: true }).fill('本地界面验证样本；没有真实客户或供应方资料。');
  await dialog.getByLabel('创建依据', { exact: true }).fill('本地真实界面验证，虚构样本。');
  await dialog.getByRole('button', { name: '创建往来资料', exact: true }).click();
  await page.getByRole('heading', { name, exact: true }).waitFor();
  assert.equal(await page.locator('.breadcrumb strong').innerText(), '往来资料');
  partyID = new URL(page.url()).pathname.split('/').at(-1);
  let current = await request(context, '/parties/' + partyID); const original = current.current_revision_id;
  assert.equal(current.project_id, project.id); assert.equal(current.email, 'first@example.invalid');
  checks.push('A temporary project-scoped role creates a real synthetic supplier through the UI with the project security classification');
  await page.getByRole('button', { name: '编辑资料', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '编辑往来资料', exact: true });
  await dialog.getByLabel('联系人', { exact: true }).fill('虚构联系人乙');
  await dialog.getByLabel('联系邮箱', { exact: true }).fill('second@example.invalid');
  await dialog.getByLabel('本次修改依据', { exact: true }).fill('核对联系资料的新修订。');
  await dialog.getByRole('button', { name: '保存为新修订', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByText('second@example.invalid', { exact: true }).waitFor();
  current = await request(context, '/parties/' + partyID); assert.notEqual(current.current_revision_id, original);
  const historic = await request(context, '/parties/' + partyID + '/revisions/' + original); assert.equal(historic.email, 'first@example.invalid');
  checks.push('A real UI edit creates a new immutable revision while the original contact email remains readable in the fixed history');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 });
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), theme + ' ' + width + ' detail overflow');
      if (theme === 'dark' && width === 1440) await page.screenshot({ path: path.join(evidence, 'detail-dark-1440.png'), fullPage: true });
    }
  }
  await page.getByRole('button', { name: '查看修订记录', exact: true }).click();
  dialog = page.getByRole('dialog', { name: name + ' · 修订记录', exact: true });
  await dialog.getByText('first@example.invalid', { exact: true }).waitFor();
  await dialog.getByText('second@example.invalid', { exact: true }).waitFor();
  await page.setViewportSize({ width: 390, height: 1050 }); await page.getByLabel('外观偏好').selectOption('light');
  await page.screenshot({ path: path.join(evidence, 'history-light-390.png'), fullPage: true });
  await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Both themes and five widths fit the real detail layout; history displays the original and current contact evidence');
  await page.getByRole('button', { name: '编辑资料', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '编辑往来资料', exact: true });
  await dialog.getByLabel('联系人', { exact: true }).fill('保留本机填写内容');
  await dialog.getByLabel('本次修改依据', { exact: true }).fill('模拟另一个真实设备先保存。');
  const { name: currentName, kind, relationships, contact_name, email, phone, notes } = current;
  await request(context, '/parties/' + partyID, 'PATCH', { expected_version: current.version, base_revision_id: current.current_revision_id, name: currentName, kind, relationships, contact_name, email, phone, notes: notes + '\n另一个真实 API 请求已先保存。', reason: '另一个设备的真实并发写入。' });
  await dialog.getByRole('button', { name: '保存为新修订', exact: true }).click();
  await dialog.getByText('资料或授权已变化。已保留填写内容，请核对最新资料后重新编辑。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('联系人', { exact: true }).inputValue(), '保留本机填写内容');
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.reload();
  await page.getByRole('heading', { name, exact: true }).waitFor();
  checks.push('A second real API write causes the stale UI revision to conflict and the local form retains its unsaved contact text');
  current = await request(context, '/parties/' + partyID); const beforeArchive = current.current_revision_id;
  await page.getByRole('button', { name: '归档此资料', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '归档往来资料', exact: true });
  await dialog.getByLabel('归档原因', { exact: true }).fill('核对归档只读及保留历史。');
  await dialog.getByRole('button', { name: '确认归档资料', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByText('已归档资料保留历史依据。后续新业务应选择使用中的往来对象。', { exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '编辑资料', exact: true }).count(), 0);
  await page.getByRole('button', { name: '恢复使用', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '恢复往来资料', exact: true });
  await dialog.getByLabel('恢复原因', { exact: true }).fill('结束归档交互验证。');
  await dialog.getByRole('button', { name: '确认恢复使用', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  current = await request(context, '/parties/' + partyID); assert.equal(current.state, 'active'); assert.equal(current.current_revision_id, beforeArchive);
  checks.push('UI archive and restore preserve the content revision and enforce read-only archived content');
  const forbidden = await administrator.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/parties/' + partyID); assert.equal(forbidden.status(), 404);
  const ordinary = await login(reader); await ordinary.page.goto(config.CP_PUBLIC_ORIGIN + '/parties');
  await ordinary.page.getByRole('heading', { name: '没有往来资料权限', exact: true }).waitFor();
  const denied = await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/parties/' + partyID); assert.equal(denied.status(), 404);
  checks.push('Configuration administration and an unrelated employee both receive no private supplier content');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  if (binding && administrator) { try { await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '本地往来资料界面验证结束，撤销临时项目授权。' }); bindingRevoked = true; } catch (error) { errors.push(error.message); passed = false; } }
  let allBrowserContextsClosed = true;
  for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const result = { passed, executed_at: new Date().toISOString(), real_api: true, synthetic_contact_data: true, temporary_binding_revoked: bindingRevoked, checks,
    javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2)); if (!passed || !bindingRevoked || !allBrowserContextsClosed) process.exitCode = 1;
}
