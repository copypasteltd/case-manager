import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { recordViewport } from './miniapp_identity_recording.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..'), mobile = process.argv.includes('--mobile');
const group = 'miniapp-identity-' + (mobile ? 'mobile' : 'web'), evidence = path.join(app, '../docs/quality/evidence', group);
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.miniapp_identity_fixture, true); assert.equal(fixture.isolated_database, true);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) h.update(file + '\0').update(await readFile(path.join(app, file))).update('\0'); return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temp = path.join(app, '.local/browser-temp'); await mkdir(temp, { recursive: true }); process.env.TMP = temp; process.env.TEMP = temp;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], checks = [], layouts = [], screenshots = [], recordings = [], errors = [], serverErrors = [];
let browser, passed = false, failure, contextsClosed = 0;
const screen = page => mobile ? mobileScreen(page) : page.locator('body');
const escaped = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const button = (page, label) => screen(page).getByRole('button', { name: new RegExp('^' + escaped(label) + '(?: ↗)?$') });
const field = (page, label) => mobile ? screen(page).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
async function checkBox(page, label) { if (mobile) await screen(page).getByText(label, { exact: true }).last().click(); else await page.getByRole('checkbox', { name: label, exact: true }).check(); }
async function settle(page) { await stableMobile(page); await page.waitForTimeout(250); }
async function api(token, route, method = 'GET', body) {
  let r;
  if (typeof token === 'string') r = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method !== 'GET' ? { 'Idempotency-Key': 'miniapp-identity-browser:' + randomUUID() } : {}) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  else {
    const csrf = (await token.cookies(fixture.origin)).find(c => c.name === 'cp_csrf')?.value;
    const response = await token.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'Content-Type': 'application/json', ...(method !== 'GET' ? { 'Idempotency-Key': 'miniapp-identity-browser:' + randomUUID(), 'X-CSRF-Token': decodeURIComponent(csrf ?? '') } : {}) }, ...(body === undefined ? {} : { data: body }) });
    r = { ok: response.ok(), status: response.status(), json: () => response.json() };
  }
  const value = await r.json(); assert.ok(r.ok, route + ': ' + r.status + ' ' + value.error?.code); return value.data;
}
async function drain() { const r = await fetch(fixture.api_origin + '/__fixture/miniapp-identity/drain', { method: 'POST', headers: { Authorization: 'Bearer ' + fixture.stop_token } }); assert.equal(r.status, 204); }
async function login(account) {
  const context = await browser.newContext({ viewport: mobile ? { width: 390, height: 900 } : { width: 1365, height: 900 }, reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', e => errors.push(e.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ path: new URL(r.url()).pathname, status: r.status() }); });
  await page.goto(fixture.origin);
  if (mobile) { await field(page, '内部账号').fill(account.login); await field(page, '密码').fill(account.password); }
  else { await page.locator('input[name="login"]').fill(account.login); await page.locator('input[name="password"]').fill(account.password); }
  const logged = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/auth/login' && r.request().method() === 'POST'); await button(page, '登录工作空间').click(); let result = await (await logged).json();
  if (account.mfa_secret) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(300);
    const step = Math.floor(Date.now() / 30000), code = otp(account.mfa_secret, step);
    if (mobile) await screen(page).locator('input[maxlength="6"]').fill(code); else await page.locator('input[name="code"]').fill(code);
    const verified = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/auth/mfa/verify' && r.request().method() === 'POST'); await button(page, '验证并继续').click(); result = await (await verified).json(); account.last_step = step;
  }
  assert.equal(result.data?.step, 'authenticated'); await screen(page).getByText('工作从这里开始', { exact: false }).first().waitFor().catch(async () => { if (!mobile) await page.getByRole('main').waitFor(); else throw new Error('Mobile login did not reach Home'); });
  return { page, context, token: mobile ? result.data.access_token : context };
}

async function readBindings(actor) {
 const response = mobile ? await fetch(fixture.api_origin + '/v1/me/miniapp-bindings', { headers: { Authorization: 'Bearer ' + actor.token } }) : await actor.context.request.get(fixture.origin + '/v1/me/miniapp-bindings');
 assert.equal(typeof response.status === 'function' ? response.status() : response.status, 200);
 return (await response.json()).data.items;
}
async function openSecurity(page) {
 if (mobile) await page.evaluate(() => { window.location.hash = '#/pages/settings/index'; });
 else await page.goto(fixture.origin + '/account/security');
 await screen(page).getByText('我的小程序绑定', { exact: true }).waitFor();
 await screen(page).getByText('wx0123456789abcdef', { exact: true }).waitFor(); await settle(page);
}
async function confirm(page, label) {
 if (mobile) await page.getByText(label, { exact: true }).last().click();
 else await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click();
}
async function screenshot(page, name) {
 const heading = screen(page).getByText('我的小程序绑定', { exact: true }); await heading.evaluate(el => el.scrollIntoView({ block:'start' })); await settle(page);
 const file = name + '.png', output = path.join(evidence, file); await page.screenshot({ path: output, fullPage: false, caret: 'hide' });
 screenshots.push({ file, sha256: createHash('sha256').update(await readFile(output)).digest('hex'), viewport: page.viewportSize() });
 const size = await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth })); assert.ok(size.scroll <= size.width + 2); layouts.push({ name, ...size });
}
try {
 await mkdir(evidence, { recursive: true });
 browser = await chromium.launch({ executablePath, headless: true, args: ['--disable-gpu', '--no-first-run'] });
 const alice = await login(fixture.alice);
 await alice.page.setViewportSize(mobile ? { width:390,height:844 } : { width:1440,height:1000 });
 await openSecurity(alice.page); assert.equal((await readBindings(alice)).filter(item => !item.revoked_at).length, 2);
 if (mobile) assert.equal(await button(alice.page, '绑定当前小程序身份').count(), 0);
 checks.push('Actual React Web or Taro H5 reads two current self-owned synthetic bindings from the native API; H5 cannot produce platform codes.');
 await screenshot(alice.page, 'bindings-light');
 await alice.page.emulateMedia({ colorScheme: 'dark' }); await screenshot(alice.page, 'bindings-dark');
 await alice.page.emulateMedia({ colorScheme: 'light' });
 const first = (await readBindings(alice))[0];
 recordings.push(await recordViewport(alice.page, app, group + '-collaboration', async () => {
  await button(alice.page, '解除绑定').first().click(); await confirm(alice.page, '取消');
  assert.equal((await readBindings(alice)).filter(item => !item.revoked_at).length, 2);
  checks.push('Explicit cancellation leaves both bindings active.');
  let committed = false, interceptionFailed = false, finishInterception;
  const interceptionDone = new Promise(resolve => { finishInterception = resolve; });
  await alice.page.route('**/v1/me/miniapp-bindings/' + first.id + '/revoke', async route => {
    try {
      const response = await route.fetch({ timeout: 30000 });
      assert.equal(response.status(), 200); committed = true; await route.abort('failed');
    } catch {
      interceptionFailed = true;
      try { await route.abort('failed'); } catch { /* request may already be disposed */ }
    } finally { finishInterception(); }
  }, { times: 1 });
  await button(alice.page, '解除绑定').first().click(); await confirm(alice.page, '确认解绑');
  let interceptionTimeout;
  try {
    await Promise.race([interceptionDone, new Promise((resolve, reject) => {
      interceptionTimeout = setTimeout(() => reject(new Error('Controlled revocation interception did not complete.')), 30000);
    })]);
  } finally { clearTimeout(interceptionTimeout); }
  assert.equal(interceptionFailed, false, 'Controlled revocation interception failed.'); assert.ok(committed);
  const stored = (await readBindings(alice)).find(item => item.id === first.id); assert.ok(stored.revoked_at); assert.equal(stored.version, 2);
  if (mobile) {
    await screen(alice.page).getByText('操作结果尚未核对。请刷新绑定列表确认，勿重复提交。', { exact: true }).waitFor();
    assert.ok(await button(alice.page, '解除绑定').first().isDisabled());
    await button(alice.page, '刷新绑定').click();
  } else {
    await alice.page.getByRole('dialog').getByRole('button', { name: '确认解绑', exact: true }).click();
    await alice.page.getByRole('dialog').waitFor({ state: 'hidden' });
  }
  await screen(alice.page).getByText(/^已解绑/).first().waitFor(); await settle(alice.page);
  checks.push('A real committed revocation with a deliberately lost response is reconciled by a stored-key replay on Web or an explicit list refresh on Taro.');
  assert.equal((await readBindings(alice)).filter(item => !item.revoked_at).length, 1);
 }));
 await screenshot(alice.page, 'bindings-revoked');
 if (!mobile) { await alice.page.setViewportSize({ width: 390, height: 844 }); await screenshot(alice.page, 'bindings-narrow'); }
 checks.push('Revocation history remains visible, all captured credential inputs stay empty, and layouts fit the viewport.');
 passed = true;
} catch (cause) {
 failure = cause.stack || String(cause);
 for (const account of [fixture.alice, fixture.bob, fixture.administrator]) for (const key of ['password', 'mfa_secret']) if (account?.[key]) failure = failure.replaceAll(account[key], '[redacted]');
 process.stderr.write(failure + '\n');
} finally {
 for (const context of contexts) { try { await context.close(); contextsClosed++; } catch { /* report cleanup mismatch */ } }
 if (browser) await browser.close(); const after = await digest(), closed = !browser || !browser.isConnected();
 const result = { passed: passed && before === after && closed && contextsClosed === contexts.length && !errors.length && !serverErrors.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, screenshots, recordings, errors, server_errors: serverErrors, failure, owned_browser_closed: closed, browser_contexts_created: contexts.length, browser_contexts_closed: contextsClosed, real_native_api: true, real_cloud: false, real_model: false, real_device: false, real_telegram_network: false, real_wechat_or_douyin_exchange: false, client: mobile ? 'Taro H5' : 'React Web', controlled_response_faults: ['committed mini-program binding revocation response lost'] };
 await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
