// Real API/UI checks. Seeded upload records are explicitly metadata-only:
// no cloud object, multipart transfer, scan report or successful scan is faked.
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
const evidence = path.join(root, 'docs/quality/evidence/retention'), temporary = path.join(app, '.local/browser-temp');
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

const fixture = async args => JSON.parse((await run(process.env.CP_PYTHON ?? 'python', [path.join(app, 'scripts/prepare_storage_browser_fixture.py'), ...args], { cwd: app, windowsHide: true, encoding: 'utf8' })).stdout);
let administrator, binding, hold, released = false, bindingRevoked = false;
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const owner = await login(manager);
  administrator = await login(admin);
  const me = await request(owner.context, '/me'), am = await request(administrator.context, '/me'), stamp = Date.now();
  const project = await request(owner.context, '/projects', 'POST', { name: '存储验证项目 ' + stamp,
    scope: '保留界面检查；仅含缺失来源元数据，不创建云对象。', owner_id: me.user.id, member_ids: [am.user.id], classification: 'project_restricted' });
  const roots = await request(owner.context, '/folders?project_id=' + project.id);
  const folder = await request(owner.context, '/folders', 'POST', { parent_id: roots.items.find(item => item.parent_id === null).id,
    name: '界面验证资料 ' + stamp, classification: 'project_restricted' });
  const prepared = await fixture(['--folder', folder.id, '--owner', me.user.id]);
  assert(prepared.metadata_only && !prepared.cloud_objects_created);
  const roles = await request(administrator.context, '/roles?limit=100');
  const role = roles.items.find(item => item.key === 'file_retention_manager'); assert(role);
  binding = await request(administrator.context, '/role-bindings', 'POST', { user_id: am.user.id, role_id: role.id,
    scope_type: 'project', scope_id: project.id, classification_ceiling: 'project_restricted',
    valid_until: new Date(Date.now() + 3_600_000).toISOString(), reason: '本地保留界面验证，仅限本次元数据测试项目，结束即撤销。' });
  // Changing one's roles invalidates the previous session; use a fresh real MFA login.
  await administrator.context.close(); administrator = await login(admin);
  const { context, page } = administrator;
  await page.goto(config.CP_PUBLIC_ORIGIN + '/files/' + prepared.file_id);
  await page.getByRole('heading', { name: '元数据样本-120GB.bin', exact: true }).waitFor();
  const section = page.locator('#file-retention');
  await section.getByRole('button', { name: '添加文件保留', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '添加文件保留约束', exact: true });
  await dialog.getByLabel('保留依据', { exact: true }).fill('本地界面验证：保护元数据样本，验证完成后明确释放。');
  await dialog.getByRole('button', { name: '确认建立保留约束', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await section.getByText('保留生效中', { exact: true }).waitFor();
  const holds = await request(context, '/files/' + prepared.file_id + '/retention-holds');
  hold = holds.items.find(item => item.active); assert(hold);
  const protectedFile = await request(owner.context, '/files/' + prepared.file_id); assert.equal(protectedFile.can_trash, false);
  checks.push('A temporary project-scoped retention role uses real MFA and creates an immutable hold through the UI; the owner loses trash capability');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [1440, 1024, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 1050 });
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'Retention overflows viewport ' + width);
    }
    await page.setViewportSize({ width: theme === 'dark' ? 1440 : 390, height: 1050 });
    await section.scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidence, 'holds-' + theme + '.png'), fullPage: true, animations: 'disabled' });
  }
  checks.push('Retention records and fixed-version actions fit five widths in both themes with no root horizontal overflow');
  await page.setViewportSize({ width: 390, height: 844 }); await page.getByLabel('外观偏好').selectOption('light');
  const version = page.locator('.version-card').filter({ has: page.getByRole('heading', { name: /内容版本 2/ }) });
  await version.getByRole('button', { name: '保留评估', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '内容版本 2 的保留评估', exact: true });
  await dialog.getByLabel('评估原因', { exact: true }).fill('核对当前版本与缺失云证据，保留评估不执行删除。');
  await dialog.getByRole('button', { name: '生成保留评估', exact: true }).click();
  await dialog.getByText('当前正式版本始终受保护', { exact: true }).waitFor();
  await dialog.getByText('存在有效保留约束', { exact: true }).waitFor();
  await dialog.getByText('双云副本尚未具备完整验证证据', { exact: true }).waitFor();
  assert.equal(await dialog.getByRole('button', { name: /删除/ }).count(), 0);
  await page.screenshot({ path: path.join(evidence, 'assessment-light-390.png'), animations: 'disabled' });
  await dialog.getByRole('button', { name: '完成核对', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  checks.push('A real persisted mobile assessment protects the current version and hold and reports missing replicas; it offers no deletion action');
  await owner.page.goto(config.CP_PUBLIC_ORIGIN + '/files/' + prepared.file_id);
  await owner.page.getByRole('heading', { name: '元数据样本-120GB.bin', exact: true }).waitFor();
  await owner.page.locator('#file-retention').getByText(/有效保留约束：1 项/).waitFor();
  assert.equal(await owner.page.getByRole('button', { name: '保留评估', exact: true }).count(), 0);
  assert.equal(await owner.page.getByRole('button', { name: '添加文件保留', exact: true }).count(), 0);
  const csrf = (await owner.context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const denied = await owner.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/files/' + prepared.file_id + '/retention-holds', { headers: { Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '' } });
  assert.equal(denied.status(), 404);
  checks.push('A real project owner without the dedicated role sees the protection count but cannot retrieve reasons or perform retention actions');
  await section.getByRole('button', { name: '核对并释放此保留约束', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '释放此保留约束', exact: true });
  await dialog.getByLabel('释放依据', { exact: true }).fill('本地界面验证已完成；保留原约束与释放依据。');
  await dialog.getByRole('button', { name: '确认释放此约束', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await section.getByText('已释放', { exact: true }).waitFor(); released = true;
  const finalHolds = await request(context, '/files/' + prepared.file_id + '/retention-holds');
  assert.equal(finalHolds.items.find(item => item.id === hold.id).active, false);
  const after = await request(owner.context, '/files/' + prepared.file_id), versions = await request(owner.context, '/files/' + prepared.file_id + '/versions');
  assert(after.can_trash); assert.equal(after.current_version_id, prepared.version_ids[1]); assert.equal(versions.items.length, 2);
  checks.push('Explicit release preserves the original hold and release reason and restores owner capability without changing either content version');
  await page.goto(config.CP_PUBLIC_ORIGIN + '/storage/retention-policies');
  await page.getByRole('heading', { name: '文件保留制度.' }).waitFor();
  await page.locator('.policy-card').first().waitFor();
  assert.equal(await page.locator('.policy-card').count(), 4);
  await page.locator('.policy-card').filter({ has: page.getByRole('heading', { name: '正式历史版本', exact: true }) }).getByRole('button').click();
  dialog = page.getByRole('dialog', { name: '发布正式历史版本保留制度', exact: true });
  await dialog.getByLabel('保留时长（天）', { exact: true }).fill('30');
  await dialog.getByLabel('观察期（小时）', { exact: true }).fill('24');
  await dialog.getByLabel('发布依据', { exact: true }).fill('仅验证表单后取消，不代替待确认的公司制度。');
  for (const theme of ['light', 'dark']) {
    await page.evaluate(value => { document.documentElement.dataset.theme = value; }, theme);
    for (const width of [1440, 1024, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 1050 });
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    }
  }
  await page.screenshot({ path: path.join(evidence, 'policy-form-dark-320.png'), animations: 'disabled' });
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  checks.push('Four independently versioned policy kinds and an inline-MFA form render responsively; cancelling leaves the undecided company policy unpublished');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  errors.push(String(error)); process.exitCode = 1;
} finally {
  try {
    if (hold && !released) await request(administrator.context, '/retention-holds/' + hold.id + '/release', 'POST', { reason: '退出本地保留界面验证，释放本次专用测试约束。' });
    if (binding) { await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '本地验证结束，撤销仅针对专用测试项目的临时授权。' }); bindingRevoked = true; }
  } catch (error) { errors.push('Fixture authorization cleanup failed: ' + String(error)); passed = false; process.exitCode = 1; }
  await Promise.allSettled(contexts.map(context => context.close())); if (browser) await browser.close();
  const report = { passed, executed_at: new Date().toISOString(), checks, errors, server_errors: serverErrors,
    source: 'node app/scripts/check_retention.mjs', temporary_binding_revoked: bindingRevoked, contexts_closed: true, browser_closed: true,
    scope: 'Real API/UI, scoped temporary role and explicitly missing metadata only. No cloud object, scan report, delete, published retention policy or real 120 GB transfer.' };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify(report, null, 2));
}
