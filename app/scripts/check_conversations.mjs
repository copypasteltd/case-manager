// Real local API/UI checks. All content is synthetic; temporary roles are revoked.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { waitForAPI } from './wait_for_api.mjs';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development'); assert.equal(new URL(config.CP_PUBLIC_ORIGIN).hostname, 'localhost');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const manager = accounts.find(item => item.login === 'project.manager'), reader = accounts.find(item => item.login === 'project.member'), admin = accounts.find(item => item.login === 'system.admin');
const mfaFile = path.join(app, '.local/development-mfa.json'), mfa = JSON.parse(await readFile(mfaFile, 'utf8'));
assert(mfa.secret, 'Use the existing owned administrator MFA record; never reseed it');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/conversations'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const files = ['backend/internal/agent/conversations.go', 'backend/internal/agent/conversation_mutations.go', 'backend/internal/agent/context.go', 'backend/internal/httpapi/conversations.go', 'backend/migrations/0031_agent_conversations.sql', 'web/src/Conversations.tsx', 'web/src/ConversationSources.tsx', 'web/src/App.tsx', 'web/src/styles.css', 'packages/contracts/src/api.d.ts'];
async function sourceDigest() { const hash = createHash('sha256'); for (const file of files) { hash.update(file + '\n'); hash.update(await readFile(path.join(app, file))); } return hash.digest('hex'); }
const before = await sourceDigest(), started = new Date().toISOString();
const contexts = [], checks = [], errors = [], serverErrors = [], bindings = [];
let browser, administrator, passed = false, revoked = 0;
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
  const step = Math.floor(Date.now() / 30_000); mfa.last_step = step;
  await writeFile(mfaFile, JSON.stringify(mfa, null, 2) + '\n'); return otp(mfa.secret, step);
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account === admin) { await page.getByLabel('六位验证码').waitFor(); await page.getByLabel('六位验证码').fill(await nextCode()); await page.getByRole('button', { name: '验证并继续', exact: true }).click(); }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { context, page };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, { method, headers: { Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  const result = await response.json(); assert(response.ok(), `Fixture API failed: ${route} ${response.status()} ${result.error?.code}`); return result.data;
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  let owner = await login(manager), ordinary = await login(reader); administrator = await login(admin);
  const me = await request(owner.context, '/me'), other = await request(ordinary.context, '/me'), stamp = Date.now();
  const project = await request(owner.context, '/projects', 'POST', { name: '会话界面验证 ' + stamp, scope: '仅用于本地会话权限与界面检查，全部为虚构资料。', owner_id: me.user.id, member_ids: [other.user.id], classification: 'project_restricted' });
  const roles = await request(administrator.context, '/roles?limit=100'), role = roles.items.find(item => item.key === 'agent_user'); assert(role);
  for (const scope of [{ user_id: me.user.id, scope_type: 'own' }, { user_id: other.user.id, scope_type: 'project', scope_id: project.id }]) {
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { ...scope, role_id: role.id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3_600_000).toISOString(), reason: '本地虚构会话界面验证，结束即撤销临时授权。' }));
  }
  await owner.context.close(); owner = await login(manager);
  await ordinary.context.close(); ordinary = await login(reader);
  const { page, context } = owner;
  await page.getByRole('link', { name: '公司秘书', exact: true }).click();
  const privateTitle = '个人会话验证 ' + stamp, sharedTitle = '共享会话验证 ' + stamp;
  await page.getByRole('button', { name: '新建会话', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '新建会话', exact: true });
  await dialog.getByLabel('会话标题', { exact: true }).fill(privateTitle);
  await dialog.getByRole('button', { name: '创建会话', exact: true }).click();
  await page.getByRole('heading', { name: privateTitle, exact: true }).waitFor();
  const personalID = new URL(page.url()).pathname.split('/').at(-1);
  assert.equal((await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/conversations/' + personalID)).status(), 404);
  assert.equal((await administrator.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/conversations/' + personalID)).status(), 404);
  assert.equal(await page.getByRole('button', { name: '管理会话成员', exact: true }).count(), 0);
  checks.push('A real UI personal conversation remains accessible only to its creator; an unrelated employee and a configuration administrator receive 404');

  await page.getByRole('link', { name: '公司秘书', exact: true }).first().click();
  await page.getByRole('button', { name: '新建会话', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '新建会话', exact: true });
  await dialog.getByLabel('会话标题', { exact: true }).fill(sharedTitle);
  await dialog.getByLabel('会话类型', { exact: true }).selectOption('project');
  await dialog.getByLabel('关联项目', { exact: true }).selectOption(project.id);
  await dialog.getByRole('button', { name: '创建会话', exact: true }).click();
  await page.getByRole('heading', { name: sharedTitle, exact: true }).waitFor();
  const id = new URL(page.url()).pathname.split('/').at(-1);
  let current = await request(context, '/conversations/' + id);
  assert.equal(current.classification, 'project_restricted'); assert.equal(current.members.length, 1);
  assert.equal((await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/conversations/' + id)).status(), 404);
  await page.getByRole('button', { name: '管理会话成员', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '管理会话成员', exact: true });
  await dialog.getByLabel('选择成员', { exact: true }).selectOption(other.user.id);
  await dialog.getByLabel('变更原因', { exact: true }).fill('加入已有项目与会话权限的虚构成员。');
  await dialog.getByRole('button', { name: '确认加入会话', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  current = await request(context, '/conversations/' + id); assert.equal(current.members.length, 2); assert.equal(current.context_security_version, 2);
  await ordinary.page.goto(config.CP_PUBLIC_ORIGIN + '/conversations/' + id);
  await ordinary.page.getByRole('heading', { name: sharedTitle, exact: true }).waitFor();
  checks.push('Project membership alone cannot reveal a shared conversation; the UI explicitly adds an eligible member and advances its context security epoch');

  await page.getByRole('button', { name: '编辑会话标题', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '编辑会话标题', exact: true });
  await dialog.getByLabel('会话标题', { exact: true }).fill('保留本机未保存的标题');
  await dialog.getByLabel('变更原因', { exact: true }).fill('第二设备先保存时保留本机内容。');
  const changedTitle = '另一成员先保存 ' + stamp;
  await request(ordinary.context, '/conversations/' + id, 'PATCH', { expected_version: current.version, title: changedTitle, state: 'active', reason: '通过另一个真实登录会话更新。' });
  await dialog.getByRole('button', { name: '保存会话标题', exact: true }).click();
  await dialog.getByText('会话或授权已变化。已保留输入，请核对最新资料后重新编辑。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('会话标题', { exact: true }).inputValue(), '保留本机未保存的标题');
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.reload();
  await page.getByRole('heading', { name: changedTitle, exact: true }).waitFor();
  checks.push('A second real member changes the version; the stale editor reports a conflict and retains the unsaved local title');

  const sourceSection = page.getByRole('region', { name: '准备给秘书的资料', exact: true });
  await sourceSection.getByLabel('选择资料', { exact: true }).selectOption(project.id);
  const projectDenied = page.waitForResponse(response => response.url().includes('/context-sources/project/') && response.request().method() === 'GET');
  await sourceSection.getByRole('button', { name: '核对并预览', exact: true }).click();
  assert.equal((await projectDenied).status(), 404);
  await sourceSection.getByRole('alert').waitFor();
  assert.equal(await sourceSection.locator('.conversation-source-preview').count(), 0);
  const post = await request(context, '/posts', 'POST', { project_id: project.id, type: 'sop', title: '会话来源核对样本 ' + stamp, body: '此内容为虚构样本；分类默认未允许 AI，不应出现在秘书资料预览中。', tags: [], classification: 'project_restricted', source_refs: [] });
  await sourceSection.getByLabel('资料类型', { exact: true }).selectOption('post');
  await sourceSection.getByLabel('选择资料', { exact: true }).selectOption(post.id);
  assert.equal(await sourceSection.getByRole('alert').count(), 0);
  const postDenied = page.waitForResponse(response => response.url().includes('/context-sources/post/') && response.request().method() === 'GET');
  await sourceSection.getByRole('button', { name: '核对并预览', exact: true }).click();
  assert.equal((await postDenied).status(), 404);
  await sourceSection.getByRole('alert').waitFor();
  assert.equal(await sourceSection.locator('.conversation-source-preview').count(), 0);
  checks.push('Real project and knowledge choices are available by title; the default AI prohibition rejects both reads without displaying source bodies, and changing a choice clears the prior result');

  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 });
      await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
      const overflow = await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth, elements: [...document.body.querySelectorAll('*')].map(element => ({ tag: element.tagName, cls: String(element.className), right: element.getBoundingClientRect().right, width: element.getBoundingClientRect().width, visible: getComputedStyle(element).visibility })).filter(item => item.visible !== 'hidden' && item.right > innerWidth + 1).slice(0, 5) }));
      if (overflow.scroll > width) await page.screenshot({ path: path.join(temporary, 'conversation-overflow.png'), fullPage: true });
      assert(overflow.scroll <= width, `${theme} ${width} conversation overflow ${JSON.stringify(overflow)}`);
      if (theme === 'dark' && width === 1440) await page.screenshot({ path: path.join(evidence, 'detail-dark-1440.png'), fullPage: true });
    }
  }
  await page.getByLabel('外观偏好', { exact: true }).selectOption('light'); await page.setViewportSize({ width: 390, height: 1050 });
  await page.screenshot({ path: path.join(evidence, 'detail-light-390.png'), fullPage: true });
  await page.getByRole('button', { name: '管理会话成员', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '管理会话成员', exact: true });
  await dialog.getByLabel('成员操作', { exact: true }).selectOption('remove');
  await dialog.getByLabel('选择成员', { exact: true }).selectOption(other.user.id);
  assert(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth), 'mobile membership dialog overflow');
  await page.screenshot({ path: path.join(evidence, 'members-light-390.png'), fullPage: true });
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  checks.push('Both themes and five widths fit the real conversation; the 390px member dialog keeps controls inside its bounds');

  await page.getByRole('button', { name: '归档会话', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '归档会话', exact: true });
  await dialog.getByLabel('变更原因', { exact: true }).fill('核查归档后停止成员调整。');
  await dialog.getByRole('button', { name: '确认归档会话', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: '管理会话成员', exact: true }).count(), 0);
  await page.getByRole('button', { name: '恢复会话', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '恢复会话', exact: true });
  await dialog.getByLabel('变更原因', { exact: true }).fill('恢复后继续核查成员撤出。');
  await dialog.getByRole('button', { name: '确认恢复会话', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  assert.equal((await request(context, '/conversations/' + id)).context_security_version, 2);
  checks.push('Archive and restore change immutable conversation revisions while retaining the unchanged security epoch and blocking archived member changes');

  await ordinary.page.reload(); await ordinary.page.getByRole('heading', { name: changedTitle, exact: true }).waitFor();
  await ordinary.page.getByRole('button', { name: '管理会话成员', exact: true }).click();
  dialog = ordinary.page.getByRole('dialog', { name: '管理会话成员', exact: true });
  await dialog.getByLabel('成员操作', { exact: true }).selectOption('remove');
  await dialog.getByLabel('选择成员', { exact: true }).selectOption(other.user.id);
  await dialog.getByLabel('变更原因', { exact: true }).fill('从非创建人的真实界面退出此共享会话。');
  await dialog.getByRole('button', { name: '确认移出会话', exact: true }).click();
  await ordinary.page.waitForURL(config.CP_PUBLIC_ORIGIN + '/conversations');
  await ordinary.page.getByRole('heading', { name: '公司秘书', exact: true }).waitFor();
  assert.equal(await ordinary.page.getByRole('link', { name: new RegExp(changedTitle) }).count(), 0);
  assert.equal((await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/conversations/' + id)).status(), 404);
  current = await request(context, '/conversations/' + id); assert.equal(current.context_security_version, 3); assert.equal(current.members.length, 1);
  checks.push('A non-creator removes themself through the real UI, clears the cached detail, leaves the conversation and loses API access immediately');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  if (administrator) for (const binding of bindings) {
    try { await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '本地虚构会话界面验证结束，撤销临时授权。' }); revoked++; }
    catch (error) { errors.push(error.message); passed = false; }
  }
  let allBrowserContextsClosed = true;
  for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const after = await sourceDigest();
  const result = { passed: passed && revoked === bindings.length && allBrowserContextsClosed && before === after, started_at: started, completed_at: new Date().toISOString(), real_api: true, synthetic_data: true, source_files: files, source_sha256: before, source_unchanged: before === after, temporary_bindings_created: bindings.length, temporary_bindings_revoked: revoked, checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2)); if (!result.passed) process.exitCode = 1;
}
