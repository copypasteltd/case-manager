// Real UI/API proof against an explicitly held, disposable native PG fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'agent-context-and-classification');
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
assert.equal(fixture.controlled_provider, true); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/agent-sources'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const sourceFiles = ['backend/internal/agent/context.go', 'backend/internal/agent/conversations.go', 'backend/internal/identity/classification.go', 'backend/internal/httpapi/administration.go', 'backend/internal/httpapi/conversations.go', 'backend/migrations/0032_classification_policy_publication.sql', 'web/src/ClassificationPolicies.tsx', 'web/src/ConversationSources.tsx', 'web/src/Conversations.tsx', 'web/src/App.tsx', 'web/src/styles.css', 'packages/contracts/src/api.d.ts'];
async function digest() { const hash = createHash('sha256'); for (const file of sourceFiles) { hash.update(file + '\n'); hash.update(await readFile(path.join(app, file))); } return hash.digest('hex'); }
const before = await digest(), started = new Date().toISOString();
const contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map(); let browser, passed = false;
function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000); page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) {
    await page.getByLabel('六位验证码').waitFor(); const previous = lastSteps.get(account.login) ?? account.last_step;
    if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 300));
    const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); await page.getByLabel('六位验证码').fill(otp(account.mfa_secret, step)); await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { page, context };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  const result = await response.json(); assert(response.ok(), route + ' ' + response.status() + ' ' + result.error?.code); return result.data;
}
async function preview(page, conversation, type) {
  await page.goto(fixture.origin + '/conversations/' + conversation);
  const section = page.getByRole('region', { name: '准备给秘书的资料', exact: true });
  await section.getByLabel('资料类型', { exact: true }).selectOption(type);
  await section.getByLabel('选择资料', { exact: true }).selectOption(type === 'post' ? fixture.post_id : fixture.project_id);
  await section.getByRole('button', { name: '核对并预览', exact: true }).click();
  return section;
}
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => { const sidebar = document.querySelector('aside.sidebar'); return !sidebar || sidebar.inert === (innerWidth < 900); });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const owner = await login(fixture.alice), reader = await login(fixture.bob), administrator = await login(fixture.administrator);
  const { page, context } = owner;
  assert.equal((await administrator.context.request.get(fixture.origin + '/v1/conversations/' + fixture.conversation_id)).status(), 404);
  await administrator.page.goto(fixture.origin + '/classification-policies'); await administrator.page.getByRole('heading', { name: '资料分类与渠道', exact: true }).waitFor();
  await administrator.page.getByRole('button', { name: '发布策略新修订', exact: true }).first().waitFor();
  assert.equal(await administrator.page.getByRole('button', { name: '发布策略新修订', exact: true }).count(), 3);
  checks.push('The configuration administrator sees current classification policies through a real MFA session but receives 404 for the business conversation');

  let section = await preview(page, fixture.conversation_id, 'post');
  await section.locator('.conversation-source-preview').waitFor();
  assert.equal(await section.getByRole('heading', { name: fixture.published_title, exact: true }).count(), 1);
  assert.equal(await section.locator('pre').innerText(), fixture.published_body);
  assert.equal(await section.getByText(fixture.draft_body, { exact: true }).count(), 0);
  await section.getByRole('link', { name: fixture.file_name, exact: true }).waitFor();
  await section.getByText('· 第 ' + fixture.file_seq + ' 版', { exact: true }).waitFor();
  checks.push('The shared conversation previews the exact published knowledge revision with its historical formal file version and withholds the newer private draft');

  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 }); await stable(page);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${theme} ${width} source preview overflow`);
      if (theme === 'light' && width === 390) await page.screenshot({ path: path.join(evidence, 'published-light-390.png'), fullPage: true });
      if (theme === 'dark' && width === 1440) await page.screenshot({ path: path.join(evidence, 'published-dark-1440.png'), fullPage: true });
    }
  }
  section = await preview(page, fixture.personal_conversation_id, 'post'); await section.locator('.conversation-source-preview').waitFor();
  assert.equal(await section.locator('pre').innerText(), fixture.draft_body); await section.getByRole('heading', { name: fixture.draft_title, exact: true }).waitFor();
  checks.push('Both themes and five widths fit an authorized source preview; the author’s separate personal conversation can explicitly read its own current draft without changing the shared view');

  const adminPage = administrator.page;
  const policySection = adminPage.locator('.classification-policy').filter({ has: adminPage.getByRole('heading', { name: '项目受限', exact: true }) });
  await policySection.getByRole('button', { name: '发布策略新修订', exact: true }).click();
  let dialog = adminPage.getByRole('dialog', { name: '发布项目受限策略', exact: true });
  await dialog.getByLabel('发布原因', { exact: true }).fill('保留未保存的策略说明，核对真实第二设备冲突。');
  let policies = await request(administrator.context, '/classification-policies'), p = policies.items.find(item => item.level === 'project_restricted');
  await request(administrator.context, '/classification-policies', 'POST', { level: p.level, expected_version: p.version, base_policy_id: p.id, ai_allowed: false, allowed_channels: p.allowed_channels, reason: 'A genuine second-device request withdraws synthetic AI admission.' });
  await dialog.getByRole('checkbox', { name: '允许秘书处理此分类下有权读取的资料', exact: true }).uncheck();
  await dialog.getByRole('button', { name: '确认发布策略', exact: true }).click();
  await dialog.getByText('策略或授权已变化。已保留当前输入，请关闭后核对最新修订再发布。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('发布原因', { exact: true }).inputValue(), '保留未保存的策略说明，核对真实第二设备冲突。');
  assert(await dialog.getByRole('button', { name: '确认发布策略', exact: true }).isDisabled());
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await adminPage.reload();
  section = await preview(page, fixture.conversation_id, 'post'); await section.getByRole('alert').waitFor(); assert.equal(await section.locator('pre').count(), 0);
  checks.push('A real second-device policy publication invalidates the stale editor without losing its reason; withdrawing AI admission prevents a fresh source read');

  await policySection.getByRole('button', { name: '发布策略新修订', exact: true }).click(); dialog = adminPage.getByRole('dialog', { name: '发布项目受限策略', exact: true });
  await dialog.getByRole('checkbox', { name: '允许秘书处理此分类下有权读取的资料', exact: true }).check();
  await dialog.getByLabel('发布原因', { exact: true }).fill('通过真实界面恢复此隔离样本的秘书处理许可。');
  await dialog.getByRole('button', { name: '确认发布策略', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await policySection.getByRole('button', { name: '查看策略历史', exact: true }).click(); dialog = adminPage.getByRole('dialog', { name: '项目受限策略历史', exact: true });
  await dialog.getByText('通过真实界面恢复此隔离样本的秘书处理许可。', { exact: true }).waitFor();
  await dialog.getByText('A genuine second-device request withdraws synthetic AI admission.', { exact: true }).waitFor();
  await dialog.getByRole('button', { name: '关闭历史', exact: true }).click();
  for (const theme of ['light', 'dark']) {
    await adminPage.getByLabel('外观偏好', { exact: true }).selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) { await adminPage.setViewportSize({ width, height: 1050 }); await stable(adminPage); assert(await adminPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${theme} ${width} policy overflow`); }
  }
  await adminPage.screenshot({ path: path.join(evidence, 'policies-dark-1440.png'), fullPage: true });
  await adminPage.setViewportSize({ width: 390, height: 1050 }); await policySection.getByRole('button', { name: '发布策略新修订', exact: true }).click(); dialog = adminPage.getByRole('dialog', { name: '发布项目受限策略', exact: true });
  assert(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)); await stable(adminPage); await adminPage.screenshot({ path: path.join(evidence, 'policy-form-dark-390.png'), fullPage: false }); await dialog.getByRole('button', { name: '取消', exact: true }).click();
  checks.push('The real policy form publishes with current MFA, exposes both immutable history entries, and fits both themes and five widths including the 390px dialog');

  section = await preview(page, fixture.conversation_id, 'post'); await section.locator('.conversation-source-preview').waitFor();
  const c = await request(context, '/conversations/' + fixture.conversation_id), me = await request(reader.context, '/me');
  await request(reader.context, '/conversations/' + c.id + '/members', 'POST', { expected_version: c.version, user_id: me.user.id, action: 'remove', reason: 'Real member exits the isolated shared conversation.' });
  const responsePromise = page.waitForResponse(response => response.url().includes('/context-sources/post/') && response.request().method() === 'GET');
  await section.getByRole('button', { name: '核对并预览', exact: true }).click(); assert.equal((await responsePromise).status(), 409); await section.getByRole('alert').waitFor(); assert.equal(await section.locator('pre').count(), 0);
  await page.reload(); section = await preview(page, fixture.conversation_id, 'post'); await section.locator('.conversation-source-preview').waitFor(); assert.equal(await section.locator('pre').innerText(), fixture.published_body);
  checks.push('A real member exit advances the security epoch; an old page gets 409 and clears prior text, and a reload rebuilds an authorized source view');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  let closed = true; for (const context of contexts) { try { await context.close(); } catch { closed = false; } }
  if (browser) { try { await browser.close(); } catch { closed = false; } }
  const after = await digest(); const result = { passed: passed && closed && before === after, started_at: started, completed_at: new Date().toISOString(), real_api: true, isolated_native_postgresql: true, controlled_file_provider: true, genuine_web_sessions_and_mfa: true, synthetic_company_policy_and_data: true, actual_model_or_cloud: false, source_files: sourceFiles, source_sha256: before, source_unchanged: before === after, checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: closed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2)); if (!result.passed) process.exitCode = 1;
}
