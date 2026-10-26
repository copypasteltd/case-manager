import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { recordViewport } from './repository_recording.mjs';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-closeout-browser');
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true); assert.ok(fixture.published_template_id);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) { h.update(file + '\0'); h.update(await readFile(path.join(app, file))); h.update('\0'); } return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], closedContexts = new Set(), checks = [], layouts = [], errors = [], serverErrors = [], cleanupErrors = [], recordings = [], screenshots = [];
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, label) => screen(page).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]');
let browser, mainPage, passed = false, failure;
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); }
async function navigate(page, url) { await page.evaluate(value => { window.location.hash = '#' + value; }, url); await stable(page); }
async function closeOwned(context) { if (!closedContexts.has(context)) { await context.close(); closedContexts.add(context); } }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 390, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ status: r.status(), path: new URL(r.url()).pathname }); });
  await page.goto(fixture.origin); await field(page, '内部账号').fill(account.login); await field(page, '密码').fill(account.password);
  const pending = page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST'); await button(page, '登录工作空间 ↗').click(); let result = await (await pending).json();
  if (account.mfa_secret) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
    const step = Math.floor(Date.now() / 30000); await screen(page).locator('input[maxlength="6"]').fill(otp(account.mfa_secret, step));
    const verified = page.waitForResponse(r => r.url().endsWith('/v1/auth/mfa/verify') && r.request().method() === 'POST'); await button(page, '验证并继续 ↗').click(); result = await (await verified).json(); account.last_step = step;
  }
  assert.equal(result.data?.step, 'authenticated'); await heading(page, '工作从这里开始'); return { page, context, token: result.data.access_token };
}
async function response(token, route, method = 'GET', body) {
  return fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'project-setup:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
}
async function request(token, route, method, body) { const r = await response(token, route, method, body), data = await r.json(); assert.ok(r.ok, `${method ?? 'GET'} ${route}: ${r.status} ${data.error?.code ?? ''}`); return data.data; }
async function submit(page, label, route, method, status = 200) {
  const refresh = status === 200 && (route.endsWith('/members') || label === '确认移交项目负责人') ? page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route.replace(/\/members$/, '') + '/members' && r.request().method() === 'GET') : undefined;
  const [r] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route && r.request().method() === method), button(page, label).click()]);
  assert.equal(r.status(), status, route); const value = (await r.json()).data;
  if (refresh) { assert.equal((await refresh).status(), 200); await stable(page); }
  return { body: r.request().postDataJSON(), value };
}
async function layout(page, name) {
  for (const width of [320, 390, 768]) {
    await page.setViewportSize({ width, height: 900 }); await stable(page);
    const value = await screen(page).evaluate(node => ({ width: node.getBoundingClientRect().width, overflow: document.documentElement.scrollWidth > innerWidth + 1, small: [...node.querySelectorAll('.cp-button,.cp-row')].filter(n => { const r = n.getBoundingClientRect(); return r.width && r.height < 43.5; }).map(n => n.textContent) }));
    assert.equal(value.overflow, false, name + ' horizontal overflow'); assert.ok(Math.abs(value.width - width) <= 1); assert.deepEqual(value.small, []); layouts.push({ page: name, width, passed: true });
  }
  await page.setViewportSize({ width: 390, height: 900 }); await stable(page); await page.screenshot({ path: path.join(evidence, name + '.png'), fullPage: true });
}


