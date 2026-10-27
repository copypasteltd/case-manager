import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './task_creation_recording.mjs';

const mobile = process.argv.includes('--mobile');
const app = fileURLToPath(new URL('../', import.meta.url));
const media = path.join(app, '../docs/media/screenshots');

await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const select = (label, value, text) => mobile ? selectConcernMobile(page, label, text) : page.getByLabel(label, { exact: true }).selectOption(value);
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin);
  await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function request(route) {
    const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {});
    assert.equal(response.status(), 200, route); return (await response.json()).data;
  }
  const conversationURL = fixture.origin + (mobile ? '/#/pages/secretary/index?kind=conversation&id=' : '/conversations/') + fixture.conversation_id;
  await page.goto(conversationURL + (mobile ? '&run=' : '?run=') + fixture.completed_run_id);
  await root().getByText('本次更新已提交', { exact: true }).waitFor();
  const completed = await request('/runs/' + fixture.completed_run_id), project = await request('/projects/' + fixture.project_id);
  assert.equal(completed.mutation.tool, 'create_task'); assert.equal(completed.mutation.result.version, 1);
  assert.equal(completed.mutation.result.state, 'todo'); assert.equal(completed.mutation.result.review_required, true);
  assert.notEqual(completed.mutation.result.id, project.id); assert.ok(completed.mutation.operation_id);
  checks.push('Actual MFA login displays a committed subtask creation with a new child ID, todo state, version one and an explicit human review requirement.');
  await assertNoCredentials(page);
  const screenshotName = mobile ? 'secretary-task-creation-mobile' : 'secretary-task-creation-web';
  await root().getByText('任务当前进展', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' })); await stableMobile(page);
  await page.screenshot({ path: path.join(media, screenshotName + '-receipt.png'), fullPage: false });
  const parent = await request('/tasks/' + fixture.parent_task_id), milestone = await request('/milestones/' + fixture.milestone_id);
  const members = await request('/projects/' + project.id + '/members'), owner = members.items.find(m => m.id === completed.mutation.result.assignee_id); assert.ok(owner);
  async function taskIDs() {
    const ids = []; let cursor = '';
    do { const r = await request('/projects/' + project.id + '/tasks?limit=20' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : '')); ids.push(...r.items.map(t => t.id)); cursor = r.next_cursor ?? ''; } while (cursor);
    return ids.sort();
  }
  const before = await taskIDs(), sendPath = '/v1/conversations/' + fixture.conversation_id + '/runs'; let sends = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === sendPath && r.method() === 'POST') sends++; });
  await select('本次秘书任务', fixture.profile_id, '更新项目 · controlled-test');
  await button('选择准确业务更新').click(); await select('本次更新方式', 'create_task', '创建任务或子任务');
  await field('任务标题').fill('尚未选择验收要求'); await select('优先级', 'normal', '普通');
  await button('将这项明确更新加入请求').click();
  await root().getByText('请明确选择人工验收要求。', { exact: true }).waitFor(); assert.equal(sends, 0);
  checks.push('The visible form refuses to infer a missing human review decision and sends no run.');
  await select('需要人工验收', 'false', '无需人工验收'); await field('本次创建原因').fill('明确创建独立待分配任务');
  await button('将这项明确更新加入请求').click(); await button('移除本次更新').waitFor();
  await root().getByText(mobile ? '无需人工验收' : '需要人工验收 → 无需人工验收', { exact: true }).waitFor();
  assert.equal(sends, 0); await button('移除本次更新').click();
  checks.push('An independent unassigned task previews explicit null references/dates and false review without executing.');
  await button('选择准确业务更新').click(); await select('本次更新方式', 'create_task', '创建任务或子任务');
  const values = { title: (mobile ? '移动端' : 'Web') + '核对的交付资料子任务', body: '核对负责人、父任务、里程碑和验收要求后，整理首轮交付资料。', assignee_id: owner.id,
    parent_id: parent.id, milestone_id: milestone.id, start_at: '2027-03-05T10:00:00.123456+08:00', due_at: '2027-03-05T18:00:00.123457+08:00', priority: 'high', review_required: true };
  await field('任务标题').fill(values.title); await field('完整任务说明').fill(values.body); await select('任务负责人', owner.id, owner.display_name);
  if (mobile) {
    await button('选择父任务').click(); await root().getByText(parent.title, { exact: true }).click();
    await button('选择关联里程碑').click(); await root().getByText(milestone.title, { exact: true }).click();
  } else { await select('父任务', parent.id); await select('关联里程碑', milestone.id); }
  await select('开始时刻处理', 'set', '设置准确时刻'); await field('开始时刻').fill(values.start_at);
  await select('截止时刻处理', 'set', '设置准确时刻'); await field('截止时刻').fill('2027-03-05T10:00:00.123455+08:00');
  await select('优先级', 'high', '高'); await select('需要人工验收', 'true', '需要人工验收');
  const reason = '员工确认完整任务资料与人工验收要求'; await field('本次创建原因').fill(reason);
  await button('将这项明确更新加入请求').click(); await root().getByText('截止时刻不能早于开始时刻。', { exact: true }).waitFor(); assert.equal(sends, 0);
  await field('截止时刻').fill(values.due_at);
  checks.push('Actual member, parent task and milestone choices survive navigation; microsecond date inversion is rejected before a send.');
  await button('将这项明确更新加入请求').scrollIntoViewIfNeeded();
  const recordings = [await recordViewport(page, app, screenshotName + '-review', async () => {
    await button('将这项明确更新加入请求').click();
    await root().getByText('本次明确更新', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' })); await stableMobile(page);
  })];
  await page.screenshot({ path: path.join(media, screenshotName + '-preview.png'), fullPage: false });
  const layouts = [];
  for (const width of mobile ? [360, 390, 768] : [768, 1280, 1440]) {
    await page.setViewportSize({ width, height: mobile ? 900 : 1100 }); await stableMobile(page);
    const size = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth })); assert.ok(size.document <= size.viewport + 2); layouts.push({ width, ...size });
  }
  await page.setViewportSize(mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 }); assert.equal(sends, 0);
  checks.push('The full nine-field subtask authorization previews within three actual viewport widths and is captured from real browser pixels.');
  await field('本次消息内容').fill('仅创建本次完整核对的项目子任务，保留人工验收要求。');
  const [sent] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === sendPath && r.request().method() === 'POST'), button('确认发送本次请求').click()]);
  assert.equal(sent.status(), 202); const input = sent.request().postDataJSON(), run = (await sent.json()).data;
  assert.deepEqual(input.mutation, { tool: 'create_task', arguments: { id: project.id, expected_version: project.version, reason, ...values } });
  assert.equal(run.mutation.status, 'authorized'); assert.equal(run.mutation.operation_id, null);
  checks.push('The real authenticated send fixes the exact parent version and all nine fields without falsely claiming creation.');
  const csrf = mobile ? undefined : (await context.cookies()).find(c => c.name === 'cp_csrf')?.value;
  const replay = await context.request.post(origin + sendPath, { headers: { ...(mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': csrf }), 'Idempotency-Key': sent.request().headers()['idempotency-key'] }, data: input });
  assert.equal(replay.status(), 202); assert.equal((await replay.json()).data.id, run.id);
  if (mobile) await button('明确取消这个任务').click();
  await field('取消任务的原因').fill('结束隔离演示，保留实际已提交任务历史。');
  const [cancelled] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1/runs/' + run.id + '/cancel'), button('确认请求取消').click()]);
  assert.equal(cancelled.status(), 200); assert.equal((await request('/runs/' + run.id)).state, 'cancelled'); assert.equal(sends, 1); assert.deepEqual(await taskIDs(), before);
  checks.push('The exact resend reuses one run; cancelling before execution leaves the actual task collection unchanged.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ mobile, recordings, layouts, synthetic_data: true,
    scope: 'Actual native API and authenticated Web/Taro H5; controlled task creation fixture receipts; no provider, device or production acceptance.' }, null, 2) + '\n');
}, 'task-creation');
