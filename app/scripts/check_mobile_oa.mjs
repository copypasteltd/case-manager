// Actual Taro H5 + native PostgreSQL/API checks; never a native-device claim.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true); assert.equal(fixture.controlled_provider, true);
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/mobile-oa'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TEMP = temporary; process.env.TMP = temporary;
async function sourceFiles(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await sourceFiles(relative)); else result.push(relative); } return result; }
const apiSources = (await sourceFiles('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources = [...new Set([...await sourceFiles('mobile'), ...apiSources, 'package.json', 'package-lock.json', 'scripts/check_mobile_oa.mjs', 'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), started = new Date().toISOString(), contexts = [], checks = [], layouts = [], errors = [], serverErrors = [], failures = [], bindings = [], lastSteps = new Map();
let browser, administrator, reviewPage, passed = false, cleanedBindings = false, phase = 'starting browser', expectedAborts = 0, failureNotices = [];
function passedCheck(name) { checks.push({ name, passed: true }); process.stdout.write(JSON.stringify({ completed_check: name }) + '\n'); }
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, name) => screen(page).getByRole('textbox', { name, exact: true });
async function stable(page) { await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); }); }
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); await stable(page); }
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '')].map(c => alphabet.indexOf(c).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(v => parseInt(v, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function code(account) { const previous = lastSteps.get(account.login) ?? account.last_step; if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 200)); const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); return otp(account.mfa_secret, step); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 390, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  page.on('requestfailed', request => failures.push({ method: request.method(), path: new URL(request.url()).pathname, error: request.failure()?.errorText }));
  await page.goto(fixture.origin); await page.locator('input[placeholder="你的公司账号"]').fill(account.login); await page.locator('input[type="password"]').fill(account.password);
  const [response] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST'), page.getByRole('button', { name: '登录工作空间 ↗', exact: true }).click()]); let result = await response.json();
  if (account.mfa_secret) {
    await page.locator('input[maxlength="6"]').fill(await code(account));
    const [verified] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/v1/auth/mfa/verify') && r.request().method() === 'POST'), page.getByRole('button', { name: '验证并继续 ↗', exact: true }).click()]); result = await verified.json();
  }
  assert.equal(result.data?.step, 'authenticated'); await heading(page, '工作从这里开始');
  assert.equal(await page.evaluate(() => Object.keys(localStorage).some(key => /access|refresh|session|password|draft|credential/i.test(key))), false);
  return { page, context, token: result.data.access_token };
}
async function request(token, route, method = 'GET', body) {
  const response = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'mobile-oa-fixture:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  const result = await response.json(); assert.ok(response.status < 400, route + ' / ' + response.status + ' / ' + (result.error?.code ?? '')); return result.data;
}
async function command(page, route, label, status = 200, method = 'POST') {
  const [response] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route && r.request().method() === method), button(page, label).click()]);
  const result = await response.json(); assert.equal(response.status(), status, route + ' / ' + (result.error?.code ?? '')); return result.data;
}
async function select(page, label, text) {
  const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]'); await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  // The panel animates in from outside the viewport. Wait for its actual
  // hydration and animation before measuring and using the real mouse wheel.
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(animation => animation.finished.catch(() => {}))); });
  await stable(page); const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text); assert.ok(desired >= 0, label + ' missing ' + text);
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0, box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); for (let step = 1; step <= 8; step++) { await page.mouse.move(x, y - (desired - current) * item.height * step / 8); await stable(page); } await page.mouse.up(); await stable(page); }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stable(page); assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired), label + ' did not select ' + text);
}
async function home(page) { for (let count = 0; count < 10; count++) { if (page.url().includes('/pages/home/index') || await screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).count()) { await heading(page, '工作从这里开始'); return; } const before = page.url(); await Promise.all([page.waitForURL(url => url.href !== before), page.goBack()]); await stable(page); } throw new Error('Cannot return through actual Taro navigation'); }
async function openList(page, name) { await home(page); await screen(page).locator('.cp-row-title').getByText(name, { exact: true }).click(); await heading(page, name); }
async function scope(page, project) { await button(page, '选择关联项目').click(); await screen(page).locator('.cp-row-title').getByText(project.name, { exact: true }).click(); await select(page, '资料分类', '项目受限'); }
async function party(page, label, name) { await button(page, label).click(); await field(page, '查找候选资料').fill(name); await button(page, '查找候选资料').click(); await screen(page).locator('.cp-row-title').getByText(name, { exact: true }).click(); }
async function fixedFile(page, label) { await button(page, label).click(); await field(page, '查找候选资料').fill(fixture.file_name); await button(page, '查找候选资料').click(); await screen(page).locator('.cp-row-title').getByText(fixture.file_name, { exact: true }).click(); await button(page, '使用固定版本 ' + fixture.file_seq).click(); }
async function layout(page, label, reopen, target) {
  for (const [theme, name] of [['light', '浅色'], ['dark', '深色']]) {
    const title = await screen(page).locator('.cp-title').textContent(); await button(page, '我的').click(); await heading(page, '我的工作空间'); await button(page, name).click(); await button(page, '返回上一页').click(); await heading(page, title); if (reopen) await reopen();
    for (const [width, height] of [[320, 900], [375, 900], [390, 900], [768, 900], [844, 390]]) {
      await page.setViewportSize({ width, height }); await stable(page);
      const info = await screen(page).evaluate(panel => ({ width: panel.getBoundingClientRect().width, overflow: document.documentElement.scrollWidth > innerWidth + 1, theme: panel.className, font: getComputedStyle(panel).fontFamily, small: [...panel.querySelectorAll('.cp-button,.cp-link,.cp-input')].filter(node => node.getBoundingClientRect().height > 0 && node.getBoundingClientRect().height < 43.5).map(node => node.textContent), faded: [...panel.querySelectorAll('.cp-button[aria-disabled="false"]')].filter(node => getComputedStyle(node).opacity !== '1').map(node => node.textContent) }));
      assert.ok(Math.abs(info.width - width) < 2); assert.equal(info.overflow, false, label + '/' + theme + '/' + width); assert.match(info.theme, new RegExp('cp-theme-' + theme)); assert.match(info.font, /Manrope/); assert.deepEqual(info.small, []); assert.deepEqual(info.faded, []);
      if (width === 390) { await (target ? screen(page).getByText(target, { exact: true }) : screen(page).locator('.cp-heading')).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, label + '-' + theme + '-390.png'), fullPage: true }); }
    }
  }
  await page.setViewportSize({ width: 390, height: 900 }); layouts.push({ page: label, themes: ['light', 'dark'], viewports: ['320x900', '375x900', '390x900', '768x900', '844x390'], configurations: 10, passed: true });
}
async function prepare(page, business) { await button(page, '建立申请草稿').click(); await field(page, '准备申请依据').fill('明确固定当前业务修订，供独立人员核对。'); const value = await command(page, '/requests', '确认建立申请草稿', 201); assert.equal(value.business_id, business.id); await page.waitForURL(url => url.href.includes('/pages/detail/index') && url.href.includes('id=' + value.id)); await heading(page, business.title); await button(page, '核对并送审').waitFor(); return value; }
async function submit(page, value) {
  await button(page, '核对并送审').click(); await screen(page).getByText('本次将固定的审批顺序', { exact: true }).waitFor();
  const box = screen(page).getByRole('checkbox', { name: '我已核对以上固定内容、附件与审批顺序', exact: true }); assert.equal(await box.isChecked(), false);
  await field(page, '送审依据').fill('本人核对完整原币、固定文件、往来方及明确审批顺序。'); await box.check();
  const result = await command(page, '/requests/' + value.id + '/submit', '确认以此内容送审'); assert.equal(result.state, 'in_review'); await button(page, '撤回此申请').waitFor(); return result;
}
async function decide(page, title, result) {
  await home(page); await screen(page).locator('.cp-row-title').getByText('待我审批', { exact: true }).click(); await screen(page).locator('.cp-row-title').getByText(title, { exact: true }).click(); await heading(page, title);
  await field(page, '变更或决定原因').fill('独立核对当前固定业务修订及附件原件。'); const value = await command(page, '/approval-steps/' + result.steps[0].id + '/decisions', '确认同意当前修订'); assert.equal(value.state, 'approved'); return value;
}

