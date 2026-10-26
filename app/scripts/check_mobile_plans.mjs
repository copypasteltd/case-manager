// Real Taro H5 interactions against the disposable native plan fixture.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.plan_fixture,true);assert.equal(fixture.fixture_kind,'mobile-business');assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-plans-mobile';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TEMP=temporary;process.env.TMP=temporary;
async function sourceFiles(directory){const out=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['dist','node_modules','.taro','.swc'].includes(entry.name))continue;const relative=path.posix.join(directory,entry.name);if(entry.isDirectory())out.push(...await sourceFiles(relative));else out.push(relative);}return out;}
const apiSources=(await sourceFiles('backend')).filter(file=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const sources=[...new Set([...await sourceFiles('mobile'),...apiSources,'package.json','package-lock.json','scripts/check_mobile_plans.mjs','scripts/run_mobile_browser.py','scripts/test_backend.py','scripts/sync_design.py','scripts/build_openapi.py','packages/contracts/src/api.d.ts','packages/design/src/tokens.json'])].sort();
async function digest(){const hash=createHash('sha256');for(const file of sources){hash.update(file+'\0');hash.update(await readFile(path.join(app,file)));hash.update('\0');}return hash.digest('hex');}
const before=await digest(),started=new Date().toISOString(),checks=[],errors=[],serverErrors=[],contexts=[];let browser,passed=false,activePage;
const screen=p=>p.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(p,name)=>screen(p).getByRole('button',{name,exact:true});
const field=(p,name)=>screen(p).getByRole('textbox',{name,exact:true});
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',bits=[...secret.replace(/=+$/,'')].map(c=>alphabet.indexOf(c).toString(2).padStart(5,'0')).join(''),key=Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const h=createHmac('sha1',key).update(counter).digest();return ((h.readUInt32BE(h[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
try{
  const executablePath=process.env.CP_BROWSER_EXECUTABLE??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);browser=await chromium.launch({executablePath,headless:true});
  const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'});contexts.push(context);const page=await context.newPage();activePage=page;page.setDefaultTimeout(25000);
  page.on('pageerror',e=>errors.push(e.message));page.on('response',r=>{if(r.status()>=500)serverErrors.push({status:r.status(),path:new URL(r.url()).pathname});});
  await page.goto(fixture.origin);await page.locator('input[placeholder="你的公司账号"]').fill(fixture.alice.login);await page.locator('input[type="password"]').fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间 ↗',exact:true}).click();
  const code=page.locator('input[maxlength="6"]');await code.waitFor();const previous=fixture.alice.last_step;if(Math.floor(Date.now()/30000)<=previous)await new Promise(r=>setTimeout(r,(previous+1)*30000-Date.now()+250));
  await code.fill(otp(fixture.alice.mfa_secret,Math.floor(Date.now()/30000)));await page.getByRole('button',{name:'验证并继续 ↗',exact:true}).click();
  await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-title').filter({hasText:'公司秘书'}).waitFor();
  await screen(page).locator('.cp-row-title').getByText('Synthetic project plans',{exact:true}).first().click();await screen(page).locator('.cp-title').filter({hasText:'Synthetic project plans'}).waitFor();
  await button(page,'拟定项目计划').click();const name='Mobile planned '+randomUUID();await field(page,'项目名称').fill(name);await field(page,'项目范围').fill('Taro H5 exact project plan.');await field(page,'拟定原因').fill('Owned mobile approval flow');
  const [created]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.plan_conversation_id+'/plans'&&r.request().method()==='POST'),button(page,'保存待批准计划').click()]);assert.equal(created.status(),201);let plan=(await created.json()).data;assert.equal(plan.state,'proposed');assert.equal(plan.project_id,null);
  checks.push({name:'Taro H5 creates a fixed proposal without creating a project',passed:true});
  let card=screen(page).locator('.cp-inset').filter({hasText:name+' · 待本人批准'});await card.getByRole('textbox',{name:'本次操作说明',exact:true}).fill('Approve on mobile');
  const plannedText=await card.innerText();assert.match(plannedText,/项目类型/);assert.match(plannedText,/general/);assert.match(plannedText,/未额外指定成员/);assert.match(plannedText,/截止日期/);assert.match(plannedText,/未指定；使用标准目录，不生成模板任务/);
  const [approved]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/plans/'+plan.id+'/approve'),card.getByRole('button',{name:'批准这份准确计划',exact:true}).click()]);assert.equal(approved.status(),200);plan=(await approved.json()).data;assert.equal(plan.state,'approved');
  checks.push({name:'Mobile consumes current interactive MFA proof for exact human approval',passed:true});
  card=screen(page).locator('.cp-inset').filter({hasText:name+' · 已批准，等待创建'});await card.getByRole('textbox',{name:'本次操作说明',exact:true}).fill('Create this approved project');
  const [committed]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/plans/'+plan.id+'/commit'),card.getByRole('button',{name:'按已批准计划创建项目',exact:true}).click()]);assert.equal(committed.status(),200);const result=(await committed.json()).data;assert.equal(result.state,'committed');assert.ok(result.project_id&&result.operation_id);assert.equal(result.plan_hash,plan.plan_hash);
  checks.push({name:'Mobile creates the exact approved project and shows a real business receipt',passed:true});
  await button(page,'打开已创建项目').click();await screen(page).locator('.cp-title').filter({hasText:name}).waitFor();checks.push({name:'Created project opens through actual Taro project navigation',passed:true});
  await page.screenshot({path:path.join(evidence,'mobile-plan-project.png'),fullPage:true});assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(e){errors.push(String(e));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally{let closed=true;for(const c of contexts)await c.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sources,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Taro H5 and native isolated API only; WeChat/Douyin/Android device execution remains unverified.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:true}));process.exitCode=passed?0:1;
