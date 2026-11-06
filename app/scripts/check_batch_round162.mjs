import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser162_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round162_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)),mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){
  await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
 }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}
 await page.goto(fixture.origin+(mobile?'/#/pages/telegram/index?section=grants':'/telegram?view=grants'));
 const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
 await screen().getByText('当前有权查阅的本人预授权',{exact:true}).waitFor();
 await screen().getByText('新建任务',{exact:true}).waitFor();
 await screen().getByText('新建任务',{exact:true}).scrollIntoViewIfNeeded();
 const group=process.env.CP_BROWSER_EVIDENCE_GROUP,media=path.join(path.dirname(app.replace(/[\\/]$/,'')),'docs/media/batch-round-162',group);await mkdir(media,{recursive:true});const shots=[];
 const shot=async(name)=>{await assertNoCredentials(page);const file=path.join(media,name+'.png');const bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide',mask:[page.locator('.cp-grant-trigger')]});shots.push({file:path.relative(path.dirname(app.replace(/[\\/]$/,'')),file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),fixed_trigger_commands_masked:true});};
 const recording=await recordViewport(page,app,group,async()=>{
  await shot(mobile?'mobile-owner-grant-inventory':'web-owner-grant-inventory');
  const field=mobile?screen().locator('input[aria-label="取消这份预授权的原因"]'):screen().getByLabel('取消这份预授权的原因',{exact:true});
  await field.fill('本轮演示：本人明确取消尚未触发的准确授权');
  if(mobile)await screen().getByText('已核对本份授权，申请停止尚未完成的执行。',{exact:true}).click();else await screen().getByLabel('已核对本份授权，申请停止尚未完成的执行。',{exact:true}).check();
  await shot(mobile?'mobile-grant-cancel-review':'web-grant-cancel-review');
  await screen().getByRole('button',{name:'取消此预授权执行',exact:true}).click();
  await field.waitFor({state:'detached'});
  await screen().getByText('新建任务',{exact:true}).waitFor();
  assert.equal(await screen().getByRole('button',{name:'取消此预授权执行',exact:true}).count(),0);
  await shot(mobile?'mobile-grant-cancelled-receipt':'web-grant-cancelled-receipt');
  checks.push('Current owner inventory reads never trigger work; explicit ordinary cancellation persists and removes the untriggered execution action.');
 });
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({synthetic_data:true,real_cloud:false,kind:'functional-wave',screenshots:shots,recording},null,2)+'\n');
},'batch-round-162');
