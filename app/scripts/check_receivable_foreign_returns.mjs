import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './receivable_foreign_returns_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './receivable_foreign_returns_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.goto(fixture.origin + '/contracts/' + fixture.contract.id);
  const historyButton = page.getByRole('button', { name: '查看节点归属历史', exact: true });
  await historyButton.waitFor();
  const screenshots = [], prefix = 'receivable-foreign-returns-web';
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    screenshots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  await historyButton.click();
  let history = page.getByRole('dialog', { name: '节点归属与核对历史', exact: true });
  await history.getByText(/本条实际付出归属／解除 USD\s*50/).waitFor();
  await shot('partial');
  await history.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Actual effective receivable contract shows390 HKD principal separately from50 USD attributed outgoing cash and100 USD whole payment.');
  await page.getByRole('button', { name: '归属实际退回款', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '归属实际退回款', exact: true });
  await dialog.getByRole('button', { name: '选择原正向资金归属', exact: true }).click();
  await page.getByRole('dialog', { name: '选择原正向资金归属', exact: true }).getByRole('button').filter({ hasText: '归属记录 1' }).click();
  await dialog.getByRole('button', { name: '选择准确合同财务来源', exact: true }).click();
  await page.getByRole('dialog', { name: '选择准确合同财务来源', exact: true }).getByRole('button').filter({ hasText: 'Synthetic contract foreign refund' }).click();
  await dialog.getByLabel('本次原币归属金额', { exact: true }).fill('195');
  await dialog.getByLabel('本次实际付出归属金额（USD）', { exact: true }).fill('25');
  await dialog.getByLabel('每1 HKD 实际兑换的 USD', { exact: true }).fill('0.1282051282');
  await dialog.getByLabel('实际退回汇率凭证来源', { exact: true }).fill('Synthetic reviewed return135 statement');
  await dialog.getByRole('checkbox', { name: '已核对合同退回本金、本次实际付出与汇率凭证。', exact: true }).check();
  await dialog.getByLabel('操作依据', { exact: true }).fill('Synthetic human135: attribute195 HKD principal to25 USD outgoing cash.');
  await dialog.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
  await dialog.getByLabel('本次原币归属金额', { exact: true }).scrollIntoViewIfNeeded();
  await shot('review');
  const [committed] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/contracts/' + fixture.contract.id + '/finance/entries'),
    dialog.getByRole('button', { name: '归属实际退回款', exact: true }).click(),
  ]);
  assert.equal(committed.status(), 200);
  const receipt = (await committed.json()).data;
  assert.equal(receipt.contract_id, fixture.contract.id); assert.equal(receipt.version, 3);
  await dialog.waitFor({ state: 'hidden' });
  const readback = await page.evaluate(async f => {
    const paths = ['/contracts/' + f.contract.id + '/finance', '/contracts/' + f.contract.id + '/finance/events?limit=100', '/payments/' + f.outgoing.id];
    return Promise.all(paths.map(async path => { const r = await fetch('/v1' + path, { credentials: 'same-origin' }); return { status: r.status, data: (await r.json()).data }; }));
  }, fixture);
  assert.deepEqual(readback.map(r => r.status), [200, 200, 200]);
  assert.equal(readback[0].data.nodes[0].cash_amount, '195.000000');
  assert.equal(readback[2].data.contract_return_reserved_amount, '75.000000');
  assert.equal(readback[2].data.unallocated_amount, '25.000000');
  const leg = readback[1].data.items.find(e => e.id === receipt.event_id);
  assert.equal(leg.source_event_id, fixture.root_id); assert.equal(leg.amount, '195.000000');
  assert.equal(leg.return_cash.amount, '25.000000'); assert.equal(leg.return_cash.whole_amount, '100.000000');
  assert.equal(leg.return_cash.source, 'Synthetic reviewed return135 statement');
  checks.push('Recent-MFA Web command commits195 HKD /25 USD and re-reads the immutable event,195 HKD contract net,75 USD reservation and25 USD free cash.');
  await historyButton.click();
  history = page.getByRole('dialog', { name: '节点归属与核对历史', exact: true });
  await history.getByText(/本条实际付出归属／解除 USD\s*25/).waitFor();
  const recording = await recordViewport(page, app, prefix, async () => {
    await shot('complete');
    await page.reload(); await historyButton.click();
    await page.getByRole('dialog', { name: '节点归属与核对历史', exact: true }).getByText(/本条实际付出归属／解除 USD\s*25/).waitFor();
  });
  checks.push('Post-MFA viewport recording and actual reload retain both cash and principal evidence; owned browser contexts close.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, credentials_excluded: true, synthetic_data: true }, null, 2) + '\n');
  await writeFile(path.join(evidence, 'business-proof.json'), JSON.stringify({ contract_id: receipt.contract_id, event_id: receipt.event_id, version: receipt.version,
    principal: '195.000000', principal_currency: 'HKD', attributed_cash: '25.000000', cash_currency: 'USD', whole_cash: '100.000000',
    contract_net: '195.000000', reserved_cash: '75.000000', unallocated_cash: '25.000000', durable_readback: true,
    bank_transfer_initiated: false, real_business_data: false }, null, 2) + '\n');
}, 'receivable-foreign-returns');
