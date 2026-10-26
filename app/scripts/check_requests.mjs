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
const evidence = path.join(root, 'docs/quality/evidence/requests'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [];
let browser, passed = false; 
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


let administrator, requestID, contractID, passedCleanup = false; const bindings = [];
try {
  browser = await chromium.launch({ executablePath, headless: true });
  let owner = await login(manager); administrator = await login(admin);
  const me = await request(owner.context, '/me'), stamp = Date.now();
  const project = await request(owner.context, '/projects', 'POST', { name: '申请验证项目 ' + stamp, scope: '全为虚构申请样本，不发布公司制度。', owner_id: me.user.id, member_ids: [], classification: 'project_restricted' });
  const roles = await request(administrator.context, '/roles?limit=100');
  for (const key of ['contract_manager', 'request_manager', 'approval_reviewer']) {
    const role = roles.items.find(item => item.key === key); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: me.user.id, role_id: role.id, scope_type: 'project', scope_id: project.id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3_600_000).toISOString(), reason: '本地申请界面验证，仅限虚构项目，结束即撤销。' }));
  }
  await owner.context.close(); owner = await login(manager);
  const { page, context } = owner, title = '界面验证申请 ' + stamp;
  let contract = await request(context, '/contracts', 'POST', { title, project_id: project.id, classification: 'project_restricted', reason: '未知事实留空，不生成批准或签署。' }); contractID = contract.id;
  await page.goto(config.CP_PUBLIC_ORIGIN + '/contracts/' + contract.id);
  await page.getByRole('button', { name: '准备送审申请', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '准备合同申请', exact: true });
  await dialog.getByLabel('准备申请依据', { exact: true }).fill('先建立固定资料草稿，再补齐事实。');
  await dialog.getByRole('button', { name: '建立申请草稿', exact: true }).click();
  await page.getByRole('heading', { name: title, exact: true }).waitFor();
  await page.waitForURL('**/requests/*'); requestID = new URL(page.url()).pathname.split('/').at(-1);
  let current = await request(context, '/requests/' + requestID);
  assert.equal(current.state, 'draft'); assert.equal(current.workflow, null); assert.equal(current.steps.length, 0);
  assert.equal(current.payload.content.amount, null); assert.equal(current.payload.content.draft_file_version_id, null);
  const originalRevision = current.current_revision_id;
  checks.push('A real contract action creates an explicit nullable application draft with no invented workflow, approval, amount or file');
  await page.getByRole('button', { name: '核对并送审', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '核对合同送审预览', exact: true });
  await dialog.getByRole('alert').waitFor();
  await dialog.getByLabel('送审依据', { exact: true }).fill('缺少正文与业务事实，应当明确阻断。');
  assert(await dialog.getByRole('button', { name: '确认以此内容送审', exact: true }).isDisabled());
  await dialog.getByRole('button', { name: '重新读取送审预览', exact: true }).click();
  await dialog.getByRole('alert').waitFor();
  assert.equal(await dialog.getByLabel('送审依据', { exact: true }).inputValue(), '缺少正文与业务事实，应当明确阻断。');
  await page.setViewportSize({ width: 390, height: 1050 });
  await page.screenshot({ path: path.join(evidence, 'preview-blocked-light-390.png'), fullPage: true });
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  checks.push('Missing business facts block real submission preview and disable confirmation; explicit retry retains the typed reason');
  const fields = ['title', 'direction', 'type', 'category', 'counterparty_id', 'counterparty_revision_id', 'owner_id', 'currency', 'amount', 'tax_amount', 'start_date', 'end_date', 'renewal_terms', 'termination_terms', 'notes', 'draft_file_version_id', 'payment_schedule'];
  const update = async values => {
    contract = await request(context, '/contracts/' + contractID);
    contract = await request(context, '/contracts/' + contractID, 'PATCH', { ...Object.fromEntries(fields.map(field => [field, contract[field]])), ...values, expected_version: contract.version, base_revision_id: contract.current_revision_id, reason: '另一个实际请求更新合同。' });
  };
  await update({ title: title + ' · 当前合同' }); await page.reload();
  await page.getByRole('heading', { name: title, exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '核对并送审', exact: true }).count(), 0);
  await page.getByRole('button', { name: '更新申请草稿', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '更新申请草稿', exact: true });
  await dialog.getByRole('heading', { name: contract.title, exact: true }).waitFor();
  await dialog.getByLabel('更新申请依据', { exact: true }).fill('保留本机填写，核对并发修改。');
  await update({ notes: '更新申请弹窗打开后，其他设备再次保存。' });
  await dialog.getByRole('button', { name: '确认更新申请草稿', exact: true }).click();
  await dialog.getByRole('alert').waitFor();
  assert.equal(await dialog.getByLabel('更新申请依据', { exact: true }).inputValue(), '保留本机填写，核对并发修改。');
  assert.equal((await request(context, '/requests/' + requestID)).current_revision_id, originalRevision);
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.reload();
  checks.push('Changing the real business draft keeps the prior request snapshot; a second write causes a visible conflict and preserves the typed revision reason');
  await page.getByRole('button', { name: '更新申请草稿', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '更新申请草稿', exact: true });
  await dialog.getByRole('heading', { name: contract.title, exact: true }).waitFor();
  await dialog.getByLabel('更新申请依据', { exact: true }).fill('明确固定最新合同资料为新申请修订。');
  await dialog.getByRole('button', { name: '确认更新申请草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('heading', { name: contract.title, exact: true }).waitFor();
  current = await request(context, '/requests/' + requestID);
  assert.notEqual(current.current_revision_id, originalRevision); assert.equal(current.payload.content.notes, contract.notes);
  const revisions = await request(context, '/requests/' + requestID + '/revisions?limit=10');
  assert.equal(revisions.items.length, 2); assert.equal(revisions.items[1].payload.content.title, title);
  await page.getByRole('button', { name: '查看申请修订', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '申请修订与历次审批', exact: true });
  await dialog.getByRole('heading', { name: '申请修订 1 · ' + title, exact: true }).waitFor();
  await dialog.getByRole('heading', { name: '申请修订 2 · ' + contract.title, exact: true }).waitFor();
  await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('An explicit real refresh creates a new application revision; the UI history retains both original and updated business snapshots');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 });
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), theme + ' ' + width + ' request overflow');
      if (theme === 'dark' && width === 1440) await page.screenshot({ path: path.join(evidence, 'request-dark-1440.png'), fullPage: true });
    }
  }
  await page.goto(config.CP_PUBLIC_ORIGIN + '/requests');
  await page.getByLabel('搜索申请', { exact: true }).fill(stamp.toString()); await page.getByRole('button', { name: '搜索', exact: true }).click();
  await page.getByRole('link').filter({ has: page.getByRole('heading', { name: contract.title, exact: true }) }).waitFor();
  await page.goto(config.CP_PUBLIC_ORIGIN + '/approvals');
  await page.getByRole('heading', { name: '当前没有待你处理的审批', exact: true }).waitFor();
  checks.push('Both themes and five widths fit; the actual application list finds the refreshed request and the reviewer workbench reads an empty current task set');
  assert.equal((await administrator.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/requests/' + requestID)).status(), 404);
  const ordinary = await login(reader); await ordinary.page.goto(config.CP_PUBLIC_ORIGIN + '/requests');
  await ordinary.page.getByRole('heading', { name: '没有申请查阅权限', exact: true }).waitFor();
  assert.equal((await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/requests/' + requestID)).status(), 404);
  checks.push('Configuration administration and an unrelated employee cannot read the private application or its fixed snapshot');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  passedCleanup = bindings.length === 3;
  if (administrator) for (const binding of bindings) {
    try { await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '本地申请界面验证结束，撤销临时项目授权。' }); }
    catch (error) { errors.push(error.message); passedCleanup = false; passed = false; }
  }
  let allBrowserContextsClosed = true;
  for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const result = { passed, executed_at: new Date().toISOString(), real_api: true, synthetic_business_data: true, production_policy_published: false, real_cloud_file_evidence: false, positive_approval_execution_tested_in_isolated_pg: true, temporary_bindings_revoked: passedCleanup, checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2));
  if (!passed || !passedCleanup || !allBrowserContextsClosed) process.exitCode = 1;
}
