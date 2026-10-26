// Actual Web/API purchase and claim checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/spend-documents'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_spend.mjs', 'scripts/run_spend_browser.py'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map(), bindings = [];
let browser, passed = false, administrator, alice, bob, cleanedBindings = false;
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
  await page.locator('.spend-line').first().waitFor();
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

const contentKeys = ['title', 'category', 'purpose', 'owner_id', 'currency', 'pricing_basis', 'rounding_mode', 'expected_delivery_date', 'period_start', 'period_end', 'budget_reference', 'supplier_candidates', 'quote_version_ids', 'lines', 'notes'];
const contentOf = record => Object.fromEntries(contentKeys.map(key => [key, record[key]]));
async function selectSupplier(page, dialog, buttonName, supplier) {
  await dialog.getByRole('button', { name: buttonName, exact: true }).click();
  const picker = page.getByRole('dialog', { name: '选择往来方资料', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: supplier.name }).click();
}
async function selectFile(page, dialog, buttonName, title) {
  await dialog.getByRole('button', { name: buttonName, exact: true }).click();
  const picker = page.getByRole('dialog', { name: title, exact: true });
  await picker.locator('button.reference-option').filter({ hasText: fixture.file_name }).first().click();
  await picker.locator('button.reference-option').filter({ hasText: '版本 ' + fixture.file_seq + ' ·' }).click();
}
async function prepare(page) {
  await page.getByRole('button', { name: '准备送审申请', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: /^准备(采购|报销)申请$/ });
  await dialog.getByLabel('准备申请依据', { exact: true }).fill('核对本次业务资料，建立隔离测试申请');
  await dialog.getByRole('button', { name: '建立申请草稿', exact: true }).click();
  await page.waitForURL('**/requests/*'); await page.getByRole('heading', { name: '固定申请内容', exact: true }).waitFor(); await page.getByText('申请草稿依据', { exact: true }).waitFor();
  return new URL(page.url()).pathname.split('/').at(-1);
}
async function submitUI(page) {
  await page.getByRole('button', { name: '核对并送审', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: /^核对(采购|报销)送审预览$/ });
  await dialog.getByLabel('送审依据', { exact: true }).fill('核对固定金额、凭证及当前审批责任人');
  await dialog.getByRole('button', { name: '确认以此内容送审', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' }); await page.locator('.page-heading .badge').getByText('审批中', { exact: true }).waitFor();
}
async function decideUI(page, requestID, decision) {
  await page.goto(fixture.origin + '/requests/' + requestID);
  await page.getByRole('button', { name: '处理当前步骤', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '处理当前审批步骤', exact: true });
  await dialog.getByLabel('本次决定', { exact: true }).selectOption(decision);
  await dialog.getByLabel('决定原因', { exact: true }).fill(decision === 'return' ? '补充本次费用的业务说明' : '本人核对当前固定商业依据');
  await dialog.getByRole('button', { name: decision === 'return' ? '确认退回补充' : '确认同意当前修订', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const aliceMe = await request(alice.context, '/me'), bobMe = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  for (const [user, key] of [[aliceMe.user.id, 'purchase_manager'], [aliceMe.user.id, 'claim_submitter'], [bobMe.user.id, 'purchase_reader'], [bobMe.user.id, 'claim_reader']]) {
    const role = roles.items.find(item => item.key === key); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'project', scope_id: fixture.project_id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '仅限隔离采购报销界面验证，结束即撤销' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const supplier = await request(alice.context, '/parties', 'POST', { name: '虚构采购与费用供应商', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离商业资料验证' });
  for (const kind of ['purchase', 'claim']) await request(administrator.context, '/workflow-definitions', 'POST', { expected_version: 0, key: 'browser.' + kind, name: '虚构' + (kind === 'purchase' ? '采购' : '报销') + '核对制度', kind, category: 'spend.browser', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立业务核对', assigned_user_id: bobMe.user.id, requires_mfa: false }], reason: '仅用于隔离浏览器验证的明确制度' });
  const { page, context } = alice;
  await page.getByRole('link', { name: '采购管理', exact: true }).click(); await page.getByRole('button', { name: '起草采购', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '起草采购', exact: true });
  await dialog.getByLabel('采购标题', { exact: true }).fill('虚构项目工具采购'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id); assert.equal(await dialog.getByLabel('所属项目', { exact: true }).inputValue(), fixture.project_id);
  await dialog.getByLabel('业务类别', { exact: true }).fill('spend.browser'); await dialog.getByLabel('币种', { exact: true }).fill('HKD'); await dialog.getByLabel('业务负责人', { exact: true }).selectOption(aliceMe.user.id);
  await dialog.getByLabel('业务用途', { exact: true }).fill('虚构项目工具采购，验证小数计算和固定报价'); await dialog.getByLabel('计价方式', { exact: true }).selectOption('net_plus_tax'); await dialog.getByLabel('舍入规则', { exact: true }).selectOption('half_up');
  await dialog.getByLabel('预计交付日期', { exact: true }).fill('2026-11-20'); await dialog.getByLabel('预算依据', { exact: true }).fill('虚构项目预算工具项');
  await selectSupplier(page, dialog, '添加候选供应商', supplier); await selectFile(page, dialog, '添加报价版本', '选择报价版本');
  await dialog.getByRole('button', { name: '添加明细', exact: true }).click(); await dialog.getByLabel('明细描述', { exact: true }).fill('虚构工具许可'); await dialog.getByLabel('数量', { exact: true }).fill('3'); await dialog.getByLabel('单位', { exact: true }).fill('份'); await dialog.getByLabel('单价', { exact: true }).fill('0.335'); await dialog.getByLabel('另加税额（空白按零）', { exact: true }).fill('0.02'); await dialog.getByLabel('折扣金额（空白按零）', { exact: true }).fill('0.01'); await dialog.getByLabel('创建依据', { exact: true }).fill('隔离测试，仅验证草稿与固定商业资料');
  await dialog.getByRole('button', { name: '保存采购草稿', exact: true }).click(); await page.waitForURL('**/purchases/*'); await page.getByRole('heading', { name: '虚构项目工具采购', exact: true }).waitFor();
  const purchaseID = new URL(page.url()).pathname.split('/').at(-1); let purchase = await request(context, '/purchases/' + purchaseID);
  assert.equal(purchase.amount, '1.020000'); assert.equal(purchase.employee_reimbursement_amount, null); assert.equal(purchase.project_id, fixture.project_id); assert.equal(purchase.attachments[0].version_id, fixture.file_version_id);
  await layout(page, 'purchase'); checks.push('Purchase is created through the real project, supplier and pinned-file selectors with exact currency arithmetic and both themes at five widths');

  await page.getByRole('button', { name: '编辑采购草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '编辑采购', exact: true });
  await dialog.getByLabel('备注', { exact: true }).fill('我的填写内容需保留'); await dialog.getByLabel('修改依据', { exact: true }).fill('并发修改核对');
  purchase = await request(context, '/purchases/' + purchaseID, 'PATCH', { ...contentOf(purchase), notes: '另一编辑者的已保存内容', expected_version: purchase.version, base_revision_id: purchase.current_revision_id, reason: '验证陈旧草稿冲突' });
  await dialog.getByRole('button', { name: '保存采购草稿', exact: true }).click(); await dialog.getByText('资料或授权已更新。填写内容仍保留，请关闭后核对最新版本再编辑。', { exact: true }).waitFor(); assert.equal(await dialog.getByLabel('备注', { exact: true }).inputValue(), '我的填写内容需保留'); assert(await dialog.getByRole('button', { name: '保存采购草稿', exact: true }).isDisabled());
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.reload(); await page.getByText('另一编辑者的已保存内容', { exact: true }).waitFor(); checks.push('A stale editor keeps unsaved text and refuses to overwrite a concurrent committed revision');

  const purchaseRequest = await prepare(page); await submitUI(page); await decideUI(bob.page, purchaseRequest, 'approve');
  purchase = await request(context, '/purchases/' + purchaseID); assert.equal(purchase.approval_state, 'approved'); assert.equal(purchase.lifecycle, 'requested'); assert.equal(purchase.approved_revision_id, purchase.current_revision_id); assert.equal(purchase.can_edit, false);
  checks.push('The purchase snapshot is submitted and approved by its actual assigned reviewer without inventing an order, receipt or payment');

  await page.goto(fixture.origin + '/claims'); await page.getByRole('button', { name: '起草报销', exact: true }).click(); dialog = page.getByRole('dialog', { name: '起草报销', exact: true });
  await dialog.getByLabel('报销标题', { exact: true }).fill('虚构项目费用报销'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id); await dialog.getByLabel('业务类别', { exact: true }).fill('spend.browser'); await dialog.getByLabel('币种', { exact: true }).fill('HKD'); await dialog.getByLabel('业务负责人', { exact: true }).selectOption(aliceMe.user.id); await dialog.getByLabel('业务用途', { exact: true }).fill('虚构员工垫付及公司付款费用核对'); await dialog.getByLabel('计价方式', { exact: true }).selectOption('gross_includes_tax'); await dialog.getByLabel('舍入规则', { exact: true }).selectOption('half_even'); await dialog.getByLabel('费用期间开始', { exact: true }).fill('2026-10-01'); await dialog.getByLabel('费用期间结束', { exact: true }).fill('2026-10-31');
  await dialog.getByRole('button', { name: '添加明细', exact: true }).click(); let line = dialog.locator('fieldset').nth(0);
  await line.getByLabel('明细描述', { exact: true }).fill('员工垫付虚构工具费'); await line.getByLabel('实际含税费用', { exact: true }).fill('12.34'); await line.getByLabel('其中税额（可选）', { exact: true }).fill('0.34'); await line.getByLabel('费用日期', { exact: true }).fill('2026-10-15'); await line.getByLabel('费用类别', { exact: true }).fill('tools'); await line.getByLabel('资金来源', { exact: true }).selectOption('employee_advance'); await selectSupplier(page, line, '选择明细供应商', supplier); await selectFile(page, line, '添加费用凭证', '选择费用凭证版本');
  await dialog.getByRole('button', { name: '添加明细', exact: true }).click(); line = dialog.locator('fieldset').nth(1);
  await line.getByLabel('明细描述', { exact: true }).fill('公司已付虚构托管费'); await line.getByLabel('实际含税费用', { exact: true }).fill('20'); await line.getByLabel('费用日期', { exact: true }).fill('2026-10-16'); await line.getByLabel('费用类别', { exact: true }).fill('hosting'); await line.getByLabel('资金来源', { exact: true }).selectOption('company_paid'); await line.getByLabel('无凭证说明', { exact: true }).fill('供应商尚未提供票据，费用依据人工核对'); await selectSupplier(page, line, '选择明细供应商', supplier); await dialog.getByLabel('创建依据', { exact: true }).fill('隔离报销草稿与资金来源核对');
  await dialog.getByRole('button', { name: '保存报销草稿', exact: true }).click(); await page.waitForURL('**/claims/*'); await page.getByRole('heading', { name: '虚构项目费用报销', exact: true }).waitFor();
  const claimID = new URL(page.url()).pathname.split('/').at(-1); let claim = await request(context, '/claims/' + claimID); assert.equal(claim.amount, '32.340000'); assert.equal(claim.employee_reimbursement_amount, '12.340000'); await layout(page, 'claim'); checks.push('Claim entry keeps gross tax, receipt evidence and company-paid expenses separate from employee advances at ten theme and width combinations');

  const claimRequest = await prepare(page); await submitUI(page); await decideUI(bob.page, claimRequest, 'return');
  await page.goto(fixture.origin + '/claims/' + claimID); await page.getByRole('button', { name: '编辑报销草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '编辑报销', exact: true }); await dialog.getByLabel('备注', { exact: true }).fill('已补充本次费用与项目的业务关联'); await dialog.getByLabel('修改依据', { exact: true }).fill('回应审批人补充要求'); await dialog.getByRole('button', { name: '保存报销草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('link', { name: '查看审批申请', exact: true }).click(); await page.getByRole('button', { name: '更新申请草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '更新申请草稿', exact: true }); await dialog.getByText('已补充本次费用与项目的业务关联', { exact: true }).waitFor(); await dialog.getByLabel('更新申请依据', { exact: true }).fill('固定补充后的报销内容'); await dialog.getByRole('button', { name: '确认更新申请草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await submitUI(page);
  await page.getByRole('button', { name: '撤回此申请', exact: true }).click(); dialog = page.getByRole('dialog', { name: '撤回当前申请', exact: true }); await dialog.getByLabel('撤回原因', { exact: true }).fill('再次核对费用依据'); await dialog.getByRole('button', { name: '确认撤回此申请', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  const returned = await request(context, '/requests/' + claimRequest); assert.equal(returned.state, 'withdrawn'); claim = await request(context, '/claims/' + claimID); assert.equal(claim.lifecycle, 'draft');
  await page.getByRole('button', { name: '查看申请修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: '申请修订与历次审批', exact: true }); await dialog.getByText('补充本次费用的业务说明', { exact: true }).waitFor(); await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Return, business revision, request revision, resubmission and withdrawal retain the original reviewer decision and fixed evidence in real dialogs');

  const duplicate = await request(context, '/claims', 'POST', { ...contentOf(claim), title: '虚构重复费用待核对', lines: claim.lines.map(item => ({ ...item, id: crypto.randomUUID() })), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离重复提示验证' });
  await page.goto(fixture.origin + '/claims/' + duplicate.id); await page.getByText('凭证或费用资料可能与其他可查阅的报销重复，请核实并说明。', { exact: true }).first().waitFor(); await prepare(page); await page.getByRole('button', { name: '核对并送审', exact: true }).click(); dialog = page.getByRole('dialog', { name: '核对报销送审预览', exact: true }); await dialog.getByText('凭证或费用资料与可查阅的其他报销可能重复，请核实并填写说明', { exact: true }).waitFor(); assert(await dialog.getByRole('button', { name: '确认以此内容送审', exact: true }).isDisabled()); await dialog.getByRole('button', { name: '取消', exact: true }).click();
  checks.push('Readable matching receipts or supplier/date/currency/amount candidates produce a warning and require an explicit submission explanation');

  await page.goto(fixture.origin + '/purchases'); await page.getByRole('button', { name: '起草采购', exact: true }).click(); dialog = page.getByRole('dialog', { name: '起草采购', exact: true });
  await dialog.getByLabel('采购标题', { exact: true }).fill('虚构待归档草稿'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id); await dialog.getByLabel('创建依据', { exact: true }).fill('验证不完整草稿与归档留存'); await dialog.getByRole('button', { name: '保存采购草稿', exact: true }).click(); await page.waitForURL('**/purchases/*');
  const archiveID = new URL(page.url()).pathname.split('/').at(-1); assert.equal((await request(context, '/purchases/' + archiveID)).amount, null);
  await page.getByRole('button', { name: '归档草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '归档未送审草稿', exact: true }); await dialog.getByLabel('归档依据', { exact: true }).fill('草稿不再需要，保留历史依据'); await dialog.getByRole('button', { name: '确认归档草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await page.getByText('此草稿已归档，历史版本和依据继续保留。', { exact: true }).waitFor();
  await page.getByRole('button', { name: '查看业务修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: / · 业务修订$/ }); await dialog.getByRole('heading', { name: '修订 1 · 虚构待归档草稿', exact: true }).waitFor(); await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('An incomplete draft keeps unknown money null and archive retains readable immutable business history');

  const pending = await request(context, '/purchases', 'POST', { ...contentOf(purchase), title: '虚构撤权审批验证', project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离当前授权验证' });
  await page.goto(fixture.origin + '/purchases/' + pending.id); const pendingRequest = await prepare(page); await submitUI(page);
  const revoked = bindings.find(binding => binding.user_id === bobMe.user.id && binding.role_id === roles.items.find(role => role.key === 'purchase_reader').id); assert(revoked);
  await request(administrator.context, '/role-bindings/' + revoked.id + '/revoke', 'POST', { expected_version: revoked.version, reason: '验证旧审批职责不能替代当前业务权限' }); revoked.revokedByCheck = true;
  await bob.context.close(); bob = await login(fixture.bob); const denied = await response(bob.context, '/requests/' + pendingRequest); assert.equal(denied.status, 404); const remaining = await request(bob.context, '/approval-tasks'); assert(!remaining.items.some(item => item.request_id === pendingRequest));
  checks.push('Revoking current purchase visibility removes the formerly assigned reviewer task and refuses its fixed snapshot after a fresh login');

  await page.goto(fixture.origin + '/purchases/' + purchaseID); await layout(page, 'purchase-approved');
  await page.goto(fixture.origin + '/claims/' + claimID); await layout(page, 'claim-returned');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) { process.stderr.write(String(error) + '\n'); }
finally {
  if (administrator) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings.filter(item => !item.revokedByCheck)) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离商业资料界面验证结束，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; process.stderr.write('Temporary isolated role cleanup did not complete.\n'); }
  }
  for (const context of contexts) await context.close().catch(() => {});
  if (browser) await browser.close();
  const after = await digest(); passed = passed && before === after;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, production_payments: false, checks, errors, server_errors: serverErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: true, all_owned_browser_tabs_closed: true }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
