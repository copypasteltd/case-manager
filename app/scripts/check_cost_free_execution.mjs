import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './cost_free_execution_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './cost_free_execution_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const select = (value, label) => mobile ? selectConcernMobile(page, '账本范围', label) : field('账本范围').selectOption(value);
  const loaded = async () => { await root().getByText(fixture.pending_run_id, { exact: true }).waitFor(); await stableMobile(page); };
  let bearer; page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {}); assert.equal(response.status(), 200); return (await response.json()).data; }
  await page.goto(fixture.origin + (mobile ? '/#/pages/budget/index' : '/agent-budget')); await button('查询历史').waitFor();
  await select('organization', '整个组织'); await button('查询历史').click(); await loaded();
  const before = await get('/agent-budget?scope=organization'); assert.equal(before.pending_tokens, 11000); assert.ok(before.unbudgeted_runs > 0); assert.equal(before.policy.organization_tokens, 0);
  assert.ok((await (mobile ? root() : page.locator('body')).innerText()).includes('秘书接单与执行无需配置费用额度'));
  for (const label of ['配置月度额度', '配置秘书月度Token额度', '核对用量', '确认发布']) assert.equal(await root().getByRole('button', { name: label, exact: true }).count(), 0);
  checks.push('Authenticated real UI loads a read-only archive with the original zero legacy policy, pending history and new unreserved runs; fee write actions are absent.');
  await assertNoCredentials(page);
  const prefix = 'secretary-cost-free-' + (mobile ? 'mobile' : 'web');
  await page.screenshot({ path: path.join(app, '../docs/media/screenshots', prefix + '.png'), fullPage: false });
  const recordings = [await recordViewport(page, app, 'cost-free-execution-' + (mobile ? 'mobile' : 'web'), async () => {
    await select('mine', '本人'); await button('查询历史').click(); await loaded(); await page.waitForTimeout(700);
    await select('project', '指定项目'); await field('范围标识').fill(fixture.project_id); await button('查询历史').click(); await loaded(); await page.waitForTimeout(700);
    await select('organization', '整个组织'); await button('查询历史').click(); await loaded(); await page.waitForTimeout(700);
  })];
  const after = await get('/agent-budget?scope=organization'); assert.deepEqual(after, before);
  checks.push('Own, project and organization archive filters use real authorized requests without changing historical policies or entries.');
  const layouts = [];
  for (const width of [390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1100 }); await stableMobile(page);
    const layout = await (mobile ? root() : page.locator('#main-content')).evaluate(node => ({ width: node.getBoundingClientRect().width, pageWidth: document.documentElement.clientWidth, scrollWidth: document.documentElement.scrollWidth }));
    assert.ok(layout.scrollWidth <= width + 2, 'Keep wide history inside its scroll container.'); layouts.push({ viewport_width: width, ...layout });
  }
  checks.push('Read-only archive remains usable at390/768/1440 widths.');
  if (mobile) await button('前往公司秘书').click(); else await root().getByRole('link', { name: '前往公司秘书', exact: true }).click();
  await page.waitForURL(mobile ? /pages\/secretary\/index/ : /\/conversations/);
  checks.push('The archive return action navigates to the actual secretary page.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ mobile, recordings, layouts, synthetic_data: true,
    scope: 'Authenticated Web/Taro H5 read-only archive, synthetic native execution fixture. No provider, cloud, physical device or production acceptance.' }, null, 2) + '\n');
}, 'cost-free-execution');
