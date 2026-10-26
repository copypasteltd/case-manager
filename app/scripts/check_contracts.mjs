// Real API/UI checks using a synthetic project and a temporary scoped role.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { waitForAPI } from './wait_for_api.mjs';

const run = promisify(execFile), app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const config = JSON.parse(await readFile(path.join(app, '.local/environment.json'), 'utf8'));
assert.equal(config.CP_ENV, 'development'); assert.equal(new URL(config.CP_PUBLIC_ORIGIN).hostname, 'localhost');
await waitForAPI(config);
const accounts = JSON.parse(await readFile(path.join(app, '.local/development-accounts.json'), 'utf8')).accounts;
const mfaFile = path.join(app, '.local/development-mfa.json'), mfa = JSON.parse(await readFile(mfaFile, 'utf8'));
assert(mfa.secret, 'The owned existing administrator MFA record is required; do not reseed accounts');
const manager = accounts.find(user => user.login === 'project.manager'), reader = accounts.find(user => user.login === 'project.member'), admin = accounts.find(user => user.login === 'system.admin');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/contracts'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true });
process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], checks = [], errors = [], serverErrors = [];
let browser, passed = false; const publications = [];
function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2)));
  const counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest();
  return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function nextCode() {
  if (Math.floor(Date.now() / 30_000) <= mfa.last_step) await new Promise(resolve => setTimeout(resolve, (mfa.last_step + 1) * 30_000 - Date.now() + 300));
  const step = Math.floor(Date.now() / 30_000); mfa.last_step = step; await writeFile(mfaFile, JSON.stringify(mfa, null, 2) + '\n'); return otp(mfa.secret, step);
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25_000);
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/v1/workflow-definitions') publications.push(request.url()); });
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(config.CP_PUBLIC_ORIGIN);
  await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录工作空间' }).click();
  if (account === admin) {
    await page.getByLabel('六位验证码').waitFor(); await page.getByLabel('六位验证码').fill(await nextCode());
    await page.getByRole('button', { name: '验证并继续' }).click();
  }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  return { context, page };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(value => value.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(config.CP_PUBLIC_ORIGIN + '/v1' + route, { method, headers: {
    Origin: config.CP_PUBLIC_ORIGIN, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID(),
  }, ...(body ? { data: body } : {}) });
  const result = await response.json();
  assert(response.ok(), 'Fixture API request failed: ' + route + ' ' + response.status() + ' ' + result.error?.code);
  return result.data;
}


