// Real native API/PG/extraction and actual UI pixels; no response replacement.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './office_text_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url)), mobile = process.argv.includes('--mobile');
const sha = value => createHash('sha256').update(value).digest('hex');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  assert.ok(fixture.docx_text_fixture && fixture.isolated_database);
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = name => root().getByRole('button', { name, exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  await page.goto(fixture.origin); await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click(); await field('六位验证码').fill(await waitOTP(fixture));
  await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor(); else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, { headers: mobile ? { Authorization: bearer } : {} }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  await page.goto(fixture.origin + (mobile ? '/#/pages/search/index' : '/search'));
  await field('搜索工作资料').fill('批注需财务确认');
  const searching = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/search'); void searching.catch(() => {});
  await button('搜索资料').click(); const search = await searching; assert.equal(search.status(), 200);
  const hits = (await search.json()).data.items; assert.equal(hits.length, 3);
  assert.ok(hits.some(h => h.resource.id === fixture.file_id && h.resource.version_id === fixture.target_version_id));
  checks.push('Actual current-head search finds referenced DOCX comments, pins formal versions and does not duplicate legacy derivations.');
  const screenshots = [], documents = [], directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  const prefix = 'docx-text-' + (mobile ? 'mobile' : 'web');
  async function shot(name) { await assertNoCredentials(page); const file = prefix + '-' + name + '.png'; await page.screenshot({ path: path.join(directory, file) }); screenshots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) }); }
  const recording = await recordViewport(page, app, prefix, async () => {
    const hit = root().locator(mobile ? '.cp-search-hit' : '.search-hit').filter({ hasText: '合同审阅.docx' });
    const response = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/files/' + fixture.file_id + '/versions/' + fixture.target_version_id + '/text'); void response.catch(() => {});
    await hit.getByRole('button', { name: '查阅此版本正文', exact: true }).click(); const result = await response; assert.equal(result.status(), 200);
    const body = (await result.json()).data; assert.equal(body.extractor, 'docx-v2'); assert.equal(body.version_id, fixture.target_version_id); assert.equal(sha(body.text), body.text_sha256); assert.equal(body.next_offset, null);
    for (const expected of ['[插入]新增条款', '[删除]删除条款', '[表格]', 'HKD 1200', '公司页眉', '合同页脚', '脚注付款依据', '尾注有效期', '批注需财务确认', '文本框交付说明', '字段指令，不执行', '缓存结果，可能过期']) assert.ok(body.text.includes(expected), expected);
    assert.ok(!body.text.includes('DO_NOT_INDEX') && !body.text.includes('legacy-main-only'));
    documents.push({ file_id: fixture.file_id, version_id: body.version_id, extractor: body.extractor, source_sha256: body.source_sha256, text_sha256: body.text_sha256 });
    const preview = mobile ? root().locator('.cp-section').filter({ hasText: '合同审阅.docx · 正文' }) : page.getByRole('region', { name: '固定版本正文', exact: true });
    await preview.getByText(/Word 正文为文字提取/).waitFor();
    const prose = preview.locator(mobile ? '.cp-search-snippet' : '.file-text-body'); await prose.waitFor();
    if (mobile) await prose.evaluate(node => node.scrollIntoView({ block: 'start' })); else await prose.scrollIntoViewIfNeeded();
    await shot('revisions');
    if (mobile) await prose.evaluate(node => node.scrollIntoView({ block: 'end' })); else await prose.evaluate(node => { node.scrollTop = node.scrollHeight; });
    await shot('stories'); await button('关闭正文').click();
    checks.push('Actual preview renders labeled revisions, table cells, referenced headers/footers/notes/comments, textboxes and inert field/cache evidence; the complete derived hash matches the API.');
    const versions = (await get('/files/' + fixture.file_id + '/versions')).items;
    await page.goto(fixture.origin + (mobile ? '/#/pages/files/index?file_id=' : '/files/') + fixture.file_id);
    const panel = mobile ? root().locator('.cp-section').filter({ hasText: '比较两个内容版本' }) : page.getByRole('region', { name: '文件版本正文比较', exact: true });
    await panel.waitFor();
    for (const [label, id] of [['比较基础版本', fixture.base_version_id], ['比较目标版本', fixture.target_version_id]]) {
      if (mobile) await selectMobile(page, label, '内容版本 ' + versions.find(v => v.id === id).seq + ' · ' + id.slice(0, 8)); else await page.getByLabel(label, { exact: true }).selectOption(id);
    }
    const comparisonResponse = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/text-comparison')); void comparisonResponse.catch(() => {});
    await button('比较所选版本').click(); const comparisonResult = await comparisonResponse; assert.equal(comparisonResult.status(), 200); const comparison = (await comparisonResult.json()).data;
    assert.equal(comparison.extractor, 'docx-v2'); assert.equal(comparison.base.extractor, 'docx-v2'); assert.equal(comparison.target.extractor, 'docx-v2'); assert.equal(comparison.text_equal, false); assert.ok(comparison.rows.some(r => r.text.includes('香港Word当前合同')));
    await panel.getByText('内容版本 1 → 2', { exact: true }).waitFor(); await panel.getByText(/Word 正文为文字提取/).waitFor(); await panel.scrollIntoViewIfNeeded(); await shot('comparison');
    checks.push('Actual selected-version comparison binds docx-v2 independently on both formal versions and keeps revision/layout limitations visible.');
    const conversation = await get('/conversations/' + fixture.conversation_id);
    const source = await get('/conversations/' + fixture.conversation_id + '/context-sources/file/' + fixture.file_id + '?context_security_version=' + conversation.context_security_version + '&version_id=' + fixture.target_version_id + '&max_chars=10000');
    assert.equal(source.text_index.extractor, 'docx-v2'); assert.equal(source.text_index.text_sha256, documents[0].text_sha256); assert.ok(source.text.includes('批注需财务确认'));
    checks.push('The authenticated secretary-source API consumes the same immutable DOCX derivation after source-object credentials are removed; this is not real-model acceptance.');
  });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, documents, synthetic_data: true, real_model: false, real_cloud: false }, null, 2) + '\n');
}, 'docx-text');
