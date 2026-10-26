// Real Taro H5 plan references against the owned native integration fixture.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));assert.equal(fixture.agent_file_fixture,true);assert.equal(fixture.fixture_kind,'mobile-business');assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-files-mobile';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TEMP=temporary;process.env.TMP=temporary;
async function sourceFiles(directory){const out=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['dist','node_modules','.taro','.swc'].includes(entry.name))continue;const relative=path.posix.join(directory,entry.name);if(entry.isDirectory())out.push(...await sourceFiles(relative));else out.push(relative);}return out;}
const apiSources=(await sourceFiles('backend')).filter(file=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources=[...new Set([...await sourceFiles('mobile'),...apiSources,'package.json','package-lock.json','scripts/check_mobile_file_text.mjs','scripts/run_mobile_browser.py','scripts/test_backend.py','scripts/sync_design.py','scripts/build_openapi.py','packages/contracts/src/api.d.ts','packages/design/src/tokens.json','packages/contracts/src/mutation.ts'])].sort();
async function digest(){const hash=createHash('sha256');for(const file of sources){hash.update(file+'\0');hash.update(await readFile(path.join(app,file)));hash.update('\0');}return hash.digest('hex');}
const before=await digest(),started=new Date().toISOString(),checks=[],errors=[],serverErrors=[],contexts=[];let browser,passed=false,activePage,closed=true;
const screen=p=>p.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(p,name)=>screen(p).getByRole('button',{name,exact:true});
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',bits=[...secret.replace(/=+$/,'')].map(c=>alphabet.indexOf(c).toString(2).padStart(5,'0')).join(''),key=Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const h=createHmac('sha1',key).update(counter).digest();return ((h.readUInt32BE(h[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
async function stable(page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});}
async function select(page, label, text) {
  const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]'); await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  // The panel animates in from outside the viewport. Wait for its actual
  // hydration and animation before measuring and using the real mouse wheel.
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(animation => animation.finished.catch(() => {}))); });
  await stable(page); const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text); assert.ok(desired >= 0, label + ' missing ' + text);
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0, box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); for (let step = 1; step <= 8; step++) { await page.mouse.move(x, y - (desired - current) * item.height * step / 8); await stable(page); } await page.mouse.up(); await stable(page); }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stable(page); assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired), label + ' did not select ' + text);
}
try{
  const executablePath=process.env.CP_BROWSER_EXECUTABLE??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);browser=await chromium.launch({executablePath,headless:true});
  const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'});contexts.push(context);const page=await context.newPage();activePage=page;page.setDefaultTimeout(25000);page.on('pageerror',e=>errors.push(e.message));page.on('response',r=>{if(r.status()>=500)serverErrors.push({status:r.status(),path:new URL(r.url()).pathname});});
  await page.goto(fixture.origin);await page.locator('input[placeholder="你的公司账号"]').fill(fixture.alice.login);await page.locator('input[type="password"]').fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间 ↗',exact:true}).click();const code=page.locator('input[maxlength="6"]');await code.waitFor();
  if(Math.floor(Date.now()/30000)<=fixture.alice.last_step)await new Promise(r=>setTimeout(r,(fixture.alice.last_step+1)*30000-Date.now()+250));
  await code.fill(otp(fixture.alice.mfa_secret,Math.floor(Date.now()/30000)));await page.getByRole('button',{name:'验证并继续 ↗',exact:true}).click();


  await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-title').filter({hasText:'公司秘书'}).waitFor();
  await screen(page).locator('.cp-row-title').getByText('准确文件正文会话',{exact:true}).click();await screen(page).locator('.cp-title').filter({hasText:'准确文件正文会话'}).waitFor();
  let sentCount=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sentCount++;});
  async function chooseVersion(seq){
    await button(page,'核对并选择上下文资料').click();await select(page,'上下文资料类型','准确文件版本正文');
    await screen(page).locator('.cp-row-title').getByText('秘书有界正文.md',{exact:true}).click();
    const row=screen(page).locator('.cp-row-title').filter({hasText:new RegExp('^版本 '+seq+' ·')});await row.waitFor();
    assert.equal(await button(page,'核对并预览资料').isDisabled(),true);await row.click();
  }
  await chooseVersion(1);assert.equal(sentCount,0);
  checks.push({name:'Mobile requires explicit file and formal version selection without automatic sending',passed:true});
  const [first]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),button(page,'核对并预览资料').click()]);assert.equal(first.status(),200);const view=(await first.json()).data,token=first.request().headers().authorization;
  assert.equal(view.version_id,fixture.historical_version_id);assert.equal(view.evidence[0].sha256,fixture.content_sha256);assert.match(view.text,new RegExp(fixture.historical_prefix));assert.equal(view.text.includes(fixture.current_prefix),false);await screen(page).getByText(new RegExp(fixture.historical_prefix)).waitFor();
  checks.push({name:'Mobile displays actual verified historical UTF-8 content and evidence instead of the newer head',passed:true});
  await screen(page).getByText(new RegExp(fixture.historical_prefix)).scrollIntoViewIfNeeded();await page.screenshot({path:path.join(evidence,'fixed-file-text.png')});
  async function request(route){const response=await context.request.get(fixture.api_origin+'/v1'+route,{headers:{Authorization:token}});assert.equal(response.status(),200);return (await response.json()).data;}
  const [next]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),button(page,'核对下一段资料').click()]);assert.equal(next.status(),200);const continued=(await next.json()).data;assert.equal(continued.offset,view.next_offset);assert.equal(continued.version_id,view.version_id);assert.equal(continued.source_sha256,view.source_sha256);
  checks.push({name:'Mobile continuation keeps the exact file version and full source digest',passed:true});
  await button(page,'将这份准确资料加入本次请求').click();await chooseVersion(2);
  const [current]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),button(page,'核对并预览资料').click()]);assert.equal(current.status(),200);assert.match((await current.json()).data.text,new RegExp(fixture.current_prefix));
  await button(page,'将这份准确资料加入本次请求').click();assert.equal(sentCount,0);
  checks.push({name:'Mobile keeps two selected formal versions of one file as distinct references',passed:true});
  await select(page,'本次秘书任务','整理项目简报 · controlled-test');await screen(page).getByRole('textbox',{name:'本次消息内容',exact:true}).fill('Compare the two exact mobile file versions '+randomUUID());
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button(page,'确认发送本次请求').click()]);assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;
  assert.equal(input.mutation,undefined);assert.deepEqual(input.context_refs.map(ref=>ref.version_id).sort(),[fixture.historical_version_id,fixture.current_version_id].sort());assert.equal(input.context_refs.every(ref=>ref.type==='file'&&ref.id===fixture.file_id),true);
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(message=>message.run_id===run.id&&message.role==='user');assert.deepEqual(saved.source_refs.map(ref=>ref.version_id).sort(),[fixture.historical_version_id,fixture.current_version_id].sort());
  checks.push({name:'One mobile send persists both exact version citations in the actual run message',passed:true});
  await button(page,'明确取消这个任务').click();await screen(page).getByRole('textbox',{name:'取消任务的原因',exact:true}).fill('Stop isolated mobile file comparison before execution');const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),button(page,'确认请求取消').click()]);assert.equal(cancelled.status(),200);
  assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/files/'+fixture.file_id)).current_version_id,fixture.current_version_id);assert.equal(sentCount,1);
  checks.push({name:'Mobile cancellation leaves the formal file head and history unchanged',passed:true});
  assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(e){errors.push(String(e));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally{for(const c of contexts)await c.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sources,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Taro H5/native isolated API; WeChat, Douyin and Android devices, model, cloud and production remain unverified.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
