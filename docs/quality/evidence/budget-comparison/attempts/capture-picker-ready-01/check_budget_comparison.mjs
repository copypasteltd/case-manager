import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './budget_comparison_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = name => mobile ? root().locator('input[aria-label="' + name + '"]') : page.getByLabel(name, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function get(route) {
    const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {});
    assert.equal(response.status(), 200); return (await response.json()).data;
  }
  const pid = fixture.project_id;
  const route = mobile ? '/#/pages/project/index?id=' + pid + '&section=budgets' : '/projects/' + pid + '?tab=budgets';
  await page.goto(fixture.origin + route); await button('预算与实际支出对比').click(); await field('对比币种').fill('HKD'); await button('查询预算对比').click();
  const report = await get('/projects/' + pid + '/budget-comparison?currency=HKD');
  assert.equal(report.matched.net_amount, '1385.000000'); assert.equal(report.outside.net_amount, '-945.000000'); assert.equal(report.lifetime.net_amount, '440.000000'); assert.equal(report.lifetime.unconfirmed_count, 2); assert.equal(report.lines.length, 3);
  await root().getByText('预算内可见净支出', { exact: true }).waitFor();
  await root().getByText(/1,385/).first().waitFor(); await assertNoCredentials(page);
  checks.push('Actual MFA login, implemented budget and authorized canonical shares reconcile HKD1385 inside, HKD-945 outside and HKD440 lifetime without counting two drafts.');
  const prefix = 'project-budget-comparison-' + (mobile ? 'mobile' : 'web'), media = path.join(app, '../docs/media/screenshots');
  await page.screenshot({ path: path.join(media, prefix + '-summary.png'), fullPage: false });
  async function select(label, value, text) {
    if (mobile) await selectConcernMobile(page, label, text); else await field(label).selectOption(value);
  }
  if (mobile) await button('查看费用来源').click();
  await select('核对范围', '0', '第1项 · software · 2025-01-01');
  await select('费用确认状态', 'draft', '未确认费用');
  await root().getByText('Synthetic software 2025-01-10', { exact: true }).waitFor();
  await root().getByText('未计入', { exact: true }).waitFor();
  checks.push('A budget-line drilldown shows the actual unconfirmed source without treating its share as incurred expenditure.');
  await select('核对范围', 'outside', '预算外费用');
  await root().getByText('Incomplete date and category', { exact: true }).waitFor();
  await root().getByText(/发生日未填/).first().waitFor();
  checks.push('A partial draft with no occurrence date or category stays unknown and readable in outside-budget detail.');
  await select('费用确认状态', 'incurred', '已发生费用');
  await root().getByText(/-1,000/).first().waitFor();
  await root().getByText('费用来源核对', { exact: true }).first().scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(media, prefix + '-sources.png'), fullPage: false });
  checks.push('Outside-budget detail retains the real reversal contribution on its own date and links canonical sources.');
  const recording = await recordViewport(page, app, 'budget-comparison-' + (mobile ? 'mobile' : 'web'), async () => {
    await select('核对范围', '', '全部可见费用');
    if (mobile) {
      await button('下一页对比费用').click(); await button('上一页对比费用').waitFor(); await button('上一页对比费用').click(); await button('下一页对比费用').waitFor();
    } else {
      await button('加载更多对比费用').click(); await button('加载更多对比费用').waitFor({ state: 'hidden' });
    }
    await select('核对范围', '0', '第1项 · software · 2025-01-01');
    await root().getByText('Synthetic software 2025-01-31', { exact: true }).waitFor();
  });
  checks.push('Real source pagination and a return to the exact line preserve the current budget basis.');
  await root().getByText('Synthetic software 2025-01-31', { exact: true }).click();
  await page.waitForURL(mobile ? /\/pages\/finance\/index\?/ : new RegExp('/costs/' + fixture.cost_id));
  const original = await get('/costs/' + fixture.cost_id); assert.equal(original.state, 'confirmed'); assert.equal(original.currency, 'HKD');
  await page.goto(fixture.origin + route); await button('预算与实际支出对比').click(); await field('对比币种').fill('USD'); await button('查询预算对比').click();
  await root().getByText('预算内可见净支出', { exact: true }).waitFor();
  const usd = await get('/projects/' + pid + '/budget-comparison?currency=USD'); assert.equal(usd.lines.length, 100); assert.equal(usd.lines[99].net_amount, '0.010000');
  await root().getByText(/slot-099/).first().waitFor({ state: 'attached' });
  await field('对比币种').fill('CNY'); await button('查询预算对比').click();
  await root().getByText('尚无批准预算', { exact: true }).first().waitFor();
  const absent = await get('/projects/' + pid + '/budget-comparison?currency=CNY'); assert.equal(absent.budget, null); assert.equal(absent.budget_variance, null);
  checks.push('Canonical source navigation opens the real cost; another original currency retains all100 lines, and an absent CNY plan is never reported as a zero allowance.');
  await field('对比币种').fill('HKD'); await button('查询预算对比').click();
  await root().getByText(/1,385/).first().waitFor();
  const layouts = [];
  for (const width of mobile ? [390, 430, 768] : [1440, 960, 390]) {
    await page.setViewportSize({ width, height: mobile ? 900 : 1100 });
    const layout = await page.evaluate(() => ({ width: innerWidth, body: document.documentElement.scrollWidth, tables: [...document.querySelectorAll('.budget-comparison .table-scroll')].map(table => ({ width: table.clientWidth, contents: table.scrollWidth, scrolling: getComputedStyle(table).overflowX })) }));
    assert.ok(layout.body <= width + 1, 'The document must not overflow horizontally.');
    if (!mobile) assert.ok(layout.tables.every(table => table.contents <= table.width + 1 || ['auto', 'scroll'].includes(table.scrolling)), 'Wide ledger columns must stay in their own scroll region.');
    layouts.push(layout);
  }
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ passed: true, layouts, recordings: [recording], synthetic_data: true, real_model_cloud_telegram: false }, null, 2) + '\n');
}, 'budget-comparison');
