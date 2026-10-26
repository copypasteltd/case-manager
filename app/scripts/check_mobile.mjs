// A real Taro H5 UI/API check. This is not a mini-program or Android runtime.
import { chromium } from 'playwright';
import { projectChecks } from './mobile_project_checks.mjs';
const projectsMode = process.argv.includes('--projects');
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.origin, 'http://localhost:5189');
assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1'); assert.equal(fixture.isolated_database, true); assert.equal(fixture.controlled_provider, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, projectsMode ? 'docs/quality/evidence/mobile-projects' : 'docs/quality/evidence/mobile-browser'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TEMP = temporary; process.env.TMP = temporary;
async function sourceFiles(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await sourceFiles(relative)); else result.push(relative); } return result; }
const apiSources = (await sourceFiles('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources = [...new Set([...await sourceFiles('mobile'), ...apiSources, 'package.json', 'package-lock.json', 'scripts/check_mobile.mjs', 'scripts/mobile_project_checks.mjs', 'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), started = new Date().toISOString(), contexts = [], checks = [], layouts = [], errors = [], serverErrors = [], lastSteps = new Map(); let browser, passed = false;
const networkFailures = [];
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000); page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  page.on('requestfailed', request => networkFailures.push({ method: request.method(), path: new URL(request.url()).pathname, error: request.failure()?.errorText }));
  await page.goto(fixture.origin); await page.locator('input[placeholder="你的公司账号"]').fill(account.login); await page.locator('input[type="password"]').fill(account.password);
  const [loginResponse] = await Promise.all([page.waitForResponse(response => response.url().endsWith('/v1/auth/login') && response.request().method() === 'POST'), page.getByRole('button', { name: '登录工作空间 ↗', exact: true }).click()]); let result = await loginResponse.json();
  if (account.mfa_secret) {
    await page.locator('input[maxlength="6"]').waitFor(); const previous = lastSteps.get(account.login) ?? account.last_step;
    if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 200));
    const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step);
    await page.locator('input[maxlength="6"]').fill(otp(account.mfa_secret, step));
    const [verified] = await Promise.all([page.waitForResponse(response => response.url().endsWith('/v1/auth/mfa/verify') && response.request().method() === 'POST'), page.getByRole('button', { name: '验证并继续 ↗', exact: true }).click()]); result = await verified.json();
  }
  assert.equal(result.data?.step, 'authenticated'); await page.locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  return { page, context, token: result.data.access_token };
}
async function request(token, route, method = 'GET', body) { const response = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'mobile-browser:' + crypto.randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); const result = await response.json(); assert.ok(response.status < 400, route + ' returned ' + response.status + ' / ' + (result.error?.code ?? '')); return result.data; }
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity));
}
// Taro retains prior route trees during animations. A generic :visible also
// matches a page translated off-screen, so wait for its actual settled state.
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
async function restoreInput(page, title, reason) {
  let commands = 0;
  const count = request => { if (request.method() === 'POST' && /\/transition$|\/decisions$/.test(new URL(request.url()).pathname)) commands++; };
  page.on('request', count);
  try {
    await screen(page).getByRole('button', { name: '我的', exact: true }).click();
    await screen(page).locator('.cp-title').filter({ hasText: '我的工作空间' }).waitFor();
    await screen(page).getByRole('button', { name: '返回上一页', exact: true }).click();
    await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor();
    assert.equal(await screen(page).getByRole('textbox', { name: '变更或决定原因', exact: true }).inputValue(), reason);
    await screen(page).getByText('已恢复此会话中尚未提交的文字输入。请再次核对当前资料；文件选择、核实声明和两步验证需要重新操作。', { exact: true }).waitFor();
    assert.equal(commands, 0, 'Returning to a draft must not submit it');
    assert.equal(await page.evaluate(() => Object.keys(localStorage).some(key => /draft|reason|decision|access|refresh|credential/i.test(key))), false);
  } finally { page.off('request', count); }
}
async function layout(page, label) {
  const originalTitle = await screen(page).locator('.cp-title').textContent();
  for (const [theme, title] of [['light', '浅色'], ['dark', '深色']]) {
    await screen(page).getByRole('button', { name: '我的', exact: true }).click();
    await screen(page).locator('.cp-title').filter({ hasText: '我的工作空间' }).waitFor();
    await screen(page).getByRole('button', { name: title, exact: true }).click(); await screen(page).getByRole('button', { name: '返回上一页', exact: true }).click();
    await screen(page).locator('.cp-title').filter({ hasText: originalTitle }).waitFor();
    await page.locator('.cp-page.cp-theme-' + theme + ':visible').first().waitFor(); await stable(page);
    for (const width of [320, 390, 768]) {
      await page.setViewportSize({ width, height: 900 }); await stable(page);
      await page.waitForFunction(title => {
        const heading = [...document.querySelectorAll('.cp-title')].find(node => node.textContent === title), panel = heading?.closest('.cp-page');
        if (!panel) return false; const rect = panel.getBoundingClientRect(); return Math.abs(rect.x) <= 1 && Math.abs(rect.width - innerWidth) <= 1;
      }, originalTitle);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1); assert.equal(overflow, false, label + '/' + theme + '/' + width + ' overflows');
      const small = await screen(page).locator('.cp-button:visible,.cp-link:visible').evaluateAll(nodes => nodes.some(node => node.getBoundingClientRect().height < 43.5)); assert.equal(small, false, label + ' has a small touch target');
      const faded = await screen(page).locator('.cp-button[aria-disabled="false"],.cp-link[aria-disabled="false"]').evaluateAll(nodes => nodes.filter(node => Number(getComputedStyle(node).opacity) < 1).map(node => ({ label: node.textContent, classes: node.className, disabled: node.getAttribute('disabled'), opacity: getComputedStyle(node).opacity })));
      assert.deepEqual(faded, [], 'Enabled controls must not inherit disabled opacity');
      const appearance = await screen(page).evaluate(panel => {
        const brand = panel.querySelector('.cp-brand-name'), style = getComputedStyle(brand);
        return { font: getComputedStyle(panel).fontFamily, brandLines: brand.getBoundingClientRect().height / parseFloat(style.fontSize), textareas: [...panel.querySelectorAll('.cp-textarea textarea')].map(node => ({ color: getComputedStyle(node).color, parentColor: getComputedStyle(node.parentElement).color, background: getComputedStyle(node).backgroundColor, fits: node.getBoundingClientRect().bottom <= node.parentElement.getBoundingClientRect().bottom + 1 })) };
      });
      assert.match(appearance.font, /Manrope/); assert.ok(appearance.brandLines < 1.8, 'Brand name wraps');
      for (const textarea of appearance.textareas) { assert.equal(textarea.color, textarea.parentColor); assert.equal(textarea.background, 'rgba(0, 0, 0, 0)'); assert.equal(textarea.fits, true); }
      if (width === 390) await page.screenshot({ path: path.join(evidence, label + '-' + theme + '-390.png'), fullPage: true });
    }
  }
  await page.setViewportSize({ width: 390, height: 844 });
  layouts.push({ page: label, themes: ['light', 'dark'], widths: [320, 390, 768], configurations: 6, passed: true });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), { page } = alice;
  assert.equal(await page.evaluate(() => Object.keys(localStorage).some(key => /access|refresh|session|credential|password|challenge/i.test(key))), false);
  checks.push({ name: 'Real mobile-channel login and MFA; no credentials in local storage', passed: true });
  assert.equal((await alice.context.cookies()).some(cookie => /^cp_(access|refresh|csrf)$/.test(cookie.name)), false);
  await page.screenshot({ path: path.join(evidence, 'home-light-390.png'), fullPage: true });
  const project = await request(alice.token, '/projects/' + fixture.project_id); assert.ok(project.name);
  await page.getByRole('button', { name: /项目 查看项目范围/ }).click(); await page.locator('.cp-row-title').filter({ hasText: project.name }).first().click(); await page.locator('.cp-title').filter({ hasText: project.name }).waitFor();
  await page.screenshot({ path: path.join(evidence, 'project-light-390.png'), fullPage: true });
  checks.push({ name: 'Real project list and scoped detail', passed: true });
  const me = await request(alice.token, '/me');
  const task = await request(alice.token, '/projects/' + project.id + '/tasks', 'POST', { title: 'Synthetic mobile task', body: 'Explicit isolated API seed; verify the real mobile transition.', assignee_id: me.user.id, priority: 'normal', review_required: false });
  await page.getByRole('button', { name: /项目任务/ }).click(); await screen(page).locator('.cp-row-title').filter({ hasText: task.title }).click(); await screen(page).locator('.cp-title').filter({ hasText: task.title }).waitFor();
  await screen(page).getByRole('textbox', { name: '变更或决定原因', exact: true }).fill('Synthetic mobile human task start');
  await restoreInput(page, task.title + '.', 'Synthetic mobile human task start');
  const [moved] = await Promise.all([page.waitForResponse(response => response.url().endsWith('/v1/tasks/' + task.id + '/transition') && response.request().method() === 'POST'), page.getByRole('button', { name: '确认开始处理', exact: true }).click()]); assert.equal(moved.status(), 200);
  await screen(page).locator('.cp-state').filter({ hasText: '进行中' }).waitFor(); assert.equal((await request(alice.token, '/tasks/' + task.id)).state, 'in_progress');
  assert.equal(await screen(page).getByRole('textbox', { name: '变更或决定原因', exact: true }).inputValue(), '', 'A committed task must not restore the prior version reason');
  await layout(page, 'task');
  checks.push({ name: 'API-seeded task changed through real versioned mobile form; both themes at 320/390/768 and touch targets', passed: true });
  await page.goBack(); await page.goBack(); await screen(page).locator('.cp-title').filter({ hasText: project.name }).waitFor();
  await page.getByRole('button', { name: /项目文件/ }).click(); await screen(page).locator('.cp-row-title').filter({ hasText: fixture.file_name }).first().click(); await screen(page).locator('.cp-title').filter({ hasText: fixture.file_name }).waitFor();
  await screen(page).getByText('正式版本', { exact: true }).waitFor(); await screen(page).getByText('SHA-256', { exact: true }).first().waitFor();
  await layout(page, 'file');
  checks.push({ name: 'Real controlled-provider file versions, scan and replica facts; two themes and three widths', passed: true });
  const contract = await request(alice.token, '/contracts/' + fixture.contract_id);
  let approval = await request(alice.token, '/requests', 'POST', { kind: 'contract', business_id: contract.id, expected_business_version: contract.version, business_revision_id: contract.current_revision_id, business_content_hash: contract.content_hash, reason: 'Explicit isolated API seed for mobile review' });
  const preview = await request(alice.token, '/requests/' + approval.id + '/submission-preview');
  approval = await request(alice.token, '/requests/' + approval.id + '/submit', 'POST', { expected_version: preview.request_version, revision_id: preview.revision_id, content_hash: preview.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: 'Explicit isolated API submission before mobile decision' });
  async function openApproval() { const account = await login(fixture.bob); await account.page.getByRole('button', { name: /待我审批 核对资料/ }).click(); await screen(account.page).locator('.cp-row-title').filter({ hasText: approval.title }).first().click(); await screen(account.page).getByText('本次申请的固定内容', { exact: true }).waitFor(); return account; }
  const bob = await openApproval(), other = await openApproval();
  for (const label of ['收付方向', '合同类型', '业务类别', '资料分类', '其中税额', '合同正文依据', '收付计划']) await screen(bob.page).getByText(label, { exact: true }).waitFor();
  await bob.page.locator('textarea:visible').fill('Synthetic mobile independent approval'); await other.page.locator('textarea:visible').fill('Preserved second device reason');
  const body = approval.payload.attachments.find(file => file.version_id === approval.payload.content.draft_file_version_id); assert.ok(body);
  const [pinned] = await Promise.all([bob.page.waitForResponse(response => response.url().endsWith('/v1/files/' + body.file_id + '/versions/' + body.version_id) && response.request().method() === 'GET'), screen(bob.page).getByRole('button', { name: '查看固定正文资料 ↗', exact: true }).click()]);
  assert.equal(pinned.status(), 200); assert.equal((await pinned.json()).data.id, body.version_id);
  await screen(bob.page).getByText('本次引用的固定版本', { exact: true }).waitFor();
  await screen(bob.page).getByText(body.sha256, { exact: true }).waitFor();
  await bob.page.goBack(); await screen(bob.page).locator('.cp-title').filter({ hasText: approval.title }).waitFor();
  assert.equal(await screen(bob.page).locator('textarea').inputValue(), 'Synthetic mobile independent approval');
  checks.push({ name: 'Complete fixed contract facts and exact version metadata navigation; returning preserves review input without substituting current content', passed: true });
  await restoreInput(bob.page, approval.title + '.', 'Synthetic mobile independent approval');
  checks.push({ name: 'Task and approval input survive real page hide/show only in memory, without automatic submission; committed task input is cleared', passed: true });
  const [decided] = await Promise.all([bob.page.waitForResponse(response => response.url().includes('/approval-steps/') && response.request().method() === 'POST'), bob.page.getByRole('button', { name: '确认同意当前修订', exact: true }).click()]); assert.equal(decided.status(), 200);
  await screen(bob.page).locator('.cp-state').filter({ hasText: '已批准' }).first().waitFor(); assert.equal((await request(bob.token, '/requests/' + approval.id)).state, 'approved');
  const [conflict] = await Promise.all([other.page.waitForResponse(response => response.url().includes('/approval-steps/') && response.request().method() === 'POST'), other.page.getByRole('button', { name: '确认同意当前修订', exact: true }).click()]); assert.ok([409, 412].includes(conflict.status())); assert.equal(await other.page.locator('textarea:visible').inputValue(), 'Preserved second device reason'); assert.equal(await other.page.getByRole('button', { name: '确认同意当前修订', exact: true }).isDisabled(), true);
  await layout(bob.page, 'approved-request');
  checks.push({ name: 'API-prepared fixed request reviewed through mobile UI; independent approval, two-device conflict preserves reason and stops resubmission', passed: true });
  await page.reload(); await screen(page).locator('.cp-title').filter({ hasText: '回到工作空间' }).waitFor(); assert.equal(await page.getByText(fixture.file_name, { exact: true }).count(), 0);
  checks.push({ name: 'Full review-page reload discards memory credentials and old business content', passed: true });
  if (projectsMode) await projectChecks({ fixture, login, request, screen, stable, layout, checks, otp, lastSteps, accounts: { bob, other } });
  assert.equal(errors.length, 0, 'JavaScript runtime errors: ' + errors.join('; ')); assert.equal(serverErrors.length, 0);
  passed = true;
} catch (error) {
  for (const context of contexts) { const page = context.pages().at(-1); if (page) { try {
    await page.screenshot({ path: path.join(app, '.local/browser-mobile-failure.png') });
    console.error(JSON.stringify(await page.evaluate(() => ({ controls: [...document.querySelectorAll('.taro_page_stationed:not(.taro_page_shade) input,.taro_page_stationed:not(.taro_page_shade) textarea')].map(node => ({tag:node.tagName, type:node.type, label:node.getAttribute('aria-label'), role:node.getAttribute('role'), ariaHidden:node.closest('[aria-hidden]')?.getAttribute('aria-hidden'), parentLabel:node.parentElement.getAttribute('aria-label'), nativeKeys:Object.keys(node.parentElement.nativeProps ?? {})})), route: location.pathname + location.hash, pages: [...document.querySelectorAll('.taro_page')].map(node => { const rect = node.getBoundingClientRect(); return { classes: node.className, display: getComputedStyle(node).display, transform: getComputedStyle(node).transform, x: rect.x, width: rect.width, titles: [...node.querySelectorAll('.cp-title')].map(title => title.textContent) }; }) }))));
  } catch {} } }
  console.error(JSON.stringify({ network_failures: networkFailures, javascript_errors: errors })); throw error;
} finally {
  await Promise.allSettled(contexts.map(context => context.close())); let closed = !browser;
  if (browser) { try { await browser.close(); closed = !browser.isConnected() && contexts.every(context => context.pages().length === 0); } catch { passed = false; } }
  const after = await digest();
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ started_at: started, completed_at: new Date().toISOString(), source_sha256: before, source_files: sources.length, source_unchanged_during_run: before === after, checks, layouts, javascript_errors: errors, server_errors: serverErrors, network_failures: networkFailures, all_browser_contexts_closed: closed, all_browser_tabs_closed: closed, environment: 'Native Edge, Taro H5 local review build and disposable native PostgreSQL', actual_mini_program_runtime_invoked: false, actual_android_runtime_invoked: false, actual_cloud_invoked: false, scope: 'Real API and Taro common UI; H5 is only a local review target, not platform acceptance', passed: passed && before === after && closed }, null, 2) + '\n');
  console.log(JSON.stringify({ checks: checks.length, passed, all_browser_contexts_closed: closed }));
}
