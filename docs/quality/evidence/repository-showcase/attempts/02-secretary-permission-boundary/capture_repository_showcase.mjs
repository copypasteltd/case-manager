import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, stableMobile as stable } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './repository_recording.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const target = process.argv[2]; assert.ok(['web', 'mobile'].includes(target));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.isolated_database, true); assert.ok(fixture.contract_id);
const evidence = path.join(app, '../docs/quality/evidence/repository-showcase');
const media = path.join(app, '../docs/media/screenshots');
const privateDirectory = path.join(app, '.local/repository-showcase');
await mkdir(media, { recursive: true }); await mkdir(privateDirectory, { recursive: true });
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const contexts = [], recordings = [], screenshots = [], events = [], errors = [], cleanupErrors = [], checks = [];
const secrets = [fixture.stop_token, fixture.alice.password, fixture.alice.mfa_secret].filter(Boolean);
const clean = value => secrets.reduce((text, secret) => text.replaceAll(secret, '[redacted]'), String(value));
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
await access(executablePath);
let browser, passed = false, failure;
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, label) => screen(page).locator('input[aria-label="' + label + '"]');

async function login() {
  const mobile = target === 'mobile';
  const context = await browser.newContext({ viewport: { width: mobile ? 390 : 1440, height: 900 }, colorScheme: 'light', reducedMotion: 'reduce' });
  contexts.push(context); const page = await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(clean(error.message)));
  page.on('response', response => {
    const pathname = new URL(response.url()).pathname;
    if (pathname.startsWith('/v1/') && !pathname.startsWith('/v1/auth/')) events.push({ method: response.request().method(), path: pathname, status: response.status() });
  });
  await page.goto(fixture.origin);
  const label = text => mobile ? field(page, text) : page.getByLabel(text, { exact: true });
  await label('内部账号').fill(fixture.alice.login); await label('密码').fill(fixture.alice.password);
  await (mobile ? button(page, '登录工作空间 ↗') : page.getByRole('button', { name: '登录工作空间', exact: true })).click();
  const last = Math.max(fixture.alice.last_step, Math.floor(Date.now() / 30000));
  while (Math.floor(Date.now() / 30000) <= last) await page.waitForTimeout(300);
  const code = otp(fixture.alice.mfa_secret, Math.floor(Date.now() / 30000)); secrets.push(code);
  await (mobile ? screen(page).locator('input[maxlength="6"]') : page.getByLabel('六位验证码', { exact: true })).fill(code);
  const result = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/auth/mfa/verify' && r.request().method() === 'POST');
  await (mobile ? button(page, '验证并继续 ↗') : page.getByRole('button', { name: '验证并继续', exact: true })).click();
  const body = (await (await result).json()).data;
  assert.equal(body.step, 'authenticated'); if (body.access_token) secrets.push(body.access_token);
  await (mobile ? screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }) : page.getByRole('heading', { name: /工作从这里开始/ })).waitFor();
  await assertNoCredentials(page); return { page, context, token: body.access_token };
}

async function api(session, route, method = 'GET', body) {
  let response;
  if (target === 'mobile') {
    response = await fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + session.token,
      'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'showcase:' + randomUUID() }) }, ...(body ? { body: JSON.stringify(body) } : {}) });
    const result = await response.json(); assert.ok(response.ok, `${method} ${route}: ${response.status} ${result.error?.code ?? ''}`); return result.data;
  }
  const cookies = await session.context.cookies(fixture.origin), csrf = cookies.find(c => c.name === 'cp_csrf')?.value;
  response = await session.context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin,
    'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'X-CSRF-Token': decodeURIComponent(csrf ?? ''), 'Idempotency-Key': randomUUID() }) }, ...(body ? { data: body } : {}) });
  const result = await response.json(); assert.ok(response.ok(), `${method} ${route}: ${response.status()} ${result.error?.code ?? ''}`); return result.data;
}

async function shot(page, name, caption) {
  await page.evaluate(() => document.fonts.ready); await page.waitForTimeout(350); await assertNoCredentials(page);
  const file = name + '.png', bytes = await page.screenshot({ path: path.join(media, file), caret: 'hide' });
  screenshots.push({ file: 'docs/media/screenshots/' + file, caption, viewport: page.viewportSize(), sha256: createHash('sha256').update(bytes).digest('hex'), synthetic_data: true });
}

