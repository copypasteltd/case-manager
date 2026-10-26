import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './project_concern_recording.mjs';

const mobile = process.argv.includes('--mobile');
const app = fileURLToPath(new URL('../', import.meta.url));
const media = path.join(app, '../docs/media/screenshots');
const tools = { create_risk: '创建项目风险', update_risk: '修改风险资料与状态', create_issue: '创建项目问题', update_issue: '修改问题资料与状态' };

await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const select = (label, value, text) => mobile ? selectConcernMobile(page, label, text) : page.getByLabel(label, { exact: true }).selectOption(value);
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin);
  if (mobile) {
    await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
    await button('登录工作空间 ↗').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button('验证并继续 ↗').click();
    await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  } else {
    await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
    await button('登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button('验证并继续').click();
    await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  }
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function request(route) {
    const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {});
    assert.equal(response.status(), 200, route); return (await response.json()).data;
  }
  const conversationURL = fixture.origin + (mobile ? '/#/pages/secretary/index?kind=conversation&id=' : '/conversations/') + fixture.conversation_id;
  await page.goto(conversationURL + (mobile ? '&run=' : '?run=') + fixture.completed_run_id);
  await root().getByText('本次更新已提交', { exact: true }).waitFor();
  const completed = await request('/runs/' + fixture.completed_run_id), project = await request('/projects/' + fixture.project_id);
  assert.equal(completed.mutation.tool, 'create_risk'); assert.equal(completed.mutation.status, 'committed');
  assert.equal(completed.mutation.result.version, 1); assert.equal(completed.mutation.result.state, 'open');
  assert.notEqual(completed.mutation.result.id, project.id); assert.ok(completed.mutation.operation_id);
  checks.push('Actual MFA login reads a committed risk creation receipt with a new child ID and version one.');
  await assertNoCredentials(page);
  const screenshotName = mobile ? 'secretary-concerns-mobile' : 'secretary-concerns-web';
  await root().getByText('任务当前进展', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' }));
  await stableMobile(page);
  await page.screenshot({ path: path.join(media, screenshotName + '-receipt.png'), fullPage: false });

  const members = await request('/projects/' + project.id + '/members'), owner = members.items.find(item => item.status === 'active'); assert.ok(owner);
  async function all(kind) {
    const items = []; let cursor = '';
    do {
      const result = await request('/projects/' + project.id + '/' + kind + '?limit=20' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''));
      items.push(...result.items); cursor = result.next_cursor ?? '';
    } while (cursor);
    return items;
  }
  const risks = await all('risks'), issues = await all('issues');
  const risk = risks.find(item => item.state === 'open'), issue = issues.find(item => item.state === 'open'); assert.ok(risk && issue);
  const before = { risks: risks.map(item => item.id).sort(), issues: issues.map(item => item.id).sort() };
  let sends = 0;
  const sendPath = '/v1/conversations/' + fixture.conversation_id + '/runs';
  page.on('request', request => { if (new URL(request.url()).pathname === sendPath && request.method() === 'POST') sends++; });
  await select('本次秘书任务', fixture.profile_id, '更新项目 · controlled-test');
  let expected;
  const recordings = [], layouts = [];
  async function fill(tool) {
    const isRisk = tool.endsWith('_risk'), creating = tool.startsWith('create_'), target = creating ? project : isRisk ? risk : issue;
    await button('选择准确业务更新').click(); await select('本次更新方式', tool, tools[tool]);
    if (!creating) {
      if (mobile) {
        await button('选择本次更新' + (isRisk ? '风险' : '问题')).click();
        await root().getByText(target.title + ' · 待处理', { exact: true }).click();
      }
      else await select('本次更新' + (isRisk ? '风险' : '问题'), target.id);
    }
    await field('事项标题').waitFor();
    const values = { title: (mobile ? '移动端' : 'Web') + '核对的项目' + (isRisk ? '风险' : '问题'), description: '明确核对实际负责人、完整资料与当前版本。',
      owner_id: owner.id, severity: 'high', due_at: isRisk ? '2027-03-05T18:00:00.123456+08:00' : null,
      ...(isRisk ? { probability: 'medium', mitigation: '先检查供应时间，再记录责任人与处理时限。' } : { resolution: creating ? '' : '已核对实际处理结果，记录解决依据。' }),
      ...(!creating ? { state: isRisk ? 'realized' : 'resolved' } : {}) };
    await field('事项标题').fill(values.title); await field('完整说明').fill(values.description);
    await select('事项负责人', owner.id, owner.display_name); await select('严重程度', 'high', '高');
    await select('截止时刻处理', isRisk ? 'set' : 'clear', isRisk ? '设置准确时刻' : '明确不设置／清空');
    if (isRisk) {
      await field('事项截止时刻').fill(values.due_at); await select('发生概率', 'medium', '中'); await field('缓解措施').fill(values.mitigation);
    } else await field('解决说明').fill(values.resolution);
    if (!creating) await select('事项状态处理', values.state, isRisk ? '已发生并转成问题' : '已解决');
    const reason = '员工确认完整资料及本次处理原因'; await field('本次更新原因').fill(reason);
    if (!creating && isRisk) {
      assert.equal(await button('将这项明确更新加入请求').isEnabled(), false);
      if (mobile) await root().getByText('我已核对：本次风险发生会新增一个关联问题', { exact: true }).click();
      else await page.getByRole('checkbox', { name: '我已核对：本次风险发生会新增一个关联问题', exact: true }).check();
      checks.push('Risk realization requires a visible confirmation before its exact intent can be prepared.');
    }
    expected = { tool, arguments: { id: target.id, expected_version: target.version, reason, ...values } };
    return values;
  }
  for (const tool of ['create_risk', 'update_risk', 'update_issue', 'create_issue']) {
    const values = await fill(tool);
    if (tool === 'update_risk') {
      await button('将这项明确更新加入请求').scrollIntoViewIfNeeded();
      recordings.push(await recordViewport(page, app, mobile ? 'secretary-concerns-mobile-review' : 'secretary-concerns-web-review', async () => {
        await field('缓解措施').scrollIntoViewIfNeeded(); await stableMobile(page);
        await button('将这项明确更新加入请求').click();
        await root().getByText('本次明确更新', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' }));
        await stableMobile(page);
      }));
      await page.screenshot({ path: path.join(media, screenshotName + '-preview.png'), fullPage: false });
      for (const width of mobile ? [360, 390, 768] : [768, 1280, 1440]) {
        await page.setViewportSize({ width, height: mobile ? 900 : 1100 }); await stableMobile(page);
        const size = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth }));
        assert.ok(size.document <= size.viewport + 2, 'Concern preview must fit the viewport.'); layouts.push({ width, ...size });
      }
      await page.setViewportSize(mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 });
    } else await button('将这项明确更新加入请求').click();
    await button('移除本次更新').waitFor(); assert.equal(sends, 0);
    await root().getByText(mobile ? values.title : '事项标题 → ' + values.title, { exact: true }).waitFor();
    checks.push(tools[tool] + ': complete actual metadata and the correct project/child version are previewed without execution.');
    if (tool !== 'create_issue') await button('移除本次更新').click();
  }
  await field('本次消息内容').fill('仅执行本次明确选择的完整项目问题创建请求。');
  const [sent] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === sendPath && response.request().method() === 'POST'), button('确认发送本次请求').click()]);
  assert.equal(sent.status(), 202); const input = sent.request().postDataJSON(), run = (await sent.json()).data;
  assert.deepEqual(input.mutation, expected); assert.equal(run.mutation.status, 'authorized'); assert.equal(run.mutation.operation_id, null);
  checks.push('The real send fixes the parent version, complete issue fields and explicit empty date without claiming a business write.');
  const csrf = mobile ? undefined : (await context.cookies()).find(cookie => cookie.name === 'cp_csrf')?.value;
  const replay = await context.request.post(origin + sendPath, { headers: { ...(mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': csrf }), 'Idempotency-Key': sent.request().headers()['idempotency-key'] }, data: input });
  assert.equal(replay.status(), 202); assert.equal((await replay.json()).data.id, run.id);
  if (mobile) await button('明确取消这个任务').click();
  await field('取消任务的原因').fill('结束隔离表单演示，保留真实已提交历史。');
  const [cancelled] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/v1/runs/' + run.id + '/cancel'), button('确认请求取消').click()]);
  assert.equal(cancelled.status(), 200); assert.equal((await request('/runs/' + run.id)).state, 'cancelled');
  assert.equal(sends, 1); assert.deepEqual((await all('risks')).map(item => item.id).sort(), before.risks); assert.deepEqual((await all('issues')).map(item => item.id).sort(), before.issues);
  checks.push('The exact resend reuses one run; cancelling before execution leaves both actual concern collections unchanged.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ mobile, recordings, layouts, synthetic_data: true,
    scope: 'Actual native API and authenticated Web/Taro H5; controlled fixture receipts; no provider, device or production acceptance.' }, null, 2) + '\n');
}, 'project-concerns');
