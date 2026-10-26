// Actual Web/API purchase completion checks against an owned disposable native fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.origin, 'http://localhost:5189');
assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1'); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/purchase-completion'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), ...await files('mobile/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_purchase_completion.mjs', 'scripts/run_spend_browser.py'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map(), bindings = [];
let browser, passed = false, administrator, alice, bob, cleanedBindings = false;
const cleanupErrors = [];
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function code(account) { const previous = lastSteps.get(account.login) ?? account.last_step; if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 250)); const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); return otp(account.mfa_secret, step); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(20000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) { await page.getByLabel('六位验证码').fill(await code(account)); await page.getByRole('button', { name: '验证并继续', exact: true }).click(); }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { context, page };
}
async function response(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const value = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  return { status: value.status(), value: await value.json() };
}
async function request(context, route, method = 'GET', body) { const result = await response(context, route, method, body); assert(result.status < 400, route + ' returned ' + result.status + '/' + result.value.error?.code + '/' + JSON.stringify(result.value.error?.details)); return result.value.data; }
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity));
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.sidebar'), narrow = matchMedia('(max-width: 899px)').matches;
    return sidebar && (narrow ? sidebar.inert && sidebar.getAttribute('aria-hidden') === 'true' : !sidebar.inert && sidebar.getAttribute('aria-hidden') !== 'true' && getComputedStyle(sidebar).visibility === 'visible' && getComputedStyle(sidebar).transform === 'none');
  });
}
async function selectFile(page, dialog) {
  await dialog.getByRole('button', { name: '添加登记依据版本', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择采购履行依据版本', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click();
  await picker.waitFor({ state: 'hidden' });
}
async function fillOrder(page, externalRef, quantity, supplier) {
  await page.getByRole('button', { name: '登记已订购订单', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '登记已订购订单', exact: true });
  await dialog.getByLabel('批准供应商资料', { exact: true }).selectOption(supplier.id + '/' + supplier.current_revision_id);
  await dialog.getByLabel('实际订单编号', { exact: true }).fill(externalRef);
  await dialog.getByLabel('实际订购日期', { exact: true }).fill(fixture.today);
  await dialog.getByLabel('订单预计交付日期', { exact: true }).fill(fixture.today);
  await dialog.getByLabel('本单包含此明细', { exact: true }).check();
  await dialog.getByLabel('订购数量 · 虚构项目工具', { exact: true }).fill(quantity);
  await selectFile(page, dialog);
  await dialog.getByLabel('本次登记依据', { exact: true }).fill('隔离样本的真实订单登记检查');
  return dialog;
}
async function modalFocus(page) {
  await page.waitForFunction(() => { const top = [...document.querySelectorAll('dialog[open]')].at(-1); return !top || top.contains(document.activeElement); });
}
async function commit(dialog, label) { await dialog.getByRole('button', { name: label, exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function close(dialog) { await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function createApproved(context, reviewer, user, supplier) {
  let purchase = await request(context, '/purchases', 'POST', { title: '虚构采购完成核对样本', category: 'purchase.completion.browser', purpose: '仅用于隔离界面核验', owner_id: user,
    currency: 'HKD', pricing_basis: 'net_plus_tax', rounding_mode: 'half_up', expected_delivery_date: fixture.today,
    supplier_candidates: [{ party_id: supplier.id, revision_id: supplier.current_revision_id }], quote_version_ids: [fixture.file_version_id],
    lines: [{ id: crypto.randomUUID(), description: '虚构项目工具', quantity: '10', unit: '件', unit_price: '2', tax_amount: '0', discount_amount: '0' }],
    project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离采购履行批准样本' });
  let q = await request(context, '/requests', 'POST', { kind: 'purchase', business_id: purchase.id, expected_business_version: purchase.version, business_revision_id: purchase.current_revision_id, business_content_hash: purchase.content_hash, reason: '固定批准商业依据' });
  const preview = await request(context, '/requests/' + q.id + '/submission-preview');
  q = await request(context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '提交隔离样本审批' });
  await request(reviewer, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '本人核对隔离商业范围' });
  purchase = await request(context, '/purchases/' + purchase.id);
  assert.equal(purchase.approval_state, 'approved'); return purchase;
}

async function layout(page, name, dialog = null) {
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme, { force: !!dialog });
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + width + ' overflows');
      const scope = dialog ?? page.locator('#main-content');
      const undersized = await scope.locator('button:visible,a:visible,select:visible,input:visible').evaluateAll(nodes => nodes.flatMap(node => { const rect = node.getBoundingClientRect(); return rect.width < 23.5 || rect.height < 23.5 ? [{ tag: node.tagName, text: node.textContent?.trim().slice(0, 100), width: rect.width, height: rect.height }] : []; }));
      assert.deepEqual(undersized, [], name + ' has undersized controls');
      if (dialog) {
        const bounds = await dialog.boundingBox(); assert(bounds && bounds.x >= -1 && bounds.x + bounds.width <= width + 1, name + ' dialog exceeds viewport');
        await modalFocus(page);
      }
      if ((theme === 'light' && width === 1440) || (theme === 'dark' && width === 390)) {
        await page.evaluate(() => { window.scrollTo({ top: 0, behavior: 'instant' }); for (const node of document.querySelectorAll('dialog[open]')) node.scrollTop = 0; });
        await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), fullPage: !dialog, animations: 'disabled' });
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light', { force: !!dialog });
  layouts.push({ name, themes: ['light', 'dark'], widths: [320, 390, 768, 1024, 1440], passed: true });
}
const layouts = [];
const base = value => ({ expected_version: value.version, revision_id: value.current_revision_id, content_hash: value.content_hash, reason: '隔离样本固定实际依据' });
async function delivered(context, reviewer, user, supplier) {
  let purchase = await createApproved(context, reviewer, user, supplier);
  purchase = await request(context, '/purchases/' + purchase.id + '/orders', 'POST', { ...base(purchase), execution_plan_revision_id: null, supplier: purchase.supplier_candidates[0], external_ref: 'SYNTH-' + crypto.randomUUID(), ordered_on: fixture.today, expected_delivery_on: fixture.today, evidence_version_ids: [fixture.file_version_id], notes: '', lines: [{ purchase_line_id: purchase.lines[0].id, quantity: '10', unit_price: '2', tax_amount: '0', discount_amount: '0' }] });
  const orders = await request(context, '/purchases/' + purchase.id + '/orders?limit=100'), order = orders.items[0];
  purchase = await request(context, '/purchases/' + purchase.id + '/receipts', 'POST', { ...base(purchase), order_id: order.id, received_on: fixture.today, evidence_version_ids: [fixture.file_version_id], notes: '', lines: [{ order_line_id: order.lines[0].id, quantity: '10', result: 'accepted', reverses_line_id: null, reason: '隔离实际交付核对' }] });
  return { purchase, order };
}
async function newCost(context, purchase, supplier, amount = '20') {
  let cost = await request(context, '/costs', 'POST', { title: '虚构采购完成费用', occurred_on: fixture.today, currency: 'HKD', amount, category: 'completion', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, allocations: [{ project_id: fixture.project_id, amount }], evidence_version_ids: [fixture.file_version_id], notes: '', project_id: fixture.project_id, classification: 'project_restricted', source_key: 'completion:' + crypto.randomUUID(), reason: '隔离实际费用归集' });
  cost = await request(context, '/costs/' + cost.id + '/confirm', 'POST', base(cost));
  return request(context, '/costs/' + cost.id + '/links', 'POST', { ...base(cost), source_type: 'purchase', source_id: purchase.id, source_revision_id: purchase.current_revision_id, relation: 'incurred_for' });
}
async function payCost(context, cost, supplier) {
  let cash = await request(context, '/payments', 'POST', { title: '虚构采购完成付款', direction: 'outgoing', paid_at: fixture.today + 'T00:00:00+08:00', currency: 'HKD', amount: '20', category: 'completion', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, employee_id: null, refund_of_id: null, method_label: '虚构转账凭证', external_ref: '', evidence_version_ids: [fixture.file_version_id], notes: '', project_id: fixture.project_id, classification: 'project_restricted', source_key: 'completion:' + crypto.randomUUID(), reason: '隔离真实付款依据' });
  cash = await request(context, '/payments/' + cash.id + '/record', 'POST', { ...base(cash), expected_source_version: null, refund_allocations: [] });
  return request(context, '/payments/' + cash.id + '/allocations', 'POST', { ...base(cash), cost_id: cost.id, cost_version: cost.version, cost_revision_id: cost.current_revision_id, cost_content_hash: cost.content_hash, amount: '20', claim_id: null, claim_revision_id: null, claim_line_id: null });
}
async function increasePriceCap(context, reviewer, purchase) {
  const plan = await request(context, '/purchases/' + purchase.id + '/execution-plan'), content = structuredClone(plan.content);
  content.lines[0].unit_price = '3'; content.title = '虚构单价额度调整';
  let proposal = await request(context, '/purchases/' + purchase.id + '/variances', 'POST', { ...content, expected_purchase_version: purchase.version, execution_plan_revision_id: plan.variance_revision_id, reason: '提高当前单价额度，旧订单价格保留' });
  let q = await request(context, '/requests', 'POST', { kind: 'purchase_variance', business_id: proposal.id, expected_business_version: proposal.version, business_revision_id: proposal.current_revision_id, business_content_hash: proposal.content_hash, reason: '准确审批新金额额度' });
  const preview = await request(context, '/requests/' + q.id + '/submission-preview');
  q = await request(context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '独立单价范围重审' });
  await request(reviewer, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '批准新额度并保留历史价格' });
  proposal = await request(context, '/purchase-variances/' + proposal.id);
  await request(context, '/purchase-variances/' + proposal.id + '/activate', 'POST', { ...base(proposal), expected_purchase_version: purchase.version });
  return request(context, '/purchases/' + purchase.id);
}
async function completionForm(page, mode) {
  await page.getByRole('button', { name: '核对并登记采购完成', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '核对并登记采购完成', exact: true });
  assert.equal(await dialog.getByLabel('结算核实方式', { exact: true }).inputValue(), '');
  assert.equal(await dialog.getByRole('button', { name: '确认登记采购完成', exact: true }).isDisabled(), true);
  const declarations = ['已核实实际交付、净验收和全部有效订单余量', '已核实完整实际费用归集、付款和退款义务', '已核实全部差异提案、未使用额度及其处置'];
  for (const name of declarations) assert.equal(await dialog.getByLabel(name, { exact: true }).isChecked(), false);
  await dialog.getByLabel('实际采购完成日期', { exact: true }).fill(fixture.today);
  await dialog.getByLabel('结算核实方式', { exact: true }).selectOption(mode);
  await dialog.getByLabel('完整费用归集与结算核实说明', { exact: true }).fill('已人工核对全部实际交付、完整费用归集及付款退款凭证，确认本次所选核实方式。');
  await dialog.getByLabel(/差异处置说明/).fill('明确核实未使用额度、未启用提案和费用差异；如无应付，已取得供应方免除义务的准确凭证。');
  await dialog.getByLabel('本次采购完成登记依据', { exact: true }).fill('隔离样本人工完整核实后的完成登记');
  await dialog.getByRole('button', { name: '添加完成凭证版本', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择采购完成凭证版本', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click();
  await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  for (const name of declarations) await dialog.getByLabel(name, { exact: true }).check();
  return dialog;
}

try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  const assignments = ['purchase_manager', 'purchase_operator', 'purchase_variance_manager', 'purchase_plan_activator', 'purchase_completer', 'cost_recorder', 'cost_confirmer', 'cost_reverser', 'payment_recorder', 'payment_reverser'].map(key => [me.user.id, key]);
  assignments.push([reviewer.user.id, 'purchase_reader'], [reviewer.user.id, 'purchase_variance_reader']);
  for (const [user, roleKey] of assignments) {
    const role = roles.items.find(item => item.key === roleKey); assert(role, roleKey);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'project', scope_id: fixture.project_id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离采购完成验证的临时授权' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const { page, context } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构采购完成供应商', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离核实的供应方' });
  for (const kind of ['purchase', 'purchase_variance']) await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'purchase.completion.browser.' + kind, name: '虚构采购完成准确批准制度', kind, category: 'purchase.completion.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立准确商业范围核对', assigned_user_id: reviewer.user.id, requires_mfa: false }], reason: '隔离采购完成商业批准依据' });

  const settled = await delivered(context, bob.context, me.user.id, supplier);
  const cost = await newCost(context, settled.purchase, supplier);
  await page.goto(fixture.origin + '/purchases/' + settled.purchase.id);
  const panel = page.getByRole('region', { name: '采购完成与结算核对', exact: true });
  await panel.getByText('已确认费用仍有未结金额，请先核对实际付款与分配。', { exact: true }).waitFor();
  assert.equal(await panel.getByRole('button', { name: '核对并登记采购完成', exact: true }).count(), 0);
  await layout(page, 'purchase-completion-unsettled');
  checks.push({ name: 'Actual linked confirmed expense blocks completion until its real payment allocation is recorded', passed: true });

  await payCost(context, cost, supplier);
  settled.purchase = await increasePriceCap(context, bob.context, settled.purchase);
  const unused = await request(context, '/purchases/' + settled.purchase.id + '/completion-preview');
  assert.equal(unused.can_complete, true); assert.equal(unused.has_difference, true);
  assert.equal(unused.basis.execution_amount, '30.000000'); assert.equal(unused.basis.lines[0].reserved_amount, '20.000000');
  assert.equal(unused.basis.lines[0].approved_quantity, unused.basis.lines[0].net_accepted_quantity);
  checks.push({ name: 'New price cap leaves an unused monetary amount despite full quantity delivery and settled old-price costs; explicit disposition is required', passed: true });
  await page.reload();
  let dialog = await completionForm(page, 'recorded_costs');
  assert.equal(await dialog.getByRole('button', { name: '确认登记采购完成', exact: true }).isEnabled(), true);
  await dialog.getByLabel('差异处置说明（必填）', { exact: true }).fill('');
  assert.equal(await dialog.getByRole('button', { name: '确认登记采购完成', exact: true }).isDisabled(), true);
  await dialog.getByLabel('差异处置说明（必填）', { exact: true }).fill('新执行总额HKD30，原订单及实际费用仍为HKD20，剩余HKD10额度明确不再使用。');
  await layout(page, 'purchase-completion-attestation', dialog);
  await commit(dialog, '确认登记采购完成');
  await page.getByRole('region', { name: '不可变采购完成记录', exact: true }).waitFor();
  const fixed = await request(context, '/purchases/' + settled.purchase.id + '/completion');
  assert.equal(fixed.basis_state, 'current'); assert.equal(fixed.basis.costs.length, 1); assert.equal(fixed.basis.payments.length, 1); assert.equal(fixed.attachments.length, 1);
  assert.equal(await page.getByRole('button', { name: '登记已订购订单', exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: '取消采购后续履行', exact: true }).count(), 0);
  await layout(page, 'purchase-completed-fixed');
  checks.push({ name: 'Web requires three initially unchecked manual declarations explicit settlement mode accurate file and recent MFA before saving immutable complete facts', passed: true });

  await page.getByRole('button', { name: '查看订单与验收', exact: true }).click();
  const orderDialog = page.getByRole('dialog', { name: '订单与验收依据', exact: true });
  await orderDialog.getByRole('button', { name: '登记此验收明细退货', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '登记准确来源退货', exact: true });
  await dialog.getByLabel('实际退货日期', { exact: true }).fill(fixture.today);
  await dialog.getByLabel('退货数量', { exact: true }).fill('1');
  await dialog.getByLabel('退货结果依据', { exact: true }).fill('隔离登记完成后的实际来源退货');
  await selectFile(page, dialog); await dialog.getByLabel('本次登记依据', { exact: true }).fill('准确固定原验收来源后追加退货');
  await commit(dialog, '确认保存来源退货'); await close(orderDialog);
  await page.getByRole('heading', { name: '完成后的当前依据', exact: true }).waitFor();
  const changed = await request(context, '/purchases/' + settled.purchase.id + '/completion');
  const now = await request(context, '/purchases/' + settled.purchase.id + '/completion-preview');
  assert.equal(changed.basis_state, 'changed'); assert.equal(changed.fingerprint, fixed.fingerprint); assert.equal(changed.basis.lines[0].net_accepted_quantity, '10.000000'); assert.equal(now.basis.lines[0].net_accepted_quantity, '9.000000');
  await layout(page, 'purchase-completed-basis-changed');
  checks.push({ name: 'Completed Web purchase permits only an exact source return while retaining original completion delivery and clearly displaying changed current obligations', passed: true });

  const noPayable = await delivered(context, bob.context, me.user.id, supplier);
  await page.goto(fixture.origin + '/purchases/' + noPayable.purchase.id);
  dialog = await completionForm(page, 'no_payable_obligation'); await commit(dialog, '确认登记采购完成');
  await page.getByRole('region', { name: '不可变采购完成记录', exact: true }).waitFor();
  const declaration = await request(context, '/purchases/' + noPayable.purchase.id + '/completion');
  assert.equal(declaration.settlement_mode, 'no_payable_obligation'); assert.equal(declaration.basis.costs.length, 0); assert.equal(declaration.basis.payments.length, 0);
  checks.push({ name: 'Human explicitly records no payable obligation with a disposition and fixed evidence while creating no invented expense or payment', passed: true });

  const stale = await delivered(context, bob.context, me.user.id, supplier);
  await page.goto(fixture.origin + '/purchases/' + stale.purchase.id); dialog = await completionForm(page, 'no_payable_obligation');
  const originalVersion = stale.purchase.version;
  await newCost(context, stale.purchase, supplier);
  assert.equal((await request(context, '/purchases/' + stale.purchase.id)).version, originalVersion);
  await dialog.getByRole('button', { name: '确认登记采购完成', exact: true }).click();
  await dialog.getByText('采购、财务依据或授权已变化，未按旧依据登记。填写内容仍保留，请关闭后核对最新完整集合。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('本次采购完成登记依据', { exact: true }).inputValue(), '隔离样本人工完整核实后的完成登记');
  assert.equal(await dialog.getByLabel('结算核实方式', { exact: true }).inputValue(), 'no_payable_obligation');
  assert.equal(await dialog.getByLabel('已核实完整实际费用归集、付款和退款义务', { exact: true }).isChecked(), true);
  assert.equal(await dialog.getByRole('button', { name: '确认登记采购完成', exact: true }).isDisabled(), true);
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label')), '采购完成登记结果');
  await layout(page, 'purchase-completion-conflict-preserved', dialog); await close(dialog);
  checks.push({ name: 'Financial change with unchanged purchase version rejects stale completion and preserves mode declarations reasons and fixed files with error focus and disabled resubmission', passed: true });

  await bob.page.goto(fixture.origin + '/purchases/' + settled.purchase.id);
  const restricted = bob.page.getByRole('region', { name: '采购完成与结算核对', exact: true });
  await restricted.getByText('当前无法完整查阅相关交付、差异或财务资料。完成条件未知，请联系业务负责人核对查阅范围。', { exact: true }).waitFor();
  assert.equal(await restricted.getByRole('button').count(), 0); assert.equal(await restricted.locator('.completion-facts').count(), 0);
  const unknown = await request(bob.context, '/purchases/' + settled.purchase.id + '/completion-preview');
  assert.equal(unknown.visibility, 'restricted'); assert.equal(unknown.basis, null); assert.equal(unknown.fingerprint, null);
  assert.equal((await response(bob.context, '/purchases/' + settled.purchase.id + '/completion')).status, 404);
  await layout(bob.page, 'purchase-completion-restricted');
  checks.push({ name: 'Limited purchase reader sees unknown completeness with no partial financial facts or historical completion data', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离采购完成验证结束，撤销临时授权' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_orders: false, checks, layouts, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
