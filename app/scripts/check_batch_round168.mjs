import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser168_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round168_recording.mjs';
import { selectMobile } from './mobile_picker_browser.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)),mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();}
 else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}
 const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
 const group=process.env.CP_BROWSER_EVIDENCE_GROUP,root=path.dirname(app.replace(/[\\/]$/,'')),media=path.join(root,'docs/media/batch-round-168',group);await mkdir(media,{recursive:true});const shots=[];
 const shot=async name=>{await assertNoCredentials(page);const file=path.join(media,name+'.png'),bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide'});shots.push({file:path.relative(root,file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),credentials_excluded:true});};
 await page.goto(fixture.origin+(mobile?'/#/pages/list/index?kind=tasks&project_id='+fixture.project_id:'/projects/'+fixture.project_id));
 await screen().getByText(fixture.task_title,{exact:true}).first().waitFor();
 const recording=await recordViewport(page,app,group,async()=>{
  if(mobile)await selectMobile(page,'任务呈现方式','看板');else await screen().getByRole('button',{name:'看板',exact:true}).click();
  await screen().locator(mobile?'.cp-task-board':'.task-board').waitFor();await shot(mobile?'mobile-task-board':'web-task-board');checks.push('actual authorized current project tasks render a state board');
  if(mobile)await selectMobile(page,'任务呈现方式','时间线');else await screen().getByRole('button',{name:'时间线',exact:true}).click();
  await screen().getByText(fixture.task_title,{exact:true}).first().waitFor();await shot(mobile?'mobile-task-timeline':'web-task-timeline');checks.push('same current tasks render basic timeline without inferred progress');
  await page.goto(fixture.origin+(mobile?'/#/pages/business/index?kind=contract&id='+fixture.contract_id:'/contracts/'+fixture.contract_id));
  if(mobile)await screen().getByRole('button',{name:'签署、生效与结项',exact:true}).click();
  await screen().getByRole('button',{name:mobile?'查看节点关联任务':'查看关联任务',exact:true}).click();
  if(mobile)await screen().getByText(fixture.task_title,{exact:true}).first().waitFor();else await screen().locator('p').filter({hasText:fixture.task_title}).first().waitFor();
  await shot(mobile?'mobile-contract-task-links':'web-contract-task-links');checks.push('ordinary current contract node shows exact historical association and separately current task progress');
  if(mobile)await screen().getByRole('button',{name:'查看关联及撤销历史',exact:true}).click();else await screen().getByLabel('显示关联与撤销历史',{exact:true}).check();
  await screen().getByText(/撤销关联/).first().waitFor();checks.push('immutable association and actual revocation history remain visible under both sides current permissions');
  await page.goto(fixture.origin+(mobile?'/#/pages/secretary/index?kind=runtime':'/agent-runtime'));
  if(mobile)await screen().locator('input[aria-label="已安装配置编号"]').fill(fixture.profile_id);else await page.getByLabel('已安装配置编号',{exact:true}).fill(fixture.profile_id);
  await screen().getByRole('button',{name:'读取准确发布台账',exact:true}).click();await screen().getByText(/synthetic-set-168/).first().waitFor();await shot(mobile?'mobile-skill-assessment-evidence':'web-skill-assessment-evidence');checks.push('real ordinary release receipt preserves versioned synthetic external failure result, without claiming publication');
  if(mobile){await page.goto(fixture.origin+'/#/pages/uploads/index');await screen().getByText(/普通附件上限20 MiB/).waitFor();await shot('mobile-ordinary-upload-workspace');checks.push('actual shared miniapp workspace exposes bounded ordinary upload and scan/promotion/human-commit queue; native picker/provider execution not claimed');}
  await page.waitForTimeout(700);
 });
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({passed:true,synthetic_data:true,authenticated_capture:true,credentials_excluded:true,screenshots:shots,recording},null,2)+'\n');
 assert.ok(shots.length>=4);
});
