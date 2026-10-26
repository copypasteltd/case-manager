// Real Taro H5 + native HTTP/PostgreSQL. No device or mini-program runtime claim.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { otp } from './notification_browser_common.mjs';

const app=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),root=path.dirname(app);
const evidence=path.join(root,'docs/quality/evidence/mobile-project-overview-browser');
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.fixture_kind,'project-overview');assert.equal(fixture.isolated_database,true);
assert.equal(fixture.origin,'http://localhost:5189');assert.equal(new URL(fixture.api_origin).hostname,'127.0.0.1');
const manifest=JSON.parse(await readFile(path.join(evidence,'source-manifest.json'),'utf8'));
async function digest(){const hash=createHash('sha256');for(const file of manifest.files){hash.update(file+'\0');hash.update(await readFile(path.join(app,file)));hash.update('\0');}return hash.digest('hex');}
const before=await digest();assert.equal(before,manifest.sha256);
const temporary=path.join(app,'.local/browser-temp');await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const executablePath='C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
const contexts=[],checks=[],layouts=[],errors=[],serverErrors=[],cleanupErrors=[];let browser,passed=false,failure=null;
const labels={tasks:'任务',milestones:'里程碑',deliverables:'交付',risks:'风险',issues:'问题',changes:'变更',budgets:'预算修订',files:'文件',posts:'知识与日志',contracts:'合同',purchases:'采购',claims:'报销',costs:'实际费用',payments:'收付款',subscriptions:'订阅',services:'服务配置'};
const screen=page=>page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(page,name)=>screen(page).getByRole('button',{name,exact:true});
const esc=value=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
const section=(page,title)=>screen(page).locator('.cp-section-title').filter({hasText:new RegExp('^'+esc(title)+'$')}).locator('..');
const amount=value=>{const [whole,fraction]=value.split('.');return whole.replace(/\B(?=(\d{3})+(?!\d))/g,',')+(fraction===undefined?'':'.'+fraction);};
async function heading(page,title){await screen(page).locator('.cp-title').filter({hasText:title}).waitFor();}
async function navigate(page,url){await page.evaluate(value=>{window.location.hash='#'+value;},url);}
async function login(account){
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'});contexts.push(context);
 const page=await context.newPage();page.setDefaultTimeout(25000);page.on('pageerror',error=>errors.push(error.message));page.on('response',response=>{if(response.status()>=500)serverErrors.push({status:response.status(),path:new URL(response.url()).pathname});});
 await page.goto(fixture.origin);await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(account.login);await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(account.password);
 const pending=page.waitForResponse(response=>response.url().endsWith('/v1/auth/login')&&response.request().method()==='POST');await button(page,'登录工作空间 ↗').click();let result=await(await pending).json();
 if(account.mfa_secret){while(Math.floor(Date.now()/30000)<=account.last_step)await new Promise(resolve=>setTimeout(resolve,500));
  await screen(page).locator('input[maxlength="6"]').fill(otp(account.mfa_secret,Math.floor(Date.now()/30000)));
  const verified=page.waitForResponse(response=>response.url().endsWith('/v1/auth/mfa/verify')&&response.request().method()==='POST');await button(page,'验证并继续 ↗').click();result=await(await verified).json();}
 assert.equal(result.data?.step,'authenticated');await heading(page,'工作从这里开始');return{page,context,token:result.data.access_token};
}
async function request(token,route,method='GET',body){
 const response=await fetch(fixture.api_origin+'/v1'+route,{method,headers:{Authorization:'Bearer '+token,'Content-Type':'application/json',...(method==='GET'?{}:{'Idempotency-Key':'mobile-overview:'+randomUUID()})},...(body===undefined?{}:{body:JSON.stringify(body)})});
 const value=await response.json();assert.ok(response.ok,`${method} ${route}: ${response.status} ${value.error?.code??''}`);return value.data;
}
async function stable(page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});await page.waitForFunction(()=>document.getAnimations().every(animation=>animation.playState!=='running'||animation.effect?.getComputedTiming().iterations===Infinity));}
async function layout(page,name){
 const title=await screen(page).locator('.cp-title').textContent();
 for(const[theme,label]of[['light','浅色'],['dark','深色']]){
  await button(page,'我的').click();await heading(page,'我的工作空间');await button(page,label).click();await button(page,'返回上一页').click();await heading(page,title);await screen(page).locator('.cp-section').first().waitFor();
  for(const width of[320,390,768]){await page.setViewportSize({width,height:900});await stable(page);
   const measured=await screen(page).evaluate(panel=>({width:panel.getBoundingClientRect().width,x:panel.getBoundingClientRect().x,overflow:document.documentElement.scrollWidth>innerWidth+1,small:[...panel.querySelectorAll('.cp-button,.cp-link,.cp-row')].filter(node=>node.getBoundingClientRect().height<43.5).map(node=>node.textContent)}));
   assert.ok(Math.abs(measured.width-width)<=1&&Math.abs(measured.x)<=1,`${name} unsettled width`);assert.equal(measured.overflow,false,`${name}/${theme}/${width} overflow`);assert.deepEqual(measured.small,[],`${name} touch targets`);
   if(width===390)await page.screenshot({path:path.join(evidence,name+'-'+theme+'-390.png'),fullPage:true});layouts.push({page:name,theme,width,passed:true});
  }
 }
 await page.setViewportSize({width:390,height:900});
}

