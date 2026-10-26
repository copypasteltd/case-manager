import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { recordViewport } from './project_change_recording.mjs';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-changes-browser');
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true);
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


async function mfa(page, account) {
  while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
  const step = Math.floor(Date.now() / 30000); await field(page, '验证器六位验证码').fill(otp(account.mfa_secret, step));
  await submit(page, '确认本人操作', '/auth/mfa/step-up', 'POST'); account.last_step = step;
  await screen(page).getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  assert.equal(await field(page, '验证器六位验证码').count(), 0);
}

const acceptedBasis = '我明确采用以上项目版本与该币种完整预算作为本次依据';
const checkedProposal = '我已核对完整提案、预算处理方式、原因与准确文件版本';
const checkbox = (page, name) => screen(page).getByRole('checkbox', { name, exact: true });
function done(message) { checks.push(message); console.log('Completed actual flow ' + checks.length + ': ' + message); }
async function image(page, name, caption) {
  await stable(page); const file = path.join(app, '../docs/media/screenshots/' + name + '.png');
  await page.screenshot({ path: file, fullPage: false });
  screenshots.push({ file: 'docs/media/screenshots/' + name + '.png', caption, sha256: createHash('sha256').update(await readFile(file)).digest('hex'), synthetic_data: true, real_device: false });
}
async function openChange(page, id) {
  const url = '/pages/finance/index?kind=change&id=' + id;
  if (await button(page, '查看当前业务资料').count()) await button(page, '查看当前业务资料').click();
  else if (new URL(page.url()).hash === '#' + url) await button(page, '重新读取项目变更').click(); else await navigate(page, url);
  await heading(page, '项目变更资料'); await button(page, '查看完整提案修订历史').waitFor(); await stable(page);
}
async function editor(page, pid, id) {
  const url = '/pages/project/index?section=change-form&id=' + pid + (id ? '&change_id=' + id : '');
  if (id) { await openChange(page, id); await button(page, '编辑完整变更提案').click(); }
  else if (await button(page, '提出完整预算调整').count()) await button(page, '提出完整预算调整').click();
  else { await button(page, '返回项目变更列表').click(); await button(page, '新建完整变更提案').click(); }
  await heading(page, id ? '编辑项目变更' : '新建项目变更');
  if (!id) await field(page, '变更币种').fill('HKD');
  await button(page, '读取项目与预算基线').click(); await screen(page).getByText('本次拟采用的完整基线', { exact: true }).waitFor();
  assert.equal(await checkbox(page, acceptedBasis).isChecked(), false);
  return url;
}
async function chooseFile(page) {
  await screen(page).getByRole('button').filter({ hasText: fixture.file_name }).first().click();
  await button(page, '选择固定版本 ' + fixture.file_seq).click();
  await screen(page).getByText('已选择的固定版本 · 1/20', { exact: true }).waitFor();
}
async function details(page, title) {
  await field(page, '变更名称').fill(title); await field(page, '变更类别').fill('synthetic-control');
  await field(page, '完整拟实施范围').fill('合成演示：完整范围，包含资料整理与明确验收成果。');
  await field(page, '范围影响说明').fill('合成演示：补充已核对的交付范围。'); await field(page, '时间影响说明').fill('合成演示：保留原计划，日期留空须明确清除。');
  await field(page, '提案保存原因').fill('合成演示：人工核对完整提案与固定依据。');
}
async function save(page, pid, id, status = id ? 200 : 201) {
  await checkbox(page, acceptedBasis).check(); await checkbox(page, checkedProposal).check();
  const result = await submit(page, id ? '保存新的变更修订' : '保存项目变更草稿', id ? '/changes/' + id : '/projects/' + pid + '/changes', id ? 'PATCH' : 'POST', status);
  if (status < 300) { await heading(page, '项目变更资料'); await screen(page).getByText(new RegExp('^变更版本 ' + result.value.version + ' ·')).waitFor(); }
  return result;
}
async function prepare(page, c) {
  await openChange(page, c.id); await button(page, '准备项目变更申请').click();
  await field(page, '准备变更申请原因').fill('合成验收：固定完整提案、预算基线与正式版本。');
  await checkbox(page, '我已核对以上完整提案、项目预算基线与固定文件').check();
  const result = await submit(page, '保存变更申请草稿', '/changes/' + c.id + '/submit', 'POST', 201);
  await button(page, '核对并送审').waitFor(); return result.value;
}
async function approve(alice, bob, q) {
  assert.ok(new URL(alice.page.url()).hash.endsWith('&id=' + q.id)); await button(alice.page, '核对并送审').click();
  await screen(alice.page).getByText('本次将固定的审批顺序', { exact: true }).waitFor();
  await field(alice.page, '送审依据').fill('合成验收：核对完整变更与独立审批顺序。');
  await checkbox(alice.page, '我已核对以上固定内容、附件与审批顺序').check();
  await submit(alice.page, '确认以此内容送审', '/requests/' + q.id + '/submit', 'POST');
  assert.equal(await button(alice.page, '确认同意当前修订').count(), 0);
  await navigate(bob.page, '/pages/home/index'); await heading(bob.page, '工作从这里开始');
  await screen(bob.page).getByRole('button').filter({ hasText: '待我审批' }).first().click();
  await screen(bob.page).getByRole('button').filter({ hasText: q.title }).first().click(); await button(bob.page, '确认同意当前修订').waitFor();
  const actual = await request(bob.token, '/requests/' + q.id), step = actual.steps.find(s => s.can_decide); assert.ok(step);
  await field(bob.page, '变更或决定原因').fill('合成验收：独立核对准确范围、日期、预算及固定附件。');
  await submit(bob.page, '确认同意当前修订', '/approval-steps/' + step.id + '/decisions', 'POST');
  const approved = await request(alice.token, '/requests/' + q.id); assert.equal(approved.state, 'approved'); return approved;
}
async function implement(alice, c) {
  await openChange(alice.page, c.id); await button(alice.page, '核对并实施已批准变更').click();
  await screen(alice.page).getByText(/已重新读取准确批准申请与当前基线/).waitFor();
  assert.equal(await button(alice.page, '确认实施准确批准变更').isDisabled(), true);
  await field(alice.page, '明确实施变更原因').fill('合成验收：明确实施此批准修订，分别核实范围与预算。');
  await select(alice.page, '范围与日期核实', '已核对完整范围与日期变化'); await select(alice.page, '预算影响核实', '已核对该币种完整预算与影响');
  assert.equal(await button(alice.page, '确认实施准确批准变更').isDisabled(), true);
  await mfa(alice.page, fixture.alice);
  const result = await submit(alice.page, '确认实施准确批准变更', '/changes/' + c.id + '/implement', 'POST');
  await screen(alice.page).getByText('实际实施事实', { exact: true }).waitFor();
  assert.equal(await field(alice.page, '明确实施变更原因').count(), 0);
  const head = await request(alice.token, '/changes/' + c.id); assert.equal(head.version, result.value.version); assert.equal(head.state, 'implemented'); return head;
}

