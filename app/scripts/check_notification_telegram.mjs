import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_telegram_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './notification_telegram_recording.mjs';

const app=fileURLToPath(new URL('../',import.meta.url));
await notificationBrowser(false,async({page,context,fixture,checks,evidence})=>{
  await page.goto(fixture.origin);
  await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);
  await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);
  await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
  await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));
  await page.getByRole('button',{name:'验证并继续',exact:true}).click();
  await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  await page.goto(fixture.origin+'/notifications');
  const panel=page.getByRole('region',{name:'TG 私聊业务提醒',exact:true});await panel.getByRole('heading',{name:'TG 私聊业务提醒',exact:true}).waitFor();
  const shots=[],layouts=[],prefix='notification-telegram-web';
  async function get(route){const r=await context.request.get(fixture.origin+'/v1'+route);assert.equal(r.status(),200);return(await r.json()).data;}
  async function shot(suffix){await assertNoCredentials(page);const file='docs/media/screenshots/'+prefix+'-'+suffix+'.png';await mkdir(path.dirname(path.join(app,'..',file)),{recursive:true});const bytes=await page.screenshot({path:path.join(app,'..',file)});shots.push({file,sha256:createHash('sha256').update(bytes).digest('hex')});const layout=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth}));assert.ok(layout.scroll<=layout.width+1);layouts.push(layout);}
  const settings=await get('/notification-telegram-settings');assert.equal(settings.items.length,1);assert.equal(settings.items[0].effective,true);
  const receipts=await get('/notification-telegram-deliveries');const unknown=receipts.items.find(x=>x.state==='delivery_unknown');assert.ok(unknown);const source=await get('/notifications/'+unknown.thread_id);assert.equal(source.read,false);
  const recording=await recordViewport(page,app,prefix,async()=>{
    await panel.scrollIntoViewIfNeeded();await panel.getByRole('button',{name:'修改本人 TG 提醒',exact:true}).click();
    const policyForm=panel.getByRole('form',{name:'本人 TG 提醒设置',exact:true});await policyForm.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。').waitFor();await policyForm.scrollIntoViewIfNeeded();await shot('settings');await policyForm.getByRole('button',{name:'取消修改',exact:true}).click();
    await panel.getByRole('button',{name:'查看投递与处置记录',exact:true}).first().click();
    const detail=panel.getByLabel('TG 投递详情',{exact:true});await detail.getByRole('form',{name:'本人 TG 投递处置',exact:true}).waitFor();
    const form=detail.getByRole('form',{name:'本人 TG 投递处置',exact:true});await form.getByLabel('本次处置',{exact:true}).selectOption('confirm_sent');await form.getByLabel('本人私聊消息号',{exact:true}).fill('9004');await form.getByLabel('投递处置原因',{exact:true}).fill('合成演示：本人核对私聊消息，仅记录人工送达事实。');await form.getByRole('checkbox').check();await form.getByRole('button',{name:'确认投递处置',exact:true}).waitFor({state:'visible'});await form.scrollIntoViewIfNeeded();await shot('review');
    const [response]=await Promise.all([page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/v1/notification-telegram-deliveries/'+unknown.id+'/reconcile'),form.getByRole('button',{name:'确认投递处置',exact:true}).click()]);assert.equal(response.status(),200);const reconciled=(await response.json()).data;assert.equal(reconciled.state,'confirmed_sent');assert.equal(reconciled.version,unknown.version+1);assert.equal(reconciled.message_id,9004);
    await panel.getByRole('button',{name:'修改本人 TG 提醒',exact:true}).waitFor();await panel.getByRole('button',{name:'修改本人 TG 提醒',exact:true}).click();const stop=panel.getByRole('form',{name:'本人 TG 提醒设置',exact:true});await stop.getByLabel('私聊提醒状态',{exact:true}).selectOption('disabled');await stop.getByLabel('提醒设置变更原因',{exact:true}).fill('合成演示：员工明确关闭后续私聊提醒，保留站内通知与历史回执。');await stop.getByRole('checkbox',{name:/我确认只向本人私聊/}).check();
    const [published]=await Promise.all([page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/v1/notification-telegram-policies/'+fixture.bot_id),stop.getByRole('button',{name:'保存本人 TG 提醒',exact:true}).click()]);assert.equal(published.status(),201);assert.equal((await published.json()).data.enabled,false);
    await panel.getByText('Bot '+fixture.bot_id+' · 本人提醒已关闭',{exact:true}).waitFor();await panel.scrollIntoViewIfNeeded();await shot('closed');
  });
  const after=await get('/notification-telegram-deliveries/'+unknown.id);assert.equal(after.receipt.state,'confirmed_sent');assert.equal(after.events.at(-1).reason_code,'human_confirm_sent');assert.equal((await get('/notifications/'+unknown.thread_id)).read,false);assert.equal((await get('/notification-telegram-settings')).items[0].effective,false);
  checks.push('Real owner-bound Web settings review requires explicit types and recent MFA; no implicit opt-in.','Actual Web/native HTTP/PG manual confirmation preserves immutable delivery history without marking business read.','Actual employee disable publishes a new immutable preference and retains the in-app notice. No real Telegram API call or device is claimed.');
  await writeFile(path.join(evidence,'media.json'),JSON.stringify({recording,screenshots:shots,layouts,synthetic_data:true,scope:'Actual Web/native HTTP/PG employee review, human reconciliation and explicit disable. Provider outcomes are controlled synthetic fixtures; no real TG message.'},null,2)+'\n');
},'notification-telegram');
