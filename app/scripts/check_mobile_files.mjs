import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { recordViewport } from './mobile_files_recording.mjs';
import { otp } from './notification_browser_common.mjs';
import { mobileScreen as screen, selectMobile as select, stableMobile as stable } from './mobile_picker_browser.mjs';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const evidence = path.join(app, '../docs/quality/evidence/mobile-files-browser');
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.isolated_database, true);
const manifest = JSON.parse(await readFile(path.join(evidence, 'source-manifest.json'), 'utf8'));
async function digest() { const h = createHash('sha256'); for (const file of manifest.files) { h.update(file + '\0'); h.update(await readFile(path.join(app, file))); h.update('\0'); } return h.digest('hex'); }
const before = await digest(); assert.equal(before, manifest.sha256);
const temporary = path.join(app, '.local/browser-temp'); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
const executablePath = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const contexts = [], closedContexts = new Set(), checks = [], layouts = [], errors = [], serverErrors = [], cleanupErrors = [], recordings = [], screenshots = [];
const button = (page, name) => screen(page).getByRole('button', { name, exact: true });
const field = (page, label) => screen(page).locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]');
let browser, mainPage, passed = false, failure;
async function heading(page, title) { await screen(page).locator('.cp-title').filter({ hasText: title }).waitFor(); }
async function navigate(page, url) { await page.evaluate(value => { window.location.hash = '#' + value; }, url); await stable(page); }
async function closeOwned(context) { if (!closedContexts.has(context)) { await context.close(); closedContexts.add(context); } }
async function login(account, existing) {
  const context = existing?.context ?? await browser.newContext({ viewport: { width: 390, height: 900 }, reducedMotion: 'reduce' }); if (!existing) contexts.push(context);
  const page = existing?.page ?? await context.newPage(); page.setDefaultTimeout(25000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', r => { if (r.status() >= 500) serverErrors.push({ status: r.status(), path: new URL(r.url()).pathname }); });
  await page.goto(fixture.origin); await field(page, '内部账号').fill(account.login); await field(page, '密码').fill(account.password);
  const pending = page.waitForResponse(r => r.url().endsWith('/v1/auth/login') && r.request().method() === 'POST'); await button(page, '登录工作空间 ↗').click(); let result = await (await pending).json();
  if (account.mfa_secret) {
    while (Math.floor(Date.now() / 30000) <= account.last_step) await page.waitForTimeout(500);
    const step = Math.floor(Date.now() / 30000); await screen(page).locator('input[maxlength="6"]').fill(otp(account.mfa_secret, step));
    const verified = page.waitForResponse(r => r.url().endsWith('/v1/auth/mfa/verify') && r.request().method() === 'POST'); await button(page, '验证并继续 ↗').click(); result = await (await verified).json(); account.last_step = step;
  }
  assert.equal(result.data?.step, 'authenticated'); await heading(page, '工作从这里开始'); return { page, context, token: result.data.access_token };
}
async function response(token, route, method = 'GET', body) {
  return fetch(fixture.api_origin + '/v1' + route, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...(method === 'GET' ? {} : { 'Idempotency-Key': 'files-setup:' + randomUUID() }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
}
async function request(token, route, method, body) { const r = await response(token, route, method, body), data = await r.json(); assert.ok(r.ok, `${method ?? 'GET'} ${route}: ${r.status} ${data.error?.code ?? ''}`); return data.data; }
async function submit(page, label, route, method, status = 200) {
  const [r] = await Promise.all([page.waitForResponse(r => new URL(r.url()).pathname === '/v1' + route && r.request().method() === method), button(page, label).click()]);
  assert.equal(r.status(), status, route); return { body: r.request().postDataJSON(), value: (await r.json()).data };
}
async function layout(page, name) {
  for (const width of [320, 390, 768]) {
    await page.setViewportSize({ width, height: 900 }); await stable(page);
    const value = await screen(page).evaluate(node => ({ width: node.getBoundingClientRect().width, overflow: document.documentElement.scrollWidth > innerWidth + 1, small: [...node.querySelectorAll('.cp-button,.cp-row')].filter(n => { const r = n.getBoundingClientRect(); return r.width && r.height < 43.5; }).map(n => n.textContent) }));
    assert.equal(value.overflow, false, name + ' horizontal overflow'); assert.ok(Math.abs(value.width - width) <= 1); assert.deepEqual(value.small, []); layouts.push({ page: name, width, passed: true });
  }
  await page.setViewportSize({ width: 390, height: 900 }); await stable(page); await assertSafe(page); await page.screenshot({ path: path.join(evidence, name + '.png'), fullPage: true });
}


function done(message) { checks.push(message); console.log('Completed actual flow ' + checks.length + ': ' + message); }
async function assertSafe(page) { assert.equal(await page.locator('input[type="password"]:visible,input[maxlength="6"]:visible,.cp-secret:visible').count(), 0); }
async function image(page, name, caption) {
  await stable(page); await assertSafe(page); const file = path.join(app, '../docs/media/screenshots/' + name + '.png');
  await page.screenshot({ path: file, fullPage: false });
  screenshots.push({ file: 'docs/media/screenshots/' + name + '.png', caption, sha256: createHash('sha256').update(await readFile(file)).digest('hex'), synthetic_data: true, real_device: false });
}


const acknowledgement = '我已核对准确对象、位置及本次操作影响';
async function check(page) { await screen(page).locator('label').filter({ hasText: acknowledgement }).click(); }
let navigationToken;
async function open(page, scope, token = navigationToken) {
  // Use actual public navigation controls. Direct hash returns can reuse an
  // older Taro stack entry, whose immutable route params belong to that entry.
  await navigate(page, '/pages/home/index'); await heading(page, '工作从这里开始');
  await screen(page).locator('.cp-row-title').getByText('文件与资料', { exact:true }).click();
  await heading(page,'文件与资料'); await field(page,'查找当前范围的名称').waitFor();
  const file = scope.file_id ? await request(token,'/files/'+scope.file_id) : undefined;
  const ancestors=[]; let id=file?.folder_id ?? scope.folder_id;
  while(id) { assert.ok(ancestors.length < 32); const folder=await request(token,'/folders/'+id); ancestors.unshift(folder); id=folder.parent_id; }
  for(const folder of ancestors) {
    await field(page,'查找当前范围的名称').fill(folder.name); await button(page,'查找目录与文件').click();
    await screen(page).locator('.cp-row-title').getByText(folder.name,{exact:true}).click();
    await heading(page,folder.name); await field(page,'查找当前范围的名称').waitFor(); await stable(page);
  }
  if(file) { await screen(page).locator('.cp-row-title').getByText(file.name,{exact:true}).click(); await heading(page,file.name); await button(page,'查阅准确内容版本 1').waitFor(); }
  await stable(page);
}
async function prepare(page, title) { await button(page, '核对本次' + title).click(); await check(page); assert.equal(await button(page, '确认' + title).isEnabled(), true); }
async function choose(page, folders) {
  await button(page, '选择目标目录').click();
  for (const folder of folders.slice(0, -1)) {
    await screen(page).locator('.cp-row').filter({ hasText: folder.name }).click(); await stable(page);
  }
  await button(page, '选用目录：' + folders.at(-1).name).click(); await field(page, '新名称').waitFor();
}
async function appearance(page, name) {
  for (const size of [{ width: 375, height: 812 }, { width: 900, height: 390 }, { width: 1024, height: 900 }]) {
    await page.setViewportSize(size); await stable(page);
    const v = await screen(page).evaluate(node => ({ overflow: document.documentElement.scrollWidth > innerWidth + 1, small: [...node.querySelectorAll('.cp-button,.cp-row')].filter(n => { const r=n.getBoundingClientRect(); return r.width && r.height < 43.5; }).map(n => n.textContent) }));
    assert.equal(v.overflow, false, name + ' overflow'); assert.deepEqual(v.small, []); layouts.push({ name, ...size, passed: true });
  }
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' }); await page.setViewportSize({ width: 375, height: 812 }); await stable(page);
  // Browser text magnification is H5 evidence, not a native dynamic-type claim.
  await page.addStyleTag({ content: 'html {font-size:200% !important}' }); await stable(page);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);
  layouts.push({ name, width:375, height:812, browser_text_percent:200, color_scheme:'dark', reduced_motion:true, real_device:false, passed:true });
  await page.evaluate(() => { document.querySelectorAll('style').forEach(n => { if(n.textContent === 'html {font-size:200% !important}') n.remove(); }); });
  await page.emulateMedia({ colorScheme: 'light', reducedMotion:'reduce' }); await page.setViewportSize({ width:390, height:900 }); await stable(page);
}

try {
  assert.equal(fixture.content_fixture, true);
  browser = await chromium.launch({ executablePath, headless:true });
  const alice = await login(fixture.alice), bob = await login(fixture.bob); mainPage = alice.page; navigationToken = alice.token;
  const page = alice.page, original = fixture.content_files.historical;
  let file = await request(alice.token, '/files/' + original.file_id);
  const root = await request(alice.token, '/folders/' + file.folder_id);
  assert.equal(root.can_create, true); assert.equal(file.can_move, true); assert.equal(file.can_trash, true);
  let target = await request(alice.token, '/folders', 'POST', { name:'交付归档 · Delivery', classification:root.classification, parent_id:root.id });
  let folder;
  await open(page, { folder_id:root.id });
  recordings.push(await recordViewport(page, app, 'mobile-files-directory', async () => {
    await button(page, '新建目录').click(); await field(page, '目录名称').fill('项目资料 · Working');
    await prepare(page, '新建目录'); await page.waitForTimeout(1800); await image(page, 'mobile-files-create', 'Create a folder under its actual parent with inherited scope and explicit confirmation.');
    const created = await submit(page, '确认新建目录', '/folders', 'POST', 201); folder=created.value;
    assert.equal(created.body.parent_id, root.id); assert.equal(created.body.classification, root.classification); assert.equal(folder.version, 1); await page.waitForTimeout(1000);
    await button(page, '新建目录').waitFor(); await screen(page).locator('.cp-row-title').getByText(folder.name,{exact:true}).click(); await heading(page,folder.name); await button(page,'改名或移动当前目录').waitFor();
    await button(page, '改名或移动当前目录').click(); await field(page, '新名称').fill('项目资料 · Reviewed'); await field(page, '操作原因').fill('人工整理隔离合成项目的交付资料。');
    await choose(page, [root, target]); await prepare(page, '整理目录'); await page.waitForTimeout(1800);
    const moved=await submit(page, '确认整理目录', '/folders/' + folder.id + '/move', 'POST');
    assert.equal(moved.body.expected_version, folder.version); assert.equal(moved.body.target_folder_id, target.id); folder=moved.value;
    await button(page, '新建目录').waitFor(); assert.equal((await request(alice.token, '/folders/'+folder.id)).parent_id, target.id);
    await image(page, 'mobile-files-directory', 'Authoritative folder move receipt, current revision and inherited project scope.'); await page.waitForTimeout(1500);
  }));
  done('Actual UI folder creation and rename/move commit exact parent, source revision, name, reason and same project/classification; complete receipts are reread.');
  await appearance(page, 'folder-directory');

  await open(page, { file_id:file.id });
  recordings.push(await recordViewport(page, app, 'mobile-files-lifecycle', async () => {
    await button(page, '改名或移动文件').click(); await field(page, '新名称').fill('项目交付记录 · Final.txt'); await field(page, '操作原因').fill('按已核对的目录归集资料，保留全部正式内容版本。');
    await choose(page, [root,target,folder]); await prepare(page, '整理文件');
    await page.waitForTimeout(1800); await image(page, 'mobile-files-move', 'Review an exact file revision and selected destination before committing.');
    const moved=await submit(page, '确认整理文件', '/files/'+file.id+'/move', 'POST');
    assert.equal(moved.body.expected_version,file.version); assert.equal(moved.body.target_folder_id,folder.id); assert.equal(moved.value.current_version_id,file.current_version_id); file=moved.value;
    await button(page, '移入回收站').waitFor(); await button(page,'移入回收站').click(); await field(page,'操作原因').fill('验证可恢复的回收站流程，未删除云端原件。'); await prepare(page,'移入回收站'); await page.waitForTimeout(1500);
    const trashed=await submit(page,'确认移入回收站','/files/'+file.id+'/trash','POST'); assert.equal(trashed.body.action,'trash'); assert.equal(trashed.value.state,'trashed'); file=trashed.value;
    await button(page,'恢复至原目录').waitFor(); assert.equal(await button(page,'查阅准确内容版本 1').isEnabled(),false); await page.waitForTimeout(1500);
    await button(page,'恢复至原目录').click(); await field(page,'操作原因').fill('确认名称未占用并恢复原目录和准确正式版本。'); await prepare(page,'恢复至原目录'); await page.waitForTimeout(1500);
    const restored=await submit(page,'确认恢复至原目录','/files/'+file.id+'/restore','POST'); assert.equal(restored.body.mode,'untrash'); assert.equal(restored.value.current_version_id,original.current_version_id); file=restored.value;
    await button(page,'移入回收站').waitFor(); await image(page,'mobile-files-restored','Restored active file with both formal versions, digest and replica state.'); await page.waitForTimeout(1500);
  }));
  done('Actual file rename/move, trash and untrash return successive revisions, retain exact formal content and block content reading while trashed.');
  assert.equal((await request(alice.token,'/files/'+file.id)).folder_id,folder.id);
  await appearance(page,'file-history');
  await button(page,'查阅准确内容版本 1').click(); await button(page,'读取这个版本的内容').waitFor();
  const [grant]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname === '/v1/file-versions/'+original.version_id+'/download' && r.request().method()==='POST'),button(page,'读取这个版本的内容').click()]);
  assert.equal((await grant.json()).data.file_version_id,original.version_id);
  await screen(page).locator('.cp-content-text').filter({hasText:'香港项目 · 固定版本一'}).waitFor();
  assert.equal(await page.evaluate(()=>window.__cpFileScriptExecuted===true),false); await screen(page).getByText(original.sha256,{exact:true}).waitFor();
  done('The two actual formal versions remain visible; the historical read requests version1 and validates its real SHA256/plain text without executing embedded markup or replacing it with the current version.');

  await open(page,{folder_id:root.id}); await button(page,'新建目录').click(); await field(page,'目录名称').fill(target.name); await prepare(page,'新建目录');
  await submit(page,'确认新建目录','/folders','POST',409); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor();
  assert.equal(await field(page,'目录名称').inputValue(),target.name); assert.equal(await button(page,'确认新建目录').isEnabled(),false);
  done('An actual duplicate-name409 preserves the draft and blocks a duplicate submission; no success is inferred.');

  await open(page,{folder_id:folder.id}); await button(page,'改名或移动当前目录').click(); await field(page,'新名称').fill('我的未提交整理'); await field(page,'操作原因').fill('并发修订核对'); await prepare(page,'整理目录');
  const competing=await request(alice.token,'/folders/'+folder.id+'/move','POST',{expected_version:folder.version,new_name:'同事已更新的目录',reason:'真实隔离API并发修改'}); folder=competing;
  await submit(page,'确认整理目录','/folders/'+folder.id+'/move','POST',409); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor();
  assert.equal(await field(page,'新名称').inputValue(),'我的未提交整理'); assert.equal((await request(alice.token,'/folders/'+folder.id)).name,competing.name);
  done('A real competing folder revision causes409; the local text survives and does not overwrite the authoritative name.');

  await open(page,{file_id:file.id}); await button(page,'改名或移动文件').click(); await field(page,'新名称').fill('已提交但响应丢失.txt'); await field(page,'操作原因').fill('显式标记的浏览器传输故障验证'); await prepare(page,'整理文件');
  let lost; const moveRoute='**/v1/files/'+file.id+'/move';
  await page.route(moveRoute,async route=> { const r=await route.fetch(); lost={status:r.status(),value:(await r.json()).data}; await route.abort('failed'); });
  await button(page,'确认整理文件').click(); await screen(page).getByText(/输入已保留。请先刷新资料/).waitFor(); await page.unroute(moveRoute);
  assert.equal(lost.status,200); assert.equal(lost.value.version,file.version+1); assert.equal(await button(page,'确认整理文件').isEnabled(),false);
  file=await request(alice.token,'/files/'+file.id); assert.equal(file.name,'已提交但响应丢失.txt'); await button(page,'刷新当前资料').click(); await button(page,'改名或移动文件').waitFor();
  done('A labelled browser transport fault loses an actually committed file response; repeat is blocked and a fresh authoritative read reconciles exactly one revision.');

  await open(page,{folder_id:target.id}); await button(page,'新建目录').click(); await field(page,'目录名称').fill('回执缺字段目录'); await prepare(page,'新建目录');
  let partial; await page.route('**/v1/folders',async route=> { if(route.request().method()!=='POST') return route.continue(); const r=await route.fetch(); const body=await r.json(); partial=body.data; delete body.data.creator_id; await route.fulfill({response:r,json:body}); });
  await button(page,'确认新建目录').click(); await screen(page).getByText(/操作返回的资料不完整/).waitFor(); await page.unroute('**/v1/folders');
  assert.equal(await button(page,'确认新建目录').isEnabled(),false); assert.equal((await request(alice.token,'/folders/'+partial.id)).name,'回执缺字段目录');
  done('A separately labelled incomplete receipt after a real create cannot announce success or clear the text draft; the record is verified through a fresh API read.');

  for(let i=0;i<23;i++) await request(alice.token,'/folders','POST',{name:'分页资料 '+String(i).padStart(2,'0'),classification:target.classification,parent_id:target.id});
  await open(page,{folder_id:target.id}); await field(page,'查找当前范围的名称').fill('分页资料'); await button(page,'查找目录与文件').click(); await button(page,'加载更多子目录').waitFor();
  assert.equal(await screen(page).locator('.cp-row').filter({hasText:'分页资料'}).count(),20); await button(page,'加载更多子目录').click(); await screen(page).locator('.cp-row').filter({hasText:'分页资料'}).nth(22).waitFor();
  assert.equal(await screen(page).locator('.cp-row').filter({hasText:'分页资料'}).count(),23);
  done('An actual name filter and opaque cursor paginate23 same-parent folders as20+3, without mixing other directories or inventing history versions.');

  await open(bob.page,{file_id:file.id},bob.token); await screen(bob.page).getByText(file.id,{exact:true}).waitFor();
  const project=await request(alice.token,'/projects/'+fixture.project_id), bobMe=await request(bob.token,'/me');
  await request(alice.token,'/projects/'+fixture.project_id+'/members','POST',{expected_version:project.version,user_id:bobMe.user.id,action:'remove',reason:'验证实际权限撤回和旧会话失效'});
  assert.equal((await response(bob.token,'/files/'+file.id)).status,401);
  await button(bob.page,'刷新当前资料').click(); await field(bob.page,'内部账号').waitFor(); assert.equal(await screen(bob.page).getByText(file.id,{exact:true}).count(),0);
  done('Actual project membership withdrawal invalidates the second identity login; refresh clears the visible file and returns to authentication.');
  assert.deepEqual(errors,[]); assert.deepEqual(serverErrors,[]); passed=true;
} catch (error) {
  failure = String(error.stack ?? error);
  if (mainPage && !mainPage.isClosed()) {
    try { failure += '\nActive page: ' + JSON.stringify({ route: new URL(mainPage.url()).hash, titles: await screen(mainPage).locator('.cp-title').allTextContents(), actions: await screen(mainPage).getByRole('button').allTextContents() });
      await mainPage.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true, mask: [mainPage.locator('input[type="password"],input[maxlength="6"],.cp-secret,.cp-code')] }); } catch { /* Preserve original failure. */ }
  }
  for (const account of [fixture.alice, fixture.bob, fixture.administrator]) for (const key of ['password', 'mfa_secret']) if (account?.[key]) failure = failure.replaceAll(account[key], '[redacted]');
  process.stderr.write(failure + '\n');
} finally {
  for (const context of contexts) { try { await closeOwned(context); } catch { cleanupErrors.push('owned context cleanup failed'); } }
  if (browser) { try { await browser.close(); } catch { cleanupErrors.push('owned browser cleanup failed'); } }
  const after = await digest(), closed = !browser || !browser.isConnected();
  const result = { passed: passed && before === after && closed && !cleanupErrors.length && closedContexts.size === contexts.length, checked_at: new Date().toISOString(), source_sha256: before, source_after_sha256: after, source_unchanged: before === after, source_count: manifest.files.length, checks, layouts, recordings, screenshots, errors, server_errors: serverErrors, cleanup_errors: cleanupErrors, failure, owned_browser_closed: closed, browser_contexts_created: contexts.length, browser_contexts_closed: closedContexts.size, real_native_api: true, real_model: false, real_cloud: false, real_device: false, real_telegram_network: false, controlled_response_faults: ['committed file move response lost at browser transport', 'incomplete committed folder receipt'] };
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify(result, null, 2) + '\n'); console.log(JSON.stringify({ passed: result.passed, checks: checks.length, closed })); if (!result.passed) process.exitCode = 1;
}
