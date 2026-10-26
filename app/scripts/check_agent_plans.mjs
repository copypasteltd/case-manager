// Actual Web UI and native API proof; every owned browser context closes below.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.plan_fixture,true);assert.equal(fixture.isolated_database,true);assert.equal(fixture.origin,'http://localhost:5189');
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-plans-web';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const executablePath=process.env.CP_BROWSER_EXECUTABLE??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
const sourceFiles=['backend/internal/agent/plans.go','backend/internal/agent/plan_mutations.go','backend/internal/agent/plan_tools.go','backend/internal/platform/commands.go','backend/internal/httpapi/agent_plans.go','backend/internal/httpapi/agent_plans_integration_test.go','backend/migrations/0049_agent_project_plans.sql','web/src/ConversationPlans.tsx','web/src/ConversationMessages.tsx','packages/contracts/src/api.d.ts','scripts/check_agent_plans.mjs','scripts/run_agent_browser.py'];
async function digest(){const h=createHash('sha256');for(const f of sourceFiles){h.update(f+'\n');h.update(await readFile(path.join(app,f)));}return h.digest('hex');}
const before=await digest(),started=new Date().toISOString(),checks=[],errors=[],serverErrors=[],contexts=[];let browser,passed=false;
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',bits=[...secret.replace(/=+$/,'').toUpperCase()].map(c=>alphabet.indexOf(c).toString(2).padStart(5,'0')).join('');const key=Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const h=createHmac('sha1',key).update(counter).digest();return ((h.readUInt32BE(h[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
try{
  browser=await chromium.launch({executablePath,headless:true,args:['--disable-dev-shm-usage']});
  const context=await browser.newContext({viewport:{width:1440,height:1100},reducedMotion:'reduce'});contexts.push(context);
  const page=await context.newPage();page.setDefaultTimeout(25000);page.on('pageerror',e=>errors.push(e.message));page.on('response',r=>{if(r.status()>=500)serverErrors.push({status:r.status(),path:new URL(r.url()).pathname});});
  await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
  await page.getByLabel('六位验证码').waitFor();const previous=fixture.alice.last_step;
  if(Math.floor(Date.now()/30000)<=previous)await new Promise(r=>setTimeout(r,(previous+1)*30000-Date.now()+300));
  await page.getByLabel('六位验证码').fill(otp(fixture.alice.mfa_secret,Math.floor(Date.now()/30000)));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  async function request(route,method='GET',body){const csrf=(await context.cookies()).find(c=>c.name==='cp_csrf')?.value;const response=await context.request.fetch(fixture.origin+'/v1'+route,{method,headers:{Origin:fixture.origin,'X-CSRF-Token':csrf??'','Idempotency-Key':randomUUID()},...(body?{data:body}:{})});return {status:response.status(),body:await response.json()};}
  await page.goto(fixture.origin+'/conversations/'+fixture.plan_conversation_id);await page.getByRole('heading',{name:'项目创建计划',exact:true}).waitFor();
  await page.getByRole('button',{name:'拟定项目计划',exact:true}).click();const name='Browser fixed plan '+randomUUID();
  await page.getByLabel('项目名称',{exact:true}).fill(name);await page.getByLabel('项目范围',{exact:true}).fill('Browser verifies exact intent and atomic creation.');await page.getByLabel('拟定原因',{exact:true}).fill('Owned browser project flow');
  const [created]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.plan_conversation_id+'/plans'&&r.request().method()==='POST'),page.getByRole('button',{name:'保存待批准计划',exact:true}).click()]);
  assert.equal(created.status(),201);let plan=(await created.json()).data;assert.equal(plan.state,'proposed');assert.equal(plan.project_id,null);assert.equal(plan.project.name,name);
  checks.push({name:'Web drafts exact plan without creating business project',passed:true});
  const denied=await request('/plans/'+plan.id+'/commit','POST',{expected_version:plan.version,plan_hash:plan.plan_hash,reason:'Cannot skip approval'});assert.equal(denied.status,409);assert.equal(denied.body.error.code,'PLAN_APPROVAL_REQUIRED');
  checks.push({name:'Server rejects commit before explicit human approval',passed:true});
  let card=page.locator('article').filter({has:page.getByRole('heading',{name:name+' · 待本人批准',exact:true})});
  assert.match(await card.innerText(),/项目类型：general/);assert.match(await card.innerText(),/未额外指定成员/);assert.match(await card.innerText(),/截止日期：未设置/);assert.match(await card.innerText(),/有效期至 [^\n]*\d{1,2}:\d{2}[^\n]*（香港）/);
  await card.getByLabel('本次操作说明',{exact:true}).fill('Approve the exact browser intent');
  const [approved]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/plans/'+plan.id+'/approve'),card.getByRole('button',{name:'批准这份准确计划',exact:true}).click()]);assert.equal(approved.status(),200);plan=(await approved.json()).data;assert.equal(plan.state,'approved');
  card=page.locator('article').filter({has:page.getByRole('heading',{name:name+' · 已批准，等待创建',exact:true})});await card.getByLabel('本次操作说明',{exact:true}).fill('Create the approved project');
  const [committed]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/plans/'+plan.id+'/commit'),card.getByRole('button',{name:'按已批准计划创建项目',exact:true}).click()]);assert.equal(committed.status(),200);const receipt=(await committed.json()).data;assert.equal(receipt.state,'committed');assert.ok(receipt.operation_id&&receipt.project_id);assert.equal(receipt.plan_hash,plan.plan_hash);
  checks.push({name:'Web approval and commit return a real project and exact business receipt',passed:true});
  const replay=await request('/plans/'+plan.id+'/commit','POST',{expected_version:plan.version,plan_hash:plan.plan_hash,reason:'Reconcile same approved grant'});assert.equal(replay.status,200);assert.equal(replay.body.data.operation_id,receipt.operation_id);assert.equal(replay.body.data.project_id,receipt.project_id);
  checks.push({name:'A second commit key reconciles one project and operation',passed:true});
  await page.getByRole('link',{name:'打开已创建项目',exact:true}).click();await page.getByRole('heading',{name,exact:true}).waitFor();
  const project=await request('/projects/'+receipt.project_id);assert.equal(project.status,200);assert.equal(project.body.data.initialization_state,'complete');assert.equal(project.body.data.name,name);
  checks.push({name:'Created project opens in the existing functional project page',passed:true});
  await page.goto(fixture.origin+'/conversations/'+fixture.plan_conversation_id);await page.getByRole('button',{name:'拟定项目计划',exact:true}).click();await page.getByLabel('项目名称',{exact:true}).fill('Revoked browser '+randomUUID());await page.getByLabel('拟定原因',{exact:true}).fill('Exercise revocation');await page.getByRole('button',{name:'保存待批准计划',exact:true}).click();
  card=page.locator('article').filter({hasText:'Exercise revocation'});await card.getByLabel('本次操作说明',{exact:true}).fill('Withdraw this intent');await card.getByRole('button',{name:'撤销计划',exact:true}).click();await card.getByRole('heading',{name:/已撤销/}).waitFor();
  checks.push({name:'Web revokes unused intent without creating a project',passed:true});
  const current=await request('/conversations/'+fixture.plan_conversation_id),me=await request('/me');assert.equal(current.status,200);assert.equal(me.status,200);
  const dueName='Due-only visible plan '+randomUUID();const due=await request('/conversations/'+fixture.plan_conversation_id+'/plans','POST',{expected_version:current.body.data.version,context_security_version:current.body.data.context_security_version,reason:'Verify all approval fields',project:{name:dueName,scope:'Visible bound dates and type',owner_id:me.body.data.user.id,classification:'internal',type:'client-delivery',due_date:'2027-06-30'}});assert.equal(due.status,201);
  await page.getByRole('button',{name:'重新核对计划状态',exact:true}).click();const dueCard=page.locator('article').filter({has:page.getByRole('heading',{name:dueName+' · 待本人批准',exact:true})});await dueCard.waitFor();const dueText=await dueCard.innerText();assert.match(dueText,/项目类型：client-delivery/);assert.match(dueText,/开始日期：未设置/);assert.match(dueText,/截止日期：2027-06-30/);assert.match(dueText,/未指定；使用标准目录，不生成模板任务/);
  checks.push({name:'Approval card exposes the exact project type and a due date without a start date',passed:true});
  await page.screenshot({path:path.join(evidence,'plans.png'),fullPage:true});assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){errors.push(String(error));}
finally{for(const c of contexts)await c.close().catch(()=>{});if(browser)await browser.close().catch(()=>{});const after=await digest();passed=passed&&before===after;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sourceFiles,checks,errors,server_errors:serverErrors,owned_contexts_closed:true,owned_browser_closed:true,scope:'Native isolated API and real Web UI; no model/provider, cloud or production activation.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:true}));process.exitCode=passed?0:1;
