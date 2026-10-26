import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
await notificationBrowser(true, async ({ page, fixture, checks }) => {
  await page.goto(fixture.origin); await button(page, '忘记密码').click();
  await screen(page).locator('input[aria-label="密码重置凭证"]').fill(fixture.mobile_reset_token);
  await screen(page).locator('input[aria-label="新密码"]').fill('mobile-recovered-bob-password'); await screen(page).locator('input[aria-label="再次输入新密码"]').fill('mobile-recovered-bob-password');
  await button(page, '重置密码').click(); await screen(page).getByText(/密码已重置，所有设备已退出/).waitFor(); await button(page, '返回登录').click();
  checks.push('The Taro forgot-password form consumes a real administrator-issued one-time credential.');
  async function login(name, password) { await screen(page).locator('input[aria-label="内部账号"]').fill(name); await screen(page).locator('input[aria-label="密码"]').fill(password); await button(page, '登录工作空间 ↗').click(); }
  await login(fixture.bob.login, 'mobile-recovered-bob-password'); await button(page, '我的').click(); await screen(page).getByText('我的登录设备', { exact: true }).waitFor();
  assert.match(await screen(page).innerText(), /当前设备/); assert.match(await screen(page).innerText(), /已撤销/); checks.push('Mobile settings display current and revoked device families from the native API.');
  await button(page, '修改密码').click(); await screen(page).locator('input[aria-label="当前密码"]').fill('mobile-recovered-bob-password'); await screen(page).locator('input[aria-label="新密码"]').fill('mobile-final-bob-password'); await screen(page).locator('input[aria-label="再次输入新密码"]').fill('mobile-final-bob-password');
  await button(page, '保存新密码').click(); await button(page, '登录工作空间 ↗').waitFor(); checks.push('Confirmed password change clears mobile credentials and returns to login.');
  await login(fixture.bob.login, 'mobile-final-bob-password'); await button(page, '我的').click(); await button(page, '退出当前设备').click();
  await page.getByText('确认撤销', { exact: true }).last().click(); await button(page, '登录工作空间 ↗').waitFor(); checks.push('A current mobile device can revoke itself and leaves no authenticated screen.');
  await login(fixture.alice.login, fixture.alice.password); const code = screen(page).locator('input[aria-label="六位验证码"]'); await code.waitFor(); await code.fill(await waitOTP(fixture)); await button(page, '验证并继续 ↗').click();
  await button(page, '我的').click(); await screen(page).getByText('我的登录设备', { exact: true }).waitFor();
  // Check the exact family requested. The fixture also retains its earlier
  // pre-enrollment session; the new authenticated Android family sorts first.
  const revoked = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/me/sessions/' + fixture.alice_android_id + '/revoke' && response.request().method() === 'POST');
  await button(page, '撤销此设备').first().click(); await page.getByText('确认撤销', { exact: true }).last().click(); assert.equal((await revoked).status(), 200);
  await screen(page).getByText('已撤销', { exact: true }).waitFor(); assert.match(await screen(page).innerText(), /当前设备/); checks.push('Revoking another device keeps the current mobile login and reloads the committed state.');
  assert.equal(await page.evaluate(() => [localStorage, sessionStorage].some(storage => Object.keys(storage).some(key => /mobile-final-bob-password|mobile-recovered-bob-password|reset_token|access_token|refresh_token/.test(storage.getItem(key) ?? '')))), false);
  checks.push('Password and authentication material are absent from generic browser storage.');
}, 'account-security');
