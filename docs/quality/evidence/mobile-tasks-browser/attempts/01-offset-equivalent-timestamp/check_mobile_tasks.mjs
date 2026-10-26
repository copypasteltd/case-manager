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
const evidence = path.join(app, '../docs/quality/evidence/mobile-tasks-browser');
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

async function fresh(page, task) { await heading(page, task.title); await screen(page).getByText(new RegExp('^修订 ' + task.version + ' ·')).waitFor(); await stable(page); }
async function savedDetail(page, task) { await button(page, '查看已保存任务').click(); await fresh(page, task); }
async function openTasks(page, pid) {
  await button(page, '查看所属项目').click(); await heading(page, 'Actual mobile task project');
  await screen(page).locator('.cp-row-title').getByText('项目任务', { exact: true }).click(); await heading(page, '项目任务');
  await button(page, '新建任务').waitFor();
}
async function choose(page, title, text) {
  const section = screen(page).locator('.cp-section').filter({ has: page.locator('.cp-section-title').filter({ hasText: new RegExp('^' + title + '$') }) }).last();
  await section.locator('.cp-row-title').getByText(text, { exact: true }).click(); await stable(page);
}
async function addDependency(page, predecessor, successor) {
  await button(page, '添加前置任务').click(); await field(page, '查找候选资料').fill(predecessor.title); await button(page, '查找候选资料').click();
  await choose(page, '选择前置任务', predecessor.title);
  const result = await submit(page, '确认添加这一条前置任务', '/tasks/' + successor.id + '/dependencies', 'POST');
  await fresh(page, result.value); await screen(page).locator('.cp-row-title').getByText(predecessor.title, { exact: true }).waitFor(); return result;
}
async function taskTransition(page, task, action, reason, status = 200) {
  const labels = { start: '开始处理', complete: task.state === 'in_review' ? '确认完成' : '标记完成', review: '提交审核' };
  await select(page, '本次动作', labels[action]); await field(page, '变更或决定原因').fill(reason);
  const result = await submit(page, '确认' + labels[action], '/tasks/' + task.id + '/transition', 'POST', status);
  if (status === 200) await fresh(page, result.value); return result;
}
async function image(page, name, caption) {
  await stable(page); const filename = path.join(app, '../docs/media/screenshots/' + name + '.png');
  await page.screenshot({ path: filename, fullPage: false });
  screenshots.push({ file: 'docs/media/screenshots/' + name + '.png', caption, sha256: createHash('sha256').update(await readFile(filename)).digest('hex'), synthetic_data: true, real_device: false });
}

