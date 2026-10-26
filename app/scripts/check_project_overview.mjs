import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { access, mkdir, readdir, readFile, writeFile } from 'node:fs/promises';
import { createHash, createHmac } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const app=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..'),root=path.dirname(app);
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
const evidence=path.join(root,'docs/quality/evidence/project-overview-web'),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
const executablePath='C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
async function files(directory){const result=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['dist','node_modules','.taro','.swc'].includes(entry.name))continue;const relative=path.posix.join(directory,entry.name);if(entry.isDirectory())result.push(...await files(relative));else result.push(relative);}return result;}
const sources=[...new Set([...(await files('backend')).filter(name=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(name)),...await files('web/src'),'web/vite.config.ts','web/package.json','package.json','package-lock.json','packages/contracts/src/api.d.ts','packages/design/src/tokens.json','scripts/check_project_overview.mjs','scripts/run_project_overview_browser.py'])].sort();
async function digest(){const hash=createHash('sha256');for(const name of sources){hash.update(name+'\0');hash.update(await readFile(path.join(app,name)));hash.update('\0');}return hash.digest('hex');}
const before=await digest();await writeFile(path.join(evidence,'source-manifest.json'),JSON.stringify({algorithm:'sha256(relative POSIX path + NUL + bytes + NUL)',files:sources},null,2)+'\n');
const contexts=[],checks=[],layouts=[],errors=[],serverErrors=[],cleanupErrors=[];let browser,passed=false,failure=null;const lastSteps=new Map();
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';let bits='';for(const c of secret.replace(/=+$/,''))bits+=alphabet.indexOf(c).toString(2).padStart(5,'0');const key=Buffer.from(bits.match(/.{8}/g).map(byte=>parseInt(byte,2)));const counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const hash=createHmac('sha1',key).update(counter).digest();return((hash.readUInt32BE(hash[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
async function code(account){const previous=lastSteps.get(account.login)??account.last_step;while(Math.floor(Date.now()/30000)<=previous)await new Promise(resolve=>setTimeout(resolve,Math.min(1000,(previous+1)*30000-Date.now()+100)));const step=Math.floor(Date.now()/30000);lastSteps.set(account.login,step);return otp(account.mfa_secret,step);}
async function login(account){const context=await browser.newContext({viewport:{width:1440,height:1050},colorScheme:'light',reducedMotion:'reduce'});contexts.push(context);const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));page.on('response',response=>{if(response.status()>=500)serverErrors.push({status:response.status(),path:new URL(response.url()).pathname});});await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(account.login);await page.getByLabel('密码',{exact:true}).fill(account.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();if(account.mfa_secret){await page.getByLabel('六位验证码').fill(await code(account));await page.getByRole('button',{name:'验证并继续',exact:true}).click();}await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();return{context,page};}
async function request(context,route,method='GET',body){const cookies=await context.cookies(fixture.origin),csrf=cookies.find(value=>value.name==='cp_csrf')?.value;const result=await context.request.fetch(fixture.origin+'/v1'+route,{method,headers:{Origin:fixture.origin,'Content-Type':'application/json',...(method!=='GET'?{'X-CSRF-Token':decodeURIComponent(csrf??''),'Idempotency-Key':crypto.randomUUID()}: {})},...(body?{data:body}:{})});const value=await result.json();assert(result.ok(),`${method} ${route} returned ${result.status()} ${value.error?.code}`);return value.data;}
async function modal(page,name){const dialog=page.getByRole('dialog',{name,exact:true});await dialog.waitFor();return dialog;}
async function action(page,name,reason){await page.getByRole('button',{name,exact:true}).click();const dialog=await modal(page,name);await dialog.getByLabel('本次操作原因',{exact:true}).fill(reason);await dialog.getByRole('checkbox',{name:/我已核对固定修订/}).check();await dialog.getByRole('button',{name,exact:true}).click();await dialog.waitFor({state:'hidden'});}
async function layout(page,name){for(const [theme,width]of[['light',1440],['dark',390],['light',320]]){await page.getByLabel('外观偏好',{exact:true}).selectOption(theme);await page.setViewportSize({width,height:1050});await page.evaluate(()=>document.fonts.ready);await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));await page.waitForFunction(()=>document.getAnimations().every(animation=>animation.playState!=='running'||animation.effect?.getComputedTiming().iterations===Infinity));await page.waitForFunction(()=>{const sidebar=document.querySelector('.sidebar'),narrow=matchMedia('(max-width:899px)').matches;return sidebar&&(narrow?sidebar.inert:!sidebar.inert);});const overflow=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,nodes:[...document.querySelectorAll('body *')].filter(n=>{const r=n.getBoundingClientRect();return r.width&&r.right>innerWidth+1;}).slice(0,10).map(n=>({tag:n.tagName,classes:n.className,text:n.textContent.slice(0,70),width:n.getBoundingClientRect().width,right:n.getBoundingClientRect().right}))}));if(overflow.scroll>width+1){layouts.push({name,theme,width,passed:false,overflow});await page.screenshot({path:path.join(evidence,'overflow-'+width+'.png'),fullPage:true});}assert.equal(overflow.scroll>width+1,false,name+' overflows '+width);layouts.push({name,theme,width,passed:true});if(width!==320)await page.screenshot({path:path.join(evidence,name+'-'+theme+'-'+width+'.png'),fullPage:true});}await page.setViewportSize({width:1440,height:1050});}

