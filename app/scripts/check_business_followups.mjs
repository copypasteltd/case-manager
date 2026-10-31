import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { followupsBrowser, waitOTP } from './business_followups_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './business_followups_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await followupsBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const ids = fixture.followup_ids, screenshots = [], layouts = [], prefix = 'business-followups-web';
  const panel = () => page.getByRole('region', { name: '申请与交付跟进', exact: true });
  const group = label => panel().getByRole('region', { name: label, exact: true });
  async function get(route) { const response = await context.request.get(fixture.origin + '/v1' + route); assert.equal(response.status(), 200); return (await response.json()).data; }
  async function home() {
    await page.goto(fixture.origin + '/');
    await panel().getByRole('heading', { name: '我可核对的已提交交付', exact: true }).waitFor();
    await panel().scrollIntoViewIfNeeded();
    await assertNoCredentials(page);
  }
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    screenshots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
    const layout = await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth }));
    assert.ok(layout.scroll <= layout.width + 1); layouts.push(layout);
  }
  await home();
  const before = await get('/dashboard/followups');
  assert.deepEqual(before.groups.map(g => [g.kind, g.count]), [['requests_to_revise', 1], ['deliverables_to_submit', 1], ['deliverables_to_review', 1]]);
  const initial = await get('/deliverables/' + ids.review);
  assert.equal(initial.state, 'submitted');
  const revision = await get('/deliverables/' + ids.review + '/revisions/' + initial.current_revision_id);
  const recording = await recordViewport(page, app, prefix, async () => {
    assert.equal(await panel().locator('em,script').count(), 0);
    assert.match(await group('我可核对的已提交交付').innerText(), /<em>literal<\/em>/);
    await shot('overview');
    await group('我发起的待推进申请').getByRole('link').click();
    await page.waitForURL(fixture.origin + '/requests/' + ids.request);
    await page.getByRole('heading', { name: 'Synthetic followup contract', exact: true }).first().waitFor();
    assert.equal((await get('/requests/' + ids.request)).state, 'returned');
    checks.push('Actual home applicant entry opens the current returned request; current metadata and title render as ordinary text.');
    await home();
    await group('我可核对的已提交交付').getByRole('link').click();
    await page.getByRole('dialog', { name: '项目交付详情', exact: true }).waitFor();
    const current = new URL(page.url());
    assert.equal(current.pathname, '/projects/' + ids.project);
    assert.equal(current.searchParams.get('tab'), 'deliverables');
    assert.equal(current.searchParams.get('deliverable'), ids.review);
    await page.getByRole('button', { name: '核实并登记验收', exact: true }).click();
    await page.getByLabel('本次验收决定', { exact: true }).selectOption('accept');
    await page.getByLabel('已核实全部固定文件版本及完整内容', { exact: true }).check();
    await page.getByLabel('已逐项核对本次固定验收准则', { exact: true }).check();
    await page.getByLabel('验收决定说明', { exact: true }).fill('合成演示：本人独立核对本次准确修订、完整固定附件和约定标准后确认验收。');
    await page.getByRole('button', { name: '确认登记验收决定', exact: true }).waitFor();
    await page.waitForFunction(() => !document.querySelector('input[autocomplete="one-time-code"]'));
    await shot('review');
    const response = page.waitForResponse(r => r.url() === fixture.origin + '/v1/deliverables/' + ids.review + '/decisions' && r.request().method() === 'POST');
    await page.getByRole('button', { name: '确认登记验收决定', exact: true }).click();
    const result = await response; assert.equal(result.status(), 200);
    const input = result.request().postDataJSON();
    assert.equal(input.revision_id, revision.id); assert.equal(input.content_hash, revision.content_hash);
    assert.equal(input.files_verified, true); assert.equal(input.criteria_verified, true);
    assert.equal((await get('/deliverables/' + ids.review)).state, 'accepted');
    checks.push('Actual project deep link opens the exact submitted revision; independent Web decision uses both explicit checks, recent human MFA and the ordinary fixed-revision command.');
    await home();
    assert.equal((await get('/dashboard/followups')).groups.find(g => g.kind === 'deliverables_to_review').count, 0);
    assert.equal(await group('我可核对的已提交交付').getByRole('link').count(), 0);
    await shot('closed');
    checks.push('After the real acceptance receipt the refreshed personal home removes the review item and preserves unrelated applicant and owner work.');
    await page.goto(fixture.origin + '/projects/' + ids.other_project + '?tab=deliverables&deliverable=' + ids.review);
    await page.getByText('交付不属于当前项目，请从正确的项目或首页入口重新打开。', { exact: true }).waitFor();
    assert.equal(await page.getByRole('button', { name: '核实并登记验收', exact: true }).count(), 0);
    checks.push('A different authorized parent project cannot render or act on the linked delivery as if it belonged there.');
    await home();
  });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ recording, screenshots, layouts, synthetic_data: true, scope: 'Actual Web/native HTTP/PG home followups, returned-request entry, independent delivery acceptance and parent-project boundary; no real provider/model/device.' }, null, 2) + '\n');
}, 'business-followups');
