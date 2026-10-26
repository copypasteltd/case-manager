// Actual Web/API legal contract completion and termination checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/contract-closure'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_contract_closure.mjs', 'scripts/run_spend_browser.py'])].sort();
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

async function newPayment(context, supplier, direction, amount, title, original = null) {
  let p = await request(context, '/payments', 'POST', { title, direction, paid_at: '2025-02-01T02:00:00+08:00', currency: 'HKD', amount, category: 'contract.browser', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, employee_id: null, refund_of_id: original?.id ?? null, method_label: '虚构转账标签', external_ref: '', evidence_version_ids: [fixture.file_version_id], notes: '', source_key: 'contract-browser:' + crypto.randomUUID(), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离真实资金依据' });
  return request(context, '/payments/' + p.id + '/record', 'POST', { ...base(p), expected_source_version: original?.version ?? null, refund_allocations: [] });
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

async function newContract(context, reviewer, supplier, title, owner, mode = 'scheduled', parent = null) {
  let record = await request(context, '/contracts', 'POST', { title, direction: mode === 'no_payment' ? 'none' : 'payable', type: 'service', category: 'execution',
    counterparty_id: supplier.id, counterparty_revision_id: supplier.current_revision_id, owner_id: owner, currency: 'HKD', amount: mode === 'no_payment' ? '0' : '100',
    start_date: fixture.today, end_date: '2030-01-01', draft_file_version_id: fixture.file_version_id, project_id: fixture.project_id, classification: 'project_restricted', parent_contract_id: parent,
    reason: '隔离合同完成终止验证', payment_schedule: mode === 'no_payment' ? { mode, reason: '经核实无收付款义务', items: [] } : { mode: 'scheduled', reason: '完整交付节点', items: [{ id: crypto.randomUUID(), title: '完整交付', amount: '100', due_date: fixture.today, conditions: '核实交付和约定条件' }] } });
  let q = await request(context, '/requests', 'POST', { kind: 'contract', business_id: record.id, expected_business_version: record.version, business_revision_id: record.current_revision_id, business_content_hash: record.content_hash, reason: '固定准确合同商业修订' });
  const preview = await request(context, '/requests/' + q.id + '/submission-preview');
  q = await request(context, '/requests/' + q.id + '/submit', 'POST', { expected_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, workflow_definition_id: preview.workflow.id, preview_hash: preview.preview_hash, reason: '提交准确合同依据' });
  await request(reviewer, '/approval-steps/' + q.steps[0].id + '/decisions', 'POST', { expected_version: q.steps[0].version, request_version: q.version, revision_id: q.current_revision_id, content_hash: q.content_hash, decision: 'approve', reason: '独立核实合同依据' });
  record = await request(context, '/contracts/' + record.id);
  record = await request(context, '/contracts/' + record.id + '/signature', 'POST', { ...base(record), signed_on: fixture.today, signed_file_version_id: fixture.file_version_id, attestation: '已人工核对隔离签署件和准确批准修订', reason: '登记实际签署依据' });
  return request(context, '/contracts/' + record.id + '/transition', 'POST', { ...base(record), action: 'activate', signature_id: record.signature.id });
}
async function closureForm(page, action, unresolved) {
  await page.getByRole('region', { name: '合同完成与终止', exact: true }).getByRole('button', { name: action, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: action, exact: true });
  const terms = dialog.getByLabel('本人已核实准确合同条款、实际履行情况和本次凭证。', { exact: true }); assert.equal(await terms.isChecked(), false);
  assert.equal(await dialog.getByRole('button', { name: action, exact: true }).isDisabled(), true);
  await dialog.getByLabel('实际完成或终止日期', { exact: true }).fill(fixture.today);
  await terms.check(); await dialog.getByLabel('条款与材料核实说明', { exact: true }).fill('人工核对实际履行、准确条款和下列固定凭证。');
  if (unresolved) {
    const ack = dialog.getByLabel('本人确认上述未结事项在终止后仍须处理。', { exact: true }); assert.equal(await ack.isChecked(), false);
    await ack.check(); await dialog.getByLabel('各项未结事项后续处理', { exact: true }).fill('未结节点保留；负责人另行核对交付、应付款与实际结算。');
  }
  await selectProof(page, dialog, '选择固定合同结束凭证', '选择准确合同结束凭证');
  await dialog.getByLabel('登记原因', { exact: true }).fill('依据准确版本人工登记合同法律阶段'); return dialog;
}
async function completeNode(context, contract, cash) {
  let view = await request(context, '/contracts/' + contract.id + '/finance');
  await request(context, '/contracts/' + contract.id + '/finance/entries', 'POST', { expected_version: view.version, contract_version: view.contract_version, milestone_id: view.nodes[0].id, kind: 'cash', source_id: cash.id, source_version: cash.version, source_revision_id: cash.current_revision_id, source_content_hash: cash.content_hash, source_event_id: null, amount: '100', reason: '隔离实际付款完整归属' });
  view = await request(context, '/contracts/' + contract.id + '/finance');
  await request(context, '/contracts/' + contract.id + '/finance/milestones/' + view.nodes[0].id + '/review', 'POST', { expected_version: view.version, contract_version: view.contract_version, expected_fingerprint: view.nodes[0].fingerprint, conditions_satisfied: true, acknowledge_difference: false, acknowledge_unattributed_refunds: false, evidence_version_ids: [fixture.file_version_id], reason: '核实当前完整交付与实际资金' });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  for (const key of ['contract_closer', 'contract_reconciler', 'cost_recorder', 'cost_confirmer', 'payment_recorder', 'payment_reverser']) {
    const role = roles.items.find(value => value.key === key); assert(role, key);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: me.user.id, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离合同结束验证临时授权' }));
  }
  await alice.context.close(); alice = await login(fixture.alice); const { context, page } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构履约协作方', kind: 'organization', relationships: ['supplier', 'customer'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离合同结束往来方' });
  const terminated = await newContract(context, bob.context, supplier, '虚构待结事项终止合同', me.user.id);
  await page.goto(fixture.origin + '/contracts/' + terminated.id);
  let region = page.getByRole('region', { name: '合同完成与终止', exact: true }); await region.getByText('计划节点与净收付存在差额', { exact: true }).waitFor();
  assert.equal(await region.getByRole('button', { name: '登记合同完成', exact: true }).count(), 0);
  let dialog = await closureForm(page, '登记合同终止', true); await dialogLayout(page, dialog, 'contract-termination-form'); await commit(dialog, '登记合同终止');
  await region.getByRole('heading', { name: '合同终止记录', exact: true }).waitFor();
  let recorded = await request(context, '/contracts/' + terminated.id + '/closure'); assert.equal(recorded.action, 'terminated'); assert.equal(recorded.basis_state, 'current'); assert.equal(recorded.basis.nodes[0].cash_amount, '0'); assert.equal(recorded.acknowledge_unsettled, true); assert.equal(recorded.attachments[0].version_id, fixture.file_version_id);
  await layout(page, 'contract-terminated');
  checks.push({ name: 'Web termination fixes unresolved obligations and actual proof with unchecked human acknowledgements, MFA and independent legal record', passed: true });

  const late = await newPayment(context, supplier, 'outgoing', '100', '虚构终止后的实际付款');
  const view = await request(context, '/contracts/' + terminated.id + '/finance');
  await request(context, '/contracts/' + terminated.id + '/finance/entries', 'POST', { expected_version: view.version, contract_version: view.contract_version, milestone_id: view.nodes[0].id, kind: 'cash', source_id: late.id, source_version: late.version, source_revision_id: late.current_revision_id, source_content_hash: late.content_hash, source_event_id: null, amount: '100', reason: '登记终止后的真实资金归属' });
  await page.reload(); await region.getByText(/登记后的财务、结算或补充协议依据已变化/).waitFor(); recorded = await request(context, '/contracts/' + terminated.id + '/closure'); assert.equal(recorded.basis_state, 'changed'); assert.equal((await request(context, '/contracts/' + terminated.id)).lifecycle, 'terminated');
  await region.getByText('查阅登记时的固定依据', { exact: true }).click(); await layout(page, 'contract-closed-basis-changed');
  checks.push({ name: 'Later real financial attribution remains available, marks changed basis and preserves the original termination snapshot and stage', passed: true });

  const completed = await newContract(context, bob.context, supplier, '虚构完整履行合同', me.user.id), cash = await newPayment(context, supplier, 'outgoing', '100', '虚构完整履行实际付款');
  await completeNode(context, completed, cash); await page.goto(fixture.origin + '/contracts/' + completed.id); await region.getByRole('button', { name: '登记合同完成', exact: true }).waitFor();
  dialog = await closureForm(page, '登记合同完成', false); await dialogLayout(page, dialog, 'contract-completion-form'); await commit(dialog, '登记合同完成');
  await region.getByRole('heading', { name: '合同完成记录', exact: true }).waitFor(); recorded = await request(context, '/contracts/' + completed.id + '/closure'); assert.equal(recorded.basis_state, 'current'); assert.equal(recorded.basis.nodes[0].review_current, true); assert.equal(recorded.basis.nodes[0].conditions_satisfied, true); assert.equal((await request(context, '/contracts/' + completed.id)).lifecycle, 'completed'); await layout(page, 'contract-completed');
  checks.push({ name: 'Web completion requires paid exact nodes and current condition review, then persists actual date human terms and fixed proof', passed: true });

  const stale = await newContract(context, bob.context, supplier, '虚构结束依据并发合同', me.user.id), staleCash = await newPayment(context, supplier, 'outgoing', '100', '虚构并发合同付款'); await completeNode(context, stale, staleCash);
  await page.goto(fixture.origin + '/contracts/' + stale.id); await region.getByRole('button', { name: '登记合同完成', exact: true }).waitFor(); dialog = await closureForm(page, '登记合同完成', false);
  await newPayment(context, supplier, 'refund', '10', '虚构外部新退款', staleCash);
  await dialog.getByRole('button', { name: '登记合同完成', exact: true }).click(); await dialog.getByText(/输入已保留/).waitFor(); assert.equal(await dialog.getByLabel('登记原因', { exact: true }).inputValue(), '依据准确版本人工登记合同法律阶段'); assert.equal(await dialog.getByLabel('登记原因', { exact: true }).isDisabled(), true);
  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark', { force: true }); await dialog.getByText(/输入已保留/).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'contract-closure-stale-basis-dark-390.png'), animations: 'disabled' }); await close(dialog);
  assert.equal((await request(context, '/contracts/' + stale.id)).lifecycle, 'effective');
  checks.push({ name: 'External actual refund changes the fixed basis without finance version, rejects the old open form and preserves disabled input', passed: true });

  const parent = await newContract(context, bob.context, supplier, '虚构有补充协议的主合同', me.user.id, 'no_payment'); await newContract(context, bob.context, supplier, '虚构仍生效补充协议', me.user.id, 'no_payment', parent.id);
  await page.goto(fixture.origin + '/contracts/' + parent.id); await region.getByText('仍有生效的补充协议，须先分别处理', { exact: true }).waitFor(); assert.equal(await region.getByRole('button', { name: /登记合同/ }).count(), 0);
  const parentPreview = await request(context, '/contracts/' + parent.id + '/closure-preview'); assert.equal(parentPreview.can_complete, false); assert.equal(parentPreview.can_terminate, false); await layout(page, 'contract-parent-supplement-blocked');
  checks.push({ name: 'Effective supplement blocks both parent close actions instead of cascading its legal status', passed: true });

  await bob.page.goto(fixture.origin + '/contracts/' + completed.id); const limitedRegion = bob.page.getByRole('region', { name: '合同完成与终止', exact: true }); await limitedRegion.getByText(/未结情况未知/).waitFor(); assert.equal(await limitedRegion.getByRole('button').count(), 0);
  const limited = await request(bob.context, '/contracts/' + completed.id + '/closure-preview'); assert.equal(limited.basis, null); assert.equal(limited.fingerprint, null); assert.equal((await response(bob.context, '/contracts/' + completed.id + '/closure')).status, 404); await layout(bob.page, 'contract-closure-read-only');
  checks.push({ name: 'Limited reader receives unknown closure basis with no controls, fixed amounts, fingerprints or historical disclosure', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离合同结束验证完成，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
