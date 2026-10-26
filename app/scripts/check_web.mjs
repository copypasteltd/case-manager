// Native browser verification. Every context and page created here is closed
// in finally; this script never attaches to the user's existing browser.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile, mkdir, writeFile, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { waitForAPI } from './wait_for_api.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
const root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development', 'Browser fixture may only mutate local development data');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const account = accounts.find(item => item.login === 'project.manager');
assert(account, 'Create the development identities first');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/web');
const temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const base = config.CP_PUBLIC_ORIGIN;
assert.equal(new URL(base).hostname, 'localhost');
const javascriptErrors = [];
const serverErrors = [];
const checks = [];
let browser, context;
let passed = false;
try {
  browser = await chromium.launch({ executablePath, headless: true });
  context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  const page = await context.newPage();
  page.setDefaultTimeout(15_000); page.setDefaultNavigationTimeout(30_000);
  page.on('pageerror', error => javascriptErrors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(base);
  await page.getByRole('heading', { name: '回到工作空间' }).waitFor();
  await page.screenshot({ path: path.join(evidence, 'login-light-1440.png'), fullPage: true });
  await page.getByLabel('内部账号', { exact: true }).fill(account.login);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  checks.push('Web login through HttpOnly session');
  const cookies = await context.cookies();
  assert(cookies.find(item => item.name === 'cp_session')?.httpOnly);
  assert(cookies.find(item => item.name === 'cp_refresh')?.httpOnly);
  await page.getByRole('link', { name: '项目空间', exact: true }).click();
  await page.getByRole('heading', { name: '项目空间.' }).waitFor();
  await page.getByRole('button', { name: '新建项目', exact: true }).first().click();
  const name = '开发验证 · 协作流程 ' + Date.now();
  await page.getByLabel('项目名称', { exact: true }).fill(name);
  await page.getByLabel('项目范围与目标').fill('验证品牌一致的工作空间、真实项目写入与可追踪的任务流转。');
  await page.getByRole('button', { name: '创建项目', exact: true }).click();
  await page.getByRole('link').filter({ hasText: name }).first().click();
  await page.getByRole('heading', { name, exact: true }).waitFor();
  checks.push('Create and open a persisted project');
  await page.getByRole('button', { name: '新建任务', exact: true }).first().click();
  await page.getByLabel('任务名称').fill('验证项目任务与验收流程');
  await page.getByLabel('描述与验收标准').fill('完成真实 API 写入，保留版本和审计记录，并检查小屏交互。');
  await page.getByLabel('负责人', { exact: true }).selectOption({ label: account.display_name });
  await page.getByRole('button', { name: '创建任务', exact: true }).click();
  await page.getByRole('button', { name: '验证项目任务与验收流程', exact: true }).click();
  await page.getByRole('button', { name: '开始任务', exact: true }).click();
  await page.getByLabel('变更原因').fill('浏览器验证：进入执行阶段');
  await page.getByRole('button', { name: '开始任务', exact: true }).click();
  await page.getByRole('dialog').getByText('进行中', { exact: true }).waitFor();
  await page.getByRole('button', { name: '关闭弹窗' }).click();
  checks.push('Create, assign and transition a task');
  await page.screenshot({ path: path.join(evidence, 'project-light-1440.png'), fullPage: true });
  await page.getByRole('button', { name: '切换至深色主题' }).click();
  await page.screenshot({ path: path.join(evidence, 'project-dark-1440.png'), fullPage: true });
  await page.getByRole('button', { name: '切换至浅色主题' }).click();
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 960 });
    const dimensions = await page.evaluate(() => ({ width: window.innerWidth, document: document.documentElement.scrollWidth }));
    assert(dimensions.document <= dimensions.width, 'Root horizontal overflow at ' + width);
    if (width === 390) {
      await page.getByRole('button', { name: '打开导航' }).click();
      await page.getByRole('link', { name: '工作台', exact: true }).click();
      await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
      await page.screenshot({ path: path.join(evidence, 'dashboard-light-390.png'), fullPage: true });
      const targets = await page.locator('.mobile-menu, .topbar .icon-button').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().height));
      assert(targets.every(height => height >= 48), 'Mobile toolbar targets are too small');
    }
  }
  checks.push('320/390/768/1024/1440 root overflow and mobile navigation');
  await page.getByRole('link', { name: '工作台', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.getByRole('link', { name: /验证项目任务与验收流程/ }).waitFor();
  await page.screenshot({ path: path.join(evidence, 'dashboard-light-1440.png'), fullPage: true });
  await page.getByRole('button', { name: '切换至深色主题' }).click();
  await page.screenshot({ path: path.join(evidence, 'dashboard-dark-1440.png'), fullPage: true });
  checks.push('Authorized dashboard shows assigned persisted task');
  await page.getByTitle('退出登录').click();
  await page.getByRole('heading', { name: '回到工作空间' }).waitFor();
  checks.push('Logout removes authenticated workspace');
  assert.deepEqual(javascriptErrors, [], 'Unhandled browser JavaScript exceptions');
  assert.deepEqual(serverErrors, [], 'API server failures');
  passed = true;
} finally {
  try { await context?.close(); } finally { await browser?.close(); }
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), checks, javascript_errors: javascriptErrors, server_errors: serverErrors, browser_closed: !browser?.isConnected() }, null, 2) + '\n');
}
console.log(JSON.stringify({ checks, javascript_errors: javascriptErrors.length, server_errors: serverErrors.length, browser_closed: true }));
