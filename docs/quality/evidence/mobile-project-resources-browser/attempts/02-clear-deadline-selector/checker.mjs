import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-project-resources-browser');
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
let browser, mainPage, passed = false, failure;
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

const names = { milestone: '里程碑', risk: '风险', issue: '问题' }, plurals = { milestone: 'milestones', risk: 'risks', issue: 'issues' };
const actionNames = { achieve: '标记里程碑达成', miss: '标记里程碑错过', cancel: '取消里程碑', reopen: '重开里程碑', mitigating: '开始风险应对', closed: '关闭事项', realized: '风险已发生，转为问题', open: '重新打开事项', investigating: '开始或继续调查', resolved: '标记问题已解决' };
async function fresh(page, item) { await heading(page, item.title); await screen(page).getByText(new RegExp('^修订 ' + item.version + ' ·')).waitFor(); await stable(page); }
async function createForm(page, kind, pid) {
  if (await button(page, '查看所属项目').count()) await button(page, '查看所属项目').click();
  else await navigate(page, '/pages/detail/index?kind=project&id=' + pid);
  await heading(page, 'Actual mobile resource project');
  await screen(page).locator('.cp-row-title').getByText(names[kind], { exact: true }).click(); await heading(page, '项目' + names[kind]);
  await button(page, '新建' + names[kind]).click(); await heading(page, '新建' + names[kind]); await field(page, names[kind] + '标题').waitFor();
}
async function editForm(page, kind) { await button(page, '编辑' + names[kind] + '资料').click(); await heading(page, '编辑' + names[kind]); await field(page, names[kind] + '标题').waitFor(); }
async function savedDetail(page, item) { await button(page, '查看已保存事项').click(); await fresh(page, item); }
async function transition(page, kind, item, action, reason, resolution, expected = 200) {
  await button(page, actionNames[action]).click(); await field(page, '事项变更原因').fill(reason);
  if (resolution !== undefined) await screen(page).locator('textarea[aria-label="本次解决说明"]').fill(resolution);
  const route = '/' + plurals[kind] + '/' + item.id + (kind === 'milestone' ? '/transition' : '');
  const changed = await submit(page, '确认' + actionNames[action], route, kind === 'milestone' ? 'POST' : 'PATCH', expected);
  if (expected === 200) await fresh(page, changed.value);
  return changed;
}

