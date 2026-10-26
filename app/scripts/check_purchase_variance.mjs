// Actual Web/API purchase variance checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/purchase-variance'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), ...await files('mobile/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_purchase_variance.mjs', 'scripts/run_spend_browser.py'])].sort();
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
  let purchase = await request(context, '/purchases', 'POST', { title: '虚构采购执行基线样本', category: 'purchase.variance.browser', purpose: '仅用于隔离界面核验', owner_id: user,
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
}
async function approveProposal(context, reviewer, purchase, quantity, price) {
  const plan = await request(context, '/purchases/' + purchase.id + '/execution-plan'), content = structuredClone(plan.content);
  content.lines[0].quantity = quantity; content.lines[0].unit_price = price;
  const proposal = await request(context, '/purchases/' + purchase.id + '/variances', 'POST', { ...content, title: '虚构待启用差异提案', expected_purchase_version: purchase.version, execution_plan_revision_id: plan.variance_revision_id, reason: '固定隔离差异完整范围' });
  let q = await request(context, '/requests', 'POST', { kind: 'purchase_variance', business_id: proposal.id, expected_business_version: proposal.version, business_revision_id: proposal.current_revision_id, business_content_hash: proposal.content_hash, reason: '固定差异独立审批' });
  const preview = await request(context, '/requests/' + q.id + '/submission-preview');
  q = await request(context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '独立重审完整商业差异' });
  await request(reviewer, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '准确差异批准' });
  return request(context, '/purchase-variances/' + proposal.id);
}
async function activationForm(page) {
  await page.getByRole('button', { name: '核对并启用执行基线', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '启用采购执行基线', exact: true });
  await dialog.getByLabel('本次启用依据', { exact: true }).fill('核对完整差异与现有承诺后明确启用');
  await dialog.getByLabel('已核对完整提案、现有承诺与履行依据，确认用于后续执行', { exact: true }).check();
  return dialog;
}
async function registerOrder(context, purchase, quantity) {
  const plan = await request(context, '/purchases/' + purchase.id + '/execution-plan');
  return request(context, '/purchases/' + purchase.id + '/orders', 'POST', { expected_version: purchase.version, revision_id: purchase.current_revision_id, content_hash: purchase.content_hash, execution_plan_revision_id: plan.variance_revision_id, supplier: plan.content.supplier_candidates[0], external_ref: 'SYNTH-' + crypto.randomUUID(), ordered_on: fixture.today, expected_delivery_on: fixture.today, evidence_version_ids: [fixture.file_version_id], lines: [{ purchase_line_id: purchase.lines[0].id, quantity, unit_price: plan.content.lines[0].unit_price, tax_amount: '0', discount_amount: '0' }], notes: '', reason: '隔离会话登记实际订购事实' });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  for (const [user, roleKey] of [[me.user.id, 'purchase_manager'], [me.user.id, 'purchase_operator'], [me.user.id, 'purchase_variance_manager'], [me.user.id, 'purchase_plan_activator'], [reviewer.user.id, 'purchase_reader'], [reviewer.user.id, 'purchase_variance_reader']]) {
    const role = roles.items.find(item => item.key === roleKey); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'project', scope_id: fixture.project_id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离采购差异验证的临时授权' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const supplier = await request(alice.context, '/parties', 'POST', { name: '虚构采购差异供应商', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离差异供应方' });
  for (const kind of ['purchase', 'purchase_variance']) await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'purchase.variance.browser.' + kind, name: kind === 'purchase' ? '虚构原采购核对制度' : '虚构采购差异独立重审制度', kind, category: 'purchase.variance.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '准确完整商业范围核对', assigned_user_id: reviewer.user.id, requires_mfa: false }], reason: '隔离检查的明确独立制度' });
  const { page, context } = alice, original = await createApproved(context, bob.context, me.user.id, supplier);
  const purchase = await registerOrder(context, original, '4');
  await page.goto(fixture.origin + '/purchases/' + purchase.id); await page.getByRole('button', { name: '起草采购差异提案', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '起草采购差异提案', exact: true });
  assert.equal(await dialog.getByLabel('币种', { exact: true }).getAttribute('readonly'), ''); assert.equal(await dialog.getByLabel('明细描述', { exact: true }).getAttribute('readonly'), ''); assert.equal(await dialog.getByLabel('计价方式', { exact: true }).isDisabled(), true);
  assert.equal(await dialog.getByRole('button', { name: '添加明细', exact: true }).count(), 0);
  await dialog.getByLabel('采购差异提案标题', { exact: true }).fill('虚构数量单价差异重审'); await dialog.getByLabel('数量', { exact: true }).fill('12'); await dialog.getByLabel('单价', { exact: true }).fill('3'); await dialog.getByLabel('创建依据', { exact: true }).fill('隔离样本完整差异与原行身份核对');
  await commit(dialog, '保存采购差异提案草稿'); await page.waitForURL('**/purchase-variances/*'); const proposalID = new URL(page.url()).pathname.split('/').at(-1);
  await page.getByRole('button', { name: '准备差异送审申请', exact: true }).click(); dialog = page.getByRole('dialog', { name: '准备采购差异申请', exact: true }); await dialog.getByLabel('准备申请依据', { exact: true }).fill('固定原执行范围与完整拟执行提案'); await commit(dialog, '建立申请草稿'); await page.waitForURL('**/requests/*'); const requestID = new URL(page.url()).pathname.split('/').at(-1);
  await page.getByRole('heading', { name: '送审时完整执行依据', exact: true }).waitFor(); await page.getByRole('heading', { name: '完整拟执行内容', exact: true }).waitFor(); await layout(page, 'variance-request-snapshot');
  await page.getByRole('button', { name: '核对并送审', exact: true }).click(); dialog = page.getByRole('dialog', { name: '核对采购差异送审预览', exact: true }); await dialog.getByRole('heading', { name: /虚构采购差异独立重审制度/ }).waitFor(); await dialog.getByLabel('送审依据', { exact: true }).fill('完整对照前后内容后送审'); await commit(dialog, '确认以此内容送审');
  await bob.page.goto(fixture.origin + '/requests/' + requestID); await bob.page.getByRole('button', { name: '处理当前步骤', exact: true }).click(); dialog = bob.page.getByRole('dialog', { name: '处理当前审批步骤', exact: true }); await dialog.getByLabel('决定原因', { exact: true }).fill('本人核对原承诺与完整拟执行差异'); await commit(dialog, '确认同意当前修订');
  assert.equal((await request(context, '/purchases/' + purchase.id + '/execution-plan')).variance_id, null);
  checks.push({ name: 'Web creates a full proposal from the fixed current basis and independently submits/reviews both complete scopes without activating it', passed: true });
  await page.goto(fixture.origin + '/purchase-variances/' + proposalID); await page.getByText('差异提案已获批，尚未启用。订单仍按当前执行基线登记。', { exact: true }).waitFor(); assert.equal(await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '采购管理', exact: true }).getAttribute('class'), 'active'); await page.locator('.breadcrumb strong').filter({ hasText: /^采购管理$/ }).waitFor(); await layout(page, 'variance-approved-inactive');
  dialog = await activationForm(page); await layout(page, 'variance-activation-form', dialog); await commit(dialog, '确认启用此执行基线');
  await page.getByText('此提案是当前已启用的执行基线。', { exact: true }).waitFor(); let plan = await request(context, '/purchases/' + purchase.id + '/execution-plan'); assert.equal(plan.variance_id, proposalID); assert.equal(plan.amount, '36.000000');
  await page.goto(fixture.origin + '/purchases/' + purchase.id); await page.getByRole('heading', { name: '执行基线与差异重审', exact: true }).waitFor(); dialog = await fillOrder(page, 'SYNTH-NEW-BASELINE', '8', supplier); await dialog.getByText(/执行数量 12 件 · 固定单价 3(?:\.0+)? HKD/).waitFor(); await commit(dialog, '确认保存订单登记');
  const orders = await request(context, '/purchases/' + purchase.id + '/orders'); assert.equal(orders.items.length, 2); assert.equal(orders.items[0].amount, '24.000000'); assert.equal(orders.items[1].amount, '8.000000'); assert.equal(orders.items[0].execution_plan_revision_id, plan.variance_revision_id); assert.equal(orders.items[1].execution_plan_revision_id, null); assert.equal((await request(context, '/purchases/' + purchase.id)).content_hash, original.content_hash); await layout(page, 'variance-enabled-orders');
  checks.push({ name: 'Explicit human MFA activation enables the reviewed cap and Web order uses its fixed new price while old orders and original approval remain intact', passed: true });
  await page.getByRole('button', { name: '查看履行记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: '采购履行追加记录', exact: true }); await dialog.getByText('启用差异执行基线', { exact: true }).waitFor(); await close(dialog);
  const stalePurchase = await createApproved(context, bob.context, me.user.id, supplier), stale = await approveProposal(context, bob.context, stalePurchase, '12', '3');
  await page.goto(fixture.origin + '/purchase-variances/' + stale.id); dialog = await activationForm(page); await registerOrder(context, stalePurchase, '1'); await dialog.getByRole('button', { name: '确认启用此执行基线', exact: true }).click(); await dialog.getByText('采购或授权已变化，未启用此提案。填写内容仍保留，请关闭后核对当前依据。', { exact: true }).waitFor(); assert.equal(await dialog.getByLabel('本次启用依据', { exact: true }).inputValue(), '核对完整差异与现有承诺后明确启用'); assert.equal(await dialog.getByRole('button', { name: '确认启用此执行基线', exact: true }).isDisabled(), true); assert.equal((await request(context, '/purchases/' + stalePurchase.id + '/execution-plan')).variance_id, null); await layout(page, 'variance-conflict-preserved', dialog); await close(dialog);
  checks.push({ name: 'An external order changes an open activation form basis; stale activation is rejected, preserves the human reason and disables resubmission', passed: true });
  await bob.page.goto(fixture.origin + '/purchase-variances/' + proposalID); await bob.page.getByRole('heading', { name: '完整拟执行内容', exact: true }).waitFor(); assert.equal(await bob.page.getByRole('button', { name: '核对并启用执行基线', exact: true }).count(), 0); assert.equal(await bob.page.getByRole('button', { name: '查看差异商业修订', exact: true }).count(), 0); await layout(bob.page, 'variance-read-only');
  checks.push({ name: 'A limited reader can review current fixed business evidence with no update activation or commercial-history controls', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离采购差异验证结束，撤销临时授权' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_orders: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
