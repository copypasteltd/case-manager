// Actual Web/API/Ed25519 checks against an owned disposable native fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, generateKeyPairSync, sign } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.origin, 'http://localhost:5189');
assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1'); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/service-status'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_status.mjs', 'scripts/run_status_browser.py'])].sort();
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
const keys = generateKeyPairSync('ed25519');
const publicKey = keys.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
let sequence = 0, lastObserved = 0;
async function sample(source, healthy) {
  lastObserved = Math.max(Date.now(), lastObserved + 1);
  const body = Buffer.from(JSON.stringify({ source_revision_id: source.revision_id, sequence: String(++sequence), observed_at: new Date(lastObserved).toISOString(), healthy, latency_ms: healthy ? 50 : null }));
  const message = Buffer.concat([Buffer.from('CP-STATUS-V1\n' + source.id + '\n'), body]);
  const value = await fetch(fixture.api_origin + '/v1/status-sources/' + source.id + '/observations', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CP-Status-Signature': sign(null, message, keys.privateKey).toString('base64') }, body });
  const result = await value.json(); assert.equal(value.status, 200, 'Signed synthetic sample was rejected: ' + result.error?.code); return result.data;
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  alice = await login(fixture.alice); bob = await login(fixture.bob); administrator = await login(fixture.administrator);
  const aliceMe = await request(alice.context, '/me'), bobMe = await request(bob.context, '/me');
  const project = await request(alice.context, '/projects', 'POST', { name: '虚构服务状态验证项目', scope: '仅用于隔离界面验证', owner_id: aliceMe.user.id, member_ids: [bobMe.user.id], classification: 'project_restricted' });
  const hiddenProject = await request(alice.context, '/projects', 'POST', { name: '虚构独立影响范围', scope: '验证全部事件影响范围', owner_id: aliceMe.user.id, member_ids: [], classification: 'project_restricted' });
  const roles = await request(administrator.context, '/roles?limit=100');
  for (const [user, roleKey, scopeType, scopeID] of [[aliceMe.user.id, 'status_operator', 'org', null], [bobMe.user.id, 'status_reader', 'project', project.id]]) {
    const role = roles.items.find(item => item.key === roleKey); assert(role);
    bindings.push(await request(administrator.context, '/role-bindings', 'POST', { user_id: user, role_id: role.id, scope_type: scopeType, scope_id: scopeID, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '仅限隔离数据库的服务界面验证，结束即撤销' }));
  }
  await alice.context.close(); await bob.context.close(); alice = await login(fixture.alice); bob = await login(fixture.bob);
  const { page, context } = alice;
  await page.getByRole('link', { name: '服务与事件', exact: true }).click(); await page.getByRole('button', { name: '登记服务', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '登记服务', exact: true });
  await dialog.getByLabel('服务名称', { exact: true }).fill('虚构生产接口'); await dialog.getByLabel('所属项目', { exact: true }).selectOption(project.id);
  await dialog.getByLabel('主机展示名称', { exact: true }).fill('虚构业务主机'); await dialog.getByLabel('端点展示名称', { exact: true }).fill('api.example.invalid / 展示名称'); await dialog.getByLabel('登记原因', { exact: true }).fill('隔离状态验证，尚未接入真实监控');
  await dialog.getByRole('button', { name: '确认登记服务', exact: true }).click(); await page.getByRole('heading', { name: '虚构生产接口', exact: true }).waitFor();
  const serviceID = new URL(page.url()).pathname.split('/').at(-1); let service = await request(context, '/services/' + serviceID);
  assert.equal(service.state, 'unknown'); await page.getByRole('heading', { name: '尚无监控数据', exact: true }).waitFor();
  const refused = await response(administrator.context, '/services/' + serviceID); assert([403, 404].includes(refused.status));
  await bob.page.goto(fixture.origin + '/services/' + serviceID); await bob.page.getByRole('heading', { name: '虚构生产接口', exact: true }).waitFor(); assert.equal(await bob.page.getByRole('button', { name: '编辑服务', exact: true }).count(), 0);
  checks.push('Real Web creation starts unknown with no monitoring data; the system administrator cannot read content and a scoped reader cannot edit');

  await page.getByRole('button', { name: '登记来源', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记观测来源', exact: true });
  await dialog.getByLabel('来源名称', { exact: true }).fill('虚构签名探测来源'); await dialog.getByLabel('外部来源参考', { exact: true }).fill('synthetic-monitor-1'); await dialog.getByLabel('Ed25519 公钥（Base64）', { exact: true }).fill(publicKey); await dialog.getByLabel('发布原因', { exact: true }).fill('登记受控签名来源');
  await dialog.getByRole('button', { name: '确认发布来源', exact: true }).click(); await dialog.waitFor({ state: 'detached' }); service = await request(context, '/services/' + serviceID); const source = service.sources[0]; assert.equal(service.state, 'unknown');
  for (let index = 0; index < 3; index++) await sample(source, false);
  assert.equal((await request(context, '/services/' + serviceID)).state, 'down');
  for (let index = 0; index < 2; index++) await sample(source, true);
  await page.getByRole('button', { name: '刷新状态', exact: true }).click(); await page.locator('.page-heading .badge').filter({ hasText: '运行正常' }).waitFor();
  checks.push('The UI publishes an immutable public-key source and real signed HTTP observations produce three-failure down and two-success recovery');

  await page.getByRole('button', { name: '查看观测事实', exact: true }).click(); dialog = page.getByRole('dialog', { name: '观测事实', exact: true });
  await dialog.getByRole('button', { name: '查看并验证原始签名', exact: true }).first().click(); await dialog.getByText(/签名及原始字节摘要校验通过/).waitFor();
  const [download] = await Promise.all([page.waitForEvent('download'), dialog.getByRole('button', { name: '导出这条验证材料', exact: true }).click()]); assert.match(download.suggestedFilename(), /^observation-.*\.json$/); await download.delete();
  await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('The browser verifies original Ed25519 bytes and SHA-256 using the exact historical public key and exports only the selected evidence');

  await page.getByRole('button', { name: '安排维护', exact: true }).click(); dialog = page.getByRole('dialog', { name: '安排维护窗口', exact: true });
  const hk = value => new Date(value + 8 * 3600000).toISOString().slice(0, 16);
  await dialog.getByLabel('开始时间（香港 UTC+8）', { exact: true }).fill(hk(Date.now() - 60000)); await dialog.getByLabel('结束时间（香港 UTC+8）', { exact: true }).fill(hk(Date.now() + 300000)); await dialog.getByLabel('维护原因', { exact: true }).fill('虚构维护覆盖验证'); await dialog.getByRole('button', { name: '确认安排维护', exact: true }).click(); await dialog.waitFor({ state: 'detached' });
  assert.equal((await request(context, '/services/' + serviceID)).state, 'maintenance'); await page.getByRole('button', { name: '查看维护记录', exact: true }).click(); dialog = page.getByRole('dialog', { name: '维护记录', exact: true }); await dialog.getByRole('button', { name: '取消此维护窗口', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '取消维护窗口', exact: true }); await dialog.getByLabel('取消原因', { exact: true }).fill('受控维护结束'); await dialog.getByRole('button', { name: '确认取消维护', exact: true }).click(); await dialog.waitFor({ state: 'detached' }); assert.equal((await request(context, '/services/' + serviceID)).state, 'operational');
  checks.push('Real maintenance creation overlays current evidence; cancellation appends history and restores the evaluated underlying state');

  // Keep the real recheck pending; the existing successful response must lose
  // its live badge using only the monotonic deadline, without another reply.
  const held = [], pattern = '**/v1/services/' + serviceID;
  const hold = route => { if (route.request().method() === 'GET') held.push(route); else return route.continue(); };
  await page.route(pattern, hold);
  try { await page.getByText('上次评估已到重读时刻，当前状态待重新确认。下方保留的是上次取得的证据。', { exact: true }).waitFor({ timeout: 15000 }); assert.equal(await page.locator('.page-heading .badge').innerText(), '待重新确认'); }
  finally { await page.unroute(pattern, hold); for (const route of held) await route.continue().catch(() => {}); }
  await page.locator('.page-heading .badge').filter({ hasText: '运行正常' }).waitFor();
  await layout(page, 'service');
  checks.push('A stalled real refresh cannot keep a cached green badge past its server deadline; service detail fits five widths in both themes');

  const hidden = await request(context, '/services', 'POST', { name: '虚构受限依赖服务', owner_id: aliceMe.user.id, host_label: '', endpoint_label: '', description: '独立项目范围', project_id: hiddenProject.id, classification: 'project_restricted', reason: '全部影响范围验证' });
  await page.goto(fixture.origin + '/incidents'); await page.getByRole('button', { name: '登记事件', exact: true }).click(); dialog = page.getByRole('dialog', { name: '登记服务事件', exact: true });
  await dialog.getByLabel('事件标题', { exact: true }).fill('虚构跨服务事件'); await dialog.getByLabel('虚构生产接口', { exact: true }).check(); await dialog.getByLabel('虚构受限依赖服务', { exact: true }).check(); await dialog.getByLabel('首次时间线记录', { exact: true }).fill('已记录准确影响范围'); await dialog.getByLabel('登记原因', { exact: true }).fill('受控事件范围验证'); await dialog.getByRole('button', { name: '确认登记事件', exact: true }).click(); await page.getByRole('heading', { name: '虚构跨服务事件', exact: true }).waitFor();
  const incidentID = new URL(page.url()).pathname.split('/').at(-1); let incident = await request(context, '/incidents/' + incidentID); assert.deepEqual(new Set(incident.service_ids), new Set([serviceID, hidden.id]));
  assert.equal((await response(bob.context, '/incidents/' + incidentID)).status, 404); assert.equal((await request(bob.context, '/incidents')).items.some(item => item.id === incidentID), false);
  checks.push('A real UI incident fixes both impact services, and a reader of only one receives neither the event nor a partial list summary');

  await page.getByRole('button', { name: '更新处置进展', exact: true }).click(); dialog = page.getByRole('dialog', { name: '更新事件进展', exact: true }); await dialog.getByLabel('新增时间线记录', { exact: true }).fill('这份未提交输入必须保留'); await dialog.getByLabel('本次变更原因', { exact: true }).fill('并发版本验证');
  incident = await request(context, '/incidents/' + incidentID, 'PATCH', { expected_version: incident.version, state: 'investigating', timeline_note: '第二设备先完成调查登记', resolution: '', postmortem: '', reason: '受控第二设备更新' });
  await dialog.getByRole('button', { name: '确认追加进展', exact: true }).click(); await dialog.getByText(/填写内容已保留/).waitFor(); assert.equal(await dialog.getByLabel('新增时间线记录', { exact: true }).inputValue(), '这份未提交输入必须保留'); assert.equal(await dialog.getByRole('button', { name: '确认追加进展', exact: true }).isDisabled(), true); await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await page.getByRole('button', { name: '刷新记录', exact: true }).click(); await page.getByText('当前第 2 版', { exact: false }).waitFor();
  await page.getByRole('button', { name: '更新处置进展', exact: true }).click(); dialog = page.getByRole('dialog', { name: '更新事件进展', exact: true }); await dialog.getByLabel('更新后的进度', { exact: true }).selectOption('resolved'); await dialog.getByLabel('新增时间线记录', { exact: true }).fill('恢复已核实'); await dialog.getByLabel('恢复说明', { exact: true }).fill('虚构依赖恢复并完成人工确认'); await dialog.getByLabel('本次变更原因', { exact: true }).fill('受控解决验证'); await dialog.getByRole('button', { name: '确认追加进展', exact: true }).click(); await dialog.waitFor({ state: 'detached' }); incident = await request(context, '/incidents/' + incidentID); const resolvedAt = incident.resolved_at; assert(resolvedAt);
  await page.getByRole('button', { name: '追加复盘', exact: true }).click(); dialog = page.getByRole('dialog', { name: '追加事件复盘', exact: true }); assert.equal(await dialog.getByLabel('更新后的进度', { exact: true }).isDisabled(), true); await dialog.getByLabel('新增时间线记录', { exact: true }).fill('追加改进措施'); await dialog.getByLabel('复盘', { exact: true }).fill('虚构复盘：完善独立告警链路'); await dialog.getByLabel('本次变更原因', { exact: true }).fill('受控追加复盘'); await dialog.getByRole('button', { name: '确认追加进展', exact: true }).click(); await dialog.waitFor({ state: 'detached' }); incident = await request(context, '/incidents/' + incidentID); assert.equal(incident.resolved_at, resolvedAt); assert.equal(incident.state, 'resolved'); assert.equal(incident.version, 4); assert.equal((await request(context, '/incidents/' + incidentID + '/timeline')).items.length, 4);
  await layout(page, 'incident');
  checks.push('A second-device conflict preserves input and disables blind resubmission; resolution and later postmortem append four immutable entries without changing first resolution, across both themes and five widths');

  await page.goto(fixture.origin + '/services/' + serviceID); await page.getByRole('button', { name: '发布来源修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: '发布来源修订', exact: true }); await dialog.getByLabel('启用此来源', { exact: true }).uncheck(); await dialog.getByLabel('发布原因', { exact: true }).fill('停止受控来源'); await dialog.getByRole('button', { name: '确认发布来源', exact: true }).click(); await dialog.waitFor({ state: 'detached' }); service = await request(context, '/services/' + serviceID); assert.equal(service.state, 'unknown'); assert.equal(service.sources[0].version, 2);
  await page.getByRole('button', { name: '查看修订', exact: true }).click(); dialog = page.getByRole('dialog', { name: '来源修订历史', exact: true }); await dialog.getByText('历史修订', { exact: true }).waitFor(); assert.equal(await dialog.locator('.status-history-entry').count(), 2); await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
  checks.push('Disabling a source publishes a new revision, immediately returns unknown and retains the prior complete configuration');

  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) { process.stderr.write(String(error) + '\n'); }
finally {
  if (administrator) {
    try { for (const binding of bindings) await request(administrator.context, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离服务界面验证结束，撤销临时权限' }); cleanedBindings = true; }
    catch { passed = false; process.stderr.write('Temporary isolated role cleanup did not complete.\n'); }
  }
  for (const context of contexts) await context.close().catch(() => {});
  if (browser) await browser.close();
  const after = await digest(); passed = passed && before === after;
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ passed, executed_at: new Date().toISOString(), source_sha256: before, source_file_count: sources.length, source_unchanged_during_tests: before === after, real_web_api: true, isolated_native_database: true, controlled_signed_observations: true, production_monitoring: false, checks, errors, server_errors: serverErrors, temporary_bindings_revoked: cleanedBindings, all_owned_browser_contexts_closed: true, all_owned_browser_tabs_closed: true }, null, 2) + '\n');
}
process.stdout.write(JSON.stringify({ passed, checks: checks.length }) + '\n'); process.exitCode = passed ? 0 : 1;