function recordCheck(value) { checks.push(value); console.log('Completed actual flow ' + checks.length + ': ' + value); }
async function freshProject(page, project) {
  await heading(page, '项目结项与归档'); await screen(page).getByText(new RegExp('^项目版本 ' + project.version + ' ·')).waitFor(); await stable(page);
}
async function openCloseout(page, project) {
  const url = '/pages/project/index?section=closeout&id=' + project.id;
  if (new URL(page.url()).hash === '#' + url) await button(page, '重新读取项目结项').click(); else await navigate(page, url);
  await freshProject(page, project);
}
async function transitionUI(page, project, label) {
  await select(page, '项目阶段动作', label); await field(page, '项目阶段原因').fill('合成验收：逐项核对原项目阶段与实际义务。');
  const result = await submit(page, '确认' + label, '/projects/' + project.id + '/transition', 'POST');
  await freshProject(page, result.value); return result.value;
}
async function transitionAPI(token, projectID, action) {
  const p = await request(token, '/projects/' + projectID);
  return request(token, '/projects/' + projectID + '/transition', 'POST', { expected_version: p.version, action, reason: 'Actual original lifecycle command for isolated review' });
}
async function notes(page, summary = '合成演示：已核对实际交付成果与项目范围。') {
  await field(page, '成果与交付总结').fill(summary); await field(page, '经验与复盘').fill('合成演示：在执行前明确验收标准，按原始资料核对闭环。');
  await field(page, '后续跟进').fill('合成演示：无遗留义务，新增工作应重开或另建项目。'); await field(page, '归档原因').fill('合成演示：人工确认完整业务与财务依据。');
}
async function attest(page) {
  await select(page, '业务闭环核实', '已核对任务、交付、风险、问题与变更');
  await select(page, '财务与义务核实', '已核对合同、费用、资金与订阅处置');
}
async function mfa(page, account) {
  while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
  const step = Math.floor(Date.now() / 30000); await field(page, '验证器六位验证码').fill(otp(account.mfa_secret, step));
  await submit(page, '确认本人操作', '/auth/mfa/step-up', 'POST'); account.last_step = step;
  await screen(page).getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  assert.equal(await field(page, '验证器六位验证码').count(), 0);
}
async function image(page, name, caption) {
  await stable(page); const filename = path.join(app, '../docs/media/screenshots/' + name + '.png');
  await page.screenshot({ path: filename, fullPage: false });
  screenshots.push({ file: 'docs/media/screenshots/' + name + '.png', caption, sha256: createHash('sha256').update(await readFile(filename)).digest('hex'), synthetic_data: true, real_device: false });
}
async function createProject(token, userID, otherID, name) {
  return request(token, '/projects', 'POST', { name, type: 'general', scope: 'Actual shared mobile closeout and immutable archive review.', owner_id: userID,
    member_ids: [userID, otherID], classification: 'project_restricted' });
}
async function closingProject(token, userID, otherID, name) {
  let p = await createProject(token, userID, otherID, name); p = await transitionAPI(token, p.id, 'activate'); return transitionAPI(token, p.id, 'close');
}
async function archiveAPI(token, pid, n) {
  const b = await request(token, '/projects/' + pid + '/closeout'); assert.equal(b.can_archive, true);
  return request(token, '/projects/' + pid + '/archive', 'POST', { expected_version: b.project.version, expected_fingerprint: b.fingerprint,
    summary: 'Actual historical archive ' + n, lessons_learned: 'Original commands preserve each immutable review.', follow_up: 'Explicitly no remaining follow-up.',
    reason: 'Actual history pagination review', business_verified: true, financial_verified: true, file_version_ids: [] });
}

