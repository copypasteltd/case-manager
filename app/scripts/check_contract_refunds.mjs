import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP, otp } from './contract_refunds_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './contract_refunds_recording.mjs';
const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const input = name => mobile ? root().locator('input[aria-label="' + name + '"]') : page.getByLabel(name, { exact: true });
  await page.goto(fixture.origin);
  await input('内部账号').fill(fixture.alice.login); await input('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  const loginCode = await waitOTP(fixture), usedStep = Math.floor(Date.now() / 30000);
  await input('六位验证码').fill(loginCode); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const url = fixture.origin + (mobile ? '/#/pages/business/index?kind=contract&id=' : '/contracts/') + fixture.contract.id;
  await page.goto(url);
  if (mobile) await button('合同财务归属与核对').click();
  await button('查看节点归属历史').waitFor();
  checks.push('Actual approved/signed/effective contract and four-channel foreign-refund attributions are read through native HTTP/PG.');
  const directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  const prefix = 'contract-refunds-' + (mobile ? 'mobile' : 'web'), screenshots = [];
  async function shot(suffix) {
    await assertNoCredentials(page);
    const name = prefix + '-' + suffix + '.png', target = path.join(directory, name);
    await page.screenshot({ path: target });
    screenshots.push({ file: 'docs/media/screenshots/' + name, sha256: createHash('sha256').update(await readFile(target)).digest('hex') });
  }
  await button('查看节点归属历史').scrollIntoViewIfNeeded(); await shot('summary');
  await button('查看节点归属历史').click();
  const latest = mobile ? root().locator('.cp-pad').filter({ hasText: '归属实际退回款 · 记录 5' })
    : page.locator('article.cost-business-link').filter({ has: page.getByRole('heading', { name: '归属实际退回款 · 记录 5', exact: true }) });
  await latest.waitFor(); await latest.scrollIntoViewIfNeeded();
  assert.match(await latest.textContent(), /USD\s*100/); assert.match(await latest.textContent(), /HKD\s*780/);
  await shot('evidence');
  checks.push('Immutable history displays the entire100 USD refund separately from195 HKD contract principal; no mixed-unit summation.');
  await latest.getByRole('button', { name: '解除部分归属', exact: true }).click();
  await input(mobile ? '本次合同原币归属本金（HKD）' : '本次原币归属金额').fill('195');
  await (mobile ? root().locator('textarea[aria-label="合同财务操作依据"]') : page.getByLabel('操作依据', { exact: true })).fill('Synthetic human release of contract principal; cash stays fixed.');
  if (mobile) {
    if (Math.floor(Date.now() / 30000) <= usedStep) await page.waitForTimeout((usedStep + 1) * 30000 - Date.now() + 300);
    const code = otp(fixture.alice.mfa_secret, Math.floor(Date.now() / 30000));
    await input('验证器六位验证码').fill(code); await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  } else await page.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
  const submit = mobile ? button('解除部分归属') : page.getByRole('dialog', { name: '解除部分归属', exact: true }).getByRole('button', { name: '解除部分归属', exact: true });
  const [committed] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'POST' && /\/finance\/entries\/[^/]+\/release$/.test(new URL(r.url()).pathname)),
    submit.click(),
  ]);
  assert.equal(committed.status(), 200); const receipt = (await committed.json()).data;
  assert.equal(receipt.contract_id, fixture.contract.id); assert.equal(receipt.version, 6);
  if (mobile) { await button('查看节点归属历史').waitFor(); }
  else { await page.getByRole('dialog', { name: '解除部分归属', exact: true }).waitFor({ state: 'hidden' }); await page.getByRole('dialog', { name: '节点归属与核对历史', exact: true }).getByRole('button', { name: '关闭弹窗', exact: true }).click(); }
  await button('查看节点归属历史').waitFor();
  await root().getByText(/原付款存在尚未归属.*退款/).waitFor();
  checks.push('The actual human-MFA UI appends one195 HKD release; current contract net and unattributed principal update, while bank cash remains100 USD.');
  const recording = await recordViewport(page, app, prefix, async () => {
    await button('查看节点归属历史').scrollIntoViewIfNeeded(); await shot('released');
    await button('查看节点归属历史').click();
    await root().getByText('解除部分归属 · 记录 6', { exact: true }).waitFor();
    if (mobile) await button('返回合同财务').click();
    else await page.getByRole('dialog', { name: '节点归属与核对历史', exact: true }).getByRole('button', { name: '关闭弹窗', exact: true }).click();
    await button('查看节点归属历史').scrollIntoViewIfNeeded();
  });
  checks.push('After-MFA recording uses actual viewport pixels and rereads the durable release history; every owned browser closes.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, credentials_excluded: true, synthetic_data: true }, null, 2) + '\n');
  await writeFile(path.join(evidence, 'business-proof.json'), JSON.stringify({ contract_id: fixture.contract.id, release_event_id: receipt.event_id, release_finance_version: receipt.version, released_principal: '195.000000', principal_currency: 'HKD', whole_refund_cash: '100.000000', whole_refund_currency: 'USD', cash_transfer_initiated: false }, null, 2) + '\n');
}, 'contract-refunds');
