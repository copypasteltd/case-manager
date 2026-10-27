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
export async function notificationBrowser(mobile, action, variant = 'notifications') {
  const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
  assert.ok(['token-budgets','task-creation','project-concerns','deliverable-creation','deliverable-updates','milestone-creation','subscription-review','telegram-delivery','finance-summary','service-summary','telegram-dispatch','telegram-ingress','secretary-project-stages','notifications', 'account-security', 'secretary-organization','secretary-file-creates','secretary-oa-requests','secretary-post-writes'].includes(variant));
  assert.equal(variant === 'token-budgets' ? fixture.token_budget_fixture : variant === 'task-creation' ? fixture.task_creation_fixture : variant === 'project-concerns' ? fixture.project_concerns_fixture : variant === 'deliverable-creation' ? fixture.deliverable_creation_fixture : variant === 'deliverable-updates' ? fixture.deliverable_update_fixture : variant === 'milestone-creation' ? fixture.milestone_creation_fixture : variant === 'subscription-review' ? fixture.subscription_review_fixture : variant === 'telegram-delivery' ? fixture.telegram_delivery_fixture : variant === 'finance-summary' ? fixture.finance_summary_fixture : variant === 'service-summary' ? fixture.service_summary_fixture : variant === 'telegram-dispatch' ? fixture.telegram_dispatch_fixture : variant === 'telegram-ingress' ? fixture.telegram_ingress_fixture : variant === 'secretary-project-stages' ? fixture.project_stage_fixture : variant === 'secretary-post-writes' ? fixture.agent_post_write_fixture : variant === 'secretary-oa-requests' ? fixture.agent_oa_request_fixture : variant === 'secretary-file-creates' ? fixture.agent_file_create_fixture : variant === 'secretary-organization' ? fixture.organization_fixture : variant === 'account-security' ? fixture.account_security_fixture : fixture.notification_fixture, true); assert.equal(fixture.isolated_database, true);
  const group = process.env.CP_BROWSER_EVIDENCE_GROUP ?? variant + (mobile ? '-mobile' : '-web');
  assert.match(group, /^[a-zA-Z0-9-]+$/);
  const evidence = path.join(root, 'docs/quality/evidence', group), temporary = path.join(app, '.local/browser-temp');
  await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
  const backend = (await collect('backend')).filter(file => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
  const checker = variant === 'token-budgets' ? 'scripts/check_token_budget.mjs' : variant === 'task-creation' ? 'scripts/check_task_creation.mjs' : variant === 'project-concerns' ? 'scripts/check_project_concerns.mjs' : variant === 'deliverable-creation' ? (mobile ? 'scripts/check_mobile_deliverable_creation.mjs' : 'scripts/check_deliverable_creation.mjs') : variant === 'deliverable-updates' ? (mobile ? 'scripts/check_mobile_deliverable_updates.mjs' : 'scripts/check_deliverable_updates.mjs') : variant === 'milestone-creation' ? (mobile ? 'scripts/check_mobile_milestone_creation.mjs' : 'scripts/check_milestone_creation.mjs') : variant === 'subscription-review' ? (mobile ? 'scripts/check_mobile_subscription_review.mjs' : 'scripts/check_subscription_review.mjs') : variant === 'telegram-delivery' ? (mobile ? 'scripts/check_mobile_telegram_delivery.mjs' : 'scripts/check_telegram_delivery.mjs') : variant === 'finance-summary' ? (mobile ? 'scripts/check_mobile_finance_summary.mjs' : 'scripts/check_finance_summary.mjs') : variant === 'service-summary' ? (mobile ? 'scripts/check_mobile_service_summary.mjs' : 'scripts/check_service_summary.mjs') : variant === 'telegram-dispatch' ? (mobile ? 'scripts/check_mobile_telegram_dispatch.mjs' : 'scripts/check_telegram_dispatch.mjs') : variant === 'telegram-ingress' ? 'scripts/check_telegram_ingress.mjs' : variant === 'secretary-project-stages' ? (mobile ? 'scripts/check_mobile_project_stages.mjs' : 'scripts/check_project_stages.mjs') : variant === 'secretary-post-writes' ? (mobile ? 'scripts/check_mobile_post_writes.mjs' : 'scripts/check_post_writes.mjs') : variant === 'secretary-oa-requests' ? (mobile ? 'scripts/check_mobile_oa_requests.mjs' : 'scripts/check_oa_requests.mjs') : variant === 'secretary-file-creates' ? (mobile ? 'scripts/check_mobile_file_creates.mjs' : 'scripts/check_file_creates.mjs') : variant === 'secretary-organization' ? (mobile ? 'scripts/check_mobile_organization.mjs' : 'scripts/check_organization.mjs') : variant === 'account-security' ? (mobile ? 'scripts/check_mobile_account_security.mjs' : 'scripts/check_account_security.mjs') : (mobile ? 'scripts/check_mobile_notifications.mjs' : 'scripts/check_notifications.mjs');
  const sources = [...new Set([...backend, ...await collect(mobile ? 'mobile' : 'web'), ...await collect('packages/contracts'), ...await collect('packages/design'), 'package.json', 'package-lock.json', 'scripts/notification_browser_common.mjs', ...(variant === 'subscription-review' ? ['scripts/subscription_review_browser_common.mjs'] : []), checker, ...(variant === 'token-budgets' ? ['scripts/token_budget_recording.mjs','scripts/project_concern_picker.mjs','scripts/mobile_picker_browser.mjs'] : []), ...(variant === 'task-creation' ? ['scripts/task_creation_sources.py','scripts/task_creation_recording.mjs','scripts/project_concern_picker.mjs','scripts/mobile_picker_browser.mjs','scripts/run_task_creation_browser.py'] : variant === 'project-concerns' ? ['scripts/project_concern_sources.py','scripts/project_concern_recording.mjs', 'scripts/project_concern_picker.mjs','scripts/mobile_picker_browser.mjs','scripts/run_project_concerns_browser.py'] : [mobile ? 'scripts/run_mobile_browser.py' : 'scripts/run_agent_browser.py']), 'scripts/test_backend.py', 'scripts/build_openapi.py'])].sort();
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
