import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './oa_line_prose_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './oa_line_prose_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, context, fixture, checks, evidence }) => {
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号', { exact: true }).fill(fixture.alice.login);
  await page.getByLabel('密码', { exact: true }).fill(fixture.alice.password);
  await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  await page.getByLabel('六位验证码', { exact: true }).fill(await waitOTP(fixture));
  await page.getByRole('button', { name: '验证并继续', exact: true }).click();
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor();
  const shots = [], adoptions = [], layouts = [], prefix = 'oa-line-prose-web';
  const review = () => page.locator('.oa-prose-review');
  const button = name => page.getByRole('button', { name, exact: true });
  async function get(route) {
    const r = await context.request.get(fixture.origin + '/v1' + route);
    assert.equal(r.status(), 200); return (await r.json()).data;
  }
  async function openRun(id) {
    await page.goto(fixture.origin + '/conversations/' + fixture.conversation_id + '?run=' + id);
    await review().getByText('逐字段核对原文与建议，确认后保存业务新修订。', { exact: true }).waitFor();
    await review().scrollIntoViewIfNeeded();
  }
  async function shot(suffix) {
    await assertNoCredentials(page);
    const file = 'docs/media/screenshots/' + prefix + '-' + suffix + '.png';
    await mkdir(path.dirname(path.join(app, '..', file)), { recursive: true });
    const bytes = await page.screenshot({ path: path.join(app, '..', file) });
    shots.push({ file, sha256: createHash('sha256').update(bytes).digest('hex') });
  }
  await openRun(fixture.purchase_run_id);
  const original = await get('/runs/' + fixture.purchase_run_id);
  const lineKey = Object.keys(original.oa_request.text_draft.fields).find(v => v.startsWith('line:'));
  assert.ok(lineKey);
  assert.equal(original.oa_request.before.business.payload.content.lines.length, 2);
  await review().getByRole('heading', { name: /^明细 1 · / }).waitFor();
  for (const width of [390, 768, 1440]) {
    await page.setViewportSize({ width, height: 1100 });
    await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
    const size = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, width: document.documentElement.scrollWidth, overflow: [...document.querySelectorAll('main *')].filter(n=>n.getBoundingClientRect().right>innerWidth+2).slice(0,8).map(n=>({tag:n.tagName,class:n.className,right:n.getBoundingClientRect().right})) }));
    layouts.push({ width, ...size });
    await writeFile(path.join(evidence,'layout-probe.json'),JSON.stringify(layouts,null,2)+'\n');
    assert.ok(size.width <= width + 2,JSON.stringify(size));
  }
  await page.setViewportSize({ width: 1440, height: 1100 }); await review().scrollIntoViewIfNeeded();
  await shot('before');
  checks.push('Authenticated purchase review shows the exact selected original line and its proposed prose at three viewport widths; model completion remains draft.');
  async function adopt(kind) {
    const id = fixture[kind + '_run_id'], before = await get('/runs/' + id), view = before.oa_request;
    assert.equal(await button('采用正文草稿').isDisabled(), true);
    await page.getByLabel('采用正文的依据', { exact: true }).fill('本人已核对准确明细原文、建议及其他商业事实');
    assert.equal(await button('采用正文草稿').isDisabled(), true);
    await page.getByLabel('我已核对全部正文建议，确认采用', { exact: true }).check();
    const pending = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/runs/' + id + '/oa-text-adoption');
    await button('采用正文草稿').click();
    const response = await pending; assert.equal(response.status(), 200);
    const saved = (await response.json()).data, draft = saved.text_draft;
    assert.equal(saved.status, 'adopted'); assert.ok(draft.operation_id);
    assert.equal(draft.draft_sha256, view.text_draft.draft_sha256);
    assert.equal(draft.result.version, view.before.business.version + 1);
    const expected = structuredClone(view.before.business.payload.content);
    for (const [field, text] of Object.entries(draft.fields)) {
      if (field.startsWith('line:')) {
        const line = expected.lines.find(v => v.id === field.split(':')[1]); assert.ok(line); line.description = text;
      } else expected[field] = text;
    }
    assert.deepEqual(draft.result.payload.content, expected);
    await review().getByText('正文已采用并保存为业务新修订。', { exact: true }).waitFor();
    const readback = await get('/runs/' + id); assert.equal(readback.outcome, 'draft'); assert.equal(readback.oa_request.result, null);
    adoptions.push({ kind, fields: Object.keys(draft.fields), business_version: draft.result.version, exact_other_facts_preserved: true, approval_submitted: false, operation_confirmed: true });
  }
  const recording = await recordViewport(page, app, prefix, async () => {
    await adopt('purchase'); await review().scrollIntoViewIfNeeded(); await shot('adopted');
    await page.waitForTimeout(600); await openRun(fixture.claim_run_id); await adopt('claim'); await page.waitForTimeout(600);
  });
  checks.push('Actual reviewed purchase and claim adoption each creates one ordinary business revision and receipt; every unselected line and monetary, receipt and supplier fact matches the exact original.');
  await button('收起任务进度').click();
  await page.getByLabel('本次秘书任务', { exact: true }).selectOption(fixture.profile_id);
  await button('选择业务申请操作').click();
  await page.getByLabel('申请业务类型', { exact: true }).selectOption('claim');
  const claim = await get('/claims/' + fixture.target_claim_id);
  await page.getByLabel('申请业务资料', { exact: true }).selectOption(claim.id);
  await page.getByLabel('申请操作', { exact: true }).selectOption('text');
  const label = n => '明细 ' + n + ' · ' + [...claim.lines[n - 1].description.trim()].slice(0, 60).join('');
  await page.getByLabel(label(1), { exact: true }).check();
  assert.equal(await page.getByLabel(label(2), { exact: true }).isChecked(), false);
  await page.getByLabel('申请操作说明', { exact: true }).fill('仅起草首条报销明细说明和备注');
  await page.getByText('起草字段', { exact: true }).scrollIntoViewIfNeeded(); await shot('selection');
  await button('将准确申请操作加入请求').click();
  await page.getByLabel('本次消息内容', { exact: true }).fill('请按我选择的准确明细整理说明，保留其他事实。');
  const pending = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/v1/conversations/' + fixture.conversation_id + '/runs');
  await button('确认发送本次请求').click();
  const sent = await pending; assert.equal(sent.status(), 202);
  const payload = sent.request().postDataJSON(), run = (await sent.json()).data;
  assert.equal(payload.oa_request.tool, 'draft_business_text');
  assert.deepEqual(payload.oa_request.arguments.fields, ['notes', 'line:' + claim.lines[0].id + ':description']);
  assert.equal(payload.oa_request.arguments.business_revision_id, claim.current_revision_id);
  assert.equal(run.oa_request.status, 'authorized'); assert.equal(run.oa_request.text_draft, undefined);
  checks.push('Real claim selection distinguishes identical descriptions by position and exact ID, sends only the selected line and notes and queues an authorized run without fabricating model execution.');
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ recording, screenshots: shots, layouts, adoptions, synthetic_data: true, scope: 'Real Web/native HTTP/PG with controlled MCP fixture; no external model, cloud, device or production acceptance.' }, null, 2) + '\n');
}, 'oa-line-prose');
