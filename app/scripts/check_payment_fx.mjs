// Actual human finance controls, real API writes and native PG; no response substitutions.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { payment_fxBrowser, otp } from './payment_fx_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './payment_fx_recording.mjs';
const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
await payment_fxBrowser(mobile, async ({ page, context, fixture, checks, evidence, openPage }) => {
  const root = (p = page) => mobile ? mobileScreen(p) : p;
  const button = (name, p = page) => root(p).getByRole('button', { name, exact: true });
  const field = (name, p = page) => mobile ? root(p).locator('input[aria-label="' + name + '"],textarea[aria-label="' + name + '"]') : root(p).getByLabel(name, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  async function nextCode(who) {
    if (Math.floor(Date.now() / 30000) <= who.last_step) await page.waitForTimeout((who.last_step + 1) * 30000 - Date.now() + 250);
    who.last_step = Math.floor(Date.now() / 30000);
    return otp(who.mfa_secret, who.last_step);
  }
  async function login(p, who, mfa = false) {
    await p.goto(fixture.origin); await field('内部账号', p).fill(who.login); await field('密码', p).fill(who.password);
    await button(mobile ? '登录工作空间 ↗' : '登录工作空间', p).click();
    if (mfa) { await field('六位验证码', p).fill(await nextCode(who)); await button(mobile ? '验证并继续 ↗' : '验证并继续', p).click(); }
    await root(p).getByText('业务待办', { exact: true }).waitFor();
  }
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function headers() { return mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': (await context.cookies()).find(c => c.name === 'cp_csrf')?.value ?? '' }; }
  async function api(route, body, status = body ? 201 : 200) {
    const response = body ? await context.request.post(origin + '/v1' + route, { headers: { ...await headers(), 'Idempotency-Key': randomUUID() }, data: body }) : await context.request.get(origin + '/v1' + route, { headers: await headers() });
    const result = await response.json(); assert.equal(response.status(), status, route + ': ' + (result.error?.code ?? ''));
    return result.data;
  }
  await login(page, fixture.alice, true);
  const me = await api('/me'), roles = (await api('/roles?limit=100')).items;
  // A real role grant invalidates the subject's prior sessions. Use the
  // independent administrator, then explicitly authenticate Alice again.
  const administrator = await openPage();
  try {
    let token;
    async function adminPost(route, data, status = 200) {
      const response = await administrator.context.request.post(fixture.api_origin + '/v1' + route,
        { headers: { Origin: fixture.origin, 'Idempotency-Key': randomUUID(), ...(token ? { Authorization: 'Bearer ' + token } : {}) }, data });
      const result = await response.json(); assert.equal(response.status(), status, route + ': ' + (result.error?.code ?? ''));
      return result.data;
    }
    const challenge = await adminPost('/auth/login', { login: fixture.administrator.login, password: fixture.administrator.password, device_type: 'android' });
    assert.equal(challenge.step, 'mfa');
    const authenticated = await adminPost('/auth/mfa/verify', { challenge: challenge.challenge, code: await nextCode(fixture.administrator) });
    assert.equal(authenticated.step, 'authenticated'); token = authenticated.access_token;
    for (const key of ['payment_recorder', 'payment_reverser', 'cost_recorder', 'cost_confirmer', 'party_manager', 'cash_reporter']) {
      const role = roles.find(r => r.key === key); assert.ok(role, key);
      await adminPost('/role-bindings', { user_id: me.user.id, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', reason: '仅隔离合成演示的明确业务授权' }, 201);
    }
  } finally { await administrator.page.close(); await administrator.context.close(); }
  await context.clearCookies();
  if (mobile) await page.evaluate(() => localStorage.clear());
  await login(page, fixture.alice, true);
  const file = await api('/files/' + fixture.transfer_file_id), version = (await api('/files/' + file.id + '/versions?limit=20')).items[0];
  const party = await api('/parties', { name: '合成跨币结算供应商', kind: 'organization', relationships: ['supplier'], contact_name: '', email: '', phone: '', notes: '', project_id: file.project_id, classification: 'project_restricted', reason: '隔离合成供应商' });
  const base = value => ({ expected_version: value.version, revision_id: value.current_revision_id, content_hash: value.content_hash, reason: '核对真实双币结算依据' });
  const counterparty = { party_id: party.id, revision_id: party.current_revision_id };
  let cost = await api('/costs', { title: '合成 USD 服务费用', occurred_on: '2025-01-15', currency: 'USD', amount: '100', category: 'settlement116', counterparty, allocations: [{ project_id: file.project_id, amount: '60' }, { project_id: null, amount: '40' }], evidence_version_ids: [version.id], notes: '仅隔离合成数据', source_key: 'browser-fx-cost:' + randomUUID(), project_id: file.project_id, classification: 'project_restricted', reason: '合成原币费用' });
  cost = await api('/costs/' + cost.id + '/confirm', base(cost), 200);
  let payment = await api('/payments', { title: '合成 HKD 实付本金', direction: 'outgoing', paid_at: '2025-02-02T12:00:00+08:00', currency: 'HKD', amount: '780', category: 'settlement116', counterparty, employee_id: null, refund_of_id: null, method_label: '合成银行结算', external_ref: '', evidence_version_ids: [version.id], notes: '非真实业务流水', source_key: 'browser-fx-bank:' + randomUUID(), project_id: file.project_id, classification: 'project_restricted', reason: '合成实际付款' });
  payment = await api('/payments/' + payment.id + '/record', { ...base(payment), expected_source_version: null, refund_allocations: [] }, 200);
  const originalFacts = { revision: payment.current_revision_id, hash: payment.content_hash, amount: payment.amount, currency: payment.currency };
  checks.push({ name: 'Existing owned fixture receives explicit synthetic finance grants and real independent USD cost/HKD cash facts', passed: true });
  async function open(action) {
    await page.goto(fixture.origin + (mobile ? '/#/pages/finance/index?kind=payment&id=' + payment.id + (action ? '&action=' + action : '') : '/payments/' + payment.id));
    if (!mobile && action) await button(action === 'allocate' ? '分配费用结算' : '释放此分配').click();
  }
  async function waitMobilePaymentDetail() {
    // The command validates its ledger receipt before redirecting. Wait for
    // that real navigation; forcing another hash navigation races Taro.
    await page.waitForURL(url => url.hash === '#/pages/finance/index?kind=payment&id=' + payment.id);
    await root().getByText('资金记录', { exact: true }).waitFor();
    await root().getByText('当前可读分配流水', { exact: true }).waitFor();
  }
  async function confirmMFA() {
    if (!mobile) { await root().getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。', { exact: true }).waitFor(); return; }
    await field('验证器六位验证码').fill(await nextCode(fixture.alice)); await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  }
  async function acknowledge() {
    if (mobile) await selectMobile(page, '人工核实声明', '已核对原事实、凭证、双币金额、汇率方向与所选依据');
    else await root().getByRole('checkbox', { name: '我已核对两种币金额、汇率方向、凭证来源与本次回转金额', exact: true }).check();
  }
  await open('allocate');
  await button(mobile ? '选择准确结算费用' : '选择结算费用').click();
  if (mobile) await root().getByText(cost.title + ' · USD 100.000000', { exact: true }).click();
  else await root().getByRole('button').filter({ hasText: cost.title }).click();
  await field('本次付款币本金（HKD）').fill('780'); await field('费用原币核销金额（USD）').fill('100');
  await field('实际汇率（每1 USD兑换 HKD）').fill('7.7'); await field('结算汇率凭证来源').fill('合成银行付款凭证，人工复核');
  await field(mobile ? '财务操作依据' : '操作依据').fill('人工复核 USD100 对应 HKD780 的准确本金。');
  await confirmMFA(); await acknowledge();
  await button('分配费用结算').last().click();
  if (mobile) {
    const rejection = root().locator('.cp-notice.cp-danger').first();
    await rejection.scrollIntoViewIfNeeded();
    assert.match(await rejection.innerText(), /费用原币金额乘实际汇率须按付款币精度舍入后等于本次付款本金。/);
  } else await root().getByText('费用原币金额乘实际汇率须按付款币精度舍入后等于本次付款本金。', { exact: false }).waitFor();
  assert.equal((await api('/payments/' + payment.id)).version, payment.version);
  checks.push({ name: 'Actual ' + (mobile ? 'shared mobile H5' : 'Web') + ' form rejects a mismatched manual quote without writing a settlement', passed: true });
  await field('实际汇率（每1 USD兑换 HKD）').fill('7.8'); await acknowledge();
  const prefix = 'payment-fx-' + (mobile ? 'mobile' : 'web'), shots = [], directory = path.join(app, '../docs/media/screenshots');
  await mkdir(directory, { recursive: true });
  async function shot(suffix) { await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png'; await page.screenshot({ path: path.join(directory, file), fullPage: false }); shots.push({ file: 'docs/media/screenshots/' + file, sha256: hash(await readFile(path.join(directory, file))) }); }
  await field('实际汇率（每1 USD兑换 HKD）').scrollIntoViewIfNeeded(); await shot('quote');
  const viewport = page.viewportSize(); await page.setViewportSize({ width: mobile ? 360 : 760, height: 1000 });
  await field('实际汇率（每1 USD兑换 HKD）').scrollIntoViewIfNeeded();
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 2)); await shot('narrow');
  const recording = await recordViewport(page, app, prefix, async () => { await field('结算汇率凭证来源').scrollIntoViewIfNeeded(); await page.waitForTimeout(1200); await field('本次付款币本金（HKD）').scrollIntoViewIfNeeded(); await page.waitForTimeout(1200); });
  await page.setViewportSize(viewport);
  const [allocated] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1/payments/' + payment.id + '/allocations' && r.request().method() === 'POST'), button('分配费用结算').last().click()]);
  assert.equal(allocated.status(), 200); payment = (await allocated.json()).data;
  let rows = (await api('/payments/' + payment.id + '/allocations?limit=100')).items, allocation = rows.find(v => v.kind === 'allocation');
  assert.equal(allocation.amount, '780.000000'); assert.equal(allocation.cost_amount, '100.000000'); assert.equal(allocation.fx.rate, '7.8000000000'); assert.equal(allocation.cost_shares.length, 2);
  assert.equal((await api('/costs/' + cost.id)).paid_amount, '100.000000');
  checks.push({ name: 'Explicit manual quote and MFA commit exact dual principal, provenance and aligned project shares', passed: true });
  if (mobile) await waitMobilePaymentDetail();
  await open('release');
  if (mobile) { await button('选择准确原分配').click(); await root().getByText('费用 ' + cost.id + ' · 可用 780.000000', { exact: true }).click(); }
  await field('本次付款币本金（HKD）').fill('390'); await field(mobile ? '财务操作依据' : '操作依据').fill('核对双币剩余额，释放 HKD390 与 USD50。');
  await root().getByText(/本次回转：390 HKD → 50.000000 USD/).waitFor(); await confirmMFA(); await acknowledge();
  const [released] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1/payments/' + payment.id + '/allocations/' + allocation.id + '/release' && r.request().method() === 'POST'), button('释放费用分配').last().click()]);
  assert.equal(released.status(), 200); payment = (await released.json()).data;
  rows = (await api('/payments/' + payment.id + '/allocations?limit=100')).items; allocation = rows.find(v => v.kind === 'allocation');
  assert.equal(allocation.remaining_amount, '390.000000'); assert.equal(allocation.remaining_cost_amount, '50.000000'); assert.equal(rows.find(v => v.kind === 'release').cost_amount, '50.000000');
  checks.push({ name: 'Human partial release displays and returns HKD390/USD50 then commits both remaining balances', passed: true });
  if (mobile) await waitMobilePaymentDetail(); else await open();
  await root().getByText(/费用原币/).first().waitFor(); await root().getByText(/每1 USD兑换/).last().scrollIntoViewIfNeeded(); await shot('ledger');
  const actual = await api('/payments/' + payment.id), incurred = await api('/costs/' + cost.id);
  assert.deepEqual({ revision: actual.current_revision_id, hash: actual.content_hash, amount: actual.amount, currency: actual.currency }, originalFacts);
  assert.equal(actual.effective_allocated_amount, '390.000000'); assert.equal(incurred.paid_amount, '50.000000'); assert.equal(incurred.outstanding_amount, '50.000000');
  checks.push({ name: 'Actual ledger UI preserves original cash facts and separates cash HKD390 from incurred USD50', passed: true });
  const denied = await openPage({ viewport: mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 } });
  await login(denied.page, fixture.bob); await denied.page.goto(fixture.origin + (mobile ? '/#/pages/finance/index?kind=payment&id=' : '/payments/') + payment.id);
  await root(denied.page).getByText(/不存在|无权|不可见|权限|未找到/).first().waitFor();
  assert.equal(await root(denied.page).getByRole('button', { name: '分配费用结算', exact: true }).count(), 0);
  checks.push({ name: 'A separately authenticated reader without finance authority cannot access the actual cash or allocation controls', passed: true });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots: shots, recording, synthetic_data: true, real_device: false, api_responses_mocked: false, fixture_reused_from: 'existing human-file-scope-transfer native fixture; finance grants and data created through actual APIs' }, null, 2) + '\n');
});
