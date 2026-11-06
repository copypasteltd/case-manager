import assert from 'node:assert/strict';
import {mkdir,readFile,writeFile} from 'node:fs/promises';import {createHash} from 'node:crypto';import {fileURLToPath} from 'node:url';import path from 'node:path';
import {batchBrowser,waitOTP} from './batch_round_browser161_common.mjs';import {assertNoCredentials,recordViewport} from './batch_round161_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)),mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){
  await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
 }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}
 const group=process.env.CP_BROWSER_EVIDENCE_GROUP,media=path.resolve(app,'../docs/media/batch-round-161',group);await mkdir(media,{recursive:true});const shots=[];
 async function shot(name){await assertNoCredentials(page);const file=path.join(media,name+'.png');await page.screenshot({path:file,fullPage:!mobile,mask:[page.locator('input[type="password"],input[maxlength="6"],.cp-secret,[data-private-capability]')]});shots.push({name,file:path.relative(path.resolve(app,'..'),file).replaceAll('\\','/'),sha256:createHash('sha256').update(await readFile(file)).digest('hex')});}

 const recording=await recordViewport(page,app,group,async()=>{
  if(mobile){
   await page.goto(fixture.origin+'/#/pages/project/index?id='+fixture.project_id+'&section=task-form&task_id='+fixture.task_id);
   await page.getByText('任务参与人',{exact:true}).waitFor();await shot('mobile-task-participants');
   await page.getByRole('button',{name:/加入参与人：/}).last().click();await page.getByRole('button',{name:'确认保存任务',exact:true}).click();
   await page.getByText('任务已保存',{exact:true}).waitFor();await page.getByRole('button',{name:'查看已保存任务',exact:true}).click();await page.getByText('任务资料与关联',{exact:true}).waitFor();await page.getByText('任务资料与关联',{exact:true}).scrollIntoViewIfNeeded();await shot('mobile-task-current-receipt');checks.push('Shared mobile changes explicit task participants through the exact ordinary versioned command');
   await page.goto(fixture.origin+'/#/pages/files/index?file_id='+fixture.file_id);
   await page.locator('.taro_page_show').getByText('我的历史内容恢复',{exact:true}).last().waitFor();await page.locator('.taro_page_show').getByText('我的历史内容恢复',{exact:true}).last().scrollIntoViewIfNeeded();await page.locator('.taro_page_show').getByText('尚无本人恢复任务。请在准确历史版本上创建。',{exact:true}).last().waitFor();assert.equal(await page.locator('.taro_page_show').getByText('我的历史内容恢复',{exact:true}).count(),1,'Exactly one settled owner inventory');await shot('mobile-history-restoration');checks.push('Shared mobile reads owner-scoped historical copy/verification/commit inventory without changing an original');
  }else{
   await page.goto(fixture.origin+'/projects/'+fixture.project_id+'?task='+fixture.task_id);
   await page.getByRole('dialog').getByRole('button',{name:'编辑',exact:true}).click();
   const dialog=page.getByRole('dialog');await dialog.getByText('任务参与人（最多50人）',{exact:true}).waitFor();await shot('web-task-participants');
   await dialog.locator('input[type=checkbox]').first().check();
   await dialog.getByRole('button',{name:'继续读取可选关联',exact:true}).first().click();await dialog.getByText(/^已读取 26 项，已到列表末尾/).first().waitFor();await shot('web-complete-reference-pages');
   await dialog.getByRole('button',{name:'保存任务',exact:true}).click();await page.getByRole('dialog').getByText(/参与人：/).waitFor();await shot('web-task-current-receipt');checks.push('Web preserves exact participant intent and loads later reference pages before ordinary save');
  }
 });assert.ok(recording.frame_count>=5);await writeFile(path.join(evidence,'media.json'),JSON.stringify({synthetic_data:true,real_cloud:false,kind:'functional-wave',screenshots:shots,recording},null,2)+'\n');
},'batch-round-161');
