import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
await notificationBrowser(false, async ({ page, context, fixture, checks }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login); await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click(); await page.getByLabel('六位验证码').waitFor();
  await page.getByLabel('六位验证码').fill(await waitOTP(fixture)); await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('link', { name: '账号安全', exact: true }).click(); await page.getByRole('heading', { name: /账号安全/ }).waitFor();
  const old = page.locator('[data-session-id="' + fixture.alice_android_id + '"]'); await old.waitFor();
  assert.match(await page.locator('main').innerText(), /当前设备/); checks.push('Web lists the actual current refresh family and another signed-in device.');
  const foreign = await page.evaluate(async id => {
    const csrf = document.cookie.split('; ').find(value => value.startsWith('cp_csrf='))?.slice(8) ?? '';
    const response = await fetch('/v1/me/sessions/' + id + '/revoke', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': decodeURIComponent(csrf), 'Idempotency-Key': crypto.randomUUID() }, body: JSON.stringify({ reason: 'Owned fixture boundary check' }) }); return response.status;
  }, fixture.bob_device_id); assert.equal(foreign, 404); checks.push('A foreign employee device cannot be revoked through the real cookie API.');
  await old.getByRole('button', { name: '撤销此设备', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认撤销', exact: true }).click();
  await old.getByText('已撤销', { exact: true }).waitFor(); checks.push('Revoking another device commits and refreshes its historical state.');
  await page.getByRole('link', { name: '成员与授权', exact: true }).click();
  const bob = page.getByRole('row').filter({ hasText: 'bob' }); await bob.getByRole('button', { name: '重置密码', exact: true }).click();
  const dialog = page.getByRole('dialog'); await dialog.getByLabel('重置原因').fill('Owned browser employee identity verification');
  await dialog.getByRole('button', { name: '签发15分钟重置凭证', exact: true }).click(); const token = (await dialog.locator('pre').textContent()).trim(); assert.equal(token.length, 43);
  await dialog.getByRole('button', { name: '完成', exact: true }).click(); checks.push('The authorized administrator issues a bounded reset credential through the members UI.');
  await page.getByTitle('退出登录', { exact: true }).click(); await page.getByRole('button', { name: '忘记密码', exact: true }).click();
  await page.getByLabel('密码重置凭证').fill(token); await page.getByLabel('新密码', { exact: true }).fill('browser-recovered-bob-password'); await page.getByLabel('再次输入新密码').fill('browser-recovered-bob-password');
  await page.getByRole('button', { name: '重置密码', exact: true }).click(); await page.getByText(/密码已重置，所有设备已退出/).waitFor();
  await page.getByRole('button', { name: '返回登录', exact: true }).click();
  async function login(password) { await page.getByLabel('内部账号', { exact: true }).fill(fixture.bob.login); await page.getByLabel('密码', { exact: true }).fill(password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click(); }
  await login('browser-recovered-bob-password'); await page.getByRole('link', { name: '账号安全', exact: true }).click(); checks.push('Forgot-password completion returns to a real login with the replacement credential.');
  await page.getByRole('button', { name: '修改密码', exact: true }).click();
  const form = page.getByRole('dialog'); await form.getByLabel('当前密码').fill('browser-recovered-bob-password'); await form.getByLabel('新密码', { exact: true }).fill('browser-final-bob-password'); await form.getByLabel('再次输入新密码').fill('browser-final-bob-password');
  await form.getByRole('button', { name: '保存新密码', exact: true }).click(); await page.getByRole('button', { name: '登录工作空间', exact: true }).waitFor();
  assert.ok(!(await context.cookies()).some(cookie => ['cp_session', 'cp_refresh', 'cp_csrf'].includes(cookie.name))); checks.push('Changing a password signs out the current Web session and clears all three cookies.');
  await login('browser-final-bob-password'); await page.getByRole('link', { name: '账号安全', exact: true }).click();
  await page.getByRole('button', { name: '退出当前设备', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认撤销', exact: true }).click(); await page.getByRole('button', { name: '登录工作空间', exact: true }).waitFor();
  assert.ok(!(await context.cookies()).some(cookie => ['cp_session', 'cp_refresh', 'cp_csrf'].includes(cookie.name))); checks.push('Self-revocation exits to login and clears cookies without a second authenticated mutation.');
}, 'account-security');
