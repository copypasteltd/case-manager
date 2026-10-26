import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, stableMobile } from './mobile_picker_browser.mjs';
import { recordViewport } from './status_bridge_recording.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/status-bridge-browser');
const client = process.env.CP_STATUS_BRIDGE_CLIENT; assert.ok(['web', 'mobile'].includes(client));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'native-status-bridge'); assert.equal(fixture.isolated_database, true);
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const f of manifest.files) { h.update(f + '\0'); h.update(await readFile(path.join(app, f))); h.update('\0'); } return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const checks = [], layouts = [], screenshots = [], recordings = [], executions = [], errors = [], serverErrors = [], cleanupErrors = [];
let browser, context, page, passed = false, closed = false, failure, token;
const button = (name) => screen(page).getByRole('button', { name, exact: true });
async function done(message) { checks.push(message); console.log('Completed actual flow: ' + message); }
async function sample(healthy = '1', up = '1') {
  const r = await fetch(fixture.prometheus_control_url, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CP-Fixture-Control': fixture.prometheus_control_token }, body: JSON.stringify({ healthy, up, duration: '0.025' }) });
  assert.equal(r.status, 204);
}
async function bridge(expected = 'ACKNOWLEDGED', status = 0) {
  let stdout = '', stderr = '', exitCode = 0;
  try { ({ stdout, stderr } = await promisify(execFile)(path.join(app, '.local/status-bridge/bin/status-bridge.exe'), ['--config', fixture.bridge_config, '--once'], { cwd: app, timeout: 15000, windowsHide: true, maxBuffer: 65536 })); }
  catch (e) { exitCode = typeof e.code === 'number' ? e.code : -1; stdout = e.stdout ?? ''; stderr = e.stderr ?? ''; }
  assert.equal(exitCode, status, 'Native bridge exit status');
  const lines = (stdout + '\n' + stderr).trim().split(/\r?\n/).filter(Boolean).map(line => JSON.parse(line));
  const result = lines.find(line => line.msg === 'collection result'); assert.ok(result); assert.equal(result.code, expected);
  assert.equal(result.source_id, fixture.source_id);
  executions.push({ code: result.code, exit_code: exitCode, source_id: result.source_id, sequence: result.sequence,
    observation_id: result.observation_id, applied: result.applied, replayed: result.replayed, native_binary: true });
  return result;
}
async function service() {
  const r = client === 'web' ? await context.request.get(fixture.origin + '/v1/services/' + fixture.service_id)
    : await fetch(fixture.api_origin + '/v1/services/' + fixture.service_id, { headers: { Authorization: 'Bearer ' + token } });
  assert.equal(typeof r.status === 'function' ? r.status() : r.status, 200); return (await r.json()).data;
}
async function refresh(state) {
  if (client === 'web') { await page.getByRole('button', { name: '刷新状态', exact: true }).click(); await page.locator('.page-heading .badge').filter({ hasText: state }).waitFor(); }
  else { await button('刷新状态与依据').click(); await screen(page).locator('.cp-state').first().filter({ hasText: new RegExp('^' + state + '$') }).waitFor(); }
}
async function image(name, caption) {
  if (client === 'mobile') await stableMobile(page); else await page.evaluate(() => document.fonts.ready);
  const file = 'docs/media/screenshots/' + name + '.png', full = path.join(app, '..', file);
  await page.screenshot({ path: full, fullPage: false, animations: 'disabled' });
  screenshots.push({ file, caption, sha256: createHash('sha256').update(await readFile(full)).digest('hex'), synthetic_data: true, real_device: false });
}
try {
  browser = await chromium.launch({ executablePath, headless: true });
  context = await browser.newContext({ viewport: { width: client === 'web' ? 1440 : 390, height: 900 }, reducedMotion: 'reduce', colorScheme: 'light' });
  page = await context.newPage(); page.setDefaultTimeout(20000);
  page.on('pageerror', e => errors.push(e.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ status: r.status(), path: new URL(r.url()).pathname }); });
  await page.goto(fixture.origin);
  const account = client === 'web' ? fixture.alice : fixture.bob;
  if (client === 'web') {
    await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password);
    await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
    if (account.mfa_secret) {
      while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
      await page.getByLabel('六位验证码').fill(otp(account.mfa_secret, Math.floor(Date.now() / 30000)));
      await page.getByRole('button', { name: '验证并继续', exact: true }).click();
    }
    await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
    await page.goto(fixture.origin + '/services/' + fixture.service_id);
    await page.getByRole('heading', { name: 'Synthetic monitored API', exact: true }).waitFor();
    assert.equal((await service()).state, 'unknown'); await refresh('状态未知');
    await done('Web starts unknown after an actual source revision; no inherited healthy status.');
    await sample(); const first = await bridge(); await sample(); await bridge();
    assert.equal(first.sequence, '1'); assert.equal((await service()).state, 'operational'); await refresh('运行正常');
    const duplicate = await bridge('NO_NEW_SAMPLE'); assert.equal(duplicate.sequence, '');
    assert.equal((await service()).sources[0].projection.sequence, '2');
    await image('native-status-bridge-operational', '实际原生桥接程序写入签名观测后，Web 显示运行正常；合成数据。');
    await done('The Windows CLI reads all six Prometheus queries, signs/durably records observations and deduplicates the exact scrape across process restarts.');
    recordings.push(await recordViewport(page, app, 'native-service-health-workflow', async () => {
      for (let n = 0; n < 3; n++) { await sample('0'); await bridge(); }
      assert.equal((await service()).state, 'down'); await refresh('服务异常');
      await image('native-status-bridge-down', '三次实际失败观测触发服务不可用；来源、时效与阈值可核对。');
      await page.waitForTimeout(1000);
      for (let n = 0; n < 2; n++) { await sample(); await bridge(); }
      assert.equal((await service()).state, 'operational'); await refresh('运行正常');
      await page.getByRole('button', { name: '查看观测事实', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: '观测事实', exact: true });
      await dialog.getByRole('button', { name: '查看并验证原始签名', exact: true }).first().click();
      await dialog.getByText(/签名及原始字节摘要校验通过/).waitFor();
      await page.waitForTimeout(1200); await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
    }));
    await done('Three native CLI failures make the Web service down; two successes recover it, and the UI verifies the original stored signature.');
    const seq = (await service()).sources[0].projection.sequence;
    await sample('0', '0'); await bridge('SCRAPE_UNAVAILABLE', 75); assert.equal((await service()).sources[0].projection.sequence, seq);
    await done('A failed Prometheus scrape exits 75 without adding a healthy or failed service fact.');
    for (const theme of ['light', 'dark']) {
      await page.getByLabel('外观偏好', { exact: true }).selectOption(theme);
      for (const width of [390, 768, 1440]) {
        await page.setViewportSize({ width, height: 900 }); await page.waitForTimeout(300);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
        layouts.push({ client, theme, width, passed: true });
      }
    }
  } else {
    await screen(page).locator('input[aria-label="内部账号"]').fill(account.login); await screen(page).locator('input[aria-label="密码"]').fill(account.password);
    const pending = page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST');
    await button('登录工作空间 ↗').click(); const login = (await (await pending).json()).data;
    assert.equal(login.step, 'authenticated'); token = login.access_token;
    await screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
    await page.evaluate(id => { location.hash = '#/pages/status/index?kind=service&id=' + id; }, fixture.service_id);
    await screen(page).getByText('Synthetic monitored API', { exact: true }).waitFor();
    for (let n = 0; n < 3; n++) { await sample('0'); await bridge(); }
    assert.equal((await service()).state, 'down'); await refresh('服务异常');
    await image('native-status-bridge-mobile', 'Taro H5 只读用户查看由原生桥接写入的故障、来源和观测事实；非真机截图。');
    assert.equal(await button('编辑服务').count(), 0); assert.equal(await button('登记来源').count(), 0);
    await done('The actual Taro H5 reader sees the same CLI-produced outage with no service/source write controls.');
    await button('查看观测事实').click(); await button('查看并验证原始签名').first().click();
    await screen(page).getByText('签名及原始字节摘要校验通过。', { exact: true }).waitFor();
    await done('The Taro H5 client reads the original signed observation facts from the same isolated API.');
    for (const width of [320, 390, 768]) {
      await page.setViewportSize({ width, height: 900 }); await stableMobile(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
      layouts.push({ client, theme: 'light', width, passed: true });
    }
  }
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); assert.equal(before, await digest()); passed = true;
} catch (e) {
  failure = String(e.stack ?? e); console.error(failure);
  if (page) {
    try { await page.screenshot({ path: path.join(app, '.local/status-bridge-' + client + '-failure.png'), fullPage: false }); }
    catch { /* Private diagnostics do not change cleanup. */ }
  }
} finally {
  if (context) try { await context.close(); closed = true; } catch (e) { cleanupErrors.push(e.message); }
  if (browser) try { await browser.close(); } catch (e) { cleanupErrors.push(e.message); }
  const after = await digest();
  await writeFile(path.join(evidence, client + '.json'), JSON.stringify({ passed: passed && closed && cleanupErrors.length === 0 && before === after,
    checked_at: new Date().toISOString(), client, source_sha256: before, source_unchanged: before === after,
    contexts_created: context ? 1 : 0, contexts_closed: closed ? 1 : 0, owned_browser_closed: !!browser && !browser.isConnected(),
    checks, layouts, screenshots, recordings, native_cli_executions: executions, errors, serverErrors, cleanupErrors,
    ...(failure ? { failure } : {}), synthetic_data: true, real_device: false, real_model: false }, null, 2) + '\n');
}
console.log(JSON.stringify({ passed, client, checks: checks.length, layouts: layouts.length, contexts_closed: closed ? 1 : 0 }));
process.exit(passed && closed && cleanupErrors.length === 0 ? 0 : 1);
