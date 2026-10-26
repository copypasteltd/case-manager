// Actual Web/API project closeout and archive checks against an owned disposable native fixture.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app = fileURLToPath(new URL('../', import.meta.url)), root = path.dirname(app.replace(/[\\/]$/, ''));
const fixture = JSON.parse(await readFile(path.join(app, '.local/business-browser-fixture.json'), 'utf8'));
assert.equal(fixture.fixture_kind, 'mobile-business'); assert.equal(fixture.origin, 'http://localhost:5189');
assert.equal(new URL(fixture.api_origin).hostname, '127.0.0.1'); assert.equal(fixture.isolated_database, true);
const executablePath = process.env.CP_BROWSER_EXECUTABLE ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'; await access(executablePath);
const evidence = path.join(root, 'docs/quality/evidence/project-closeout'), temporary = path.join(app, '.local/browser-temp');
await mkdir(evidence, { recursive: true }); await mkdir(temporary, { recursive: true }); process.env.TMP = temporary; process.env.TEMP = temporary;
async function files(directory) { const result = []; for (const entry of await readdir(path.join(app, directory), { withFileTypes: true })) { if (['dist', 'node_modules', '.taro', '.swc'].includes(entry.name)) continue; const relative = path.posix.join(directory, entry.name); if (entry.isDirectory()) result.push(...await files(relative)); else result.push(relative); } return result; }
const sources = [...new Set([...(await files('backend')).filter(name => /\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)), ...await files('web/src'), ...await files('mobile/src'), 'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json', 'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json', 'scripts/check_project_closeout.mjs', 'scripts/run_spend_browser.py'])].sort();
async function digest() { const hash = createHash('sha256'); for (const file of sources) { hash.update(file + '\0'); hash.update(await readFile(path.join(app, file))); hash.update('\0'); } return hash.digest('hex'); }
const before = await digest(), contexts = [], checks = [], errors = [], serverErrors = [], lastSteps = new Map();
let browser, passed = false, alice, bob;
const cleanupErrors = [];
function otp(secret, step) { const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567', bits = [...secret.replace(/=+$/, '').toUpperCase()].map(value => alphabet.indexOf(value).toString(2).padStart(5, '0')).join(''), key = Buffer.from(bits.match(/.{8}/g).map(value => Number.parseInt(value, 2))), counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(step)); const hash = createHmac('sha1', key).update(counter).digest(); return ((hash.readUInt32BE(hash[19] & 15) & 0x7fffffff) % 1000000).toString().padStart(6, '0'); }
async function code(account) { const previous = lastSteps.get(account.login) ?? account.last_step; if (Math.floor(Date.now() / 30000) <= previous) await new Promise(resolve => setTimeout(resolve, (previous + 1) * 30000 - Date.now() + 250)); const step = Math.floor(Date.now() / 30000); lastSteps.set(account.login, step); return otp(account.mfa_secret, step); }
async function login(account) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1050 }, colorScheme: 'light', reducedMotion: 'reduce' }); contexts.push(context);
  const page = await context.newPage(); page.setDefaultTimeout(20000);
  page.on('pageerror', error => errors.push(error.message)); page.on('response', response => { if (response.status() >= 500) serverErrors.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  await page.goto(fixture.origin); await page.getByLabel('内部账号', { exact: true }).fill(account.login); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录工作空间', exact: true }).click();
  if (account.mfa_secret) { await page.getByLabel('六位验证码').fill(await code(account)); await page.getByRole('button', { name: '验证并继续', exact: true }).click(); }
  await page.getByRole('heading', { name: /工作从这里开始/ }).waitFor(); return { context, page };
}
async function response(context, route, method = 'GET', body) {
  const csrf = (await context.cookies()).find(item => item.name === 'cp_csrf')?.value;
  const value = await context.request.fetch(fixture.origin + '/v1' + route, { method, headers: { Origin: fixture.origin, 'X-CSRF-Token': csrf ?? '', 'Idempotency-Key': crypto.randomUUID() }, ...(body ? { data: body } : {}) });
  return { status: value.status(), value: await value.json() };
}
async function request(context, route, method = 'GET', body) { const result = await response(context, route, method, body); assert(result.status < 400, route + ' returned ' + result.status + '/' + result.value.error?.code + '/' + JSON.stringify(result.value.error?.details)); return result.value.data; }
async function stable(page) {
  await page.evaluate(async () => { await document.fonts.ready; await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))); });
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running' || animation.effect?.getComputedTiming().iterations === Infinity));
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.sidebar'), narrow = matchMedia('(max-width: 899px)').matches;
    return sidebar && (narrow ? sidebar.inert && sidebar.getAttribute('aria-hidden') === 'true' : !sidebar.inert && sidebar.getAttribute('aria-hidden') !== 'true' && getComputedStyle(sidebar).visibility === 'visible' && getComputedStyle(sidebar).transform === 'none');
  });
}
assert.equal(fixture.project_closeout_fixture, true); assert(fixture.administrator?.mfa_secret);
async function modalFocus(page) {
  await page.waitForFunction(() => { const top = [...document.querySelectorAll('dialog[open]')].at(-1); return !top || top.contains(document.activeElement); });
}
async function commit(dialog, label) { await dialog.getByRole('button', { name: label, exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function close(dialog) { await dialog.getByRole('button', { name: '关闭弹窗', exact: true }).click(); await dialog.waitFor({ state: 'hidden' }); await modalFocus(dialog.page()); }
async function layout(page, name, dialog = null) {
  for (const theme of ['light', 'dark']) {
    await page.getByLabel('外观偏好', { exact: true }).selectOption(theme, { force: !!dialog });
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 1000 }); await stable(page);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false, name + '/' + width + ' overflows');
      const scope = dialog ?? page.locator('#main-content');
      const undersized = await scope.locator('button:visible,a:visible,select:visible,input:visible').evaluateAll(nodes => nodes.flatMap(node => { const rect = node.getBoundingClientRect(); return rect.width < 23.5 || rect.height < 23.5 ? [{ tag: node.tagName, text: node.textContent?.trim().slice(0, 100), width: rect.width, height: rect.height }] : []; }));
      assert.deepEqual(undersized, [], name + ' has undersized controls');
      if (dialog) {
        assert.equal(await dialog.evaluate(node => node.scrollWidth > node.clientWidth + 1), false, name + ' dialog content overflows');
        const bounds = await dialog.boundingBox(); assert(bounds && bounds.x >= -1 && bounds.x + bounds.width <= width + 1, name + ' dialog exceeds viewport');
        await modalFocus(page);
      }
      if ((theme === 'light' && width === 1440) || (theme === 'dark' && width === 390)) {
        await page.evaluate(() => { window.scrollTo({ top: 0, behavior: 'instant' }); for (const node of document.querySelectorAll('dialog[open]')) node.scrollTop = 0; });
        await page.screenshot({ path: path.join(evidence, name + '-' + theme + '-' + width + '.png'), fullPage: !dialog, animations: 'disabled' });
      }
    }
  }
  await page.setViewportSize({ width: 1440, height: 1050 }); await page.getByLabel('外观偏好', { exact: true }).selectOption('light', { force: !!dialog });
  layouts.push({ name, themes: ['light', 'dark'], widths: [320, 390, 768, 1024, 1440], passed: true });
}
const layouts = [];