try{
 browser=await chromium.launch({executablePath,headless:true});
 const alice=await login(fixture.alice),bob=await login(fixture.bob),pid=fixture.project_id,page=alice.page;
 const project=await request(alice.token,'/projects/'+pid),basis=await request(alice.token,'/projects/'+pid+'/change-basis?currency=HKD');
 const created=[];
 // These are deliberate local test proposals through ordinary commands, not
 // business data seeded in daily development or direct metadata fixtures.
 for(let n=0;n<23;n++)created.push(await request(alice.token,'/projects/'+pid+'/changes','POST',{base_project_version:basis.project_version,base_budget_id:basis.budget_id,reason:'Synthetic mobile read/pagination proposal',content:{title:'移动预算提案 '+String(n+1).padStart(2,'0'),category:'mobile-overview',currency:'HKD',scope:basis.scope+'\nSynthetic proposed scope.',scope_delta:'Explicit proposed addition.',schedule_delta:'Keep the original dates.',start_date:basis.start_date,due_date:basis.due_date,budget_lines:[{category:'mobile-review',period_start:'2026-10-01',period_end:'2026-10-31',amount:'90071992547409.90'}],file_version_ids:[]}}));
 const overview=await request(alice.token,'/projects/'+pid+'/overview'),budgetPage=await request(alice.token,'/projects/'+pid+'/budgets?limit=100'),oldBudget=budgetPage.items.find(item=>item.change_id===created[0].id);assert.ok(oldBudget);assert.equal(overview.budgets.length,0,'Unapproved draft budgets are not current approved amounts');
 await screen(page).locator('.cp-row-title').getByText('项目',{exact:true}).click();await heading(page,'项目');await screen(page).locator('.cp-row-title').getByText(project.name,{exact:true}).click();await heading(page,project.name);
 await screen(page).locator('.cp-row-title').getByText('项目业务概览',{exact:true}).click();await heading(page,'项目业务概览');await section(page,'任务 · '+overview.groups.find(group=>group.kind==='tasks').count).waitFor();
 for(const group of overview.groups){const card=section(page,labels[group.kind]+' · '+(group.available?group.count:'—'));assert.equal(await card.count(),1,group.kind);assert.equal(await card.locator('.cp-row').count(),group.available?group.items.length:0);if(!group.available)await card.getByText('未核实 · 当前读取范围不足',{exact:true}).waitFor();}
 const tasks=overview.groups.find(group=>group.kind==='tasks');assert.ok(tasks.count>5);await screen(page).getByText('仅预览最近 5 项，共 '+tasks.count+' 项可读记录。',{exact:true}).waitFor();
 const financial=section(page,'本项目发生支出');for(const total of overview.incurred.totals){await financial.getByText('费用原额 '+amount(total.expense_amount),{exact:true}).waitFor();await financial.getByText('冲销原额 '+amount(total.reversal_amount),{exact:true}).waitFor();await financial.getByText('净发生额 '+amount(total.net_amount),{exact:true}).waitFor();}
 await section(page,'当前批准预算').getByText('暂无当前批准预算。',{exact:true}).waitFor();await screen(page).getByText('这里展示服务配置；打开服务后核对当前健康状态。',{exact:true}).waitFor();
 await layout(page,'mobile-project-overview');checks.push({name:'Actual sixteen authorized categories, complete counts beyond five previews, precise project allocations/reversals and unapproved-budget separation match native HTTP facts',passed:true});

 let writes=0;const count=request=>{if(['POST','PATCH'].includes(request.method())&&!new URL(request.url()).pathname.startsWith('/v1/auth/'))writes++;};page.on('request',count);
 const overviewURL='/pages/project/index?id='+pid+'&section=overview';
 for(const kind of['tasks','contracts','purchases','claims','files','posts','services','costs','payments','subscriptions','changes']){
  const group=overview.groups.find(value=>value.kind===kind);assert.ok(group.items.length,kind+' needs a real record');const item=group.items[0];
  const read=page.waitForResponse(response=>new URL(response.url()).pathname==='/v1/'+kind+'/'+item.id&&response.request().method()==='GET');await section(page,labels[kind]+' · '+group.count).locator('.cp-row').first().click();const readResponse=await read;assert.equal(readResponse.status(),200);assert.equal((await readResponse.json()).data.id,item.id);await heading(page,kind==='services'?'服务详情':item.title);if(kind==='services')await section(page,item.title).waitFor();
  if(kind==='costs'){const actual=await request(alice.token,'/costs/'+item.id);await screen(page).getByText(actual.current_revision_id,{exact:true}).waitFor();await section(page,actual.entry_kind==='reversal'?'冲销费用原额':'费用原额').getByText((actual.currency??'币种未核实')+' '+amount(actual.amount),{exact:true}).waitFor();await layout(page,'mobile-project-cost');}
  if(kind==='payments'){const allocations=await request(alice.token,'/payments/'+item.id+'/allocations?limit=20');assert.ok(allocations.items.length);await section(page,'当前可读分配流水').getByRole('button',{name:'查看对应费用当前资料',exact:true}).first().waitFor();await screen(page).getByText('登记是已有资金事实的记录，不会自动发起银行付款，也不增加第二份发生支出。',{exact:true}).waitFor();}
  if(kind==='subscriptions'){await section(page,'当前可读账期').getByText('暂无已生成的可读账期；不据此判断没有订阅义务。',{exact:true}).waitFor();await screen(page).getByText('内部取消与供应商实际停费分别核对。价格和账期是预测，不自动成为费用或付款。',{exact:true}).waitFor();}
  if(kind==='changes'){await screen(page).getByText('尚未实施；批准不等于已改变项目',{exact:true}).waitFor();await section(page,'预算提案').getByText('HKD 90,071,992,547,409.900000',{exact:true}).first().waitFor();}
  await page.goBack();await heading(page,'项目业务概览');await section(page,'任务 · '+tasks.count).waitFor();
 }
 assert.equal(writes,0,'Read navigation must not issue business commands');page.off('request',count);
 checks.push({name:'Eleven original task/business/file/log/service/finance routes complete real reads; exact cost revisions, actual payment allocations and subscription empty obligations remain distinct; navigation issues no business commands',passed:true});

 await button(page,'查看变更').click();await heading(page,'项目变更');await section(page,'当前可读变更').locator('.cp-row').first().waitFor();assert.equal(await section(page,'当前可读变更').locator('.cp-row').count(),20);await button(page,'加载更多项目变更').click();await section(page,'当前可读变更').locator('.cp-row-title').getByText('移动预算提案 01',{exact:true}).waitFor();assert.equal(await section(page,'当前可读变更').locator('.cp-row').count(),23);await section(page,'当前可读变更').locator('.cp-row-title').getByText('移动预算提案 01',{exact:true}).click();await heading(page,'移动预算提案 01');
 await button(page,'查看所属项目').click();await heading(page,project.name);await screen(page).locator('.cp-row-title').getByText('项目业务概览',{exact:true}).click();await heading(page,'项目业务概览');const originalPreview=overview.groups.find(group=>group.kind==='budgets').items[0];const selectedBudget=budgetPage.items.find(item=>item.id===originalPreview.id);assert.ok(selectedBudget);const originalCard=section(page,'预算修订 · 23');await originalCard.waitFor();for(let n=0;n<22;n++)await request(alice.token,'/projects/'+pid+'/changes','POST',{base_project_version:basis.project_version,base_budget_id:basis.budget_id,reason:'Synthetic newer proposal moves the selected preview to page two',content:{title:'后续预算提案 '+String(n+1).padStart(2,'0'),category:'mobile-overview',currency:'HKD',scope:basis.scope+' Synthetic later scope.',scope_delta:'Explicit later addition.',schedule_delta:'Keep dates.',start_date:basis.start_date,due_date:basis.due_date,budget_lines:[{category:'mobile-review',period_start:'2026-10-01',period_end:'2026-10-31',amount:'90071992547409.90'}],file_version_ids:[]}});await originalCard.locator('.cp-row').first().click();await heading(page,'项目预算修订');await screen(page).getByText('所选预算修订尚未载入或当前不可读；有后续页时请继续读取，不以其他修订替代。',{exact:true}).waitFor();await button(page,'加载更多预算修订').click();const chosen=section(page,'HKD · 预算修订 '+selectedBudget.revision+' · 所选修订');await chosen.waitFor();await chosen.getByText(selectedBudget.id,{exact:true}).waitFor();await chosen.getByText('草稿',{exact:true}).waitFor();await chosen.getByText('HKD 90,071,992,547,409.900000',{exact:true}).first().waitFor();
 const selectedChange=await request(alice.token,'/changes/'+selectedBudget.change_id);await chosen.getByRole('button',{name:/查看来源变更/}).click();await heading(page,selectedChange.revision.content.title);await page.goBack();await heading(page,'项目预算修订');await page.goBack();await heading(page,'项目业务概览');await section(page,'任务 · '+tasks.count).waitFor();checks.push({name:'Actual proposal and budget lists paginate beyond twenty; twenty-two newer proposals move the original clicked preview beyond page one without replacing its selected ID or source proposal',passed:true});

 const sensitive=overview.groups.find(group=>group.kind==='costs').items[0];await section(page,'实际费用 · '+overview.groups.find(group=>group.kind==='costs').count).locator('.cp-row').first().click();await heading(page,sensitive.title);
 const path='/v1/costs/'+sensitive.id;await page.route('**'+path,route=>route.abort('failed'));await button(page,'刷新业务资料').click();await screen(page).locator('.cp-notice.cp-danger').waitFor();assert.equal(await section(page,'实际分摊').count(),0);assert.equal(await screen(page).getByText(sensitive.title,{exact:true}).count(),0);await page.unroute('**'+path);await button(page,'刷新业务资料').click();await heading(page,sensitive.title);
 await page.goBack();await heading(page,'项目业务概览');await section(page,'任务 · '+tasks.count).waitFor();const overviewPath='/v1/projects/'+pid+'/overview';await page.route('**'+overviewPath,route=>route.abort('failed'));await button(page,'刷新项目概览').click();await screen(page).getByText('概览未核实，请重新读取。',{exact:true}).waitFor();assert.equal(await section(page,'本项目发生支出').count(),0);await page.unroute('**'+overviewPath);await button(page,'刷新项目概览').click();await section(page,'任务 · '+tasks.count).waitFor();checks.push({name:'Explicit transport fault injection clears prior cost and overview facts; actual API retry restores them without stale amounts or fabricated empty results',passed:true});

 const narrow=await request(bob.token,'/projects/'+pid+'/overview');await screen(bob.page).locator('.cp-row-title').getByText('项目',{exact:true}).click();await heading(bob.page,'项目');await screen(bob.page).locator('.cp-row-title').getByText(project.name,{exact:true}).click();await heading(bob.page,project.name);await screen(bob.page).locator('.cp-row-title').getByText('项目业务概览',{exact:true}).click();await heading(bob.page,'项目业务概览');await section(bob.page,'任务 · '+narrow.groups.find(group=>group.kind==='tasks').count).waitFor();
 for(const group of narrow.groups){const card=section(bob.page,labels[group.kind]+' · '+(group.available?group.count:'—'));await card.waitFor();if(!group.available){assert.equal(await card.locator('.cp-row').count(),0);await card.getByText('未核实 · 当前读取范围不足',{exact:true}).waitFor();}}
 assert.equal(narrow.incurred.available,false);await screen(bob.page).getByText('发生支出未核实 · 当前报表读取范围不足',{exact:true}).waitFor();assert.equal(await screen(bob.page).getByText('Unpublished overview secret',{exact:true}).count(),0);await layout(bob.page,'mobile-project-reader');
 await navigate(bob.page,'/pages/finance/index?kind=cost&id='+sensitive.id);await screen(bob.page).locator('.cp-notice.cp-danger').waitFor();assert.equal(await screen(bob.page).getByText(sensitive.title,{exact:true}).count(),0);await bob.page.goBack();await heading(bob.page,'项目业务概览');await section(bob.page,'任务 · '+tasks.count).waitFor();checks.push({name:'Narrow viewer retains published knowledge and authorized counts, cannot infer financial amounts/hidden categories, and direct unauthorized finance navigation exposes no cached record',passed:true});

 const bobMe=await request(bob.token,'/me'),members=await request(alice.token,'/projects/'+pid+'/members'),bobUser=members.items.find(user=>user.id===bobMe.user.id);assert.ok(bobUser);
 let currentProject=await request(alice.token,'/projects/'+pid);await request(alice.token,'/projects/'+pid+'/members','POST',{expected_version:currentProject.version,user_id:bobUser.id,action:'remove',reason:'Synthetic real mobile overview revocation'});await button(bob.page,'刷新项目概览').click();await screen(bob.page).getByRole('textbox',{name:'内部账号',exact:true}).waitFor();assert.equal(await screen(bob.page).getByText(project.name,{exact:true}).count(),0);
 currentProject=await request(alice.token,'/projects/'+pid);await request(alice.token,'/projects/'+pid+'/transition','POST',{expected_version:currentProject.version,action:'cancel',reason:'Synthetic actual project read-only history'});await button(page,'刷新项目概览').click();await section(page,'任务 · '+tasks.count).waitFor();const cancelled=await request(alice.token,'/projects/'+pid+'/overview');assert.deepEqual(cancelled.incurred,overview.incurred);await screen(page).getByText(/已取消 · 版本/).waitFor();checks.push({name:'Actual membership removal invalidates the viewer session and clears old facts; a cancelled project preserves authorized financial/record history through the same read-only overview',passed:true});
 assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){failure=String(error.stack??error);process.stderr.write(failure+'\n');for(const context of contexts){for(const page of context.pages()){try{await page.screenshot({path:path.join(evidence,'failure-'+contexts.indexOf(context)+'.png'),fullPage:true});}catch{}}}}
finally{
 for(const context of contexts){try{await context.close();}catch{cleanupErrors.push('owned context cleanup failed');}}if(browser){try{await browser.close();}catch{cleanupErrors.push('owned browser cleanup failed');}}
 const after=await digest(),closed=!browser||!browser.isConnected();passed=passed&&before===after&&closed&&!cleanupErrors.length;
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,executed_at:new Date().toISOString(),source_sha256:before,source_file_count:manifest.files.length,source_unchanged_during_tests:before===after,real_taro_h5_api:true,isolated_native_database:true,actual_cloud:false,actual_model:false,actual_device:false,transport_fault_injection:true,failure,checks,layouts,errors,server_errors:serverErrors,cleanup_errors:cleanupErrors,owned_browser_closed:closed,all_owned_browser_tabs_closed:closed},null,2)+'\n');
}
process.stdout.write(JSON.stringify({passed,checks:checks.length,layouts:layouts.length})+'\n');process.exitCode=passed?0:1;
