import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { assertReviewFacts, orderReviewSources } from './subscription_review_browser_common.mjs';
const screen = page => page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });

await notificationBrowser(true, async ({ page, context, fixture, checks, evidence }) => {
  let token; page.on('request', request => { const value = request.headers().authorization; if (value) token = value; });
  await page.goto(fixture.origin);
  await screen(page).getByRole('textbox', { name: '内部账号', exact: true }).fill(fixture.alice.login);
  await screen(page).getByRole('textbox', { name: '密码', exact: true }).fill(fixture.alice.password);
  await button(page, '登录工作空间 ↗').click(); await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));
  await button(page, '验证并继续 ↗').click(); await screen(page).locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  await screen(page).locator('.cp-row-title').getByText('公司秘书', { exact: true }).click();
  const pending = page.waitForResponse(response => response.url().includes('/conversations/' + fixture.conversation_id + '/messages') && response.request().method() === 'GET');
  await page.goto(fixture.origin + '/#/pages/secretary/index?kind=conversation&id=' + fixture.conversation_id + '&run=' + fixture.completed_run_id);
  const response = await pending; assert.equal(response.status(), 200);
  const completed = (await response.json()).data.items.find(v => v.role === 'assistant' && v.visibility === 'available'); assert.ok(completed); assert.ok(token);
  const reports = await assertReviewFacts(context.request, fixture.api_origin, fixture, { Authorization: token });
  const ids = [fixture.finance_report_id, fixture.cost_report_id, fixture.payment_report_id];
  for (let i = 0; i < ids.length; i++) assert.ok(completed.source_refs.some(v => v.type === 'agent_finance_report' && v.id === ids[i] && v.sha256 === reports[i].digest));
  checks.push({ name: 'Taro opens genuine completed review evidence under current mobile authorization', passed: true });
  checks.push({ name: 'Mobile finance detail retains exact forecast, incurred cost, allocations, outstanding balance and original record scope', passed: true });
  let pages = 0;
  const ordered = await orderReviewSources(context.request, fixture.api_origin, fixture, completed, reports, { Authorization: token });
  for (let i = 0; i < ordered.length; i++) {
    await button(page, '查看固定财务快照').nth(i).click();
    // Compare each exact server page, rather than assuming the first excerpt is complete.
    for (let j = 0; j < ordered[i].parts.length; j++) {
      await screen(page).getByText(ordered[i].parts[j], { exact: true }).waitFor(); pages++;
      if (j + 1 < ordered[i].parts.length) await button(page, '读取快照下一段').click();
    }
    assert.match(await screen(page).textContent(), /原币种不合并/);
    if (ordered[i].parts.length > 1) { await button(page, '回到快照第一段').click(); await screen(page).getByText(ordered[i].parts[0], { exact: true }).waitFor(); }
    if (ordered[i].content.view === 'subscription_review' && ordered[i].content.report.coverage_complete) {
      await screen(page).locator('.cp-section-title').filter({ hasText: '固定财务快照' }).scrollIntoViewIfNeeded();
      await page.screenshot({ path: evidence + '/detail.png' });
    }
    await button(page, '收起财务快照').click(); await button(page, '收起财务快照').waitFor({ state: 'hidden' });
  }
  checks.push({ name: 'Taro traverses all source chunks, returns to the first chunk and closes all readers', passed: true, pages });
}, 'subscription-review');
