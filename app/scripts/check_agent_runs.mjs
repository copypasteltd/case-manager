// Real UI/API proof against an explicitly held, disposable native PG fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'agent-context-and-classification');
assert.equal(fixture.origin, 'http://localhost:5189'); assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1');
assert.equal(fixture.controlled_provider, true); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const group = process.env.CP_BROWSER_EVIDENCE_GROUP ?? 'secretary-runs';
assert.match(group, /^[a-zA-Z0-9-]+$/);
const evidence = path.join(root, 'docs/quality/evidence', group), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const sourceFiles = ['backend/internal/agent/runs.go', 'backend/internal/agent/run_reads.go', 'backend/internal/agent/run_cancel.go', 'backend/internal/httpapi/agent_runs.go', 'backend/internal/httpapi/agent_context_integration_test.go', 'backend/internal/httpapi/agent_runs_integration_test.go', 'backend/migrations/0047_agent_runs_and_messages.sql', 'backend/migrations/0048_agent_execution_and_receipts.sql', 'backend/internal/agent/execution.go', 'backend/internal/jobqueue/queue.go', 'web/src/ConversationMessages.tsx', 'web/src/ConversationSources.tsx', 'web/src/Conversations.tsx', 'web/src/App.tsx', 'packages/contracts/src/api.d.ts', 'scripts/check_agent_runs.mjs', 'scripts/run_agent_browser.py'];
async function digest() { const hash = createHash('sha256'); for (const file of sourceFiles) { hash.update(file + '\n'); hash.update(await readFile(path.join(app, file))); } return hash.digest('hex'); }
const before = await digest(), started = new Date().toISOString();
const contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map(); let browser, passed = false;
function otp(secret, step) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join('');
  const key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step));
  const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0');
}
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(25000); page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) {
    await page.getByLabel('六位验证码').waitFor(); const previous = lastSteps.get(account.login) ?? account.last_step;
    if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 300));
    const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); await page.getByLabel('六位验证码').fill(otp(account.mfa_secret, step)); await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { page, context };
}
async function request(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const response = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  const result = await response.json(); assert(response.ok(), route + ' ' + response.status() + ' ' + result.error?.code); return result.data;
}
async function preview(page, conversation, type) {
  await page.goto(fixture.origin + '/conversations/' + conversation);
  const section = page.getByRole('region', { name: '准备给秘书的资料', exact: true });
  await section.getByLabel('资料类型', { exact: true }).selectOption(type);
  await section.getByLabel('选择资料', { exact: true }).selectOption(type === 'post' ? fixture.post_id : fixture.project_id);
  await section.getByRole('button', { name: '核对并预览', exact: true }).click();
  return section;
}
async function command(page, route, label, expected = 200) {
  const [response] = await Promise.all([page.waitForResponse(value => new URL(value.url()).pathname === '/v1' + route && value.request().method() === 'POST'), page.getByRole('button', { name: label, exact: true }).click()]);
  const result = await response.json(); assert.equal(response.status(), expected, result.error?.code); return result.data;
}
async function cancelRun(page, run) {
  await page.getByLabel('取消任务的原因', { exact: true }).fill('核对实际受理后取消');
  const cancelled = await command(page, '/runs/' + run.id + '/cancel', '确认请求取消');
  assert.equal(cancelled.state, 'cancelled'); assert.equal(cancelled.version, run.version + 1);
  await page.getByText('已确认取消。已提交的历史业务事实仍保留。', { exact: true }).waitFor();
  await page.getByRole('button', { name: '收起任务进度', exact: true }).click();
}
let phase = 'starting', activePage, abortedResponses = 0;
try {
  assert.ok(fixture.run_profile_id);
  browser = await chromium.launch({ executablePath, headless: true });
  const alice = await login(fixture.alice), bob = await login(fixture.bob), page = alice.page; activePage = page;
  const c = await request(alice.context, '/conversations/' + fixture.conversation_id), unique = crypto.randomUUID().slice(0, 8);
  phase = 'fixed source submission';
  const section = await preview(page, c.id, 'post'); await section.locator('.conversation-source-preview').waitFor();
  assert.equal(await section.locator('pre').innerText(), fixture.published_body);
  await section.getByRole('button', { name: '将这份准确资料加入本次请求', exact: true }).click();
  await page.getByLabel('本次秘书任务', { exact: true }).selectOption(fixture.run_profile_id);
  const firstText = '依据固定知识整理项目简报 ' + unique;
  await page.getByLabel('本次消息内容', { exact: true }).fill(firstText);
  const first = await command(page, '/conversations/' + c.id + '/runs', '确认发送本次请求', 202); assert.equal(first.state, 'queued');
  const messages = await request(alice.context, '/conversations/' + c.id + '/messages');
  assert.equal(messages.items.length, 1); assert.equal(messages.items[0].body, firstText);
  assert.equal(messages.items[0].source_refs[0].id, fixture.post_id); assert.ok(messages.items[0].source_refs[0].version_id);
  assert.equal(await page.getByLabel('本次消息内容', { exact: true }).inputValue(), '');
  checks.push('Web submits a real queued run with a fixed published source and clears text only after a matching receipt');

  phase = 'shared reader';
  await bob.page.goto(fixture.origin + '/conversations/' + c.id);
  await bob.page.getByText(firstText, { exact: true }).waitFor();
  await bob.page.getByRole('button', { name: '查看任务进度', exact: true }).click();
  await bob.page.getByRole('heading', { name: '任务当前进展', exact: true }).waitFor();
  assert.equal(await bob.page.getByLabel('取消任务的原因', { exact: true }).count(), 0);
  assert.equal((await request(bob.context, '/runs/' + first.id)).can_cancel, false);
  await cancelRun(page, first);
  checks.push('A shared reader sees persisted messages and task events while only the requester can cancel');

  phase = 'lost response and navigation reconciliation';
  const uncertainText = '已保存但回包中断的请求 ' + unique; await page.getByLabel('本次消息内容', { exact: true }).fill(uncertainText);
  let committed, sentKey, retriedKey;
  const pattern = '**/v1/conversations/' + c.id + '/runs';
  await page.route(pattern, async route => { sentKey = route.request().headers()['idempotency-key']; const response = await route.fetch(); assert.equal(response.status(), 202); committed = (await response.json()).data; abortedResponses++; await route.abort('failed'); });
  await page.getByRole('button', { name: '确认发送本次请求', exact: true }).click();
  await page.getByRole('button', { name: '允许以原请求继续核对', exact: true }).waitFor(); assert.ok(committed); await page.unroute(pattern);
  assert.equal(await page.getByLabel('本次消息内容', { exact: true }).isDisabled(), true);
  await page.locator('a[href="/conversations"]').first().click();
  await page.locator('a[href="/conversations/' + c.id + '"]').first().click();
  await page.getByRole('button', { name: '允许以原请求继续核对', exact: true }).waitFor();
  assert.equal(await page.getByLabel('本次消息内容', { exact: true }).inputValue(), uncertainText);
  await page.getByRole('button', { name: '允许以原请求继续核对', exact: true }).click();
  const recordRetry = value => { if (new URL(value.url()).pathname === '/v1/conversations/' + c.id + '/runs' && value.method() === 'POST') retriedKey = value.headers()['idempotency-key']; }; page.on('request', recordRetry);
  const reconciled = await command(page, '/conversations/' + c.id + '/runs', '确认发送本次请求', 202);
  page.off('request', recordRetry); assert.equal(reconciled.id, committed.id); assert.equal(retriedKey, sentKey);
  const saved = await request(alice.context, '/conversations/' + c.id + '/messages'); assert.equal(saved.items.filter(value => value.body === uncertainText).length, 1);
  await cancelRun(page, reconciled);
  checks.push('A lost response remains a bounded memory-only pending intent across navigation; explicit retry uses the original key and creates no duplicate');

  phase = 'current member withdrawal';
  const me = await request(bob.context, '/me');
  await page.getByRole('button', { name: '管理会话成员', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '管理会话成员', exact: true });
  await dialog.getByLabel('成员操作', { exact: true }).selectOption('remove'); await dialog.getByLabel('选择成员', { exact: true }).selectOption(me.user.id);
  await dialog.getByLabel('变更原因', { exact: true }).fill('核对移出后不再读取消息');
  await command(page, '/conversations/' + c.id + '/members', '确认移出会话');
  await bob.page.getByRole('alert').first().waitFor(); await bob.page.getByText(firstText, { exact: true }).waitFor({ state: 'hidden' }); assert.equal(await bob.page.getByText(firstText, { exact: true }).count(), 0);
  assert.equal(await bob.page.getByText(uncertainText, { exact: true }).count(), 0);
  checks.push('Removing a member through Web causes their already open page to clear message and run facts after current authorization is checked');
  await page.screenshot({ path: path.join(evidence, 'conversation-after-real-commands.png'), fullPage: true });
  assert.deepEqual(errors, []); assert.deepEqual(serverErrors, []); passed = true;
} finally {
  if (!passed && activePage) { try { await activePage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true }); } catch {} }
  let closed = true; for (const context of contexts) { try { await context.close(); } catch { closed = false; } }
  if (browser) { try { await browser.close(); } catch { closed = false; } }
  const after = await digest(), result = { passed: passed && closed && before === after, phase, started_at: started, completed_at: new Date().toISOString(), real_api: true, isolated_native_postgresql: true, actual_model_or_cloud: false, source_files: sourceFiles, source_sha256: before, source_unchanged: before === after, checks, javascript_errors: errors, server_errors: serverErrors, intentionally_aborted_commit_responses: abortedResponses, all_browser_contexts_closed: closed };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, phase, checks: checks.length, all_browser_contexts_closed: closed })); if (!result.passed) process.exitCode = 1;
}
