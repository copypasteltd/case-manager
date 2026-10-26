// Actual Web/API subscription and forecast checks against an owned disposable native fixture.
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
const evidence = path.join(root, 'docs/quality/evidence/subscriptions'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_subscriptions.mjs', 'scripts/run_spend_browser.py'])].sort();
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
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const me = await request(alice.context, '/me'), reader = await request(bob.context, '/me'), roles = await request(administrator.context, '/roles?limit=100');
  const grants = [...['subscription_manager', 'forecast_reporter', 'cost_recorder', 'cost_confirmer', 'cost_reverser', 'finance_reporter'].map(key => [me.user.id, key]), ...['subscription_reader', 'forecast_reporter'].map(key => [reader.user.id, key])];
  for (const [user, key] of grants) {
    const role = roles.items.find(r => r.key === key); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: 'org', classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '隔离订阅界面临时验证授权' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const { page, context } = alice;
  const supplier = await request(context, '/parties', 'POST', { name: '虚构订阅服务供应方', kind: 'organization', relationships: ['supplier'], project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离订阅供应方' });
  await page.goto(fixture.origin + '/subscriptions'); await page.getByRole('heading', { name: '订阅与续费', exact: true }).waitFor();
  await page.getByRole('button', { name: '新建订阅', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '新建订阅草稿', exact: true });
  await dialog.getByLabel('订阅产品', { exact: true }).fill('虚构云协作订阅');
  await dialog.getByLabel('所属项目', { exact: true }).selectOption(fixture.project_id);
  await dialog.getByLabel('开支类别', { exact: true }).fill('subscription.browser');
  await dialog.getByRole('button', { name: '选择供应方资料', exact: true }).click();
  let picker = page.getByRole('dialog', { name: '选择往来方资料', exact: true });
  await picker.locator('button.reference-option').filter({ hasText: supplier.name }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
  await dialog.getByLabel('业务负责人', { exact: true }).selectOption(me.user.id);
  await dialog.getByLabel('订阅开始日期', { exact: true }).fill('2025-01-01');
  await dialog.getByLabel('计费周期', { exact: true }).selectOption('month');
  await dialog.getByLabel('首次收费日期', { exact: true }).fill('2025-01-31');
  await dialog.getByLabel('原始计费日（1至31）', { exact: true }).fill('31');
  await dialog.getByLabel('续费方式', { exact: true }).selectOption('automatic');
  await dialog.getByLabel('提前提醒天数', { exact: true }).fill('7, 0');
  await dialog.getByRole('button', { name: '添加已知价格', exact: true }).click();
  await dialog.getByLabel('价格生效日 1', { exact: true }).fill('2025-01-01');
  await dialog.getByLabel('原币币种 1', { exact: true }).fill('HKD');
  await dialog.getByLabel('原币价格 1', { exact: true }).fill('100');
  await dialog.getByLabel('套餐说明 1', { exact: true }).fill('虚构标准月套餐');
  await dialog.getByLabel('登记或修订依据', { exact: true }).fill('核对准确供应方、月末原点及原币价格');
  await commit(dialog, '保存订阅资料'); await page.getByRole('heading', { name: '虚构云协作订阅', exact: true }).waitFor();
  const subscriptionID = new URL(page.url()).pathname.split('/').at(-1);
  let sub = await request(context, '/subscriptions/' + subscriptionID); assert.equal(sub.state, 'draft');
  await page.getByRole('button', { name: '启用订阅', exact: true }).click(); dialog = page.getByRole('dialog', { name: '启用订阅', exact: true });
  await dialog.getByLabel('操作依据', { exact: true }).fill('人工启用准确订阅配置'); await commit(dialog, '启用订阅');
  sub = await request(context, '/subscriptions/' + subscriptionID); assert.equal(sub.state, 'active');
  await layout(page, 'subscription-detail');
  checks.push({ name: 'Web creates a scoped subscription with fixed supplier, named owner, month-end anchor, exact price and explicit human activation', passed: true });

  await page.getByRole('button', { name: '生成账期预测', exact: true }).click(); dialog = page.getByRole('dialog', { name: '生成账期预测', exact: true });
  await dialog.getByLabel('收费日从', { exact: true }).fill('2025-01-01'); await dialog.getByLabel('收费日至（不含）', { exact: true }).fill('2025-04-01');
  await dialog.getByLabel('操作依据', { exact: true }).fill('补建首三期已知预测，不登记费用或付款'); await dialog.getByRole('button', { name: '生成账期预测', exact: true }).click();
  await dialog.getByText(/已新增 3 个账期/).waitFor(); await commit(dialog, '返回订阅');
  let periods = (await request(context, '/subscriptions/' + subscriptionID + '/periods?limit=100')).items;
  assert.equal(periods.length, 3); assert.deepEqual(periods.map(p => p.start_date).sort(), ['2025-01-31', '2025-02-28', '2025-03-31']);
  assert.equal((await request(context, '/costs?category=subscription.browser')).items.length, 0);
  checks.push({ name: 'Human generation persists three anchored forecasts and keeps actual costs absent', passed: true });

  let period = periods.find(p => p.start_date === '2025-01-31');
  const newCost = async (amount, title) => {
    let cost = await request(context, '/costs', 'POST', { title, occurred_on: '2025-01-31', currency: 'HKD', amount, category: 'subscription.browser', counterparty: { party_id: supplier.id, revision_id: supplier.current_revision_id }, allocations: [{ project_id: fixture.project_id, amount }], evidence_version_ids: [fixture.file_version_id], notes: '', source_key: 'subscription-cost:' + crypto.randomUUID(), project_id: fixture.project_id, classification: 'project_restricted', reason: '隔离真实发生费用依据' });
    return request(context, '/costs/' + cost.id + '/confirm', 'POST', base(cost));
  };
  const first = await newCost('40', '虚构首笔订阅扣款'), second = await newCost('80', '虚构第二笔订阅扣款');
  await page.goto(fixture.origin + '/subscription-periods/' + period.id); await page.getByRole('heading', { name: '2025-01-31 账期', exact: true }).waitFor();
  for (const cost of [first, second]) {
    await page.getByRole('button', { name: '关联真实费用', exact: true }).click(); dialog = page.getByRole('dialog', { name: '关联真实费用', exact: true });
    await dialog.getByRole('button', { name: '选择准确费用', exact: true }).click(); picker = page.getByRole('dialog', { name: '选择准确订阅费用', exact: true });
    await picker.locator('button.reference-option').filter({ hasText: cost.title }).click(); await picker.waitFor({ state: 'hidden' }); await modalFocus(page);
    await dialog.getByLabel('操作依据', { exact: true }).fill('关联准确原币扣款费用，保留原预测'); await commit(dialog, '关联真实费用');
  }
  period = await request(context, '/subscription-periods/' + period.id); assert.equal(period.actual_amount, '120.000000'); assert.equal(period.amount, '100.000000');
  await page.getByText('实际扣款超出预测', { exact: true }).waitFor();
  await page.getByRole('button', { name: '核对实际扣款', exact: true }).click(); dialog = page.getByRole('dialog', { name: '确认本次核对', exact: true });
  assert.equal(await dialog.getByRole('button', { name: '确认本次核对', exact: true }).isDisabled(), true);
  await dialog.getByLabel('我已核对预计与实际不一致的原因，并在依据中说明。', { exact: true }).check();
  await dialog.getByLabel('发票仍待补，保留异常提示。', { exact: true }).check();
  await selectProof(page, dialog, '选择固定核对凭证', '选择准确扣款核对凭证');
  await dialog.getByLabel('操作依据', { exact: true }).fill('两笔真实扣款合计120，超出预计20；发票待补'); await commit(dialog, '确认本次核对');
  period = await request(context, '/subscription-periods/' + period.id); assert.equal(period.review_current, true); assert(period.anomalies.includes('invoice_missing'));
  await layout(page, 'billing-period-detail');
  checks.push({ name: 'Web links 40 and 80 actual expenses once, retains forecast 100, and explicitly confirms overcharge 120 with fixed evidence and missing invoice', passed: true });

  assert.equal((await response(bob.context, '/subscription-periods/' + period.id)).status, 404);
  assert.equal((await request(bob.context, '/reports/forecast?from=2025-01-01&to=2025-02-01&category=subscription.browser')).totals.length, 0);
  await bob.page.goto(fixture.origin + '/subscriptions'); await bob.page.getByRole('heading', { name: '订阅与续费', exact: true }).waitFor();
  assert.equal(await bob.page.getByRole('button', { name: '新建订阅', exact: true }).count(), 0);
  checks.push({ name: 'Reader cannot infer a subscription containing unreadable actual expenses or obtain write controls', passed: true });

  await page.setViewportSize({ width: 390, height: 1000 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await stable(page);
  await page.getByRole('button', { name: '核对实际扣款', exact: true }).click(); dialog = page.getByRole('dialog', { name: '确认本次核对', exact: true });
  await dialog.getByLabel('我已核对预计与实际不一致的原因，并在依据中说明。', { exact: true }).check();
  await selectProof(page, dialog, '选择固定核对凭证', '选择准确扣款核对凭证');
  await dialog.getByLabel('操作依据', { exact: true }).fill('保留输入，另一会话冲销后必须重新核对');
  const other = await login(fixture.alice);
  const currentCost = await request(other.context, '/costs/' + second.id);
  await request(other.context, '/costs/' + currentCost.id + '/reverse', 'POST', { ...base(currentCost), effective_on: '2025-02-01' });
  const stale = await request(other.context, '/subscription-periods/' + period.id); assert.equal(stale.version, period.version); assert.notEqual(stale.actual_fingerprint, period.actual_fingerprint);
  await dialog.getByRole('button', { name: '确认本次核对', exact: true }).click(); await dialog.getByText(/输入已保留/).waitFor();
  assert.equal(await dialog.getByLabel('操作依据', { exact: true }).inputValue(), '保留输入，另一会话冲销后必须重新核对');
  await dialog.getByLabel('操作依据', { exact: true }).scrollIntoViewIfNeeded(); await page.screenshot({ path: path.join(evidence, 'billing-stale-review-dark-390.png'), animations: 'disabled' }); await close(dialog);
  await page.reload(); await page.getByRole('heading', { name: '2025-01-31 账期', exact: true }).waitFor(); await page.getByText('费用已变化，须重新核对', { exact: true }).waitFor();
  await page.getByRole('button', { name: '账期操作历史', exact: true }).click(); dialog = page.getByRole('dialog', { name: '账期操作历史', exact: true });
  await dialog.getByRole('heading', { name: '当时固定的费用依据', exact: true }).waitFor(); assert.equal(await dialog.getByText('当时已确认', { exact: false }).count(), 2); await close(dialog);
  checks.push({ name: 'Another session reverses a real cost without changing the period version; exact actual fingerprint rejects the old open form and preserves its input and historical cost snapshot', passed: true });

  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light');
  await page.goto(fixture.origin + '/reports/forecast'); await page.getByLabel('预测月份', { exact: true }).fill('2025-01'); await page.getByLabel('开支类别', { exact: true }).fill('subscription.browser'); await page.getByRole('button', { name: '查询预测', exact: true }).click();
  await page.locator('.cost-net').filter({ hasText: 'HKD 100' }).waitFor(); await page.getByText('本次正常预测覆盖完整', { exact: true }).waitFor(); await layout(page, 'subscription-forecast');
  await page.getByRole('link', { name: '核对账期明细', exact: true }).click(); await page.getByRole('heading', { name: '账期与扣款核对', exact: true }).waitFor(); await page.locator('.billing-period-row').waitFor();
  assert.equal(await page.locator('.billing-period-row').count(), 1); assert.equal(new URL(page.url()).searchParams.get('forecast_only'), 'true'); await layout(page, 'subscription-drilldown');
  const forecast = await request(context, '/reports/forecast?from=2025-01-01&to=2025-02-01&category=subscription.browser'), incurred = await request(context, '/reports/expenses?kind=incurred&from=2025-01-01&to=2025-02-01&category=subscription.browser');
  assert.equal(forecast.totals[0].expected_amount, '100.000000'); assert.equal(incurred.totals[0].net_amount, '120.000000');
  checks.push({ name: 'Forecast 100 and actual incurred 120 retain separate date bases; drilldown preserves exact forecast eligibility and original-currency scope', passed: true });

  await page.goto(fixture.origin + '/subscriptions/' + subscriptionID); await page.getByRole('heading', { name: '虚构云协作订阅', exact: true }).waitFor();
  await page.getByRole('button', { name: '登记新价格', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记新价格', exact: true });
  await dialog.getByLabel('价格生效日', { exact: true }).fill('2028-01-01'); await dialog.getByLabel('原币价格', { exact: true }).fill('150'); await dialog.getByLabel('套餐说明', { exact: true }).fill('虚构未来套餐'); await dialog.getByLabel('操作依据', { exact: true }).fill('预先核对未来价格，历史预测不变'); await commit(dialog, '登记新价格');
  assert.equal((await request(context, '/subscription-periods/' + period.id)).amount, '100.000000');
  await page.getByRole('button', { name: '登记内部取消', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记内部取消', exact: true });
  await dialog.getByLabel('操作依据', { exact: true }).fill('内部已停用，供应方取消仍须核实'); await dialog.getByLabel('我已核对本次操作范围与供应方实际状态的区别。', { exact: true }).check(); await commit(dialog, '登记内部取消');
  sub = await request(context, '/subscriptions/' + subscriptionID); assert.equal(sub.state, 'cancelled'); assert.equal(sub.vendor_state, 'unknown'); assert(sub.signals.includes('vendor_stop_unverified'));
  await page.getByRole('button', { name: '核实供应方状态', exact: true }).click(); dialog = page.getByRole('dialog', { name: '核实供应方状态', exact: true });
  await dialog.getByLabel('供应方实际状态', { exact: true }).selectOption('cancelled');
  const checked = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(Date.now() - 1000)).replace(' ', 'T');
  await dialog.getByLabel('实际核实时间（香港时间）', { exact: true }).fill(checked); await selectProof(page, dialog, '选择固定核实凭证', '选择准确核实凭证'); await dialog.getByLabel('操作依据', { exact: true }).fill('人工核对供应方取消结果及固定凭证'); await commit(dialog, '核实供应方状态');
  sub = await request(context, '/subscriptions/' + subscriptionID); assert.equal(sub.vendor_state, 'cancelled'); assert.equal(sub.vendor_evidence_version_ids[0], fixture.file_version_id);
  checks.push({ name: 'Future repricing preserves existing forecasts; internal cancellation stays separate from actual vendor stop and fixed proof', passed: true });
  assert.equal(errors.length, 0); assert.equal(serverErrors.length, 0); passed = true;
} catch (error) { process.stderr.write(String(error.stack ?? error) + '\n'); }
finally {
  if (administrator?.context && bindings.length) {
    try { await request(administrator.context, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离订阅验证完成，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; cleanupErrors.push('temporary role cleanup failed'); }
  }
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned browser context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), browserClosed = !browser || !browser.isConnected(); passed = passed && before === after && browserClosed && !cleanupErrors.length;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_file_provider: true, controlled_file_size_bytes: '15', production_payments: false, checks, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: !cleanupErrors.some(error => error.includes('context')), all_owned_browser_tabs_closed: browserClosed }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
