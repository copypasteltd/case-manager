import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { officeCreateBrowser, waitOTP } from './agent_office_create_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './agent_office_create_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await officeCreateBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const screenshots = [], prefix = 'agent-office-create-web';
  const name = 'Web生成会议纪要.docx', media = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
  const route = '/v1/conversations/' + fixture.conversation_id + '/runs';
  let sends = 0;
  page.on('request', request => { if (new URL(request.url()).pathname === route && request.method() === 'POST') sends++; });
  async function get(route) {
    const response = await context.request.get(fixture.origin + '/v1' + route); assert.equal(response.status(), 200, 'Ordinary GET ' + route);
    return (await response.json()).data;
  }
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.join(app, '../docs/media/screenshots'), { recursive: true });
    const bytes = await page.screenshot({ fullPage: false });
    await writeFile(path.join(app, '..', file), bytes);
    screenshots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  await page.goto(fixture.origin + '/conversations/' + fixture.conversation_id);
  await page.getByLabel('本次秘书任务', { exact: true }).selectOption(fixture.profile_id);
  const recording = await recordViewport(page, app, prefix, async () => {
    await page.getByRole('button', { name: '选择新建文件位置', exact: true }).click();
    await page.getByLabel('新文件资料目录', { exact: true }).selectOption(fixture.folder_id);
    await page.getByText(/目标：.+修订/).waitFor();
    await page.getByLabel('新文件格式', { exact: true }).selectOption(media);
    await page.getByLabel('新文件名称', { exact: true }).fill(name);
    await page.getByLabel('新文件格式', { exact: true }).scrollIntoViewIfNeeded();
    assert.equal(sends, 0);
    await shot('scope');
    await page.getByRole('button', { name: '将准确文件新建加入请求', exact: true }).click();
    assert.equal(sends, 0);
    assert.equal(await page.getByRole('button', { name: '选择准确文件更新', exact: true }).isDisabled(), true);
    checks.push('Selecting an exact directory/name/Word format creates no run and cannot mix an existing-file update.');
    const folder = await get('/folders/' + fixture.folder_id);
    await page.getByLabel('本次消息内容', { exact: true }).fill('请新建Word项目会议纪要：香港团队，已确认交付事项，等待员工核对完整文件，并附事项与责任表格。');
    const [sent] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === route && response.request().method() === 'POST'),
      page.getByRole('button', { name: '确认发送本次请求', exact: true }).click(),
    ]);
    assert.equal(sent.status(), 202);
    const input = sent.request().postDataJSON(), run = (await sent.json()).data;
    assert.deepEqual(input.file_create, { folder_id: fixture.folder_id, expected_folder_version: folder.version, name, media_type: media });
    assert.equal(input.file_update, undefined); assert.equal(run.file_create.status, 'authorized');
    assert.equal(run.file_create.operation_id, null); assert.deepEqual(run.file_create.drafts, []);
    await page.getByText(name + ' · 尚未提交正式版本', { exact: true }).waitFor();
    await page.getByText(name, { exact: false }).last().scrollIntoViewIfNeeded();
    await shot('pending');
    checks.push('One actual Web request pins the Office media/directory revision/name and returns authorization without a formal artifact.');
    const messages = await get('/conversations/' + fixture.conversation_id + '/messages?limit=100');
    const message = messages.items.find(value => value.run_id === run.id && value.role === 'user');
    assert.equal(message.source_refs[0].type, 'agent_file_create');
    assert.equal(message.source_refs[0].sha256, run.file_create.intent_sha256);
    checks.push('The real user message pins the immutable exact creation intent rather than a model-provided filename.');
    const process = await context.request.post(fixture.api_origin + '/__fixture/office-create/complete', {
      headers: { Authorization: 'Bearer ' + fixture.stop_token }, data: { run_id: run.id },
    });
    assert.equal(process.status(), 200, 'Owned controlled Office execution callback');
    const actual = await process.json();
    const completed = await get('/runs/' + run.id), version = completed.file_create.result.file_version;
    assert.equal(completed.state, 'succeeded'); assert.equal(completed.file_create.status, 'committed');
    assert.equal(version.id, actual.version_id); assert.equal(version.sha256, actual.sha256); assert.equal(version.media_type, media);
    assert.equal(version.source, 'agent'); assert.equal(version.scan_state, 'clean'); assert.equal(version.seq, 1); assert.equal(version.base_version_id, null);
    const exactPath = '/files/' + actual.file_id + '?version=' + actual.version_id;
    const exact = page.locator('a[href="' + exactPath + '"]');
    await exact.waitFor();
    const receipt = exact.locator('xpath=ancestor::section[1]');
    await receipt.scrollIntoViewIfNeeded();
    await receipt.getByText('Office内容摘录', { exact: true }).waitFor();
    assert.equal(await receipt.getByText(/服务端计算差异.*BOM/).isVisible(), false);
    await shot('committed');
    checks.push('The controlled model driver executes real tools/hash/scan/promotion/commit; the actual Web receipt identifies Office text excerpts and complete binary evidence.');
    await exact.click(); await page.waitForURL(fixture.origin + exactPath);
    await page.getByRole('heading', { name: '本次引用的内容版本', exact: true }).waitFor();
    const ordinary = await get('/files/' + actual.file_id + '/versions/' + actual.version_id + '/office');
    assert.equal(ordinary.source_sha256, actual.sha256);
    assert.equal(ordinary.workspace.sha256, completed.file_create.drafts[0].diff.office.workspace_sha256);
    assert.equal(sends, 1);
    checks.push('The real completed receipt opens the ordinary exact file/version entry and independently reads the generated Office workspace.');
  });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots, recording, synthetic_data: true, real_browser_pixels: true,
    scope: 'Actual Web selection/send/pending/complete/exact-version read; controlled native model driver and synthetic object transport, not real Codex or cloud.',
    real_model_invoked: false, real_cloud_invoked: false, mobile_ui_executed: false }, null, 2) + '\n');
}, 'agent-office-create');
