import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './financial_reports_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const button = name => root().getByRole('button', { name, exact: true });
  const field = name => mobile ? root().locator('input[aria-label="' + name + '"]') : page.getByLabel(name, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) { await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); await root().locator('.cp-row-title').getByText('财务报表', { exact: true }).click(); }
  else { await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); await page.goto(fixture.origin + '/reports/expenses'); }
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {}); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  async function filters() { if (!await field(mobile ? '报表起始日' : '起始日（含）').isVisible()) await button(mobile ? '调整筛选条件' : '调整筛选').click(); }
  async function scope(value) {
    if (!mobile) { await field('业务／分摊范围').selectOption(value); return; }
    await root().locator('.cp-row-title').getByText('项目／分摊范围', { exact: true }).click();
    if (!value) await button('全部项目／分摊范围').click();
    else { const project = await get('/projects/' + value); await root().locator('.cp-row-title').getByText(project.name, { exact: true }).click(); }
  }
  await field(mobile ? '报表起始日' : '起始日（含）').fill('2025-01-01'); await field(mobile ? '报表结束日' : '结束日（不含）').fill('2025-02-01'); await scope(fixture.project_id); await button('查询报表').click();
  const query = new URLSearchParams({ kind: 'incurred', from: '2025-01-01', to: '2025-02-01', project_id: fixture.project_id, limit: '20' });
  const actual = await get('/finance/reports?' + query);
  assert.equal(actual.summary.totals[0].net_amount, '76.400000'); assert.equal(actual.summary.totals[1].currency, 'USD'); assert.equal(actual.summary.totals[1].net_amount, '10.000000'); assert.equal(actual.summary.unconfirmed_count, 1); assert.ok(actual.next_cursor);
  await root().getByText('76.4', { exact: true }).first().waitFor(); await assertNoCredentials(page);
  checks.push('Real MFA login and native report reconcile HKD76.4 and USD10 separately, with one unconfirmed draft and actual paged sources.');
  const prefix = 'financial-reports-' + (mobile ? 'mobile' : 'web'), media = path.join(app, '../docs/media/screenshots');
  if (mobile) await root().locator('.cp-heading').scrollIntoViewIfNeeded(); else await page.getByRole('heading', { name: /财务，一目了然/ }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: path.join(media, prefix + '-summary.png'), fullPage: false });
  async function choose(title, label) {
    if (mobile) { await root().locator('.cp-row-title').getByText(title, { exact: true }).click(); await root().locator('.cp-row-title').getByText(label, { exact: true }).click(); }
    else { await page.getByLabel(title, { exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click(); }
  }
  const recording = await recordViewport(page, app, 'financial-reports-' + (mobile ? 'mobile' : 'web'), async () => {
    await button('下一页明细').click(); await root().getByText(mobile ? '原始明细 · 第 2 页' : '第 2 页', { exact: true }).waitFor();
    await button(mobile ? '上一页明细' : '上一页').click(); await root().getByText(mobile ? '原始明细 · 第 1 页' : '第 1 页', { exact: true }).waitFor();
    await filters(); await choose('费用类别', 'software'); await choose('供应方', 'Synthetic report supplier'); await choose('经办／负责人', 'alice');
    await field(mobile ? '报表币种' : '原币币种').fill('HKD'); await field(mobile ? '报表关键词' : '标题／来源编号').fill('later fact'); await button('查询报表').click();
    await root().getByText('Synthetic later fact', { exact: true }).waitFor();
    checks.push('Category, supplier, recorder, currency and keyword choices operate through readable native source records on the actual client.');
    await filters(); await field(mobile ? '报表关键词' : '标题／来源编号').fill(''); await button('查询报表').click();
    await root().getByText('76.4', { exact: true }).first().waitFor();
    if (mobile) await root().getByText('原始明细 · 第 1 页', { exact: true }).scrollIntoViewIfNeeded(); else await page.getByRole('heading', { name: '核对原始明细', exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(media, prefix + '-sources.png'), fullPage: false });
    await field('导出用途').fill('Synthetic monthly project report review');
    const pending = page.waitForResponse(response => new URL(response.url()).pathname === '/v1/finance/report-exports' && response.request().method() === 'POST');
    await button('按当前条件生成 CSV').click();
    const receipt = await pending; assert.equal(receipt.status(), 202); const accepted = (await receipt.json()).data; assert.equal(accepted.state, 'queued'); assert.equal(accepted.can_download, false);
    const jobRow = mobile ? root().locator('.cp-fact').filter({ hasText: '导出任务 ' + accepted.id }) : page.locator('[data-export-id="' + accepted.id + '"]');
    const downloadButton = jobRow.getByRole('button', { name: mobile ? '校验并打开 CSV' : '下载 CSV', exact: true }); await downloadButton.waitFor();
    const ready = await get('/finance/report-exports/' + accepted.id); assert.equal(ready.state, 'succeeded'); assert.equal(ready.row_count, 27); assert.equal(ready.filters.category, 'software'); assert.equal(ready.filters.supplier_id, fixture.supplier_id); assert.equal(ready.filters.owner_id, fixture.owner_id);
    const downloaded = page.waitForEvent('download'); await downloadButton.click(); const artifact = await downloaded;
    const destination = path.join(app, '.local/repository-showcase', prefix + '-actual.csv'); await mkdir(path.dirname(destination), { recursive: true }); await artifact.saveAs(destination);
    assert.equal(artifact.suggestedFilename(), 'finance-incurred-' + accepted.id + '.csv'); const bytes = await readFile(destination); assert.equal(createHash('sha256').update(bytes).digest('hex'), ready.csv_sha256); assert.equal(bytes.length, ready.csv_bytes); assert.ok(bytes.includes(Buffer.from("'=Synthetic spreadsheet text")));
    checks.push('A real 202 enqueue becomes a fenced immutable snapshot; actual downloaded CSV has all27 matching sources and its declared whole-file SHA256, without spreadsheet formula execution text.');
    await writeFile(path.join(evidence, 'download-proof.json'), JSON.stringify({ export_id: accepted.id, state: ready.state, filters: ready.filters, row_count: ready.row_count, bytes: bytes.length, sha256: ready.csv_sha256, actual_download_verified: true, synthetic_data: true, csv_private_and_ignored: true }, null, 2) + '\n');
    if (mobile) await root().getByText('我的报表导出', { exact: true }).scrollIntoViewIfNeeded(); else await page.getByRole('heading', { name: '我的报表导出', exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(media, prefix + '-exports.png'), fullPage: false });
    await filters(); await scope(''); await button('查询报表').click(); await button('现金支出').click();
    await root().getByText('90', { exact: true }).first().waitFor();
    const cash = await get('/finance/reports?kind=cash&from=2025-01-01&to=2025-02-01'); assert.equal(cash.summary.totals[0].net_amount, '90.000000');
    await button('订阅预测').click(); await filters(); await field(mobile ? '报表结束日' : '结束日（不含）').fill('2025-04-01'); await button('查询报表').click();
    await root().getByText('200', { exact: true }).first().waitFor();
    const forecast = await get('/finance/reports?kind=forecast&from=2025-01-01&to=2025-04-01'); assert.equal(forecast.summary.totals[0].expected_amount, '200.000000'); assert.equal(forecast.summary.missing_period_count, 1); assert.equal(forecast.summary.coverage_complete, false);
    checks.push('Cash retains its own HKD90 recorded/refunded facts, while stored subscription forecasts remain HKD200 with an explicit missing period.');
  });
  const layouts = [];
  for (const width of mobile ? [390, 430, 768] : [1440, 960, 390]) { await page.setViewportSize({ width, height: mobile ? 900 : 1100 }); const layout = await page.evaluate(() => ({ width: innerWidth, document: document.documentElement.scrollWidth })); assert.ok(layout.document <= layout.width + 1, JSON.stringify(layout)); layouts.push(layout); }
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ recordings: [recording], layouts, audio: false, real_browser_pixels: true }, null, 2) + '\n');
  checks.push('Three responsive widths retain the report and its honest coverage without document overflow; all owned contexts are closed by the harness.');
}, 'financial-reports');
