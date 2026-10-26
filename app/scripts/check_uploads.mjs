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
const evidence = path.join(root, 'docs/quality/evidence/uploads'), temporary = path.join(app, '.local/browser-temp');
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
const fixture = async args => JSON.parse((await run(process.env.CP_PYTHON ?? 'python', [path.join(app, 'scripts/prepare_upload_browser_fixture.py'), ...args], { cwd: app, windowsHide: true, encoding: 'utf8' })).stdout);
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const { context, page } = await login(manager);
  const me = await request(context, '/me'), stamp = Date.now();
  const project = await request(context, '/projects', 'POST', { name: '上传队列验证项目 ' + stamp, scope: '仅验证上传元数据及界面；不建立任何云对象。', owner_id: me.user.id, member_ids: [], classification: 'project_restricted' });
  const roots = await request(context, '/folders?project_id=' + project.id);
  const folder = await request(context, '/folders', 'POST', { parent_id: roots.items[0].id, name: '上传界面验证资料 ' + stamp, classification: 'project_restricted' });
  assert(folder.can_upload);
  const prepared = await fixture(['--folder', folder.id, '--owner', me.user.id]);
  assert(prepared.metadata_only && !prepared.cloud_objects_created && !prepared.scan_reports_created);
  await page.goto(config.CP_PUBLIC_ORIGIN + '/files/folders/' + folder.id);
  await page.getByRole('button', { name: '上传文件', exact: true }).waitFor();
  assert(await page.getByRole('button', { name: '上传文件', exact: true }).isDisabled());
  await page.getByText('上传接入尚未配置，请联系管理员。', { exact: true }).waitFor();
  checks.push('Target upload capability is distinct from explicitly disabled storage configuration');
  await page.getByRole('link', { name: /^上传队列/ }).click();
  await page.getByRole('heading', { name: '上传队列.' }).waitFor();
  const large = page.locator('.upload-card[data-upload-id="' + prepared.upload_ids[1] + '"]');
  await large.waitFor(); assert((await large.innerText()).includes('3577'));
  await large.getByLabel('重新选择原文件 120GB-元数据样本.bin').setInputFiles({ name: 'different.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('x') });
  await large.getByRole('alert').filter({ hasText: '所选文件与原上传源不一致' }).waitFor();
  checks.push('120 GB metadata and missing source display accurately; mismatched re-selection stops before signing');
  const waiting = page.locator('.upload-card[data-upload-id="' + prepared.upload_ids[0] + '"]');
  await waiting.getByLabel('重新选择原文件 等待准备-元数据样本.bin').setInputFiles({ name: 'waiting.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('x') });
  await waiting.getByRole('button', { name: '暂停本机传输', exact: true }).waitFor();
  await page.getByRole('link', { name: '知识与记录', exact: true }).click();
  await page.getByRole('link', { name: /^上传队列/ }).click();
  await waiting.getByRole('button', { name: '暂停本机传输', exact: true }).click();
  await waiting.getByText('本机传输已暂停', { exact: true }).waitFor();
  await page.reload(); await waiting.getByText('等待准备上传', { exact: true }).waitFor();
  await waiting.getByRole('button', { name: '重选原文件并继续', exact: true }).waitFor();
  checks.push('Queue intent survives route navigation; reload drops local source and requires re-selection without claiming background upload');
  const failed = page.locator('.upload-card[data-upload-id="' + prepared.upload_ids[2] + '"]');
  await failed.getByRole('button', { name: '终止上传', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '终止上传', exact: true });
  await dialog.getByLabel('申请或操作原因').fill('浏览器验证：终止无云对象的元数据会话');
  await dialog.getByRole('button', { name: '终止上传', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await failed.getByText('已终止', { exact: true }).waitFor();
  const state = await request(context, '/uploads/' + prepared.upload_ids[2]); assert.equal(state.state, 'aborted');
  checks.push('Explicit termination records a real API state change without deleting or inventing cloud content');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [1440, 1024, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 1050 });
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'Queue overflows viewport ' + width);
    }
    await page.setViewportSize({ width: theme === 'light' ? 1440 : 390, height: 1050 });
    await page.screenshot({ path: path.join(evidence, 'queue-' + theme + '.png'), fullPage: true, animations: 'disabled' });
  }
  checks.push('Queue states fit five viewport widths in both visual themes');
  await context.close();
  const outsider = await login(reader);
  await outsider.page.goto(config.CP_PUBLIC_ORIGIN + '/uploads');
  await outsider.page.getByRole('heading', { name: '当前没有可见的上传记录', exact: true }).waitFor();
  assert.equal(await outsider.page.locator('.upload-card').count(), 0);
  await outsider.page.goto(config.CP_PUBLIC_ORIGIN + '/storage/scan-policies');
  await outsider.page.getByRole('heading', { name: '当前账号没有制度管理权限', exact: true }).waitFor();
  checks.push('Other users see no owner queue records and cannot open policy administration');
  await outsider.context.close();
  const administrator = await login(admin), ap = administrator.page;
  const session = (await administrator.context.cookies()).find(value => value.name === 'cp_session')?.value; assert(session);
  await fixture(['--expire-session-hash', createHash('sha256').update(session).digest('hex')]);
  await ap.getByRole('link', { name: '存储制度', exact: true }).click();
  await ap.getByRole('heading', { name: '扫描例外制度.' }).waitFor();
  const internal = ap.locator('.policy-card').filter({ has: ap.getByRole('heading', { name: '公司内部', exact: true }) });
  await internal.getByRole('button', { name: /制定例外范围|发布制度新修订/ }).click();
  dialog = ap.getByRole('dialog', { name: '发布公司内部扫描例外制度', exact: true });
  await dialog.getByLabel('允许的媒体类型', { exact: true }).fill('invalid media type');
  await dialog.getByRole('checkbox', { name: '扫描引擎未配置', exact: true }).check();
  await dialog.getByLabel('单文件最大允许字节数').fill('1000');
  await dialog.getByLabel('制度指定审核人').selectOption(me.user.id);
  const reason = '浏览器验证保留填写内容；本轮不发布有效制度';
  await dialog.getByLabel('发布原因', { exact: true }).fill(reason);
  await dialog.getByLabel('验证器动态码').waitFor();
  assert(await dialog.getByRole('button', { name: '发布新修订', exact: true }).isDisabled());
  await dialog.getByLabel('验证器动态码').fill(await nextCode());
  await dialog.getByRole('button', { name: '验证当前身份', exact: true }).click();
  await dialog.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('发布原因', { exact: true }).inputValue(), reason);
  await dialog.getByRole('button', { name: '发布新修订', exact: true }).click();
  await dialog.getByRole('alert').waitFor();
  assert.equal(await dialog.getByLabel('发布原因', { exact: true }).inputValue(), reason);
  assert.equal(await dialog.getByLabel('允许的媒体类型', { exact: true }).inputValue(), 'invalid media type');
  checks.push('Recent MFA is renewed inside a retained policy form; invalid policy is rejected without publishing a revision');
  await ap.screenshot({ path: path.join(evidence, 'policy-mfa-light-1440.png'), fullPage: true, animations: 'disabled' });
  await ap.setViewportSize({ width: 390, height: 1050 });
  assert(await ap.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
  await ap.screenshot({ path: path.join(evidence, 'policy-mfa-light-390.png'), fullPage: true, animations: 'disabled' });
  await dialog.locator('.mfa-confirmation').scrollIntoViewIfNeeded();
  await ap.screenshot({ path: path.join(evidence, 'policy-mfa-proof-390.png'), fullPage: true, animations: 'disabled' });
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await ap.goto(config.CP_PUBLIC_ORIGIN + '/scan-waivers');
  await ap.getByRole('heading', { name: '当前筛选下没有可见申请', exact: true }).waitFor();
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  try { for (const context of contexts) await context.close(); } finally { await browser?.close(); }
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), checks,
    metadata_fixture_only: true, cloud_objects_created: false, scan_reports_created: false, javascript_errors: errors, server_errors: serverErrors,
    browser_closed: !browser?.isConnected() }, null, 2) + '\n');
}
console.log(JSON.stringify({ passed, checks, browser_closed: true }));