try {
 browser=await chromium.launch({executablePath,headless:true,args:['--disable-gpu','--no-first-run','--disable-background-networking']});
 const alice=await login(fixture.alice),bob=await login(fixture.bob),page=alice.page,pid=fixture.project_id;
 const url=fixture.origin+'/projects/'+pid+'?tab=overview';
 const overview=await request(alice.context,'/projects/'+pid+'/overview');
 assert.equal(overview.groups.length,16);assert.equal(overview.scope,'readable_records');
 await page.goto(url);await page.getByRole('heading',{name:'项目业务概览',exact:true}).waitFor();
 await page.locator('.overview-card').first().waitFor();assert.equal(await page.locator('.overview-card').count(),16);
 for(const group of overview.groups){const card=page.locator('[data-overview-kind="'+group.kind+'"]');assert.equal((await card.locator('.overview-count').innerText()).trim(),group.available?String(group.count):'—');assert.equal(await card.locator('.overview-items li').count(),group.items.length);}
 const tasks=overview.groups.find(g=>g.kind==='tasks');assert(tasks.count>5);assert.equal(tasks.items.length,5);
 await page.getByText('仅预览最近 5 项，共 '+tasks.count+' 项可读记录。',{exact:true}).waitFor();
 const finance=page.getByRole('region',{name:'项目财务概览',exact:true});
 await finance.getByText('45,035,996,273,805.050000',{exact:true}).waitFor();await finance.getByText('100.100000',{exact:true}).waitFor();
 await page.getByText('这里展示服务配置；打开服务后核对当前健康状态。',{exact:true}).waitFor();
 await layout(page,'project-overview');
 checks.push({name:'Actual sixteen-domain readable counts match the API beyond five-row previews; precise project allocations, currency separation, original costs and reversals render at three sizes',passed:true});

 const task=tasks.items[0];await page.locator('[data-overview-kind="tasks"] a[href*="task='+task.id+'"]').click();
 await page.waitForURL('**task='+task.id);let dialog=await modal(page,'任务详情');assert.equal((await dialog.locator('.task-name').innerText()).trim(),task.title);await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 await page.getByRole('tab',{name:'项目概览',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('tab'),'overview');
 await page.locator('[data-overview-kind="tasks"]').getByRole('link',{name:'查看任务',exact:true}).click();await page.getByRole('tabpanel',{name:'任务',exact:true}).waitFor();
 await page.getByRole('tab',{name:'项目概览',exact:true}).click();
 for(const [kind,route] of [['contracts','contracts'],['costs','costs'],['files','files'],['posts','knowledge'],['services','services']]){
  const group=overview.groups.find(g=>g.kind===kind);assert(group.items.length>0);const item=group.items[0];
  await page.locator('[data-overview-kind="'+kind+'"] a[href="/'+route+'/'+item.id+'"]').click();await page.waitForURL('**/'+route+'/'+item.id);
  await page.getByRole('heading',{name:item.title,exact:true}).first().waitFor();
  await page.goto(url);await page.locator('[data-overview-kind="'+kind+'"]').waitFor();
 }
 checks.push({name:'Stable task IDs open the original task dialog; same-page tab links and contract/cost/file/log/service detail links complete real API navigation without substituted responses',passed:true});

 const narrow=await request(bob.context,'/projects/'+pid+'/overview');
 await bob.page.goto(url);await bob.page.locator('.overview-card').first().waitFor();
 for(const group of narrow.groups){const card=bob.page.locator('[data-overview-kind="'+group.kind+'"]');assert.equal((await card.locator('.overview-count').innerText()).trim(),group.available?String(group.count):'—');if(!group.available){assert.equal(await card.locator('.overview-items li').count(),0);await card.getByText('未核实 · 当前读取范围不足',{exact:true}).waitFor();}}
 assert.equal(narrow.incurred.available,false);await bob.page.getByText('发生支出未核实 · 当前报表读取范围不足',{exact:true}).waitFor();
 assert.equal(await bob.page.getByText('Unpublished overview secret',{exact:true}).count(),0);assert.equal(await bob.page.getByText('Unpublished private draft',{exact:true}).count(),0);assert.equal(await bob.page.getByRole('button',{name:'编辑项目',exact:true}).count(),0);
 assert.equal(await bob.page.getByText('Hidden other contract',{exact:true}).count(),0);
 await layout(bob.page,'project-overview-reader');
 let project=await request(alice.context,'/projects/'+pid);const bobMe=await request(bob.context,'/me');
 project=await request(alice.context,'/projects/'+pid+'/members','POST',{expected_version:project.version,user_id:bobMe.user.id,action:'remove',reason:'Revoke exact synthetic overview reader'});
 await bob.page.getByRole('button',{name:'刷新概览',exact:true}).click();await bob.page.getByRole('heading',{name:'回到工作空间',exact:true}).waitFor();assert.equal(await bob.page.locator('.overview-card').count(),0);assert.equal(await bob.page.getByText('Reviewed overview log',{exact:true}).count(),0);
 checks.push({name:'Narrow viewer receives only currently authorized counts and published log revisions; actual membership removal revokes the session and clears all cached overview facts',passed:true});

 project=await request(alice.context,'/projects/'+pid+'/transition','POST',{expected_version:project.version,action:'cancel',reason:'Synthetic read-only project closure'});
 await page.goto(url);await page.getByText('此项目已结束，项目内容为只读。',{exact:true}).waitFor();await page.locator('.overview-card').first().waitFor();
 const cancelled=await request(alice.context,'/projects/'+pid+'/overview');assert.equal(cancelled.project_version,project.version);assert.deepEqual(cancelled.incurred,overview.incurred);
 assert.equal(await page.getByRole('button',{name:'编辑项目',exact:true}).count(),0);await page.getByRole('button',{name:'刷新概览',exact:true}).click();
 await page.locator('[data-overview-kind="payments"] .overview-count').getByText('1',{exact:true}).waitFor();
 checks.push({name:'Cancelled project retains accurate incurred/payment history and readable references while the original project remains read-only',passed:true});
 assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){failure=String(error.stack??error);process.stderr.write(failure+'\n');}
finally{
 for(const context of contexts){try{await context.close();}catch{cleanupErrors.push('owned context cleanup failed');}}if(browser){try{await browser.close();}catch{cleanupErrors.push('owned browser cleanup failed');}}
 const after=await digest(),closed=!browser||!browser.isConnected();passed=passed&&before===after&&closed&&!cleanupErrors.length;
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,executed_at:new Date().toISOString(),source_sha256:before,source_file_count:sources.length,source_unchanged_during_tests:before===after,real_web_api:true,isolated_native_database:true,real_model:false,real_cloud:false,failure,checks,layouts,errors,server_errors:serverErrors,cleanup_errors:cleanupErrors,owned_browser_closed:closed,all_owned_browser_tabs_closed:closed},null,2)+'\n');
}
process.stdout.write(JSON.stringify({passed,checks:checks.length,layouts:layouts.length})+'\n');process.exitCode=passed?0:1;
