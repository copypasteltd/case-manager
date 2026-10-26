// Actual Web/API contract attribution and reconciliation checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/contract-finance'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_contract_finance.mjs', 'scripts/run_spend_browser.py'])].sort();
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
async function request(context, route, method = 'GET', body) { const result = await response(context, route, method, body); assert(result.status < 400, route + ' returned ' + result.status + '/' + result.value.error?.code); return result.value.data; }
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity));
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.sidebar'), narrow = matchMedia('(max-width: 899px)').matches;
    return sidebar && (narrow ? sidebar.inert && sidebar.getAttribute('aria-hidden') === 'true' : !sidebar.inert && sidebar.getAttribute('aria-hidden') !== 'true' && getComputedStyle(sidebar).visibility === 'visible' && getComputedStyle(sidebar).transform === 'none');
  });
}
async function layout(page, name) {
  await page.getByRole('heading', { level: 1 }).waitFor();

  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + theme + '/' + width + ' overflows');
      const main = page.locator('#main-content');
      const small = await main.locator('button:visible,a:visible,select:visible,input:visible').evaluateAll(nodes => nodes.flatMap(node => { const rect = node.getBoundingClientRect(); return rect.width < 23.5 || rect.height < 23.5 ? [{ label: node.textContent?.trim().slice(0, 80), width: rect.width, height: rect.height }] : []; }));
      assert.equal(small.length, 0, name + '/' + theme + '/' + width + ' has undersized visible controls: ' + JSON.stringify(small));
      if ((theme === 'light' && width === 1440) || (theme === 'dark' && width === 390)) {
        await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'instant' })); await stable(page);
        await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), fullPage: true, animations: 'disabled' });
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
}


