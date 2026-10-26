import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { access, mkdir, readdir, readFile, writeFile } from 'node:fs/promises';
import { createHash, createHmac } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),root=path.dirname(app);
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
const evidence=path.join(root,'docs/quality/evidence/project-task-views-web'),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const executablePath='C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
async function files(directory){const result=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['dist','node_modules','.taro','.swc'].includes(entry.name))continue;const relative=path.posix.join(directory,entry.name);if(entry.isDirectory())result.push(...await files(relative));else result.push(relative);}return result;}
const sources=[...new Set([...(await files('backend')).filter(name=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)),...await files('web/src'),'web/vite.config.ts','web/package.json','package.json','package-lock.json','packages/contracts/src/api.d.ts','packages/design/src/tokens.json','scripts/check_project_task_views.mjs','scripts/run_project_task_views_browser.py'])].sort();
async function digest(){const hash=createHash('sha256');for(const name of sources){hash.update(name+'\0');hash.update(await readFile(path.join(app,name)));hash.update('\0');}return hash.digest('hex');}
const before=await digest();await writeFile(path.join(evidence,'source-manifest.json'),JSON.stringify({algorithm:'sha256(relative POSIX path + NUL + bytes + NUL)',files:sources},null,2)+'\n');
const contexts=[],checks=[],layouts=[],errors=[],serverErrors=[],cleanupErrors=[];let browser,passed=false,failure=null;const lastSteps=new Map();
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';let bits='';for(const c of secret.replace(/=+$/,''))bits+=alphabet.indexOf(c).toString(2).padStart(5,'0');const key=Buffer.from(bits.match(/.{8}/g).map(byte=>parseInt(byte,2)));const counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const hash=createHmac('sha1',key).update(counter).digest();return((hash.readUInt32BE(hash[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
async function code(account){const previous=lastSteps.get(account.login)??account.last_step;while(Math.floor(Date.now()/30000)<=previous)await new Promise(resolve=>setTimeout(resolve,Math.min(1000,(previous+1)*30000-Date.now()+100)));const step=Math.floor(Date.now()/30000);lastSteps.set(account.login,step);return otp(account.mfa_secret,step);}
async function login(account){const context=await browser.newContext({viewport:{width:1440,height:1050},colorScheme:'light',reducedMotion:'reduce'});contexts.push(context);const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));page.on('response',response=>{if(response.status()>=500)serverErrors.push({status:response.status(),path:new URL(response.url()).pathname});});await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(account.login);await page.getByLabel('密码',{exact:true}).fill(account.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码').fill(await code(account));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();return{context,page};}
async function request(context,route,method='GET',body){const cookies=await context.cookies(fixture.origin),csrf=cookies.find(value=>value.name==='cp_csrf')?.value;const result=await context.request.fetch(fixture.origin+'/v1'+route,{method,headers:{Origin:fixture.origin,'Content-Type':'application/json',...(method!=='GET'?{'X-CSRF-Token':decodeURIComponent(csrf??''),'Idempotency-Key':crypto.randomUUID()}: {})},...(body?{data:body}:{})});const value=await result.json();assert(result.ok(),`${method} ${route} returned ${result.status()} ${value.error?.code}`);return value.data;}
async function modal(page,name){const dialog=page.getByRole('dialog',{name,exact:true});await dialog.waitFor();return dialog;}
async function action(page,name,reason){await page.getByRole('button',{name,exact:true}).click();const dialog=await modal(page,name);await dialog.getByLabel('本次操作原因',{exact:true}).fill(reason);await dialog.getByRole('checkbox',{name:/我已核对固定修订/}).check();await dialog.getByRole('button',{name,exact:true}).click();await dialog.waitFor({state:'hidden'});}
async function layout(page,name){for(const [theme,width]of[['light',1440],['dark',390],['light',320]]){await page.getByLabel('外观偏好',{exact:true}).selectOption(theme);await page.setViewportSize({width,height:1050});await page.evaluate(()=>document.fonts.ready);await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));await page.waitForFunction(()=>document.getAnimations().every(animation=>animation.playState!=='running'||animation.effect?.getComputedTiming().iterations===Infinity));await page.waitForFunction(()=>{const sidebar=document.querySelector('.sidebar'),narrow=matchMedia('(max-width:899px)').matches;return sidebar&&(narrow?sidebar.inert:!sidebar.inert);});const overflow=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,nodes:[...document.querySelectorAll('body *')].filter(n=>{const r=n.getBoundingClientRect();return r.width&&r.right>innerWidth+1;}).slice(0,10).map(n=>({tag:n.tagName,classes:n.className,text:n.textContent.slice(0,70),width:n.getBoundingClientRect().width,right:n.getBoundingClientRect().right}))}));if(overflow.scroll>width+1){layouts.push({name,theme,width,passed:false,overflow});await page.screenshot({path:path.join(evidence,'overflow-'+width+'.png'),fullPage:true});}assert.equal(overflow.scroll>width+1,false,name+' overflows '+width);layouts.push({name,theme,width,passed:true});if(width!==320)await page.screenshot({path:path.join(evidence,name+'-'+theme+'-'+width+'.png'),fullPage:true});}await page.setViewportSize({width:1440,height:1050});}