try {
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), admin = await login(fixture.administrator), page = alice.page; mainPage = page;
  const me = await request(alice.token, '/me');
  for (const action of ['task.create', 'task.update', 'task.assign', 'task.transition']) assert.ok(me.actions.includes(action));
  let project = await request(alice.token, '/projects', 'POST', { name: 'Actual mobile task project', type: 'general', scope: 'Actual creation, assignment, subtasks, nullable edits and dependencies.', owner_id: me.user.id, classification: 'project_restricted' });
  const pid = project.id, directory = await request(alice.token, '/projects/' + pid + '/member-candidates'), bob = directory.items.find(user => user.display_name === 'bob'); assert.ok(bob);
  project = await request(alice.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: bob.id, action: 'set', project_role: 'member', reason: 'Actual task responsibility candidate' });
  project = await request(alice.token, '/projects/' + pid + '/transition', 'POST', { expected_version: project.version, action: 'activate', reason: 'Begin task collaboration review' });
  const milestone = await request(alice.token, '/projects/' + pid + '/milestones', 'POST', { title: 'Actual task checkpoint', owner_id: me.user.id, due_at: '2028-05-03T10:00:00.123456Z' });
  // Actual commands create more than one candidate page; no browser state is injected.
  for (let n = 0; n < 22; n++) await request(alice.token, '/projects/' + pid + '/tasks', 'POST', { title: 'Reference candidate ' + String(n).padStart(2, '0'), priority: 'normal', body: 'Controlled pagination fixture.', review_required: false });
  await navigate(page, '/pages/detail/index?kind=project&id=' + pid); await heading(page, project.name);
  await screen(page).locator('.cp-row-title').getByText('项目任务', { exact: true }).click(); await heading(page, '项目任务'); await button(page, '加载更多任务').waitFor();
  const first = await screen(page).locator('.cp-row-title').allTextContents(); await button(page, '加载更多任务').click();
  await screen(page).locator('.cp-row-title').getByText('Reference candidate 00', { exact: true }).waitFor(); assert.ok((await screen(page).locator('.cp-row-title').count()) > first.length);
  await button(page, '新建任务').click(); await heading(page, '新建任务');
  let writes = 0; page.on('request', r => { if (new URL(r.url()).pathname === '/v1/projects/' + pid + '/tasks' && r.method() === 'POST') writes++; });
  await field(page, '任务名称').fill('移动任务 · 核对实际交付'); await field(page, '描述与验收标准').fill('合成演示：核对完整范围与原始资料。');
  await field(page, '开始时间（香港时间）').fill('2026-02-29 09:00'); await button(page, '确认保存任务').click(); await screen(page).getByText(/日期时间不存在/).waitFor(); assert.equal(writes, 0);
  await field(page, '开始时间（香港时间）').fill('2028-05-01 09:00:00.123456'); await field(page, '截止时间（香港时间）').fill('2028-05-03 18:00:00.123457');
  await select(page, '任务负责人', bob.display_name); await select(page, '任务优先级', '高');
  await layout(page, 'task-create'); await image(page, 'mobile-task-form', '实际Taro任务表单；合成数据，H5 / Actual shared Taro task form; synthetic H5.');
  let main, child;
  recordings.push(await recordViewport(page, app, 'mobile-task-workflow', async () => {
    let result = await submit(page, '确认保存任务', '/projects/' + pid + '/tasks', 'POST', 201); main = result.value;
    assert.equal(main.version, 1); assert.equal(main.assignee_id, bob.id); assert.equal(main.start_at, '2028-05-01T01:00:00.123456Z'); assert.equal('expected_version' in result.body, false); assert.equal(writes, 1);
    await savedDetail(page, main); await button(page, '新建子任务').click(); await heading(page, '新建子任务'); await field(page, '任务名称').fill('移动子任务 · 固定依据核验');
    await field(page, '描述与验收标准').fill('合成演示：独立核对里程碑和准确资料。'); await select(page, '完成要求', '提交审核后完成');
    await button(page, '选择任务里程碑').click(); await choose(page, '选择任务里程碑', milestone.title);
    result = await submit(page, '确认保存任务', '/projects/' + pid + '/tasks', 'POST', 201); child = result.value;
    assert.equal(child.parent_id, main.id); assert.equal(child.milestone_id, milestone.id); assert.equal(child.review_required, true); assert.equal(child.assignee_id, null);
    await savedDetail(page, child);
    child = (await addDependency(page, main, child)).value;
    await image(page, 'mobile-task-dependencies', '实际子任务、里程碑和已核对的前置依赖 / Actual subtask, milestone and reconciled prerequisite.');
  }));
  checks.push('Actual mobile creation retains responsible member, full content and Hong Kong microseconds; impossible dates send no command, with real task pagination.');
  checks.push('Creating a subtask binds its real parent and selected milestone; an explicit dependency is verified through actual edge and task reads.');

  await button(page, '编辑任务资料').click(); await heading(page, '编辑任务'); await button(page, '选择父任务').click(); await button(page, '加载更多候选').waitFor();
  const count = await screen(page).locator('.cp-row-title').count(); await button(page, '加载更多候选').click(); await screen(page).locator('.cp-row-title').getByText('Reference candidate 00', { exact: true }).waitFor(); assert.ok(await screen(page).locator('.cp-row-title').count() > count);
  await button(page, '返回表单，保留输入').click(); await button(page, '解除父任务关联').click(); await button(page, '解除任务里程碑关联').click();
  await field(page, '开始时间（香港时间）').fill('2028-05-01 10:00'); await field(page, '截止时间（香港时间）').fill('2028-05-02 18:00');
  let result = await submit(page, '确认保存任务', '/tasks/' + child.id, 'PATCH'); child = result.value;
  assert.equal(result.body.parent_id, null); assert.equal(result.body.milestone_id, null); assert.equal('assignee_id' in result.body, false);
  await savedDetail(page, child); await button(page, '编辑任务资料').click(); await field(page, '开始时间（香港时间）').fill(''); await field(page, '截止时间（香港时间）').fill('');
  result = await submit(page, '确认保存任务', '/tasks/' + child.id, 'PATCH'); child = result.value; assert.equal(result.body.start_at, null); assert.equal(result.body.due_at, null); assert.equal(child.start_at ?? null, null); assert.equal(child.due_at ?? null, null); await savedDetail(page, child);
  checks.push('Parent candidate pagination is real; editing explicitly unlinks parent/milestone and clears existing dates with null while preserving assignment permission semantics.');

  await taskTransition(page, child, 'start', 'Actual predecessor is not completed', 422); await screen(page).getByText('前置任务尚未完成', { exact: true }).waitFor(); assert.equal((await request(alice.token, '/tasks/' + child.id)).state, 'todo');
  await button(page, '查看父任务：' + main.title).count().then(async count => { assert.equal(count, 0); });
  // Visit the actual visible predecessor through its row, then attempt an opposing edge.
  await screen(page).locator('.cp-row-title').getByText(main.title, { exact: true }).click(); await fresh(page, main);
  await button(page, '添加前置任务').click(); await field(page, '查找候选资料').fill(child.title); await button(page, '查找候选资料').click(); await choose(page, '选择前置任务', child.title);
  await submit(page, '确认添加这一条前置任务', '/tasks/' + main.id + '/dependencies', 'POST', 409); await screen(page).getByText('任务依赖不能形成循环', { exact: true }).waitFor(); assert.equal((await request(alice.token, '/tasks/' + main.id)).version, main.version);
  await button(page, '刷新资料').click(); await fresh(page, main);
  main = (await taskTransition(page, main, 'start', 'Start the actual predecessor')).value;
  main = (await taskTransition(page, main, 'complete', 'Verify the actual predecessor outcome')).value;
  await openTasks(page, pid); await field(page, '任务标题关键词').fill(child.title); await button(page, '查找任务').click(); await screen(page).locator('.cp-row-title').getByText(child.title, { exact: true }).click(); await fresh(page, child);
  child = (await taskTransition(page, child, 'start', 'Predecessor is now actually done')).value;
  await layout(page, 'task-dependencies');
  await button(page, '移除前置：' + main.title).click(); result = await submit(page, '确认移除这一条前置任务', '/tasks/' + child.id + '/dependencies', 'POST'); child = result.value; await fresh(page, child);
  assert.deepEqual((await request(alice.token, '/tasks/' + child.id + '/dependencies')).items, []);
  checks.push('Original server rejects an incomplete prerequisite and a real dependency cycle; completing the predecessor permits start, and explicit removal reconciles the actual edge.');

  // The original transaction commits, but the browser receives an incomplete receipt.
  await button(page, '编辑任务资料').click(); await field(page, '任务名称').fill('Reconciled task after incomplete receipt');
  const patchRoute = '**/v1/tasks/' + child.id;
  await page.route(patchRoute, async route => { if (route.request().method() !== 'PATCH') return route.continue(); const actual = await route.fetch(); const body = await actual.json(); body.data.project_id = project.id === body.data.project_id ? randomUUID() : project.id; await route.fulfill({ response: actual, json: body }); });
  await submit(page, '确认保存任务', '/tasks/' + child.id, 'PATCH'); await screen(page).getByText(/操作返回的资料不完整/).waitFor(); assert.equal(await field(page, '任务名称').inputValue(), 'Reconciled task after incomplete receipt'); assert.equal(await button(page, '确认保存任务').isDisabled(), true);
  await page.unroute(patchRoute); child = await request(alice.token, '/tasks/' + child.id); assert.equal(child.title, 'Reconciled task after incomplete receipt');
  await button(page, '重新读取当前权限与任务').click(); await field(page, '任务名称').waitFor(); assert.equal(await field(page, '任务名称').inputValue(), child.title);
  const stale = child; child = await request(alice.token, '/tasks/' + child.id, 'PATCH', { expected_version: child.version, body: 'Actual concurrent revision' });
  await field(page, '任务名称').fill('Unsubmitted conflicting title'); result = await submit(page, '确认保存任务', '/tasks/' + child.id, 'PATCH', 409); assert.equal(result.body.expected_version, stale.version); await screen(page).getByText(/输入已保留/).waitFor(); assert.equal(await field(page, '任务名称').inputValue(), 'Unsubmitted conflicting title'); assert.equal((await request(alice.token, '/tasks/' + child.id)).title, child.title);
  checks.push('An incomplete success receipt and an actual concurrent revision preserve typed input, block replay and require reading the real committed version.');

  await button(page, '查看任务列表核对结果').click(); await heading(page, '项目任务'); await button(page, '新建任务').click(); await field(page, '任务名称').fill('Actual task with a lost response');
  let lostID, lostCount = 0; const createRoute = '**/v1/projects/' + pid + '/tasks';
  await page.route(createRoute, async route => { if (route.request().method() !== 'POST') return route.continue(); lostCount++; const r = await route.fetch(); assert.equal(r.status(), 201); lostID = (await r.json()).data.id; await route.abort('failed'); });
  await button(page, '确认保存任务').click(); await screen(page).getByText(/输入已保留/).waitFor(); assert.equal(lostCount, 1); assert.equal(await button(page, '确认保存任务').isDisabled(), true); await page.unroute(createRoute);
  const actualLost = await request(alice.token, '/tasks/' + lostID); assert.equal(actualLost.title, 'Actual task with a lost response');
  const matches = await request(alice.token, '/projects/' + pid + '/tasks?limit=20&q=' + encodeURIComponent(actualLost.title)); assert.equal(matches.items.length, 1); assert.equal(matches.items[0].id, lostID);
  checks.push('A committed creation with a lost browser response produces exactly one task and blocks repeat creation until the actual record is reconciled.');

  const foreign = await request(alice.token, '/projects', 'POST', { name: 'Other actual task project', type: 'general', scope: 'Deep-link scope guard.', owner_id: me.user.id, classification: 'project_restricted' });
  const foreignTask = await request(alice.token, '/projects/' + foreign.id + '/tasks', 'POST', { title: 'Foreign task', body: 'Not the first project', priority: 'normal', review_required: false });
  const cross = await login(fixture.alice); mainPage = cross.page; await navigate(cross.page, '/pages/project/index?section=task-form&id=' + pid + '&task_id=' + foreignTask.id); await screen(cross.page).getByText(/任务与项目不匹配/).waitFor(); assert.equal(await button(cross.page, '确认保存任务').count(), 0); await closeOwned(cross.context);
  await closeOwned(alice.context);
  checks.push('A currently accessible foreign task cannot open an editing form beneath a different project.');

  const roles = await request(admin.token, '/roles'), role = roles.items.find(value => value.key === 'project_manager'); assert.ok(role);
  await request(admin.token, '/role-bindings', 'POST', { user_id: bob.id, role_id: role.id, scope_type: 'project', scope_id: pid, classification_ceiling: 'project_restricted', reason: 'Independent project manager for actual viewer restriction review' });
  const successor = await login(fixture.bob); project = await request(successor.token, '/projects/' + pid);
  project = await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, owner_id: bob.id, reason: 'Actual ownership transfer before restricting previous owner' });
  project = await request(successor.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: me.user.id, action: 'set', project_role: 'viewer', reason: 'Actual read-only member proof' });
  const viewer = await login(fixture.alice); mainPage = viewer.page; await navigate(viewer.page, '/pages/detail/index?kind=task&id=' + child.id); await fresh(viewer.page, child); await screen(viewer.page).getByText('当前以只读方式查阅任务。', { exact: true }).waitFor();
  for (const label of ['编辑任务资料', '新建子任务', '添加前置任务']) assert.equal(await button(viewer.page, label).count(), 0);
  await button(viewer.page, '查看所属项目').click(); await screen(viewer.page).locator('.cp-row-title').getByText('项目任务', { exact: true }).click(); await heading(viewer.page, '项目任务'); assert.equal(await button(viewer.page, '新建任务').count(), 0); await closeOwned(viewer.context);
  project = await request(successor.token, '/projects/' + pid); await request(successor.token, '/projects/' + pid + '/transition', 'POST', { expected_version: project.version, action: 'cancel', reason: 'Actual stopped-project task guard' });
  mainPage = successor.page; await navigate(successor.page, '/pages/project/index?section=task-form&id=' + pid); await screen(successor.page).getByText('当前权限、项目角色或任务阶段不允许创建或编辑。', { exact: true }).waitFor(); assert.equal(await button(successor.page, '确认保存任务').count(), 0);
  checks.push('Actual member restriction requires fresh MFA; a viewer has no create/edit/dependency controls and a cancelled project rejects creation deep links.');
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

