import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './token_budget_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const select = (value, label) => mobile ? selectConcernMobile(page, '账本范围', label) : field('账本范围').selectOption(value);
  const responses = []; page.on('response', r => { const path = new URL(r.url()).pathname; if (path.startsWith('/v1/')) responses.push({ path, status: r.status() }); });
  let bearer; page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  fixture.alice.last_step = Math.floor(Date.now() / 30000);
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  async function mobileStepUp() {
    if (!mobile) return;
    await field('验证器六位验证码').fill(await waitOTP(fixture));
    await button('确认本次操作').click();
    await button('确认本次操作').waitFor({ state: 'hidden' });
    await writeFile(path.join(app, '.local/token-budgets-mobile-diagnostic.json'), JSON.stringify({ stage: 'mobile MFA confirmed in UI', responses }, null, 2));
    fixture.alice.last_step = Math.floor(Date.now() / 30000);
  }
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {}); assert.equal(response.status(), 200); return (await response.json()).data; }
  const url = fixture.origin + (mobile ? '/#/pages/budget/index' : '/agent-budget');
  await page.goto(url); await button('查询账本').waitFor();
  await select('organization', '整个组织'); await button('查询账本').click();
  await button('核对用量').waitFor(); await assertNoCredentials(page);
  const before = await get('/agent-budget?scope=organization'); assert.equal(before.pending_tokens, 11000); assert.equal(before.unbudgeted_runs, 0);
  checks.push('Real MFA login loads the organization ledger with a conservative pending charge and no invented historical usage.');
  const media = path.join(app, '../docs/media/screenshots'), prefix = 'secretary-token-budgets-' + (mobile ? 'mobile' : 'web');
  await page.screenshot({ path: path.join(media, prefix + '-ledger.png'), fullPage: false });
  await button('配置月度额度').click(); await field('组织月度额度').fill('1000000100'); await field('发布原因').fill('Synthetic browser budget revision 86');
  await mobileStepUp();
  await button('确认发布').click(); await button('确认发布').waitFor({ state: 'hidden' }); await button('查询账本').waitFor();
  const revised = await get('/agent-budget?scope=organization'); assert.equal(revised.policy.organization_tokens, 1000000100); assert.equal(revised.policy.version, before.policy.version + 1);
  checks.push('Publishing all four explicit monthly allowances creates a real policy revision with an audit reason; already admitted usage remains.');
  await button('核对用量').click();
  await field('实际输入Token').fill('70'); await field('实际输出Token').fill('10'); await field('核对凭据编号').fill('SYNTHETIC-BROWSER-86'); await field('核对原因').fill('Controlled usage evidence, no real provider request');
  await mobileStepUp();
  await button('确认核对').click(); await button('确认核对').waitFor({ state: 'hidden' }); await button('查询账本').waitFor();
  const entries = await get('/agent-budget/entries?scope=organization&limit=100'), reconciled = entries.items.find(e => e.id === fixture.pending_run_id);
  assert.equal(reconciled.status, 'reconciled'); assert.equal(reconciled.charged_tokens, 80); assert.equal(reconciled.pending_tokens, 0); assert.equal(reconciled.evidence_reference, 'SYNTHETIC-BROWSER-86');
  const after = await get('/agent-budget?scope=organization'); assert.equal(after.pending_tokens, 0); assert.equal(before.charged_tokens - after.charged_tokens, 10920);
  checks.push('Human reconciliation commits exact known tokens and a reference, releases the unused estimate and updates organization totals.');
  await assertNoCredentials(page);
  await page.screenshot({ path: path.join(media, prefix + '-reconciled.png'), fullPage: false });
  const recordings = [await recordViewport(page, app, 'token-budgets-' + (mobile ? 'mobile' : 'web'), async () => {
    await select('mine', '本人'); await button('查询账本').click(); await stableMobile(page); await page.waitForTimeout(600);
    await select('organization', '整个组织'); await button('查询账本').click(); await stableMobile(page); await page.waitForTimeout(600);
  })];
  const layouts = [];
  for (const width of [390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1100 }); await stableMobile(page);
    const layout = await root().locator(mobile ? '.cp-page' : '#main-content').evaluate(node => ({ width: node.getBoundingClientRect().width, pageWidth: document.documentElement.clientWidth, scrollWidth: document.documentElement.scrollWidth }));
    assert.ok(layout.scrollWidth <= width + 2, 'Page must keep wide records inside their own scroll container.');
    layouts.push({ viewport_width: width, ...layout });
  }
  checks.push('The actual ledger stays usable at 390, 768 and 1440 pixels.');
  await select('project', '指定项目'); await field('范围标识').fill(fixture.project_id); await button('查询账本').click(); await stableMobile(page);
  const project = await get('/agent-budget?scope=project&scope_id=' + fixture.project_id); assert.equal(project.scope_id, fixture.project_id); assert.ok(project.charged_tokens > 0);
  checks.push('Project-filtered reporting uses the original project account, rather than only the current conversation.');
  await select('mine', '本人'); await button('查询账本').click(); await stableMobile(page);
  const own = await get('/agent-budget?scope=mine'); assert.equal(own.scope, 'user'); assert.ok(own.scope_id);
  checks.push('Returning to the own-user account preserves access to an independent user quota.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ mobile, recordings, layouts, synthetic_data: true,
    scope: 'Authenticated Web/Taro H5 and native PostgreSQL token budgets; synthetic controller usage only, no provider invoice or device acceptance.' }, null, 2) + '\n');
}, 'token-budgets');
