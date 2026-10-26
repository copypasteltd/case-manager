import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';

// Every webhook targets the owned native fixture. No Telegram or model is contacted.
await notificationBrowser(false, async ({ page, context, fixture, checks, openPage }) => {
  async function login(target, account) {
    await target.goto(fixture.origin);
    await target.getByLabel('内部账号', { exact: true }).fill(account.login);
    await target.getByLabel('密码', { exact: true }).fill(account.password);
    await target.getByRole('button', { name: '登录工作空间', exact: true }).click();
    await target.getByLabel('六位验证码').fill(await waitOTP({ alice: account }));
    await target.getByRole('button', { name: '验证并继续', exact: true }).click();
    await target.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  }
  async function read(client, route) {
    const response = await client.request.get(fixture.origin + '/v1' + route);
    assert.equal(response.status(), 200, route);
    return (await response.json()).data;
  }
  async function inbound(update) {
    const response = await context.request.post(fixture.api_origin + '/v1/integrations/telegram/' + fixture.bot_id + '/webhook', {
      headers: { 'X-Telegram-Bot-Api-Secret-Token': fixture.webhook_secret }, data: update,
    });
    assert.equal(response.status(), 200);
    assert.deepEqual(await response.json(), { ok: true });
  }
  let sequence = 10000;
  function request(text) {
    const id = ++sequence;
    return { update_id: id, message: { message_id: id, date: Math.floor(Date.now() / 1000), message_thread_id: 42,
      from: { id: 987654321, is_bot: false }, chat: { id: -1001122334455, type: 'supergroup' }, text,
      entities: [{ type: 'bot_command', offset: 0, length: text.split(' ')[0].length }] } };
  }
  async function actualRequest(text) {
    for (let attempt = 0; attempt < 50; attempt++) {
      const result = (await read(context, '/me/telegram/requests')).items.find(item => item.text === text);
      if (result?.run_id) return result;
      await page.waitForTimeout(150);
    }
    throw new Error('The actual dispatcher did not create the requested run.');
  }
  const card = id => page.locator('[data-telegram-request-id="' + id + '"]');
  await login(page, fixture.alice);
  await page.goto(fixture.origin + '/telegram');
  await page.getByRole('heading', { name: '本人绑定', exact: true }).waitFor();
  const completed = (await read(context, '/me/telegram/requests')).items.find(item => item.run_id === fixture.completed_run_id);
  assert.ok(completed); assert.equal(completed.run_state, 'succeeded');
  await card(completed.id).getByRole('heading', { name: '秘书任务已结束', exact: true }).waitFor();
  await card(completed.id).getByRole('link', { name: '查看此秘书任务', exact: true }).click();
  await page.getByText('TG渠道会话 · 本人请求与结果', { exact: false }).waitFor();
  await page.getByText('此会话由TG入口管理，请从指定群话题发送新的请求。', { exact: true }).waitFor();
  await page.getByText('已核对浏览器指定项目的真实资料。', { exact: false }).waitFor();
  assert.equal(await page.getByLabel('本次消息内容', { exact: true }).count(), 0);
  checks.push('The completed inbound request opens its actual Telegram conversation and receipt-backed result; portal task injection has no input.');

  await page.goto(fixture.origin + '/conversations');
  await page.getByLabel('会话类型', { exact: true }).selectOption('telegram');
  await page.locator('a.party-row[href="/conversations/' + fixture.conversation_id + '"]').waitFor();
  checks.push('The real conversation list filters Telegram contexts through the public contract.');

  const admin = await openPage(); await login(admin.page, fixture.administrator);
  const foreign = await admin.context.request.get(fixture.origin + '/v1/conversations/' + fixture.conversation_id);
  assert.equal(foreign.status(), 404);
  await admin.page.goto(fixture.origin + '/telegram');
  await admin.page.getByRole('button', { name: '编辑群入口', exact: true }).click();
  await admin.page.getByLabel('默认任务', { exact: true }).selectOption('project-briefing');
  await admin.page.getByRole('checkbox', { name: '我已核对群与话题，并允许接收指定任务', exact: true }).check();
  await admin.page.getByLabel('配置原因', { exact: true }).fill('核对独立TG只读简报入口与默认任务');
  const changed = admin.page.waitForResponse(response => new URL(response.url()).pathname === '/v1/telegram/channels/' + fixture.channel_id && response.request().method() === 'PATCH');
  await admin.page.getByRole('button', { name: '保存此群入口配置', exact: true }).click();
  const saved = await changed; assert.equal(saved.status(), 200);
  assert.equal((await saved.json()).data.default_skill_id, 'project-briefing');
  checks.push('A separate administrator session saves the explicit default task but cannot read the requester’s conversation.');

  await page.goto(fixture.origin + '/telegram');
  const text = '/brief 浏览器实际接收并调度的项目简报';
  const update = request(text); await inbound(update); await inbound(update);
  const queued = await actualRequest(text);
  assert.equal(queued.state, 'dispatched'); assert.equal(queued.run_state, 'queued');
  assert.equal(queued.requested_skill_id, 'project-briefing');
  assert.notEqual(queued.conversation_id, fixture.conversation_id);
  await page.getByRole('button', { name: '重新读取接入状态', exact: true }).click();
  await card(queued.id).getByRole('heading', { name: '秘书任务已排队', exact: true }).waitFor();
  await inbound(update);
  const matched = (await read(context, '/me/telegram/requests')).items.filter(item => item.text === text);
  assert.equal(matched.length, 1); assert.equal(matched[0].run_id, queued.run_id);
  checks.push('The real automatic dispatcher converts repeated webhook delivery into one durable run; a changed channel revision gets a new context.');

  await card(queued.id).getByRole('link', { name: '查看此秘书任务', exact: true }).click();
  await page.getByLabel('取消任务的原因', { exact: true }).fill('本人从门户取消尚未执行的TG任务');
  const cancelled = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/runs/' + queued.run_id + '/cancel' && response.request().method() === 'POST');
  await page.getByRole('button', { name: '确认请求取消', exact: true }).click();
  const stopped = await cancelled; assert.equal(stopped.status(), 200); assert.equal((await stopped.json()).data.state, 'cancelled');
  await page.goto(fixture.origin + '/telegram');
  await card(queued.id).getByRole('heading', { name: '秘书任务已取消', exact: true }).waitFor();
  checks.push('The original requester cancels the actual queued run in the UI, and the Telegram receipt displays the committed cancelled state.');

  const nextText = '/brief 暂停群入口后应撤回的实际请求';
  await inbound(request(nextText)); const next = await actualRequest(nextText);
  await inbound({ update_id: ++sequence, my_chat_member: { date: Math.floor(Date.now() / 1000), from: { id: 987654321, is_bot: false },
    chat: { id: -1001122334455, type: 'supergroup' }, old_chat_member: { status: 'member', user: { id: Number(fixture.bot_id), is_bot: true } },
    new_chat_member: { status: 'left', user: { id: Number(fixture.bot_id), is_bot: true } } } });
  await page.getByRole('button', { name: '重新读取接入状态', exact: true }).click();
  await card(next.id).getByRole('heading', { name: '已撤回', exact: true }).waitFor();
  const history = await read(admin.context, '/telegram/channels/' + fixture.channel_id + '/history');
  assert.equal(history.items[0].source, 'membership'); assert.equal(history.items[0].snapshot.state, 'paused');
  assert.equal((await read(context, '/me/telegram/requests')).items.find(item => item.id === completed.id).run_state, 'succeeded');
  checks.push('A real group membership loss withdraws active work and records the channel revision while preserving the previously completed result.');
  await page.getByRole('link', {name:'查看此请求及群通知回执',exact:true}).first().waitFor();
  checks.push('The UI states that task completion does not mean Telegram delivery; the test invokes neither a provider nor Telegram outbound.');
}, 'telegram-dispatch');
