// Actual Web/API project changes and budgets checks against an owned disposable native fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.origin, 'http://localhost:5189');
assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1'); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/project-changes'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), ...await files('mobile/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_project_changes.mjs', 'scripts/run_spend_browser.py'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map();
let browser, passed = false, alice, bob;
const cleanupErrors = [];
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function code(account) { const previous = lastSteps.get(account.login) ?? account.last_step; if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 250)); const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); return otp(account.mfa_secret, step); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(20000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) { await page.getByLabel('六位验证码').fill(await code(account)); await page.getByRole('button', { name: '验证并继续', exact: true }).click(); }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { context, page };
}
async function response(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const value = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  return { status: value.status(), value: await value.json() };
}
async function request(context, route, method = 'GET', body) { const result = await response(context, route, method, body); assert(result.status < 400, route + ' returned ' + result.status + '/' + result.value.error?.code + '/' + JSON.stringify(result.value.error?.details)); return result.value.data; }
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity));
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.sidebar'), narrow = matchMedia('(max-width: 899px)').matches;
    return sidebar && (narrow ? sidebar.inert && sidebar.getAttribute('aria-hidden') === 'true' : !sidebar.inert && sidebar.getAttribute('aria-hidden') !== 'true' && getComputedStyle(sidebar).visibility === 'visible' && getComputedStyle(sidebar).transform === 'none');
  });
}
assert.equal(fixture.project_change_fixture, true); assert(fixture.administrator?.mfa_secret);
async function modalFocus(page) {
  await page.waitForFunction(() => { const top = [...document.querySelectorAll('dialog[open]')].at(-1); return !top || top.contains(document.activeElement); });
}
async function commit(dialog, label) { await dialog.getByRole('button', { name: label, exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function close(dialog) { await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function layout(page, name, dialog = null) {
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme, { force: !!dialog });
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + width + ' overflows');
      const scope = dialog ?? page.locator('#main-content');
      const undersized = await scope.locator('button:visible,a:visible,select:visible,input:visible').evaluateAll(nodes => nodes.flatMap(node => { const rect = node.getBoundingClientRect(); return rect.width < 23.5 || rect.height < 23.5 ? [{ tag: node.tagName, text: node.textContent?.trim().slice(0, 100), width: rect.width, height: rect.height }] : []; }));
      assert.deepEqual(undersized, [], name + ' has undersized controls');
      if (dialog) {
        assert.equal(await dialog.evaluate(node => node.scrollWidth > node.clientWidth + 1), false, name + ' dialog content overflows');
        const bounds = await dialog.boundingBox(); assert(bounds && bounds.x >= -1 && bounds.x + bounds.width <= width + 1, name + ' dialog exceeds viewport');
        await modalFocus(page);
      }
      if ((theme === 'light' && width === 1440) || (theme === 'dark' && width === 390)) {
        await page.evaluate(() => { window.scrollTo({ top: 0, behavior: 'instant' }); for (const node of document.querySelectorAll('dialog[open]')) node.scrollTop = 0; });
        await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), fullPage: !dialog, animations: 'disabled' });
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light', { force: !!dialog });
  layouts.push({ name, themes: ['light', 'dark'], widths: [320, 390, 768, 1024, 1440], passed: true });
}
const layouts = [];