async function webShowcase(session) {
  const { page } = session, name = '示例 · 品牌官网交付';
  const me = await api(session, '/me'); await shot(page, 'web-dashboard', '实际Web工作台；隔离合成数据 / Actual Web dashboard with synthetic data.');
  let pid;
  recordings.push(await recordViewport(page, app, 'web-project-flow', async () => {
    await page.getByRole('link', { name: '项目空间', exact: true }).click();
    await page.getByRole('button', { name: '新建项目', exact: true }).first().click();
    let dialog = page.getByRole('dialog', { name: '新建项目', exact: true });
    await dialog.getByLabel('项目名称', { exact: true }).fill(name);
    await dialog.getByLabel('项目范围与目标', { exact: true }).fill('合成演示：明确交付目标，拆分任务并保留实际完成依据。');
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/projects' && r.request().method() === 'POST');
    await dialog.getByRole('button', { name: '创建项目', exact: true }).click();
    const response = await created; assert.equal(response.status(), 201); const project = (await response.json()).data; pid = project.id;
    await dialog.waitFor({ state: 'hidden' }); await page.getByRole('link', { name: '查看项目 ' + name, exact: true }).click();
    await page.getByRole('heading', { name, exact: true }).waitFor(); await shot(page, 'web-project', '实际项目资料和协作入口 / Actual project details and collaboration.');
    await page.getByRole('tab', { name: '任务', exact: true }).click(); await page.getByRole('button', { name: '新建任务', exact: true }).first().click();
    dialog = page.getByRole('dialog', { name: '新建任务', exact: true });
    const title = '确认交付范围与验收依据'; await dialog.getByLabel('任务名称', { exact: true }).fill(title);
    await dialog.getByLabel('描述与验收标准', { exact: true }).fill('合成演示：确认交付清单，并记录完成依据。');
    await dialog.getByLabel('负责人', { exact: true }).selectOption({ label: me.user.display_name });
    await dialog.getByLabel('优先级', { exact: true }).selectOption('high');
    await dialog.getByLabel('开始时间（香港时间）', { exact: true }).fill('2028-05-01T09:00');
    await dialog.getByLabel('截止时间（香港时间）', { exact: true }).fill('2028-05-03T18:00');
    await dialog.getByRole('button', { name: '创建任务', exact: true }).click(); await dialog.waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: title, exact: true }).click();
    dialog = page.getByRole('dialog', { name: '任务详情', exact: true });
    for (const label of ['开始任务', '完成任务']) {
      await dialog.getByRole('button', { name: label, exact: true }).click();
      await page.getByLabel('变更原因', { exact: true }).fill('合成演示：' + label + '，按实际事项记录。');
      await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click();
      await dialog.getByText(label === '开始任务' ? '进行中' : '已完成', { exact: true }).waitFor();
    }
    await shot(page, 'web-task', '真实创建、开始和完成的任务 / A task created, started and completed through real commands.');
    await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click();
    await page.getByRole('button', { name: '看板', exact: true }).click(); await page.getByRole('region', { name: '项目任务看板', exact: true }).waitFor();
    await shot(page, 'web-project-board', '任务看板显示原业务状态 / Task board with actual business state.');
    assert.equal((await api(session, '/projects/' + pid + '/tasks')).items.find(t => t.title === title).state, 'done');
  }));
  checks.push('Real Web UI creates a project and a task, starts/completes it and renders the actual board.');
  await writeFile(path.join(privateDirectory, 'project.json'), JSON.stringify({ project_id: pid, name }) + '\n');
  recordings.push(await recordViewport(page, app, 'web-operations-tour', async () => {
    const views = [
      ['/projects/' + fixture.project_id + '?tab=overview', 'web-overview', '项目16类业务聚合；合成金额用于精度核验 / Sixteen-domain project overview with synthetic precision fixtures.'],
      ['/contracts/' + fixture.contract_id, 'web-contract', '合同、固定业务修订及执行入口 / Contract revisions and execution entry.'],
      ['/costs', 'web-finance', '实际费用台账；合成数据 / Actual-cost ledger with synthetic data.'],
      ['/files', 'web-files', '文件资料界面；元数据夹具不代表真实云传输 / File metadata interface; no real cloud transfer is implied.'],
      ['/knowledge', 'web-knowledge', '知识与项目记录入口 / Knowledge and project records.'],
      ['/conversations', 'web-secretary', '秘书会话入口；真实模型未启用 / Secretary conversation entry; real model is disabled.'],
      ['/services', 'web-status', '服务配置与状态入口；观测为合成夹具 / Service/status interface with synthetic observations.'],
    ];
    for (const [route, filename, caption] of views) {
      await page.goto(fixture.origin + route); await page.locator('main h1').first().waitFor();
      await page.waitForTimeout(800); await shot(page, filename, caption); await page.waitForTimeout(500);
    }
  }));
  checks.push('Current authenticated Web pages render project, contract, finance, files, knowledge, secretary and service interfaces without model/cloud activation.');
}

