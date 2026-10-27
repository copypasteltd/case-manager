import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './task_creation_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const media = path.join(app, '../docs/media/screenshots');
// Use the visible wheel rows; every click settles before inspecting its actual transform.
async function selectDependencyMobile(page, label, text) {
  const picker = mobileScreen(page).locator('taro-picker-core[aria-label="' + label + '"]');
  await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await page.waitForTimeout(450); await stableMobile(page);
  const values = await overlay.locator('.weui-picker__item').allTextContents(), desired = values.indexOf(text);
  assert.ok(desired >= 0, label + ' actual option');
  const selected = () => group.evaluate(node => {
    const height = node.querySelector('.weui-picker__item').getBoundingClientRect().height;
    const translate = new DOMMatrixReadOnly(getComputedStyle(node.querySelector('.weui-picker__content')).transform).m42;
    return Math.round(((node.getBoundingClientRect().height - height) / 2 - translate) / height);
  });
  for (let attempt = 0; attempt < values.length + 2 && await selected() !== desired; attempt++) {
    const current = await selected(), next = current + Math.sign(desired - current) * Math.min(2, Math.abs(desired - current));
    const box = await overlay.locator('.weui-picker__item').nth(next).boundingBox();
    assert.ok(box && box.y >= 0 && box.y + box.height <= page.viewportSize().height, label + ' visible row');
    await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
    await page.waitForTimeout(450); await stableMobile(page);
    assert.notEqual(await selected(), current, label + ' visible row click must move the wheel');
  }
  assert.equal(await selected(), desired, label + ' settled wheel row');
  await page.waitForTimeout(2500); // The actual conversation poll must not reset the visible choice.
  assert.equal(await selected(), desired, label + ' row survives live polling');
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click();
  await overlay.waitFor({ state: 'hidden' }); await stableMobile(page);
  for (let wait = 0; wait < 25 && (await picker.locator('.cp-picker').textContent()).trim() !== text; wait++) await page.waitForTimeout(100);
  assert.equal((await picker.locator('.cp-picker').textContent()).trim(), text, label + ' chosen business value');
}
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const select = (label, value, text) => mobile ? selectDependencyMobile(page, label, text) : page.getByLabel(label, { exact: true }).selectOption(value);
  let bearer;
  page.on('request', r => { if (r.headers().authorization?.startsWith('Bearer ')) bearer = r.headers().authorization; });
  await page.goto(fixture.origin);
  await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function request(route) {
    const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {});
    assert.equal(response.status(), 200, route); return (await response.json()).data;
  }
  const url = fixture.origin + (mobile ? '/#/pages/secretary/index?kind=conversation&id=' : '/conversations/') + fixture.conversation_id;
  const runURL = id => url + (mobile ? '&run=' : '?run=') + id;
  const task = await request('/tasks/' + fixture.task_id), pred = await request('/tasks/' + fixture.predecessor_id);
  const completed = await request('/runs/' + fixture.completed_run_id), removed = await request('/runs/' + fixture.removed_run_id);
  for (const [run, action, present] of [[completed, 'add', true], [removed, 'remove', false]]) {
    assert.equal(run.mutation.tool, 'update_task_dependency'); assert.equal(run.mutation.result.dependency_action, action);
    assert.equal(run.mutation.result.dependency_present, present); assert.ok(run.mutation.operation_id);
    assert.equal(run.mutation.result.version, run.mutation.base_version + 1);
    const actual = await request('/tasks/' + run.mutation.resource_id + '/dependencies');
    assert.equal(actual.items.some(item => item.predecessor_id === run.mutation.arguments.predecessor_id), present);
  }
  checks.push('Actual original-command MCP commits expose exact add/remove edge receipts and agree with the native database-backed dependencies endpoint.');
  await page.goto(runURL(fixture.completed_run_id)); await root().getByText('本次更新已提交', { exact: true }).waitFor();
  await assertNoCredentials(page);
  const name = 'secretary-task-dependencies-' + (mobile ? 'mobile' : 'web');
  await select('本次秘书任务', fixture.profile_id, '更新项目 · controlled-test');
  await button('选择准确业务更新').click(); await select('本次更新方式', 'update_task_dependency', '调整任务前置依赖');
  if (mobile) { await button('选择本次更新任务').click(); await root().getByText(task.title + ' · 待处理', { exact: true }).click(); }
  else await select('本次更新任务', task.id);
  await button('将这项明确更新加入请求').waitFor(); assert.equal(await button('将这项明确更新加入请求').isDisabled(), true);
  if (mobile) { await button('选择前置任务').click(); await root().getByText(pred.title + ' · 待处理', { exact: true }).click(); }
  else await select('前置任务', pred.id);
  await button('将这项明确更新加入请求').waitFor({ state: 'visible' });
  const reason = '员工核对首轮交付：先完成资料核对，再开始发布';
  await field('本次依赖更新原因').fill(reason);
  await button('将这项明确更新加入请求').click(); await root().getByText('请明确选择新增或解除依赖。', { exact: true }).waitFor();
  await select('依赖动作', 'remove', '解除依赖'); await button('将这项明确更新加入请求').click();
  await root().getByText('当前关系不会发生变化，请重新核对依赖。', { exact: true }).waitFor();
  checks.push('The visible form requires an actual second task, an explicit action and a real change; missing choices and removing an absent edge cannot grant authority.');
  await select('依赖动作', 'add', '新增依赖');
  await root().getByText('当前关系不会发生变化，请重新核对依赖。', { exact: true }).waitFor({ state: 'hidden' });
  await button('将这项明确更新加入请求').scrollIntoViewIfNeeded(); await stableMobile(page);
  await page.screenshot({ path: path.join(media, name + '-form.png'), fullPage: false });
  const sendPath = '/v1/conversations/' + fixture.conversation_id + '/runs'; let sends = 0;
  page.on('request', r => { if (new URL(r.url()).pathname === sendPath && r.method() === 'POST') sends++; });
  const layouts = [];
    for (const width of mobile ? [360, 390, 768] : [768, 1280, 1440]) {
      await page.setViewportSize({ width, height: mobile ? 900 : 1100 }); await stableMobile(page);
      const size = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth })); assert.ok(size.document <= size.viewport + 2); layouts.push({ width, ...size });
    }
    await page.setViewportSize(mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 });

  const recordings = [await recordViewport(page, app, name + '-review', async () => {
    await page.waitForTimeout(1600);
    await button('将这项明确更新加入请求').click(); await button('移除本次更新').waitFor();
    await root().getByText('本次明确更新', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' })); await stableMobile(page);
    await page.screenshot({ path: path.join(media, name + '-preview.png'), fullPage: false });
    await page.waitForTimeout(1800);
    assert.equal(sends, 0);
    checks.push('Selecting and previewing the exact two task versions sends no run; three actual viewport widths have no horizontal document overflow.');
    await field('本次消息内容').fill('只按这次明确选择新增这一条前置依赖。'); await page.waitForTimeout(1200);
    const [sent] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === sendPath && r.request().method() === 'POST'), button('确认发送本次请求').click()]);
    assert.equal(sent.status(), 202); const run = (await sent.json()).data, input = sent.request().postDataJSON();
    assert.deepEqual(input.mutation.arguments, { id: task.id, expected_version: task.version, predecessor_id: pred.id, expected_predecessor_version: pred.version, action: 'add', reason });
    assert.equal(run.mutation.status, 'authorized'); assert.equal(run.mutation.operation_id, null);
    const csrf = (await context.cookies()).find(c => c.name === '__Host-cp_csrf' || c.name === 'cp_csrf')?.value;
    const replay = await context.request.post(origin + sendPath, { headers: { ...(mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': csrf }), 'Idempotency-Key': sent.request().headers()['idempotency-key'] }, data: input });
    assert.equal(replay.status(), 202); assert.equal((await replay.json()).data.id, run.id);
    await page.waitForTimeout(1400);
    if (mobile) await button('明确取消这个任务').click();
    await field('取消任务的原因').fill('结束合成演示；本次尚未执行，保留实际提交历史。');
    const [cancelled] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1/runs/' + run.id + '/cancel'), button('确认请求取消').click()]); assert.equal(cancelled.status(), 200);
    assert.equal((await request('/runs/' + run.id)).state, 'cancelled'); assert.equal(sends, 1); await page.waitForTimeout(1400);
    assert.equal((await request('/tasks/' + task.id)).version, task.version); assert.equal((await request('/tasks/' + pred.id)).version, pred.version);
    assert.equal((await request('/tasks/' + task.id + '/dependencies')).items.some(item => item.predecessor_id === pred.id), false);
    checks.push('An authenticated send preserves all six exact choices, replay returns the same run, and cancellation before execution leaves both tasks and their edge unchanged.');
    for (const [id, suffix, expected] of [[fixture.completed_run_id, 'receipt', '存在依赖'], [fixture.removed_run_id, 'removed', '没有依赖']]) {
      await page.goto(runURL(id)); await root().getByText('本次更新已提交', { exact: true }).waitFor();
      await root().getByText('任务当前进展', { exact: true }).evaluate(node => node.scrollIntoView({ block: 'start' })); await stableMobile(page);
      await root().getByText(new RegExp('前置版本.*' + expected)).waitFor();
      await page.waitForTimeout(900);
      await page.screenshot({ path: path.join(media, name + '-' + suffix + '.png'), fullPage: false }); await page.waitForTimeout(2000);
    }
    checks.push('Actual committed add and remove views retain readable predecessor names, base versions, factual relationship states and real operation receipts.');
  })];
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ mobile, recordings, layouts, synthetic_data: true,
    scope: 'Actual authenticated Web/Taro H5 and native PostgreSQL API. UI send is queued then cancelled; completed add/remove views are genuine controlled-MCP fixture commits. No live model, device, cloud or production claim.' }, null, 2) + '\n');
}, 'task-dependencies');
