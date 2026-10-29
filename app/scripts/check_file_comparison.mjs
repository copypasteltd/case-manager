// Real synthetic UI input reaches the owned native API and PostgreSQL.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { comparisonBrowser, waitOTP } from './file_comparison_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './file_comparison_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
await comparisonBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  assert.ok(fixture.file_comparison_fixture && fixture.isolated_database);
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = name => root().getByRole('button', { name, exact: true });
  let bearer, reads = 0;
  page.on('request', request => {
    if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization;
    if (new URL(request.url()).pathname.endsWith('/text-comparison')) reads++;
  });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture));
  await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = async () => mobile ? { Authorization: bearer } : { Origin: fixture.origin, 'X-CSRF-Token': (await context.cookies()).find(c => c.name === 'cp_csrf')?.value ?? '' };
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, { headers: await headers() }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  const file = await get('/files/' + fixture.file_id), versions = await get('/files/' + fixture.file_id + '/versions');
  const items = Array.isArray(versions) ? versions : versions.items;
  assert.equal(file.current_version_id, fixture.normalized_version_id);
  await page.goto(fixture.origin + (mobile ? '/#/pages/files/index?file_id=' : '/files/') + fixture.file_id);
  const panel = () => mobile ? root().locator('.cp-section').filter({ hasText: '比较两个内容版本' }) : page.getByRole('region', { name: '文件版本正文比较', exact: true });
  await panel().waitFor();
  async function select(label, value) {
    if (mobile) await selectMobile(page, label, label === '差异显示范围' ? (value === 'all' ? '全部正文' : '差异与前后文') : '内容版本 ' + items.find(v => v.id === value).seq + ' · ' + value.slice(0, 8));
    else await page.getByLabel(label, { exact: true }).selectOption(value);
  }
  async function compare() {
    const response = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/files/' + fixture.file_id + '/text-comparison');
    await button('比较所选版本').click(); const result = await response; assert.equal(result.status(), 200);
    await button('比较所选版本').waitFor({ state: 'visible' });
    const data = (await result.json()).data; await panel().getByText('内容版本 ' + data.base.seq + ' → ' + data.target.seq, { exact: true }).waitFor();
    return data;
  }
  const initialReads = reads;
  await select('比较基础版本', fixture.base_version_id); await select('比较目标版本', fixture.target_version_id);
  assert.equal(reads, initialReads);
  checks.push('Selecting exact historical versions does not fetch comparison content; the current formal head is a different version.');
  const directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  const prefix = 'file-comparison-' + (mobile ? 'mobile' : 'web'), shots = [], layouts = [], pages = [];
  async function shot(suffix) { await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png'; await page.screenshot({ path: path.join(directory, file) }); shots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) }); }
  const recording = await recordViewport(page, app, prefix, async () => {
    const contextPage = await compare();
    assert.equal(contextPage.base.version_id, fixture.base_version_id); assert.equal(contextPage.target.version_id, fixture.target_version_id);
    assert.equal(contextPage.base.text_sha256, fixture.base_text_sha256); assert.equal(contextPage.target.text_sha256, fixture.target_text_sha256);
    assert.ok(contextPage.rows.some(r => r.kind === 'skip' && r.count > 0));
    await panel().getByText('新条款 <script>window.__comparisonExecuted=true</script>', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => window.__comparisonExecuted), undefined);
    await panel().getByText('内容版本 1 → 2', { exact: true }).scrollIntoViewIfNeeded(); await shot('context');
    checks.push('An actual comparison shows every changed line, counted context omissions, exact original/text digests and literal script-looking text without executing it.');
    const beforeRangeChange = reads;
    await select('差异显示范围', 'all');
    assert.equal(await panel().getByText('内容版本 1 → 2', { exact: true }).count(), 0); assert.equal(reads, beforeRangeChange);
    const first = await compare(); pages.push(first); assert.equal(first.view, 'all'); assert.equal(first.rows.length, 60); assert.equal(first.next_offset, 60);
    const nextResponse = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/text-comparison') && new URL(r.url()).searchParams.get('offset') === '60');
    await button('下一页正文差异').click(); const next = await nextResponse; assert.equal(next.status(), 200); const last = (await next.json()).data; pages.push(last);
    assert.equal(last.comparison_sha256, first.comparison_sha256); assert.equal(last.offset, 60); assert.equal(last.next_offset, null);
    await panel().getByText('新条款 <script>window.__comparisonExecuted=true</script>', { exact: true }).waitFor();
    const rows = pages.flatMap(p => p.rows); assert.ok(rows.every(r => r.kind !== 'skip'));
    for (const [kind, expected] of [['insert', fixture.base_text_sha256], ['delete', fixture.target_text_sha256]]) {
      const body = rows.filter(r => r.kind !== kind).map(r => r.text + (r.newline ? '\n' : '')).join(''); assert.equal(sha(body), expected);
    }
    await panel().getByText('新条款 <script>window.__comparisonExecuted=true</script>', { exact: true }).scrollIntoViewIfNeeded(); await shot('full');
    checks.push('Real UI paging pins one comparison, reconstructs both complete historical bodies to their recorded SHA-256 and preserves final-newline differences.');
    await select('比较目标版本', fixture.normalized_version_id); const normalized = await compare();
    assert.equal(normalized.text_equal, true); assert.equal(normalized.source_equal, false);
    await panel().getByText('提取正文相同，但原件字节不同；BOM、换行、排版或其他原件信息可能不同。', { exact: true }).waitFor();
    checks.push('The actual UI distinguishes normalized text equality from byte equality for BOM and line-ending changes.');
    await select('比较目标版本', fixture.target_version_id); const pinned = await compare();
    const tampered = await context.request.get(origin + '/v1/files/' + fixture.file_id + '/text-comparison?base_version_id=' + fixture.base_version_id + '&target_version_id=' + fixture.target_version_id + '&view=all&offset=60&comparison_sha256=' + '0'.repeat(64), { headers: await headers() });
    assert.equal(tampered.status(), 409); assert.ok(!(await tampered.json()).data);
    if (!mobile) { await page.getByLabel('外观偏好', { exact: true }).selectOption('dark'); await panel().getByText('内容版本 1 → 2', { exact: true }).scrollIntoViewIfNeeded(); await shot('dark'); await page.getByLabel('外观偏好', { exact: true }).selectOption('light'); }
    const currentFile = await get('/files/' + fixture.file_id);
    const changed = await context.request.post(origin + '/v1/files/' + fixture.file_id + '/move', { headers: { ...await headers(), 'Idempotency-Key': randomUUID() }, data: { expected_version: currentFile.version, new_name: '版本核对（新修订）.md', reason: '隔离合成页面检查：验证比较分页遇到元数据修订后拒绝旧依据' } });
    assert.equal(changed.status(), 200);
    const staleResponse = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/text-comparison') && new URL(r.url()).searchParams.get('comparison_sha256') === pinned.comparison_sha256);
    await button('下一页正文差异').click(); const stale = await staleResponse; assert.equal(stale.status(), 409); assert.ok(!(await stale.json()).data);
    await panel().getByText(/比较依据|比较内容|重新比较|重新核对/).last().waitFor();
    assert.equal(await panel().getByText('内容版本 1 → 2', { exact: true }).count(), 0);
    await panel().scrollIntoViewIfNeeded(); await shot('stale');
    checks.push('Forged comparison pins and a real metadata revision reject old pages with no body; the actual UI hides the stale result.');
  });
  for (const width of mobile ? [390] : [1440, 390]) { await page.setViewportSize({ width, height: 900 }); const size = await (mobile ? root() : page.locator('html')).evaluate(node => ({ width: node.clientWidth, scroll: node.scrollWidth })); assert.ok(size.scroll <= size.width + 1); layouts.push({ width, ...size, passed: true }); }
  checks.push('Actual Web and shared H5 comparison controls remain inside the viewport; owned fixture, tabs and servers are released after checking.');
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ recordings: [recording], layouts, screenshots: shots, synthetic_data: true, real_browser_pixels: true, audio: false, comparison: { base_version_id: fixture.base_version_id, target_version_id: fixture.target_version_id, base_text_sha256: fixture.base_text_sha256, target_text_sha256: fixture.target_text_sha256, all_rows: pages.flatMap(p => p.rows).length, complete_body_reconstruction: true } }, null, 2) + '\n');
}, 'file-comparison');