async function modalFocus(page) { await page.waitForFunction(() => { const top = [...document.querySelectorAll('dialog[open]')].at(-1); return !top || top.contains(document.activeElement); }); }
async function close(dialog) { await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function commit(dialog, label) { await dialog.getByRole('button', { name: label, exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
const base = (value, reason = '隔离账期准确事实核对') => ({ expected_version: value.version, revision_id: value.current_revision_id, content_hash: value.content_hash, reason });
async function selectProof(page, dialog, action, title) {
  await dialog.getByRole('button', { name: action, exact: true }).click();
  const picker = page.getByRole('dialog', { name: title, exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click();
  await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
}

async function newContract(context, reviewer, supplier, direction, title, owner) {
  let record = await request(context, '/contracts', 'POST', { title, direction, type: 'service', category: 'execution', counterparty_id: supplier.id, counterparty_revision_id: supplier.current_revision_id, owner_id: owner, currency: 'HKD', amount: '100', start_date: fixture.today, end_date: '2030-01-01', draft_file_version_id: fixture.file_version_id, project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离合同节点实际归属验证', payment_schedule: { mode: 'scheduled', reason: '分批交付计划', items: [{ id: crypto.randomUUID(), title: '首批交付', amount: '60', due_date: fixture.today, conditions: '核实首批交付' }, { id: crypto.randomUUID(), title: '最终验收', amount: '40', due_date: fixture.today, conditions: '核实最终验收' }] } });
  let q = await request(context, '/requests', 'POST', { kind: 'contract', business_id: record.id, expected_business_version: record.version, business_revision_id: record.current_revision_id, business_content_hash: record.content_hash, reason: '固定合同商业修订' });
  const preview = await request(context, '/requests/' + q.id + '/submission-preview');
  q = await request(context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '核对准确批准依据' });
  await request(reviewer, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '独立确认合同计划' });
  record = await request(context, '/contracts/' + record.id);
  record = await request(context, '/contracts/' + record.id + '/signature', 'POST', { ...base(record), signed_on: fixture.today, signed_file_version_id: fixture.file_version_id, attestation: '隔离签署件已人工核对准确批准修订', reason: '登记隔离签署事实' });
  return request(context, '/contracts/' + record.id + '/transition', 'POST', { ...base(record), action: 'activate', signature_id: record.signature.id });
}
async function newPayment(context, supplier, direction, amount, title, original = null) {
  let p = await request(context, '/payments', 'POST', { title, direction, paid_at: '2025-02-01T02:00:00+08:00', currency: 'HKD', amount, category: 'contract.browser', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, employee_id: null, refund_of_id: original?.id ?? null, method_label: '虚构转账标签', external_ref: '', evidence_version_ids: [fixture.file_version_id], notes: '', source_key: 'contract-browser:' + crypto.randomUUID(), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离真实资金依据' });
  return request(context, '/payments/' + p.id + '/record', 'POST', { ...base(p), expected_source_version: original?.version ?? null, refund_allocations: [] });
}
async function assign(page, node, kind, title, amount) {
  const labels = { cost: '归属真实费用', cash: '归属实际收付款', refund: '归属实际退回款' };
  await node.getByRole('button', { name: labels[kind], exact: true }).click();
  const dialog = page.getByRole('dialog', { name: labels[kind], exact: true });
  if (kind === 'refund') {
    await dialog.getByRole('button', { name: '选择原正向资金归属', exact: true }).click();
    const roots = page.getByRole('dialog', { name: '选择原正向资金归属', exact: true });
    await roots.locator('button.reference-option').first().click(); await roots.waitFor({ state: 'hidden' }); await modalFocus(page);
  }
  await dialog.getByRole('button', { name: '选择准确合同财务来源', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择准确合同财务来源', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  await dialog.getByLabel('本次原币归属金额', { exact: true }).fill(amount); await dialog.getByLabel('操作依据', { exact: true }).fill('人工核对准确来源，部分归属 ' + amount); await commit(dialog, labels[kind]);
}
async function prepareReview(page, node, condition = false) {
  await node.getByRole('button', { name: '核对节点财务', exact: true }).click(); const dialog = page.getByRole('dialog', { name: '核对节点财务', exact: true });
  if (condition) await dialog.getByLabel('已核实本节点约定条件均已满足。', { exact: true }).check();
  const difference = dialog.getByLabel('已核对净收付款与计划差额，并在依据中说明。', { exact: true }); if (await difference.count()) await difference.check();
  const refunds = dialog.getByLabel('已核实尚未归属的退款对本节点的影响。', { exact: true }); if (await refunds.count()) await refunds.check();
  await selectProof(page, dialog, '选择固定节点核对凭证', '选择准确节点核对凭证');
  await dialog.getByLabel('操作依据', { exact: true }).fill('已核对当前真实费用、净收付款、差额与固定凭证'); return dialog;
}
async function dialogLayout(page, dialog, name) {
  // Theme selection still runs the actual React onChange while the parent is inert.
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme, { force: true });
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page); await modalFocus(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + theme + '/' + width + ' overflows');
      const bad = await dialog.locator('button:visible,input:visible,select:visible').evaluateAll(nodes => nodes.filter(node => { const r = node.getBoundingClientRect(); return r.width < 23.5 || r.height < 23.5; }).map(node => node.textContent?.trim())); assert.equal(bad.length, 0, name + ' has undersized controls ' + JSON.stringify(bad));
      if (theme === 'light' && width === 1440 || theme === 'dark' && width === 390) { await dialog.evaluate(node => node.scrollTop = 0); await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), animations: 'disabled' }); }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light', { force: true });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  for (const key of ['contract_reconciler', 'cost_recorder', 'cost_confirmer', 'payment_recorder', 'payment_reverser', 'cash_reporter']) {
    const role = roles.items.find(value => value.key === key); assert(role, key);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: me.user.id, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离合同财务验证临时授权' }));
  }
  await alice.context.close(); alice = await login(fixture.alice); const { context, page } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构合同协作方', kind: 'organization', relationships: ['supplier', 'customer'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离合同归属往来方' });
  const contract = await newContract(context, bob.context, supplier, 'payable', '虚构服务交付合同', me.user.id);
  let cost = await request(context, '/costs', 'POST', { title: '虚构合同服务实际费用', occurred_on: '2025-01-15', currency: 'HKD', amount: '100', category: 'contract.browser', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, allocations: [{ project_id: fixture.project_id, amount: '100' }], evidence_version_ids: [fixture.file_version_id], notes: '', source_key: 'contract-cost:' + crypto.randomUUID(), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离规范费用' });
  cost = await request(context, '/costs/' + cost.id + '/confirm', 'POST', base(cost));
  cost = await request(context, '/costs/' + cost.id + '/links', 'POST', { ...base(cost), source_type: 'contract', source_id: contract.id, source_revision_id: contract.active_revision_id, relation: 'contractual_basis' });
  const cash = await newPayment(context, supplier, 'outgoing', '100', '虚构合同实际付款');
  await page.goto(fixture.origin + '/contracts/' + contract.id); await page.getByRole('heading', { name: '合同财务归属', exact: true }).waitFor();
  const node = page.getByRole('article', { name: '首批交付财务归属', exact: true });
  await assign(page, node, 'cost', cost.title, '30'); await assign(page, node, 'cash', cash.title, '50');
  let view = await request(context, '/contracts/' + contract.id + '/finance'); assert.equal(view.nodes[0].cost_amount, '30.000000'); assert.equal(view.nodes[0].cash_amount, '50.000000'); assert.equal(view.nodes[0].outstanding_amount, '10.000000');
  await layout(page, 'contract-finance-summary');
  checks.push({ name: 'Web fixes real confirmed cost 30 and cash 50 separately against approved milestone 60 without revising contract terms', passed: true });
  let dialog = await prepareReview(page, node, true); await dialogLayout(page, dialog, 'contract-finance-review'); await commit(dialog, '核对节点财务'); await node.getByText('当前依据已核对', { exact: true }).waitFor();
  view = await request(context, '/contracts/' + contract.id + '/finance'); const beforeRefund = view.version;
  const currentCash = await request(context, '/payments/' + cash.id), refund = await newPayment(context, supplier, 'refund', '10', '虚构供应方退回款', currentCash);
  await page.reload(); await node.getByText(/尚未归属的实际退款/).waitFor(); await node.getByText('来源已变化，须重新核对', { exact: true }).waitFor(); view = await request(context, '/contracts/' + contract.id + '/finance'); assert.equal(view.version, beforeRefund); assert.equal(view.nodes[0].cash_amount, '50.000000');
  await assign(page, node, 'refund', refund.title, '10'); view = await request(context, '/contracts/' + contract.id + '/finance'); assert.equal(view.nodes[0].cash_amount, '40.000000'); assert.equal(view.nodes[0].has_unattributed_refunds, false);
  checks.push({ name: 'Actual unclassified refund stales an old review without changing finance version; explicit Web attribution changes net cash to 40', passed: true });
  await node.getByRole('button', { name: '查看节点归属历史', exact: true }).click(); dialog = page.getByRole('dialog', { name: '节点归属与核对历史', exact: true });
  const refundEvent = dialog.locator('article').filter({ has: page.getByRole('heading', { name: /归属实际退回款 · 记录/ }) }); await refundEvent.getByRole('button', { name: '解除部分归属', exact: true }).click();
  let child = page.getByRole('dialog', { name: '解除部分归属', exact: true }); await child.getByLabel('本次原币归属金额', { exact: true }).fill('5'); await child.getByLabel('操作依据', { exact: true }).fill('保留原记录，部分退款重新核对'); await commit(child, '解除部分归属');
  await dialog.getByText('保留原记录，部分退款重新核对', { exact: true }).waitFor(); await close(dialog);
  dialog = await prepareReview(page, node); await commit(dialog, '核对节点财务'); view = await request(context, '/contracts/' + contract.id + '/finance'); assert.equal(view.nodes[0].cash_amount, '45.000000'); assert.equal(view.nodes[0].conditions_satisfied, false); assert.equal(view.nodes[0].has_unattributed_refunds, true);
  checks.push({ name: 'Web partial release retains ledger history, reveals unclassified refund and records conditions separately from financial review', passed: true });
  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); dialog = await prepareReview(page, node);
  const reason = '保留输入，实际退款冲销后重新核对'; await dialog.getByLabel('操作依据', { exact: true }).fill(reason);
  const refundNow = await request(context, '/payments/' + refund.id); await request(context, '/payments/' + refund.id + '/reverse', 'POST', { ...base(refundNow), effective_at: '2025-02-02T02:00:00+08:00' });
  await dialog.getByRole('button', { name: '核对节点财务', exact: true }).click(); await dialog.getByText(/输入已保留/).waitFor(); assert.equal(await dialog.getByLabel('操作依据', { exact: true }).inputValue(), reason); assert.equal(await dialog.getByLabel('操作依据', { exact: true }).isDisabled(), true);
  await dialog.getByLabel('操作依据', { exact: true }).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'contract-stale-review-dark-390.png'), animations: 'disabled' }); await close(dialog); await page.reload(); await node.getByText('来源已变化，须重新核对', { exact: true }).waitFor();
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
  await node.getByRole('button', { name: '查看节点归属历史', exact: true }).click(); dialog = page.getByRole('dialog', { name: '节点归属与核对历史', exact: true });
  await dialog.getByText('查阅当时固定的财务依据', { exact: true }).first().click(); await dialog.getByText('当时状态 已登记实际资金', { exact: false }).first().waitFor(); await dialogLayout(page, dialog, 'contract-finance-history'); await close(dialog);
  checks.push({ name: 'Old open review is rejected on actual refund reversal, preserves disabled input, and historical fixed source facts remain readable', passed: true });
  const incomeContract = await newContract(context, bob.context, supplier, 'receivable', '虚构项目收款合同', me.user.id), income = await newPayment(context, supplier, 'incoming', '100', '虚构客户实际收款'), returned = await newPayment(context, supplier, 'outgoing', '25', '虚构退回客户收款');
  await page.goto(fixture.origin + '/contracts/' + incomeContract.id); await node.waitFor(); await assign(page, node, 'cash', income.title, '100'); await assign(page, node, 'refund', returned.title, '25');
  await node.getByText('应收节点不登记支出费用', { exact: true }).waitFor(); view = await request(context, '/contracts/' + incomeContract.id + '/finance'); assert.equal(view.nodes[0].cash_amount, '75.000000'); assert.equal(view.nodes[0].cost_amount, null); await layout(page, 'contract-receivable-return');
  const reserved = await request(context, '/payments/' + returned.id); assert.equal(reserved.contract_return_reserved_amount, '25.000000'); assert.equal(reserved.unallocated_amount, '0.000000');
  const exhausted = await response(context, '/payments/' + returned.id + '/allocations', 'POST', { ...base(reserved), cost_id: cost.id, cost_version: cost.version, cost_revision_id: cost.current_revision_id, cost_content_hash: cost.content_hash, amount: '1', claim_id: null, claim_revision_id: null, claim_line_id: null }); assert.equal(exhausted.status, 409);
  const blocked = await response(context, '/payments/' + income.id + '/reverse', 'POST', { ...base(income), effective_at: '2025-02-02T02:00:00+08:00' }); assert.equal(blocked.status, 409);
  await page.goto(fixture.origin + '/payments/' + returned.id); await page.getByText('收款退回已预留', { exact: true }).waitFor(); await layout(page, 'contract-return-payment-balance');
  const report = await request(context, '/reports/cash?from=2025-02-01&to=2025-03-01&category=contract.browser&project_id=' + fixture.project_id); assert.equal(report.totals[0].net_amount, '25.000000');
  await request(context, '/payments/' + returned.id + '/reverse', 'POST', { ...base(reserved), effective_at: '2025-02-02T02:00:00+08:00' }); assert.equal((await request(context, '/contracts/' + incomeContract.id + '/finance')).nodes[0].cash_amount, '100.000000');
  checks.push({ name: 'Receivable Web handles income 100 and return 25; reserved cash blocks duplicate expense settlement and original reversal; scoped cash report and actual reversal remain consistent', passed: true });
  await bob.page.goto(fixture.origin + '/contracts/' + contract.id); await bob.page.getByRole('heading', { name: '合同财务归属', exact: true }).waitFor();
  const readerNode = bob.page.getByRole('article', { name: '首批交付财务归属', exact: true }); await readerNode.getByText('当前不可查阅', { exact: true }).first().waitFor();
  assert.equal(await readerNode.getByRole('button').count(), 0); const limited = await request(bob.context, '/contracts/' + contract.id + '/finance'); assert.equal(limited.nodes[0].cash_amount, null); assert.equal(limited.nodes[0].cost_amount, null); assert.equal(limited.nodes[0].fingerprint, null); assert.equal((await response(bob.context, '/contracts/' + contract.id + '/finance/events')).status, 404); await layout(bob.page, 'contract-finance-read-only');
  checks.push({ name: 'Contract reader sees unknown financial facts with no write/history controls and cannot obtain source fingerprints or event history', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离合同对账验证完成，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