async function mobileShowcase(session) {
  const { page } = session, project = JSON.parse(await readFile(path.join(privateDirectory, 'project.json'), 'utf8'));
  const heading = text => screen(page).locator('.cp-title').filter({ hasText: text });
  async function transition(item, kind, label, reason, resolution) {
    await button(page, label).click(); await field(page, '事项变更原因').fill(reason);
    if (resolution) await screen(page).locator('textarea[aria-label="本次解决说明"]').fill(resolution);
    const changed = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/' + kind + '/' + item.id && r.request().method() === 'PATCH');
    await button(page, '确认' + label).click(); const response = await changed; assert.equal(response.status(), 200);
    const value = (await response.json()).data; await screen(page).getByText(new RegExp('^修订 ' + value.version + ' ·')).waitFor(); return value;
  }
  await page.evaluate(pid => { window.location.hash = '#/pages/detail/index?kind=project&id=' + pid; }, project.project_id);
  await heading(project.name).waitFor(); await stable(page); await shot(page, 'mobile-project', 'Taro共享移动界面，H5实际录制；非真机 / Shared Taro UI recorded in H5, not on a device.');
  recordings.push(await recordViewport(page, app, 'mobile-risk-issue', async () => {
    await screen(page).locator('.cp-row-title').getByText('风险', { exact: true }).click(); await heading('项目风险').waitFor();
    await button(page, '新建风险').click(); await field(page, '风险标题').fill('示例 · 第三方交付延期');
    await screen(page).locator('textarea[aria-label="情况说明"]').fill('合成演示：记录可能影响交付的依赖事项。');
    await screen(page).locator('textarea[aria-label="应对措施"]').fill('明确替代方案，并联系责任人核对交付时间。');
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/projects/' + project.project_id + '/risks' && r.request().method() === 'POST');
    await button(page, '确认保存风险').click(); const response = await created; assert.equal(response.status(), 201); let risk = (await response.json()).data;
    await button(page, '查看已保存事项').click(); await heading(risk.title).waitFor(); await screen(page).getByText('明确替代方案，并联系责任人核对交付时间。', { exact: true }).waitFor();
    await shot(page, 'mobile-risk', '实际新建风险及应对措施 / Actual risk creation and mitigation.');
    risk = await transition(risk, 'risks', '风险已发生，转为问题', '合成演示：依赖延期已经发生，按原流程转为问题。'); assert.equal(risk.state, 'realized'); assert.ok(risk.issue_id);
    await button(page, '查看已发生的问题').click(); let issue = await api(session, '/issues/' + risk.issue_id); await heading(issue.title).waitFor();
    issue = await transition(issue, 'issues', '开始或继续调查', '合成演示：与责任人核对实际影响。');
    const resolution = '合成演示：替代交付已完成，并核对最终结果。';
    issue = await transition(issue, 'issues', '标记问题已解决', '合成演示：完成纠正措施并核实。', resolution);
    issue = await transition(issue, 'issues', '关闭事项', '合成演示：保留实际解决说明后关闭。', resolution); assert.equal(issue.state, 'closed');
    await shot(page, 'mobile-issue', '真实关联问题调查、解决及关闭 / Actual linked issue investigated, resolved and closed.');
    assert.equal((await api(session, '/issues/' + issue.id)).resolution, resolution);
  }));
  checks.push('Real Taro UI creates a risk, realizes it into the actual linked issue and investigates/resolves/closes that issue.');
}

try {
  browser = await chromium.launch({ executablePath, headless: true }); const session = await login();
  if (target === 'web') await webShowcase(session); else await mobileShowcase(session);
  assert.deepEqual(errors, []); assert.ok(!events.some(event => event.status >= 500)); passed = true;
} catch (error) { failure = clean(error.stack ?? error); process.stderr.write(failure + '\n'); }
finally {
  for (const context of contexts) { try { await context.close(); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const closed = !browser || !browser.isConnected();
  const report = { passed: passed && closed && !cleanupErrors.length, target, checked_at: new Date().toISOString(),
    checks, recordings, screenshots, events, errors, cleanup_errors: cleanupErrors, failure,
    owned_browser_closed: closed, contexts_closed: contexts.length, credentials_excluded: true,
    synthetic_data: true, real_model: false, real_cloud: false, real_device: false };
  await writeFile(path.join(evidence, target + '.json'), clean(JSON.stringify(report, null, 2)) + '\n');
  console.log(JSON.stringify({ passed: report.passed, target, recordings: recordings.length, screenshots: screenshots.length, closed }));
  if (!report.passed) process.exitCode = 1;
}
