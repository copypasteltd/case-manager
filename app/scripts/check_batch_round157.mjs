import assert from 'node:assert/strict';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {batchBrowser,waitOTP} from './batch_round_browser_common.mjs';
import {assertNoCredentials,recordViewport} from './batch_round_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url));
await batchBrowser(false,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);
 await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);
 await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
 await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));
 await page.getByRole('button',{name:'验证并继续',exact:true}).click();
 await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 const kind=fixture.fixture_kind.endsWith('office')?'office':'telegram',group=process.env.CP_BROWSER_EVIDENCE_GROUP;
 const media=path.resolve(app,'../docs/media/batch-round-157',group);await mkdir(media,{recursive:true});
 const shots=[];
 async function snapshot(name){await assertNoCredentials(page);const file=name+'.png',target=path.join(media,file);await page.screenshot({path:target,fullPage:true,mask:[page.locator('input[type="password"],input[maxlength="6"],.cp-secret,[data-private-capability]')]});shots.push({name,file:path.relative(path.resolve(app,'..'),target).replaceAll('\\','/'),sha256:createHash('sha256').update(await readFile(target)).digest('hex')});}
 let recording;
 if(kind==='office'){
  await page.goto(fixture.origin+'/agent-runtime');
  await page.getByRole('heading',{name:'Skill评估与发布决定',exact:true}).waitFor();
  await page.getByLabel('已安装配置编号',{exact:true}).fill(fixture.profile_id);
  await page.getByRole('button',{name:'读取准确发布台账',exact:true}).click();
  await page.getByText('暂无人工发布决定；配置安装与真实验收仍分别记录。',{exact:true}).waitFor();
  recording=await recordViewport(page,app,group,async()=>{
   await page.getByLabel('决定原因',{exact:true}).fill('合成演示：核对准确配置并登记本轮试运行');
   await page.getByLabel('已核对准确代码、本人评估结果及当前任务影响',{exact:true}).check();
   await page.getByRole('button',{name:'记录此发布决定',exact:true}).click();
   await page.getByRole('heading',{name:'第 1 版 · 试运行',exact:true}).waitFor();
  });
  checks.push('Real native HTTP/PG pilot decision through current Web UI; no production publication claim');await snapshot('skill-governance');
  await page.goto(fixture.origin+'/files/'+fixture.file_id+'?version='+fixture.version_id);
  await page.getByText('原有资料.docx',{exact:true}).first().waitFor();await snapshot('office-exact-version');
  await page.goto(fixture.origin+'/conversations/'+fixture.conversation_id+'?run='+fixture.stopped_run_id);
  await page.getByLabel('本次秘书任务',{exact:true}).selectOption(fixture.profile_id);
  await page.getByRole('button',{name:'选择旧草稿继续处理',exact:true}).click();
  await page.getByLabel('选择原文件请求',{exact:true}).selectOption(fixture.stopped_run_id);
  await page.getByLabel('本次恢复的准确草稿',{exact:true}).selectOption({index:1});
  await page.getByText('此准确草稿已被本人放弃恢复。',{exact:false}).first().waitFor();await snapshot('recovery-barrier');
  checks.push('Actual second Office version and permanent stopped-draft recovery barrier are readable');
 }else{
  await page.goto(fixture.origin+'/telegram');
  await page.getByRole('heading',{name:'审核TG人工授权任务的群内结果',exact:true}).waitFor();
  recording=await recordViewport(page,app,group,async()=>{
   await page.getByLabel('本人已授权TG请求编号',{exact:true}).fill(fixture.request_id);
   await page.getByRole('button',{name:'读取准确已完成结果',exact:true}).click();
   await page.getByText('完整结果',{exact:false}).first().waitFor();
  });
  await snapshot('telegram-reviewed-result');
  checks.push('Owner reads exact completed portal-handoff result and separate original-group disclosure controls');
 }
 assert.ok(recording.frame_count>=5);
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({synthetic_data:true,controlled_model:true,real_cloud:false,kind,screenshots:shots,recording},null,2)+'\n');
},'batch-round-157');