try {
 browser=await chromium.launch({executablePath,headless:true,args:['--disable-gpu','--no-first-run','--disable-background-networking']});
 const alice=await login(fixture.alice);let bob=await login(fixture.bob);
 const me=await request(alice.context,'/me'),bobMe=await request(bob.context,'/me');
 let project=await request(alice.context,'/projects','POST',{name:'Synthetic task workspace '+Date.now(),scope:'Only fictional browser verification data',type:'general',owner_id:me.user.id,classification:'project_restricted'});
 const created=[];
 async function create(title,extra={}){const task=await request(alice.context,'/projects/'+project.id+'/tasks','POST',{title,body:'Synthetic task evidence',priority:'normal',review_required:false,...extra});created.push(task);return task;}
 async function transition(task,action){return request(alice.context,'/tasks/'+task.id+'/transition','POST',{expected_version:task.version,action,reason:'Controlled synthetic state change'});}
 let predecessor=await create('Synthetic predecessor',{start_at:'2026-10-25T10:00:00Z',due_at:'2026-10-25T16:30:00Z',priority:'high'});
 predecessor=await transition(predecessor,'start');
 let dependent=await create('Synthetic dependent',{due_at:'2026-10-26T16:30:00Z'});
 dependent=await request(alice.context,'/tasks/'+dependent.id+'/dependencies','POST',{expected_version:dependent.version,predecessor_id:predecessor.id,action:'add'});
 await create('Synthetic undated');await create('Synthetic start-only',{start_at:'2026-10-26T10:00:00Z'});
 for(let index=0;index<31;index++){
  let task=await create('Synthetic pagination '+String(index).padStart(2,'0'),{review_required:index===1});
  if(index===0)task=await transition(task,'block');
  if(index===1){task=await transition(task,'start');task=await transition(task,'review');}
  if(index===2){task=await transition(task,'start');task=await transition(task,'complete');}
  if(index===3)await transition(task,'cancel');
 }
 const page=alice.page;await page.goto(fixture.origin+'/projects/'+project.id+'?view=board');
 await page.getByRole('region',{name:'项目任务看板',exact:true}).waitFor();
 await page.getByRole('status').filter({hasText:'已加载 30 项'}).waitFor();
 await page.getByText('还有任务未加载，当前视图与分列数量只涵盖已加载部分。',{exact:false}).waitFor();
 assert.equal(await page.locator('.task-board-card').count(),30);
 await page.getByRole('button',{name:'加载更多任务',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已加载 35 项'}).waitFor();assert.equal(await page.locator('.task-board-card').count(),35);
 const all=(await request(alice.context,'/projects/'+project.id+'/tasks?limit=100')).items;
 for(const [state,name]of[['todo','待开始'],['in_progress','进行中'],['in_review','待验收'],['blocked','受阻'],['done','已完成'],['cancelled','已取消']]){
  const lane=page.getByRole('region',{name:name+'任务列',exact:true});
  // Named sections expose region semantics through their accessible label.
  assert.equal(await lane.locator('.task-board-card').count(),all.filter(task=>task.state===state).length);
 }
 await layout(page,'project-board');
 checks.push({name:'Real 35-task project spans two server pages; six board states and loaded-only counts match current API facts',passed:true});

 await page.getByLabel('搜索任务标题',{exact:true}).fill('Synthetic pagination');await page.getByRole('button',{name:'搜索任务',exact:true}).click();
 await page.getByLabel('任务状态',{exact:true}).selectOption('blocked');
 await page.getByRole('status').filter({hasText:'已加载 1 项'}).waitFor();
 assert.equal(await page.locator('.task-board-card').count(),1);await page.getByRole('button',{name:'时间线',exact:true}).click();
 await page.getByRole('heading',{name:'未排期及日期待核对',exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'Synthetic pagination 00',exact:true}).count(),1);
 assert.equal(new URL(page.url()).searchParams.get('view'),'timeline');
 await page.getByLabel('任务状态',{exact:true}).selectOption('');await page.getByLabel('搜索任务标题',{exact:true}).fill('');await page.getByRole('button',{name:'搜索任务',exact:true}).click();
 await page.getByRole('region',{name:'项目任务时间线',exact:true}).waitFor();
 const dependentRow=page.locator('.task-timeline-row').filter({has:page.getByRole('button',{name:'Synthetic dependent',exact:true})});
 await dependentRow.getByRole('img',{name:/Synthetic dependent：仅截止日期/}).waitFor();
 assert.equal(await page.locator('.task-timeline-row').count(),3);
 assert.equal(await page.locator('.task-undated-row').count(),32);
 await page.getByRole('img',{name:/Synthetic predecessor：计划区间/}).waitFor();
 await page.getByRole('img',{name:/Synthetic start-only：仅开始日期/}).waitFor();
 await page.getByText(/26\/10\/2026.*00:30/).first().waitFor();
 await dependentRow.getByRole('button',{name:'查看前置任务',exact:true}).click();
 await dependentRow.locator('.task-timeline-dependencies').getByRole('button',{name:'Synthetic predecessor',exact:true}).waitFor();
 await layout(page,'project-timeline');
 await dependentRow.getByRole('button',{name:'Synthetic dependent',exact:true}).click();let dialog=await modal(page,'任务详情');
 await dialog.getByRole('button',{name:'开始任务',exact:true}).click();dialog=await modal(page,'开始任务');
 await dialog.getByLabel('变更原因',{exact:true}).fill('Dependency must remain enforced');await dialog.getByRole('button',{name:'开始任务',exact:true}).click();
 await dialog.getByText('前置任务尚未完成',{exact:true}).waitFor();
 assert.equal((await request(alice.context,'/tasks/'+dependent.id)).state,'todo');
 await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();dialog=await modal(page,'任务详情');await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 predecessor=await transition(predecessor,'complete');
 await dependentRow.getByRole('button',{name:'Synthetic dependent',exact:true}).click();dialog=await modal(page,'任务详情');await dialog.getByRole('button',{name:'开始任务',exact:true}).click();dialog=await modal(page,'开始任务');await dialog.getByLabel('变更原因',{exact:true}).fill('Exact predecessor is now done');await dialog.getByRole('button',{name:'开始任务',exact:true}).click();
 dialog=await modal(page,'任务详情');await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 dependent=await request(alice.context,'/tasks/'+dependent.id);assert.equal(dependent.state,'in_progress');
 checks.push({name:'Server title/state filtering persists across views; Hong Kong schedule shows ranges, single dates and undated work; actual dependency blocks start until predecessor completes',passed:true});

 project=await request(alice.context,'/projects/'+project.id);
 project=await request(alice.context,'/projects/'+project.id+'/members','POST',{expected_version:project.version,user_id:bobMe.user.id,project_role:'viewer',action:'set',reason:'Synthetic read-only project access'});
 // Membership changes invalidate that employee's existing credentials. Use a
 // real new MFA session before testing the newly granted viewer authority.
 await bob.context.close();bob=await login(fixture.bob);
 await bob.page.goto(fixture.origin+'/projects/'+project.id+'?view=board');await bob.page.getByRole('region',{name:'项目任务看板',exact:true}).waitFor();
 await bob.page.getByText('你以只读成员身份访问这个项目。',{exact:true}).waitFor();assert.equal(await bob.page.getByRole('button',{name:'新建任务',exact:true}).count(),0);
 await bob.page.getByRole('button',{name:'Synthetic dependent',exact:true}).click();dialog=await modal(bob.page,'任务详情');assert.equal(await dialog.locator('.task-actions button').count(),0);await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 await request(alice.context,'/projects/'+project.id+'/members','POST',{expected_version:project.version,user_id:bobMe.user.id,action:'remove',reason:'Revoke only synthetic viewer access'});
 await bob.page.getByRole('button',{name:'刷新任务',exact:true}).click();await bob.page.getByRole('button',{name:'登录工作空间',exact:true}).waitFor();assert.equal(await bob.page.locator('.task-board-card').count(),0);assert.equal(await bob.page.getByRole('region',{name:'项目任务看板',exact:true}).count(),0);
 checks.push({name:'Actual viewer can read board and task facts without write controls; membership revocation invalidates credentials and removes cached task views on refetch',passed:true});

 project=await request(alice.context,'/projects/'+project.id);project=await request(alice.context,'/projects/'+project.id+'/transition','POST',{expected_version:project.version,action:'cancel',reason:'Cancel synthetic verification workspace'});
 await page.goto(fixture.origin+'/projects/'+project.id+'?view=timeline');await page.getByRole('region',{name:'项目任务时间线',exact:true}).waitFor();
 await page.getByText('此项目已结束，项目内容为只读。',{exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'新建任务',exact:true}).count(),0);
 await page.getByRole('button',{name:'Synthetic dependent',exact:true}).click();dialog=await modal(page,'任务详情');assert.equal(await dialog.getByRole('button',{name:'编辑',exact:true}).count(),0);await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 const cookies=await alice.context.cookies(fixture.origin),csrf=cookies.find(value=>value.name==='cp_csrf')?.value;
 const rejected=await alice.context.request.patch(fixture.origin+'/v1/tasks/'+dependent.id,{headers:{Origin:fixture.origin,'X-CSRF-Token':decodeURIComponent(csrf??''),'Idempotency-Key':crypto.randomUUID()},data:{expected_version:dependent.version,title:'Must not mutate cancelled project'}});
 assert.equal(rejected.status(),409);assert.equal((await rejected.json()).error.code,'PROJECT_READ_ONLY');assert.equal((await request(alice.context,'/tasks/'+dependent.id)).title,'Synthetic dependent');
 checks.push({name:'Cancelled project retains real timeline facts while both UI and existing API keep tasks read-only',passed:true});
 assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;

}catch(error){failure=String(error.stack??error);process.stderr.write(failure+'\n');}
finally{
 for(const context of contexts){try{await context.close();}catch{cleanupErrors.push('owned context cleanup failed');}}if(browser){try{await browser.close();}catch{cleanupErrors.push('owned browser cleanup failed');}}
 const after=await digest(),closed=!browser||!browser.isConnected();passed=passed&&before===after&&closed&&!cleanupErrors.length;
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,executed_at:new Date().toISOString(),source_sha256:before,source_file_count:sources.length,source_unchanged_during_tests:before===after,real_web_api:true,isolated_native_database:true,real_model:false,real_cloud:false,failure,checks,layouts,errors,server_errors:serverErrors,cleanup_errors:cleanupErrors,owned_browser_closed:closed,all_owned_browser_tabs_closed:closed},null,2)+'\n');
}
process.stdout.write(JSON.stringify({passed,checks:checks.length,layouts:layouts.length})+'\n');process.exitCode=passed?0:1;
