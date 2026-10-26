// Actual Web/API purchase fulfillment checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/purchase-fulfillment'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_purchase_fulfillment.mjs', 'scripts/run_spend_browser.py'])].sort();
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
  await page.locator('.purchase-fulfillment .purchase-order-card').first().waitFor();
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + theme + '/' + width + ' overflows');
      const main = page.locator('#main-content');
      assert.equal(await main.locator('button:visible,a:visible,select:visible,input:visible').evaluateAll(nodes => nodes.some(node => { const rect = node.getBoundingClientRect(); return rect.width < 23.5 || rect.height < 23.5; })), false, name + ' has an undersized visible control');
      if ((theme === 'light' && width === 1440) || (theme === 'dark' && width === 390)) {
        await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'instant' })); await stable(page);
        await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), fullPage: true, animations: 'disabled' });
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
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
async function openOrder(page, externalRef) {
  await page.locator('.purchase-order-card').filter({ hasText: externalRef }).getByRole('button', { name: '查看订单与验收', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '订单与验收依据', exact: true });
  await dialog.getByRole('heading', { name: externalRef, exact: true }).waitFor();
  return dialog;
}
async function fillReceipt(page, orderDialog, quantity, returned = false) {
  if (!returned) await orderDialog.getByRole('button', { name: '登记本单分批验收', exact: true }).click();
  else await orderDialog.getByRole('button', { name: '登记此验收明细退货', exact: true }).first().click();
  const dialog = page.getByRole('dialog', { name: returned ? '登记准确来源退货' : '登记订单分批验收', exact: true });
  await dialog.getByLabel(returned ? '实际退货日期' : '实际验收日期', { exact: true }).fill(fixture.today);
  await dialog.getByLabel(returned ? '退货数量' : '验收行 1数量', { exact: true }).fill(quantity);
  await dialog.getByLabel(returned ? '退货结果依据' : '验收行 1结果依据', { exact: true }).fill(returned ? '按准确通过验收来源退回样本数量' : '逐项核对样本数量与规格');
  await selectFile(page, dialog);
  await dialog.getByLabel('本次登记依据', { exact: true }).fill(returned ? '来源退货的固定样本依据' : '分批验收的固定样本依据');
  return dialog;
}
async function modalFocus(page) {
  await page.waitForFunction(() => { const top = [...document.querySelectorAll('dialog[open]')].at(-1); return !top || top.contains(document.activeElement); });
}
async function commit(dialog, label) { await dialog.getByRole('button', { name: label, exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function close(dialog) { await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function facts(context, id) { return request(context, '/purchases/' + id + '/fulfillment'); }
async function createApproved(context, reviewer, user, supplier) {
  let purchase = await request(context, '/purchases', 'POST', { title: '虚构采购履行完整样本', category: 'purchase.fulfillment.browser', purpose: '仅用于隔离界面核验', owner_id: user,
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
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  for (const [user, roleKey] of [[me.user.id, 'purchase_manager'], [me.user.id, 'purchase_operator'], [reviewer.user.id, 'purchase_reader']]) {
    const role = roles.items.find(item => item.key === roleKey); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'project', scope_id: fixture.project_id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离履行验证的临时权限' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const supplier = await request(alice.context, '/parties', 'POST', { name: '虚构采购履行供应方', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离履行供应方' });
  await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'purchase.fulfillment.browser', name: '虚构采购履行核对制度', kind: 'purchase', category: 'purchase.fulfillment.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '准确商业依据核对', assigned_user_id: reviewer.user.id, requires_mfa: false }], reason: '隔离测试的明确审批制度' });
  const purchase = await createApproved(alice.context, bob.context, me.user.id, supplier), { page, context } = alice;
  await page.goto(fixture.origin + '/purchases/' + purchase.id); await page.getByRole('heading', { name: purchase.title, exact: true }).waitFor();
  let dialog = await fillOrder(page, 'SYNTH-ORDER-A', '10', supplier);
  await commit(dialog, '确认保存订单登记');
  let summary = await facts(context, purchase.id); assert.equal(summary.lines[0].reserved_quantity, '10.000000'); assert.equal(summary.lines[0].reserved_amount, '20.000000');
  checks.push({ name: 'Web registers an exact approved order with fixed evidence and immutable original quantities', passed: true });

  let detail = await openOrder(page, 'SYNTH-ORDER-A'); dialog = await fillReceipt(page, detail, '4');
  await dialog.getByRole('button', { name: '添加验收行', exact: true }).click();
  await dialog.getByLabel('验收行 2 · 结果', { exact: true }).selectOption('rejected'); await dialog.getByLabel('验收行 2数量', { exact: true }).fill('2'); await dialog.getByLabel('验收行 2结果依据', { exact: true }).fill('两件规格未符合约定，拒收后等待补送');
  await commit(dialog, '确认保存本批验收'); await detail.getByText('通过验收 4', { exact: false }).waitFor();
  assert.equal((await facts(context, purchase.id)).lines[0].net_accepted_quantity, '4.000000');
  checks.push({ name: 'Web appends one batch with accepted and rejected lines while net acceptance stays four', passed: true });

  dialog = await fillReceipt(page, detail, '7');
  await dialog.getByRole('button', { name: '确认保存本批验收', exact: true }).click(); await dialog.getByRole('alert').getByText('本批通过与拒收数量合计超过订单待收数量', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('验收行 1数量', { exact: true }).inputValue(), '7');
  assert.equal((await facts(context, purchase.id)).receipt_count, 1); await close(dialog);
  await detail.getByRole('button', { name: '取消本单未交付余量', exact: true }).click(); dialog = page.getByRole('dialog', { name: '取消订单未交付余量', exact: true }); await dialog.getByLabel('取消依据', { exact: true }).fill('余下交付改为重新订购，原始记录保留'); await commit(dialog, '确认取消本单余量');
  await detail.getByText('订购承诺已取消', { exact: true }).waitFor(); await close(detail);
  summary = await facts(context, purchase.id); assert.equal(summary.lines[0].reserved_quantity, '4.000000'); assert.equal(summary.lines[0].reserved_amount, '8.000000'); assert.equal(summary.lines[0].outstanding_quantity, '0.000000');
  const original = (await request(context, '/purchases/' + purchase.id + '/orders')).items[0]; assert.equal(original.lines[0].quantity, '10.000000'); assert.equal(original.amount, '20.000000');
  checks.push({ name: 'Overreceipt preserves input and cancellation releases only unaccepted commitment without rewriting order facts', passed: true });

  const second = await login(fixture.alice); await second.page.goto(fixture.origin + '/purchases/' + purchase.id); const stale = await fillOrder(second.page, 'SYNTH-STALE', '6', supplier);
  dialog = await fillOrder(page, 'SYNTH-ORDER-B', '6', supplier); await commit(dialog, '确认保存订单登记');
  await stale.getByRole('button', { name: '确认保存订单登记', exact: true }).click(); await stale.getByText('采购或授权已变化。填写内容仍保留，请关闭后核对最新记录。', { exact: true }).waitFor();
  assert.equal(await stale.getByLabel('实际订单编号', { exact: true }).inputValue(), 'SYNTH-STALE'); assert.equal((await facts(context, purchase.id)).order_count, 2); await close(stale); await second.context.close();
  checks.push({ name: 'Two actual browser sessions reject stale order submission and preserve the complete entered draft', passed: true });

  detail = await openOrder(page, 'SYNTH-ORDER-A'); dialog = await fillReceipt(page, detail, '1', true); await commit(dialog, '确认保存来源退货'); await detail.getByText('退货 1', { exact: false }).waitFor(); await close(detail);
  summary = await facts(context, purchase.id); assert.equal(summary.lines[0].net_accepted_quantity, '3.000000'); assert.equal(summary.lines[0].reserved_quantity, '9.000000');
  detail = await openOrder(page, 'SYNTH-ORDER-B'); dialog = await fillReceipt(page, detail, '6'); await commit(dialog, '确认保存本批验收'); await detail.getByText('净验收齐全', { exact: true }).waitFor(); await close(detail);
  await page.getByRole('button', { name: '取消采购后续履行', exact: true }).click(); dialog = page.getByRole('dialog', { name: '取消采购后续履行', exact: true }); await dialog.getByLabel('取消依据', { exact: true }).fill('关闭隔离样本后续履行，保留全部已发生事实'); await commit(dialog, '确认取消后续履行');
  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await stable(page);
  detail = await openOrder(page, 'SYNTH-ORDER-B'); dialog = await fillReceipt(page, detail, '1', true); await commit(dialog, '确认保存来源退货'); await detail.getByText('退货 1', { exact: false }).waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false); await modalFocus(page);
  await detail.locator('.purchase-receipt-card').first().scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'order-source-return-dark-390.png'), animations: 'disabled' });
  await detail.locator('.purchase-receipt-card').last().scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'order-accepted-source-dark-390.png'), animations: 'disabled' });
  await close(detail); await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
  summary = await facts(context, purchase.id); assert.equal(summary.lifecycle, 'cancelled'); assert.equal(summary.lines[0].net_accepted_quantity, '8.000000'); assert.equal(summary.lines[0].reserved_quantity, '8.000000'); assert.equal(summary.lines[0].reserved_amount, '16.000000'); assert.equal(summary.lines[0].outstanding_quantity, '0.000000');
  checks.push({ name: 'Source-specific returns remain appendable after order and whole-purchase cancellation and retain cancelled lifecycle', passed: true });

  await bob.page.goto(fixture.origin + '/purchases/' + purchase.id); await bob.page.locator('.purchase-order-card').first().waitFor(); assert.equal(await bob.page.getByRole('button', { name: '登记已订购订单', exact: true }).count(), 0);
  assert.equal((await response(administrator.context, '/purchases/' + purchase.id + '/orders')).status, 404);
  const forbidden = await response(bob.context, '/purchases/' + purchase.id + '/orders', 'POST', { expected_version: summary.version, revision_id: purchase.current_revision_id, content_hash: purchase.content_hash, reason: '只读用户不得登记', supplier: { party_id: supplier.id, revision_id: supplier.current_revision_id }, external_ref: 'FORBIDDEN', ordered_on: fixture.today, expected_delivery_on: fixture.today, evidence_version_ids: [fixture.file_version_id], lines: [{ purchase_line_id: purchase.lines[0].id, quantity: '1', unit_price: '2', tax_amount: '0', discount_amount: '0' }], notes: '' }); assert([403, 404].includes(forbidden.status));
  await page.getByRole('button', { name: '查看履行记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: '采购履行追加记录', exact: true }); await dialog.locator('.party-revision').first().waitFor(); assert.equal(await dialog.locator('.party-revision').count(), 8); await close(dialog);
  const events = await request(context, '/purchases/' + purchase.id + '/execution-events'); assert.equal(events.items.length, 8); assert.equal(events.items[0].lifecycle, 'cancelled');
  checks.push({ name: 'Readers see bounded immutable history, cannot execute, and system administration grants no procurement content access', passed: true });

  await layout(page, 'purchase-fulfillment'); assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0);
  checks.push({ name: 'Loaded procurement pages pass both themes at 320, 390, 768, 1024 and 1440 widths; actual source-return dialog captured on narrow screen', passed: true });
  passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离采购履行验证结束，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
