import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';

// The webhook is the isolated native test server, never Telegram's network.
await notificationBrowser(false, async ({ page, context, fixture, checks, openPage }) => {
  async function login(target, account) {
    await target.goto(fixture.origin);
    await target.getByLabel('内部账号', { exact: true }).fill(account.login);
    await target.getByLabel('密码', { exact: true }).fill(account.password);
    await target.getByRole('button', { name: '登录工作空间', exact: true }).click();
    await target.getByLabel('六位验证码').fill(await waitOTP({ alice: account }));
    await target.getByRole('button', { name: '验证并继续', exact: true }).click();
    await target.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
    await target.goto(fixture.origin + '/telegram');
    await target.getByRole('heading', { name: '本人绑定', exact: true }).waitFor();
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
    assert.equal(response.status(), 200, 'isolated inbound acceptance');
    assert.deepEqual(await response.json(), { ok: true });
  }
  function writeResponse(target, route, method) {
    return target.waitForResponse(response => new URL(response.url()).pathname === '/v1' + route && response.request().method() === method);
  }
  await login(page, fixture.alice);
  await page.getByText('本入口已能接收明确请求，可在指定配置可用时进入秘书任务；群回复尚未开放。', { exact: false }).waitFor();
  assert.equal((await read(context, '/me/telegram')).bindings.length, 0);
  checks.push('The real authenticated portal shows ingress availability separately from the unavailable executor and delivery.');

  await page.getByRole('button', { name: '发起本人绑定', exact: true }).click();
  await page.getByLabel('选择Bot', { exact: true }).selectOption(fixture.bot_id);
  await page.getByLabel('绑定原因', { exact: true }).fill('核对本人的实际TG数字身份');
  const [issuedResponse] = await Promise.all([writeResponse(page, '/me/telegram/challenges', 'POST'), page.getByRole('button', { name: '生成本人私聊绑定链接', exact: true }).click()]);
  assert.equal(issuedResponse.status(), 201);
  const issued = (await issuedResponse.json()).data;
  assert.equal(typeof issued.start_token, 'string'); assert.equal(issued.start_token.length, 43);
  await page.getByRole('link', { name: '打开Bot私聊完成证明', exact: true }).waitFor();
  const beforeProof = await read(context, '/me/telegram');
  assert.equal(beforeProof.bindings.length, 0); assert.equal(beforeProof.challenges.find(item => item.id === issued.id).start_token, undefined);
  checks.push('A recent MFA login issues a private single-use link; issuing it grants no binding and status reloads reveal no link token.');

  const telegramID = 987654399;
  await inbound({ update_id: 1000, message: { message_id: 1000, date: Math.floor(Date.now() / 1000), from: { id: telegramID, is_bot: false }, chat: { id: telegramID, type: 'private' }, text: '/start ' + issued.start_token, entities: [{ type: 'bot_command', offset: 0, length: 6 }] } });
  await page.getByRole('button', { name: '核对并确认绑定', exact: true }).waitFor();
  assert.equal((await read(context, '/me/telegram')).bindings.length, 0);
  await page.getByRole('button', { name: '核对并确认绑定', exact: true }).click();
  await page.getByRole('checkbox', { name: '我已核对该数字身份属于本人', exact: true }).check();
  await page.getByLabel('确认原因', { exact: true }).fill('原登录核对实际数字身份后确认');
  const [confirmedResponse] = await Promise.all([writeResponse(page, '/me/telegram/challenges/' + issued.id + '/confirm', 'POST'), page.getByRole('button', { name: '确认绑定这个数字身份', exact: true }).click()]);
  assert.equal(confirmedResponse.status(), 201);
  const binding = (await confirmedResponse.json()).data;
  assert.equal(binding.telegram_user_id, String(telegramID));
  await page.getByRole('button', { name: '撤销此绑定', exact: true }).waitFor();
  checks.push('A real private webhook proves the numeric identity; only the original portal login explicitly confirming creates the formal binding.');

  const admin = await openPage(); await login(admin.page, fixture.administrator);
  await admin.page.getByRole('button', { name: '编辑群入口', exact: true }).click();
  await admin.page.getByLabel('接收状态', { exact: true }).selectOption('active');
  await admin.page.getByRole('checkbox', { name: '我已核对群与话题，并允许接收指定任务', exact: true }).check();
  await admin.page.getByLabel('配置原因', { exact: true }).fill('核对准确群号话题后启用接收');
  const [channelResponse] = await Promise.all([writeResponse(admin.page, '/telegram/channels/' + fixture.channel_id, 'PATCH'), admin.page.getByRole('button', { name: '保存此群入口配置', exact: true }).click()]);
  assert.equal(channelResponse.status(), 200);
  const channel = (await channelResponse.json()).data;
  assert.equal(channel.state, 'active'); assert.equal(channel.chat_id, '-1001122334455'); assert.equal(channel.topic_id, 42);
  checks.push('A separate administrator session explicitly enables the exact group and topic with a versioned, audited portal change.');

  const requestText = '/ask Web核验指定项目简报请求';
  const update = { update_id: 1001, message: { message_id: 1001, date: Math.floor(Date.now() / 1000), message_thread_id: 42, from: { id: telegramID, is_bot: false }, chat: { id: -1001122334455, type: 'supergroup' }, text: requestText, entities: [{ type: 'bot_command', offset: 0, length: 4 }] } };
  await inbound(update); await inbound(update);
  await page.getByRole('button', { name: '重新读取接入状态', exact: true }).click();
  await page.getByText(requestText, { exact: true }).waitFor();
  const ownRequests = (await read(context, '/me/telegram/requests')).items.filter(item => item.text === requestText);
  assert.equal(ownRequests.length, 1); assert.equal(ownRequests[0].state, 'awaiting_executor');
  assert.equal(ownRequests[0].channel_version, channel.version);
  assert.equal((await read(admin.context, '/me/telegram/requests')).items.some(item => item.text === requestText), false);
  checks.push('The bound sender can inspect one durable request after identical webhook retries; another portal identity cannot see it and no executor completion is claimed.');

  await inbound({ update_id: 1002, chat_member: { chat: { id: -1001122334455, type: 'supergroup' } } });
  await admin.page.getByRole('button', { name: '重新读取接入状态', exact: true }).click();
  await admin.page.getByText('接收已暂停', { exact: false }).waitFor();
  await admin.page.getByRole('button', { name: '查看入口历史', exact: true }).click();
  await admin.page.getByText('群成员变化', { exact: false }).first().waitFor();
  await page.getByRole('button', { name: '重新读取接入状态', exact: true }).click();
  const withdrawn = (await read(context, '/me/telegram/requests')).items.find(item => item.text === requestText);
  assert.equal(withdrawn.state, 'withdrawn');
  const history = await read(admin.context, '/telegram/channels/' + fixture.channel_id + '/history');
  assert.equal(history.items[0].source, 'membership'); assert.equal(history.items[0].snapshot.state, 'paused');
  checks.push('A membership update pauses the group, withdraws pending work and appears as the latest actual channel revision in the portal.');

  await page.getByRole('button', { name: '撤销此绑定', exact: true }).click();
  await page.getByLabel('结束原因', { exact: true }).fill('结束浏览器验证并撤销本次身份');
  const [revoked] = await Promise.all([writeResponse(page, '/me/telegram/bindings/' + binding.id + '/revoke', 'POST'), page.getByRole('button', { name: '确认撤销此绑定', exact: true }).click()]);
  assert.equal(revoked.status(), 200); assert.equal((await read(context, '/me/telegram')).bindings.length, 0);
  await page.getByText('尚无有效绑定。私聊证明后，仍需返回本页核对数字身份并确认。', { exact: true }).waitFor();
  checks.push('The actual owner revokes the versioned binding in the UI; no active browser-created binding remains.');
}, 'telegram-ingress');
