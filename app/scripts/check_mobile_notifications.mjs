import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
async function stable(page) { await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); }); }
async function select(page, label, text) {
  const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]'); await picker.click();
  const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor(); const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(animation => animation.finished.catch(() => {}))); });
  await stable(page); const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text); assert.ok(desired >= 0);
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0, box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); for (let step = 1; step <= 8; step++) { await page.mouse.move(x, y - (desired - current) * item.height * step / 8); await stable(page); } await page.mouse.up(); await stable(page); }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stable(page);
  assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired));
}
await notificationBrowser(true, async ({ page, context, fixture, checks }) => {
  let authToken; page.on('request', request => { const token = request.headers().authorization; if (token?.startsWith('Bearer ')) authToken = token; });
  await page.goto(fixture.origin); await page.locator('input[placeholder="你的公司账号"]').fill(fixture.alice.login); await page.locator('input[type="password"]').fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间 ↗', exact: true }).click(); const code = page.locator('input[maxlength="6"]'); await code.waitFor();
  await code.fill(await waitOTP(fixture)); await page.getByRole('button', { name: '验证并继续 ↗', exact: true }).click();
  await screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  await screen(page).locator('.cp-row-title').getByText('通知中心', { exact: true }).click(); await screen(page).locator('.cp-title').filter({ hasText: '通知中心' }).waitFor();
  const card = () => screen(page).locator('.cp-pad.cp-paragraph').filter({ hasText: fixture.task_title }); await card().waitFor();
  assert.match(await card().innerText(), /未读/); assert.ok(!(await screen(page).innerText()).includes(fixture.foreign_task_title));
  checks.push('The Taro workbench badge opens the actual employee inbox without another recipient’s task.');
  assert.ok(authToken);
  const request = route => context.request.get(fixture.api_origin + '/v1' + route, { headers: { Authorization: authToken } });
  const foreign = await request('/notifications/' + fixture.foreign_notification_id); assert.equal(foreign.status(), 404); assert.ok(!(await foreign.text()).includes(fixture.foreign_task_title));
  checks.push('The mobile session cannot read another employee’s notification by ID.');
  const ack = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/notifications/' + fixture.notification_id + '/read' && response.request().method() === 'POST');
  await card().getByRole('button', { name: '标记已读', exact: true }).click(); const response = await ack; assert.equal(response.status(), 200); assert.deepEqual(response.request().postDataJSON(), { delivery_seq: 1 });
  const saved = (await response.json()).data; assert.equal(saved.read, true); assert.equal(saved.attention_required, true);
  await card().getByText(/已读 · 仍需处理/).waitFor(); const task = await request('/tasks/' + fixture.task_id); assert.equal(task.status(), 200); assert.equal((await task.json()).data.state, 'todo');
  checks.push('Marking the observed notification read preserves the actual outstanding task.');
  const unread = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/notifications' && new URL(response.url()).searchParams.get('state') === 'unread');
  await select(page, '处理范围', '仅未读'); assert.equal((await unread).status(), 200); await card().waitFor({ state: 'hidden' });
  await select(page, '处理范围', '仍需处理'); await card().waitFor(); await card().getByText(/已读 · 仍需处理/).waitFor();
  await select(page, '通知类型', '待审批'); await screen(page).getByText('当前没有符合条件的通知。', { exact: true }).waitFor();
  await select(page, '通知类型', '任务'); await card().waitFor(); checks.push('Actual Taro picker gestures filter unread, duty and business category pages.');
  await screen(page).getByText(/UNSUPPORTED_EVENT_VERSION/).waitFor(); assert.ok(!(await screen(page).innerText()).includes('private_diagnostic'));
  checks.push('Authorized mobile operators see sanitized delivery failure metadata.');
  await card().getByRole('button', { name: '打开相关资料', exact: true }).click(); await screen(page).locator('.cp-title').filter({ hasText: fixture.task_title }).waitFor();
  await screen(page).getByText('任务内容', { exact: true }).waitFor(); checks.push('Opening the notification rechecks permission and reads the exact live task.');
  assert.ok(!(await screen(page).innerText()).includes(fixture.foreign_task_title));
  assert.equal(await page.evaluate(() => { let count = 0; for (const storage of [localStorage, sessionStorage]) for (let i = 0; i < storage.length; i++) { const key = storage.key(i), value = storage.getItem(key); if (/access_token|refresh_token|mfa_secret|private_diagnostic/.test(value ?? '')) count++; } return count; }), 0);
  checks.push('Notification navigation leaves no authentication secrets or private event bodies in generic browser storage.');
  await page.goBack(); await screen(page).locator('.cp-title').filter({ hasText: '通知中心' }).waitFor(); await card().waitFor();
});