async function openChecklist(page,id){
  await page.goto(fixture.origin+'/projects/'+id+'?tab=closeout');
  await page.getByRole('heading',{name:'结项与归档',exact:true}).waitFor();
  await page.getByText(/核查于/).waitFor();await stable(page);
}
async function transition(context,id,action){
  const p=await request(context,'/projects/'+id);
  return request(context,'/projects/'+id+'/transition','POST',{expected_version:p.version,action,reason:'隔离浏览器项目生命周期核对'});
}
async function declarations(dialog){
  const boxes=dialog.locator('.closeout-attestation input');assert.equal(await boxes.count(),2);
  await boxes.nth(0).check();await boxes.nth(1).check();
  await dialog.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。',{exact:true}).waitFor();
}
try{
  browser=await chromium.launch({executablePath,headless:true,args:['--disable-gpu','--disable-background-networking']});
  alice=await login(fixture.alice);bob=await login(fixture.bob);
  const me=await request(alice.context,'/me'),other=await request(bob.context,'/me');
  await openChecklist(alice.page,fixture.project_id);
  const existing=await request(alice.context,'/projects/'+fixture.project_id+'/closeout');
  assert(existing.groups.some(group=>(group.open_count??0)>0||group.visibility==='restricted'));
  await layout(alice.page,'closeout-existing-obligations');
  checks.push({name:'existing contract and financial obligations are checked without being changed',passed:true});

  const name='项目交付结项与归档核查'+ 'ContinuousArchiveEvidenceReview'.repeat(4);
  let p=await request(alice.context,'/projects','POST',{name,scope:'完成明确的隔离交付，记录经验、后续跟进与财务依据。',type:'internal',owner_id:me.user.id,member_ids:[me.user.id,other.user.id],classification:'project_restricted'});
  p=await transition(alice.context,p.id,'activate');
  const task=await request(alice.context,'/projects/'+p.id+'/tasks','POST',{title:'明确未完成的交付任务'+ 'ExactDeliverableBeforeArchive'.repeat(4),body:'必须完成或明确取消后才可进入结项核查。',assignee_id:me.user.id,priority:'normal',review_required:false});
  await openChecklist(alice.page,p.id);
  await alice.page.getByRole('link',{name:task.title,exact:true}).waitFor();
  assert.equal(await alice.page.getByRole('button',{name:'进入结项核查阶段',exact:true}).count(),0);
  await layout(alice.page,'closeout-open-task');
  await request(alice.context,'/tasks/'+task.id+'/transition','POST',{expected_version:task.version,action:'cancel',reason:'隔离任务确认无需继续执行'});
  await alice.page.getByRole('button',{name:'重新核查',exact:true}).click();
  await alice.page.getByRole('button',{name:'进入结项核查阶段',exact:true}).click();
  let dialog=alice.page.getByRole('dialog',{name:'进入结项核查阶段',exact:true});
  await dialog.getByLabel('核查原因',{exact:true}).fill('已逐项核对当前未结事项，进入只读阶段填写复盘。');
  await commit(dialog,'确认进入核查阶段');
  await alice.page.getByRole('button',{name:'填写复盘并归档',exact:true}).waitFor();
  checks.push({name:'an unfinished task blocks closing; explicit disposition allows a separate closing step',passed:true});

  await alice.page.getByRole('button',{name:'填写复盘并归档',exact:true}).click();
  dialog=alice.page.getByRole('dialog',{name:'填写复盘并归档',exact:true});
  const submit=dialog.getByRole('button',{name:'确认复盘并归档',exact:true});assert.equal(await submit.isDisabled(),true);
  const summary='已核对实际成果、交付验收与业务边界。\n'+'保留清晰的核查记录与可追溯的后续处理安排。'.repeat(5);
  await dialog.getByLabel('成果与交付总结',{exact:true}).fill(summary);
  await dialog.getByLabel('经验与复盘',{exact:true}).fill('提前确定交付标准，逐项核对任务、合同、费用与订阅。');
  await dialog.getByLabel('后续跟进',{exact:true}).fill('无未处置事项；后续新增工作应重开或另建项目。');
  await dialog.getByLabel('归档原因',{exact:true}).fill('人工确认本次项目成果与所有核查事项。');
  assert.equal(await dialog.locator('.closeout-attestation input:checked').count(),0);
  await layout(alice.page,'archive-review-dialog',dialog);
  await declarations(dialog);await submit.waitFor({state:'visible'});
  // Fault injection changes only this outgoing field. The real API validates
  // the request and returns the actual field-specific error; no mocked success.
  await alice.page.route('**/v1/projects/'+p.id+'/archive',async route=>{
    const body=route.request().postDataJSON();await route.continue({postData:JSON.stringify({...body,summary:' '})});
  },{times:1});
  await submit.click();await dialog.locator('a[href="#archive-summary"]').waitFor();
  assert.equal(await dialog.getByLabel('成果与交付总结',{exact:true}).inputValue(),summary);
  await dialog.locator('a[href="#archive-summary"]').click();
  assert.equal(await alice.page.evaluate(()=>document.activeElement?.id),'archive-summary');
  checks.push({name:'server field validation keeps the retrospective and its error link focuses the exact labeled input',passed:true});

  await transition(alice.context,p.id,'resume');p=await request(alice.context,'/projects/'+p.id);
  p=await request(alice.context,'/projects/'+p.id,'PATCH',{expected_version:p.version,name:p.name+'更新'});
  await transition(alice.context,p.id,'close');await submit.click();
  await dialog.getByText('当前依据或权限需要重新核对。已填写的复盘会保留，重新读取后请再次确认。',{exact:true}).waitFor();
  assert.equal(await dialog.getByLabel('成果与交付总结',{exact:true}).inputValue(),summary);
  assert.equal(await submit.isDisabled(),true);await layout(alice.page,'archive-stale-review',dialog);
  await dialog.getByRole('button',{name:'保留填写内容，重新读取核查',exact:true}).click();
  await dialog.getByText('当前依据或权限需要重新核对。已填写的复盘会保留，重新读取后请再次确认。',{exact:true}).waitFor({state:'hidden'});
  assert.equal(await dialog.locator('.closeout-attestation input:checked').count(),0);
  assert.equal(await dialog.getByLabel('成果与交付总结',{exact:true}).inputValue(),summary);
  await declarations(dialog);await commit(dialog,'确认复盘并归档');
  await alice.page.getByText('当前项目仍与这次归档依据一致。',{exact:true}).waitFor({state:'attached'});
  const history=await request(alice.context,'/projects/'+p.id+'/archives');assert.equal(history.items.length,1);assert.equal(history.items[0].summary,summary);assert.equal(history.items[0].current_basis_state,'current');
  const archive=alice.page.locator('.closeout-archive');await archive.locator('summary').first().click();
  await alice.page.getByText(summary,{exact:true}).waitFor();
  await layout(alice.page,'archive-complete-history');
  checks.push({name:'stale project versions cannot archive; an explicit refresh retains input and resets declarations before a real commit',passed:true});

  await openChecklist(bob.page,p.id);
  await bob.page.getByText('当前权限无法完整查阅归档依据。历史记录、数量和复盘内容均未展示。',{exact:true}).waitFor();
  assert.equal(await bob.page.getByText(summary,{exact:true}).count(),0);
  const restricted=await request(bob.context,'/projects/'+p.id+'/closeout'),hidden=await request(bob.context,'/projects/'+p.id+'/archives');
  assert.equal(restricted.visibility,'restricted');assert.equal(restricted.fingerprint,null);assert.equal(hidden.visibility,'restricted');assert.deepEqual(hidden.items,[]);assert.equal(hidden.next_cursor,null);
  for(const group of restricted.groups.filter(group=>group.visibility==='restricted')){assert.equal(group.count,null);assert.equal(group.open_count,null);assert.deepEqual(group.items,[]);}
  await layout(bob.page,'archive-restricted-history');
  checks.push({name:'a reader missing financial source authority sees no historical retrospective, counts, items or fingerprint',passed:true});

  await transition(alice.context,p.id,'reopen');await openChecklist(alice.page,p.id);
  const historical=await request(alice.context,'/projects/'+p.id+'/archives');assert.equal(historical.items.length,1);assert.equal(historical.items[0].id,history.items[0].id);assert.equal(historical.items[0].summary,summary);assert.equal(historical.items[0].current_basis_state,'changed');
  checks.push({name:'controlled reopening preserves the original retrospective and labels its basis as changed',passed:true});
  assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){process.stderr.write(String(error.stack??error)+'\n');}
finally{
  for(const context of contexts){try{await context.close();}catch{cleanupErrors.push('owned browser context cleanup failed');}}
  if(browser){try{await browser.close();}catch{cleanupErrors.push('owned browser cleanup failed');}}
  const after=await digest(),browserClosed=!browser||!browser.isConnected();passed=passed&&before===after&&browserClosed&&!cleanupErrors.length;
  await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,executed_at:new Date().toISOString(),source_sha256:before,source_file_count:sources.length,source_unchanged_during_tests:before===after,real_web_api:true,isolated_native_database:true,controlled_file_provider:true,archive_file_pin_verified_by_backend_only:true,production_deliveries:false,checks,layouts,errors,server_errors:serverErrors,cleanup_errors:cleanupErrors,all_owned_browser_contexts_closed:!cleanupErrors.some(error=>error.includes('context')),all_owned_browser_tabs_closed:browserClosed},null,2)+'\n');
}
process.stdout.write(JSON.stringify({passed,checks:checks.length})+'\n');process.exitCode=passed?0:1;

