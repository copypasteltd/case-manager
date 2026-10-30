// Real synthetic human journeys; every business read/write uses the actual API.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { payment_feeBrowser, otp } from './payment_fee_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './payment_fee_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
await payment_feeBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = name => mobile ? root().locator('input[aria-label="' + name + '"],textarea[aria-label="' + name + '"]') : root().getByLabel(name, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function headers() { return mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': (await context.cookies()).find(cookie => cookie.name === 'cp_csrf')?.value ?? '' }; }
  async function api(route, body, status = 200, key = randomUUID()) {
    const response = body ? await context.request.post(origin + '/v1' + route, { headers: { ...await headers(), 'Idempotency-Key': key }, data: body }) : await context.request.get(origin + '/v1' + route, { headers: await headers() });
    const result = await response.json(); assert.equal(response.status(), status, route + ': ' + (result.error?.code ?? '')); return result.data;
  }
  const url = fixture.origin + (mobile ? '/#/pages/finance/index?kind=payment&id=' : '/payments/') + fixture.payment.id;
  const pathAPI = '/payments/' + fixture.payment.id + '/fees';
  let lastStep;
  async function verifyMobile() {
    const step = Math.floor(Date.now() / 30000);
    if (step <= lastStep) await page.waitForTimeout((lastStep + 1) * 30000 - Date.now() + 250);
    lastStep = Math.floor(Date.now() / 30000);
    await field('验证器六位验证码').fill(otp(fixture.alice.mfa_secret, lastStep)); await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  }
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  if (Math.floor(Date.now() / 30000) <= fixture.alice.last_step) await page.waitForTimeout((fixture.alice.last_step + 1) * 30000 - Date.now() + 250);
  lastStep = Math.floor(Date.now() / 30000); await field('六位验证码').fill(otp(fixture.alice.mfa_secret, lastStep)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  await root().getByText('业务待办', { exact: true }).first().waitFor();
  checks.push('Actual employee login and MFA complete before recording.');
  const p = await api('/payments/' + fixture.payment.id), fee = await api('/payments/' + fixture.fee_payment.id), cost = await api('/costs/' + fixture.cost.id);
  const before = { p, fee, cost, allocations: await api('/payments/' + fee.id + '/allocations?limit=100') };
  assert.equal(p.state, 'recorded'); assert.equal(cost.state, 'confirmed'); assert.equal(fee.state, 'recorded');
  checks.push('The real fixture has confirmed incurred cost and recorded cash with an existing dual-currency allocation.');
  await page.goto(url); await button('查阅手续费依据').click();
  await root().getByText('尚无手续费关联记录；这不代表此付款没有手续费。', { exact: true }).waitFor();
  checks.push('An empty real fee ledger does not claim that fees are absent.');
  await button('关联手续费依据').click(); assert.equal(await button('确认关联手续费依据').isDisabled(), true);
  checks.push('A blank form cannot submit a fee association.');
  await button('选择手续费实际资金').click();
  if (mobile) await root().locator('.cp-row-title').getByText(fee.title + ' · ' + fee.currency + ' ' + fee.amount, { exact: true }).first().click();
  else await root().getByRole('button', { name: fee.title + ' · ' + fee.currency + ' ' + fee.amount, exact: true }).first().click();
  await button('选择手续费原分配').click();
  const label = '费用 ' + fixture.allocation.cost_id + ' · ' + fixture.allocation.remaining_amount + ' ' + fixture.allocation.payment_currency + ' · ' + fixture.allocation.remaining_cost_amount + ' ' + fixture.allocation.cost_currency;
  if (mobile) await root().locator('.cp-row-title').getByText(label, { exact: true }).click(); else await root().getByRole('button', { name: label, exact: true }).click();
  if (mobile) await verifyMobile(); else await root().getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor();
  const preview = await api(pathAPI + '/basis?' + new URLSearchParams({ fee_payment_id: fee.id, allocation_id: fixture.allocation.id })), basis = JSON.parse(preview.basis);
  assert.equal(hash(Buffer.from(preview.basis)), preview.basis_sha256); assert.equal(basis.cost.id, cost.id); assert.equal(basis.allocation.id, fixture.allocation.id);
  checks.push('Actual candidate pickers and server preview bind the exact original cost, cash, root allocation and SHA256.');
  const prefix = 'payment-fee-' + (mobile ? 'mobile' : 'web'), directory = path.join(app, '../docs/media/screenshots'), shots = [];
  await mkdir(directory, { recursive: true });
  async function shot(suffix) { await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png'; await page.screenshot({ path: path.join(directory, file), fullPage: false }); shots.push({ file: 'docs/media/screenshots/' + file, sha256: hash(await readFile(path.join(directory, file))) }); }
  await field('手续费操作依据').fill('核对本次银行实际手续费及双币核销凭证');
  assert.equal(await button('确认关联手续费依据').isDisabled(), true);
  checks.push('A reason and fresh MFA still require explicit human review.');
  await field('手续费操作依据').scrollIntoViewIfNeeded(); await shot('review');
  const reviewed = '已核对原费用、实际资金、整条原分配及凭证，确认为手续费依据';
  if (mobile) await selectMobile(page, '确认手续费依据', reviewed); else await root().getByLabel('确认手续费依据', { exact: true }).check();
  const body = { expected_version: p.version, revision_id: p.current_revision_id, content_hash: p.content_hash, reason: '核对本次银行实际手续费及双币核销凭证', expected_fee_version: preview.fee_version, fee_payment_id: fee.id, allocation_id: fixture.allocation.id, basis_sha256: preview.basis_sha256, reviewed: true };
  await api(pathAPI, { ...body, reviewed: false }, 422); await api(pathAPI, { ...body, basis_sha256: '0'.repeat(64) }, 409);
  checks.push('Real API rejects missing human review and altered source checksum.');
  const [savedResponse] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + pathAPI && r.request().method() === 'POST'), button('确认关联手续费依据').click()]);
  assert.equal(savedResponse.status(), 201); const saved = (await savedResponse.json()).data;
  assert.deepEqual(savedResponse.request().postDataJSON(), body); assert.equal(saved.basis, preview.basis); assert.equal(saved.version, 1);
  if (mobile) await button('查阅手续费依据').click();
  await root().getByText(mobile ? '有效关联' : cost.title + ' · 有效关联', { exact: true }).waitFor();
  checks.push('Explicit UI submission persists immutable evidence and displays its actual active history.');
  const replay = await api(pathAPI, body, 201, savedResponse.request().headers()['idempotency-key']); assert.equal(replay.id, saved.id); assert.equal((await api(pathAPI)).items.length, 1);
  checks.push('An exact HTTP replay adds no duplicate association.');
  assert.deepEqual({ p: await api('/payments/' + p.id), fee: await api('/payments/' + fee.id), cost: await api('/costs/' + cost.id), allocations: await api('/payments/' + fee.id + '/allocations?limit=100') }, before);
  checks.push('Association leaves every original cost, cash version, balance and project allocation unchanged.');
  await root().getByText(mobile ? '有效关联' : cost.title + ' · 有效关联', { exact: true }).scrollIntoViewIfNeeded(); await shot('linked');
  const recording = await recordViewport(page, app, prefix, async () => {
    await root().getByText(mobile ? '有效关联' : cost.title + ' · 有效关联', { exact: true }).scrollIntoViewIfNeeded(); await page.waitForTimeout(800);
    await page.mouse.wheel(0, 360); await page.waitForTimeout(800); await button('撤销此手续费关联').scrollIntoViewIfNeeded(); await page.waitForTimeout(800);
  });
  checks.push('Actual authenticated viewport recording shows the linked facts and human correction entry.');
  await button('撤销此手续费关联').click(); await field('手续费操作依据').fill('纠正误关联，保留原费用及真实资金记录');
  assert.equal(await button('确认撤销手续费关联').isDisabled(), true);
  if (mobile) await verifyMobile();
  if (mobile) await selectMobile(page, '确认手续费依据', '已核对准确关联，确认撤销并保留历史'); else await root().getByLabel('确认手续费依据', { exact: true }).check();
  const [revokedResponse] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + pathAPI + '/' + saved.id + '/revoke' && r.request().method() === 'POST'), button('确认撤销手续费关联').click()]);
  assert.equal(revokedResponse.status(), 200); const revoked = (await revokedResponse.json()).data;
  assert.equal(revoked.basis, saved.basis); assert.equal(revoked.revocation.version, 2); assert.equal(revoked.version, 1);
  if (mobile) await button('查阅手续费依据').click();
  await root().getByText(mobile ? '关联已撤销' : cost.title + ' · 关联已撤销', { exact: true }).waitFor();
  checks.push('Explicit UI revocation appends an independent version while keeping the original linked evidence.');
  assert.deepEqual({ p: await api('/payments/' + p.id), fee: await api('/payments/' + fee.id), cost: await api('/costs/' + cost.id), allocations: await api('/payments/' + fee.id + '/allocations?limit=100') }, before);
  checks.push('Revocation neither refunds real cash nor erases actual incurred fees.');
  await root().getByText(mobile ? '关联已撤销' : cost.title + ' · 关联已撤销', { exact: true }).scrollIntoViewIfNeeded(); await shot('revoked');
  await page.goto(url); await button('查阅手续费依据').click(); await root().getByText(mobile ? '关联已撤销' : cost.title + ' · 关联已撤销', { exact: true }).waitFor();
  assert.equal(await button('撤销此手续费关联').count(), 0); assert.equal((await api(pathAPI)).fee_version, 2);
  checks.push('A fresh page reads durable corrected history and removes the repeated revocation action.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots: shots, recording, synthetic_data: true, real_device: false, real_model: false, api_responses_mocked: false, fixture_reused_from: 'dedicated TestPaymentFeeEvidence with actual native PG finance records' }, null, 2) + '\n');
});
