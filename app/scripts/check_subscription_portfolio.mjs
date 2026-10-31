import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './subscription_portfolio_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './subscription_portfolio_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  await page.goto(fixture.origin + '/conversations/' + fixture.conversation_id);
  const portfolio = page.getByTestId('subscription-portfolio'), screenshots = [], prefix = 'subscription-portfolio-web';
  const fillDates = async () => {
    await portfolio.getByLabel('审阅开始日', { exact: true }).fill(fixture.from);
    await portfolio.getByLabel('审阅结束日（不含）', { exact: true }).fill(fixture.to);
  };
  async function shot(suffix) {
    await portfolio.scrollIntoViewIfNeeded(); await assertNoCredentials(page);
    assert.equal(await page.getByText(/PRIVATE_/).count(), 0);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    screenshots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  async function read() {
    const pending = page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname === '/v1/conversations/' + fixture.conversation_id + '/subscription-review');
    await portfolio.getByRole('button', { name: '读取完整审阅', exact: true }).click();
    const response = await pending; assert.equal(response.status(), 200);
    const value = (await response.json()).data;
    assert.equal(value.from, fixture.from); assert.equal(value.to_exclusive, fixture.to);
    assert.equal(value.summary.subscription_count, 2); assert.equal(value.summary.missing_period_count, 2);
    assert.equal(value.summary.stored_period_count, 2); assert.equal(value.summary.expected_period_count, 4);
    assert.equal(value.summary.unique_cost_count, 1); assert.equal(value.summary.unique_payment_count, 1);
    assert.equal(value.summary.schedule_coverage_complete, false); assert.equal(value.summary.subscription_set_complete, true);
    assert.deepEqual(value.summary.items.map(v => v.id).sort(), [...fixture.subscription_ids].sort());
    assert.equal(createHash('sha256').update(value.content_text).digest('hex'), value.content_sha256);
    await portfolio.getByText('所选日期内存在尚未生成的应有账期。', { exact: true }).waitFor();
    assert.equal(await portfolio.locator('tbody tr').count(), 2);
    return value;
  }
  await fillDates(); await shot('dates');
  let value;
  const recording = await recordViewport(page, app, prefix, async () => {
    value = await read(); await shot('coverage');
    await portfolio.getByRole('button', { name: '查看完整逐笔依据', exact: true }).click();
    await portfolio.locator('pre').waitFor();
    assert.equal(await portfolio.locator('pre').textContent(), [...value.content_text].slice(0, 2400).join(''));
    await portfolio.getByRole('button', { name: '下一页依据', exact: true }).click();
    assert.equal(await portfolio.locator('pre').textContent(), [...value.content_text].slice(2400, 4800).join(''));
    await shot('evidence');
  });
  checks.push('Authenticated Web selects explicit local dates and reads both real subscriptions, all stored/missing periods and unique actual financial links, without a monetary sum or completion claim.');
  checks.push('Exact UTF-8 SHA-256 and consecutive inert text pages match the same complete returned evidence; actual pixels record only synthetic authenticated business content.');
  await page.reload(); await portfolio.getByRole('heading', { name: '跨订阅完整审阅', exact: true }).waitFor();
  assert.equal(await portfolio.locator('tbody tr').count(), 0);
  await fillDates(); const again = await read();
  assert.deepEqual(again.summary, value.summary);
  checks.push('Reload clears the ephemeral preview; an explicit fresh read preserves scope and business counts. Native PG independently verifies that preview created no model report or financial facts.');
  await writeFile(path.join(evidence, 'business-proof.json'), JSON.stringify({ conversation_id: fixture.conversation_id, from: fixture.from, to_exclusive: fixture.to, summary: value.summary, content_sha256: value.content_sha256, reread_content_sha256: again.content_sha256, ephemeral_preview: true, financial_commands_executed: false, model_contacted: false }, null, 2) + '\n');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, credentials_excluded: true, synthetic_data: true }, null, 2) + '\n');
}, 'subscription-portfolio');
