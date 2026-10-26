// Real UI/API checks with an explicitly labelled metadata-only local fixture.
// There are no object uploads, scans, cloud copies or simulated network APIs.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { waitForAPI } from './wait_for_api.mjs';

const run = promisify(execFile);
const app = fileURLToPath(new URL('../', import.meta.url));
const root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development');
assert.equal(new URL(config.CP_PUBLIC_ORIGIN).hostname, 'localhost');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const manager = accounts.find(value => value.login === 'project.manager'), reader = accounts.find(value => value.login === 'project.member');
assert(manager && reader);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/files');
const temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [];
let browser, passed = false;
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(20_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  return { context, page };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(value => value.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, { method,
    headers: { Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() },
    ...(body ? { data: body } : {}),
  });
  const result = await response.json();
  assert(response.ok(), 'Fixture request failed: ' + route + ' ' + response.status() + ' ' + result.error?.code);
  return result.data;
}
async function directory(page, name) {
  await page.getByRole('button', { name: '新建目录', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '新建资料目录', exact: true });
  await dialog.getByLabel('目录名称', { exact: true }).fill(name);
  await dialog.getByRole('button', { name: '创建目录', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  return page.locator('.directory-card > a').filter({ hasText: name });
}
async function move(page, name, target) {
  await page.getByRole('button', { name: '修改目录 ' + name, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '修改资料目录', exact: true });
  await dialog.getByLabel('目录名称', { exact: true }).fill(target);
  await dialog.getByLabel('变更原因', { exact: true }).fill('浏览器验证：保持相同项目与分类');
  await dialog.getByRole('button', { name: '保存变更', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const { context, page } = await login(manager);
  await page.getByRole('link', { name: '项目空间', exact: true }).click();
  await page.getByRole('button', { name: '新建项目', exact: true }).first().click();
  const stamp = Date.now(), projectName = '存储验证项目 ' + stamp, folderName = '界面验证资料 ' + stamp;
  await page.getByLabel('项目名称', { exact: true }).fill(projectName);
  await page.getByLabel('项目范围与目标').fill('仅验证目录与元数据；未上传或复制任何云对象。');
  await page.getByRole('button', { name: '创建项目', exact: true }).click();
  await page.getByRole('link').filter({ hasText: projectName }).first().click();
  const projectID = new URL(page.url()).pathname.split('/').at(-1);
  await page.goto(config.CP_PUBLIC_ORIGIN + '/files?project_id=' + projectID);
  await page.locator('.directory-card > a').filter({ hasText: '项目资料' }).click();
  await (await directory(page, folderName)).click();
  const folderID = new URL(page.url()).pathname.split('/').at(-1);
  await directory(page, 'ＡBC');
  await page.getByRole('button', { name: '新建目录', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '新建资料目录', exact: true });
  await dialog.getByLabel('目录名称').fill('abc');
  await dialog.getByRole('button', { name: '创建目录', exact: true }).click();
  await dialog.getByRole('alert').filter({ hasText: '记录已存在' }).waitFor();
  assert.equal(await dialog.getByLabel('目录名称').inputValue(), 'abc');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await move(page, 'ＡBC', '合同资料');
  await directory(page, '参考资料');
  await page.getByRole('button', { name: '修改目录 合同资料', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '修改资料目录', exact: true });
  await dialog.getByRole('button', { name: '选择其他目录', exact: true }).click();
  await dialog.getByRole('button', { name: '项目资料', exact: true }).click();
  await dialog.getByRole('button', { name: folderName, exact: true }).click();
  await dialog.getByRole('button', { name: '选用目录 参考资料', exact: true }).click();
  await dialog.getByLabel('变更原因').fill('浏览器验证：将合同资料整理到参考目录');
  await dialog.getByRole('button', { name: '保存变更', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '修改目录 合同资料', exact: true }).count(), 0);
  checks.push('Real directory creation, Unicode duplicate rejection with retained input, rename and same-scope directory move');
  await page.screenshot({ path: path.join(evidence, 'directory-light-1440.png'), fullPage: true, animations: 'disabled' });
  const me = await request(context, '/me');
  const prepared = await run(process.env.CP_PYTHON ?? 'python', [path.join(app, 'scripts/prepare_storage_browser_fixture.py'), '--folder', folderID, '--owner', me.user.id], { cwd: app, windowsHide: true, encoding: 'utf8' });
  const fixture = JSON.parse(prepared.stdout); assert(fixture.metadata_only && !fixture.cloud_objects_created);
  await page.reload();
  await page.getByRole('link', { name: /元数据样本-120GB.bin/ }).click();
  await page.getByRole('heading', { name: '内容版本与存储记录', exact: true }).waitFor();
  await page.locator('.version-card').first().waitFor();
  assert.equal(await page.locator('.version-card').count(), 2);
  assert.equal(await page.getByText('120,000,000,000 字节', { exact: true }).count(), 2);
  assert.equal(await page.getByText('无法扫描 · 缺失', { exact: true }).count(), 2);
  await page.getByText('复制失败', { exact: true }).first().waitFor();
  assert.equal(await page.getByText('已校验', { exact: true }).count(), 0);
  assert.equal((await page.textContent('body')).includes('test-internal-detail-must-not-be-disclosed'), false);
  await page.getByLabel('外观偏好').selectOption('dark');
  await page.screenshot({ path: path.join(evidence, 'versions-dark-1440.png'), fullPage: true, animations: 'disabled' });
  checks.push('Two immutable metadata versions render decimal 120 GB size, unavailable scan/source and independent queued/failed replicas without false verification or private storage detail');
  await page.getByRole('button', { name: '重新安排副本复制', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '重新安排副本复制', exact: true });
  await dialog.getByLabel('重试原因', { exact: true }).fill('元数据夹具没有真实对象，应保留失败与输入供核对');
  await dialog.getByRole('button', { name: '安排复制', exact: true }).click();
  await dialog.getByRole('alert').filter({ hasText: '缺少可验证的来源副本' }).waitFor();
  assert.equal(await dialog.getByLabel('重试原因').inputValue(), '元数据夹具没有真实对象，应保留失败与输入供核对');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await page.getByRole('button', { name: '重新安排副本复制', exact: true }).waitFor();
  checks.push('Replica retry dialog uses the real API, retains reason when its metadata-only source is unavailable, and does not claim a cloud copy was queued');
  const downloads = page.getByRole('button', { name: '下载此内容版本', exact: true });
  assert.equal(await downloads.count(), 2);
  for (const button of await downloads.all()) assert(await button.isDisabled());
  assert.equal(await page.getByText('此版本尚未具备可用内容与扫描或放行条件。', { exact: true }).count(), 2);
  checks.push('Unscannable missing metadata fixtures cannot start downloads; the UI explains unmet availability and scanning conditions without inventing usable cloud bytes');
  await page.getByRole('button', { name: '修改位置与名称', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '修改文件位置与名称', exact: true });
  await dialog.getByLabel('文件名称').fill('元数据样本-已重命名.bin'); await dialog.getByLabel('变更原因').fill('浏览器验证：只修改名称');
  await dialog.getByRole('button', { name: '保存变更', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('heading', { name: '元数据样本-已重命名.bin', exact: true }).waitFor();
  assert.equal(await page.getByText('原始素材.bin', { exact: true }).count(), 2);
  await page.getByRole('button', { name: '移入回收站', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '移入回收站', exact: true }); await dialog.getByLabel('操作原因').fill('浏览器验证：保留历史的软删除');
  await dialog.getByRole('button', { name: '移入回收站', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: '恢复至原目录', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '恢复至原目录', exact: true }); await dialog.getByLabel('操作原因').fill('浏览器验证：恢复逻辑文件');
  await dialog.getByRole('button', { name: '恢复至原目录', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  const after = await request(context, '/files/' + fixture.file_id);
  assert.equal(after.current_version_id, fixture.version_ids[1]); assert.equal(after.version, 4);
  checks.push('File rename, trash and untrash preserve current immutable version and original names');
  const outsider = await login(reader);
  await outsider.page.goto(config.CP_PUBLIC_ORIGIN + '/files/' + fixture.file_id);
  await outsider.page.getByRole('alert').filter({ hasText: '资料不存在或你没有访问权限' }).waitFor();
  assert.equal(await outsider.page.locator('.version-card').count(), 0);
  checks.push('A nonmember receives no file metadata or version history in the UI');
  await page.getByLabel('外观偏好').selectOption('light');
  for (const width of [1440, 1024, 768, 390, 320]) {
    await page.setViewportSize({ width, height: 1050 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'File detail overflows viewport ' + width);
  }
  await page.setViewportSize({ width: 390, height: 1050 });
  await page.screenshot({ path: path.join(evidence, 'versions-light-390.png'), fullPage: true, animations: 'disabled' });
  checks.push('File details fit five viewport widths and both themes; 120 GB is metadata only, never an upload test');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  try { for (const context of contexts) await context.close(); } finally { await browser?.close(); }
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), checks, metadata_fixture_only: true, cloud_objects_created: false, javascript_errors: errors, server_errors: serverErrors, browser_closed: !browser?.isConnected() }, null, 2) + '\n');
}
console.log(JSON.stringify({ passed, checks, browser_closed: true }));