try {
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), admin = await login(fixture.administrator), page = alice.page;
  mainPage = page;
  const me = await request(alice.token, '/me');
  for (const action of ['milestone.create', 'milestone.update', 'risk.create', 'risk.update', 'issue.create', 'issue.update']) assert.ok(me.actions.includes(action));
  let project = await request(alice.token, '/projects', 'POST', { name: 'Actual mobile resource project', type: 'general', scope: 'Milestone, risk and issue lifecycle.', owner_id: me.user.id, classification: 'project_restricted' });
  const pid = project.id, directory = await request(alice.token, '/projects/' + pid + '/member-candidates'), bob = directory.items.find(v => v.display_name === 'bob'); assert.ok(bob);
  project = await request(alice.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: bob.id, action: 'set', project_role: 'member', reason: 'Actual successor and responsibility candidate' });
  project = await request(alice.token, '/projects/' + pid + '/transition', 'POST', { expected_version: project.version, action: 'activate', reason: 'Begin actual lifecycle review' });

  await createForm(page, 'milestone', pid); let creates = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === '/v1/projects/' + pid + '/milestones' && r.method() === 'POST') creates++; });
  await field(page, '里程碑标题').fill('Actual mobile checkpoint'); await select(page, '事项负责人', bob.display_name);
  await field(page, '目标时间（香港时间）').fill('2026-02-29 18:00'); await button(page, '确认保存里程碑').click(); await screen(page).getByText(/日期时间不存在/).waitFor(); assert.equal(creates, 0);
  await field(page, '目标时间（香港时间）').fill('2020-01-02 18:00:00.123456'); await layout(page, 'milestone-creation');
  let saved = await submit(page, '确认保存里程碑', '/projects/' + pid + '/milestones', 'POST', 201), milestone = saved.value;
  assert.deepEqual(saved.body, { title: 'Actual mobile checkpoint', owner_id: bob.id, due_at: '2020-01-02T10:00:00.123456Z' }); assert.equal(milestone.version, 1); assert.equal(milestone.project_id, pid); assert.equal(creates, 1);
  await savedDetail(page, milestone); assert.equal((await request(alice.token, '/milestones/' + milestone.id)).overdue, true);
  milestone = (await transition(page, 'milestone', milestone, 'miss', 'The actual target was missed')).value;
  await editForm(page, 'milestone'); await field(page, '里程碑标题').fill('Revised mobile checkpoint'); await select(page, '事项负责人', me.user.display_name); await field(page, '目标时间（香港时间）').fill('2028-02-29 18:00:00.123456');
  saved = await submit(page, '确认保存里程碑', '/milestones/' + milestone.id, 'PATCH'); assert.equal(saved.body.expected_version, milestone.version); assert.equal(saved.body.owner_id, me.user.id); assert.equal(saved.body.due_at, '2028-02-29T10:00:00.123456Z'); milestone = saved.value;
  await savedDetail(page, milestone);
  checks.push('Actual milestone creation rejects invalid dates without a request; exact Hong Kong time, owner, microseconds, missed state and versioned editing persist.');

  await transition(page, 'milestone', milestone, 'achieve', 'Verify actual completed evidence', undefined, 422); await screen(page).getByText(/须有已完成的关联任务/).waitFor(); assert.equal((await request(alice.token, '/milestones/' + milestone.id)).version, milestone.version);
  let task = await request(alice.token, '/projects/' + pid + '/tasks', 'POST', { title: 'Actual completed milestone task', body: 'Independent real completion evidence.', assignee_id: me.user.id, milestone_id: milestone.id, priority: 'normal', review_required: false });
  for (const action of ['start', 'complete']) task = await request(alice.token, '/tasks/' + task.id + '/transition', 'POST', { expected_version: task.version, action, reason: 'Actual original task lifecycle' });
  assert.equal(task.state, 'done');
  saved = await submit(page, '确认标记里程碑达成', '/milestones/' + milestone.id + '/transition', 'POST'); milestone = saved.value; await fresh(page, milestone); assert.equal(milestone.state, 'achieved'); assert.equal(await button(page, '编辑里程碑资料').count(), 0);
  milestone = (await transition(page, 'milestone', milestone, 'reopen', 'Review next milestone evidence')).value;
  milestone = (await transition(page, 'milestone', milestone, 'cancel', 'Cancel actual checkpoint')).value;
  milestone = (await transition(page, 'milestone', milestone, 'reopen', 'Continue original checkpoint')).value;
  checks.push('Milestone achievement refuses missing evidence without mutation, succeeds after an original completed associated task and supports exact reopen/cancel commands.');

  await createForm(page, 'risk', pid); await field(page, '风险标题').fill('Actual mobile risk'); await screen(page).locator('textarea[aria-label="情况说明"]').fill('Original actual risk description.');
  await select(page, '事项负责人', bob.display_name); await select(page, '影响程度', '严重'); await field(page, '截止时间（香港时间）').fill('2028-03-01 09:30'); await screen(page).locator('textarea[aria-label="应对措施"]').fill('Initial actual mitigation.');
  saved = await submit(page, '确认保存风险', '/projects/' + pid + '/risks', 'POST', 201); let risk = saved.value;
  assert.deepEqual(saved.body, { title: 'Actual mobile risk', owner_id: bob.id, due_at: '2028-03-01T01:30:00.000000Z', description: 'Original actual risk description.', severity: 'critical', mitigation: 'Initial actual mitigation.' }); assert.equal(risk.version, 1); assert.equal(risk.probability, undefined);
  await savedDetail(page, risk); await screen(page).getByText('Initial actual mitigation.', { exact: true }).waitFor(); await editForm(page, 'risk');
  let riskPatches = 0; page.on('request', r => { if (new URL(r.url()).pathname === '/v1/risks/' + risk.id && r.method() === 'PATCH') riskPatches++; });
  await field(page, '截止时间（香港时间）').fill(''); await button(page, '确认保存风险').click(); await screen(page).getByText(/暂不支持清空/).waitFor(); assert.equal(riskPatches, 0);
  await field(page, '截止时间（香港时间）').fill('2028-03-01 09:30'); await screen(page).locator('textarea[aria-label="应对措施"]').fill('Preserve this unsubmitted mitigation.');
  const concurrent = await request(alice.token, '/risks/' + risk.id, 'PATCH', { expected_version: risk.version, severity: 'high' });
  await submit(page, '确认保存风险', '/risks/' + risk.id, 'PATCH', 409); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(await screen(page).locator('textarea[aria-label="应对措施"]').inputValue(), 'Preserve this unsubmitted mitigation.');
  await button(page, '重新读取当前权限与事项').click(); await screen(page).locator('textarea[aria-label="应对措施"]').waitFor(); assert.equal(await screen(page).locator('textarea[aria-label="应对措施"]').inputValue(), concurrent.mitigation);
  await screen(page).locator('textarea[aria-label="应对措施"]').fill('Actual revised mitigation.'); saved = await submit(page, '确认保存风险', '/risks/' + risk.id, 'PATCH'); risk = saved.value; assert.equal(saved.body.expected_version, concurrent.version); assert.equal(risk.severity, 'high'); await savedDetail(page, risk); await layout(page, 'risk-follow-up');
  risk = (await transition(page, 'risk', risk, 'mitigating', 'Begin actual mitigation')).value;
  checks.push('Actual risk creation and revision preserve optional probability, show mitigation, prevent false date clearing and retain text on a real concurrent 409.');

  await button(page, actionNames.realized).click(); await field(page, '事项变更原因').fill('This risk actually occurred; create the original related issue.');
  let realized, realizations = 0;
  await page.route('**/v1/risks/' + risk.id, async route => {
    if (route.request().method() !== 'PATCH' || route.request().postDataJSON()?.state !== 'realized') return route.continue();
    realizations++; const r = await route.fetch(); realized = { status: r.status(), value: (await r.json()).data }; await route.abort('failed');
  });
  await button(page, '确认' + actionNames.realized).click(); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(realized.status, 200); assert.equal(realizations, 1); assert.equal(await button(page, '确认' + actionNames.realized).isDisabled(), true);
  await page.unroute('**/v1/risks/' + risk.id); risk = await request(alice.token, '/risks/' + risk.id); assert.equal(risk.state, 'realized'); assert.equal(risk.version, realized.value.version); assert.ok(risk.issue_id); assert.equal(risk.probability, undefined);
  const issues = await request(alice.token, '/projects/' + pid + '/issues?limit=100'); assert.equal(issues.items.length, 1); assert.equal(issues.items[0].id, risk.issue_id);
  await button(page, '刷新资料').click(); await fresh(page, risk); assert.equal(await button(page, '编辑风险资料').count(), 0); assert.equal(await button(page, actionNames.realized).count(), 0);
  await button(page, '查看已发生的问题').click(); let issue = await request(alice.token, '/issues/' + risk.issue_id); await fresh(page, issue); assert.equal(issue.owner_id, risk.owner_id); assert.equal(issue.description, risk.description); assert.equal(issue.severity, risk.severity);
  checks.push('A committed risk realization with a lost response blocks replay, creates exactly one real issue and exposes its actual link even when probability is unset.');

  issue = (await transition(page, 'issue', issue, 'investigating', 'Investigate the original risk occurrence')).value;
  await button(page, actionNames.resolved).click(); await field(page, '事项变更原因').fill('Verify actual corrective action'); let resolves = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === '/v1/issues/' + issue.id && r.method() === 'PATCH') resolves++; });
  await button(page, '确认' + actionNames.resolved).click(); await screen(page).getByText(/解决或关闭问题前，请填写实际解决说明/).waitFor(); assert.equal(resolves, 0);
  await screen(page).locator('textarea[aria-label="本次解决说明"]').fill('Actually verified corrective action and result.');
  saved = await submit(page, '确认' + actionNames.resolved, '/issues/' + issue.id, 'PATCH'); issue = saved.value; assert.equal(saved.body.expected_version, issue.version - 1); assert.equal(issue.resolution, 'Actually verified corrective action and result.'); await fresh(page, issue);
  issue = (await transition(page, 'issue', issue, 'closed', 'Close after actual verification', issue.resolution)).value; assert.equal(await button(page, '编辑问题资料').count(), 0);
  issue = (await transition(page, 'issue', issue, 'open', 'A new fact requires reopening')).value; assert.equal(issue.state, 'open'); assert.equal(issue.resolution, 'Actually verified corrective action and result.');
  checks.push('The actual linked issue follows investigation, resolution, closure and reopening; empty resolution sends no request and the original solution persists.');

  await createForm(page, 'issue', pid); await field(page, '问题标题').fill('Direct actual mobile issue'); await screen(page).locator('textarea[aria-label="情况说明"]').fill('Independent observed issue.'); await layout(page, 'issue-creation');
  saved = await submit(page, '确认保存问题', '/projects/' + pid + '/issues', 'POST', 201); let direct = saved.value;
  assert.equal('due_at' in saved.body, false); assert.equal('state' in saved.body, false); assert.equal(direct.state, 'open'); await savedDetail(page, direct); await editForm(page, 'issue');
  await screen(page).locator('textarea[aria-label="解决说明"]').fill('Direct issue facts and proposed solution.'); saved = await submit(page, '确认保存问题', '/issues/' + direct.id, 'PATCH'); direct = saved.value; await savedDetail(page, direct);
  assert.equal(direct.resolution, 'Direct issue facts and proposed solution.'); assert.equal(direct.state, 'open');
  checks.push('Direct issue creation and editing use the original commands, preserve absent deadlines and keep editing separate from an explicit state decision.');

  const foreign = await request(alice.token, '/projects', 'POST', { name: 'Other actual project', type: 'general', scope: 'Foreign record guard.', owner_id: me.user.id, classification: 'project_restricted' });
  const foreignRisk = await request(alice.token, '/projects/' + foreign.id + '/risks', 'POST', { title: 'Actual other project risk', description: 'Must not attach to the first project.', owner_id: me.user.id, severity: 'low' });
  await navigate(page, '/pages/project/index?section=resource-form&kind=risk&id=' + pid + '&resource_id=' + foreignRisk.id); await screen(page).getByText('事项与所属项目不匹配，请重新打开。', { exact: true }).waitFor(); assert.equal(await button(page, '确认保存风险').count(), 0); assert.equal((await request(alice.token, '/risks/' + foreignRisk.id)).version, 1);
  checks.push('A real accessible item from another project cannot open an editing form under the wrong project and remains unchanged.');

  const roles = await request(admin.token, '/roles'), managerRole = roles.items.find(v => v.key === 'project_manager'); assert.ok(managerRole);
  await request(admin.token, '/role-bindings', 'POST', { user_id: bob.id, role_id: managerRole.id, scope_type: 'project', scope_id: pid, classification_ceiling: 'project_restricted', reason: 'Explicit independent manager for exact-project restriction review' });
  const successor = await login(fixture.bob); project = await request(successor.token, '/projects/' + pid);
  // Transfer first: the owner cannot be downgraded by the member command.
  project = await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, owner_id: bob.id });
  await request(successor.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: me.user.id, action: 'set', project_role: 'viewer', reason: 'Verify actual fresh viewer and stale-login boundaries' });
  assert.equal((await response(alice.token, '/me')).status, 401); const viewer = await login(fixture.alice);
  for (const [kind, item] of [['milestone', milestone], ['risk', risk], ['issue', direct]]) {
    await navigate(viewer.page, '/pages/project/index?id=' + pid + '&section=' + plurals[kind]); await heading(viewer.page, '项目' + names[kind]); await screen(viewer.page).locator('.cp-row-title').getByText(item.title, { exact: true }).waitFor(); assert.equal(await button(viewer.page, '新建' + names[kind]).count(), 0);
    const parentRead = viewer.page.waitForResponse(r => new URL(r.url()).pathname === '/v1/projects/' + pid && r.request().method() === 'GET'), membershipRead = viewer.page.waitForResponse(r => new URL(r.url()).pathname === '/v1/projects/' + pid + '/members' && r.request().method() === 'GET');
    const [parentResponse, membershipResponse] = await Promise.all([parentRead, membershipRead, (async () => { await navigate(viewer.page, '/pages/detail/index?kind=' + kind + '&id=' + item.id); await fresh(viewer.page, item); })()]);
    assert.equal(parentResponse.status(), 200); const membership = await membershipResponse.json(); assert.equal(membership.data.memberships.find(v => v.user_id === me.user.id).project_role, 'viewer');
    await screen(viewer.page).getByText('正在读取资料…', { exact: true }).waitFor({ state: 'hidden' }); await stable(viewer.page); assert.equal(await button(viewer.page, '编辑' + names[kind] + '资料').count(), 0);
    await navigate(viewer.page, '/pages/project/index?section=resource-form&kind=' + kind + '&id=' + pid); await screen(viewer.page).getByText('当前权限、项目角色或事项阶段不允许新建或编辑。', { exact: true }).waitFor(); assert.equal(await button(viewer.page, '确认保存' + names[kind]).count(), 0);
  }
  project = await request(successor.token, '/projects/' + pid); await request(successor.token, '/projects/' + pid + '/transition', 'POST', { expected_version: project.version, action: 'cancel', reason: 'Read-only original project lifecycle verification' });
  for (const kind of ['milestone', 'risk', 'issue']) {
    await navigate(successor.page, '/pages/project/index?section=resource-form&kind=' + kind + '&id=' + pid); await screen(successor.page).getByText('当前权限、项目角色或事项阶段不允许新建或编辑。', { exact: true }).waitFor(); assert.equal(await button(successor.page, '确认保存' + names[kind]).count(), 0);
  }
  checks.push('Real membership restriction invalidates the old login; fresh MFA viewer and cancelled-project manager cannot create, edit or follow state-changing deep links.');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  failure = String(error.stack ?? error);
  if (mainPage && !mainPage.isClosed()) {
    try {
      const current = { route: new URL(mainPage.url()).hash, titles: await screen(mainPage).locator('.cp-title').allTextContents(), actions: await screen(mainPage).getByRole('button').allTextContents() };
      failure += '\nActive page diagnostics: ' + JSON.stringify(current);
      await mainPage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true, mask: [mainPage.locator('input[type="password"],.cp-code')] });
    } catch { /* Preserve the original failure even if its page has disappeared. */ }
  }
  process.stderr.write(failure + '\n');
}
finally {
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && !cleanupErrors.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, failure, owned_browser_closed: closed, browser_contexts_closed: contexts.length, real_native_api: true, real_model: false, real_cloud: false, real_device: false };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
