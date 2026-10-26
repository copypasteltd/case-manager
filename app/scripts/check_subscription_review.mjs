import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { assertReviewFacts, orderReviewSources } from './subscription_review_browser_common.mjs';

await notificationBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码').fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.goto(fixture.origin + '/conversations/' + fixture.conversation_id);
  await page.getByRole('button', { name: '查看固定财务快照', exact: true }).first().waitFor();
  const reports = await assertReviewFacts(context.request, fixture.origin, fixture);
  const response = await context.request.get(fixture.origin + '/v1/conversations/' + fixture.conversation_id + '/messages?limit=20'); assert.equal(response.status(), 200);
  const history = (await response.json()).data.items, completed = history.find(v => v.role === 'assistant' && v.visibility === 'available');
  assert.ok(completed);
  const ids = [fixture.finance_report_id, fixture.cost_report_id, fixture.payment_report_id];
  for (let i = 0; i < ids.length; i++) assert.ok(completed.source_refs.some(v => v.type === 'agent_finance_report' && v.id === ids[i] && v.sha256 === reports[i].digest));
  checks.push({ name: 'Completed Web review cites the genuine detail and original record source digests', passed: true });
  checks.push({ name: 'Forecast, incurred cost, allocated cash and unpaid balance match the ordinary APIs without floating-point conversion', passed: true });
  checks.push({ name: 'Cost and payment drill-down preserve actual associations and fixed evidence; candidate anomalies remain candidates', passed: true });
  let pages = 0;
  const ordered = await orderReviewSources(context.request, fixture.origin, fixture, completed, reports);
  for (let i = 0; i < ordered.length; i++) {
    await page.getByRole('button', { name: '查看固定财务快照', exact: true }).nth(i).click();
    const section = page.locator('section.context-preview').filter({ has: page.getByRole('heading', { name: '固定财务快照', exact: true }) });
    assert.match(await section.textContent(), /原币种不合并/);
    for (let j = 0; j < ordered[i].parts.length; j++) {
      await section.locator('pre').filter({ hasText: ordered[i].parts[j].slice(0, 70) }).waitFor();
      assert.equal(await section.locator('pre').textContent(), ordered[i].parts[j]); pages++;
      if (j + 1 < ordered[i].parts.length) await section.getByRole('button', { name: '读取快照下一段', exact: true }).click();
    }
    if (ordered[i].parts.length > 1) {
      await section.getByRole('button', { name: '回到快照第一段', exact: true }).click();
      await section.locator('pre').filter({ hasText: ordered[i].parts[0].slice(0, 70) }).waitFor();
    }
    if (ordered[i].content.view === 'subscription_review' && ordered[i].content.report.coverage_complete) await section.screenshot({ path: evidence + '/detail.png' });
    await page.getByRole('button', { name: '收起财务快照', exact: true }).click(); await section.waitFor({ state: 'hidden' });
  }
  assert.ok(pages >= 3);
  checks.push({ name: 'Web traverses every chunk of all four sources including missing-period coverage and closes each reader', passed: true, pages });
}, 'subscription-review');
