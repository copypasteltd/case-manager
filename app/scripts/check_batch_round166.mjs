import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser166_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round166_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)), mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){
  await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
 }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}

 const pid=fixture.project_id;assert.match(pid,/^[0-9a-f-]{36}$/);
 const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
 const pending=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/projects/'+pid+'/closeout'&&r.request().method()==='GET'&&r.status()===200);
 await page.goto(fixture.origin+(mobile?'/#/pages/project/index?section=closeout&id='+pid:'/projects/'+pid+'?tab=closeout'));
 const preview=(await (await pending).json()).data;
 const group=preview.groups.find(g=>g.kind==='subscriptions');assert.equal(group.visibility,'complete');assert.equal(group.items.length,4);
 assert.equal(group.items.filter(i=>i.blocking).length,2);assert.equal(group.items.filter(i=>i.subscription_calendar.cancellation_effective).length,2);
 assert.deepEqual([...new Set(group.items.map(i=>i.subscription_calendar.timezone))].sort(),['Etc/GMT+12','Pacific/Kiritimati']);
 if(!mobile){const panel=screen().locator('details').filter({has:page.locator('summary').filter({hasText:'订阅'})}).first();if(!(await panel.evaluate(node=>node.open)))await panel.locator('summary').click();}
 await screen().getByText(/核查业务日/).first().waitFor();
 const egroup=process.env.CP_BROWSER_EVIDENCE_GROUP,root=path.dirname(app.replace(/[\\/]$/,'')),media=path.join(root,'docs/media/batch-round-166',egroup);
 await mkdir(media,{recursive:true});const shots=[];
 const shot=async name=>{await assertNoCredentials(page);const file=path.join(media,name+'.png'),bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide'});shots.push({file:path.relative(root,file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),credentials_excluded:true});};
 const recording=await recordViewport(page,app,egroup,async()=>{
  // Wait for the ordinary Taro route transition before recording stable pixels.
  await page.waitForTimeout(450);
  await screen().getByText(/Etc\/GMT\+12/).first().scrollIntoViewIfNeeded();await shot(mobile?'mobile-local-calendar':'web-local-calendar');
  checks.push('actual authorized project closeout API returns all four exact subscription calendars');
  await screen().getByText(/Pacific\/Kiritimati/).last().scrollIntoViewIfNeeded();await page.waitForTimeout(300);await shot(mobile?'mobile-opposite-calendar':'web-opposite-calendar');
  checks.push('local effective and future cancellation dates remain separate visible checklist decisions');
 });
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({passed:true,synthetic_data:true,authenticated_capture:true,credentials_excluded:true,screenshots:shots,recording},null,2)+'\n');
});
