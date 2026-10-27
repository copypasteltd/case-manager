import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
import { mobileScreen, selectMobile, stableMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './agent_indexed_search_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page.locator('body');
  const field = name => mobile ? root().locator('input[aria-label="' + name + '"]') : page.getByLabel(name, { exact: true });
  const button = name => root().getByRole('button', { name, exact: true });
  let bearer, sends = 0;
  page.on('request', request => {
    if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization;
    if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/runs')) sends++;
  });
  await page.goto(fixture.origin);
  await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  await field('六位验证码').fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  if (mobile) await root().locator('.cp-title').filter({ hasText: '工作从这里开始' }).waitFor();
  else await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const route = fixture.origin + (mobile ? '/#/pages/secretary/index?kind=conversation&id=' : '/conversations/') + fixture.conversation_id;
  await page.goto(route);
  if (mobile) await root().locator('.cp-title').filter({ hasText: '正文索引与准确版本会话' }).waitFor();
  else await page.getByRole('heading', { name: '给公司秘书的请求', exact: true }).waitFor();
  checks.push('actual-authenticated-conversation-and-original-source-preparation');
  const screenshots = path.join(app, '../docs/media/screenshots'); await mkdir(screenshots, { recursive: true });
  const prefix = 'agent-indexed-search-' + (mobile ? 'mobile' : 'web');
  const get = async route => {
    if (mobile) assert.ok(bearer); const response = await context.request.get((mobile ? fixture.api_origin : fixture.origin) + '/v1' + route, { headers: mobile ? { Authorization: bearer } : {} });
    assert.equal(response.status(), 200); return (await response.json()).data;
  };
  async function selectVersion(seq, id) {
    if (mobile) {
      await button('核对并选择上下文资料').click(); await selectMobile(page, '上下文资料类型', '准确文件版本正文');
      await root().locator('.cp-row-title').getByText('秘书设计资料.docx', { exact: true }).click();
      assert.equal(await button('核对并预览资料').isDisabled(), true);
      await root().locator('.cp-row-title').filter({ hasText: new RegExp('^版本 ' + seq + ' ·') }).click();
    } else {
      await page.getByLabel('资料类型', { exact: true }).selectOption('file');
      const select = page.getByLabel('选择资料', { exact: true }); await select.locator('option[value="' + fixture.file_id + '"]').waitFor({ state: 'attached' }); await select.selectOption(fixture.file_id);
      const versions = page.getByLabel('准确文件版本', { exact: true }); await versions.locator('option[value="' + id + '"]').waitFor({ state: 'attached' }); await versions.selectOption(id);
    }
  }
  async function preview(next = false) {
    const response = page.waitForResponse(r => new URL(r.url()).pathname.includes('/context-sources/file/'));
    response.catch(() => {});
    await button(next ? (mobile ? '核对下一段资料' : '核对下一段') : (mobile ? '核对并预览资料' : '核对并预览')).click();
    const received = await response; assert.equal(received.status(), 200); return (await received.json()).data;
  }
  const video = await recordViewport(page, app, prefix, async () => {
    await selectVersion(2, fixture.version_id);
    checks.push('explicit-formal-file-and-version-selection');
    const first = await preview();
    assert.equal(first.version_id, fixture.version_id); assert.ok(first.text.startsWith(fixture.text_prefix)); assert.equal(first.untrusted_data, true);
    assert.equal(first.text_index.state, 'ready'); assert.equal(first.text_index.extractor, 'text-v1');
    assert.match(first.text_index.text_sha256, /^[a-f0-9]{64}$/); assert.ok(Number.isFinite(Date.parse(first.text_index.indexed_at)));
    assert.equal(first.text_index.source_sha256, first.evidence[0].sha256); assert.equal(first.text_index.total_characters, first.total_chars);
    checks.push('actual-DOCX-body-binds-original-and-derived-digests');
    await root().getByText('固定版本派生正文', { exact: true }).scrollIntoViewIfNeeded();
    await root().getByText(first.text_index.text_sha256, { exact: true }).waitFor();
    await assertNoCredentials(page); await page.screenshot({ path: path.join(screenshots, prefix + '-preview.png'), fullPage: true });
    checks.push('extractor-time-digest-and-format-limits-visible');
    assert.notEqual(first.next_offset, null); const second = await preview(true);
    assert.equal(second.offset, first.next_offset); assert.equal(second.source_sha256, first.source_sha256); assert.deepEqual(second.text_index, first.text_index);
    checks.push('Unicode-continuation-retains-exact-derivation');
    await button('将这份准确资料加入本次请求').click();
    await selectVersion(1, fixture.old_version_id); const old = await preview();
    assert.equal(old.version_id, fixture.old_version_id); assert.ok(old.text.startsWith('旧版正文独有词🙂')); assert.notEqual(old.text_index.text_sha256, first.text_index.text_sha256);
    checks.push('historical-preview-does-not-follow-current-head');
    await button('将这份准确资料加入本次请求').click();
    if (mobile) await root().getByText('给公司秘书的请求', { exact: true }).scrollIntoViewIfNeeded();
    else await page.getByRole('heading', { name: '给公司秘书的请求', exact: true }).scrollIntoViewIfNeeded();
    await assertNoCredentials(page); await page.screenshot({ path: path.join(screenshots, prefix + '-sources.png'), fullPage: true });
    assert.equal(sends, 0); checks.push('both-exact-context-versions-attached-without-implicit-send');
  });
  const completed = await get('/runs/' + fixture.completed_run_id); assert.equal(completed.state, 'succeeded');
  const messages = await get('/conversations/' + fixture.conversation_id + '/messages?limit=100');
  const answer = messages.items.find(message => message.run_id === fixture.completed_run_id && message.role === 'assistant');
  assert.ok(answer); assert.equal(answer.visibility, 'unavailable'); assert.equal(answer.body, ''); assert.deepEqual(answer.source_refs, []);
  checks.push('historical-native-MCP-result-withdraws-after-fixture-authority-and-lifecycle-changes');
  if (mobile) await stableMobile(page);
  await writeFile(path.join(evidence, 'capture.json'), JSON.stringify({ synthetic_data: true, recordings: [video] }, null, 2) + '\n');
  await writeFile(path.join(evidence, 'acceptance.json'), JSON.stringify({ passed: true, actual_native_api: true, real_cloud: false, real_model: false,
    actual_device: false, controlled_MCP_result: true, historical_result_withdrawal: true, implicit_sends: sends, scope: 'Actual Web/H5 preview, continuation and attachment; controlled native MCP result, no live model.' }, null, 2) + '\n');
}, 'agent-indexed-search');
