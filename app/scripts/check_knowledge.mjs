// Real local API/UI verification. No user browser/profile or cloud is used.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { waitForAPI } from './wait_for_api.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
const root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development');
assert.equal(new URL(config.CP_PUBLIC_ORIGIN).hostname, 'localhost');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const manager = accounts.find(value => value.login === 'project.manager');
const reader = accounts.find(value => value.login === 'project.member');
assert(manager && reader);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/knowledge');
const temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [], externalImages = [];
let browser, passed = false;

async function login(account, colorScheme = 'light') {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme });
  contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(20_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  page.on('request', request => { if (new URL(request.url()).hostname === 'invalid.example') externalImages.push(request.resourceType()); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  return { context, page };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(value => value.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, {
    method, headers: { Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() },
    ...(body ? { data: body } : {}),
  });
  const result = await response.json();
  assert(response.ok(), 'Owned fixture API request failed: ' + route + ' ' + response.status() + ' ' + result.error?.code);
  return result.data;
}
async function publish(page) {
  await page.getByRole('button', { name: '内部发布', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '内部发布文档', exact: true });
  await dialog.getByLabel('变更原因', { exact: true }).fill('浏览器验证：核对当前固定修订后内部发布');
  await dialog.getByRole('button', { name: '内部发布文档', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: '归档', exact: true }).waitFor();
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const { page, context } = await login(manager, 'dark');
  assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
  await page.emulateMedia({ colorScheme: 'light' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  await page.getByLabel('外观偏好').selectOption('dark');
  await page.reload(); await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
  await page.getByLabel('外观偏好').selectOption('system');
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  checks.push('System theme follows the OS; explicit theme persists and can return to system mode');

  await page.getByRole('link', { name: '项目空间', exact: true }).click();
  await page.getByRole('button', { name: '新建项目', exact: true }).first().click();
  const projectName = '知识验证项目 ' + Date.now();
  await page.getByLabel('项目名称', { exact: true }).fill(projectName);
  await page.getByLabel('项目范围与目标').fill('验证知识文档的完整修订、发布与讨论流程。');
  await page.getByRole('button', { name: '创建项目', exact: true }).click();
  await page.getByRole('link').filter({ hasText: projectName }).first().click();
  const projectURL = page.url();
  await page.getByRole('tab', { name: '成员', exact: true }).click();
  await page.getByRole('button', { name: '添加成员', exact: true }).click();
  await page.getByLabel('选择成员', { exact: true }).selectOption({ label: reader.display_name });
  await page.getByLabel('项目角色').selectOption('viewer');
  await page.getByLabel('变更原因', { exact: true }).fill('浏览器验证：只读成员查看已发布知识');
  await page.getByRole('button', { name: '保存成员变更', exact: true }).click();
  await page.getByRole('cell', { name: reader.display_name, exact: true }).waitFor();

  await page.getByRole('link', { name: '知识与记录', exact: true }).click();
  await page.getByRole('button', { name: '新建文档', exact: true }).click();
  const originalTitle = '可追溯知识 ' + Date.now();
  const originalBody = '# 交付结论\n\n- **已核对资料**\n- 下一步跟进\n\n> 来源记录\n\n<script>window.cpUnsafeExecuted=true</script>\n![外部图像](https://invalid.example/pixel)';
  await page.getByLabel('文档标题', { exact: true }).fill(originalTitle);
  await page.getByLabel('文档类型').selectOption('sop');
  await page.getByLabel('关联项目', { exact: true }).selectOption({ label: projectName });
  await page.getByLabel('文档摘要').fill('由真实接口保存的内部交接结论。');
  await page.getByLabel('Markdown 正文', { exact: true }).fill(originalBody);
  await page.getByLabel('标签', { exact: true }).fill('知识验证，交付流程');
  await page.getByRole('button', { name: '预览正文', exact: true }).click();
  await page.getByRole('heading', { name: '交付结论', exact: true }).waitFor();
  await page.getByRole('button', { name: '保存文档草稿', exact: true }).click();
  await page.getByRole('heading', { name: originalTitle, exact: true }).waitFor();
  const postURL = page.url(), postID = new URL(postURL).pathname.split('/').at(-1);
  assert.equal(await page.evaluate(() => window.cpUnsafeExecuted), undefined);
  assert.equal(await page.locator('.post-document script,.post-document img,.post-document iframe').count(), 0);
  await publish(page);
  checks.push('Create and preview a real project SOP, safely render untrusted Markdown, and publish its fixed revision');

  await page.getByLabel('写一条评论', { exact: true }).fill('第一条交接反馈');
  await page.getByRole('button', { name: '发布评论', exact: true }).click();
  await page.locator('.comment-item').getByText('第一条交接反馈', { exact: true }).waitFor();
  await page.locator('.comment-item').getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByLabel('修改我的评论', { exact: true }).fill('核对后的交接反馈');
  await page.getByRole('button', { name: '保存评论修订', exact: true }).click();
  await page.locator('.comment-item').getByText('核对后的交接反馈', { exact: true }).waitFor();
  await page.getByRole('button', { name: '修改记录', exact: true }).click();
  const commentHistory = page.getByRole('dialog', { name: '评论修改记录', exact: true });
  await commentHistory.getByText('第一条交接反馈', { exact: true }).waitFor();
  await commentHistory.getByText('核对后的交接反馈', { exact: true }).waitFor();
  await commentHistory.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Comment editing creates a new revision and the original remains readable in its history');
  await page.screenshot({ path: path.join(evidence, 'published-light-1440.png'), fullPage: true });

  const draftTitle = '未发布修订 ' + Date.now();
  const retainedComment = '正文并发更新时，这段尚未提交的评论必须保留。';
  await page.getByLabel('写一条评论', { exact: true }).fill(retainedComment);
  const current = await request(context, '/posts/' + postID);
  await request(context, '/posts/' + postID, 'PATCH', { expected_version: current.version, base_revision_id: current.current_revision_id, title: draftTitle, body: '# 未发布正文\n\n此内容在发布前不得出现在普通读者页面。' });
  await page.getByRole('button', { name: '发布评论', exact: true }).click();
  await page.getByRole('button', { name: '已核对当前正文', exact: true }).waitFor();
  assert.equal(await page.getByLabel('写一条评论', { exact: true }).inputValue(), retainedComment);
  assert.equal(await page.getByRole('button', { name: '发布评论', exact: true }).isDisabled(), true);
  await page.getByRole('button', { name: '已核对当前正文', exact: true }).click();
  await page.getByRole('button', { name: '发布评论', exact: true }).click();
  await page.locator('.comment-item').getByText(retainedComment, { exact: true }).waitFor();
  checks.push('A stale comment is rejected; its text survives refresh and requires review before submission on the new revision');

  const { page: viewer } = await login(reader);
  await viewer.goto(postURL);
  await viewer.getByRole('heading', { name: originalTitle, exact: true }).waitFor();
  assert.equal(await viewer.getByRole('heading', { name: draftTitle, exact: true }).count(), 0);
  assert.equal(await viewer.getByText(retainedComment, { exact: true }).count(), 0);
  assert.equal(await viewer.getByRole('button', { name: '编辑草稿', exact: true }).count(), 0);
  assert.equal(await viewer.getByLabel('写一条评论', { exact: true }).count(), 0);
  await viewer.locator('.comment-item').getByText('核对后的交接反馈', { exact: true }).waitFor();
  await viewer.screenshot({ path: path.join(evidence, 'reader-published-light-1440.png'), fullPage: true });
  checks.push('A project viewer sees the published title, body and discussion while the new draft and its comments remain hidden');

  await page.getByRole('button', { name: '编辑草稿', exact: true }).click();
  await page.getByLabel('文档摘要').fill('补充后的摘要，仍保存为未发布草稿。');
  await page.getByRole('button', { name: '保存文档草稿', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: '修订记录', exact: true }).click();
  const postHistory = page.getByRole('dialog', { name: '文档修订记录', exact: true });
  await postHistory.getByRole('button', { name: new RegExp('修订 1 · ' + originalTitle) }).click();
  await postHistory.getByLabel('恢复原因', { exact: true }).fill('浏览器验证：以历史正文生成新草稿');
  await postHistory.getByRole('button', { name: '恢复为新草稿', exact: true }).click();
  await postHistory.waitFor({ state: 'hidden' });
  await page.getByRole('heading', { name: originalTitle, exact: true }).waitFor();
  await publish(page);
  checks.push('Edit a draft and restore an immutable historical revision into a new draft before publishing it');

  await page.getByLabel('外观偏好').selectOption('dark');
  await page.screenshot({ path: path.join(evidence, 'restored-dark-1440.png'), fullPage: true });
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 960 });
    const size = await page.evaluate(() => ({ window: innerWidth, content: document.documentElement.scrollWidth }));
    assert(size.content <= size.window, 'Knowledge page root overflow at ' + width);
    if (width < 900) {
      assert.equal(await page.locator('.sidebar').getAttribute('aria-hidden'), 'true');
      assert.equal(await page.locator('.sidebar').evaluate(node => node.inert && getComputedStyle(node).visibility === 'hidden'), true);
    }
  }
  await page.setViewportSize({ width: 390, height: 960 });
  await page.getByLabel('外观偏好').selectOption('light');
  await page.getByRole('button', { name: '打开导航', exact: true }).click();
  const navigation = page.getByRole('dialog', { name: '工作区导航', exact: true });
  await navigation.waitFor();
  assert.equal(await page.locator('.main-shell').evaluate(node => node.inert), true);
  assert.equal(await navigation.evaluate(node => node.contains(document.activeElement)), true);
  await page.keyboard.press('Shift+Tab');
  assert.equal(await navigation.evaluate(node => node.contains(document.activeElement)), true);
  await page.keyboard.press('Escape');
  await navigation.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '打开导航', exact: true }).evaluate(node => node === document.activeElement), true);
  assert.equal(await page.locator('.main-shell').evaluate(node => node.inert), false);
  await page.screenshot({ path: path.join(evidence, 'restored-light-390.png'), fullPage: true, animations: 'disabled' });
  checks.push('Knowledge pages fit five viewport widths; mobile navigation hides fully, traps focus while open and restores focus when closed');
  await page.setViewportSize({ width: 1440, height: 1050 });
  await page.getByRole('button', { name: '归档', exact: true }).click();
  const archive = page.getByRole('dialog', { name: '归档文档', exact: true });
  await archive.getByLabel('变更原因', { exact: true }).fill('浏览器验证：归档并保留历史');
  await archive.getByRole('button', { name: '归档文档', exact: true }).click();
  await archive.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '编辑草稿', exact: true }).count(), 0);
  assert.equal(await page.getByLabel('写一条评论', { exact: true }).count(), 0);
  await page.getByRole('button', { name: '修订记录', exact: true }).click();
  const archivedHistory = page.getByRole('dialog', { name: '文档修订记录', exact: true });
  await archivedHistory.getByRole('button', { name: new RegExp('修订 1 · ' + originalTitle) }).click();
  assert.equal(await archivedHistory.getByRole('button', { name: '恢复为新草稿', exact: true }).count(), 0);
  await archivedHistory.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Archived posts retain readable revision history and expose no ordinary edit, comment or restore action');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); assert.deepEqual(externalImages, []);
  passed = true;
} finally {
  try { for (const context of contexts) await context.close(); } finally { await browser?.close(); }
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), checks, javascript_errors: errors, server_errors: serverErrors, external_image_requests: externalImages, browser_closed: !browser?.isConnected() }, null, 2) + '\n');
}
console.log(JSON.stringify({ passed, checks, browser_closed: true }));