try {
  assert.equal(fixture.project_change_fixture, true); assert.ok(fixture.file_version_id);
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), bob = await login(fixture.bob), admin = await login(fixture.administrator), page = alice.page; mainPage = page;
  const me = await request(alice.token, '/me'), other = await request(bob.token, '/me'), pid = fixture.project_id;
  for (const action of ['change.create', 'change.update', 'change.submit', 'change.implement', 'budget.read']) assert.ok(me.actions.includes(action));
  await request(admin.token, '/workflow-definitions', 'POST', { expected_version: 0, key: 'synthetic-mobile-change', name: '移动变更独立核对制度', kind: 'change', category: 'synthetic-control', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true,
    steps: [{ title: '独立核对完整项目与预算', assigned_user_id: other.user.id, requires_mfa: false }], reason: '隔离移动端验收，不涉及真实公司制度。' });
  let project = await request(alice.token, '/projects/' + pid);
  if (project.state === 'draft') project = await request(alice.token, '/projects/' + pid + '/transition', 'POST', { expected_version: project.version, action: 'activate', reason: 'Actual isolated project change review' });
  const original = structuredClone(project);
  await navigate(page, '/pages/project/index?section=changes&id=' + pid); await button(page, '新建完整变更提案').click(); await heading(page, '新建项目变更');
  assert.equal(await field(page, '变更币种').inputValue(), ''); await field(page, '变更币种').fill('HKD');
  await button(page, '读取项目与预算基线').click(); await screen(page).getByText('完整变更提案', { exact: true }).waitFor();
  assert.equal(await checkbox(page, acceptedBasis).isChecked(), false); assert.equal(await button(page, '保存项目变更草稿').isDisabled(), true);
  recordings.push(await recordViewport(page, app, 'mobile-project-change-workflow', async () => {
    await details(page, '移动变更 · 完整范围与预算'); await page.waitForTimeout(800);
    await select(page, '该币种预算处理', '完整替换该币种预算'); await button(page, '添加预算明细').click();
    await field(page, '预算类别 1').fill('资料整理'); await field(page, '预算开始日期 1').fill('2026-10-01'); await field(page, '预算结束日期 1').fill('2026-10-31'); await field(page, '预算原币金额 1').fill('100.01');
    await page.waitForTimeout(1000); await chooseFile(page); await checkbox(page, acceptedBasis).check(); await checkbox(page, checkedProposal).check();
    await page.waitForTimeout(700); const saved = await submit(page, '保存项目变更草稿', '/projects/' + pid + '/changes', 'POST', 201);
    await heading(page, '项目变更资料'); await screen(page).getByText(new RegExp('^变更版本 ' + saved.value.version + ' ·')).waitFor();
  }));
  let c = (await request(alice.token, '/projects/' + pid + '/changes?limit=20')).items.find(v => v.revision.content.title === '移动变更 · 完整范围与预算'); assert.ok(c);
  assert.equal(c.revision.attachments[0].version_id, fixture.file_version_id); assert.equal(c.revision.content.budget_lines[0].amount, '100.010000');
  assert.equal((await request(alice.token, '/projects/' + pid)).version, original.version);
  assert.equal((await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD')).budget_id, null);
  const drafts = await request(alice.token, '/projects/' + pid + '/budgets?state=draft&currency=HKD'); assert.ok(drafts.items.some(b => b.id === c.revision.target_budget_id));
  done('A real mobile proposal stores complete scope, exact decimal budget and an explicitly chosen historical file; project/current budget remain unchanged.');

  const first = structuredClone(c), q = await prepare(page, c);
  assert.equal(q.state, 'draft'); assert.equal(q.payload.kind, 'change'); assert.equal(q.payload.business_revision_id, first.current_revision_id);
  await editor(page, pid, c.id); await field(page, '变更名称').fill('移动变更 · 修订后的完整范围与预算'); await field(page, '提案保存原因').fill('合成验收：修订完整提案后重新固定申请。');
  await field(page, '预算原币金额 1').fill('120.01');
  await layout(page, 'change-editor'); await field(page, '完整拟实施范围').scrollIntoViewIfNeeded(); await image(page, 'mobile-change-editor', 'Actual shared mobile editor with explicit full proposal and per-currency replacement.');
  const edited = await save(page, pid, c.id); c = await request(alice.token, '/changes/' + c.id); assert.equal(c.current_revision_id, edited.value.revision_id); assert.equal(c.revision.seq, 2);
  assert.equal((await request(alice.token, '/changes/' + c.id + '/revisions/' + first.current_revision_id)).content.budget_lines[0].amount, '100.010000');
  await button(page, '查看原申请与独立审批').click(); await button(page, '更新申请草稿').click();
  await field(page, '更新变更申请依据').fill('合成验收：明确采用新修订与原固定基线。'); await checkbox(page, '我已核对本次将采用的完整变更修订与预算基线').check();
  const revised = await submit(page, '确认更新变更申请草稿', '/requests/' + q.id + '/revisions', 'POST');
  assert.notEqual(revised.value.current_revision_id, q.current_revision_id); assert.equal(revised.value.payload.business_revision_id, c.current_revision_id);
  done('Editing appends an immutable revision; the original request is explicitly revised to the new full proposal without silent resubmission or inherited approval.');

  await approve(alice, bob, revised.value); c = await request(alice.token, '/changes/' + c.id); assert.equal(c.state, 'approved');
  assert.equal((await request(alice.token, '/projects/' + pid)).version, original.version);
  assert.equal((await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD')).budget_id, null);
  await openChange(page, c.id); await layout(page, 'approved-change'); await image(page, 'mobile-change-approved', 'Actual independent approval still requires separate implementation.');
  done('Original preview/submission and an independent reviewer approve the exact revised payload; approval itself changes neither project nor current budget.');
  c = await implement(alice, c); project = await request(alice.token, '/projects/' + pid);
  let b = await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD'); assert.equal(b.budget_id, c.revision.target_budget_id); assert.equal(b.budget_total, '120.010000'); assert.equal(project.version, original.version + 1);
  const firstApprovedBudget = b.budget_id;
  done('Separate human scope/budget attestations and fresh interactive MFA implement the exact approved revision; actual project, implementation record and current budget are reconciled.');

  await editor(page, pid); await details(page, '移动变更 · 保持预算'); await chooseFile(page);
  const keepReceipt = await save(page, pid); let keep = await request(alice.token, '/changes/' + keepReceipt.value.id); assert.equal(keep.revision.content.budget_lines, null);
  await approve(alice, bob, await prepare(page, keep)); keep = await request(alice.token, '/changes/' + keep.id); await implement(alice, keep);
  b = await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD'); assert.equal(b.budget_id, firstApprovedBudget); assert.equal(b.budget_total, '120.010000');
  done('The explicit keep mode sends null and a separate approved/implemented change retains the exact original budget identity and total.');

  await editor(page, pid); await details(page, '移动变更 · 明确清空预算'); await select(page, '该币种预算处理', '完整替换该币种预算'); await button(page, '删除预算明细 1').click(); await chooseFile(page);
  await screen(page).getByText(/本次提案将清空该币种预算明细/).waitFor(); const clearReceipt = await save(page, pid); let clear = await request(alice.token, '/changes/' + clearReceipt.value.id); assert.deepEqual(clear.revision.content.budget_lines, []);
  await approve(alice, bob, await prepare(page, clear)); clear = await request(alice.token, '/changes/' + clear.id); await implement(alice, clear);
  b = await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD'); assert.equal(b.budget_total, '0.000000'); assert.notEqual(b.budget_id, firstApprovedBudget);
  assert.ok((await request(alice.token, '/projects/' + pid + '/budgets?state=superseded&currency=HKD')).items.some(v => v.id === firstApprovedBudget));
  await button(page, '查看项目预算历史').click(); await field(page, '预算币种筛选').fill('HKD'); await select(page, '预算状态筛选', '当前批准预算');
  await submit(page, '筛选预算修订', '/projects/' + pid + '/budgets', 'GET');
  await layout(page, 'budget-revisions'); await image(page, 'mobile-change-budget-history', 'Actual current and historical budget revisions are read with currency/state filters.');
  done('Explicit empty replacement sends [], activates a zero-line budget only after approval and implementation, and retains the actual superseded predecessor in currency/state history.');

  await editor(page, pid); await details(page, '移动变更 · 结果待核对'); await checkbox(page, acceptedBasis).check(); await checkbox(page, checkedProposal).check();
  const createPath = '**/v1/projects/' + pid + '/changes'; let creates = 0;
  await page.route(createPath, async route => {
    if (route.request().method() !== 'POST') return route.continue(); creates++; const actual = await route.fetch(), body = await actual.json();
    delete body.data.revision_id; await route.fulfill({ response: actual, json: body });
  });
  await submit(page, '保存项目变更草稿', '/projects/' + pid + '/changes', 'POST', 201);
  await screen(page).getByText(/操作返回的资料不完整/).waitFor(); assert.equal(await field(page, '变更名称').inputValue(), '移动变更 · 结果待核对'); assert.equal(await button(page, '保存项目变更草稿').isDisabled(), true); assert.equal(creates, 1);
  await page.unroute(createPath); const uncertain = (await request(alice.token, '/projects/' + pid + '/changes?limit=100')).items.filter(v => v.revision.content.title === '移动变更 · 结果待核对'); assert.equal(uncertain.length, 1);
  done('A deliberately incomplete real-write response keeps text/reason, blocks replay and never announces success; actual source reads confirm exactly one created proposal.');

  await button(page, '返回变更列表核对结果').click(); await screen(page).getByRole('button').filter({ hasText: '移动变更 · 结果待核对' }).first().click();
  await editor(page, pid, uncertain[0].id); await field(page, '变更名称').fill('移动变更 · 冲突后保留文字'); await field(page, '提案保存原因').fill('冲突后明确核对新基线。');
  project = await request(alice.token, '/projects/' + pid); await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, name: project.name + ' · 新基线' });
  await save(page, pid, uncertain[0].id, 409); assert.equal(await field(page, '变更名称').inputValue(), '移动变更 · 冲突后保留文字'); assert.equal(await button(page, '保存新的变更修订').isDisabled(), true);
  await button(page, '重新读取并核对基线').click(); await screen(page).getByText(project.name + ' · 新基线 · ' + (project.version + 1), { exact: true }).waitFor(); await stable(page);
  assert.equal(await checkbox(page, acceptedBasis).isChecked(), false); assert.equal(await checkbox(page, checkedProposal).isChecked(), false); assert.equal(await field(page, '变更名称').inputValue(), '移动变更 · 冲突后保留文字');
  await save(page, pid, uncertain[0].id); let historyHead = await request(alice.token, '/changes/' + uncertain[0].id);
  for (let i = 0; i < 11; i++) {
    const baseline = await request(alice.token, '/projects/' + pid + '/change-basis?currency=HKD');
    await request(alice.token, '/changes/' + historyHead.id, 'PATCH', { expected_version: historyHead.version, base_revision_id: historyHead.current_revision_id, base_project_version: baseline.project_version, base_budget_id: baseline.budget_id,
      content: { ...historyHead.revision.content, title: '移动历史核对 ' + (i + 1) }, reason: 'Actual immutable history pagination ' + i }); historyHead = await request(alice.token, '/changes/' + historyHead.id);
  }
  await openChange(page, historyHead.id); await button(page, '查看完整提案修订历史').click(); await button(page, '加载更早完整提案修订').click(); await button(page, '查阅完整提案修订 1').click();
  await screen(page).getByText('历史准确修订', { exact: true }).waitFor(); await screen(page).getByText('移动变更 · 结果待核对', { exact: true }).waitFor();
  done('A real project-version conflict preserves text and blocks commands; explicit reread resets basis/attestations, saves a fresh revision and paginates immutable historical content.');

  const deniedPath = '**/v1/changes/' + historyHead.id;
  await page.route(deniedPath, route => route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: { code: 'PERMISSION_DENIED', message: '合成权限收回响应' } }) }));
  await button(page, '重新读取项目变更').click(); await screen(page).getByText('合成权限收回响应', { exact: true }).waitFor();
  assert.equal(await button(page, '查看完整提案修订历史').count(), 0); assert.equal(await screen(page).getByText('历史准确修订', { exact: true }).count(), 0); await page.unroute(deniedPath);
  done('A controlled 403 refresh removes all cached change/history details and write controls instead of showing stale protected data.');

  await closeOwned(alice.context);
  const roles = await request(admin.token, '/roles'), role = roles.items.find(r => r.key === 'project_manager'); assert.ok(role);
  await request(admin.token, '/role-bindings', 'POST', { user_id: other.user.id, role_id: role.id, scope_type: 'project', scope_id: pid, classification_ceiling: 'project_restricted', reason: 'Actual independent manager for viewer review' });
  const successor = await login(fixture.bob); project = await request(successor.token, '/projects/' + pid);
  await request(alice.token, '/projects/' + pid, 'PATCH', { expected_version: project.version, owner_id: other.user.id });
  project = await request(successor.token, '/projects/' + pid); await request(successor.token, '/projects/' + pid + '/members', 'POST', { expected_version: project.version, user_id: me.user.id, action: 'set', project_role: 'viewer', reason: 'Actual read-only change reviewer' });
  const viewer = await login(fixture.alice); mainPage = viewer.page;
  await navigate(viewer.page, '/pages/project/index?section=change-form&id=' + pid); await screen(viewer.page).getByText('当前权限、项目阶段或变更状态只允许查阅。', { exact: true }).waitFor(); assert.equal(await button(viewer.page, '保存项目变更草稿').count(), 0);
  await openChange(viewer.page, historyHead.id); assert.equal(await button(viewer.page, '编辑完整变更提案').count(), 0); assert.equal(await button(viewer.page, '准备项目变更申请').count(), 0);
  await button(viewer.page, '返回项目变更列表').click(); await heading(viewer.page, '项目变更'); assert.equal(await button(viewer.page, '新建完整变更提案').count(), 0);

  done('An actual ownership transfer and viewer membership remove create/edit/prepare controls, including direct form routes, after fresh interactive login.');
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