let administrator, contractID, projectID, passedCleanup = false; const bindings = [];
try {
  browser = await chromium.launch({ executablePath, headless: true });
  let owner = await login(manager); administrator = await login(admin);
  const me = await request(owner.context, '/me'), stamp = Date.now();
  const project = await request(owner.context, '/projects', 'POST', { name: '合同验证项目 ' + stamp, scope: '合同页面验证，全部合同与往来资料均为虚构样本。', owner_id: me.user.id, member_ids: [], classification: 'project_restricted' }); projectID = project.id;
  const roles = await request(administrator.context, '/roles?limit=100');
  for (const key of ['contract_manager', 'party_manager']) {
    const role = roles.items.find(item => item.key === key); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: me.user.id, role_id: role.id, scope_type: 'project', scope_id: project.id, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3_600_000).toISOString(), reason: '本地合同界面验证，仅限此虚构项目，结束即撤销。' }));
  }
  await owner.context.close(); owner = await login(manager);
  const { page, context } = owner, title = '界面验证合同 ' + stamp;
  let party = await request(context, '/parties', 'POST', { name: '虚构合同供应方 ' + stamp, kind: 'organization', relationships: ['supplier'], contact_name: '', email: 'contract@example.invalid', phone: '', notes: '虚构样本', project_id: project.id, classification: 'project_restricted', reason: '合同选择器验证。' });
  const originalPartyName = party.name, originalPartyRevision = party.current_revision_id;
  await page.getByRole('link', { name: '合同', exact: true }).click(); await page.getByRole('button', { name: '起草合同', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '起草合同', exact: true });
  await dialog.getByLabel('合同标题', { exact: true }).fill(title); await dialog.getByLabel('所属项目', { exact: true }).selectOption(project.id);
  await dialog.getByLabel('创建依据', { exact: true }).fill('虚构合同草稿，未知金额留空。'); await dialog.getByRole('button', { name: '保存合同草稿', exact: true }).click();
  await page.getByRole('heading', { name: title, exact: true }).waitFor(); contractID = new URL(page.url()).pathname.split('/').at(-1);
  assert.equal(await page.locator('.breadcrumb strong').innerText(), '合同');
  let current = await request(context, '/contracts/' + contractID); const originalRevision = current.current_revision_id;
  assert.equal(current.amount, null); assert.equal(current.currency, null); assert.equal(current.owner_id, null); assert.equal(current.approval_state, 'not_submitted'); assert.equal(current.lifecycle, 'draft');
  checks.push('UI creates an incomplete real draft with unknown amount, currency and owner remaining null and approval separate from lifecycle');
  await page.getByRole('button', { name: '编辑草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '编辑合同草稿', exact: true });
  await dialog.getByRole('button', { name: '选择往来方', exact: true }).click();
  let picker = page.getByRole('dialog', { name: '选择往来方资料', exact: true });
  await picker.getByRole('button', { name: new RegExp(originalPartyName) }).click();
  await dialog.getByText(originalPartyName, { exact: true }).waitFor();
  await dialog.getByRole('button', { name: '选择正文版本', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择合同正文版本', exact: true });
  await picker.getByRole('heading', { name: '未找到可选文件', exact: true }).waitFor();
  await picker.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  await dialog.getByLabel('收付方向', { exact: true }).selectOption('payable'); await dialog.getByLabel('合同类型', { exact: true }).fill('服务合同'); await dialog.getByLabel('业务类别', { exact: true }).fill('software');
  await dialog.getByLabel('合同币种', { exact: true }).selectOption('HKD'); await dialog.getByLabel('合同总额', { exact: true }).fill('12.34'); await dialog.getByLabel('其中税额', { exact: true }).fill('0');
  await dialog.getByLabel('起始日期', { exact: true }).fill('2026-11-01'); await dialog.getByLabel('结束日期', { exact: true }).fill('2027-10-31');
  await dialog.getByLabel('续约条款', { exact: true }).fill('虚构条款：到期前由双方另行确认。'); await dialog.getByLabel('本次修改依据', { exact: true }).fill('补充条款与固定往来方依据。');
  await page.setViewportSize({ width: 390, height: 1050 }); await page.screenshot({ path: path.join(evidence, 'editor-light-390.png'), fullPage: true });
  await dialog.getByRole('button', { name: '保存为新修订', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  current = await request(context, '/contracts/' + contractID); assert.equal(current.amount, '12.340000'); assert.equal(current.tax_amount, '0.000000'); assert.equal(current.counterparty_revision_id, originalPartyRevision); assert.notEqual(current.current_revision_id, originalRevision);
  party = await request(context, '/parties/' + party.id, 'PATCH', { expected_version: party.version, base_revision_id: party.current_revision_id, name: party.name + '（后续更名）', kind: party.kind, relationships: party.relationships, contact_name: party.contact_name, email: party.email, phone: party.phone, notes: party.notes, reason: '验证固定合同引用不随主资料更名。' });
  await page.reload(); await page.getByText(originalPartyName, { exact: true }).waitFor(); assert.equal((await request(context, '/contracts/' + contractID)).counterparty.name, originalPartyName);
  checks.push('The real party selector fixes its revision; a later party rename preserves the contract evidence, and an empty file selector preserves the form without inventing an attachment');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 }); assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), theme + ' ' + width + ' detail overflow');
      if (theme === 'dark' && width === 1440) await page.screenshot({ path: path.join(evidence, 'detail-dark-1440.png'), fullPage: true });
    }
  }
  await page.getByRole('button', { name: '查看修订记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: current.contract_no + ' · 修订记录', exact: true });
  await dialog.getByRole('heading', { name: '修订 1 · ' + title, exact: true }).waitFor(); await dialog.getByRole('heading', { name: '修订 2 · ' + title, exact: true }).waitFor();
  await page.setViewportSize({ width: 390, height: 1050 }); await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Contract detail fits both themes and five widths, and the history dialog retains the first incomplete draft alongside the updated revision');
  await page.getByRole('button', { name: '编辑草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '编辑合同草稿', exact: true });
  await dialog.getByLabel('续约条款', { exact: true }).fill('本机未保存的条款。'); await dialog.getByLabel('本次修改依据', { exact: true }).fill('模拟其他设备先保存。');
  const fields = ['title', 'direction', 'type', 'category', 'counterparty_id', 'counterparty_revision_id', 'owner_id', 'currency', 'amount', 'tax_amount', 'start_date', 'end_date', 'renewal_terms', 'termination_terms', 'notes', 'draft_file_version_id', 'payment_schedule'];
  await request(context, '/contracts/' + contractID, 'PATCH', { ...Object.fromEntries(fields.map(field => [field, current[field]])), expected_version: current.version, base_revision_id: current.current_revision_id, notes: '另一个实际请求先更新。', reason: '并发界面验证。' });
  await dialog.getByRole('button', { name: '保存为新修订', exact: true }).click(); await dialog.getByText('合同或授权已变化。已保留填写内容，请核对最新合同后重新编辑。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('续约条款', { exact: true }).inputValue(), '本机未保存的条款。'); await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.reload();
  checks.push('A second real API write causes a stale form conflict and preserves the unsaved contract terms');
  await page.getByRole('button', { name: '起草补充协议', exact: true }).click(); dialog = page.getByRole('dialog', { name: '起草补充协议', exact: true });
  const childTitle = title + ' · 补充协议'; await dialog.getByLabel('合同标题', { exact: true }).fill(childTitle); await dialog.getByLabel('创建依据', { exact: true }).fill('继承主合同项目范围。'); await dialog.getByRole('button', { name: '保存合同草稿', exact: true }).click();
  await page.getByRole('heading', { name: childTitle, exact: true }).waitFor(); const childID = new URL(page.url()).pathname.split('/').at(-1), child = await request(context, '/contracts/' + childID);
  assert.equal(child.parent_contract_id, contractID); assert.equal(child.project_id, projectID); assert.equal(child.classification, 'project_restricted');
  await page.getByRole('button', { name: '回收此草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '回收合同草稿', exact: true });
  await dialog.getByLabel('回收原因', { exact: true }).fill('先处理补充协议。'); await dialog.getByRole('button', { name: '确认回收草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.goto(config.CP_PUBLIC_ORIGIN + '/contracts/' + contractID); await page.getByRole('heading', { name: title, exact: true }).waitFor();
  current = await request(context, '/contracts/' + contractID); const preserved = current.current_revision_id;
  await page.getByRole('button', { name: '回收此草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '回收合同草稿', exact: true });
  await dialog.getByLabel('回收原因', { exact: true }).fill('验证草稿回收而非删除。'); await dialog.getByRole('button', { name: '确认回收草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  await page.getByText('此草稿已回收，全部修订与引用依据仍保留。', { exact: true }).waitFor(); assert.equal(await page.getByRole('button', { name: '编辑草稿', exact: true }).count(), 0);
  await page.getByRole('button', { name: '恢复草稿', exact: true }).click(); dialog = page.getByRole('dialog', { name: '恢复合同草稿', exact: true });
  await dialog.getByLabel('恢复原因', { exact: true }).fill('结束草稿回收验证。'); await dialog.getByRole('button', { name: '确认恢复草稿', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  current = await request(context, '/contracts/' + contractID); assert.equal(current.deleted_at, null); assert.equal(current.current_revision_id, preserved);
  checks.push('A real supplement inherits the parent scope; ordered UI trash and restore preserve draft revisions');
  assert.equal((await administrator.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/contracts/' + contractID)).status(), 404);
  const ordinary = await login(reader); await ordinary.page.goto(config.CP_PUBLIC_ORIGIN + '/contracts'); await ordinary.page.getByRole('heading', { name: '没有合同查阅权限', exact: true }).waitFor(); assert.equal((await ordinary.context.request.get(config.CP_PUBLIC_ORIGIN + '/v1/contracts/' + contractID)).status(), 404);
  checks.push('Configuration administration and an unrelated employee cannot access private contract content');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  passedCleanup = bindings.length === 2;
  if (administrator) for (const binding of bindings) { try { await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '本地合同界面验证结束，撤销临时项目授权。' }); } catch (error) { errors.push(error.message); passedCleanup = false; passed = false; } }
  let allBrowserContextsClosed = true;
  for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const result = { passed, executed_at: new Date().toISOString(), real_api: true, synthetic_contract_data: true, real_cloud_file_evidence: false, fixed_formal_file_references_tested_in_isolated_pg: true, temporary_bindings_revoked: passedCleanup, checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2)); if (!passed || !passedCleanup || !allBrowserContextsClosed) process.exitCode = 1;
}
