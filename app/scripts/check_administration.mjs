import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { waitForAPI } from './wait_for_api.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
const root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const admin = accounts.find(value => value.login === 'system.admin');
const mfaFile = path.join(app, '.local/development-mfa.json');
let mfa = null;
try { mfa = JSON.parse(await readFile(mfaFile, 'utf8')); } catch (error) { if (error.code !== 'ENOENT') throw error; }
const evidence = path.join(root, 'docs/quality/evidence/administration');
const temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
function decodeBase32(value) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'; let bits = '';
  for (const character of value.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(character).toString(2).padStart(5, '0');
  return Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2)));
}
function otp(secret, step) {
  const counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', decodeBase32(secret)).update(counter).digest();
  return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
let browser, context, passed = false;
const checks = [], errors = [];
try {
  browser = await chromium.launch({ executablePath, headless: true });
  context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  const page = await context.newPage(); page.setDefaultTimeout(20_000);
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(admin.login);
  await page.getByLabel('密码', { exact: true }).fill(admin.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  const setup = page.getByRole('heading', { name: '设置两步验证', exact: true });
  await page.getByLabel('六位验证码').waitFor();
  if (await setup.isVisible()) {
    const secret = await page.locator('.mfa-setup code').innerText();
    mfa = { secret, last_step: 0 };
    await writeFile(mfaFile, JSON.stringify(mfa, null, 2) + '\n');
  }
  assert(mfa?.secret, 'Existing development MFA must have its owned fixture record');
  const current = Math.floor(Date.now() / 30_000);
  if (current <= mfa.last_step) await new Promise(resolve => setTimeout(resolve, (mfa.last_step + 1) * 30_000 - Date.now() + 500));
  const step = Math.floor(Date.now() / 30_000);
  await page.getByLabel('六位验证码').fill(otp(mfa.secret, step));
  await page.getByRole('button', { name: '验证并继续' }).click();
  await page.getByRole('heading', { name: '保存恢复码', exact: true }).or(page.getByRole('heading', { name: /工作从这里开始/ })).waitFor();
  const save = page.getByRole('checkbox', { name: '我已安全保存这些恢复码' });
  if (await save.isVisible()) {
    mfa.recovery_codes = await page.locator('.recovery-codes code').allTextContents();
    await save.check(); await page.getByRole('button', { name: '进入工作空间' }).click();
  }
  mfa.last_step = step;
  await writeFile(mfaFile, JSON.stringify(mfa, null, 2) + '\n'); // Ignored development fixture only.
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  checks.push('Administrator enrolls or verifies MFA through the UI');
  await page.getByRole('link', { name: '成员与授权', exact: true }).click();
  await page.getByRole('heading', { name: '成员与授权.' }).waitFor();
  const name = '受邀验证成员 ' + Date.now(), login = 'browser.invited.' + Date.now();
  await page.getByRole('button', { name: '邀请成员', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('内部账号', { exact: true }).fill(login);
  await dialog.getByLabel('显示名称').fill(name);
  await dialog.getByRole('button', { name: '创建邀请' }).click();
  await page.getByRole('heading', { name: '邀请 ' + name }).waitFor();
  const token = await page.getByLabel('单次使用的邀请码').inputValue();
  assert.equal(token.length, 43);
  await page.getByRole('button', { name: '完成', exact: true }).click();
  await page.getByText(name, { exact: true }).first().waitFor();
  await page.screenshot({ path: path.join(evidence, 'users-light-1440.png'), fullPage: true });
  await page.getByRole('tab', { name: '角色授权' }).click();
  await page.screenshot({ path: path.join(evidence, 'roles-light-1440.png'), fullPage: true });
  checks.push('Invite is created with a protected activation token');
  await page.getByTitle('退出登录').click();
  await page.getByRole('heading', { name: '回到工作空间' }).waitFor();
  await page.getByRole('button', { name: '接受邀请', exact: true }).click();
  await page.getByLabel('邀请码', { exact: true }).fill(token);
  const password = 'Browser-validation-' + Date.now() + '-long-password';
  await page.getByLabel('设置密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '激活内部账号', exact: true }).click();
  await page.getByText('账号已激活，请使用管理员分配的内部账号和刚设置的密码登录。', { exact: true }).waitFor();
  await page.getByLabel('内部账号', { exact: true }).fill(login);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  assert.equal(await page.getByRole('link', { name: '成员与授权', exact: true }).count(), 0);
  assert.equal(await page.locator('.project-card').count(), 0);
  checks.push('Activation and login do not grant unassigned business or administration authority');
  assert.deepEqual(errors, []);
  passed = true;
} finally {
  try { await context?.close(); } finally { await browser?.close(); }
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), checks, javascript_errors: errors, browser_closed: !browser?.isConnected() }, null, 2) + '\n');
}
console.log(JSON.stringify({ passed, checks, browser_closed: true }));
