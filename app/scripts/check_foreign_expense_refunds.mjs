import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './foreign_expense_refunds_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './foreign_expense_refunds_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const url = fixture.origin + '/payments/' + fixture.refund.id;
  await page.goto(url);
  const open = page.getByRole('button', { name: '关联退款分配', exact: true });
  await open.waitFor();
  const shots = [], prefix = 'foreign-expense-refunds-web';
  async function shot(name) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + name + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    shots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  await page.getByText(/本条实际退回现金 71.790000 USD/).waitFor();
  await page.getByRole('heading', { name: '不可变费用分配', exact: true }).scrollIntoViewIfNeeded();
  await shot('partial');
  checks.push('A real native HTTP/PG refund shows560 HKD principal,143.59 EUR expense and71.79 USD attributed cash separately.');
  await open.click();
  const dialog = page.getByRole('dialog', { name: '关联退款分配', exact: true });
  await dialog.getByRole('radio', { name: '引用此原分配', exact: true }).check();
  await dialog.getByLabel('本次原付款本金（HKD）', { exact: true }).fill('220');
  await dialog.getByText(/费用回转 56.410000 EUR；实收现金份额 28.210000 USD/).waitFor();
  await dialog.getByRole('checkbox').check();
  await dialog.getByLabel('操作依据', { exact: true }).fill('Synthetic human refund134: consume final original principal and exact cash tail.');
  await dialog.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
  await dialog.getByLabel('本次原付款本金（HKD）', { exact: true }).scrollIntoViewIfNeeded();
  await shot('review');
  const [committed] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/payments/' + fixture.refund.id + '/refund-allocations'),
    dialog.getByRole('button', { name: '关联退款分配', exact: true }).click(),
  ]);
  assert.equal(committed.status(), 200);
  const receipt = (await committed.json()).data;
  assert.equal(receipt.id, fixture.refund.id);
  assert.equal(receipt.version, fixture.refund.version + 1);
  assert.equal(receipt.refund_allocated_principal, '780.000000');
  assert.equal(receipt.effective_allocated_amount, '100.000000');
  assert.equal(receipt.unallocated_amount, '0.000000');
  await dialog.waitFor({ state: 'hidden' });
  await page.getByText(/本条实际退回现金 28.210000 USD/).waitFor();
  const readback = await page.evaluate(async id => {
    const p = await fetch('/v1/payments/' + id, { credentials: 'same-origin' });
    const a = await fetch('/v1/payments/' + id + '/allocations?limit=100', { credentials: 'same-origin' });
    return { status: [p.status, a.status], payment: (await p.json()).data, allocations: (await a.json()).data.items };
  }, fixture.refund.id);
  assert.deepEqual(readback.status, [200, 200]);
  assert.equal(readback.payment.unallocated_amount, '0.000000');
  assert.equal(readback.allocations.length, 2);
  const leg = readback.allocations.find(a => a.amount === '220.000000');
  assert.equal(leg.source_allocation_id, fixture.source_allocation.id);
  assert.equal(leg.cost_amount, '56.410000');
  assert.equal(leg.refund_cash.amount, '28.210000');
  assert.equal(leg.refund_cash.whole_amount, '100.000000');
  assert.match(leg.refund_cash.original_amount, /^780(?:\.0+)?$/);
  checks.push('Actual recent-MFA human UI appends220 HKD /56.41 EUR /28.21 USD and re-reads two immutable allocations with zero cash remainder.');
  const recording = await recordViewport(page, app, prefix, async () => {
    await page.getByRole('heading', { name: '不可变费用分配', exact: true }).scrollIntoViewIfNeeded();
    await shot('complete');
    await page.reload();
    await page.getByText(/本条实际退回现金 28.210000 USD/).waitFor();
    await page.getByRole('heading', { name: '不可变费用分配', exact: true }).scrollIntoViewIfNeeded();
  });
  checks.push('Post-authentication viewport recording and reload retain the committed journal; screenshots contain synthetic data only.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots: shots, recording, credentials_excluded: true, synthetic_data: true }, null, 2) + '\n');
  await writeFile(path.join(evidence, 'business-proof.json'), JSON.stringify({
    refund_id: receipt.id, payment_version: receipt.version, allocation_id: leg.id,
    original_principal: '220.000000', original_currency: 'HKD', expense_inverse: '56.410000', expense_currency: 'EUR',
    attributed_cash: '28.210000', cash_currency: 'USD', whole_cash: '100.000000',
    total_attributed_principal: '780.000000', total_attributed_cash: '100.000000',
    durable_readback: true, bank_transfer_initiated: false, real_business_data: false,
  }, null, 2) + '\n');
}, 'foreign-expense-refunds');
