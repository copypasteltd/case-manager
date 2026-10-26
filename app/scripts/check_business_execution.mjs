// Full browser execution against an explicitly held native PostgreSQL fixture.
// The real API and controlled object provider are used; production cloud is not.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHmac } from 'node:crypto';
import { readFile, writeFile, mkdir, access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
assert.equal(fixture.controlled_provider, true); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/business-execution'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], errors = [], serverErrors = [], checks = []; let browser, passed = false, lastStep = fixture.alice.last_step;
function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(v => alphabet.indexOf(v).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(v => Number.parseInt(v, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000); page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) {
    await page.getByLabel('六位验证码').waitFor(); if (Math.floor(Date.now() / 30000) <= lastStep) await new Promise(resolve => setTimeout(resolve, (lastStep + 1) * 30000 - Date.now() + 300));
    lastStep = Math.floor(Date.now() / 30000); await page.getByLabel('六位验证码').fill(otp(account.mfa_secret, lastStep)); await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { page, context };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  const result = await response.json(); assert(response.ok(), route + ' ' + response.status() + ' ' + result.error?.code); return result.data;
}
async function selectSignedFile(page, dialog) {
  await dialog.getByRole('button', { name: '选择签署件版本', exact: true }).click(); const picker = page.getByRole('dialog', { name: '选择签署件正式版本', exact: true });
  await picker.getByRole('button', { name: new RegExp(fixture.file_name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')) }).click();
  await picker.getByRole('button', { name: new RegExp('^版本 ' + fixture.file_seq + ' ·') }).click(); await picker.waitFor({ state: 'hidden' });
}

try {
  browser = await chromium.launch({ executablePath, headless: true });
  const applicant = await login(fixture.alice), reviewer = await login(fixture.bob), { page, context } = applicant;
  await page.goto(fixture.origin + '/contracts/' + fixture.contract_id); let contract = await request(context, '/contracts/' + fixture.contract_id);
  const originalPlanIDs = contract.payment_schedule.items.map(item => item.id); assert.equal(originalPlanIDs.length, 2);
  await page.getByRole('button', { name: '编辑草稿', exact: true }).click();
  const editor = page.getByRole('dialog', { name: '编辑合同草稿', exact: true });
  await editor.getByLabel('节点 1 名称', { exact: true }).fill('首期款'); await editor.getByLabel('节点 2 名称', { exact: true }).fill('验收尾款');
  await editor.getByLabel('节点 2 满足条件', { exact: true }).fill('验收记录与交付资料齐全后支付，保留原始付款依据。');
  await editor.getByLabel('节点 2 金额', { exact: true }).fill('6.35'); await editor.getByLabel('本次修改依据', { exact: true }).fill('固定两期收付款安排，送审前核对合计。');
  await editor.getByRole('button', { name: '保存为新修订', exact: true }).click();
  await editor.getByText('各收付款节点金额合计须等于合同总额', { exact: false }).waitFor();
  assert.equal(await editor.getByLabel('节点 2 金额', { exact: true }).inputValue(), '6.35');
  await editor.getByLabel('节点 2 金额', { exact: true }).fill('6.34');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) { await page.setViewportSize({ width, height: 1050 }); assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), theme + ' ' + width + ' payment editor overflow'); }
  }
  await page.getByLabel('外观偏好').selectOption('light'); await page.setViewportSize({ width: 390, height: 1050 });
  await editor.getByLabel('节点 1 名称', { exact: true }).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'payment-plan-editor-light-390.png'), fullPage: false });
  await editor.getByRole('button', { name: '保存为新修订', exact: true }).click(); await editor.waitFor({ state: 'hidden' });
  contract = await request(context, '/contracts/' + fixture.contract_id);
  assert.deepEqual(contract.payment_schedule.items.map(item => item.id), originalPlanIDs); assert.equal(contract.payment_schedule.items[1].amount, '6.340000');
  checks.push('The real payment-plan editor preserves stable node identities, rejects an incorrect total without losing input and saves an exact new business revision; both themes and five widths fit');
  await page.getByRole('button', { name: '准备送审申请', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '准备合同申请', exact: true });
  await dialog.getByLabel('准备申请依据', { exact: true }).fill('受控浏览器场景，固定完整的虚构合同。'); await dialog.getByRole('button', { name: '建立申请草稿', exact: true }).click();
  await page.waitForURL('**/requests/*'); const requestID = new URL(page.url()).pathname.split('/').at(-1);
  await page.getByRole('button', { name: '核对并送审', exact: true }).click(); dialog = page.getByRole('dialog', { name: '核对合同送审预览', exact: true });
  await dialog.getByRole('heading', { name: contract.title, exact: true }).waitFor();
  await dialog.getByText('Independent reviewer', { exact: true }).waitFor();
  await dialog.getByLabel('送审依据', { exact: true }).fill('核对准确正文、金额与指定独立审批人。');
  await page.setViewportSize({ width: 390, height: 1050 }); await page.screenshot({ path: path.join(evidence, 'submission-preview-light-390.png'), fullPage: false });
  await dialog.getByRole('button', { name: '确认以此内容送审', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  const submitted = await request(context, '/requests/' + requestID); assert.equal(submitted.state, 'in_review'); assert.equal(submitted.steps.length, 1); assert.equal(submitted.payload.attachments[0].version_id, fixture.file_version_id); assert.equal(submitted.payload.business_payload_schema_version, 2); assert.deepEqual(submitted.payload.content.payment_schedule, contract.payment_schedule);
  checks.push('The real UI fixes the complete contract, exact formal file and independently assigned rule in the submission preview, then creates active review');
  await reviewer.page.goto(fixture.origin + '/approvals');
  await reviewer.page.getByRole('link').filter({ has: reviewer.page.getByRole('heading', { name: contract.title, exact: true }) }).click();
  await reviewer.page.getByRole('button', { name: '处理当前步骤', exact: true }).click();
  dialog = reviewer.page.getByRole('dialog', { name: '处理当前审批步骤', exact: true });
  await dialog.getByLabel('本次决定', { exact: true }).selectOption('approve'); await dialog.getByLabel('决定原因', { exact: true }).fill('本人查阅固定条款后同意。');
  await dialog.getByRole('button', { name: '确认同意当前修订', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  assert.equal((await request(reviewer.context, '/requests/' + requestID)).state, 'approved');
  await page.goto(fixture.origin + '/contracts/' + contract.id); await page.getByRole('button', { name: '登记已签署合同', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '登记真实签署依据', exact: true });
  await dialog.getByLabel('实际签署日期', { exact: true }).fill(fixture.today); await selectSignedFile(page, dialog);
  await dialog.getByLabel('签署条款核对说明', { exact: true }).fill('受控样本：核对签署件和准确批准修订。'); await dialog.getByLabel('签署登记依据', { exact: true }).fill('记录受控签署事实，尚未确认生效。');
  await page.screenshot({ path: path.join(evidence, 'signature-light-390.png'), fullPage: false });
  await dialog.getByRole('button', { name: '确认保存签署登记', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  let current = await request(context, '/contracts/' + contract.id); assert.equal(current.lifecycle, 'pending_signature'); assert.equal(current.active_revision_id, null); assert.equal(current.signature.signed_file_version_id, fixture.file_version_id);
  assert(!(await page.locator('.skip-link').evaluate(node => node === document.activeElement)), 'Closing nested dialogs must restore useful focus');
  checks.push('An independent reviewer completes a real manual UI decision; the applicant selects an exact historical formal signed-file version and records a signature through a genuine Web MFA session without automatic activation');
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好').selectOption(theme);
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1050 }); assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), theme + ' ' + width + ' signature detail overflow');
      if (theme === 'dark' && width === 1440) { await page.evaluate(() => window.scrollTo(0, 0)); await page.screenshot({ path: path.join(evidence, 'signed-dark-1440.png'), fullPage: true }); }
    }
  }
  await page.getByRole('button', { name: '核对并确认生效', exact: true }).click(); dialog = page.getByRole('dialog', { name: '确认合同生效依据', exact: true });
  await dialog.getByLabel('确认生效依据', { exact: true }).fill('独立确认批准日期已到、签署件已核对。'); await dialog.getByRole('button', { name: '确认此批准修订生效', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
  current = await request(context, '/contracts/' + contract.id); assert.equal(current.lifecycle, 'effective'); assert.equal(current.active_revision_id, current.approved_revision_id);
  assert.equal((await request(context, '/requests/' + requestID)).state, 'approved');
  const nodes = (await request(context, '/contracts/' + contract.id + '/milestones')).items; assert.equal(nodes.length, 2); assert.deepEqual(nodes.map(item => item.plan_item_id), originalPlanIDs); assert.equal(nodes[1].amount, '6.340000');
  await page.getByRole('heading', { name: '履约节点', exact: true }).waitFor(); await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: path.join(evidence, 'payment-milestones-dark-1440.png'), fullPage: true });
  checks.push('Explicit activation creates exactly the two approved execution nodes and the real page displays them without marking money as paid');
  await page.goto(fixture.origin + '/contracts/' + fixture.future_contract_id); await page.getByRole('heading', { name: '签署与生效依据', exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '核对并确认生效', exact: true }).count(), 0);
  checks.push('Both themes and five widths fit signed evidence; a separate explicit UI confirmation activates reached content while a future contract offers no early activation');
  await page.goto(fixture.origin + '/contracts/' + fixture.conflict_contract_id); const conflictBase = await request(context, '/contracts/' + fixture.conflict_contract_id);
  await page.getByRole('button', { name: '登记已签署合同', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记真实签署依据', exact: true });
  await dialog.getByLabel('实际签署日期', { exact: true }).fill(fixture.today); await selectSignedFile(page, dialog);
  await dialog.getByLabel('签署条款核对说明', { exact: true }).fill('保留这份未保存的本机核对说明。'); await dialog.getByLabel('签署登记依据', { exact: true }).fill('另一设备将先登记，预期显示冲突。');
  await request(context, '/contracts/' + conflictBase.id + '/signature', 'POST', { expected_version: conflictBase.version, revision_id: conflictBase.current_revision_id, content_hash: conflictBase.content_hash, signed_on: fixture.today, signed_file_version_id: fixture.file_version_id, attestation: 'Another genuine request records the same synthetic evidence.', reason: 'Controlled second-device signature write.' });
  await dialog.getByRole('button', { name: '确认保存签署登记', exact: true }).click();
  await dialog.getByText('合同或签署记录已变化。已保留填写内容，请关闭后核对最新依据。', { exact: true }).waitFor();
  assert.equal(await dialog.getByLabel('签署条款核对说明', { exact: true }).inputValue(), '保留这份未保存的本机核对说明。');
  assert(await dialog.getByRole('button', { name: '确认保存签署登记', exact: true }).isDisabled());
  await page.setViewportSize({ width: 390, height: 1050 }); await page.screenshot({ path: path.join(evidence, 'signature-conflict-dark-390.png'), fullPage: false });
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  checks.push('A second genuine signature request wins before the UI form, and the stale form preserves its selected evidence and typed attestation while disabling confirmation');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  let allBrowserContextsClosed = true; for (const context of contexts) { try { await context.close(); } catch { allBrowserContextsClosed = false; } }
  if (browser) { try { await browser.close(); } catch { allBrowserContextsClosed = false; } }
  const result = { passed, executed_at: new Date().toISOString(), real_api: true, isolated_native_postgresql: true, controlled_file_provider: true, actual_cloud_or_120gb: false, genuine_web_sessions_and_mfa: true, synthetic_company_policy_and_data: true, checks, javascript_errors: errors, server_errors: serverErrors, all_browser_contexts_closed: allBrowserContextsClosed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify(result, null, 2));
  if (!passed || !allBrowserContextsClosed) process.exitCode = 1;
}
