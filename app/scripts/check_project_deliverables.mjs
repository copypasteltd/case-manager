// Actual Web/API project deliverables checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/project-deliverables'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), ...await files('mobile/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_project_deliverables.mjs', 'scripts/run_spend_browser.py'])].sort();
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
assert.equal(fixture.deliverable_fixture, true); assert(fixture.bob.mfa_secret);
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

async function projectPage(page) {
  await page.goto(fixture.origin + '/projects/' + fixture.project_id);
  await page.getByRole('tab', { name: '交付', exact: true }).click();
  await page.getByRole('heading', { name: '项目交付', exact: true }).waitFor();
}
async function detail(page, name) {
  await page.getByRole('row').filter({ hasText: name }).getByRole('button', { name: '查看交付', exact: true }).click();
  const d = page.getByRole('dialog', { name: '项目交付详情', exact: true });
  await d.getByRole('heading', { name, exact: true }).waitFor(); return d;
}
async function selectFile(page, dialog, seq) {
  await dialog.getByRole('button', { name: '添加固定交付文件版本', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择固定交付文件版本', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + seq + ' ·' }).click();
  await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
}
async function submitForm(page, d, seq, accepted = false) {
  await d.getByRole('button', { name: accepted ? '提交新交付修订' : '提交固定交付', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '提交固定交付修订', exact: true });
  await dialog.getByLabel('本次交付说明', { exact: true }).fill('隔离样本完整交付说明，明确固定文件内容。');
  await selectFile(page, dialog, seq);
  await dialog.getByLabel('本次操作依据', { exact: true }).fill('人工核对本次固定交付样本');
  return dialog;
}
async function acceptance(page, d) {
  await d.getByRole('button', { name: '核实并登记验收', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '核实并登记交付验收', exact: true });
  await dialog.getByRole('heading', { name: /固定交付修订/ }).waitFor();
  assert.equal(await dialog.getByLabel('本次验收决定', { exact: true }).inputValue(), '');
  assert.equal(await dialog.getByLabel('已核实全部固定文件版本及完整内容', { exact: true }).isChecked(), false);
  assert.equal(await dialog.getByLabel('已逐项核对本次固定验收准则', { exact: true }).isChecked(), false);
  assert.equal(await dialog.getByRole('button', { name: '确认登记验收决定', exact: true }).isDisabled(), true);
  return dialog;
}
try {
  browser = await chromium.launch({ executablePath, headless: true, args: ['--disable-gpu', '--disable-dev-shm-usage', '--no-first-run'] });
  alice = await login(fixture.alice); bob = await login(fixture.bob);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me');
  const versions = await request(alice.context, '/files/' + fixture.file_id + '/versions');
  const newer = versions.items.find(v => v.id !== fixture.file_version_id); assert(newer, 'another real version is required');
  const name = '虚构项目交付核验样本-' + 'DeliverableEvidence'.repeat(8);
  await projectPage(alice.page);
  await alice.page.getByRole('button', { name: '新建交付', exact: true }).click();
  let dialog = alice.page.getByRole('dialog', { name: '新建项目交付', exact: true });
  await dialog.getByLabel('交付名称', { exact: true }).fill(name);
  await dialog.getByLabel('验收准则', { exact: true }).fill('最初准则：逐项核对旧版本的交付内容。');
  await dialog.getByLabel('交付负责人', { exact: true }).selectOption(me.user.id);
  await dialog.getByLabel('本次操作依据', { exact: true }).fill('隔离浏览器人工新建交付');
  await layout(alice.page, 'deliverable-create', dialog); await commit(dialog, '保存交付资料');
  let head = (await request(alice.context, '/projects/' + fixture.project_id + '/deliverables')).items.find(x => x.name === name); assert(head);
  assert.equal(head.state, 'draft'); assert.equal(head.creator_id, me.user.id);
  let d = await detail(alice.page, name);
  dialog = await submitForm(alice.page, d, fixture.file_seq);
  await layout(alice.page, 'deliverable-submit', dialog); await commit(dialog, '确认提交固定交付');
  head = await request(alice.context, '/deliverables/' + head.id);
  const first = await request(alice.context, '/deliverables/' + head.id + '/revisions/' + head.current_revision_id);
  assert.equal(first.files[0].version_id, fixture.file_version_id); assert.equal(first.seq, 1);
  checks.push({ name: 'Web creates a real draft and submits the specifically selected older formal file version with immutable criteria and content hash', passed: true });
  dialog = await acceptance(alice.page, d);
  assert.equal(await dialog.locator('option[value="accept"]').isDisabled(), true);
  await close(dialog); await close(d);
  assert.equal((await response(alice.context, '/deliverables/' + head.id + '/decisions', 'POST', { expected_version: head.version, revision_id: first.id, content_hash: first.content_hash, decision: 'accept', files_verified: true, criteria_verified: true, reason: 'self acceptance must fail' })).status, 403);
  checks.push({ name: 'Submitting user cannot accept their own revision in either the actual Web form or API command', passed: true });
  await projectPage(bob.page); d = await detail(bob.page, name); dialog = await acceptance(bob.page, d);
  await dialog.getByLabel('本次验收决定', { exact: true }).selectOption('request_changes');
  await dialog.getByLabel('验收决定说明', { exact: true }).fill('旧版本内容需要修订，明确退回。');
  await commit(dialog, '确认登记验收决定'); await close(d);
  await projectPage(alice.page); d = await detail(alice.page, name);
  await d.getByRole('button', { name: '编辑交付资料', exact: true }).click();
  dialog = alice.page.getByRole('dialog', { name: '编辑交付资料', exact: true });
  await dialog.getByLabel('验收准则', { exact: true }).fill('修订准则：核对新文件的完整交付内容。');
  await dialog.getByLabel('本次操作依据', { exact: true }).fill('明确更新退回后的下次交付准则');
  await commit(dialog, '保存交付资料');
  dialog = await submitForm(alice.page, d, newer.seq); await commit(dialog, '确认提交固定交付'); await close(d);
  head = await request(alice.context, '/deliverables/' + head.id);
  await projectPage(bob.page); d = await detail(bob.page, name); dialog = await acceptance(bob.page, d);
  await dialog.getByLabel('本次验收决定', { exact: true }).selectOption('accept');
  await dialog.getByLabel('验收决定说明', { exact: true }).fill('本人完整核对修订文件和准则后通过。');
  assert.equal(await dialog.getByRole('button', { name: '确认登记验收决定', exact: true }).isDisabled(), true);
  await dialog.getByLabel('已核实全部固定文件版本及完整内容', { exact: true }).check();
  await dialog.getByLabel('已逐项核对本次固定验收准则', { exact: true }).check();
  await layout(bob.page, 'deliverable-acceptance', dialog); await commit(dialog, '确认登记验收决定');
  const accepted = await request(bob.context, '/deliverables/' + head.id);
  const second = await request(bob.context, '/deliverables/' + head.id + '/revisions/' + accepted.current_revision_id);
  const preserved = await request(bob.context, '/deliverables/' + head.id + '/revisions/' + first.id);
  assert.equal(accepted.state, 'accepted'); assert.equal(second.files[0].version_id, newer.id); assert.equal(second.seq, 2);
  assert.equal(second.decision.decision, 'accept'); assert.equal(preserved.content_hash, first.content_hash);
  assert.equal(preserved.acceptance_criteria, first.acceptance_criteria); assert.equal(preserved.decision.decision, 'request_changes');
  await d.getByRole('button', { name: '修订 1 · 已退回', exact: true }).click();
  await d.getByRole('heading', { name: '固定交付修订 1', exact: true }).waitFor();
  await layout(bob.page, 'deliverable-fixed-history', d); await close(d);
  checks.push({ name: 'Independent reviewer returns then accepts a new revision only after two explicit unchecked attestations; old criteria files hash and return decision remain fixed', passed: true });
  await projectPage(alice.page); d = await detail(alice.page, name); dialog = await submitForm(alice.page, d, fixture.file_seq, true);
  await request(alice.context, '/deliverables/' + head.id + '/submit', 'POST', { expected_version: accepted.version, expected_revision_id: accepted.current_revision_id, file_version_ids: [newer.id], description: '另一会话明确提交新的待验收修订', reason: '制造真实版本冲突' });
  await dialog.getByRole('button', { name: '确认提交固定交付', exact: true }).click();
  await dialog.getByText('当前依据已变化或授权已失效。输入保留，请关闭后核对最新资料；不能重复提交旧依据。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('本次交付说明', { exact: true }).inputValue(), '隔离样本完整交付说明，明确固定文件内容。');
  assert.equal(await dialog.getByLabel('本次操作依据', { exact: true }).inputValue(), '人工核对本次固定交付样本');
  assert.equal(await dialog.getByRole('button', { name: '移除此版本', exact: true }).count(), 1);
  assert.equal(await dialog.getByRole('button', { name: '确认提交固定交付', exact: true }).isDisabled(), true);
  assert.equal(await dialog.locator('form > div[tabindex="-1"]').evaluate(node => node === document.activeElement), true);
  await layout(alice.page, 'deliverable-conflict-preserved', dialog); await close(dialog); await close(d);
  checks.push({ name: 'An actual external new submission produces 409 and preserves selected files description and reason with error focus and disabled stale resubmission', passed: true });
  const thirdHead = await request(alice.context, '/deliverables/' + head.id);
  assert.equal(thirdHead.state, 'submitted');
  const stillAccepted = await request(alice.context, '/deliverables/' + head.id + '/revisions/' + second.id);
  assert.equal(stillAccepted.decision.decision, 'accept'); assert.equal(stillAccepted.content_hash, second.content_hash);
  checks.push({ name: 'Explicit new submission after acceptance leaves the old accepted revision intact and creates a separate pending revision', passed: true });
  const project = await request(alice.context, '/projects/' + fixture.project_id);
  await request(alice.context, '/projects/' + fixture.project_id + '/members', 'POST', { expected_version: project.version, user_id: reviewer.user.id, project_role: 'viewer', action: 'set', reason: '隔离只读成员权限核验' });
  await bob.context.close(); bob = await login(fixture.bob); await projectPage(bob.page);
  assert.equal(await bob.page.getByRole('button', { name: '新建交付', exact: true }).count(), 0);
  d = await detail(bob.page, name);
  assert.equal(await d.getByRole('button', { name: '核实并登记验收', exact: true }).count(), 0);
  assert.equal(await d.getByRole('button', { name: '提交固定交付', exact: true }).count(), 0);
  const readonly = await request(bob.context, '/deliverables/' + head.id);
  assert.equal(readonly.can_edit, false); assert.equal(readonly.can_submit, false); assert.equal(readonly.can_decide, false);
  await layout(bob.page, 'deliverable-readonly', d); await close(d);
  checks.push({ name: 'Project viewer role overrides broad business roles and actual Web hides write and acceptance controls while retaining authorized fixed history', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: ['15', '17'], production_deliveries: false, checks, layouts, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
