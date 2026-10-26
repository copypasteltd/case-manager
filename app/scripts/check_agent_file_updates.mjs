import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.agent_file_update_fixture,true);assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-file-updates-web';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
async function collect(directory){const files=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['node_modules','dist','.taro','.swc'].includes(entry.name))continue;const file=path.posix.join(directory,entry.name);if(entry.isDirectory())files.push(...await collect(file));else files.push(file);}return files;}
const apiFiles=(await collect('backend')).filter(file=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const uiFiles=(await collect('web')).filter(file=>/\.(tsx?|json|css|html)$/.test(file));
const sourceFiles=[...new Set([...apiFiles,...uiFiles,'package.json','package-lock.json','.env.example','packages/contracts/src/api.d.ts','packages/contracts/src/mutation.ts','packages/contracts/src/file-update.ts','packages/contracts/package.json','packages/design/src/tokens.json','scripts/check_agent_file_updates.mjs','scripts/run_agent_browser.py','scripts/test_backend.py','scripts/build_openapi.py'])].sort();
async function digest(){const h=createHash('sha256');for(const file of sourceFiles){h.update(file+'\n');h.update(await readFile(path.join(app,file)));}return h.digest('hex');}
function otp(secret,step){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',bits=[...secret.replace(/=+$/,'').toUpperCase()].map(c=>alphabet.indexOf(c).toString(2).padStart(5,'0')).join(''),key=Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2))),counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(step));const h=createHmac('sha1',key).update(counter).digest();return ((h.readUInt32BE(h[19]&15)&0x7fffffff)%1000000).toString().padStart(6,'0');}
const before=await digest(),started=new Date().toISOString(),checks=[],errors=[],serverErrors=[],contexts=[];let browser,activePage,passed=false,closed=true;
try{
  const executablePath=process.env.CP_BROWSER_EXECUTABLE??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';await access(executablePath);
  browser=await chromium.launch({executablePath,headless:true});const context=await browser.newContext({viewport:{width:1440,height:1100},reducedMotion:'reduce'});contexts.push(context);const page=await context.newPage();activePage=page;page.setDefaultTimeout(25000);
  page.on('pageerror',error=>errors.push(error.message));page.on('response',response=>{if(response.status()>=500)serverErrors.push({status:response.status(),path:new URL(response.url()).pathname});});
  await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码').waitFor();
  if(Math.floor(Date.now()/30000)<=fixture.alice.last_step)await new Promise(resolve=>setTimeout(resolve,(fixture.alice.last_step+1)*30000-Date.now()+250));
  await page.getByLabel('六位验证码').fill(otp(fixture.alice.mfa_secret,Math.floor(Date.now()/30000)));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  async function request(route){const response=await context.request.get(fixture.origin+'/v1'+route);assert.equal(response.status(),200);return (await response.json()).data;}
  await page.goto(fixture.origin+'/conversations/'+fixture.conversation_id);await page.getByRole('heading',{name:'给公司秘书的请求',exact:true}).waitFor();
  let sentCount=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sentCount++;});
  await page.getByRole('button',{name:'查看任务进度',exact:true}).first().click();
  await page.getByRole('heading',{name:'文件更新实际结果',exact:true}).waitFor();
  const receipt=page.locator('section').filter({has:page.getByRole('heading',{name:'文件更新实际结果',exact:true})}).last();
  assert.match(await receipt.innerText(),/正式新版本已提交/);assert.deepEqual(await receipt.locator('pre').allTextContents(),['旧计划','新计划🙂']);
  const completed=await request('/runs/'+fixture.completed_run_id);assert.equal(completed.file_update.status,'committed');assert.equal(completed.file_update.result.file_version.sha256,fixture.content_sha256);assert.equal(completed.file_update.result.file_version.replicas[1].state,'queued');
  checks.push({name:'Web displays actual server diff and scanned formal version receipt',passed:true});
  assert.match(await receipt.innerText(),/第二存储副本按文件详情的实际复制校验状态/);await receipt.scrollIntoViewIfNeeded();await page.screenshot({path:path.join(evidence,'file-update-receipt.png')});
  checks.push({name:'Web keeps queued backup distinct from formal primary commit',passed:true});
  await page.getByRole('button',{name:'收起任务进度',exact:true}).click();
  await page.getByLabel('本次秘书任务',{exact:true}).selectOption(fixture.profile_id);await page.getByRole('button',{name:'选择准确文件更新',exact:true}).click();
  await page.getByLabel('本次更新文件',{exact:true}).locator('option[value="'+fixture.file_id+'"]').waitFor({state:'attached'});await page.getByLabel('本次更新文件',{exact:true}).selectOption(fixture.file_id);
  assert.equal(await page.getByRole('button',{name:'将准确文件更新加入请求',exact:true}).isDisabled(),true);assert.equal(sentCount,0);
  await page.getByLabel('本次更新基础版本',{exact:true}).locator('option[value="'+fixture.current_version_id+'"]').waitFor({state:'attached'});
  await page.getByLabel('本次更新基础版本',{exact:true}).selectOption(fixture.current_version_id);await page.getByRole('button',{name:'将准确文件更新加入请求',exact:true}).click();assert.equal(sentCount,0);
  checks.push({name:'Web requires an explicit current file and base version without automatic sending',passed:true});
  await page.getByLabel('本次消息内容',{exact:true}).fill('在准确文件范围内修改计划 '+randomUUID());
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);assert.equal(sent.status(),202);
  const input=sent.request().postDataJSON(),run=(await sent.json()).data;assert.deepEqual(input.file_update,{file_id:fixture.file_id,base_version_id:fixture.current_version_id,expected_file_version:fixture.file_version});assert.equal(input.mutation,undefined);assert.equal(run.file_update.base_version_id,fixture.current_version_id);
  checks.push({name:'One real Web request pins exact current file version and metadata revision',passed:true});
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(message=>message.run_id===run.id&&message.role==='user');assert.equal(saved.source_refs.length,1);assert.equal(saved.source_refs[0].type,'agent_file_update');assert.equal(saved.source_refs[0].id,run.id);assert.equal(saved.source_refs[0].sha256,run.file_update.intent_sha256);
  checks.push({name:'User message fixes the immutable file-update authorization hash',passed:true});
  await page.getByLabel('取消任务的原因',{exact:true}).fill('取消本地测试的新请求');const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);assert.equal(cancelled.status(),200);
  assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/files/'+fixture.file_id)).current_version_id,fixture.current_version_id);assert.equal(sentCount,1);
  checks.push({name:'Cancelling queued file update preserves the formal file and exact commit history',passed:true});
  assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){errors.push(String(error));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png')}).catch(()=>{});}
finally{for(const context of contexts)await context.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sourceFiles,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Web/native isolated API and controlled MCP file bytes; actual model, cloud and production remain unverified.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
