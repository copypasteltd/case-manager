// Real Taro H5 plan references against the owned native integration fixture.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));assert.equal(fixture.mutation_fixture,true);assert.equal(fixture.fixture_kind,'mobile-business');assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-mutations-mobile';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TEMP=temporary;process.env.TMP=temporary;
async function sourceFiles(directory){const out=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['dist','node_modules','.taro','.swc'].includes(entry.name))continue;const relative=path.posix.join(directory,entry.name);if(entry.isDirectory())out.push(...await sourceFiles(relative));else out.push(relative);}return out;}
const apiSources=(await sourceFiles('backend')).filter(file=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources=[...new Set([...await sourceFiles('mobile'),...apiSources,'package.json','package-lock.json','scripts/check_mobile_mutations.mjs','scripts/run_mobile_browser.py','scripts/test_backend.py','scripts/sync_design.py','scripts/build_openapi.py','packages/contracts/src/api.d.ts','packages/design/src/tokens.json','packages/contracts/src/mutation.ts'])].sort();
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

  await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-title').filter({hasText:'公司秘书'}).waitFor();await screen(page).locator('.cp-row-title').getByText('Exact project update conversation',{exact:true}).click();
  await screen(page).locator('.cp-title').filter({hasText:'Exact project update conversation'}).waitFor();
  let token;page.on('request',r=>{if(r.headers().authorization)token=r.headers().authorization;});
  const [firstRead]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+fixture.committed_run_id+'/event-log'),screen(page).locator('.cp-editor-row').filter({hasText:'Only execute the exact selected update; never widen fields or replace human review.'}).last().getByRole('button',{name:'查看本次准确更新',exact:true}).click()]);
  assert.equal(firstRead.status(),200);const historical=(await firstRead.json()).data.run;token=firstRead.request().headers().authorization;
  await screen(page).getByText('本次更新已提交',{exact:true}).waitFor();assert.match(await screen(page).innerText(),new RegExp(historical.mutation.operation_id));
  checks.push({name:'Mobile historical intent opens the actual committed update and business receipt without an unsupported route',passed:true});
  await button(page,'收起任务进度').click();
  async function request(route){const response=await context.request.get(fixture.api_origin+'/v1'+route,{headers:{Authorization:token}});assert.equal(response.status(),200);return (await response.json()).data;}
  const target=await request('/tasks/'+fixture.task_id);
  await select(page,'本次秘书任务','更新项目 · controlled-test');await button(page,'选择准确业务更新').click();await button(page,'选择本次更新任务').click();await screen(page).locator('.cp-row-title').getByText(target.title+' · 处理中',{exact:true}).click();
  await screen(page).getByRole('textbox',{name:'任务标题的新值',exact:true}).waitFor();const title='Mobile exact selected title '+randomUUID();await screen(page).getByRole('textbox',{name:'任务标题的新值',exact:true}).fill(title);await screen(page).getByRole('textbox',{name:'本次更新原因',exact:true}).fill('Explicit mobile L1 update');
  let sentCount=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sentCount++;});
  await button(page,'将这项明确更新加入请求').click();assert.match(await screen(page).innerText(),new RegExp(title));assert.equal(sentCount,0);assert.equal((await request('/tasks/'+fixture.task_id)).version,target.version);
  checks.push({name:'Mobile selects one real task and current version, previews a fixed field change, and does not send automatically',passed:true});
  await button(page,'移除本次更新').click();await button(page,'选择准确业务更新').click();await button(page,'将这项明确更新加入请求').click();assert.equal(sentCount,0);
  checks.push({name:'The selected mobile instruction can be removed and reselected before any request is submitted',passed:true});
  const text='Execute only my explicit mobile title '+randomUUID();await screen(page).getByRole('textbox',{name:'本次消息内容',exact:true}).fill(text);
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button(page,'确认发送本次请求').click()]);assert.equal(sent.status(),202);const payload=sent.request().postDataJSON(),run=(await sent.json()).data;
  assert.equal(payload.mutation.tool,'update_task');assert.deepEqual(payload.mutation.arguments,{id:target.id,expected_version:target.version,title,reason:'Explicit mobile L1 update'});assert.equal(run.mutation.status,'authorized');assert.equal(sentCount,1);
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(value=>value.run_id===run.id&&value.role==='user');assert.equal(saved.source_refs[0].type,'agent_mutation');assert.equal(saved.source_refs[0].id,run.id);assert.equal(saved.source_refs[0].sha256,run.mutation.intent_sha256);
  checks.push({name:'A real mobile send persists the exact selected L1 instruction and immutable source in its own run',passed:true});
  await screen(page).getByText('本次明确更新范围',{exact:true}).waitFor();assert.equal((await request('/tasks/'+fixture.task_id)).version,target.version);
  checks.push({name:'The mobile run shows authorized scope while queued, with no false write completion or second approval',passed:true});
  await button(page,'明确取消这个任务').click();await screen(page).getByRole('textbox',{name:'取消任务的原因',exact:true}).fill('Cancel isolated mobile update before execution');const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),button(page,'确认请求取消').click()]);assert.equal(cancelled.status(),200);
  const stopped=await request('/runs/'+run.id);assert.equal(stopped.state,'cancelled');assert.equal(stopped.mutation.status,'authorized');assert.equal(stopped.mutation.operation_id,null);assert.equal((await request('/tasks/'+fixture.task_id)).version,target.version);
  checks.push({name:'Mobile cancellation preserves the request history and leaves the unexecuted target unchanged',passed:true});
  await page.screenshot({path:path.join(evidence,'mobile-mutation-request.png'),fullPage:true});assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(e){errors.push(String(e));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally{for(const c of contexts)await c.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sources,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Taro H5/native isolated API; WeChat, Douyin and Android devices, model, cloud and production remain unverified.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
