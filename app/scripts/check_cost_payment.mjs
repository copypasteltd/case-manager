// Actual human form -> composed ordinary cash/settlement commands -> native PG.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './project_business_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url)), mobile = process.argv.includes('--mobile');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = label => root().getByRole('button', { name: label, exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const get = async route => { const r = await context.request.get((mobile ? fixture.api_origin : fixture.origin) + '/v1' + route, { headers: mobile ? { Authorization: bearer } : {} }); assert.equal(r.status(), 200, route); return (await r.json()).data; };
  const navigate = route => page.goto(fixture.origin + (mobile ? '/#' : '') + route);
  const file = await get('/files/' + fixture.file_id), version = await get('/files/' + fixture.file_id + '/versions/' + fixture.file_version_id);
  const screenshots = [], created = [], prefix = 'cost-payment-' + (mobile ? 'mobile' : 'web');
  await mkdir(path.join(app, '../docs/media/screenshots'), { recursive: true });
  async function shot(name) { await assertNoCredentials(page); const file = 'docs/media/screenshots/' + prefix + '-' + name + '.png'; await page.screenshot({ path: path.join(app, '..', file) }); screenshots.push({ file, sha256: createHash('sha256').update(await readFile(path.join(app, '..', file))).digest('hex') }); }
  let recording;
  for (const item of fixture.payment_costs) {
    const before = await get('/costs/' + item.id), foreign = item.scenario === 'foreign', employee = item.scenario === 'employee';
    if (mobile) {
      await navigate('/pages/home/index'); await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
      await root().getByText('实际费用', { exact: true }).click(); await root().getByText(item.title, { exact: true }).click(); await root().locator('.cp-title').filter({ hasText: item.title }).waitFor();
    } else { await navigate('/costs/' + item.id); await page.getByRole('heading', { name: item.title, exact: true }).waitFor(); }
    await button('登记付款并核销费用').click();
    await field('付款标题').fill('实际付款核销 · ' + item.scenario + ' · ' + (mobile ? '移动端' : 'Web'));
    assert.equal(await field('实际付款总额').inputValue(), ''); assert.equal(await field('本次核销付款额').inputValue(), ''); assert.equal(await field('实际付款香港时间').inputValue(), '');
    await field('实际付款香港时间').fill('2025-02-10T09:30:00');
    if (foreign) { if (mobile) await selectMobile(page, '实际付款币种', 'USD'); else await page.getByLabel('实际付款币种', { exact: true }).selectOption('USD'); }
    await field('实际付款总额').fill(foreign ? '13' : employee ? '30' : '60'); await field('本次核销付款额').fill(foreign ? '6.50' : employee ? '25' : '40');
    await field('实际付款方式').fill('已完成银行转账'); await field('外部流水标识').fill('synthetic-reviewed-' + item.scenario);
    if (foreign) { await field('本次费用原币核销额').fill('50'); await field('每1费用原币兑换付款币种').fill('0.13'); await field('实际汇率来源').fill('已核对银行实际兑换凭证'); }
    if (employee) {
      if (mobile) await selectMobile(page, '实际收款类型', '已批准员工垫付报销'); else await page.getByLabel('实际收款类型', { exact: true }).selectOption('employee');
      const claim = await get('/claims/' + item.source_id); await button('选择批准垫付报销').click();
      await root().getByRole('button', { name: new RegExp(claim.title) }).click();
      if (mobile) await selectMobile(page, '批准垫付明细', claim.lines[0].description + ' · ' + claim.currency + ' ' + claim.lines[0].declared_amount); else await page.getByLabel('批准垫付明细', { exact: true }).selectOption(claim.lines[0].id);
    } else { await button('选择实际收款供应方').click(); await root().getByRole('button', { name: new RegExp(fixture.supplier_name) }).click(); }
    await button('选择付款凭证版本').click(); await root().getByRole('button').filter({ hasText: file.name }).first().click();
    if (mobile) await button('使用固定版本 ' + version.seq).click(); else await page.getByRole('button', { name: new RegExp('^版本 ' + version.seq + ' ·') }).click();
    await field('付款核销登记原因').fill('已核对真实付款与本笔准确费用，剩余款保留未分配。');
    if (mobile) {
      await selectMobile(page, '我已核对实际付款时间、总额、币种、收款方及凭证', '已核对实际付款'); await selectMobile(page, '我已核对本笔费用、核销金额、报销或异币依据', '已核对核销依据');
      fixture.alice.last_step = Math.floor(Date.now() / 30000); await field('验证器六位验证码').fill(await waitOTP(fixture)); await button('确认本人操作').click(); await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
    } else { await page.getByLabel('我已核对实际付款时间、总额、币种、收款方及凭证', { exact: true }).check(); await page.getByLabel('我已核对本笔费用、核销金额、报销或异币依据', { exact: true }).check(); await page.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor(); }
    const submit = async () => {
      const pending = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/payments/from-cost' && r.request().method() === 'POST'); void pending.catch(() => {}); await button('确认实际付款并核销').click(); const r = await pending; assert.equal(r.status(), 201, await r.text()); const { payment: p, allocation: a } = (await r.json()).data;
      assert.equal(p.version, 3); assert.equal(p.state, 'recorded'); assert.equal(p.project_id, fixture.project_id); assert.equal(p.classification, before.classification); assert.equal(a.cost_id, before.id); assert.equal(a.cost_revision_id, before.current_revision_id); assert.deepEqual(p.evidence_version_ids, [fixture.file_version_id]);
      assert.equal(p.unallocated_amount, foreign ? '6.500000' : employee ? '5.000000' : '20.000000'); assert.equal(a.cost_amount, foreign ? '50.000000' : employee ? '25.000000' : '40.000000');
      if (employee) { assert.equal(p.employee_id, fixture.employee_id); assert.equal(a.claim_id, item.source_id); assert.ok(a.claim_line_id); } else { assert.equal(p.counterparty.party_id, fixture.supplier_id); assert.equal(a.claim_id, null); }
      if (foreign) { assert.equal(p.currency, 'USD'); assert.equal(a.cost_currency, 'HKD'); assert.equal(a.fx.rate, '0.1300000000'); }
      if (mobile) await root().getByText('资金记录', { exact: true }).waitFor(); else await page.getByRole('heading', { name: '准确资金事实', exact: true }).waitFor();
      if (mobile) await root().getByText('当前可读分配流水', { exact: true }).scrollIntoViewIfNeeded(); else await page.getByRole('heading', { name: '不可变费用分配', exact: true }).scrollIntoViewIfNeeded();
      await root().getByText('已核对真实付款与本笔准确费用，剩余款保留未分配。', { exact: true }).first().waitFor(); await shot(item.scenario);
      const events = await get('/payments/' + p.id + '/events?limit=20'); assert.deepEqual([...events.items].sort((a, b) => a.version - b.version).map(v => v.kind), ['draft_created', 'recorded', 'allocation_added']);
      const after = await get('/costs/' + before.id); assert.equal(after.version, before.version); assert.equal(after.current_revision_id, before.current_revision_id); assert.equal(after.amount, '100.000000'); assert.equal(after.paid_amount, a.cost_amount); assert.equal(after.outstanding_amount, foreign ? '50.000000' : employee ? '75.000000' : '60.000000');
      if (item.scenario === 'supplier') {
        if (mobile) { await button('查看对应费用当前资料').click(); await root().getByText('资金结算核对', { exact: true }).scrollIntoViewIfNeeded(); }
        else { await page.getByRole('link', { name: '核对准确结算费用', exact: true }).click(); await page.getByRole('heading', { name: '准确费用事实', exact: true }).waitFor(); }
      }
      created.push({ scenario: item.scenario, payment_id: p.id, allocation_id: a.id, cost_id: before.id, currency: p.currency, actual_cash: p.amount, allocation_cash: a.amount, allocation_cost: a.cost_amount, unassigned_cash: p.unallocated_amount, cost_outstanding: after.outstanding_amount });
    };
    if (item.scenario === 'supplier') recording = await recordViewport(page, app, prefix, submit); else await submit();
    checks.push('Actual ' + (mobile ? 'Taro H5' : 'Web') + ' ' + item.scenario + ' path records actual cash and a separately reviewed exact expense settlement, preserves unassigned cash and ordinary immutable journals.');
  }
  const incurred = await get('/reports/expenses?kind=incurred&category=cost-payment&from=2025-02-01&to=2025-03-01'); assert.equal(incurred.totals[0].net_amount, '300.000000');
  const cash = await get('/reports/cash?category=cost-payment&from=2025-02-01&to=2025-03-01');
  checks.push('Three original incurred expenses stay HKD300. Actual cash and dual settlement are independently retained; no extra incurred expense is fabricated.');
  assert.ok(recording && screenshots.length === 3 && created.length === 3);
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, created, cash_totals: cash.totals, synthetic_data: true, real_model: false, real_cloud: false, mfa_recorded: false }, null, 2) + '\n');
}, 'cost-payment');