try {
  browser = await chromium.launch({ executablePath, headless: true }); phase = 'real accounts and isolated API setup';
  let alice = await login(fixture.alice), bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.token, '/me'), reviewer = await request(bob.token, '/me'), project = await request(alice.token, '/projects/' + fixture.project_id), roles = await request(administrator.token, '/roles?limit=100');
  for (const [user, key] of [[me.user.id, 'purchase_manager'], [me.user.id, 'claim_submitter'], [reviewer.user.id, 'purchase_reader'], [reviewer.user.id, 'claim_reader']]) {
    const role = roles.items.find(item => item.key === key); assert.ok(role);
    bindings.push(await request(administrator.token, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'project', scope_id: project.id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '仅为隔离移动OA检查临时授权' }));
  }
  for (const kind of ['purchase', 'claim']) await request(administrator.token, '/workflow-definitions', 'POST', { expected_version: 0, key: 'mobile.oa.' + kind, name: '隔离移动' + (kind === 'purchase' ? '采购' : '报销') + '制度', kind, category: 'mobile.oa', currency: 'HKD', min_amount: '0', max_amount: null, enabled: true, self_approval_policy: 'deny', withdrawal_allowed: true, steps: [{ title: '独立业务核对', assigned_user_id: reviewer.user.id, requires_mfa: false }], reason: '明确准备隔离移动OA审批依据' });
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob); reviewPage = alice.page;
  const supplier = await request(alice.token, '/parties', 'POST', { name: '移动OA受控供应商', kind: 'organization', relationships: ['supplier'], project_id: project.id, classification: 'project_restricted', reason: '仅作隔离往来方选择候选' });
  passedCheck('Three real mobile-channel accounts; temporary roles, workflows and supplier explicitly seeded through actual APIs');
  const page = alice.page;
  phase = 'incomplete contract, validation and authoritative server field errors';
  await openList(page, '合同'); await button(page, '起草合同').click(); await heading(page, '起草合同');
  await button(page, '保存合同草稿').click(); await screen(page).getByText('有 2 处需要补充或修正', { exact: true }).waitFor(); assert.equal(await field(page, '合同标题').evaluate(node => node === document.activeElement), true);
  await field(page, '合同标题').fill('移动端完整合同'); await scope(page, project); await field(page, '创建依据').fill('明确保存不完整业务草稿。');
  await layout(page, 'contract-draft');
  const incomplete = await command(page, '/contracts', '保存合同草稿', 201); assert.equal(incomplete.amount, null); assert.equal(incomplete.currency, null); assert.equal(incomplete.owner_id, me.user.id);
  await heading(page, incomplete.title); await button(page, '编辑合同草稿').click(); await field(page, '合同币种').fill('ZZZ'); await field(page, '本次修改依据').fill('核对服务器币种目录。');
  await command(page, '/contracts/' + incomplete.id, '保存为新的业务修订', 422, 'PATCH'); await screen(page).getByText('该币种不在支持的精度目录中', { exact: true }).last().waitFor(); assert.equal(await field(page, '合同币种').inputValue(), 'ZZZ'); assert.equal(await field(page, '合同币种').getAttribute('aria-invalid'), 'true');
  await field(page, '合同币种').fill('HKD'); await select(page, '收付方向', '付款合同'); await field(page, '合同类型').fill('service'); await field(page, '业务类别').fill('execution'); await field(page, '合同总额').fill('12.34'); await field(page, '其中税额').fill('0.34'); await field(page, '起始日期').fill(fixture.today); await field(page, '结束日期').fill('2030-12-31');
  await select(page, '计划情况', '按节点收款或付款'); await field(page, '节点 1 名称').fill('独立核对后的付款'); await field(page, '节点 1 金额').fill('12.34'); await field(page, '节点 1 到期日').fill('2030-12-31');
  await party(page, '选择往来方修订', supplier.name); await fixedFile(page, '选择合同正文版本'); await field(page, '本次修改依据').fill('补齐准确往来方、正文版本、原币与付款计划。');
  const contract = await command(page, '/contracts/' + incomplete.id, '保存为新的业务修订', 200, 'PATCH'); assert.equal(contract.amount, '12.340000'); assert.equal(contract.counterparty_revision_id, supplier.current_revision_id); assert.equal(contract.draft_file_version_id, fixture.file_version_id);
  passedCheck('Real UI creates an incomplete contract, focuses local errors, preserves server-rejected currency and saves exact money/party/body/payment-plan revision');
  phase = 'contract fixed request, preview, explicit submit and independent approval';
  let approval = await prepare(page, contract); await layout(page, 'contract-preview', async () => { await button(page, '核对并送审').click(); await screen(page).getByText('本次将固定的审批顺序', { exact: true }).waitFor(); }, '本次将固定的审批顺序');
  await button(page, '返回申请详情').click(); approval = await submit(page, approval); await decide(bob.page, contract.title, approval);
  assert.equal((await request(alice.token, '/contracts/' + contract.id)).lifecycle, 'pending_signature', 'Commercial approval reaches the actual awaiting-signature stage, without fabricating signature or activation');
  passedCheck('Contract fixed request and workflow preview are confirmed through UI, independently approved, without fabricating signature or activation');

  phase = 'purchase UI with line-stable calculations, supplier and fixed quote';
  await openList(page, '采购'); await button(page, '起草采购').click(); await field(page, '采购标题').fill('移动端完整采购'); await scope(page, project); await field(page, '业务类别').fill('mobile.oa'); await field(page, '币种').fill('HKD'); await field(page, '业务用途').fill('明确的小团队项目工具采购。');
  await select(page, '计价方式', '单价未含税，税额另加'); await select(page, '舍入规则', '四舍五入'); await field(page, '预计交付日期').fill('2030-12-31'); await layout(page, 'purchase-draft'); await party(page, '添加候选供应商', supplier.name); await fixedFile(page, '添加报价版本');
  await button(page, '添加明细').click(); await field(page, '明细 1 描述').fill('固定计算的项目工具'); await field(page, '明细 1 数量').fill('3'); await field(page, '明细 1 单位').fill('份'); await field(page, '明细 1 单价').fill('10.005'); await field(page, '明细 1 折扣金额').fill('0.01'); await field(page, '创建依据').fill('核对数量、精确单价、折扣与当前报价。');
  const purchase = await command(page, '/purchases', '保存采购草稿', 201); assert.equal(purchase.amount, '30.010000'); assert.equal(purchase.lines[0].unit_price, '10.005000'); assert.deepEqual(purchase.quote_version_ids, [fixture.file_version_id]);
  await heading(page, purchase.title); let purchaseRequest = await prepare(page, purchase); purchaseRequest = await submit(page, purchaseRequest); await decide(bob.page, purchase.title, purchaseRequest);
  passedCheck('Purchase is created and submitted entirely through UI, server calculates 3×10.005 minus 0.01 as HKD30.01, fixed quote and independent approval preserved');

  phase = 'claim UI, mixed funding and receipt/missing-evidence explanations';
  await openList(page, '报销'); await button(page, '起草报销').click(); await field(page, '报销标题').fill('移动端完整报销'); await scope(page, project); await field(page, '业务类别').fill('mobile.oa'); await field(page, '币种').fill('HKD'); await field(page, '业务用途').fill('香港项目工作费用，分别标明员工垫付和公司已付。');
  await select(page, '计价方式', '金额已含税，税额作说明'); await select(page, '舍入规则', '五成双'); await field(page, '费用期间开始').fill(fixture.today); await field(page, '费用期间结束').fill(fixture.today); await layout(page, 'claim-draft');
  await button(page, '添加明细').click(); await field(page, '明细 1 描述').fill('员工垫付工作材料'); await field(page, '明细 1 实际含税费用').fill('10.50'); await field(page, '明细 1 费用日期').fill(fixture.today); await field(page, '明细 1 费用类别').fill('materials'); await select(page, '明细 1 资金来源', '员工垫付'); await party(page, '选择本行供应商', supplier.name); await fixedFile(page, '添加本行费用凭证');
  await button(page, '添加明细').click(); await field(page, '明细 2 描述').fill('公司已付交通'); await field(page, '明细 2 实际含税费用').fill('5.20'); await field(page, '明细 2 费用日期').fill(fixture.today); await field(page, '明细 2 费用类别').fill('travel'); await select(page, '明细 2 资金来源', '公司已付'); await party(page, '选择本行供应商', supplier.name); await field(page, '明细 2 无凭证说明').fill('受控测试中的确切无凭证说明，须由独立人员核对。'); await field(page, '创建依据').fill('分别登记费用合计与实际员工垫付部分。');
  const claim = await command(page, '/claims', '保存报销草稿', 201); assert.equal(claim.amount, '15.700000'); assert.equal(claim.employee_reimbursement_amount, '10.500000'); assert.equal(claim.lines.length, 2); assert.deepEqual(claim.lines[0].receipt_version_ids, [fixture.file_version_id]);
  await heading(page, claim.title); await screen(page).getByText('尚未送审', { exact: true }).waitFor(); await layout(page, 'claim-current'); let claimRequest = await prepare(page, claim); claimRequest = await submit(page, claimRequest);
  passedCheck('Claim is created from real two-line UI with fixed receipt and explicit missing-receipt explanation; employee/company funding totals remain distinct');

  phase = 'withdraw, revise, explicit re-preview, preserved history and independent approval';
  await button(page, '撤回此申请').click(); await field(page, '撤回原因').fill('明确撤回并补充业务用途后重新核对。'); await screen(page).getByRole('checkbox', { name: '我确认撤回当前固定修订的申请', exact: true }).check(); await command(page, '/requests/' + claimRequest.id + '/withdraw', '确认撤回此申请');
  await button(page, '查看当前业务资料').click(); await heading(page, claim.title); await button(page, '编辑报销草稿').click(); await field(page, '业务用途').fill('已经明确补充的香港项目费用用途。'); await field(page, '本次修改依据').fill('明确补充用途，不改变原员工垫付与公司已付。'); const amended = await command(page, '/claims/' + claim.id, '保存为新的业务修订', 200, 'PATCH');
  await button(page, '查看关联申请与进展').click(); await heading(page, claim.title); await button(page, '更新申请草稿').click(); await field(page, '更新申请依据').fill('采用刚刚核对的完整业务修订。'); await screen(page).getByRole('checkbox', { name: '我已核对本次将采用的完整业务修订', exact: true }).check(); claimRequest = await command(page, '/requests/' + claimRequest.id + '/revisions', '确认更新申请草稿'); assert.equal(claimRequest.payload.business_revision_id, amended.current_revision_id);
  claimRequest = await submit(page, claimRequest); await decide(bob.page, claim.title, claimRequest); await button(page, '查看申请修订历史').click(); await screen(page).getByRole('button', { name: /^申请修订 1 ·/ }).click(); await screen(page).getByText('香港项目工作费用，分别标明员工垫付和公司已付。', { exact: true }).waitFor(); await layout(page, 'claim-history', async () => { await button(page, '查看申请修订历史').click(); await screen(page).getByRole('button', { name: /^申请修订 1 ·/ }).click(); }, '以下是该修订当时固定的内容与决定，不代表当前申请状态。');
  passedCheck('Real withdrawal, business amendment, explicit new request snapshot, re-preview, resubmission, independent approval and immutable first-revision history');

  phase = 'concurrent edit preserves human input and rejects stale save';
  await openList(page, '合同'); await button(page, '起草合同').click(); await field(page, '合同标题').fill('移动端冲突保留合同'); await scope(page, project); await field(page, '创建依据').fill('隔离双编辑并发验证。'); const conflict = await command(page, '/contracts', '保存合同草稿', 201);
  await heading(page, conflict.title); await button(page, '编辑合同草稿').click(); await field(page, '补充说明').fill('本机尚未提交的重要文字。'); await field(page, '本次修改依据').fill('本机原版本修改依据。');
  const { payment_schedule, title, direction, type, category, counterparty_id, counterparty_revision_id, owner_id, currency, amount, tax_amount, start_date, end_date, renewal_terms, termination_terms, notes, draft_file_version_id } = conflict;
  await request(alice.token, '/contracts/' + conflict.id, 'PATCH', { payment_schedule, title, direction, type, category, counterparty_id, counterparty_revision_id, owner_id, currency, amount, tax_amount, start_date, end_date, renewal_terms, termination_terms, notes: notes + '另一会话已保存', draft_file_version_id, expected_version: conflict.version, base_revision_id: conflict.current_revision_id, reason: '隔离另一真实API会话的先行保存' });
  await command(page, '/contracts/' + conflict.id, '保存为新的业务修订', 409, 'PATCH'); assert.equal(await field(page, '补充说明').inputValue(), '本机尚未提交的重要文字。'); assert.equal(await button(page, '保存为新的业务修订').getAttribute('aria-disabled'), 'true');
  passedCheck('A competing real API edit yields genuine409; stale form preserves human notes and stops stale resubmission');

  phase = 'network ambiguity after actual server commit; no automatic or duplicate create';
  await openList(page, '报销'); await button(page, '起草报销').click(); await field(page, '报销标题').fill('移动端结果未知单次草稿'); await scope(page, project); await field(page, '创建依据').fill('真实服务器提交后主动中断回包，核对一次性创建。');
  let created, posts = 0; const owned = fixture.api_origin + '/v1/claims'; const watcher = req => { if (req.url() === owned && req.method() === 'POST') posts++; }; page.on('request', watcher);
  await page.route(owned, async route => { if (route.request().method() !== 'POST') return route.continue(); const response = await route.fetch(); assert.equal(response.status(), 201); created = (await response.json()).data; expectedAborts++; await route.abort('failed'); });
  await button(page, '保存报销草稿').click(); await screen(page).getByText('连接中断，操作结果尚未确认。请先刷新资料核对。', { exact: true }).waitFor(); await page.unroute(owned); assert.ok(created); assert.equal(posts, 1);
  await button(page, '我的').click(); await heading(page, '我的工作空间'); await button(page, '返回上一页').click(); await heading(page, '起草报销'); assert.equal(await field(page, '报销标题').inputValue(), '移动端结果未知单次草稿'); assert.equal(posts, 1);
  const reconciled = await command(page, '/claims', '保存报销草稿', 201); assert.equal(reconciled.id, created.id); await heading(page, created.title);
  await openList(page, '报销'); await field(page, '查找资料').fill('移动端结果未知单次草稿'); await button(page, '查找 / 刷新').click(); await screen(page).locator('.cp-row-title').getByText('移动端结果未知单次草稿', { exact: true }).waitFor();
  const found = await request(alice.token, '/claims?limit=100&q=' + encodeURIComponent('移动端结果未知单次草稿')); assert.equal(found.items.filter(item => item.title === '移动端结果未知单次草稿').length, 1); assert.equal(posts, 2, 'Explicit reconciliation reuses the original idempotency key instead of creating a new row'); page.off('request', watcher);
  passedCheck('A deliberate wire abort after actual commit preserves input, performs no automatic write, and explicit same-intent reconciliation resolves to one real draft');

  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); assert.equal(failures.length, expectedAborts); assert.equal(before, await digest()); passed = true;
} catch (error) {
  checks.push({ name: phase, passed: false, error: error.message });
  if (reviewPage) try { failureNotices = (await screen(reviewPage).locator('.cp-notice').allTextContents()).slice(0, 20); } catch { /* diagnostics cannot interrupt cleanup */ }
  if (reviewPage) try { await reviewPage.screenshot({ path: path.join(evidence, 'failed-attempt.png'), fullPage: true }); } catch { /* finally still closes all owned contexts */ }
  process.stderr.write(phase + ': ' + error.message + '\n');
} finally {
  if (administrator && bindings.length) {
    try { await request(administrator.token, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.token, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离移动OA检查结束，撤销临时授权' }); cleanedBindings = true; } catch (error) { checks.push({ name: 'Revoke owned temporary bindings', passed: false, error: error.message }); passed = false; }
  }
  for (const context of contexts) await context.close(); if (browser) await browser.close();
  const closed = (!browser || !browser.isConnected()) && contexts.every(context => context.pages().length === 0);
  const after = await digest(), result = { started_at: started, completed_at: new Date().toISOString(), passed: passed && before === after && cleanedBindings && closed, scope: 'Actual Taro H5 UI and native API/PostgreSQL; both mini-program and Android native runtime remain separate', setup_via_api: ['temporary role bindings', 'purchase/claim workflow policies', 'supplier candidate', 'one competing edit'], source_sha256: before, source_sha256_after: after, source_files: sources.length, sources, source_unchanged_during_run: before === after, checks, layouts, failure_notices: failureNotices, javascript_errors: errors, unexpected_server_errors: serverErrors, expected_network_aborts: expectedAborts, network_failures: failures, owned_temporary_bindings_revoked: cleanedBindings, owned_contexts_closed: contexts.length, all_browser_contexts_closed: closed, all_browser_tabs_closed: closed, all_owned_browser_tabs_closed: closed, actual_cloud_invoked: false, actual_mini_program_runtime_invoked: false, actual_android_runtime_invoked: false };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); process.stdout.write(JSON.stringify({ passed: result.passed, checks: checks.length, layouts: layouts.length, closed_contexts: contexts.length }) + '\n');
  if (!result.passed) process.exitCode = 1;
}
