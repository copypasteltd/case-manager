// Actual pinned extraction, native HTTP/PostgreSQL and authenticated UI pixels.
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
  assert.ok(fixture.document_text_fixture && fixture.isolated_database);
  const root = () => mobile ? mobileScreen(page) : page;
  const field = name => mobile ? root().locator('input[aria-label="' + name + '"],textarea[aria-label="' + name + '"]') : page.getByLabel(name, { exact: true });
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
  await field('搜索工作资料').fill('INVOICE');
  const searching = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/search'); void searching.catch(() => {});
  await button('搜索资料').click(); const search = await searching; assert.equal(search.status(), 200);
  const hits = (await search.json()).data.items; assert.equal(hits.length, 3);
  for (const hit of hits) assert.equal(hit.text_extractor, 'document-ocr-v1');
  assert.ok(hits.some(h => h.resource.id === fixture.file_id && h.resource.version_id === fixture.target_version_id));
  checks.push('Actual search finds OCR inside current PDF/PNG/JPEG originals with explicit extraction identity and no duplicate legacy rows.');
  const screenshots = [], documents = [], directory = path.join(app, '../docs/media/screenshots'); await mkdir(directory, { recursive: true });
  const prefix = 'document-text-' + (mobile ? 'mobile' : 'web');
  async function shot(name) { await assertNoCredentials(page); const file = prefix + '-' + name + '.png'; await page.screenshot({ path: path.join(directory, file) }); screenshots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) }); }
  const recording = await recordViewport(page, app, prefix, async () => {
    for (const [title, file, version, pages, name] of [['扫描交付.pdf', fixture.file_id, fixture.target_version_id, 2, 'pdf'], ['验收图片.png', fixture.image_file_id, fixture.image_version_id, 1, 'image']]) {
      const hit = root().locator(mobile ? '.cp-search-hit' : '.search-hit').filter({ hasText: title });
      const response = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/files/' + file + '/versions/' + version + '/text'); void response.catch(() => {});
      await hit.getByRole('button', { name: '查阅此版本正文', exact: true }).click(); const result = await response; assert.equal(result.status(), 200);
      const body = (await result.json()).data; assert.equal(body.extractor, 'document-ocr-v1'); assert.equal(body.version_id, version); assert.equal(sha(body.text), body.text_sha256); assert.equal(body.next_offset, null);
      assert.equal(body.native_evidence.page_count, pages); assert.equal(body.native_evidence.ocr_page_count, pages); assert.match(body.native_evidence.engine_sha256, /^[a-f0-9]{64}$/);
      for (const expected of ['INVOICE', '123.45', 'OCR识别']) assert.ok(body.text.includes(expected), expected);
      if (pages === 2) for (const expected of ['CP128 embedded document text', 'CP128 DELIVERY READY', 'PDF第2页']) assert.ok(body.text.includes(expected), expected);
      documents.push({ file_id: file, version_id: body.version_id, extractor: body.extractor, source_sha256: body.source_sha256, text_sha256: body.text_sha256, native_evidence: body.native_evidence });
      const preview = mobile ? root().locator('.cp-section').filter({ hasText: title + ' · 正文' }) : page.getByRole('region', { name: '固定版本正文', exact: true });
      await preview.getByText(/PDF逐页保留内嵌文字及OCR结果/).waitFor();
      await preview.getByText(new RegExp('完整处理 ' + pages + ' 页')).waitFor();
      const prose = preview.locator(mobile ? '.cp-search-snippet' : '.file-text-body'); await prose.waitFor();
      if (mobile) await prose.evaluate(node => node.scrollIntoView({ block: 'start' })); else await prose.scrollIntoViewIfNeeded();
      await shot(name); await button('关闭正文').click();
    }
    checks.push('Actual PDF and image previews bind complete native page evidence, unchanged original hashes and explicit OCR/financial verification limits.');
    const versions = (await get('/files/' + fixture.file_id + '/versions')).items;
    await page.goto(fixture.origin + (mobile ? '/#/pages/files/index?file_id=' : '/files/') + fixture.file_id);
    const panel = mobile ? root().locator('.cp-section').filter({ hasText: '比较两个内容版本' }) : page.getByRole('region', { name: '文件版本正文比较', exact: true }); await panel.waitFor();
    for (const [label, id] of [['比较基础版本', fixture.base_version_id], ['比较目标版本', fixture.target_version_id]]) {
      if (mobile) await selectMobile(page, label, '内容版本 ' + versions.find(v => v.id === id).seq + ' · ' + id.slice(0, 8)); else await page.getByLabel(label, { exact: true }).selectOption(id);
    }
    const pending = page.waitForResponse(r => new URL(r.url()).pathname.endsWith('/text-comparison')); void pending.catch(() => {});
    await button('比较所选版本').click(); const response = await pending; assert.equal(response.status(), 200); const comparison = (await response.json()).data;
    assert.equal(comparison.extractor, 'document-ocr-v1'); assert.equal(comparison.base.native_evidence.page_count, 1); assert.equal(comparison.target.native_evidence.page_count, 2); assert.equal(comparison.text_equal, false);
    await panel.getByText('内容版本 1 → 2', { exact: true }).waitFor(); await panel.getByText(/完整处理 2 页/).waitFor(); await panel.scrollIntoViewIfNeeded(); await shot('comparison');
    checks.push('Actual selected-version comparison binds full page/engine evidence for both PDF versions and preserves original-byte/text equality separation.');
    const conversation = await get('/conversations/' + fixture.conversation_id);
    const source = await get('/conversations/' + fixture.conversation_id + '/context-sources/file/' + fixture.file_id + '?context_security_version=' + conversation.context_security_version + '&version_id=' + fixture.target_version_id + '&max_chars=10000');
    assert.equal(source.text_index.native_evidence.page_count, 2); assert.equal(source.text_index.text_sha256, documents[0].text_sha256); assert.ok(source.text.includes('OCR识别'));
    checks.push('Authenticated secretary-source reads use the same immutable native derivation with object readers removed; no real model was invoked.');
  });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, documents, synthetic_data: true, real_native_extraction: true, real_model: false, real_cloud: false }, null, 2) + '\n');
}, 'document-text');
