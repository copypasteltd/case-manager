import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './finance_fx_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './finance_fx_recording.mjs';

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
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, mobile ? { headers: { Authorization: bearer } } : {}); assert.equal(response.status(),200,route); return (await response.json()).data; }
  await root().getByRole('button',{name:/^(\u8c03\u6574\u7b5b\u9009\u6761\u4ef6|\u6536\u8d77\u7b5b\u9009|\u8c03\u6574\u7b5b\u9009)$/}).waitFor();
  if (!await field(mobile ? '报表起始日' : '起始日（含）').isVisible()) await button(mobile ? '调整筛选条件' : '调整筛选').click();
  await field(mobile ? '报表起始日' : '起始日（含）').fill('2025-01-01'); await field(mobile ? '报表结束日' : '结束日（不含）').fill('2025-02-01'); await button('查询报表').click();
  await button('选择汇率版本').click();
  const row = title => root().locator(mobile ? '.cp-fact' : '.report-context').filter({ hasText: title });
  const missing = (await get('/finance/fx-books?limit=100')).items.find(v => v.revision === 2); assert.ok(missing);
  await row(missing.title).getByRole('button',{name:'用于当前报表',exact:true}).click();
  await root().getByText('完整合计待补齐',{exact:true}).waitFor();
  const prefix = 'finance-fx-' + (mobile ? 'mobile' : 'web'), screenshots = path.join(app,'../docs/media/screenshots'); await mkdir(screenshots,{recursive:true});
  const layouts = [], before = prefix + '-before.png', after = prefix + '-after.png';
  const recording = await recordViewport(page,app,prefix,async () => {
    await assertNoCredentials(page); await root().getByText('完整合计待补齐',{exact:true}).scrollIntoViewIfNeeded();
    await page.screenshot({path:path.join(screenshots,before)});
    const first = (await get('/finance/fx-books/' + fixture.fx_book_id));
    await row(first.title).getByRole('button',{name:'用于当前报表',exact:true}).click();
    await root().getByText('204',{exact:true}).waitFor();
    const check = await get('/finance/reports?kind=incurred&from=2025-01-01&to=2025-02-01&fx_book_id=' + first.id + '&limit=100');
    assert.equal(check.valuation.total_amount,'204.000000'); assert.equal(check.valuation.book.id,first.id);
    checks.push('Explicit book selection changes missing-rate coverage into the exact authorized 204 HKD view; original facts remain separately denominated.');
    await button('发布人工汇率版本').click();
    await field('汇率版本标题').fill('Synthetic ' + (mobile ? 'mobile' : 'Web') + ' reviewed FX'); await field('基础币种').fill('HKD');
    const textarea = mobile ? root().locator('textarea[aria-label="人工汇率明细"]') : page.getByLabel('人工汇率明细',{exact:true});
    await textarea.fill('USD\t2025-01-01\t2025-02-01\t2024-12-31\t7.9\tSynthetic approved treasury quotation'); await field('汇率发布依据').fill('Synthetic human reviewed quote for reporting only');
    if (mobile) await button('确认基础币种、来源与舍入口径').click(); else await page.getByRole('checkbox',{name:'我确认基础币种、汇率来源与舍入口径'}).check();
    const pending = page.waitForResponse(v => v.request().method()==='POST' && new URL(v.url()).pathname==='/v1/finance/fx-books'); await button('确认发布').click();
    const response = await pending; assert.equal(response.status(),201); const published = (await response.json()).data; assert.equal(published.revision,4);
    await row(published.title).getByRole('button',{name:'用于当前报表',exact:true}).click(); await root().getByText('205',{exact:true}).waitFor();
    const fixed = await get('/finance/reports?kind=incurred&from=2025-01-01&to=2025-02-01&fx_book_id=' + first.id); assert.equal(fixed.valuation.total_amount,'204.000000');
    checks.push('Actual MFA-authenticated human publish creates audited revision4; the new view is205 HKD while revision1 stays204 HKD.');
    const collapse = button(mobile ? '收起汇率版本' : '选择汇率版本'); await collapse.click();
    await root().getByText('205',{exact:true}).scrollIntoViewIfNeeded(); await page.screenshot({path:path.join(screenshots,after)});
    for (const width of [390,768,1440]) { await page.setViewportSize({width,height:1000}); await root().getByText('205',{exact:true}).scrollIntoViewIfNeeded(); assert.equal(await root().getByText('205',{exact:true}).isVisible(),true); layouts.push({width,height:1000,valuation_visible:true}); }
    await page.setViewportSize(mobile ? {width:390,height:900} : {width:1440,height:1100});
    await field('导出用途').fill('Synthetic pinned base valuation export');
    const exportResponse = page.waitForResponse(v => v.request().method()==='POST' && new URL(v.url()).pathname==='/v1/finance/report-exports'); await button('按当前条件生成 CSV').click();
    const acceptedResponse = await exportResponse; assert.equal(acceptedResponse.status(),202); const accepted=(await acceptedResponse.json()).data; assert.equal(accepted.filters.fx_book_id,published.id);
    let ready;
    for (let i=0;i<60;i++) { ready=await get('/finance/report-exports/'+accepted.id); if (ready.can_download) break; await new Promise(resolve=>setTimeout(resolve,250)); }
    assert.equal(ready.state,'succeeded');
    const download = await context.request.get(origin + '/v1/finance/report-exports/' + accepted.id + '/download',mobile ? {headers:{Authorization:bearer}} : {}); assert.equal(download.status(),200); const bytes=await download.body();
    assert.equal(createHash('sha256').update(bytes).digest('hex'),ready.csv_sha256); assert.ok(bytes.toString('utf8').includes(published.content_sha256)); assert.ok(bytes.toString('utf8').includes('Synthetic approved treasury quotation'));
    checks.push('UI creates a real queued export; the native fenced worker generates CSV bound to the selected immutable quote book and whole-file SHA256.');
    await writeFile(path.join(evidence,'download-proof.json'),JSON.stringify({export_id:ready.id,book_id:published.id,book_revision:published.revision,book_sha256:published.content_sha256,bytes:bytes.length,sha256:ready.csv_sha256,actual_native_http_download:true,synthetic_data:true},null,2)+'\n');
  });
  const shots = [];
  for (const name of [before,after]) shots.push({file:'docs/media/screenshots/'+name,sha256:createHash('sha256').update(await readFile(path.join(screenshots,name))).digest('hex')});
  await writeFile(path.join(evidence,'capture.json'),JSON.stringify({recordings:[recording],layouts,screenshots:shots,synthetic_data:true,real_browser_pixels:true,audio:false},null,2)+'\n');
},'finance-fx');
