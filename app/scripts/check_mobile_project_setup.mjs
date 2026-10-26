import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-project-setup-browser');
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true); assert.ok(fixture.published_template_id);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) { h.update(file + '\0'); h.update(await readFile(path.join(app, file))); h.update('\0'); } return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], checks = [], layouts = [], errors = [], serverErrors = [], cleanupErrors = [];
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, label) => screen(page).locator('input[aria-label="' + label + '"]');
let browser, passed = false, failure;
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); }
async function navigate(page, url) { await page.evaluate(value => { window.location.hash = '#' + value; }, url); await stable(page); }
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

try {
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), admin = await login(fixture.administrator), page = alice.page;
  const me = await request(alice.token, '/me'), template = await request(alice.token, '/project-templates/' + fixture.published_template_id);
  assert.equal(template.state, 'published'); assert.ok(me.actions.includes('project.create') && me.actions.includes('project.update') && me.actions.includes('project.manage_members'));
  await navigate(page, '/pages/list/index?kind=projects'); await button(page, '新建项目').click(); await heading(page, '新建项目');
  const projectName = 'Mobile original project ' + Date.now(); let creates = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === '/v1/projects' && r.method() === 'POST') creates++; });
  await field(page, '项目名称').fill(projectName); await screen(page).locator('textarea[aria-label="项目范围与目标"]').fill('Complete mobile project scope.');
  await field(page, '项目开始日期').fill('2026-02-29'); await button(page, '确认创建项目').click(); await screen(page).getByText(/请使用有效的 YYYY-MM-DD/).waitFor(); assert.equal(creates, 0);
  await field(page, '项目开始日期').fill('2028-02-29'); await field(page, '项目截止日期').fill('2028-03-01');
  await button(page, '选择已发布项目模板').click(); await field(page, '查找候选资料').fill(template.key); await button(page, '查找候选资料').click();
  await screen(page).locator('.cp-row-title').getByText(template.name + ' · ' + template.key + ' · 修订 ' + template.revision, { exact: true }).click();
  await screen(page).getByText(template.content_hash, { exact: true }).waitFor(); await layout(page, 'project-creation');
  const created = await submit(page, '确认创建项目', '/projects', 'POST', 201); const pid = created.value.id;
  assert.deepEqual(created.body, { name: projectName, type: 'general', scope: 'Complete mobile project scope.', owner_id: me.user.id, classification: 'project_restricted', start_date: '2028-02-29', due_date: '2028-03-01', template_id: template.id });
  await button(page, '查看已保存项目').waitFor(); assert.equal(creates, 1);
  const initial = await request(alice.token, '/projects/' + pid), tasks = await request(alice.token, '/projects/' + pid + '/tasks?limit=100'), milestones = await request(alice.token, '/projects/' + pid + '/milestones?limit=100'), folders = await request(alice.token, '/folders?project_id=' + pid + '&limit=100');
  assert.equal(initial.template_version, template.revision); assert.equal(initial.initialization_state, 'complete');
  assert.deepEqual(tasks.items.map(v => v.title).sort(), template.schema.tasks.map(v => v.title).sort()); assert.deepEqual(milestones.items.map(v => v.title).sort(), template.schema.milestones.map(v => v.title).sort());
  const rootFolder = folders.items.find(v => v.project_id === pid && !v.parent_id); assert.ok(rootFolder, 'Actual project root folder');
  const children = await request(alice.token, '/folders?parent_id=' + rootFolder.id + '&limit=100'); assert.equal(children.next_cursor, null);
  for (const name of new Set(['合同', '交付', '工作资料', ...template.schema.folders])) assert.ok(children.items.some(v => v.name === name), name);
  checks.push('Actual mobile template creation fixes owner, dates and exact revision and initializes real tasks, milestones and logical folders once.');

  await button(page, '查看已保存项目').click(); await heading(page, projectName); await button(page, '编辑项目资料').click(); await heading(page, '编辑项目资料');
  await field(page, '项目名称').fill('Mobile revised project'); await field(page, '项目开始日期').fill(''); await field(page, '项目截止日期').fill('');
  const edited = await submit(page, '确认保存项目资料', '/projects/' + pid, 'PATCH'); assert.equal(edited.body.expected_version, initial.version); assert.equal(edited.body.start_date, null); assert.equal(edited.body.due_date, null); assert.equal(edited.value.version, initial.version + 1); await button(page, '查看已保存项目').waitFor();
  checks.push('Draft editing uses the actual project version and explicit date clearing without changing owner, classification or selected template.');

  await button(page, '查看已保存项目').click(); await button(page, '编辑项目资料').click(); await field(page, '项目名称').fill('Committed mobile response lost');
  let lost;
  await page.route('**/v1/projects/' + pid, async route => { if (route.request().method() !== 'PATCH') return route.continue(); const r = await route.fetch(); lost = { status: r.status(), value: (await r.json()).data }; await route.abort('failed'); });
  await button(page, '确认保存项目资料').click(); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(lost.status, 200); assert.equal(await button(page, '确认保存项目资料').isDisabled(), true);
  await page.unroute('**/v1/projects/' + pid); const afterLoss = await request(alice.token, '/projects/' + pid); assert.equal(afterLoss.name, 'Committed mobile response lost'); assert.equal(afterLoss.version, edited.value.version + 1);
  await button(page, '重新读取当前权限与项目').click(); await field(page, '项目名称').waitFor(); assert.equal(await field(page, '项目名称').inputValue(), afterLoss.name);
  checks.push('A real committed update with a lost browser response blocks resubmission and explicit reread reconciles its actual result.');

  await field(page, '项目名称').fill('Stale mobile input'); const concurrent = await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: afterLoss.version, name: 'Concurrent actual project' });
  await submit(page, '确认保存项目资料', '/projects/' + pid, 'PATCH', 409); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(await field(page, '项目名称').inputValue(), 'Stale mobile input'); assert.equal((await request(alice.token, '/projects/' + pid)).name, concurrent.name);
  await button(page, '重新读取当前权限与项目').click(); await field(page, '项目名称').waitFor(); assert.equal(await field(page, '项目名称').inputValue(), concurrent.name);
  const active = await request(alice.token, '/projects/' + pid + '/transition', 'POST', { expected_version: concurrent.version, action: 'activate', reason: 'Reviewed actual project setup' });
  await button(page, '重新读取当前权限与项目').click(); await field(page, '项目名称').waitFor(); assert.equal(await field(page, '项目开始日期').isDisabled(), true); assert.equal(await screen(page).locator('textarea[aria-label="项目范围与目标"]').isDisabled(), true);
  await field(page, '项目名称').fill('Active mobile project'); const activeEdit = await submit(page, '确认保存项目资料', '/projects/' + pid, 'PATCH'); assert.deepEqual(activeEdit.body, { expected_version: active.version, name: 'Active mobile project', type: 'general' }); await button(page, '查看已保存项目').waitFor();
  checks.push('Concurrent edits preserve unsubmitted text and block stale writes; active project scope and dates are read-only and absent from ordinary updates.');

  await button(page, '安排项目成员').click(); await heading(page, '项目成员'); await button(page, '配置或移除项目成员').click();
  const directory = await request(alice.token, '/projects/' + pid + '/member-candidates'), bob = directory.items.find(v => v.display_name === 'bob'); assert.ok(bob);
  await select(page, '待配置项目成员', bob.display_name); await select(page, '本次成员安排', '设为只读成员'); await field(page, '成员变更原因').fill('Explicit mobile viewer assignment');
  let beforeMember = await request(alice.token, '/projects/' + pid); const member = await submit(page, '确认本次成员变更', '/projects/' + pid + '/members', 'POST'); assert.deepEqual(member.body, { expected_version: beforeMember.version, user_id: bob.id, action: 'set', project_role: 'viewer', reason: 'Explicit mobile viewer assignment' });
  await button(page, '配置或移除项目成员').waitFor(); assert.equal((await request(alice.token, '/projects/' + pid + '/members')).memberships.find(v => v.user_id === bob.id).project_role, 'viewer');
  await button(page, '配置或移除项目成员').click(); await select(page, '待配置项目成员', bob.display_name); await select(page, '本次成员安排', '移除项目成员'); await field(page, '成员变更原因').fill('No unresolved assignments for this member');
  await submit(page, '确认本次成员变更', '/projects/' + pid + '/members', 'POST'); await button(page, '配置或移除项目成员').waitFor(); assert.equal((await request(alice.token, '/projects/' + pid + '/members')).items.some(v => v.id === bob.id), false);
  await button(page, '配置或移除项目成员').click(); await select(page, '待配置项目成员', bob.display_name); await field(page, '成员变更原因').fill('Restore explicit project membership'); await submit(page, '确认本次成员变更', '/projects/' + pid + '/members', 'POST'); await button(page, '移交项目负责人').waitFor();
  checks.push('Actual member role changes and removal use exact versions, identities and reasons, then reload authoritative membership.');

  await button(page, '移交项目负责人').click(); await select(page, '接任项目负责人', bob.display_name); beforeMember = await request(alice.token, '/projects/' + pid);
  const transfer = await submit(page, '确认移交项目负责人', '/projects/' + pid, 'PATCH'); assert.deepEqual(transfer.body, { expected_version: beforeMember.version, owner_id: bob.id }); await button(page, '移交项目负责人').waitFor();
  const transferred = await request(alice.token, '/projects/' + pid + '/members'); assert.equal(transferred.memberships.find(v => v.user_id === bob.id).project_role, 'owner'); assert.equal(transferred.memberships.find(v => v.user_id === me.user.id).project_role, 'manager'); await layout(page, 'project-members');
  checks.push('Owner handover uses a distinct command, returns the actual new owner and retains the former owner as manager.');

  const roles = await request(admin.token, '/roles'), managerRole = roles.items.find(v => v.key === 'project_manager'); assert.ok(managerRole);
  await request(admin.token, '/role-bindings', 'POST', { user_id: bob.id, role_id: managerRole.id, scope_type: 'project', scope_id: pid, classification_ceiling: 'project_restricted', reason: 'Give the successor explicit business actions for this exact project' });
  const successor = await login(fixture.bob), current = await request(successor.token, '/projects/' + pid);
  await request(successor.token, '/projects/' + pid + '/members', 'POST', { expected_version: current.version, user_id: me.user.id, action: 'set', project_role: 'viewer', reason: 'Verify real scope restriction and old-login invalidation' });
  assert.equal((await response(alice.token, '/me')).status, 401);
  const viewer = await login(fixture.alice); await navigate(viewer.page, '/pages/detail/index?kind=project&id=' + pid); await heading(viewer.page, 'Active mobile project'); await screen(viewer.page).getByText('项目范围', { exact: true }).waitFor(); assert.equal(await button(viewer.page, '编辑项目资料').count(), 0);
  await screen(viewer.page).locator('.cp-row-title').getByText('项目成员', { exact: true }).click(); await heading(viewer.page, '项目成员'); await screen(viewer.page).getByText('当前成员', { exact: true }).waitFor(); assert.equal(await button(viewer.page, '配置或移除项目成员').count(), 0);
  await navigate(viewer.page, '/pages/project/index?id=' + pid + '&section=setup'); await screen(viewer.page).getByText('当前权限、项目角色或阶段不允许立项或编辑。', { exact: true }).waitFor(); assert.equal(await button(viewer.page, '确认保存项目资料').count(), 0);
  checks.push('Membership restriction invalidates the old login; a fresh real MFA viewer session has no editing, membership or deep-link write form.');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) { failure = String(error.stack ?? error); process.stderr.write(failure + '\n'); }
finally {
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && !cleanupErrors.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, failure, owned_browser_closed: closed, browser_contexts_closed: contexts.length, real_native_api: true, real_model: false, real_cloud: false, real_device: false };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
