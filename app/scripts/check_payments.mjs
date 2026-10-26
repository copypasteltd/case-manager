// Actual Web/API cash and settlement checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/payments-settlement'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_payments.mjs', 'scripts/run_spend_browser.py'])].sort();
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
async function createApproved(kind, owner, supplier) {
  await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'cash.browser.' + kind, name: '虚构费用依据核对制度', kind, category: 'cash.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立费用依据核对', assigned_user_id: owner.reviewer, requires_mfa: false }], reason: '隔离费用依据制度' });
  const shared = { title: '虚构费用依据 · ' + kind, category: 'cash.browser', owner_id: owner.id, currency: 'HKD', project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离费用来源', notes: '' };
  const body = kind === 'contract' ? { ...shared, direction: 'payable', type: 'service', amount: '200', counterparty_id: supplier.id, counterparty_revision_id: supplier.current_revision_id, start_date: '2025-01-15', end_date: '2027-12-31', draft_file_version_id: fixture.file_version_id, payment_schedule: null } : kind === 'purchase' ? { ...shared, purpose: '核对一笔真实发生费用', pricing_basis: 'net_plus_tax', rounding_mode: 'half_up', expected_delivery_date: '2025-01-15', supplier_candidates: [{ party_id: supplier.id, revision_id: supplier.current_revision_id }], quote_version_ids: [fixture.file_version_id], lines: [{ id: crypto.randomUUID(), description: '虚构服务', quantity: '1', unit: '项', unit_price: '200', tax_amount: '0', discount_amount: '0' }] } : { ...shared, purpose: '同一费用报销依据', pricing_basis: 'gross_includes_tax', rounding_mode: 'half_up', period_start: '2025-01-15', period_end: '2025-01-15', lines: [{ id: crypto.randomUUID(), description: '虚构同一服务', declared_amount: '200', expense_date: '2025-01-15', category: 'cash.browser', supplier: { party_id: supplier.id, revision_id: supplier.current_revision_id }, receipt_version_ids: [fixture.file_version_id], funding: 'employee_advance' }] };
  const route = kind === 'contract' ? '/contracts' : kind === 'purchase' ? '/purchases' : '/claims';
  let record = await request(alice.context, route, 'POST', body), q = await request(alice.context, '/requests', 'POST', { kind, business_id: record.id, expected_business_version: record.version, business_revision_id: record.current_revision_id, business_content_hash: record.content_hash, reason: '准确费用来源' });
  const preview = await request(alice.context, '/requests/' + q.id + '/submission-preview');
  q = await request(alice.context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '提交准确费用依据' });
  await request(bob.context, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '独立核对商业依据' });
  record = await request(alice.context, route + '/' + record.id);
  if (kind === 'contract') {
    record = await request(alice.context, route + '/' + record.id + '/signature', 'POST', { expected_version: record.version, revision_id: record.current_revision_id, content_hash: record.content_hash, signed_on: '2025-01-15', signed_file_version_id: fixture.file_version_id, attestation: '核对虚构签署正文与准确批准修订', reason: '记录隔离签署事实' });
    record = await request(alice.context, route + '/' + record.id + '/transition', 'POST', { expected_version: record.version, revision_id: record.current_revision_id, content_hash: record.content_hash, signature_id: record.signature.id, action: 'activate', reason: '隔离已签署合同生效' });
  }
  assert.equal(record.approval_state, 'approved'); return record;
}

