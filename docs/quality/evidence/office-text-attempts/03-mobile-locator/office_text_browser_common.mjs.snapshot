import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
export function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(c => alphabet.indexOf(c).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(v => parseInt(v, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest();
  return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function collect(directory) {
  const files = [];
  for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) {
    if (['node_modules', 'dist', '.taro', '.swc', '.gradle', 'build'].includes(entry.name) || (directory === 'mobile' && ['android', 'native'].includes(entry.name))) continue;
    if (entry.name.endsWith('.tsbuildinfo')) continue;
    const file = path.posix.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await collect(file)); else files.push(file);
  }
  return files;
}
export async function officeTextBrowser(mobile, action, variant = 'office-text') {
  const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
  assert.equal(variant, 'office-text');
  assert.equal(fixture.fixture_kind, 'agent-context-and-classification'); assert.equal(fixture.office_text_fixture,true);
  const group = process.env.CP_BROWSER_EVIDENCE_GROUP ?? variant + (mobile ? '-mobile' : '-web');
  assert.match(group, /^[a-zA-Z0-9-]+$/);
  const evidence = path.join(root, 'docs/quality/evidence', group), temporary = path.join(app, '.local/browser-temp');
  await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
  const backend = (await collect('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
  const checker = 'scripts/check_office_text.mjs';
  const sources = [...new Set([...backend, ...await collect(mobile ? 'mobile' : 'web'), ...await collect('packages/contracts'), ...await collect('packages/design'), 'package.json', 'package-lock.json', 'scripts/office_text_browser_common.mjs', checker, 'scripts/office_text_sources.py', 'scripts/run_office_text_browser.py', 'scripts/office_text_recording.mjs', 'scripts/mobile_picker_browser.mjs', 'scripts/project_concern_picker.mjs', 'scripts/test_backend.py', 'scripts/test_office_text_native.py', 'scripts/office_text_native_cluster.py', 'scripts/windows_native_path.py', 'scripts/build_openapi.py'])].sort();
  async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
  const before = await digest(), started = new Date().toISOString(), checks = [], errors = [], serverErrors = [], contexts = [];
  let browser, page, passed = false, closed = true, failure;
  try {
    const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
    browser = await chromium.launch({ executablePath, headless: true }); closed = false;
    const context = await browser.newContext({ viewport: mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 }, reducedMotion: 'reduce' }); contexts.push(context);
    page = await context.newPage(); page.setDefaultTimeout(25000);
    function observe(observed) { observed.setDefaultTimeout(25000); observed.on('pageerror', error => errors.push(error.message)); observed.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); }); } observe(page);
    await action({ page, context, fixture, checks, evidence, openPage: async (options = {}) => { const owned = await browser.newContext({ viewport: { width: 1440, height: 1100 }, reducedMotion: 'reduce', ...options }); contexts.push(owned); const opened = await owned.newPage(); observe(opened); return { page: opened, context: owned }; } });
    assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []);
    passed = true;
  } catch (error) { failure = error.message; }
  finally {
    if (page && !page.isClosed()) { try { await page.screenshot({ path: path.join(evidence, passed ? 'inbox.png' : 'failure.png'), fullPage: true, mask: [page.locator('input[type="password"],input[maxlength="6"],.cp-secret,[data-private-capability]')] }); } catch {} }
    for (const context of contexts) { try { await context.close(); } catch { closed = false; } }
    if (browser) { try { await browser.close(); closed = !browser.isConnected(); } catch { closed = false; } }
    const after = await digest();
    const result = { passed: passed && before === after && closed, started_at: started, finished_at: new Date().toISOString(), checks, source_count: sources.length, source_sha256: before, source_after_sha256: after, source_unchanged: before === after, browser_contexts_closed: contexts.length, owned_browser_closed: closed, console_errors: errors, server_errors: serverErrors, ...(failure ? { failure } : {}), scope: mobile ? 'Taro H5 with real native API; no device or store approval claim' : 'Web with real native API and committed business events' };
    await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n');
    await writeFile(path.join(evidence, 'source-manifest.json'), JSON.stringify({ files: sources, sha256: before }, null, 2) + '\n');
    console.log(JSON.stringify({ passed: result.passed, checks: checks.length, owned_browser_closed: closed }));
    if (!result.passed) process.exitCode = 1;
  }
}
export async function waitOTP(fixture) {
  if (Math.floor(Date.now() / 30000) <= fixture.alice.last_step) await new Promise(resolve => setTimeout(resolve, (fixture.alice.last_step + 1) * 30000 - Date.now() + 250));
  return otp(fixture.alice.mfa_secret, Math.floor(Date.now() / 30000));
}
