import assert from 'node:assert/strict';
import { access, writeFile } from 'node:fs/promises';
import { chromium } from 'playwright';

const [url, output] = process.argv.slice(2);
assert.equal(new URL(url).origin, 'http://127.0.0.1:5189');
let browser, context;
const errors = [], checks = [];
const result = { passed: false, checks, context_closed: false, browser_closed: false, recorded_credentials: false, new_media: false };
try {
  const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
  await access(executablePath);
  browser = await chromium.launch({ executablePath, headless: true });
  result.browser_version = browser.version();
  result.browser_scope = 'Owned native Chromium browser with an isolated temporary context.';
  context = await browser.newContext({ viewport: { width: 430, height: 900 } });
  const page = await context.newPage();
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(url + '/#/pages/login/index', { waitUntil: 'domcontentloaded', timeout: 90000 });
  const account = page.locator('input[placeholder="你的公司账号"]');
  await account.waitFor({ state: 'visible', timeout: 90000 });
  assert.equal(await account.count(), 1);
  await page.getByText('仅限获授权的公司成员使用', { exact: true }).waitFor({ state: 'visible' });
  checks.push({ name: 'Current Taro watch output renders the ordinary login form', passed: true });
  assert.equal(await page.locator('input[placeholder="输入密码"]').inputValue(), '');
  checks.push({ name: 'No credential or business data is entered or recorded', passed: true });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await account.waitFor({ state: 'visible' });
  assert.deepEqual(errors, []);
  checks.push({ name: 'Reload executes current bundles without page errors', passed: true });
  result.passed = true;
} catch (error) {
  result.error = String(error.message).slice(0, 1500);
} finally {
  if (context) {
    try { await context.close(); result.context_closed = true; } catch { result.context_closed = false; }
  }
  if (browser) {
    try { await browser.close(); result.browser_closed = !browser.isConnected(); } catch { result.browser_closed = false; }
  }
  result.passed = result.passed && result.context_closed && result.browser_closed;
  await writeFile(output, JSON.stringify(result, null, 2) + '\n');
}
console.log(JSON.stringify(result));
process.exitCode = result.passed ? 0 : 1;
