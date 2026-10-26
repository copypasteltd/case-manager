// Real API/UI checks; company approval policies are never published by this check.
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
const evidence = path.join(root, 'docs/quality/evidence/workflow-definitions'), temporary = path.join(app, '.local/browser-temp');
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


try {
  browser = await chromium.launch({ executablePath, headless: true });
  const administrator = await login(admin), { context, page } = administrator;
  const before = await request(context, '/workflow-definitions?limit=100');
  await page.getByRole('link', { name: '审批制度', exact: true }).click();
  await page.getByRole('heading', { name: '审批制度', exact: true }).waitFor();
  assert.equal(await page.locator('.breadcrumb strong').innerText(), '审批制度');
  await page.getByLabel('业务类型', { exact: true }).selectOption('claim');
  await page.getByRole('button', { name: '制定审批规则', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '制定审批规则', exact: true });
  await dialog.waitFor();
  assert.equal(await dialog.getByLabel('金额下界（含）', { exact: true }).inputValue(), '');
  assert.equal(await dialog.getByLabel('本人审批政策', { exact: true }).inputValue(), '');
  assert.equal(await dialog.getByLabel('审批中撤回政策', { exact: true }).inputValue(), '');
  assert.equal(await dialog.getByLabel('启用此修订，供后续送审匹配', { exact: true }).isChecked(), false);
  checks.push('The real policy form leaves company thresholds, self-approval and withdrawal decisions explicit; a new rule begins disabled');
  await dialog.getByLabel('规则名称', { exact: true }).fill('界面核对草稿 · 不发布');
  await dialog.getByLabel('稳定规则代码', { exact: true }).fill('ui.sample.unpublished');
  await dialog.getByLabel('路由类别', { exact: true }).fill('ui-preview');
  await dialog.getByLabel('币种', { exact: true }).fill('hkd');
  assert.equal(await dialog.getByLabel('币种', { exact: true }).inputValue(), 'HKD');
  await dialog.getByLabel('金额下界（含）', { exact: true }).fill('0');
  await dialog.getByLabel('金额上界（不含）', { exact: true }).fill('100');
  await dialog.getByLabel('第 1 步名称', { exact: true }).fill('经办复核');
  const users = await request(context, '/users?limit=100');
  const active = users.items.filter(user => user.status === 'active'); assert(active.length >= 2);
  await dialog.getByLabel('第 1 步审批人', { exact: true }).selectOption(active[0].id);
  await dialog.getByRole('button', { name: '添加下一步', exact: true }).click();
  await dialog.getByLabel('第 2 步名称', { exact: true }).fill('股东复核');
  await dialog.getByLabel('第 2 步审批人', { exact: true }).selectOption(active[1].id);
  await dialog.getByRole('button', { name: '上移第 2 步', exact: true }).click();
  assert.equal(await dialog.getByLabel('第 1 步名称', { exact: true }).inputValue(), '股东复核');
  assert.equal(await dialog.getByLabel('第 1 步审批人', { exact: true }).inputValue(), active[1].id);
  await dialog.getByRole('button', { name: '下移第 1 步', exact: true }).click();
  assert.equal(await dialog.getByLabel('第 1 步名称', { exact: true }).inputValue(), '经办复核');
  await dialog.getByLabel('本人审批政策', { exact: true }).selectOption('allow');
  await dialog.getByText('这条规则允许指定审批人处理自己发起的申请。请核对公司已确定的授权政策。', { exact: true }).waitFor();
  await dialog.getByLabel('本人审批政策', { exact: true }).selectOption('deny');
  await dialog.getByLabel('审批中撤回政策', { exact: true }).selectOption('allow');
  await dialog.getByLabel('发布依据', { exact: true }).fill('本地界面填写验证，仅在浏览器内核对，取消且不发布制度。');
  checks.push('Named approvers, sequence reordering, currency normalization and explicit policy choices retain their values in the same form');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 });
      const fits = await page.evaluate(() => {
        const d = document.querySelector('dialog[open]'), box = d.getBoundingClientRect();
        return { body: document.documentElement.scrollWidth <= innerWidth, dialog: box.left >= 0 && box.right <= innerWidth && d.scrollWidth <= d.clientWidth + 1 };
      });
      assert(fits.body && fits.dialog, theme + ' ' + width + ' layout overflow');
      if (theme === 'light' && width === 390) {
        await dialog.getByLabel('规则名称', { exact: true }).scrollIntoViewIfNeeded();
        await page.screenshot({ path: path.join(evidence, 'form-light-390.png'), fullPage: true });
      }
      if (theme === 'dark' && width === 1440) {
        await dialog.getByLabel('本人审批政策', { exact: true }).scrollIntoViewIfNeeded();
        await page.screenshot({ path: path.join(evidence, 'policy-dark-1440.png'), fullPage: true });
      }
    }
  }
  checks.push('The real form fits five widths in both themes, with usable scroll regions and unchanged draft choices');
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  const after = await request(context, '/workflow-definitions?limit=100');
  assert.deepEqual(after.items.map(item => item.id), before.items.map(item => item.id));
  assert.equal(publications.length, 0);
  checks.push('Cancelling the UI draft sends no publication request and preserves the existing company rules');
  const ordinary = await login(manager);
  await ordinary.page.goto(config.CP_PUBLIC_ORIGIN + '/workflows');
  await ordinary.page.getByRole('heading', { name: '没有制度管理权限', exact: true }).waitFor();
  assert.equal(await ordinary.page.getByRole('button', { name: '制定审批规则', exact: true }).count(), 0);
  const forbidden = await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/workflow-definitions');
  assert.equal(forbidden.status(), 404);
  checks.push('An employee without configuration rights sees no policy editor and receives no rules from the API');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  let allBrowserContextsClosed = true;
  for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const result = { passed, executed_at: new Date().toISOString(), real_api: true, company_policy_published: publications.length !== 0,
    checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n');
  console.log(JSON.stringify(result, null, 2));
}