async function changePage(page, id, name) {
  await page.goto(fixture.origin + '/changes/' + id);
  await page.getByRole('heading', { level: 1, name, exact: true }).waitFor();
  await page.getByRole('heading', { name: '固定变更修订', exact: true }).waitFor();
}
async function selectFile(page, dialog, seq) {
  await dialog.getByRole('button', { name: '选择准确文件版本', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择项目变更依据版本', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + seq + ' ·' }).click();
  await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
}
async function approveThroughUI(id, name) {
  await changePage(alice.page, id, name);
  await alice.page.getByRole('button', { name: '准备送审申请', exact: true }).click();
  let dialog = alice.page.getByRole('dialog', { name: '准备项目变更申请', exact: true });
  await dialog.getByLabel('准备申请原因', { exact: true }).fill('人工固定本次完整提案与项目预算基线');
  await commit(dialog, '保存申请草稿');
  await alice.page.waitForURL(/\/requests\/[^/]+$/);
  const qid = new URL(alice.page.url()).pathname.split('/').at(-1);
  await alice.page.getByRole('button', { name: '核对并送审', exact: true }).click();
  dialog = alice.page.getByRole('dialog', { name: '核对项目变更送审预览', exact: true });
  await dialog.getByRole('heading', { name: '固定项目基线', exact: true }).waitFor();
  await dialog.getByLabel('送审依据', { exact: true }).fill('核对原值、完整提案及固定版本后正式送审');
  await commit(dialog, '确认以此内容送审');
  assert.equal(await alice.page.getByRole('button', { name: '处理当前步骤', exact: true }).count(), 0);
  await bob.page.goto(fixture.origin + '/requests/' + qid);
  await bob.page.getByRole('button', { name: '处理当前步骤', exact: true }).waitFor();
  await bob.page.getByRole('heading', { name: '原币预算对照', exact: true }).waitFor();
  await bob.page.getByRole('button', { name: '处理当前步骤', exact: true }).click();
  dialog = bob.page.getByRole('dialog', { name: '处理当前审批步骤', exact: true });
  await dialog.getByLabel('决定原因', { exact: true }).fill('独立核对完整范围、日期、预算及固定文件');
  await commit(dialog, '确认同意当前修订');
  const q = await request(alice.context, '/requests/' + qid); assert.equal(q.state, 'approved');
  return q;
}
async function implementationDialog(page) {
  await page.getByRole('button', { name: '实施已批准变更', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '实施已批准项目变更', exact: true });
  assert.equal(await dialog.getByLabel('我已核对本次项目范围和日期调整。', { exact: true }).isChecked(), false);
  assert.equal(await dialog.getByLabel('我已核对本次预算调整及其影响。', { exact: true }).isChecked(), false);
  assert.equal(await dialog.getByRole('button', { name: '确认实施准确批准变更', exact: true }).isDisabled(), true);
  return dialog;
}
async function confirmImplementation(dialog, reason) {
  await dialog.getByLabel('我已核对本次项目范围和日期调整。', { exact: true }).check();
  await dialog.getByLabel('我已核对本次预算调整及其影响。', { exact: true }).check();
  await dialog.getByLabel('明确实施原因', { exact: true }).fill(reason);
  await dialog.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
}
try {
  browser = await chromium.launch({ executablePath, headless: true, args: ['--disable-gpu', '--disable-background-networking'] });
  alice = await login(fixture.alice); bob = await login(fixture.bob); const admin = await login(fixture.administrator);
  const reviewer = await request(bob.context, '/me');
  await request(admin.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'synthetic-browser-project-change', name: '隔离项目变更独立核对制度', kind: 'change', category: 'synthetic-control', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立核对准确项目与预算', assigned_user_id: reviewer.user.id, requires_mfa: false }], reason: '隔离浏览器验证，不涉及真实公司审批制度' });
  let project = await request(alice.context, '/projects/' + fixture.project_id);
  if (project.state === 'draft') project = await request(alice.context, '/projects/' + fixture.project_id + '/transition', 'POST', { expected_version: project.version, action: 'activate', reason: '隔离进行中项目变更验证' });
  const beforeProject = await request(alice.context, '/projects/' + fixture.project_id);
  const name = '隔离项目范围与预算核对' + 'ContinuousExactProjectChangeBaseline'.repeat(5);
  await alice.page.goto(fixture.origin + '/projects/' + fixture.project_id + '?tab=changes');
  await alice.page.getByRole('heading', { name: '项目变更', exact: true }).waitFor();
  await alice.page.getByRole('button', { name: '提出项目变更', exact: true }).click();
  let dialog = alice.page.getByRole('dialog', { name: '提出项目变更', exact: true });
  await dialog.getByRole('button', { name: '核对并固定此基线', exact: true }).click();
  await dialog.getByLabel('变更标题', { exact: true }).fill(name);
  await dialog.getByLabel('业务类别', { exact: true }).fill('synthetic-control');
  await dialog.getByLabel('完整拟实施范围', { exact: true }).fill(beforeProject.scope + '\n明确增加可验收的隔离样本范围。');
  await dialog.getByLabel('范围影响说明', { exact: true }).fill('保留原交付范围并增加明确成果。');
  await dialog.getByLabel('时间影响说明', { exact: true }).fill('本次保留原有起止日期。');
  await dialog.getByLabel('预算修改方式', { exact: true }).selectOption('replace');
  await dialog.getByRole('button', { name: '添加预算明细', exact: true }).click();
  await dialog.getByRole('group', { name: '预算明细 1', exact: true }).getByLabel('类别', { exact: true }).fill('temporary-row');
  await dialog.getByRole('button', { name: '添加预算明细', exact: true }).click();
  await dialog.getByRole('group', { name: '预算明细 2', exact: true }).getByLabel('类别', { exact: true }).fill('retained-row');
  await dialog.getByRole('button', { name: '移除预算明细 1', exact: true }).click();
  assert.equal(await dialog.getByRole('group', { name: '预算明细 1', exact: true }).getByLabel('类别', { exact: true }).inputValue(), 'retained-row');
  await alice.page.waitForFunction(() => document.activeElement?.value === 'retained-row');
  await dialog.getByRole('button', { name: '移除预算明细 1', exact: true }).click();
  await alice.page.waitForFunction(() => document.activeElement?.textContent === '添加预算明细');
  await dialog.getByRole('button', { name: '添加预算明细', exact: true }).click();
  const line = dialog.getByRole('group', { name: '预算明细 1', exact: true });
  await line.getByLabel('类别', { exact: true }).fill('equipment');
  await line.getByLabel('期间开始', { exact: true }).fill('2026-10-01');
  await line.getByLabel('期间结束', { exact: true }).fill('2026-10-31');
  await line.getByLabel('原币金额', { exact: true }).fill('100');
  await selectFile(alice.page, dialog, fixture.file_seq);
  await dialog.getByLabel('修订原因', { exact: true }).fill('固定范围、预算期间与原正式文件版本');
  await layout(alice.page, 'change-create-dialog', dialog);
  await commit(dialog, '保存固定变更草稿'); await alice.page.waitForURL(/\/changes\/[^/]+$/);
  const id = new URL(alice.page.url()).pathname.split('/').at(-1);
  let head = await request(alice.context, '/changes/' + id);
  assert.equal(head.revision.attachments[0].version_id, fixture.file_version_id);
  assert.equal(head.revision.approval_amount, '100.000000'); assert.equal(head.revision.budget_delta, '100.000000');
  assert.equal((await request(alice.context, '/projects/' + fixture.project_id)).version, beforeProject.version);
  checks.push({ name: 'Actual Web creates exact project and dated-budget proposal, fixes the old formal file version, and leaves the live project unchanged', passed: true });
  const approved = await approveThroughUI(id, name);
  assert.equal(approved.payload.project_change_budget_delta, '100.000000');
  assert.equal((await request(alice.context, '/projects/' + fixture.project_id + '/budgets?state=approved')).items.length, 0);
  await layout(bob.page, 'change-approved-request');
  checks.push({ name: 'Separate human reviewer uses complete fixed Web approval snapshots; approval alone does not create an active budget', passed: true });
  await changePage(alice.page, id, name); dialog = await implementationDialog(alice.page);
  await confirmImplementation(dialog, '人工核对准确修订后明确实施初始预算与范围');
  await layout(alice.page, 'change-implementation-dialog', dialog);
  await commit(dialog, '确认实施准确批准变更');
  await alice.page.getByRole('heading', { name: '明确实施记录', exact: true }).waitFor();
  head = await request(alice.context, '/changes/' + id); assert.equal(head.state, 'implemented'); assert.equal(head.implementation.before_project_version, beforeProject.version);
  assert.equal((await request(alice.context, '/projects/' + fixture.project_id)).scope, head.revision.content.scope);
  await layout(alice.page, 'change-implemented-detail');
  checks.push({ name: 'Two initially unchecked human declarations plus actual current MFA gate explicit implementation and preserve exact project-version evidence', passed: true });
  await alice.page.goto(fixture.origin + '/projects/' + fixture.project_id + '?tab=budgets');
  await alice.page.getByRole('heading', { name: '项目预算', exact: true }).waitFor();
  await alice.page.getByRole('heading', { name: /HKD · 预算修订/ }).waitFor();
  await layout(alice.page, 'change-active-budget');
  const baseline = await request(alice.context, '/projects/' + fixture.project_id + '/change-basis?currency=HKD');
  const content = { ...head.revision.content, title: '隔离预算同额调整与完整修订历史', scope: baseline.scope + '\n第二次独立提案。', budget_lines: [{ category: 'equipment', period_start: '2026-10-01', period_end: '2026-10-31', amount: '60' }, { category: 'service', period_start: '2026-10-01', period_end: '2026-10-31', amount: '40' }] };
  const receipt = await request(alice.context, '/projects/' + fixture.project_id + '/changes', 'POST', { content, base_project_version: baseline.project_version, base_budget_id: baseline.budget_id, reason: '隔离相抵金额完整提案' });
  const second = receipt.id, versions = await request(alice.context, '/files/' + fixture.file_id + '/versions'), newer = versions.items.find(version => version.id !== fixture.file_version_id); assert(newer);
  await changePage(alice.page, second, content.title); await alice.page.getByRole('button', { name: '修订项目变更', exact: true }).click();
  dialog = alice.page.getByRole('dialog', { name: '修订项目变更', exact: true });
  await dialog.getByRole('button', { name: '核对并固定此基线', exact: true }).click();
  await dialog.getByLabel('范围影响说明', { exact: true }).fill('人工修改明确说明，并固定另一正式版本。');
  await dialog.getByRole('button', { name: '移除固定文件 ' + fixture.file_name, exact: true }).click();
  await selectFile(alice.page, dialog, newer.seq);
  await dialog.getByLabel('修订原因', { exact: true }).fill('明确保存第二次修订及替换资料');
  await commit(dialog, '保存固定变更草稿');
  head = await request(alice.context, '/changes/' + second); assert.equal(head.revision.seq, 2); assert.equal(head.revision.budget_delta, '0.000000'); assert.equal(head.revision.approval_amount, '40.000000');
  await alice.page.getByRole('button', { name: '修订 1', exact: true }).click();
  await alice.page.getByRole('heading', { name: '历史固定修订 1', exact: true }).waitFor();
  assert.equal((await request(alice.context, '/changes/' + second + '/revisions/' + head.revision.base_revision_id)).attachments[0].version_id, fixture.file_version_id);
  await alice.page.getByRole('button', { name: '当前修订', exact: true }).click();
  await layout(alice.page, 'change-net-zero-increase');
  checks.push({ name: 'Actual Web revision retains complete immutable history and exact old/new files; net-zero budget still exposes positive approval amount', passed: true });
  await approveThroughUI(second, content.title); await changePage(alice.page, second, content.title);
  dialog = await implementationDialog(alice.page); const retained = '冲突后必须保留的人工实施说明'; await confirmImplementation(dialog, retained);
  project = await request(alice.context, '/projects/' + fixture.project_id);
  await request(alice.context, '/projects/' + fixture.project_id, 'PATCH', { expected_version: project.version, name: project.name + ' · 新基线' });
  await dialog.getByRole('button', { name: '确认实施准确批准变更', exact: true }).click();
  await dialog.getByText('固定基线或授权已变化，当前输入保留。请关闭后核对最新事实；不能重复提交旧依据。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('明确实施原因', { exact: true }).inputValue(), retained);
  assert.equal(await dialog.getByLabel('我已核对本次预算调整及其影响。', { exact: true }).isChecked(), true);
  assert.equal(await dialog.getByRole('button', { name: '确认实施准确批准变更', exact: true }).isDisabled(), true);
  assert.equal((await request(alice.context, '/changes/' + second)).can_implement, false);
  assert.equal((await request(alice.context, '/projects/' + fixture.project_id + '/change-basis?currency=HKD')).budget_total, '100.000000');
  await close(dialog);
  checks.push({ name: 'A genuinely changed live project causes a real 409, preserves human input and declarations, prevents retry, and leaves the active budget unchanged', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }

finally {
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: ['15', '17'], production_deliveries: false, checks, layouts, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
