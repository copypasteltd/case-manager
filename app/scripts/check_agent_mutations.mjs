// Real Web selection and persisted plan context; no provider or cloud calls.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.mutation_fixture,true);assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-mutations-web';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const sourceFiles=['backend/migrations/0050_agent_run_mutations.sql','backend/internal/agent/mutations.go','backend/internal/agent/execution.go','backend/internal/agent/execution_tools.go','backend/internal/agent/execution_results.go','backend/internal/agent/runs.go','backend/internal/agent/run_reads.go','backend/internal/agentworker/mcp.go','backend/internal/agentworker/bridge.go','backend/internal/httpapi/agent_mutations_integration_test.go','web/src/ConversationMutation.tsx','web/src/ConversationMessages.tsx','packages/contracts/src/api.d.ts','packages/contracts/src/mutation.ts','scripts/check_agent_mutations.mjs','scripts/run_agent_browser.py'];
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

  await page.goto(fixture.origin+'/conversations/'+fixture.conversation_id);await page.getByRole('heading',{name:'给公司秘书的请求',exact:true}).waitFor();
  const historical=await request('/runs/'+fixture.committed_run_id);assert.equal(historical.mutation.status,'committed');
  const record=page.locator('article').filter({has:page.locator('code').filter({hasText:historical.mutation.intent_sha256})}).first();await record.getByRole('button',{name:'查看任务进度',exact:true}).click();
  await page.getByRole('heading',{name:'本次更新已提交',exact:true}).waitFor();assert.match(await page.locator('.conversation-source-preview').filter({has:page.getByRole('heading',{name:'本次更新已提交',exact:true})}).innerText(),new RegExp(historical.mutation.operation_id));
  checks.push({name:'Web reads the actual committed update with exact before/after and business operation receipt',passed:true});
  await page.getByRole('button',{name:'收起任务进度',exact:true}).click();
  const target=await request('/tasks/'+fixture.task_id),profile=page.getByLabel('本次秘书任务',{exact:true});await profile.selectOption(fixture.mutation_profile_id);
  let sentCount=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sentCount++;});
  await page.getByRole('button',{name:'选择准确业务更新',exact:true}).click();await page.getByLabel('本次更新任务',{exact:true}).selectOption(fixture.task_id);await page.getByLabel('本次修改字段',{exact:true}).waitFor();
  const title='Web exact selected title '+randomUUID();await page.getByLabel('任务标题的新值',{exact:true}).fill(title);await page.getByLabel('本次更新原因',{exact:true}).fill('Explicit Web L1 update');
  await page.getByRole('button',{name:'将这项明确更新加入请求',exact:true}).click();assert.equal(sentCount,0);assert.equal((await request('/tasks/'+fixture.task_id)).title,target.title);
  const choice=page.locator('section.conversation-source-preview').filter({has:page.getByRole('heading',{name:'本次明确更新',exact:true})});assert.match(await choice.innerText(),new RegExp(title));
  checks.push({name:'Selecting and previewing an exact target version does not send or execute the update',passed:true});
  await page.getByRole('button',{name:'移除本次更新',exact:true}).click();await page.getByRole('button',{name:'选择准确业务更新',exact:true}).click();await page.getByRole('button',{name:'将这项明确更新加入请求',exact:true}).click();assert.equal(sentCount,0);
  checks.push({name:'A user can withdraw the selected change before sending without creating a mutation grant',passed:true});
  const text='Execute only my selected task title '+randomUUID();await page.getByLabel('本次消息内容',{exact:true}).fill(text);
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);
  assert.equal(sent.status(),202);const payload=sent.request().postDataJSON(),run=(await sent.json()).data;assert.equal(payload.mutation.tool,'update_task');assert.deepEqual(payload.mutation.arguments,{id:target.id,expected_version:target.version,title,reason:'Explicit Web L1 update'});assert.equal(run.mutation.status,'authorized');assert.equal(run.mutation.run_id,run.id);assert.equal(sentCount,1);
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(value=>value.run_id===run.id&&value.role==='user');assert.equal(saved.source_refs[0].type,'agent_mutation');assert.equal(saved.source_refs[0].id,run.id);assert.equal(saved.source_refs[0].sha256,run.mutation.intent_sha256);
  checks.push({name:'One explicit send persists the exact server-bound L1 instruction and immutable source hash',passed:true});
  await page.getByRole('heading',{name:'本次明确更新范围',exact:true}).waitFor();assert.equal((await request('/tasks/'+fixture.task_id)).version,target.version);assert.equal(await page.getByRole('button',{name:/批准/}).count(),0);
  checks.push({name:'Queued updates show authorized scope without a false completion claim or a second approval',passed:true});
  await page.getByLabel('取消任务的原因',{exact:true}).fill('Cancel owned browser request before execution');const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);assert.equal(cancelled.status(),200);
  const stopped=await request('/runs/'+run.id);assert.equal(stopped.state,'cancelled');assert.equal(stopped.mutation.status,'authorized');assert.equal(stopped.mutation.operation_id,null);assert.equal((await request('/tasks/'+fixture.task_id)).version,target.version);
  checks.push({name:'Cancelling the queued update preserves its intent, creates no business commit, and leaves the target unchanged',passed:true});
  await page.screenshot({path:path.join(evidence,'mutation-request.png'),fullPage:true});assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){errors.push(String(error));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally{for(const c of contexts)await c.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sourceFiles,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Web/native isolated API and controlled MCP fixture; no actual SDK model, cloud or production activation.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
