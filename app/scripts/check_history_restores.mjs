// Real API and native browser UI checks. The labelled fixture has unavailable
// bytes, no scan report, no worker job and no successful-copy evidence.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
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
const manager = accounts.find(user => user.login === 'project.manager'), reader = accounts.find(user => user.login === 'project.member');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/history-restores'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [];
let browser, passed = false;
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  return { context, page };
}
async function request(context, route, method = 'GET', body, expected = 200) {
  const csrf = (await context.cookies()).find(value => value.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, { method, headers: {
    Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID(),
  }, ...(body ? { data: body } : {}) });
  const result = await response.json();
  assert.equal(response.status(), expected, route + ': ' + result.error?.code);
  return result.data;
}
const fixture = async (script, args) => JSON.parse((await run(process.env.CP_PYTHON ?? 'python',
  [path.join(app, 'scripts', script), ...args], { cwd: app, windowsHide: true, encoding: 'utf8' })).stdout);
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const { context, page } = await login(manager), second = await login(reader);
  const me = await request(context, '/me'), other = await request(second.context, '/me'), stamp = Date.now();
  const project = await request(context, '/projects', 'POST', { name: '存储验证项目 ' + stamp,
    scope: '历史恢复页面检查，只有明确标记的不可用元数据，不含实际云对象。', owner_id: me.user.id,
    member_ids: [other.user.id], classification: 'project_restricted' }, 201);
  const folders = await request(context, '/folders?project_id=' + project.id);
  const projectRoot = folders.items.find(folder => folder.parent_id === null);
  assert(projectRoot);
  const folder = await request(context, '/folders', 'POST', { parent_id: projectRoot.id,
    name: '界面验证资料 ' + stamp, classification: 'project_restricted' }, 201);
  const stored = await fixture('prepare_storage_browser_fixture.py', ['--folder', folder.id, '--owner', me.user.id]);
  assert(stored.metadata_only && !stored.cloud_objects_created);
  await page.goto(config.CP_PUBLIC_ORIGIN + '/files/' + stored.file_id);
  await page.getByRole('heading', { name: '元数据样本-120GB.bin', exact: true }).waitFor();
  const history = page.locator('#history-restoration');
  await history.getByRole('heading', { name: '尚无历史恢复任务', exact: true }).waitFor();
  const restoreButton = page.getByRole('button', { name: '恢复为新版本', exact: true });
  assert.equal(await restoreButton.count(), 1); assert(await restoreButton.isDisabled());
  assert.equal(await page.getByRole('button', { name: '下载此内容版本', exact: true }).count(), 2);
  checks.push('Historical restoration has its own task section; unavailable unscannable history cannot create a recovery candidate or claim readable bytes');
  const seeded = await fixture('prepare_history_browser_fixture.py', ['--file', stored.file_id, '--owner', me.user.id]);
  assert(seeded.metadata_only && !seeded.cloud_objects_created && !seeded.scan_reports_created);
  await page.reload();
  const card = page.locator('[data-restore-id="' + seeded.restore_id + '"]');
  await card.getByText('等待复制', { exact: true }).waitFor();
  await card.getByText('内容版本 1', { exact: true }).waitFor(); await card.getByText('内容版本 2', { exact: true }).waitFor();
  assert.equal(await card.getByRole('button', { name: '确认并生成正式版本', exact: true }).count(), 0);
  assert.equal(await card.getByText('0 字节 / 120,000,000,000 字节', { exact: true }).count(), 2);
  assert.equal(await page.getByText('已校验', { exact: true }).count(), 0);
  checks.push('A labelled queued metadata task renders human content revisions and separate zero copy/hash progress without fabricated ready, cloud or scanning evidence');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [1440, 1024, 768, 390, 320]) {
      await page.setViewportSize({ width, height: width < 500 ? 844 : 1050 });
      await card.scrollIntoViewIfNeeded();
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, theme + '/' + width);
      assert(await card.getByRole('button', { name: '取消恢复任务', exact: true }).isVisible());
      if (width === 1440 || (theme === 'light' && width === 390)) await page.screenshot({
        path: path.join(evidence, 'history-' + theme + '-' + width + '.png'), animations: 'disabled', fullPage: true,
      });
    }
  }
  checks.push('Both brand themes at 1440/1024/768/390/320 retain separate progress, readable content revisions and cancellation with no root horizontal overflow');
  await page.setViewportSize({ width: 390, height: 844 }); await page.getByLabel('外观偏好').selectOption('light');
  await card.getByRole('button', { name: '取消恢复任务', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '取消历史恢复任务', exact: true });
  await dialog.getByLabel('取消原因', { exact: true }).fill('仅取消界面元数据任务，不删除任何历史内容。');
  await page.screenshot({ path: path.join(evidence, 'cancel-light-390.png'), animations: 'disabled' });
  await dialog.getByRole('button', { name: '确认取消恢复', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  await card.getByText('已取消', { exact: true }).waitFor();
  const after = await request(context, '/files/' + stored.file_id), versions = await request(context, '/files/' + stored.file_id + '/versions');
  assert.equal(after.current_version_id, stored.version_ids[1]); assert.equal(versions.items.length, 2);
  checks.push('Mobile cancellation confirms a fixed task version through the real API and preserves both immutable versions and the current content pointer');
  await request(second.context, '/history-restores/' + seeded.restore_id, 'GET', undefined, 404);
  await request(second.context, '/history-restores/' + seeded.restore_id + '/cancel', 'POST', { expected_version: 1, reason: 'not the owner' }, 404);
  const otherFile = await request(second.context, '/files/' + stored.file_id);
  await second.page.goto(config.CP_PUBLIC_ORIGIN + '/files/' + stored.file_id);
  await second.page.getByRole('heading', { name: '元数据样本-120GB.bin', exact: true }).waitFor();
  assert.equal(await second.page.locator('[data-restore-id]').count(), 0);
  assert.equal(await second.page.getByRole('button', { name: '恢复为新版本', exact: true }).count(), otherFile.can_restore_history ? 1 : 0);
  checks.push('Another real employee cannot read or cancel the owner task; the file UI follows server-provided restoration capabilities');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  errors.push(String(error)); process.exitCode = 1;
} finally {
  await Promise.allSettled(contexts.map(context => context.close())); if (browser) await browser.close();
  const report = { passed, executed_at: new Date().toISOString(), checks, errors, server_errors: serverErrors,
    source: 'node app/scripts/check_history_restores.mjs', contexts_closed: true, browser_closed: true,
    scope: 'Real API/UI and explicitly unavailable metadata fixture only. No cloud objects, scan reports, recovery copy, ready commit, real 120 GB transfer or native mobile build.' };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify(report, null, 2));
}
