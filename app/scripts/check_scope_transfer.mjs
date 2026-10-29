// Actual human UI decisions against the native domain fixture; no response mocks.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { scope_transferBrowser, waitOTP } from './scope_transfer_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './scope_transfer_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
await scope_transferBrowser(mobile, async ({ page, context, fixture, checks, evidence, openPage }) => {
  const root = (p = page) => mobile ? mobileScreen(p) : p;
  const button = (label, p = page) => root(p).getByRole('button', { name: label, exact: true });
  const field = (label, p = page) => mobile ? root(p).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : p.getByLabel(label, { exact: true });
  let bearer;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  async function login(p, who, mfa = false) {
    await p.goto(fixture.origin);
    await field('内部账号', p).fill(who.login); await field('密码', p).fill(who.password);
    await button(mobile ? '登录工作空间 ↗' : '登录工作空间', p).click();
    if (mfa) { await field('六位验证码', p).fill(await waitOTP(fixture)); await button(mobile ? '验证并继续 ↗' : '验证并继续', p).click(); }
    await root(p).getByText('业务待办', { exact: true }).waitFor();
  }
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = () => mobile ? { Authorization: bearer } : { Origin: fixture.origin };
  async function get(route) { const response = await context.request.get(origin + '/v1' + route, { headers: headers() }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  await login(page, fixture.alice, true);
  const source = await get('/files/' + fixture.transfer_file_id), versions = await get('/files/' + source.id + '/versions?limit=20');
  const target = await get('/folders/' + fixture.transfer_target_folder);
  async function openFile(p) {
    if (mobile) { await root(p).getByText('文件与资料', { exact: true }).click(); await root(p).getByText(source.name, { exact: true }).click(); }
    else await p.goto(fixture.origin + '/files/' + source.id);
    await root(p).getByText('跨项目与分类整理', { exact: true }).waitFor({ state: p === page ? 'visible' : 'hidden' }).catch(async e => { if (p === page) throw e; });
  }
  const oldReader = await openPage({ viewport: mobile ? { width: 390, height: 900 } : { width: 1440, height: 1100 } });
  await login(oldReader.page, fixture.bob);
  // Bob has ordinary source access but no organization policy administration.
  if (mobile) { await root(oldReader.page).getByText('文件与资料', { exact: true }).click(); await root(oldReader.page).getByText(source.name, { exact: true }).click(); }
  else await oldReader.page.goto(fixture.origin + '/files/' + source.id);
  await root(oldReader.page).getByText(mobile ? '文件记录' : '内容版本与存储记录', { exact: true }).waitFor();
  assert.equal(await root(oldReader.page).getByText('拟定跨范围移动', { exact: true }).count(), 0);
  await openFile(page);
  checks.push({ name: 'Genuine MFA owner and a separate ordinary source reader receive their actual permitted UI', passed: true });

  await button('拟定跨范围移动').click();
  const targetRow = root().locator(mobile ? '.cp-fact' : '.report-context').filter({ hasText: target.name });
  // The displayed actual project name disambiguates similarly named roots.
  await targetRow.getByRole('button', { name: mobile ? '选择此目标' : '选择目标 ' + target.name, exact: true }).click();
  await field('跨范围整理原因').fill('人工核对两个项目的完整读取权限，整理此合成文件。');
  const [proposal] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/v1/file-scope-transfers' && r.request().method() === 'POST'),
    button('分析跨范围移动').click(),
  ]);
  assert.equal(proposal.status(), 201); const plan = (await proposal.json()).data;
  assert.equal(plan.input.resource_id, source.id); assert.equal(plan.snapshot.target.id, target.id);
  assert.equal(plan.state, 'pending'); assert.equal(plan.can_execute, true); assert.equal(plan.snapshot.items.length, 1);
  assert.ok(plan.snapshot.items[0].visibility_changes.some(v => v.display_name === 'bob' && v.channel === 'web' && !v.ai && v.before && !v.after));
  assert.equal((await get('/files/' + source.id)).folder_id, source.folder_id);
  await root().getByText('普通读取策略组合', { exact: false }).last().waitFor();
  checks.push({ name: 'Actual target picker creates one exact non-mutating plan and exposes complete changed policy combinations', passed: true });

  const prefix = 'scope-transfer-' + (mobile ? 'mobile' : 'web'), directory = path.join(app, '../docs/media/screenshots'), shots = [];
  await mkdir(directory, { recursive: true });
  async function shot(suffix) { await assertNoCredentials(page); const file = prefix + '-' + suffix + '.png'; await page.screenshot({ path: path.join(directory, file), fullPage: false }); shots.push({ file: 'docs/media/screenshots/' + file, sha256: sha(await readFile(path.join(directory, file))) }); }
  await root().getByText(mobile ? '跨范围移动计划详情' : '待人工核对', { exact: true }).last().scrollIntoViewIfNeeded(); await shot('plan');
  const originalViewport = page.viewportSize(); await page.setViewportSize({ width: mobile ? 360 : 760, height: 1000 });
  await root().getByText(mobile ? '跨范围移动计划详情' : '待人工核对', { exact: true }).last().scrollIntoViewIfNeeded();
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2)); await shot('narrow');
  const recording = await recordViewport(page, app, prefix, async () => {
    await root().getByText('普通读取策略组合', { exact: false }).last().scrollIntoViewIfNeeded(); await page.waitForTimeout(900);
    if (mobile) { await button('刷新计划与当前依据').click(); await root().getByText('跨范围移动计划详情', { exact: true }).last().waitFor(); }
    else { const [r] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1/file-scope-transfers/' + plan.id && r.request().method() === 'GET'), button('重新核对这份计划').click()]); assert.equal(r.status(), 200); }
    await button('核对并执行跨范围移动').scrollIntoViewIfNeeded(); await page.waitForTimeout(900);
  });
  await page.setViewportSize(originalViewport);
  checks.push({ name: 'Responsive exact-plan review is recorded from real authenticated pixels without password or MFA fields', passed: true });

  await button('核对并执行跨范围移动').click();
  await field('移动计划确认原因').fill('本人已逐项核对读取权限变化和完整清单，确认移动。');
  if (mobile) {
    await root().getByText('我已核对全部清单、读取权限变化及本次操作影响', { exact: true }).click();
    // Login consumed a TOTP counter. Wait for a fresh one without recording it.
    await page.waitForTimeout(30000 - Date.now() % 30000 + 250);
    await field('验证器六位验证码').fill(await waitOTP(fixture)); await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。', { exact: true }).waitFor();
  } else await page.getByRole('checkbox', { name: '我已核对全部清单、读取权限变化及本次操作影响', exact: true }).check();
  const [execution] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/v1/file-scope-transfers/' + plan.id + '/execute' && r.request().method() === 'POST'),
    button('确认并执行跨范围移动').click(),
  ]);
  assert.equal(execution.status(), 200); const receipt = (await execution.json()).data;
  assert.equal(receipt.state, 'executed'); assert.equal(receipt.plan_hash, plan.plan_hash); assert.equal(receipt.version, 2);
  await root().getByText('已完成跨范围移动', { exact: true }).last().waitFor();
  const after = await get('/files/' + source.id), history = await get('/files/' + source.id + '/versions?limit=20');
  assert.equal(after.folder_id, target.id); assert.equal(after.project_id, target.project_id); assert.equal(after.version, source.version + 1);
  assert.deepEqual(history.items, versions.items);
  checks.push({ name: 'Explicit acknowledgement and current human MFA execute the exact immutable plan while preserving all content versions', passed: true });
  await root().getByText('已完成跨范围移动', { exact: true }).last().scrollIntoViewIfNeeded(); await shot('receipt');

  if (mobile) { await button('打开移动后的资料').click(); await root().getByText('当前目录', { exact: true }).waitFor(); await root().getByText(target.name + ' · 修订 ' + target.version, { exact: true }).waitFor(); }
  else { await page.getByRole('link', { name: '返回所在目录', exact: true }).click(); await page.waitForURL(url => url.pathname === '/files/folders/' + target.id); }
  checks.push({ name: 'The ordinary actual destination navigation resolves the new file scope after execution', passed: true });
  if (mobile) await root(oldReader.page).getByRole('button', { name: '刷新当前资料', exact: true }).click();
  else await oldReader.page.reload();
  await root(oldReader.page).getByText(/不存在|无权|不可见|权限|未找到/).first().waitFor();
  assert.equal(await root(oldReader.page).getByText('文件记录', { exact: true }).count(), 0);
  checks.push({ name: 'A separate former project reader loses the moved resource on an actual authorized refresh', passed: true });
  await writeFile(path.join(evidence, 'media.json'), JSON.stringify({ screenshots: shots, recording, synthetic_data: true, real_device: false, api_responses_mocked: false }, null, 2) + '\n');
});