const base = (p, reason = '人工核对准确资金事实') => ({ expected_version: p.version, revision_id: p.current_revision_id, content_hash: p.content_hash, reason });
async function pickEvidence(page, dialog) {
  await dialog.getByRole('button', { name: '选择资金凭证', exact: true }).click(); const picker = page.getByRole('dialog', { name: '选择准确资金凭证', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click(); await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  const grants = [...['payment_recorder', 'payment_reverser', 'cash_reporter', 'cost_recorder', 'cost_confirmer', 'cost_reverser', 'finance_reporter', 'claim_submitter'].map(key => [me.user.id, key]), ...['payment_reader', 'cost_reader', 'claim_reader'].map(key => [reviewer.user.id, key])];
  for (const [user, key] of grants) { const role = roles.items.find(r => r.key === key); assert(role); bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离现金结算临时验证权限' })); }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const { page, context } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构实际付款供应方', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离付款供应方' });
  const newCost = async (title, allocations) => { let c = await request(context, '/costs', 'POST', { title, occurred_on: '2025-01-15', currency: 'HKD', amount: '100', category: 'cash.browser', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, allocations: allocations ?? [{ project_id: fixture.project_id, amount: '60' }, { project_id: null, amount: '40' }], evidence_version_ids: [fixture.file_version_id], notes: '', source_key: 'cash-cost:' + crypto.randomUUID(), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离实际费用依据' }); return request(context, '/costs/' + c.id + '/confirm', 'POST', base(c)); };
  const cost = await newCost('虚构付款结算费用');
  await page.goto(fixture.origin + '/payments'); await page.getByRole('button', { name: '起草实际资金', exact: true }).click(); let dialog = page.getByRole('dialog', { name: '起草实际资金', exact: true });
  await dialog.getByLabel('资金记录标题', { exact: true }).fill('虚构项目实际付款'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id); await dialog.getByLabel('登记或修订原因', { exact: true }).fill('先登记待核实真实资金'); await commit(dialog, '保存资金草稿');
  await page.getByRole('heading', { name: '虚构项目实际付款', exact: true }).waitFor(); const id = new URL(page.url()).pathname.split('/').at(-1); let payment = await request(context, '/payments/' + id); assert.equal(payment.state, 'draft'); assert.equal(payment.amount, null); assert.equal((await response(administrator.context, '/payments/' + id)).status, 404);
  checks.push({ name: 'Actual Web saves incomplete payment draft without inventing cash, administration lacks payment read', passed: true });
  const second = await login(fixture.alice); await second.page.goto(fixture.origin + '/payments/' + id); await second.page.getByRole('button', { name: '修订资金草稿', exact: true }).click(); const stale = second.page.getByRole('dialog', { name: '修订资金草稿', exact: true }); await stale.getByLabel('资金记录标题', { exact: true }).fill('保留的过期付款输入'); await stale.getByLabel('登记或修订原因', { exact: true }).fill('并发草稿不覆盖');
  await page.getByRole('button', { name: '修订资金草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '修订资金草稿', exact: true });
  await dialog.getByLabel('资金方向', { exact: true }).selectOption('outgoing'); await dialog.getByLabel('实际发生时间（香港）', { exact: true }).fill('2025-02-01T02:00'); await dialog.getByLabel('资金类别', { exact: true }).fill('cash.browser'); await dialog.getByLabel('原币', { exact: true }).selectOption('HKD'); await dialog.getByLabel('实际原币金额', { exact: true }).fill('100'); await dialog.getByLabel('付款方式标签', { exact: true }).fill('虚构银行转账');
  await dialog.getByRole('button', { name: '选择资金往来方', exact: true }).click(); let picker = page.getByRole('dialog', { name: '选择往来方资料', exact: true }); await picker.locator('button.reference-option').filter({ hasText: supplier.name }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page); await pickEvidence(page, dialog);
  await dialog.getByLabel('登记或修订原因', { exact: true }).fill('核对香港实际时间与固定凭证'); await commit(dialog, '保存资金草稿');
  await stale.getByRole('button', { name: '保存资金草稿', exact: true }).click(); await stale.getByText('输入已保留。请关闭并核对最新资料和来源标识。', { exact: true }).waitFor(); assert.equal(await stale.getByLabel('资金记录标题', { exact: true }).inputValue(), '保留的过期付款输入'); await close(stale); await second.context.close();
  await page.getByRole('button', { name: '登记实际资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记实际资金', exact: true }); await dialog.getByLabel('操作依据', { exact: true }).fill('本人核对真实资金与凭证'); await commit(dialog, '登记实际资金'); payment = await request(context, '/payments/' + id); assert.equal(payment.state, 'recorded'); assert.equal(payment.paid_on, '2025-02-01');
  await page.getByText(/02:00:00（香港时间）/).waitFor();
  checks.push({ name: 'Fixed party and file pickers, Hong Kong timestamp with seconds, human recording and stale concurrent input preservation work through the Web', passed: true });
  await page.getByRole('button', { name: '分配费用结算', exact: true }).click(); dialog = page.getByRole('dialog', { name: '分配费用结算', exact: true }); await dialog.getByRole('button', { name: '选择结算费用', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择准确结算费用', exact: true }); await picker.locator('button.reference-option').filter({ hasText: cost.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page); await dialog.getByLabel('本次原币分配金额', { exact: true }).fill('60'); await dialog.getByLabel('操作依据', { exact: true }).fill('首次部分结算真实费用'); await commit(dialog, '分配费用结算');
  payment = await request(context, '/payments/' + id); assert.equal(payment.effective_allocated_amount, '60.000000'); assert.equal(payment.unallocated_amount, '40.000000');
  await page.getByRole('button', { name: '释放此分配', exact: true }).click(); dialog = page.getByRole('dialog', { name: '释放费用分配', exact: true }); await dialog.getByLabel('本次原币分配金额', { exact: true }).fill('20'); await dialog.getByLabel('操作依据', { exact: true }).fill('按实际归属释放部分费用分配'); await commit(dialog, '释放费用分配');
  payment = await request(context, '/payments/' + id); assert.equal(payment.effective_allocated_amount, '40.000000'); assert.equal(payment.unallocated_amount, '60.000000'); assert.equal((await request(context, '/costs/' + cost.id)).paid_amount, '40.000000');
  assert.equal((await response(context, '/costs/' + cost.id + '/reverse', 'POST', { ...base(cost), effective_on: '2025-02-02' })).status, 409);
  await layout(page, 'payment-detail');
  await bob.page.goto(fixture.origin + '/payments/' + id); await bob.page.getByRole('heading', { name: payment.title, exact: true }).waitFor(); assert.equal(await bob.page.getByRole('button', { name: '分配费用结算', exact: true }).count(), 0); assert.equal(await bob.page.getByRole('button', { name: '全额冲销资金', exact: true }).count(), 0);
  checks.push({ name: 'Partial allocation and release conserve cash and cost balances, fence cost reversal, and remain read-only for a separate user', passed: true });
  await page.getByRole('button', { name: '查看资金修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: '资金修订记录', exact: true }); await dialog.locator('button.reference-option').filter({ hasText: '资金修订 1 ·' }).click(); await dialog.getByRole('heading', { name: '准确历史资金修订 1', exact: true }).waitFor(); assert(await dialog.locator('.cost-history-detail').innerText().then(text => text.includes('尚未确定'))); await close(dialog);
  await page.getByRole('button', { name: '登记收到退款', exact: true }).click(); dialog = page.getByRole('dialog', { name: '起草收到退款', exact: true }); await dialog.getByLabel('实际发生时间（香港）', { exact: true }).fill('2025-02-02T08:00'); await dialog.getByLabel('实际原币金额', { exact: true }).fill('20'); await dialog.getByLabel('付款方式标签', { exact: true }).fill('虚构原渠道退款'); await pickEvidence(page, dialog); await dialog.getByLabel('登记或修订原因', { exact: true }).fill('核对实际收到部分退款'); await commit(dialog, '保存资金草稿');
  await page.getByRole('heading', { name: '虚构项目实际付款 · 退款', exact: true }).waitFor(); const refundID = new URL(page.url()).pathname.split('/').at(-1);
  await page.getByRole('button', { name: '登记实际资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记实际资金', exact: true }); await dialog.getByLabel('本次退款分配 1', { exact: true }).fill('10'); await dialog.getByLabel('操作依据', { exact: true }).fill('退款同时减少对应费用分配'); await commit(dialog, '登记实际资金');
  await page.getByRole('button', { name: '关联退款分配', exact: true }).click(); dialog = page.getByRole('dialog', { name: '关联退款分配', exact: true }); await dialog.getByRole('radio').check(); await dialog.getByLabel('本次原币分配金额', { exact: true }).fill('10'); await dialog.getByLabel('操作依据', { exact: true }).fill('补充退款原始费用分配'); await commit(dialog, '关联退款分配');
  let refund = await request(context, '/payments/' + refundID); assert.equal(refund.effective_allocated_amount, '20.000000'); payment = await request(context, '/payments/' + id); assert.equal(payment.refunded_amount, '20.000000'); assert.equal(payment.effective_allocated_amount, '20.000000'); assert.equal(payment.unallocated_amount, '60.000000');
  checks.push({ name: 'Refund draft pins its original payment; recording atomically releases a selected source and later refund allocation appends a second exact inverse', passed: true });
  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await stable(page); await page.getByRole('button', { name: '全额冲销资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '全额冲销资金', exact: true }); await dialog.getByLabel('冲销生效时间（香港）', { exact: true }).fill('2025-03-01T08:00'); await dialog.getByLabel('操作依据', { exact: true }).fill('真实更正退款登记，保留原始历史'); await dialog.getByLabel('操作依据', { exact: true }).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'payment-reversal-dark-390.png'), animations: 'disabled' }); await commit(dialog, '全额冲销资金');
  await page.goto(fixture.origin + '/payments/' + id); await page.getByRole('button', { name: '全额冲销资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '全额冲销资金', exact: true }); await dialog.getByLabel('冲销生效时间（香港）', { exact: true }).fill('2025-03-02T08:00'); await dialog.getByLabel('操作依据', { exact: true }).fill('追加原付款及所有分配的反向记录'); await commit(dialog, '全额冲销资金'); assert.equal((await request(context, '/costs/' + cost.id)).paid_amount, '0.000000');
  await page.getByRole('button', { name: '查看资金登记记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: '资金登记记录', exact: true }); await dialog.getByText('原资金登记冲销', { exact: true }).waitFor(); await close(dialog);
  checks.push({ name: 'Narrow-screen refund reversal and original cash reversal preserve facts, compensate every allocation leg and restore zero effective cost settlement', passed: true });
  await page.goto(fixture.origin + '/reports/cash'); await page.getByLabel('现金统计月份', { exact: true }).fill('2025-02'); await page.getByLabel('资金类别筛选', { exact: true }).fill('cash.browser'); await page.getByRole('button', { name: '查询现金支出', exact: true }).click(); await page.locator('.cost-net').filter({ hasText: 'HKD 80' }).waitFor(); await layout(page, 'cash-report');
  await page.getByLabel('现金分配范围', { exact: true }).selectOption('company'); await page.getByRole('button', { name: '查询现金支出', exact: true }).click(); await page.locator('.cost-net').filter({ hasText: 'HKD 8' }).waitFor(); await page.getByRole('link', { name: '核对现金来源', exact: true }).click(); await page.getByRole('heading', { name: '付款与结算', exact: true }).waitFor(); await page.locator('.contract-row').first().waitFor(); assert.equal(await page.locator('.contract-row').count(), 2); await layout(page, 'cash-drilldown');
  const feb = await request(context, '/reports/cash?from=2025-02-01&to=2025-03-01&category=cash.browser'), mar = await request(context, '/reports/cash?from=2025-03-01&to=2025-04-01&category=cash.browser'); assert.equal(feb.totals[0].net_amount, '80.000000'); assert.equal(mar.totals[0].net_amount, '-80.000000'); assert.equal(feb.totals[0].unallocated_amount, '60.000000');
  checks.push({ name: 'Actual monthly cash report and company drilldown preserve February net 80, unallocated 60 and March corrections minus 80', passed: true });

  const claim = await createApproved('claim', { id: me.user.id, reviewer: reviewer.user.id }, supplier); let employeeCost = await newCost('虚构员工垫付准确费用', [{ project_id: fixture.project_id, amount: '100' }]); employeeCost = await request(context, '/costs/' + employeeCost.id + '/links', 'POST', { ...base(employeeCost), source_type: 'claim', source_id: claim.id, source_revision_id: claim.approved_revision_id, relation: 'reimbursed_via' });
  await page.goto(fixture.origin + '/payments'); await page.getByRole('button', { name: '起草实际资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '起草实际资金', exact: true }); await dialog.getByLabel('资金记录标题', { exact: true }).fill('虚构员工实际报销付款'); await dialog.getByLabel('资金方向', { exact: true }).selectOption('outgoing'); await dialog.getByRole('button', { name: '按批准报销选择收款员工', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择已批准员工垫付报销', exact: true }); await picker.locator('button.reference-option').filter({ hasText: claim.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  await dialog.getByLabel('实际发生时间（香港）', { exact: true }).fill('2025-04-01T10:00'); await dialog.getByLabel('资金类别', { exact: true }).fill('cash.employee'); await dialog.getByLabel('实际原币金额', { exact: true }).fill('100'); await dialog.getByLabel('付款方式标签', { exact: true }).fill('虚构员工报销转账'); await pickEvidence(page, dialog); await dialog.getByLabel('登记或修订原因', { exact: true }).fill('员工实际收到公司款项'); await commit(dialog, '保存资金草稿');
  await page.getByRole('button', { name: '登记实际资金', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记实际资金', exact: true }); await dialog.getByLabel('操作依据', { exact: true }).fill('本人核对员工真实到账'); await commit(dialog, '登记实际资金');
  await page.getByRole('button', { name: '分配费用结算', exact: true }).click(); dialog = page.getByRole('dialog', { name: '分配费用结算', exact: true }); await dialog.getByRole('button', { name: '选择员工垫付依据', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择已批准员工垫付报销', exact: true }); await picker.locator('button.reference-option').filter({ hasText: claim.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page); await dialog.getByLabel('批准垫付明细', { exact: true }).selectOption(claim.lines[0].id); await dialog.getByRole('button', { name: '选择结算费用', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择准确结算费用', exact: true }); await picker.locator('button.reference-option').filter({ hasText: employeeCost.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page); await dialog.getByLabel('本次原币分配金额', { exact: true }).fill('100'); await dialog.getByLabel('操作依据', { exact: true }).fill('结算准确批准员工垫付明细'); await commit(dialog, '分配费用结算');
  await page.goto(fixture.origin + '/claims/' + claim.id); await page.getByRole('heading', { name: '员工垫付结算', exact: true }).waitFor(); await page.getByText('部分结算', { exact: true }).waitFor(); const settlement = await request(context, '/claims/' + claim.id + '/settlement'); assert.equal(settlement.eligible_amount, '200.000000'); assert.equal(settlement.paid_amount, '100.000000'); assert.equal(settlement.outstanding_amount, '100.000000'); await layout(page, 'employee-settlement');
  checks.push({ name: 'Web chooses approved claim employee, exact advance line and linked canonical cost; the actual claim shows eligible 200, paid 100 and outstanding 100', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); checks.push({ name: 'Both themes and five widths pass real payment detail, cash report, drilldown and employee settlement with no browser/server errors', passed: true }); passed = true;

} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离付款结算验证结束，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
