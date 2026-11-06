import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser163_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round163_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)),mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){
  await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
 }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}

 const cid=fixture.conversation_id,rid=fixture.authorization_run_id;
 assert.match(cid,/^[0-9a-f-]{36}$/);assert.match(rid,/^[0-9a-f-]{36}$/);
 await page.goto(fixture.origin+(mobile?'/#/pages/secretary/index?kind=conversation&id='+cid+'&run='+rid:'/conversations/'+cid+'?run='+rid));
 const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
 const notice=()=>screen().getByText(/原授权登录已失效，后续文件处理已停止/).last();
 await notice().waitFor();await notice().scrollIntoViewIfNeeded();
 const group=process.env.CP_BROWSER_EVIDENCE_GROUP,media=path.join(path.dirname(app.replace(/[\\/]$/,'')),'docs/media/batch-round-163',group);
 await mkdir(media,{recursive:true});const shots=[];
 const shot=async(name)=>{await assertNoCredentials(page);const file=path.join(media,name+'.png');const bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide'});shots.push({file:path.relative(path.dirname(app.replace(/[\\/]$/,'')),file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),credentials_excluded:true});};
 const recording=await recordViewport(page,app,group,async()=>{
  await shot(mobile?'mobile-original-session-changed':'web-original-session-changed');
  const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+rid+'/event-log'&&r.request().method()==='GET'&&r.status()===200);
  await screen().getByRole('button',{name:'重新读取任务结果',exact:true}).click();
  const actual=await (await response).json();
  assert.equal(actual.data.run.id,rid);assert.equal((actual.data.run.file_create??actual.data.run.file_update).authorization_state,'changed');
  await notice().waitFor();await notice().scrollIntoViewIfNeeded();
  await shot(mobile?'mobile-history-read-receipt':'web-history-read-receipt');
  checks.push('New authenticated owner can read immutable file history after original login revocation; original authorization remains changed after another ordinary read, without creating another execution or formal file commit.');
 });
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({synthetic_data:true,real_cloud:false,kind:'functional-wave',screenshots:shots,recording},null,2)+'\n');
},'batch-round-163');
