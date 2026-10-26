import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { createHash, createHmac, randomUUID } from 'node:crypto';
import { access, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const app=fileURLToPath(new URL('../',import.meta.url)),root=path.dirname(app.replace(/[\\/]$/,''));
const fixture=JSON.parse(await readFile(path.join(app,'.local/business-browser-fixture.json'),'utf8'));
assert.equal(fixture.agent_file_fixture,true);assert.equal(fixture.isolated_database,true);
const group=process.env.CP_BROWSER_EVIDENCE_GROUP??'secretary-files-web';assert.match(group,/^[a-zA-Z0-9-]+$/);
const evidence=path.join(root,'docs/quality/evidence',group),temporary=path.join(app,'.local/browser-temp');
await mkdir(evidence,{recursive:true});await mkdir(temporary,{recursive:true});process.env.TMP=temporary;process.env.TEMP=temporary;
async function collect(directory){const files=[];for(const entry of await readdir(path.join(app,directory),{withFileTypes:true})){if(['node_modules','dist','.taro','.swc'].includes(entry.name))continue;const file=path.posix.join(directory,entry.name);if(entry.isDirectory())files.push(...await collect(file));else files.push(file);}return files;}
const apiFiles=(await collect('backend')).filter(file=>/\.(go|sql)$|\/(go\.mod|go\.sum)$|\/internal\/contract\/openapi\.json$/.test(file));
const uiFiles=(await collect('web')).filter(file=>/\.(tsx?|json|css|html)$/.test(file));
const sourceFiles=[...new Set([...apiFiles,...uiFiles,'package.json','package-lock.json','.env.example','packages/contracts/src/api.d.ts','packages/contracts/src/mutation.ts','packages/contracts/package.json','packages/design/src/tokens.json','scripts/check_agent_file_text.mjs','scripts/run_agent_browser.py','scripts/test_backend.py','scripts/build_openapi.py'])].sort();
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
  const section=page.locator('section').filter({has:page.getByRole('heading',{name:'准备给秘书的资料',exact:true})}).first();
  await page.getByLabel('资料类型',{exact:true}).selectOption('file');await page.getByLabel('选择资料',{exact:true}).locator('option[value="'+fixture.file_id+'"]').waitFor({state:'attached'});await page.getByLabel('选择资料',{exact:true}).selectOption(fixture.file_id);
  await page.getByLabel('准确文件版本',{exact:true}).locator('option[value="'+fixture.historical_version_id+'"]').waitFor({state:'attached'});assert.equal(await page.getByRole('button',{name:'核对并预览',exact:true}).isDisabled(),true);assert.equal(sentCount,0);
  checks.push({name:'Web requires an explicit formal version and file selection does not read or send automatically',passed:true});
  await page.getByLabel('准确文件版本',{exact:true}).selectOption(fixture.historical_version_id);
  const [first]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),page.getByRole('button',{name:'核对并预览',exact:true}).click()]);assert.equal(first.status(),200);const view=(await first.json()).data;
  assert.equal(new URL(first.url()).searchParams.get('version_id'),fixture.historical_version_id);assert.equal(view.version_id,fixture.historical_version_id);assert.equal(view.evidence[0].sha256,fixture.content_sha256);assert.match(view.text,new RegExp(fixture.historical_prefix));assert.equal(view.text.includes(fixture.current_prefix),false);
  await section.getByRole('heading',{name:'秘书有界正文.md',exact:true}).waitFor();assert.match(await section.locator('pre').innerText(),new RegExp(fixture.historical_prefix));
  checks.push({name:'Web displays verified historical UTF-8 content while a newer head exists',passed:true});
  await section.getByRole('heading',{name:'秘书有界正文.md',exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:path.join(evidence,'fixed-file-text.png')});
  const [next]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),page.getByRole('button',{name:'核对下一段',exact:true}).click()]);assert.equal(next.status(),200);const continued=(await next.json()).data;assert.equal(continued.offset,view.next_offset);assert.equal(continued.version_id,view.version_id);assert.equal(continued.source_sha256,view.source_sha256);
  checks.push({name:'Next text segment stays bound to the selected version and source hash',passed:true});
  await page.getByRole('button',{name:'将这份准确资料加入本次请求',exact:true}).click();await page.getByLabel('准确文件版本',{exact:true}).selectOption(fixture.current_version_id);
  const [current]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname.includes('/context-sources/file/')),page.getByRole('button',{name:'核对并预览',exact:true}).click()]);assert.equal(current.status(),200);assert.match((await current.json()).data.text,new RegExp(fixture.current_prefix));
  await page.getByRole('button',{name:'将这份准确资料加入本次请求',exact:true}).click();assert.equal(sentCount,0);
  checks.push({name:'Two exact versions of one file remain distinct without automatic sending or write authority',passed:true});
  await page.getByLabel('本次秘书任务',{exact:true}).selectOption(fixture.profile_id);await page.getByLabel('本次消息内容',{exact:true}).fill('Compare only the two selected formal text versions '+randomUUID());
  const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;
  assert.equal(input.mutation,undefined);assert.deepEqual(input.context_refs.map(ref=>ref.version_id).sort(),[fixture.historical_version_id,fixture.current_version_id].sort());assert.equal(input.context_refs.every(ref=>ref.type==='file'&&ref.id===fixture.file_id),true);
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(message=>message.run_id===run.id&&message.role==='user');assert.deepEqual(saved.source_refs.map(ref=>ref.version_id).sort(),[fixture.historical_version_id,fixture.current_version_id].sort());
  checks.push({name:'One Web send persists both file-version citations in the actual run message',passed:true});
  await page.getByLabel('取消任务的原因',{exact:true}).fill('Stop isolated file comparison before model execution');const [cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);assert.equal(cancelled.status(),200);
  assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/files/'+fixture.file_id)).current_version_id,fixture.current_version_id);assert.equal(sentCount,1);
  checks.push({name:'Cancelling the queued comparison leaves the formal file head and history unchanged',passed:true});
  assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);passed=true;
}catch(error){errors.push(String(error));if(activePage)await activePage.screenshot({path:path.join(evidence,'failure.png')}).catch(()=>{});}
finally{for(const context of contexts)await context.close().catch(()=>{closed=false;});if(browser)await browser.close().catch(()=>{closed=false;});const after=await digest();passed=passed&&before===after&&closed;await writeFile(path.join(evidence,'result.json'),JSON.stringify({passed,started_at:started,finished_at:new Date().toISOString(),source_sha256:before,source_after_sha256:after,source_files:sourceFiles,checks,errors,server_errors:serverErrors,owned_contexts_closed:closed,owned_browser_closed:closed,scope:'Real Web/native isolated API and controlled MCP file bytes; actual model, cloud and production remain unverified.'},null,2)+'\n');}
console.log(JSON.stringify({passed,checks:checks.length,owned_browser_closed:closed}));process.exitCode=passed?0:1;
