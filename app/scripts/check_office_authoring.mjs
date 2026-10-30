import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { workbenchBrowser, waitOTP } from './workbench_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './project_business_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)),mobile=process.argv.includes('--mobile');
await workbenchBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
 const root=()=>mobile?mobileScreen(page):page;
 const field=label=>mobile?root().locator('input[aria-label="'+label+'"],textarea[aria-label="'+label+'"]'):page.getByLabel(label,{exact:true});
 const button=name=>root().getByRole('button',{name,exact:true});
 let bearer;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))bearer=r.headers().authorization;});
 await page.goto(fixture.origin);await field('内部账号').fill(fixture.alice.login);await field('密码').fill(fixture.alice.password);await button(mobile?'登录工作空间 ↗':'登录工作空间').click();await field('六位验证码').fill(await waitOTP(fixture));await button(mobile?'验证并继续 ↗':'验证并继续').click();
 if(mobile)await root().locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();else await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 const get=async route=>{const r=await context.request.get((mobile?fixture.api_origin:fixture.origin)+'/v1'+route,{headers:mobile?{Authorization:bearer}:{}});assert.equal(r.status(),200);return(await r.json()).data;};
 const navigate=route=>page.goto(fixture.origin+(mobile?'/#':'')+route);
 const screenshots=[],prefix='office-authoring-'+(mobile?'mobile':'web');await mkdir(path.join(app,'../docs/media/screenshots'),{recursive:true});
 async function shot(label){await assertNoCredentials(page);const file='docs/media/screenshots/'+prefix+'-'+label+'.png';await page.screenshot({path:path.join(app,'..',file)});screenshots.push({file,sha256:createHash('sha256').update(await readFile(path.join(app,'..',file))).digest('hex')});}
 let recording;
 for(let i=0;i<3;i++){
  const file=await get('/files/'+fixture.office_file_ids[i]),history=await get('/files/'+file.id+'/versions?limit=20');
  assert.equal(history.items.length,2);
  await navigate(mobile?'/pages/files/index?file_id='+file.id:'/files/'+file.id);
  if(mobile){
   await button('查阅 Office 内容片段').click();await root().getByText('正式修订 香港🙂',{exact:true}).waitFor();
   assert.equal(await button('生成并核对候选文件').count(),0);
   await root().getByText('正式修订 香港🙂',{exact:true}).scrollIntoViewIfNeeded();
   if(i===0)recording=await recordViewport(page,app,prefix,async()=>{await button('重新读取依据并清空修改').click();await root().getByText('正式修订 香港🙂',{exact:true}).waitFor();await root().getByText('正式修订 香港🙂',{exact:true}).scrollIntoViewIfNeeded();});
   await shot(['word','excel','slides'][i]);
  }else{
   await button('编辑 Office 内容').click();await button('读取可编辑内容').click();
   const dialog=page.getByRole('dialog');await dialog.locator('textarea').first().waitFor();
   await dialog.locator('textarea').first().fill('浏览器人工修订 '+['Word','Excel','PPT'][i]+' 香港🙂');
   const generate=async()=>{const pending=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/files/'+file.id+'/office-render'&&r.request().method()==='POST');await button('生成并核对候选文件').click();const response=await pending;assert.equal(response.status(),200);const a=(await response.json()).data;assert.equal(a.upload.file_id,file.id);assert.equal(a.upload.base_version_id,file.current_version_id);assert.equal(a.sha256,createHash('sha256').update(Buffer.from(a.content_base64,'base64')).digest('hex'));await dialog.getByRole('heading',{name:file.name,exact:true}).waitFor();};
   if(i===0)recording=await recordViewport(page,app,prefix,generate);else await generate();
   await shot(['word','excel','slides'][i]);await dialog.getByRole('button',{name:'关闭弹窗',exact:true}).click();
   const after=await get('/files/'+file.id);assert.equal(after.current_version_id,file.current_version_id);assert.equal(after.version,file.version);
  }
  checks.push((mobile?'Shared Taro H5 readonly ':'Web exact-source edit/render ')+['Word','Excel','PPT'][i]+': real API and retained two-version history, no formal publication from rendering.');
 }
 if(!mobile){
  await navigate('/files/folders/'+fixture.folder_id);await button('新建 Office 文件').click();await field('文件名称').fill('浏览器新建说明');await field('文档标题').fill('新建说明');await field('文档内容').fill('公司资料\n人工核对后提交');
  const pending=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/folders/'+fixture.folder_id+'/office-render');await button('生成并核对候选文件').click();assert.equal((await pending).status(),200);await page.getByRole('dialog').getByRole('heading',{name:'浏览器新建说明.docx',exact:true}).waitFor();checks.push('Web folder creation form generates a real DOCX candidate through the current API; final publication remains the separate upload workflow.');
 }
 assert.ok(recording&&screenshots.length===3);await writeFile(path.join(evidence,'media.json'),JSON.stringify({screenshots,recording,scope:'Actual synthetic Web edit/render and shared H5 readonly pixels; HTTP tests separately exercise full scan/hash/promotion/commit. No device/cloud/Office layout claim.'},null,2)+'\n');
},'office-authoring');