try {
  assert.equal(fixture.project_closeout_fixture, true);
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), admin = await login(fixture.administrator), bobInitial = await login(fixture.bob), page = alice.page; mainPage = page;
  const me = await request(alice.token, '/me'), other = await request(bobInitial.token, '/me');
  for (const action of ['project.update', 'project.archive', 'project.reopen']) assert.ok(me.actions.includes(action));
  let project = await createProject(alice.token, me.user.id, other.user.id, '移动项目 · 结项与固定复盘'); const pid = project.id;
  await navigate(page, '/pages/detail/index?kind=project&id=' + pid); await heading(page, project.name);
  await screen(page).locator('.cp-row-title').getByText('项目结项与归档', { exact: true }).click(); await freshProject(page, project);
  project = await transitionUI(page, project, '启动项目'); project = await transitionUI(page, project, '暂停项目'); project = await transitionUI(page, project, '恢复进行');
  assert.equal(project.state, 'active');
  recordCheck('Actual project detail opens closeout; original mobile activate, hold and resume commands advance only the intended project revision.');

  const task = await request(alice.token, '/projects/' + pid + '/tasks', 'POST', { title: '合成演示 · 尚未完成的交付', body: 'Original unfinished work must be explicitly disposed.', priority: 'normal', review_required: false });
  await button(page, '重新读取项目结项').click(); await freshProject(page, project);
  await screen(page).locator('.cp-row-title').getByText(task.title, { exact: true }).waitFor();
  assert.equal(await button(page, '确认进入结项核查阶段').count(), 0);
  const blocked = await request(alice.token, '/projects/' + pid + '/closeout'); assert.equal(blocked.groups.length, 13); assert.equal(blocked.can_close, false);
  assert.equal(blocked.groups.find(g => g.kind === 'tasks').open_count, 1);
  await layout(page, 'closeout-checklist'); await image(page, 'mobile-project-closeout', '实际十三类结项核查与未完成任务；合成H5 / Actual thirteen-category checklist and unfinished work; synthetic H5.');
  await screen(page).locator('.cp-row-title').getByText(task.title, { exact: true }).click(); await heading(page, task.title);
  await request(alice.token, '/tasks/' + task.id + '/transition', 'POST', { expected_version: task.version, action: 'cancel', reason: 'Actual unnecessary work explicitly cancelled' });
  await openCloseout(page, project); project = await transitionUI(page, project, '进入结项核查阶段'); assert.equal(project.state, 'closing');
  await field(page, '成果与交付总结').waitFor(); assert.equal(await button(page, '确认复盘并归档').isDisabled(), true);
  recordCheck('All thirteen actual categories are loaded; an unfinished task blocks closing, its original record link works, and explicit disposition permits the separate closing stage.');

  const summary = '合成演示：已核对实际交付成果与项目范围。'; await notes(page, summary); await attest(page); await mfa(page, fixture.alice);
  const staleVersion = project.version;
  project = await transitionAPI(alice.token, pid, 'resume'); project = await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, name: project.name + ' Concurrent revision' });
  project = await transitionAPI(alice.token, pid, 'close');
  const conflict = await submit(page, '确认复盘并归档', '/projects/' + pid + '/archive', 'POST', 409);
  assert.equal(conflict.body.expected_version, staleVersion); await screen(page).getByText(/输入已保留/).waitFor(); assert.equal(await field(page, '成果与交付总结').inputValue(), summary);
  assert.equal(await button(page, '确认复盘并归档').isDisabled(), true); assert.equal((await request(alice.token, '/projects/' + pid + '/archives')).items.length, 0);
  await button(page, '保留文字和选定版本，重新核查').click(); await freshProject(page, project);
  assert.equal(await field(page, '成果与交付总结').inputValue(), summary);
  for (const label of ['业务闭环核实', '财务与义务核实']) assert.equal((await screen(page).locator('taro-picker-core[aria-label="' + label + '"] .cp-picker').textContent()).trim(), '尚未确认');
  assert.equal(await field(page, '验证器六位验证码').count(), 1); assert.equal(await button(page, '确认复盘并归档').isDisabled(), true);
  recordCheck('A real concurrent project revision rejects stale archive without creating history; explicit reread preserves text while resetting both attestations and current MFA.');

  await attest(page); await mfa(page, fixture.alice); await layout(page, 'archive-form');
  await field(page, '成果与交付总结').scrollIntoViewIfNeeded(); await image(page, 'mobile-project-archive-form', '实际复盘表单；已清除验证码，需明确确认 / Actual retrospective form; MFA input cleared, explicit confirmation required.');
  let firstArchive;
  recordings.push(await recordViewport(page, app, 'mobile-project-closeout-workflow', async () => {
    const result = await submit(page, '确认复盘并归档', '/projects/' + pid + '/archive', 'POST', 201); project = result.value.project; firstArchive = result.value.archive;
    assert.equal(result.body.business_verified, true); assert.equal(result.body.financial_verified, true); assert.deepEqual(result.body.file_version_ids, []);
    await freshProject(page, project); await screen(page).getByText('当前仍与此归档依据一致。', { exact: true }).waitFor();
    await button(page, '查阅本次完整复盘').click(); await screen(page).getByText(summary, { exact: true }).scrollIntoViewIfNeeded();
    await screen(page).getByText(summary, { exact: true }).waitFor(); await image(page, 'mobile-project-archive-history', '原服务提交后的固定复盘与归档时清单 / Immutable retrospective and checklist after an actual server commit.');
    project = await transitionUI(page, project, '重新打开项目');
    await screen(page).getByText('当前依据已变化；以下保留归档当时事实。', { exact: true }).waitFor();
    await button(page, '查阅本次完整复盘').click(); await screen(page).getByText(summary, { exact: true }).scrollIntoViewIfNeeded();
  }));
  const preserved = await request(alice.token, '/projects/' + pid + '/archives'); assert.equal(preserved.items[0].id, firstArchive.id); assert.equal(preserved.items[0].summary, summary); assert.equal(preserved.items[0].current_basis_state, 'changed');
  recordCheck('Actual archive requires complete notes, two explicit attestations and recent MFA; success reconciles the current project and actual history, and controlled reopen preserves the immutable old review.');

  for (let n = 2; n <= 11; n++) { if (n > 2) await transitionAPI(alice.token, pid, 'reopen'); await transitionAPI(alice.token, pid, 'close'); await archiveAPI(alice.token, pid, n); }
  project = await request(alice.token, '/projects/' + pid); await openCloseout(page, project);
  await button(page, '加载更早归档记录').waitFor(); assert.equal(await button(page, '查阅本次完整复盘').count(), 10);
  await button(page, '加载更早归档记录').click(); await screen(page).getByText('当前载入 11 份历史，后续按原服务分页读取。', { exact: true }).waitFor();
  assert.equal(await button(page, '查阅本次完整复盘').count(), 11); assert.equal(await button(page, '加载更早归档记录').count(), 0);
  recordCheck('Eleven real original-command archive cycles produce distinct immutable records; the actual cursor loads ten then eleven reviews with no duplicate or false complete count.');

  let proofProject = await closingProject(alice.token, me.user.id, other.user.id, 'Actual archive proof failure'); await openCloseout(page, proofProject);
  await notes(page, 'Actual committed review with incomplete follow-up proof'); await attest(page); await mfa(page, fixture.alice);
  const proofRoute = '**/v1/projects/' + proofProject.id + '/archives?*';
  await page.route(proofRoute, async route => { const actual = await route.fetch(); const body = await actual.json(); body.data.items = []; body.data.next_cursor = null; await route.fulfill({ response: actual, json: body }); });
  await submit(page, '确认复盘并归档', '/projects/' + proofProject.id + '/archive', 'POST', 201);
  await screen(page).getByText(/操作返回的资料不完整/).waitFor(); assert.equal(await button(page, '确认复盘并归档').isDisabled(), true);
  assert.equal((await request(alice.token, '/projects/' + proofProject.id + '/archives')).items.length, 1);
  assert.equal(await field(page, '成果与交付总结').inputValue(), 'Actual committed review with incomplete follow-up proof');
  await page.unroute(proofRoute); await button(page, '保留文字和选定版本，重新核查').click(); proofProject = await request(alice.token, '/projects/' + proofProject.id); await freshProject(page, proofProject);
  await screen(page).getByText('当前仍与此归档依据一致。', { exact: true }).waitFor();
  recordCheck('An actual committed archive with concealed asynchronous history proof blocks false success and replay, retains input, and is reconciled by an explicit read of the real archived project.');

  let lostProject = await closingProject(alice.token, me.user.id, other.user.id, 'Actual archive with lost browser response'); await openCloseout(page, lostProject);
  await notes(page, 'Actual archive response lost after commit'); await attest(page); await mfa(page, fixture.alice);
  const lostRoute = '**/v1/projects/' + lostProject.id + '/archive'; let writes = 0;
  await page.route(lostRoute, async route => { writes++; const actual = await route.fetch(); assert.equal(actual.status(), 201); await route.abort('failed'); });
  await button(page, '确认复盘并归档').click(); await screen(page).getByText(/输入已保留/).waitFor(); assert.equal(writes, 1); assert.equal(await button(page, '确认复盘并归档').isDisabled(), true);
  assert.equal((await request(alice.token, '/projects/' + lostProject.id + '/archives')).items.length, 1); await page.unroute(lostRoute);
  await button(page, '保留文字和选定版本，重新核查').click(); lostProject = await request(alice.token, '/projects/' + lostProject.id); await freshProject(page, lostProject);
  await screen(page).getByText('当前仍与此归档依据一致。', { exact: true }).waitFor();
  recordCheck('A real archive transaction with a lost browser response produces exactly one history record, blocks repeat submission, and requires explicit reconciliation.');

  mainPage = bobInitial.page; await openCloseout(bobInitial.page, project);
  await screen(bobInitial.page).getByText('权限受限，历史内容、数量和复盘尚未核实。', { exact: true }).waitFor();
  const restricted = await request(bobInitial.token, '/projects/' + pid + '/closeout'), hidden = await request(bobInitial.token, '/projects/' + pid + '/archives');
  assert.equal(restricted.visibility, 'restricted'); assert.equal(restricted.fingerprint, null); assert.equal(hidden.visibility, 'restricted'); assert.deepEqual(hidden.items, []); assert.equal(hidden.next_cursor, null);
  for (const g of restricted.groups.filter(g => g.visibility === 'restricted')) { assert.equal(g.count, null); assert.equal(g.open_count, null); assert.deepEqual(g.items, []); }
  assert.equal(await button(bobInitial.page, '查阅本次完整复盘').count(), 0); assert.equal(await button(bobInitial.page, '确认复盘并归档').count(), 0);
  await layout(bobInitial.page, 'restricted-closeout');
  const wrongRoute = '**/v1/projects/' + pid + '/closeout';
  await bobInitial.page.route(wrongRoute, async route => { const actual = await route.fetch(); const body = await actual.json(); body.data.project.id = lostProject.id; await route.fulfill({ response: actual, json: body }); });
  await button(bobInitial.page, '重新读取项目结项').click(); await screen(bobInitial.page).getByText(/结项核查资料不完整或不属于当前项目/).waitFor();
  assert.equal(await button(bobInitial.page, '查阅本次完整复盘').count(), 0); await bobInitial.page.unroute(wrongRoute); await closeOwned(bobInitial.context);
  recordCheck('A reader missing complete source authority sees restricted null counts and no history metadata; a mismatched project receipt cannot open the closeout workspace.');

  const roles = await request(admin.token, '/roles'), manager = roles.items.find(r => r.key === 'project_manager'); assert.ok(manager);
  await request(admin.token, '/role-bindings', 'POST', { user_id: other.user.id, role_id: manager.id, scope_type: 'project', scope_id: pid, classification_ceiling: 'project_restricted', reason: 'Actual independent manager for viewer restriction review' });
  const successor = await login(fixture.bob); await transitionAPI(alice.token, pid, 'reopen'); project = await request(alice.token, '/projects/' + pid);
  project = await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, owner_id: other.user.id });
  project = await request(successor.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: me.user.id, action: 'set', project_role: 'viewer', reason: 'Actual read-only archive reviewer' });
  await closeOwned(alice.context); const viewer = await login(fixture.alice); mainPage = viewer.page; await openCloseout(viewer.page, project);
  assert.equal(await field(viewer.page, '项目阶段原因').count(), 0); assert.equal(await button(viewer.page, '确认复盘并归档').count(), 0);
  const actualViewer = await request(viewer.token, '/projects/' + pid + '/members'); assert.equal(actualViewer.memberships.find(m => m.user_id === me.user.id).project_role, 'viewer');
  recordCheck('After a real ownership transfer and member-role restriction, a fresh MFA login reads the project as viewer and exposes no lifecycle or archive write controls.');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  failure = String(error.stack ?? error);
  if (mainPage && !mainPage.isClosed()) {
    try { failure += '\nActive page: ' + JSON.stringify({ route: new URL(mainPage.url()).hash, titles: await screen(mainPage).locator('.cp-title').allTextContents(), actions: await screen(mainPage).getByRole('button').allTextContents() }); await mainPage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true, mask: [mainPage.locator('input[type="password"],input[maxlength="6"],.cp-code')] }); } catch { /* Keep original error. */ }
  }
  for (const account of [fixture.alice, fixture.bob, fixture.administrator]) for (const key of ['password', 'mfa_secret']) if (account?.[key]) failure = failure.replaceAll(account[key], '[redacted]');
  process.stderr.write(failure + '\n');
} finally {
  for (const context of contexts) { try { await closeOwned(context); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && !cleanupErrors.length && closedContexts.size === contexts.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, recordings, screenshots, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, failure, owned_browser_closed: closed, browser_contexts_created: contexts.length, browser_contexts_closed: closedContexts.size, real_native_api: true, real_model: false, real_cloud: false, real_device: false };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
