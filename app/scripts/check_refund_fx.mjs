// Actual human UI and native Go/PG HTTP; isolated synthetic statements only.
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { refund_fxBrowser, otp } from './refund_fx_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './refund_fx_recording.mjs';

const mobile=process.argv.includes('--mobile'),app=fileURLToPath(new URL('../',import.meta.url));
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
await refund_fxBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
  const root=()=>mobile?mobileScreen(page):page,button=name=>root().getByRole('button',{name,exact:true}).last();
  const field=name=>mobile?root().locator('input[aria-label="'+name+'"],textarea[aria-label="'+name+'"]'):root().getByLabel(name,{exact:true});
  let bearer,lastStep;
  page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))bearer=r.headers().authorization;});
  const origin=mobile?fixture.api_origin:fixture.origin;
  async function api(route,body,status=200,key=randomUUID()){
    const headers=mobile?{Authorization:bearer}:{Origin:fixture.origin,'X-CSRF-Token':(await context.cookies()).find(c=>c.name==='cp_csrf')?.value??''};
    const r=body?await context.request.post(origin+'/v1'+route,{headers:{...headers,'Idempotency-Key':key},data:body}):await context.request.get(origin+'/v1'+route,{headers});
    const v=await r.json();assert.equal(r.status(),status,route+': '+(v.error?.code??''));return v.data;
  }
  async function verifyMobile(){
    const step=Math.floor(Date.now()/30000);if(step<=lastStep)await page.waitForTimeout((lastStep+1)*30000-Date.now()+250);
    lastStep=Math.floor(Date.now()/30000);await field('验证器六位验证码').fill(otp(fixture.alice.mfa_secret,lastStep));await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。',{exact:true}).waitFor();
  }
  await page.goto(fixture.origin);await field('内部账号').fill(fixture.alice.login);await field('密码').fill(fixture.alice.password);await button(mobile?'登录工作空间 ↗':'登录工作空间').click();
  if(Math.floor(Date.now()/30000)<=fixture.alice.last_step)await page.waitForTimeout((fixture.alice.last_step+1)*30000-Date.now()+250);
  lastStep=Math.floor(Date.now()/30000);await field('六位验证码').fill(otp(fixture.alice.mfa_secret,lastStep));await button(mobile?'验证并继续 ↗':'验证并继续').click();await root().getByText('业务待办',{exact:true}).first().waitFor();
  checks.push('Human login and MFA completed before any capture.');
  const id=fixture.refund.id,url=fixture.origin+(mobile?'/#/pages/finance/index?kind=payment&id=':'/payments/')+id;
  const p=await api('/payments/'+fixture.payment.id);assert.equal(p.unallocated_amount,'780.000000');
  await page.goto(mobile?url+'&action=edit':url);
  if(!mobile)await button('修订资金草稿').click();
  const rate=mobile?'实际汇率（每1原付款币兑换实收币）':'实际汇率（每1 HKD兑换 USD）';
  await field(rate).fill('0.12');await field(mobile?'登记或修订原因':'登记或修订原因').fill('核对异币退款实际银行凭证');await button(mobile?'保存财务草稿':'保存资金草稿').click();
  await root().getByText(mobile?'请核对异币退款原本金、实收金额、币种精度、实际汇率方向与凭证来源。':/原付款本金乘实际汇率.*实收/).first().waitFor();
  checks.push('Actual draft UI rejects a rate that does not reproduce actual received cash.');
  await field(rate).fill('0.1282051282');await field('退款汇率凭证来源').fill('合成银行退款凭证121');
  const [draftResponse]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/payments/'+id&&r.request().method()==='PATCH'),button(mobile?'保存财务草稿':'保存资金草稿').click()]);
  assert.equal(draftResponse.status(),200);const draft=(await draftResponse.json()).data;assert.equal(draft.refund_fx.source,'合成银行退款凭证121');assert.equal(draft.refund_fx.original_currency,'HKD');assert.equal(draft.currency,'USD');
  if(mobile)await page.waitForURL(u=>u.hash===new URL(url).hash);else await page.goto(url);await root().getByText('异币退款实收依据',{exact:true}).first().waitFor();
  checks.push('Actual editor persists distinct HKD original principal and USD refund cash with pinned statement provenance.');
  const prefix='refund-fx-'+(mobile?'mobile':'web'),directory=path.join(app,'../docs/media/screenshots'),shots=[];await mkdir(directory,{recursive:true});
  async function shot(suffix){await assertNoCredentials(page);const file=prefix+'-'+suffix+'.png';await page.screenshot({path:path.join(directory,file),fullPage:false});shots.push({file:'docs/media/screenshots/'+file,sha256:hash(await readFile(path.join(directory,file)))});}
  await root().getByText('异币退款实收依据',{exact:true}).first().scrollIntoViewIfNeeded();await shot('draft');
  await button('登记实际资金').click();
  if(mobile)await button('读取原付款与退款分配').click();
  await field(mobile?'财务操作依据':'操作依据').fill('人工核实实收100 USD及退回780 HKD本金');
  assert.equal(await button('登记实际资金').isDisabled(),true);
  if(mobile){await verifyMobile();await selectMobile(page,'人工核实声明','已核对原事实、凭证、双币金额、汇率方向与所选依据');}
  else{await root().getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。',{exact:true}).waitFor();await root().getByLabel('我已核对两种币金额、汇率方向、凭证来源与本次回转金额',{exact:true}).check();}
  const input={expected_version:draft.version,revision_id:draft.current_revision_id,content_hash:draft.content_hash,reason:'人工核实实收100 USD及退回780 HKD本金',expected_source_version:p.version,refund_allocations:[],refund_fx_acknowledged:true};
  await api('/payments/'+id+'/record',{...input,refund_fx_acknowledged:false},422);
  checks.push('Fresh human confirmation is required independently of the saved quote.');
  const [savedResponse]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/payments/'+id+'/record'&&r.request().method()==='POST'),button('登记实际资金').click()]);
  assert.equal(savedResponse.status(),200);assert.deepEqual(savedResponse.request().postDataJSON(),input);const saved=(await savedResponse.json()).data;
  assert.equal(saved.can_allocate,false);assert.equal(saved.amount,'100.000000');assert.equal(saved.refund_fx.original_amount,'780.000000');
  const replay=await api('/payments/'+id+'/record',input,200,savedResponse.request().headers()['idempotency-key']);assert.equal(replay.version,saved.version);
  const after=await api('/payments/'+p.id);assert.equal(after.refunded_amount,'780.000000');assert.equal(after.unallocated_amount,'0.000000');assert.equal(after.content_hash,p.content_hash);
  if(mobile)await page.waitForURL(u=>u.hash===new URL(url).hash);else await page.goto(url);await root().getByText('异币退款实收依据',{exact:true}).first().scrollIntoViewIfNeeded();await shot('recorded');
  assert.equal(await button(mobile?'关联退款分配':'关联退款分配').count(),0);
  checks.push('Actual UI confirmation and exact HTTP replay conserve USD cash and HKD principal without creating an expense allocation.');
  const recording=await recordViewport(page,app,prefix,async()=>{await page.waitForTimeout(800);await page.mouse.wheel(0,260);await page.waitForTimeout(800);await page.mouse.wheel(0,-180);await page.waitForTimeout(800);});
  await button(mobile?'全额冲销原记录':'全额冲销资金').click();await field('冲销生效时间（香港）').fill(mobile?'2025-02-03T12:00:00':'2025-02-03T12:00');await field(mobile?'财务操作依据':'操作依据').fill('纠正合成实收退款并保留原始凭证');
  if(mobile){await verifyMobile();await selectMobile(page,'人工核实声明','已核对原事实、凭证、双币金额、汇率方向与所选依据');}
  const [reversedResponse]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/payments/'+id+'/reverse'&&r.request().method()==='POST'),button(mobile?'全额冲销原记录':'全额冲销资金').click()]);assert.equal(reversedResponse.status(),200);
  if(mobile)await page.waitForURL(u=>u.hash===new URL(url).hash);else await page.goto(url);await root().getByText('异币退款实收依据',{exact:true}).first().scrollIntoViewIfNeeded();await shot('reversed');
  const restored=await api('/payments/'+p.id);assert.equal(restored.refunded_amount,'0.000000');assert.equal(restored.unallocated_amount,'780.000000');assert.equal((await api('/payments/'+id)).content_hash,draft.content_hash);
  checks.push('Actual human reversal preserves the original quote and restores exactly 780 HKD principal.');
  await writeFile(path.join(evidence,'media.json'),JSON.stringify({screenshots:shots,recording,synthetic_data:true,real_device:false,real_model:false,api_responses_mocked:false,fixture_reused_from:'dedicated TestActualForeignRefunds with native PG'},null,2)+'\n');
});
