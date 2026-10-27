import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, otp } from './notification_browser_common.mjs';
import { mobileScreen, selectMobile, stableMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './mobile_finance_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(true, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobileScreen(page), escape = v => v.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const button = name => root().getByRole('button', { name: new RegExp('^' + escape(name) + '(?:\\s*↗)?$') });
  const field = name => root().locator('input[aria-label="' + name + '"],textarea[aria-label="' + name + '"]');
  const title = name => root().locator('.cp-row-title').getByText(name, { exact: true });
  const recordings = [], receipts = [], layouts = [];
  async function nextCode(account) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await new Promise(resolve => setTimeout(resolve, 500));
    account.last_step = Math.floor(Date.now() / 30000); return otp(account.mfa_secret, account.last_step);
  }
  async function raw(route, method = 'GET', body, token) {
    const response = await context.request.fetch(fixture.api_origin + '/v1' + route, { method,
      headers: { Origin: fixture.origin, ...(token ? { Authorization: 'Bearer ' + token } : {}), 'Idempotency-Key': randomUUID() }, ...(body ? { data: body } : {}) });
    const value = await response.json(); assert.ok(response.ok(), route + ': ' + response.status() + '/' + value.error?.code); return value.data;
  }
  async function apiLogin(account) {
    const login = await raw('/auth/login', 'POST', { login: account.login, password: account.password, device_type: 'android' });
    assert.equal(login.step, 'mfa'); const verified = await raw('/auth/mfa/verify', 'POST', { challenge: login.challenge, code: await nextCode(account) });
    assert.equal(verified.step, 'authenticated'); return verified.access_token;
  }
  const adminToken = await apiLogin(fixture.administrator), admin = (r, m, b) => raw(r, m, b, adminToken);
  const roles = await admin('/roles?limit=100');
  const aliceLogin = await apiLogin(fixture.alice), bobLogin = await apiLogin(fixture.bob);
  const alice = (await raw('/me', 'GET', undefined, aliceLogin)).user, bob = (await raw('/me', 'GET', undefined, bobLogin)).user;
  const bindings = [], existing = await admin('/role-bindings?limit=100');
  for (const [user, keys] of [[alice.id, ['party_manager', 'cost_recorder', 'cost_confirmer', 'cost_reverser', 'payment_recorder', 'payment_reverser', 'finance_reporter', 'cash_reporter', 'claim_submitter', 'purchase_manager']], [bob.id, ['approval_reviewer', 'claim_reader', 'purchase_reader']]]) {
    for (const key of keys) {
      const role = roles.items.find(r => r.key === key); assert.ok(role, 'Actual seeded role ' + key);
      if (existing.items.some(b => b.user_id === user && b.role_id === role.id && b.scope_type === 'org')) continue;
      const binding = await admin('/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离移动财务完整流程权限' }); bindings.push({ ...binding, key, user });
    }
  }
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization.slice(7); });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password); await button('登录工作空间').click(); await field('六位验证码').fill(await nextCode(fixture.alice)); await button('验证并继续').click();
  await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); assert.ok(bearer);
  const api = (r, m, b) => raw(r, m, b, bearer), reviewerToken = await apiLogin(fixture.bob), reviewer = (r, m, b) => raw(r, m, b, reviewerToken);
  const me = await api('/me'), project = await api('/projects/' + fixture.project_id);
  const supplier = await api('/parties', 'POST', { name: '合成移动财务供应方', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离财务流程' });
  const files = await api('/files?limit=20&project_id=' + fixture.project_id), file = files.items.find(f => f.current_version_id); assert.ok(file);
  const version = await api('/files/' + file.id + '/versions?limit=20'); const formal = version.items.find(v => v.id === file.current_version_id); assert.ok(formal);
  const base = (v, reason) => ({ expected_version: v.version, revision_id: v.current_revision_id, content_hash: v.content_hash, reason });
  const media = path.join(app, '../docs/media/screenshots'); await mkdir(media, { recursive: true });
  async function openList(kind) {
    await page.evaluate(() => { location.hash = '/pages/home/index'; }); await stableMobile(page);
    await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
    await title(kind === 'cost' ? '实际费用' : '收付款').click();
    await button(kind === 'cost' ? '起草实际费用' : '起草实际资金').waitFor();
  }
  async function openRecord(kind, record) { await openList(kind); await title(record.title).click(); await button('刷新业务资料').waitFor(); }
  async function current(kind) {
    await root().locator('.cp-heading .cp-title').waitFor();
    await button('刷新业务资料').waitFor();
    const currentURL = new URL(page.url()), route = currentURL.hash ? currentURL.hash.slice(1) : currentURL.pathname + currentURL.search, id = new URL(route, 'http://review.invalid').searchParams.get('id'); assert.ok(id);
    return api((kind === 'cost' ? '/costs/' : '/payments/') + id);
  }
  async function reviewAndSubmit(name) {
    await field('财务操作依据').fill('隔离移动核实 · ' + name); await selectMobile(page, '人工核实声明', '已核对原事实、凭证、金额与所选依据');
    await field('验证器六位验证码').fill(await nextCode(fixture.alice)); await button('确认本人操作').click(); await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
    await button(name).click(); await button('刷新业务资料').waitFor();
  }
  async function pickFile() { await button('选择财务凭证版本').click(); await title(file.name).click(); await button('使用固定版本 ' + formal.seq).click(); }
  async function pickSupplier() { await button('选择固定往来方').click(); await title(supplier.name).click(); }
  async function startDraft(kind, name, employee = false) {
    if (kind === 'cost') { await button('起草实际费用').click(); } else await button('起草实际资金').click();
    await field('财务记录标题').fill(name); await selectMobile(page, '资料分类', '项目受限'); await button('选择所属项目').click(); await title(project.name).click();
    await field('原币').fill('HKD'); await field('实际原币金额').fill('100'); await field('财务类别').fill('mobile.finance.browser');
    await field(kind === 'cost' ? '费用发生日期' : '实际发生时间（香港）').fill(kind === 'cost' ? '2025-01-15' : '2025-02-01T02:00:00');
    if (kind === 'payment') { await selectMobile(page, '资金方向', '支出'); await field('付款方式标签').fill('bank_transfer'); }
    if (employee) { await button('从批准报销选择员工').click(); await title('合成员工报销依据').click(); } else await pickSupplier();
    if (kind === 'cost') { await button('添加费用分摊').click(); await field('分摊金额 1').fill('60'); await button('添加费用分摊').click(); await button('设为公司分摊 2').click(); await field('分摊金额 2').fill('40'); }
    await pickFile(); await field('登记或修订原因').fill('隔离草稿完整录入'); await button('保存财务草稿').click();
    const record = await current(kind); assert.equal(record.state, 'draft'); assert.equal(record.amount, '100.000000'); assert.equal(record.project_id, project.id); assert.deepEqual(record.evidence_version_ids, [formal.id]); return record;
  }
  async function approved(kind, name) {
    await admin('/workflow-definitions', 'POST', { expected_version: 0, key: 'mobile.finance.' + kind, name: '合成移动财务独立核对', kind, category: 'mobile.finance.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立审批', assigned_user_id: bob.id, requires_mfa: false }], reason: '隔离完整业务依据' });
    const shared = { title: name, category: 'mobile.finance.browser', purpose: '核对同一规范费用与资金', owner_id: me.user.id, currency: 'HKD', project_id: project.id, classification: 'project_restricted', reason: '隔离业务依据', notes: '', rounding_mode: 'half_up' };
    const body = kind === 'purchase' ? { ...shared, pricing_basis: 'net_plus_tax', expected_delivery_date: '2025-01-15', supplier_candidates: [{ party_id: supplier.id, revision_id: supplier.current_revision_id }], quote_version_ids: [formal.id], lines: [{ id: randomUUID(), description: '合成采购服务', quantity: '1', unit: '项', unit_price: '100', tax_amount: '0', discount_amount: '0' }] } : { ...shared, pricing_basis: 'gross_includes_tax', period_start: '2025-01-15', period_end: '2025-01-15', lines: [{ id: randomUUID(), description: '合成员工垫付行', declared_amount: '100', expense_date: '2025-01-15', category: 'mobile.finance.browser', supplier: { party_id: supplier.id, revision_id: supplier.current_revision_id }, receipt_version_ids: [formal.id], funding: 'employee_advance' }] };
    const route = kind === 'purchase' ? '/purchases' : '/claims'; let record = await api(route, 'POST', body);
    let request = await api('/requests', 'POST', { kind, business_id: record.id, expected_business_version: record.version, business_revision_id: record.current_revision_id, business_content_hash: record.content_hash, reason: '固定业务依据' });
    const preview = await api('/requests/' + request.id + '/submission-preview'); request = await api('/requests/' + request.id + '/submit', 'POST', { ...base(request, '提交独立核对'), workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash });
    await reviewer('/approval-steps/' + request.steps[0].id + '/decisions', 'POST', { expected_version: request.steps[0].version, request_version: request.version, revision_id: request.current_revision_id, content_hash: request.content_hash, decision: 'approve', reason: '独立审批通过' });
    record = await api(route + '/' + record.id); assert.equal(record.approval_state, 'approved'); return record;
  }

  await title('实际费用').click(); await button('起草实际费用').waitFor();
  recordings.push(await recordViewport(page, app, 'mobile-finance-cost-draft', async () => {
    await startDraft('cost', '合成移动费用 · 100');
  }));
  let cost = await current('cost'); assert.deepEqual(cost.allocations.map(v => v.amount), ['60.000000', '40.000000']);
  checks.push('Actual mobile login, list entry and full cost draft preserve exact HKD100, project60/company40 and one fixed file; saving remains draft.'); console.log('check: cost draft');

  await button('修订财务草稿').click(); await field('财务备注').fill('并发前的保留输入'); await field('登记或修订原因').fill('核对冲突');
  const changed = await api('/costs/' + cost.id, 'PATCH', { title: cost.title, occurred_on: cost.occurred_on, currency: cost.currency, amount: cost.amount, category: cost.category, counterparty: cost.counterparty, allocations: cost.allocations, evidence_version_ids: cost.evidence_version_ids, notes: '另一次真实修订', ...base(cost, '另设备修订') });
  await button('保存财务草稿').click(); await root().getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(await field('财务备注').inputValue(), '并发前的保留输入'); assert.equal((await api('/costs/' + cost.id)).version, changed.version);
  await button('返回，保留文字输入').click(); cost = await current('cost');
  await button('修订财务草稿').click(); await field('财务备注').fill('成功已提交，响应连接中断'); await field('登记或修订原因').fill('验证未知结果');
  let writes = 0; const target = fixture.api_origin + '/v1/costs/' + cost.id;
  await page.route(target, async route => { if (route.request().method() === 'PATCH') { writes++; await route.fetch(); await route.abort('failed'); } else await route.continue(); });
  await button('保存财务草稿').click(); await root().getByText(/输入已保留。请先刷新资料/).waitFor(); assert.equal(writes, 1); assert.equal(await field('财务备注').inputValue(), '成功已提交，响应连接中断');
  await page.unroute(target); assert.equal((await api('/costs/' + cost.id)).version, cost.version + 1); await button('返回，保留文字输入').click(); cost = await current('cost');
  checks.push('Concurrent editing rejects the old basis without losing text; an actually committed but lost PATCH response is blocked and never resent.'); console.log('check: conflicts and unknown result');

  await button('确认实际费用').click(); await reviewAndSubmit('确认实际费用'); cost = await current('cost'); assert.equal(cost.state, 'confirmed');
  const purchase = await approved('purchase', '合成采购批准依据'); await button('关联批准业务依据').click(); await button('选择批准业务依据').click(); await title(purchase.title).click(); await reviewAndSubmit('关联批准业务依据'); cost = await current('cost'); assert.ok(cost.links.some(v => v.source_id === purchase.id && v.source_revision_id === purchase.approved_revision_id));
  await assertNoCredentials(page); await page.screenshot({ path: path.join(media, 'mobile-finance-cost-confirmed.png'), fullPage: false });
  checks.push('Mobile confirmation uses fresh MFA and preserves the facts; an independently approved purchase is linked by its exact revision without a second cost.'); console.log('check: cost confirmed and purchase linked');

  await openList('payment'); await button('起草实际资金').waitFor();
  recordings.push(await recordViewport(page, app, 'mobile-finance-payment-draft', async () => { await startDraft('payment', '合成移动资金 · 100'); }));
  let payment = await current('payment'); assert.equal(payment.paid_at, '2025-01-31T18:00:00Z');
  await button('登记实际资金').click(); await reviewAndSubmit('登记实际资金'); payment = await current('payment'); assert.equal(payment.state, 'recorded'); assert.equal(payment.paid_on, '2025-02-01');
  await button('分配费用结算').click(); await button('选择准确结算费用').click(); await title(cost.title + ' · HKD 100.000000').click(); await field('本次分配金额').fill('60'); await reviewAndSubmit('分配费用结算'); payment = await current('payment'); assert.equal(payment.effective_allocated_amount, '60.000000');
  cost = await api('/costs/' + cost.id); assert.equal(cost.paid_amount, '60.000000'); assert.equal(cost.outstanding_amount, '40.000000');
  await button('释放费用分配').click(); await button('选择准确原分配').click(); await root().locator('.cp-row-title').filter({ hasText: cost.id }).first().click(); await field('本次分配金额').fill('20'); await reviewAndSubmit('释放费用分配'); payment = await current('payment'); assert.equal(payment.effective_allocated_amount, '40.000000');
  checks.push('Actual mobile cash registration preserves Hong Kong February1 and UTC January31; allocating60 then releasing20 yields exactly40 settled and60 outstanding.'); console.log('check: cash and allocation/release');
  await assertNoCredentials(page); await page.screenshot({ path: path.join(media, 'mobile-finance-payment-allocations.png'), fullPage: false });

  await button('起草收到退款').click(); await field('财务记录标题').fill('合成收到退款 · 10'); await field('实际发生时间（香港）').fill('2025-02-03T10:00:00'); await field('实际原币金额').fill('10'); await field('付款方式标签').fill('bank_transfer'); await pickFile(); await field('登记或修订原因').fill('确实收到退款'); await button('保存财务草稿').click(); let refund = await current('payment'); assert.equal(refund.refund_of_id, payment.id);
  await button('登记实际资金').click(); await button('读取原付款与退款分配').click(); await root().locator('.cp-row-title').filter({ hasText: cost.id }).first().click(); await field('原分配退款金额 1').fill('5'); await reviewAndSubmit('登记实际资金'); refund = await current('payment'); assert.equal(refund.state, 'recorded');
  await button('关联退款分配').click(); await button('选择准确原分配').click(); await root().locator('.cp-row-title').filter({ hasText: cost.id }).first().click(); await field('本次分配金额').fill('5'); await reviewAndSubmit('关联退款分配'); refund = await current('payment');
  payment = await api('/payments/' + payment.id); cost = await api('/costs/' + cost.id); assert.equal(payment.refunded_amount, '10.000000'); assert.equal(payment.effective_allocated_amount, '30.000000'); assert.equal(cost.paid_amount, '30.000000');
  await assertNoCredentials(page); await page.screenshot({ path: path.join(media, 'mobile-finance-refund.png'), fullPage: false });
  checks.push('A real separate refund draft fixes the original payment, records5 against its original allocation and later associates5 more; original refund10 and net settlement30 reconcile.'); console.log('check: refund and late mapping');

  const claim = await approved('claim', '合成员工报销依据');
  await openList('cost'); await button('起草实际费用').waitFor(); let employeeCost = await startDraft('cost', '合成移动员工费用 · 100');
  await button('确认实际费用').click(); await reviewAndSubmit('确认实际费用'); employeeCost = await current('cost');
  await button('关联批准业务依据').click(); await selectMobile(page, '关联业务类型', '报销'); await button('选择批准业务依据').click(); await title(claim.title).click(); await reviewAndSubmit('关联批准业务依据'); employeeCost = await current('cost');
  await openList('payment'); await button('起草实际资金').waitFor(); let employeePayment = await startDraft('payment', '合成移动员工结算 · 100', true); assert.equal(employeePayment.employee_id, me.user.id);
  await button('登记实际资金').click(); await reviewAndSubmit('登记实际资金'); employeePayment = await current('payment');
  await button('分配费用结算').click(); await button('选择准确批准报销').click(); await title(claim.title).click(); await selectMobile(page, '员工垫付明细', '合成员工垫付行'); await button('选择准确结算费用').click(); await title(employeeCost.title + ' · HKD 100.000000').click(); await field('本次分配金额').fill('60'); await reviewAndSubmit('分配费用结算'); employeePayment = await current('payment');
  const settlement = await api('/claims/' + claim.id + '/settlement'); assert.equal(settlement.state, 'partial'); assert.equal(settlement.paid_amount, '60.000000'); assert.equal(settlement.outstanding_amount, '40.000000');
  await assertNoCredentials(page); await page.screenshot({ path: path.join(media, 'mobile-finance-employee-settlement.png'), fullPage: false });
  checks.push('Independent claim approval, mobile expense linking and employee settlement fix the original applicant, advance line, supplier and approved revision; claim100 settles60 with40 outstanding.'); console.log('check: employee claim journey');

  await button('全额冲销原记录').click(); await field('冲销生效时间（香港）').fill('2025-02-10T10:00:00'); await reviewAndSubmit('全额冲销原记录'); employeePayment = await current('payment'); assert.equal(employeePayment.state, 'reversed');
  const reversedCash = await api('/payments/' + employeePayment.reversal_id); assert.equal(reversedCash.reverses_id, employeePayment.id); assert.equal(reversedCash.paid_at, '2025-02-10T02:00:00Z');
  const correctedSettlement = await api('/claims/' + claim.id + '/settlement'); assert.equal(correctedSettlement.paid_amount, '0.000000'); assert.equal(correctedSettlement.outstanding_amount, '100.000000');
  checks.push('A mobile full cash reversal creates the exact Hong Kong effective-time reverse fact and reverses the employee allocation; claim settlement returns to0/100 with history retained.'); console.log('check: cash reversal and claim correction');

  await openRecord('payment', payment); await button('释放费用分配').waitFor(); await button('释放费用分配').click(); await button('选择准确原分配').click(); await root().locator('.cp-row-title').filter({ hasText: cost.id }).first().click(); await field('本次分配金额').fill('30'); await reviewAndSubmit('释放费用分配');
  await openRecord('cost', cost); await button('全额冲销原记录').waitFor(); await button('全额冲销原记录').click(); await field('冲销生效日').fill('2025-02-05'); await reviewAndSubmit('全额冲销原记录'); cost = await current('cost'); assert.equal(cost.state, 'reversed');
  const reverse = await api('/costs/' + cost.reversal_id); assert.equal(reverse.reverses_id, cost.id); assert.equal(reverse.occurred_on, '2025-02-05'); assert.deepEqual(reverse.allocations, cost.allocations);
  checks.push('After releasing the remaining30, the mobile full cost reversal produces a distinct dated reverse record and retains original project/company shares.'); console.log('check: reversal');
  await button('核对准确修订').first().click(); await root().getByText('固定业务日期', { exact: true }).waitFor();
  for (const width of [390, 430, 768]) { await page.setViewportSize({ width, height: 900 }); const layout = await page.evaluate(() => ({ width: innerWidth, document: document.documentElement.scrollWidth })); assert.ok(layout.document <= layout.width + 1, JSON.stringify(layout)); layouts.push(layout); }
  checks.push('Canonical fixed revisions, immutable operation history and three responsive widths remain readable without document overflow.');
  receipts.push({ cost_id: cost.id, cost_version: cost.version, cost_state: cost.state, reversal_id: cost.reversal_id, payment_id: payment.id, refund_id: refund.id, employee_cost_id: employeeCost.id, employee_payment_id: employeePayment.id, claim_id: claim.id, employee_settlement_before_correction: settlement, employee_settlement_after_correction: correctedSettlement, synthetic_data: true });
  await writeFile(path.join(evidence, 'business-proof.json'), JSON.stringify(receipts, null, 2) + '\n');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ recordings, layouts, audio: false, real_browser_pixels: true }, null, 2) + '\n');
}, 'mobile-finance');
