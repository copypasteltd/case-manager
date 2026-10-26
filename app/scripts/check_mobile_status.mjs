// Actual Taro H5 + native PostgreSQL/API checks; never a native-device claim.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, generateKeyPairSync, randomUUID, sign } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true); assert.equal(fixture.controlled_provider, true);
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/mobile-status'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TEMP = temporary; process.env.TMP = temporary;
async function sourceFiles(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await sourceFiles(relative)); else result.push(relative); } return result; }
const apiSources = (await sourceFiles('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources = [...new Set([...await sourceFiles('mobile'), ...apiSources, 'package.json', 'package-lock.json', 'scripts/check_mobile_status.mjs', 'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json'])].sort();
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
  const response = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'mobile-status-fixture:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
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
const row = (page, title) => screen(page).locator('.cp-row-title').getByText(title, { exact: true });
async function mfa(page, account) {
  await screen(page).locator('input[maxlength="6"]').fill(await code(account));
  await command(page, '/auth/mfa/step-up', '确认本人操作');
  await screen(page).getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
}
async function chooseService(page, name) {
  await button(page, '添加受影响服务').click(); await field(page, '查找候选资料').fill(name);
  await button(page, '查找候选资料').click(); await row(page, name).click(); await button(page, '添加受影响服务').waitFor();
}
async function statusBadge(page, text) { await screen(page).locator('.cp-state').first().filter({ hasText: new RegExp('^' + text + '$') }).waitFor(); }
const signingKeys = generateKeyPairSync('ed25519');
const publicKey = signingKeys.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
let sequence = 0, lastObserved = 0, operator, reader, service, source, project, hiddenProject, hidden, incident;
async function sample(healthy, options = {}) {
  lastObserved = Math.max(Date.now(), lastObserved + 1);
  const bytes = Buffer.from(JSON.stringify({ source_revision_id: source.revision_id, sequence: String(++sequence), observed_at: new Date(lastObserved).toISOString(), healthy, latency_ms: healthy ? 50 : null, ...options }));
  const signature = sign(null, Buffer.concat([Buffer.from('CP-STATUS-V1\n' + source.id + '\n'), bytes]), signingKeys.privateKey).toString('base64');
  const response = await fetch(fixture.api_origin + '/v1/status-sources/' + source.id + '/observations', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CP-Status-Signature': signature }, body: bytes });
  const result = await response.json(); assert.equal(response.status, 200, result.error?.code); return result.data;
}
async function setupService(token, name, projectID = project.id) {
  return request(token, '/services', 'POST', { name, owner_id: (await request(token, '/me')).user.id, host_label: '虚构境内主机', endpoint_label: 'api.example.invalid / 仅展示', description: '隔离数据库的真实签名观测验证', project_id: projectID, classification: 'project_restricted', reason: '隔离移动服务功能验证' });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  administrator = await login(fixture.administrator); const initialOperator = await login(fixture.alice), initialReader = await login(fixture.bob);
  const alice = await request(initialOperator.token, '/me'), bob = await request(initialReader.token, '/me');
  project = await request(initialOperator.token, '/projects', 'POST', { name: '虚构移动状态项目', owner_id: alice.user.id, member_ids: [bob.user.id], classification: 'project_restricted', scope: '仅限隔离状态验证' });
  hiddenProject = await request(initialOperator.token, '/projects', 'POST', { name: '虚构独立影响范围', owner_id: alice.user.id, member_ids: [], classification: 'project_restricted', scope: '验证全部影响范围' });
  const roles = await request(administrator.token, '/roles?limit=100');
  for (const [userID, roleKey, scopeType, scopeID] of [[alice.user.id, 'status_operator', 'org', null], [bob.user.id, 'status_reader', 'project', project.id]]) {
    const role = roles.items.find(value => value.key === roleKey); assert(role);
    bindings.push(await request(administrator.token, '/role-bindings', 'POST', { user_id: userID, role_id: role.id, scope_type: scopeType, scope_id: scopeID, classification_ceiling: 'project_restricted', valid_until: new Date(Date.now() + 3600000).toISOString(), reason: '仅限本次隔离移动状态验证，结束撤销' }));
  }
  await initialOperator.context.close(); await initialReader.context.close();
  operator = await login(fixture.alice); reader = await login(fixture.bob); const page = operator.page; reviewPage = page;
  // API setup gives real, authorized objects; all reviewed work below uses the
  // actual Taro page. No page.route supplies a synthetic business success.
  phase = 'register real services and verify pagination';
  for (let n = 0; n < 21; n++) await setupService(operator.token, '虚构分页服务 ' + String(n).padStart(2, '0'));
  service = await setupService(operator.token, '虚构生产接口'); hidden = await setupService(operator.token, '虚构受限依赖服务', hiddenProject.id);
  assert.equal(service.state, 'unknown');
  await openList(page, '服务状态'); await button(page, '加载更多服务').waitFor();
  const firstPageCount = await screen(page).locator('.cp-row-title').count(); assert.equal(firstPageCount, 20);
  await button(page, '加载更多服务').click(); await row(page, '虚构分页服务 00').waitFor(); assert.equal(await screen(page).locator('.cp-row-title').count(), 23);
  await field(page, '服务名称或展示信息').fill(service.name); await button(page, '查找服务').click(); await row(page, service.name).waitFor();
  assert.equal(await screen(page).locator('.cp-row-title').count(), 1);
  await button(page, '按项目筛选服务').click(); await row(page, project.name).click(); await row(page, service.name).waitFor();
  await layout(page, 'service-list');
  await row(page, service.name).click(); await screen(page).getByText('尚无监控数据。登记服务本身不会产生正常状态。', { exact: true }).waitFor(); await statusBadge(page, '状态未知');
  passedCheck('Actual service pages paginate, search and filter by a real project; registration without observations remains unknown');

  phase = 'signed observations and historical proof';
  service = await request(operator.token, '/services/' + service.id + '/sources', 'POST', { expected_version: service.version, expected_source_version: 0, base_revision_id: null, name: '虚构签名来源', provider: 'adapter', external_ref: 'isolated-mobile-status', public_key: publicKey, enabled: true, failure_count: 3, recovery_count: 2, interval_seconds: 60, stale_after_seconds: 180, latency_limit_ms: 500, reason: '固定历史公钥与阈值' }); source = service.sources[0];
  for (let n = 0; n < 3; n++) await sample(false); await statusBadge(page, '服务异常');
  sequence += 2; // Leave unused sequence numbers for a genuine late fact.
  for (let n = 0; n < 2; n++) await sample(true); await statusBadge(page, '运行正常');
  const late = await sample(true, { sequence: '4' }); assert.equal(late.applied, false); assert.equal(late.ignored_reason, 'LATE_SEQUENCE');
  await button(page, '查看观测事实').click(); await screen(page).getByText('仅保留事实 · 序号迟到', { exact: true }).waitFor();
  await button(page, '查看并验证原始签名').first().click(); await screen(page).getByText('签名及原始字节摘要校验通过。', { exact: true }).waitFor();
  await layout(page, 'observation-proof', async () => { await button(page, '查看观测事实').click(); await button(page, '查看并验证原始签名').first().click(); await screen(page).getByText('签名及原始字节摘要校验通过。', { exact: true }).waitFor(); }, '签名及原始字节摘要校验通过。');
  await button(page, '来源概览').click(); await button(page, '查看来源修订历史').click(); await screen(page).getByText('第 1 版 · 当前修订', { exact: true }).waitFor();
  passedCheck('Real Ed25519 observations produce failure/recovery thresholds; late facts, exact history and portable raw-byte signature verification remain distinct from current health');

  phase = 'maintenance and stalled fresh status';
  service = await request(operator.token, '/services/' + service.id);
  const maintenance = await request(operator.token, '/services/' + service.id + '/maintenance-windows', 'POST', { expected_version: service.version, start_at: new Date(Date.now() - 60000).toISOString(), end_at: new Date(Date.now() + 300000).toISOString(), reason: '虚构受控维护窗口' });
  await statusBadge(page, '维护中'); await screen(page).getByText('当前评估包含维护覆盖，底层观测仍单独保留。', { exact: true }).waitFor();
  service = await request(operator.token, '/services/' + service.id);
  const windowID = maintenance.maintenance_id; assert(windowID && windowID !== service.id);
  await request(operator.token, '/services/' + service.id + '/maintenance-windows/' + windowID + '/cancel', 'POST', { expected_version: service.version, reason: '虚构维护已取消' });
  await statusBadge(page, '运行正常'); await button(page, '查看维护记录').click(); await screen(page).getByText('已取消的窗口', { exact: true }).waitFor(); await screen(page).getByText('虚构维护已取消', { exact: true }).waitFor();
  const held = [], pattern = '**/v1/services/' + service.id;
  const hold = route => route.request().method() === 'GET' ? held.push(route) : route.continue();
  await page.route(pattern, hold);
  try {
    await page.evaluate(() => { window.__originalStatusDateNow = Date.now; Date.now = () => window.__originalStatusDateNow() - 86400000; });
    await statusBadge(page, '待重新确认');
    await screen(page).getByText('上次评估已到重读时刻，当前状态待重新确认。下方保留的是上次取得的证据。', { exact: true }).waitFor();
  } finally {
    await page.evaluate(() => { if (window.__originalStatusDateNow) Date.now = window.__originalStatusDateNow; delete window.__originalStatusDateNow; });
    await page.unroute(pattern, hold); for (const route of held) await route.continue().catch(() => {});
  }
  await statusBadge(page, '运行正常'); await layout(page, 'service-detail');
  passedCheck('Maintenance preserves underlying observations and cancellation history; a held real refresh and device wall-clock rollback cannot keep an expired green status');

  phase = 'MFA incident creation and all-service visibility';
  await button(page, '事件记录').click(); await heading(page, '服务事件'); await button(page, '登记服务事件').click(); await heading(page, '登记服务事件');
  await field(page, '事件标题').fill('虚构跨服务处理事件'); await field(page, '事件影响描述').fill('准确登记两个项目的受影响服务，不自动扩展可见范围。');
  await select(page, '严重程度', '紧急'); await chooseService(page, service.name); await chooseService(page, hidden.name);
  await field(page, '首次时间线记录').fill('已登记准确影响范围及初次观测。'); await field(page, '操作依据').fill('明确人工登记，供负责人后续处理');
  await layout(page, 'incident-create', async () => { await chooseService(page, service.name); await chooseService(page, hidden.name); });
  assert.equal(await button(page, '确认登记事件').getAttribute('aria-disabled'), 'true');
  await mfa(page, fixture.alice);
  let committedID, originalKey, drop = true;
  const dropPattern = '**/v1/incidents', dropAfterCommit = async route => {
    if (route.request().method() !== 'POST' || !drop) return route.continue();
    drop = false; originalKey = route.request().headers()['idempotency-key'];
    const response = await route.fetch(), value = await response.json(); assert.equal(response.status(), 201); committedID = value.data.id;
    expectedAborts++; await route.abort('failed');
  };
  await page.route(dropPattern, dropAfterCommit);
  try {
    await button(page, '确认登记事件').click(); await screen(page).getByText('连接中断，操作结果尚未确认。请先刷新资料核对。', { exact: true }).waitFor();
    assert.equal(await button(page, '确认登记事件').getAttribute('aria-disabled'), 'true'); assert(committedID);
  } finally { await page.unroute(dropPattern, dropAfterCommit); }
  await button(page, '事件记录').click(); await button(page, '登记服务事件').click(); await field(page, '事件标题').waitFor();
  assert.equal(await field(page, '事件标题').inputValue(), '虚构跨服务处理事件'); assert.equal(await button(page, '确认登记事件').getAttribute('aria-disabled'), 'true');
  await chooseService(page, service.name); await chooseService(page, hidden.name); await mfa(page, fixture.alice);
  const resent = page.waitForRequest(request => new URL(request.url()).pathname === '/v1/incidents' && request.method() === 'POST');
  incident = await command(page, '/incidents', '确认登记事件', 201); assert.equal((await resent).headers()['idempotency-key'], originalKey); assert.equal(incident.id, committedID);
  assert.deepEqual(new Set(incident.service_ids), new Set([service.id, hidden.id])); assert.equal(incident.state, 'open'); assert.equal(incident.severity, 'critical');
  await page.waitForURL(url => url.href.includes('kind=incident') && url.href.includes('id=' + incident.id)); await button(page, '追加事件进展').waitFor();
  assert.equal((await request(operator.token, '/incidents/' + incident.id + '/timeline?limit=20')).items.length, 1);
  await openList(reader.page, '服务事件'); await reader.page.locator('.cp-page').last().getByText('没有符合条件且全部影响范围可见的事件。', { exact: true }).waitFor();
  assert.equal(await screen(reader.page).getByRole('button', { name: '登记服务事件', exact: true }).count(), 0);
  const denied = await fetch(fixture.api_origin + '/v1/incidents/' + incident.id, { headers: { Authorization: 'Bearer ' + reader.token } }); assert([403, 404].includes(denied.status));
  assert.equal((await request(operator.token, '/incidents?limit=100')).items.filter(item => item.title === incident.title).length, 1);
  passedCheck('The UI requires MFA and fixes all affected services; a real after-commit response loss preserves the original key and reopens exactly one incident, while a scoped reader cannot see or write the cross-project record');

  phase = 'incident drafts, conflict comparison and explicit rebase';
  await button(page, '追加事件进展').click(); await select(page, '事件处理阶段', '调查中');
  await field(page, '本次时间线说明').fill('移动端人工调查输入需要保留。'); await field(page, '操作依据').fill('核对实际观测后更新');
  // Settings hide/unmounts the editor, preserving text but never MFA state.
  await button(page, '我的').click(); await heading(page, '我的工作空间'); await button(page, '返回上一页').click(); await button(page, '追加事件进展').waitFor(); await button(page, '追加事件进展').click();
  assert.equal(await field(page, '本次时间线说明').inputValue(), '移动端人工调查输入需要保留。');
  assert.equal(await button(page, '确认追加事件进展').getAttribute('aria-disabled'), 'true'); await mfa(page, fixture.alice);
  const competing = await request(operator.token, '/incidents/' + incident.id, 'PATCH', { expected_version: incident.version, state: 'monitoring', timeline_note: '另一位负责人已完成初步恢复核对。', resolution: '竞争更新的实际恢复说明', postmortem: '竞争更新的临时复盘', reason: '真实版本竞争验证' });
  await command(page, '/incidents/' + incident.id, '确认追加事件进展', 409, 'PATCH');
  assert.equal(await field(page, '本次时间线说明').inputValue(), '移动端人工调查输入需要保留。'); assert.equal(await button(page, '确认追加事件进展').getAttribute('aria-disabled'), 'true');
  await button(page, '读取最新事件用于比较').click(); await screen(page).getByText('竞争更新的实际恢复说明', { exact: true }).waitFor();
  await button(page, '保留我的文字，以最新修订继续核对').click(); await screen(page).locator('input[maxlength="6"]').waitFor();
  assert.equal(await field(page, '本次时间线说明').inputValue(), '移动端人工调查输入需要保留。'); assert.equal(await button(page, '确认追加事件进展').getAttribute('aria-disabled'), 'true');
  await mfa(page, fixture.alice); incident = await command(page, '/incidents/' + incident.id, '确认追加事件进展', 200, 'PATCH'); assert.equal(incident.version, competing.version + 1); assert.equal(incident.state, 'investigating');
  await button(page, '追加事件进展').waitFor();
  passedCheck('Hiding restores only bounded text; a genuine 409 keeps input and blocks resubmission, compares the actual latest record and requires explicit rebase plus new MFA');

  phase = 'resolution validation, immutable recovery time and full timeline';
  await button(page, '追加事件进展').click(); await select(page, '事件处理阶段', '已解决'); await field(page, '本次时间线说明').fill('人工核对已恢复。'); await field(page, '操作依据').fill('记录实际恢复');
  await mfa(page, fixture.alice); await button(page, '确认追加事件进展').click(); await screen(page).getByText('解决事件前请填写实际恢复与解决说明。', { exact: true }).first().waitFor();
  await field(page, '恢复与解决说明').fill('两项受影响服务均已人工核对恢复，保留观测来源。'); await field(page, '事件复盘').fill('补充防止再次发生的处理措施。');
  incident = await command(page, '/incidents/' + incident.id, '确认追加事件进展', 200, 'PATCH'); const resolvedAt = incident.resolved_at; assert(resolvedAt);
  await button(page, '追加事件进展').waitFor(); await layout(page, 'incident-detail');
  await button(page, '追加事件进展').click(); await select(page, '事件处理阶段', '已解决');
  const statePicker = screen(page).locator('taro-picker-core[aria-label="事件处理阶段"]'); await statePicker.click(); await page.locator('.weui-picker:visible').waitFor(); assert.deepEqual(await page.locator('.weui-picker:visible .weui-picker__item').allTextContents(), ['已解决']); await page.locator('.weui-picker:visible .weui-picker__action').filter({ hasText: /^确定$/ }).click();
  await field(page, '本次时间线说明').fill('已解决事件补充后续复盘。'); await field(page, '事件复盘').fill('后续复盘补充，首次恢复时间保持原事实。'); await field(page, '操作依据').fill('补充复盘，不重新打开事件');
  await mfa(page, fixture.alice); incident = await command(page, '/incidents/' + incident.id, '确认追加事件进展', 200, 'PATCH'); assert.equal(incident.resolved_at, resolvedAt);
  await button(page, '追加事件进展').waitFor();
  for (let n = 0; n < 21; n++) incident = await request(operator.token, '/incidents/' + incident.id, 'PATCH', { expected_version: incident.version, state: 'resolved', timeline_note: '虚构历史记录 ' + n, resolution: incident.resolution, postmortem: incident.postmortem, reason: '验证真实时间线分页' });
  await button(page, '重新读取当前权限与资料').click(); await button(page, '加载更早事件进展').waitFor(); await button(page, '加载更早事件进展').click(); await screen(page).getByText('已登记准确影响范围及初次观测。', { exact: true }).waitFor();
  assert.equal((await request(operator.token, '/incidents/' + incident.id)).resolved_at, resolvedAt);
  await layout(page, 'incident-history', undefined, '不可变事件时间线');
  passedCheck('Resolution requires actual recovery text; resolved records accept postmortems without reopening or changing first recovery time, and actual timeline pagination reaches the original record');

  phase = 'reader permissions revoked during a live status page';
  await button(reader.page, '服务目录').click(); await row(reader.page, service.name).click(); await screen(reader.page).getByText('可信观测来源', { exact: true }).waitFor();
  await request(administrator.token, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) });
  const readerBinding = bindings.find(binding => binding.user_id === bob.user.id); assert(readerBinding);
  const revokedRead = reader.page.waitForResponse(response => new URL(response.url()).pathname === '/v1/services/' + service.id && response.request().method() === 'GET' && response.status() === 401);
  await request(administrator.token, '/role-bindings/' + readerBinding.id + '/revoke', 'POST', { expected_version: readerBinding.version, reason: '验证当前权限收回后清理观测资料' }); bindings.splice(bindings.indexOf(readerBinding), 1);
  const revokedResponse = await revokedRead; assert.equal((await revokedResponse.json()).error.code, 'UNAUTHENTICATED');
  await reader.page.getByRole('button', { name: '登录工作空间 ↗', exact: true }).waitFor();
  assert.equal(await reader.page.getByText(service.name, { exact: true }).count(), 0);
  assert.equal(await reader.page.getByText('可信观测来源', { exact: true }).count(), 0);
  const relogged = await login(fixture.bob);
  const deniedAfterLogin = await fetch(fixture.api_origin + '/v1/services/' + service.id, { headers: { Authorization: 'Bearer ' + relogged.token } });
  assert.equal(deniedAfterLogin.status, 404); assert.equal((await deniedAfterLogin.json()).error.code, 'RESOURCE_NOT_FOUND');
  assert.equal(await screen(relogged.page).getByText('服务状态', { exact: true }).count(), 0);
  passedCheck('Real role revocation invalidates the old session, clears service facts and returns to login; a fresh session still cannot read the revoked service');
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} catch (error) {
  checks.push({ name: phase, passed: false, error: error.message });
  if (reviewPage) try { failureNotices = (await screen(reviewPage).locator('.cp-notice').allTextContents()).slice(0, 20); await reviewPage.screenshot({ path: path.join(evidence, 'failed-attempt.png'), fullPage: true }); } catch { /* cleanup remains mandatory */ }
  process.stderr.write(phase + ': ' + error.message + '\n');
} finally {
  if (administrator && bindings.length) {
    try { await request(administrator.token, '/auth/mfa/step-up', 'POST', { code: await code(fixture.administrator) }); for (const binding of bindings) await request(administrator.token, '/role-bindings/' + binding.id + '/revoke', 'POST', { expected_version: binding.version, reason: '隔离移动状态检查结束，撤销临时授权' }); cleanedBindings = true; }
    catch (error) { checks.push({ name: 'Revoke owned temporary bindings', passed: false, error: error.message }); passed = false; }
  }
  for (const context of contexts) await context.close(); if (browser) await browser.close();
  const closed = (!browser || !browser.isConnected()) && contexts.every(context => context.pages().length === 0), after = await digest();
  const result = { started_at: started, completed_at: new Date().toISOString(), passed: passed && before === after && cleanedBindings && closed, scope: 'Actual Taro H5 UI and native API/PostgreSQL; native mini-program clocks/devices and Android require separate acceptance', setup_via_api: ['temporary scoped roles', '23 services', 'one immutable monitoring source', 'real Ed25519 signed HTTP observations', 'maintenance creation/cancellation', 'one competing event update', '21 additional immutable timeline entries'], source_sha256: before, source_sha256_after: after, source_files: sources.length, sources, source_unchanged_during_run: before === after, checks, layouts, failure_notices: failureNotices, javascript_errors: errors, unexpected_server_errors: serverErrors, expected_network_aborts: expectedAborts, network_failures: failures, owned_temporary_bindings_revoked: cleanedBindings, owned_contexts_closed: contexts.length, all_browser_contexts_closed: closed, all_browser_tabs_closed: closed, all_owned_browser_tabs_closed: closed, actual_cloud_invoked: false, actual_mini_program_runtime_invoked: false, actual_android_runtime_invoked: false };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); process.stdout.write(JSON.stringify({ passed: result.passed, checks: checks.length, layouts: layouts.length, closed_contexts: contexts.length }) + '\n');
  if (!result.passed) process.exitCode = 1;
}
