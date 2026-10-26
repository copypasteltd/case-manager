// Actual Web/API actual incurred-cost checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/actual-costs'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_costs.mjs', 'scripts/run_spend_browser.py'])].sort();
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
async function editor(page) { await page.getByRole('button', { name: '修订费用草稿', exact: true }).click(); return page.getByRole('dialog', { name: '修订费用草稿', exact: true }); }
async function createApproved(kind, owner, supplier) {
  await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'cost.browser.' + kind, name: '虚构费用依据核对制度', kind, category: 'cost.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立费用依据核对', assigned_user_id: owner.reviewer, requires_mfa: false }], reason: '隔离费用依据制度' });
  const shared = { title: '虚构费用依据 · ' + kind, category: 'cost.browser', owner_id: owner.id, currency: 'HKD', project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离费用来源', notes: '' };
  const body = kind === 'contract' ? { ...shared, direction: 'payable', type: 'service', amount: '200', counterparty_id: supplier.id, counterparty_revision_id: supplier.current_revision_id, start_date: '2025-01-15', end_date: '2027-12-31', draft_file_version_id: fixture.file_version_id, payment_schedule: null } : kind === 'purchase' ? { ...shared, purpose: '核对一笔真实发生费用', pricing_basis: 'net_plus_tax', rounding_mode: 'half_up', expected_delivery_date: '2025-01-15', supplier_candidates: [{ party_id: supplier.id, revision_id: supplier.current_revision_id }], quote_version_ids: [fixture.file_version_id], lines: [{ id: crypto.randomUUID(), description: '虚构服务', quantity: '1', unit: '项', unit_price: '200', tax_amount: '0', discount_amount: '0' }] } : { ...shared, purpose: '同一费用报销依据', pricing_basis: 'gross_includes_tax', rounding_mode: 'half_up', period_start: '2025-01-15', period_end: '2025-01-15', lines: [{ id: crypto.randomUUID(), description: '虚构同一服务', declared_amount: '200', expense_date: '2025-01-15', category: 'cost.browser', supplier: { party_id: supplier.id, revision_id: supplier.current_revision_id }, receipt_version_ids: [fixture.file_version_id], funding: 'employee_advance' }] };
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
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reviewer = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  const grants = [...['cost_recorder', 'cost_confirmer', 'cost_reverser', 'finance_reporter', 'purchase_manager', 'claim_submitter'].map(key => [me.user.id, key]), ...['cost_reader', 'purchase_reader', 'claim_reader'].map(key => [reviewer.user.id, key])];
  for (const [user, roleKey] of grants) { const role = roles.items.find(item => item.key === roleKey); assert(role); bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离费用核验临时权限' })); }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const { page, context } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构实际费用供应方', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离费用供应方' });
  await page.goto(fixture.origin + '/costs'); await page.getByRole('button', { name: '起草费用', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '起草实际费用', exact: true });
  await dialog.getByLabel('费用标题', { exact: true }).fill('虚构项目服务实际费用'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id); await dialog.getByLabel('登记或修订原因', { exact: true }).fill('登记待核实经济事项');
  const sourceKey = await dialog.getByLabel('永久来源标识', { exact: true }).inputValue(); await commit(dialog, '保存费用草稿');
  await page.getByRole('heading', { name: '虚构项目服务实际费用', exact: true }).waitFor(); const id = new URL(page.url()).pathname.split('/').at(-1);
  let cost = await request(context, '/costs/' + id); assert.equal(cost.state, 'draft'); assert.equal(cost.amount, null);
  assert.equal((await response(administrator.context, '/costs/' + id)).status, 404);
  const duplicate = await response(context, '/costs', 'POST', { title: cost.title, occurred_on: null, currency: null, amount: null, category: '', counterparty: null, allocations: [], evidence_version_ids: [], notes: '', source_key: sourceKey, project_id: fixture.project_id, classification: 'project_restricted', reason: '重复同一经济事项' }); assert.equal(duplicate.status, 409);
  checks.push({ name: 'Web saves a genuinely incomplete draft, permanent source identity rejects a second record, administration has no cost read grant', passed: true });

  const second = await login(fixture.alice); await second.page.goto(fixture.origin + '/costs/' + id); const stale = await editor(second.page); await stale.getByLabel('费用标题', { exact: true }).fill('保留的过期费用编辑'); await stale.getByLabel('登记或修订原因', { exact: true }).fill('另一个会话尚未提交');
  dialog = await editor(page); await dialog.getByLabel('实际发生日', { exact: true }).fill('2025-01-15'); await dialog.getByLabel('费用类别', { exact: true }).fill('cost.browser'); await dialog.getByLabel('原币', { exact: true }).selectOption('HKD'); await dialog.getByLabel('费用总额', { exact: true }).fill('100');
  await dialog.getByRole('button', { name: '选择费用供应方', exact: true }).click(); let picker = page.getByRole('dialog', { name: '选择往来方资料', exact: true }); await picker.getByRole('button').filter({ hasText: supplier.name }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  await dialog.getByRole('button', { name: '添加费用分摊', exact: true }).click(); await dialog.getByLabel('分摊金额 1', { exact: true }).fill('60'); await dialog.getByRole('button', { name: '添加费用分摊', exact: true }).click(); await dialog.getByLabel('分摊范围 2', { exact: true }).selectOption(''); await dialog.getByLabel('分摊金额 2', { exact: true }).fill('39');
  await dialog.getByRole('button', { name: '选择费用凭证', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择费用凭证版本', exact: true }); await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click(); await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  await dialog.getByLabel('登记或修订原因', { exact: true }).fill('固定供应方凭证与待核分摊'); await commit(dialog, '保存费用草稿');
  await stale.getByRole('button', { name: '保存费用草稿', exact: true }).click(); await stale.getByText('输入已保留。请关闭并核对最新费用或来源标识，再继续编辑。', { exact: true }).waitFor(); assert.equal(await stale.getByLabel('费用标题', { exact: true }).inputValue(), '保留的过期费用编辑'); await close(stale); await second.context.close();
  await page.getByRole('button', { name: '确认实际费用', exact: true }).click(); dialog = page.getByRole('dialog', { name: '确认实际费用', exact: true }); await dialog.getByLabel('操作依据', { exact: true }).fill('先核对不平衡分摊'); await dialog.getByRole('button', { name: '确认实际费用', exact: true }).click(); await dialog.getByRole('alert').waitFor(); assert.equal((await request(context, '/costs/' + id)).state, 'draft'); assert.equal(await dialog.getByLabel('操作依据', { exact: true }).inputValue(), '先核对不平衡分摊'); await close(dialog);
  dialog = await editor(page); await dialog.getByLabel('分摊金额 2', { exact: true }).fill('40'); await dialog.getByLabel('登记或修订原因', { exact: true }).fill('按真实金额更正分摊'); await commit(dialog, '保存费用草稿');
  await page.getByRole('button', { name: '确认实际费用', exact: true }).click(); dialog = page.getByRole('dialog', { name: '确认实际费用', exact: true }); await dialog.getByLabel('操作依据', { exact: true }).fill('本人核对实际发生日金额与固定凭证'); await commit(dialog, '确认实际费用'); cost = await request(context, '/costs/' + id); assert.equal(cost.state, 'confirmed'); assert.equal(cost.can_edit, false);
  checks.push({ name: 'Actual editor fixes party and file revisions, preserves stale concurrent input, rejects unbalanced confirmation and accepts balanced human confirmation', passed: true });
  await page.getByRole('button', { name: '查看费用修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: '费用修订记录', exact: true }); await dialog.getByRole('button').filter({ hasText: '修订 1 ·' }).click(); await dialog.getByRole('heading', { name: '准确历史修订 1', exact: true }).waitFor(); assert(await dialog.locator('.cost-history-detail').innerText().then(text => text.includes('尚未确定'))); await close(dialog);
  checks.push({ name: 'The first incomplete historical revision stays exact after later confirmation', passed: true });

  for (const [kind, label] of [['purchase', '采购'], ['claim', '报销'], ['contract', '合同']]) {
    const record = await createApproved(kind, { id: me.user.id, reviewer: reviewer.user.id }, supplier);
    await page.getByRole('button', { name: '关联业务依据', exact: true }).click(); dialog = page.getByRole('dialog', { name: '关联业务依据', exact: true }); await dialog.getByLabel('关联业务类型', { exact: true }).selectOption(kind); await dialog.getByRole('button', { name: '选择' + label + '依据', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择' + label + '依据', exact: true }); await picker.locator('button.reference-option').filter({ hasText: record.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page); await dialog.getByLabel('操作依据', { exact: true }).fill('关联同一经济事项的' + label + '依据'); await commit(dialog, '关联业务依据');
  }
  cost = await request(context, '/costs/' + id); assert.equal(cost.links.length, 3);
  let report = await request(context, '/reports/expenses?kind=incurred&from=2025-01-01&to=2025-02-01&category=cost.browser'); assert.equal(report.totals[0].net_amount, '100.000000'); assert.equal(report.totals[0].entry_count, 1);
  checks.push({ name: 'Web chooses exact approved purchase and claim plus a signed effective payable contract; all three trace one economic cost and count only once', passed: true });
  await layout(page, 'actual-cost-detail');
  await bob.page.goto(fixture.origin + '/costs/' + id); await bob.page.getByRole('heading', { name: cost.title, exact: true }).waitFor(); assert.equal(await bob.page.getByRole('button', { name: '全额冲销费用', exact: true }).count(), 0); assert.equal(await bob.page.getByRole('button', { name: '关联业务依据', exact: true }).count(), 0);
  const denied = await response(bob.context, '/costs/' + id + '/reverse', 'POST', { expected_version: cost.version, revision_id: cost.current_revision_id, content_hash: cost.content_hash, effective_on: '2025-02-10', reason: '只读访问不得冲销' }); assert([403, 404].includes(denied.status));
  checks.push({ name: 'A real read-only browser sees facts without write controls and the API also rejects its reversal', passed: true });

  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await stable(page); await page.getByRole('button', { name: '全额冲销费用', exact: true }).click(); dialog = page.getByRole('dialog', { name: '全额冲销费用', exact: true }); await dialog.getByLabel('冲销生效日', { exact: true }).fill('2025-02-10'); await dialog.getByLabel('操作依据', { exact: true }).fill('真实费用更正，原凭证与发生月保留'); await modalFocus(page); await dialog.getByLabel('操作依据', { exact: true }).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'cost-reversal-dark-390.png'), animations: 'disabled' }); await commit(dialog, '全额冲销费用');
  cost = await request(context, '/costs/' + id); assert.equal(cost.state, 'reversed'); const reversal = await request(context, '/costs/' + cost.reversal_id); assert.equal(reversal.entry_kind, 'reversal'); assert.equal(reversal.allocations[1].amount, '40.000000');
  await page.getByRole('button', { name: '查看登记记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: '费用登记记录', exact: true }); await dialog.getByText('原费用登记冲销', { exact: true }).waitFor(); assert.equal(await dialog.locator('article.cost-business-link').count(), 8); await close(dialog);
  checks.push({ name: 'Narrow-screen human reversal appends a complete inverse record and retains eight immutable original events', passed: true });

  await page.goto(fixture.origin + '/reports/expenses'); await page.getByLabel('统计月份', { exact: true }).fill('2025-01'); await page.getByLabel('费用类别', { exact: true }).fill('cost.browser'); await page.getByRole('button', { name: '查询支出', exact: true }).click(); await page.locator('.cost-net').filter({ hasText: 'HKD 100' }).waitFor(); await layout(page, 'actual-cost-report');
  await page.getByLabel('统计月份', { exact: true }).fill('2025-02'); await page.getByLabel('分摊范围', { exact: true }).selectOption('company'); await page.getByRole('button', { name: '查询支出', exact: true }).click(); await page.locator('.cost-net').filter({ hasText: 'HKD -40' }).waitFor(); await page.getByRole('link', { name: '核对费用明细', exact: true }).click(); await page.getByRole('heading', { name: '实际费用', exact: true }).waitFor(); await page.locator('.contract-row').first().waitFor(); assert.equal(await page.locator('.contract-row').count(), 1); assert(await page.locator('.contract-row').innerText().then(text => text.includes('全额冲销记录'))); await layout(page, 'actual-cost-drilldown');
  report = await request(context, '/reports/expenses?kind=incurred&from=2025-02-01&to=2025-03-01&category=cost.browser&company_only=true'); assert.equal(report.totals[0].net_amount, '-40.000000'); assert.equal(report.base_currency, null);
  checks.push({ name: 'Actual monthly report preserves January 100 and February company reversal minus 40; allocation drilldown yields exactly its contributing inverse record', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); checks.push({ name: 'Loaded detail, monthly report and drilldown pass both themes at five widths with no horizontal overflow, undersized controls or browser/server errors', passed: true }); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离实际费用验证结束，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
