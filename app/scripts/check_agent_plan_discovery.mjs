// Real Web selection and persisted plan context; no provider or cloud calls.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.plan_discovery_fixture,true);assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-discovery-web';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const sourceFiles=['backend/internal/agent/plan_discovery.go','backend/internal/agent/plan_tools.go','backend/internal/agent/execution_tools.go','backend/internal/agent/runs.go','backend/internal/agentworker/mcp.go','backend/internal/httpapi/agent_project_tools_integration_test.go','web/src/ConversationPlans.tsx','web/src/ConversationMessages.tsx','packages/contracts/src/api.d.ts','scripts/check_agent_plan_discovery.mjs','scripts/run_agent_browser.py'];
async function digest(){const h=createHash('sha256');for(const f of sourceFiles){h.update(f+'\n');h.update(await readFile(path.join(app,f)));}return h.digest('hex');}
const before=await digest(),started=new Date().toISOString(),checks=[],errors=[],serverErrors=[],contexts=[];let browser,passed=false,activePage,closed=true;
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',bits=[...secret.replace(/=+$/,'').toUpperCase()].map(c=>alphabet.indexOf(c).toString(2).padStart(5,'0')).join('');const key=Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const h=createHmac('sha1',key).update(counter).digest();return ((h.readUInt32BE(h[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
try{
  const executablePath=process.env.CP_BROWSER_EXECUTABLE??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
  browser=await chromium.launch({executablePath,headless:true});const context=await browser.newContext({viewport:{width:1440,height:1100},reducedMotion:'reduce'});contexts.push(context);
  const page=await context.newPage();activePage=page;page.setDefaultTimeout(25000);page.on('pageerror',e=>errors.push(e.message));page.on('response',r=>{if(r.status()>=500)serverErrors.push({status:r.status(),path:new URL(r.url()).pathname});});
  await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码').waitFor();
  if(Math.floor(Date.now()/30000)<=fixture.alice.last_step)await new Promise(r=>setTimeout(r,(fixture.alice.last_step+1)*30000-Date.now()+250));
  await page.getByLabel('六位验证码').fill(otp(fixture.alice.mfa_secret,Math.floor(Date.now()/30000)));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  async function request(route){const response=await context.request.get(fixture.origin+'/v1'+route);assert.equal(response.status(),200);return (await response.json()).data;}
  await page.goto(fixture.origin+'/conversations/'+fixture.plan_conversation_id);await page.getByRole('heading',{name:'项目创建计划',exact:true}).waitFor();
  const card=page.locator('article').filter({has:page.getByRole('heading',{name:'Synthetic tool project · 项目已创建',exact:true})});
  await card.waitFor();assert.match(await card.innerText(),new RegExp(fixture.plan_id));await card.locator('summary').filter({hasText:'核对准确计划标识'}).click();assert.match(await card.innerText(),new RegExp(fixture.plan_hash));
  checks.push({name:'Web exposes the exact existing plan ID and immutable hash in its current card',passed:true});
  const profile=page.getByLabel('本次秘书任务',{exact:true});await profile.locator('option').filter({hasText:'整理项目简报'}).waitFor({state:'attached'});
  await card.getByRole('button',{name:'引用这份计划',exact:true}).click();assert.equal(await profile.inputValue(),fixture.project_create_profile_id);assert.equal(await profile.locator('option[value="'+fixture.incompatible_profile_id+'"]').count(),0);
  const composer=page.locator('section').filter({has:page.getByRole('heading',{name:'给公司秘书的请求',exact:true})});assert.match(await composer.innerText(),/固定计划意图/);
  checks.push({name:'Explicit plan selection chooses project-create and excludes incompatible skills',passed:true});
  await composer.getByRole('button',{name:'移除此项',exact:true}).click();await profile.locator('option[value="'+fixture.incompatible_profile_id+'"]').waitFor({state:'attached'});await card.getByRole('button',{name:'引用这份计划',exact:true}).click();
  checks.push({name:'Removing a selected plan restores task choices without sending a request',passed:true});
  const text='Only inspect this exact retained plan '+randomUUID();await page.getByLabel('本次消息内容',{exact:true}).fill(text);
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.plan_conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);
  assert.equal(sent.status(),202);const payload=sent.request().postDataJSON(),run=(await sent.json()).data;assert.equal(payload.profile_id,fixture.project_create_profile_id);assert.deepEqual(payload.context_refs,[{type:'agent_plan',id:fixture.plan_id,version_id:null}]);assert.equal(run.state,'queued');
  const messages=await request('/conversations/'+fixture.plan_conversation_id+'/messages?limit=20'),saved=messages.items.find(value=>value.body===text);assert.ok(saved);assert.equal(saved.run_id,run.id);assert.equal(saved.source_refs.length,1);assert.equal(saved.source_refs[0].type,'agent_plan');assert.equal(saved.source_refs[0].id,fixture.plan_id);assert.equal(saved.source_refs[0].sha256,fixture.plan_hash);
  checks.push({name:'A real RunCreate persists the exact selected intent and server-verified source hash',passed:true});
  await page.getByRole('heading',{name:'任务当前进展',exact:true}).waitFor();await page.getByLabel('取消任务的原因',{exact:true}).fill('Finish isolated reference verification');
  const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);assert.equal(cancelled.status(),200);const cancel=(await cancelled.json()).data;assert.ok(cancel.cancel_requested_at);
  assert.equal((await request('/runs/'+run.id)).state,cancel.state);await page.getByText(cancel.state==='cancelled'?'已确认取消。已提交的历史业务事实仍保留。':'已请求取消，执行停止尚待核对，请以实际结果为准。',{exact:true}).waitFor();
  checks.push({name:'Cancellation feedback agrees with the actual server state',passed:true});
  await page.screenshot({path:path.join(evidence,'plan-reference.png'),fullPage:true});assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){errors.push(String(error));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally{for(const c of contexts)await c.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sourceFiles,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Web/native isolated API and controlled MCP fixture; no actual SDK model, cloud or production activation.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
