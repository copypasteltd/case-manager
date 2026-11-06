import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser165_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round165_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)), mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 if(mobile){
  await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
 }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}
 const sid=fixture.subscription_id, child=fixture.supplement_id, parent=fixture.parent_contract_id;
 for(const id of [sid,child,parent]) assert.match(id,/^[0-9a-f-]{36}$/);
 const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
 const history=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/subscriptions/'+sid+'/reminders'&&r.request().method()==='GET'&&r.status()===200);
 await page.goto(fixture.origin+(mobile?'/#/pages/finance/index?kind=subscription&id='+sid:'/subscriptions/'+sid));
 const body=await (await history).json(), rows=body.data.items;
 assert.equal(rows.length,4);assert.deepEqual(rows.map(n=>n.period_version).sort(),[1,2,3,5]);
 assert.equal(rows.find(n=>n.period_version===1).days_before,7);assert.equal(rows.find(n=>n.period_version===5).delivery_state,'delivered');
 const heading=screen().getByText('提醒与投递记录',{exact:true});await heading.waitFor();await heading.scrollIntoViewIfNeeded();
 const group=process.env.CP_BROWSER_EVIDENCE_GROUP,root=path.dirname(app.replace(/[\\/]$/,'')),media=path.join(root,'docs/media/batch-round-165',group);
 await mkdir(media,{recursive:true});const shots=[];
 const shot=async name=>{await assertNoCredentials(page);const file=path.join(media,name+'.png'),bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide'});shots.push({file:path.relative(root,file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),credentials_excluded:true});};
 const recording=await recordViewport(page,app,group,async()=>{
  await shot(mobile?'mobile-reminder-versions':'web-reminder-versions');if(mobile){await screen().getByRole('button',{name:/账期 · 版本 1$/}).scrollIntoViewIfNeeded();await shot('mobile-pinned-old-reminder');}checks.push('actual API immutable reminder versions and fixed old days are visible');
  if(mobile){await page.goto(fixture.origin+'/#/pages/business/index?kind=contract&id='+child);}else{await page.evaluate(route=>{window.history.pushState({},'',route);window.dispatchEvent(new PopStateEvent('popstate'));},'/contracts/'+child);}
  const link=mobile?screen().getByRole('button',{name:'查看主合同',exact:true}):screen().getByRole('link',{name:/主合同/});
  await link.waitFor();await link.scrollIntoViewIfNeeded();await shot(mobile?'mobile-supplement-parent-entry':'web-supplement-parent-entry');
  const read=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/contracts/'+parent&&r.request().method()==='GET'&&r.status()===200);
  await link.click();const result=await (await read).json();assert.equal(result.data.id,parent);
  await screen().getByText('合成主合同 · 服务协议',{exact:true}).first().waitFor();
  await shot(mobile?'mobile-parent-contract':'web-parent-contract');checks.push('ordinary exact parent navigation reads current authorized contract');
 });
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({passed:true,synthetic_data:true,authenticated_capture:true,credentials_excluded:true,screenshots:shots,recording},null,2)+'\n');
});
