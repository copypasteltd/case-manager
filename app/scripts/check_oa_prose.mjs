import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './oa_prose_browser_common.mjs';
import { mobileScreen, stableMobile } from './mobile_picker_browser.mjs';
import { selectConcernMobile } from './project_concern_picker.mjs';
import { assertNoCredentials, recordViewport } from './oa_prose_recording.mjs';

const mobile=process.argv.includes('--mobile'),app=fileURLToPath(new URL('../',import.meta.url));
await notificationBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
 const root=()=>mobile?mobileScreen(page):page;
 const button=name=>root().getByRole('button',{name,exact:true});
 const field=label=>mobile?root().locator('input[aria-label="'+label+'"],textarea[aria-label="'+label+'"]'):page.getByLabel(label,{exact:true});
 const select=(label,value,text)=>mobile?selectConcernMobile(page,label,text):field(label).selectOption(value);
 let bearer;page.on('request',request=>{if(request.headers().authorization?.startsWith('Bearer '))bearer=request.headers().authorization;});
 await page.goto(fixture.origin);await field('内部账号').fill(fixture.alice.login);await field('密码').fill(fixture.alice.password);
 await button(mobile?'登录工作空间 ↗':'登录工作空间').click();await field('六位验证码').fill(await waitOTP(fixture));await button(mobile?'验证并继续 ↗':'验证并继续').click();
 if(mobile)await root().locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();else await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 async function get(route){const response=await context.request.get((mobile?fixture.api_origin:fixture.origin)+'/v1'+route,mobile?{headers:{Authorization:bearer}}:{});assert.equal(response.status(),200);return(await response.json()).data;}
 async function openRun(id){
  await page.goto(mobile?fixture.origin+'#/pages/secretary/index?kind=conversation&id='+fixture.conversation_id+'&run='+id:fixture.origin+'/conversations/'+fixture.conversation_id+'?run='+id);
  await root().getByText('业务正文草稿',{exact:true}).waitFor();await root().getByText('逐字段核对原文与建议，确认后保存业务新修订。',{exact:true}).waitFor();await stableMobile(page);
 }
 const original=await get('/runs/'+fixture.contract_run_id);assert.equal(original.oa_request.status,'drafted');assert.equal(original.outcome,'draft');assert.ok(original.oa_request.text_draft.source_refs.length);
 await openRun(fixture.contract_run_id);
 const layouts=[];
 for(const width of [390,768,1440]){
  await page.setViewportSize({width,height:1100});await stableMobile(page);
  const measured=await page.evaluate(()=>({viewport:document.documentElement.clientWidth,width:document.documentElement.scrollWidth}));assert.ok(measured.width<=width+2);layouts.push({width,...measured});
 }
 await page.setViewportSize(mobile?{width:390,height:1000}:{width:1440,height:1100});await root().getByText('业务正文草稿',{exact:true}).scrollIntoViewIfNeeded();
 await assertNoCredentials(page);const prefix='secretary-oa-prose-'+(mobile?'mobile':'web');
 await page.screenshot({path:path.join(app,'../docs/media/screenshots',prefix+'-before.png'),fullPage:false});
 checks.push('Authenticated original and proposed contract prose are visible at390/768/1440 widths; the model result is a draft with bound source snapshots.');
 const adoptions=[];
 async function adopt(kind){
  const id=fixture[kind+'_run_id'],before=await get('/runs/'+id),v=before.oa_request;
  assert.equal(await button('采用正文草稿').isDisabled(),true);
  await field('采用正文的依据').fill('本人已逐字段核对原文和正文建议');assert.equal(await button('采用正文草稿').isDisabled(),true);
  if(mobile)await root().getByText('我已核对全部正文建议，确认采用',{exact:true}).click();else await page.getByLabel('我已核对全部正文建议，确认采用',{exact:true}).check();
  const [response]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+id+'/oa-text-adoption'&&r.request().method()==='POST'),button('采用正文草稿').click()]);
  assert.equal(response.status(),200);const adopted=(await response.json()).data,d=adopted.text_draft;
  assert.equal(adopted.status,'adopted');assert.equal(d.draft_sha256,v.text_draft.draft_sha256);assert.ok(d.operation_id);assert.equal(d.result.version,v.before.business.version+1);
  assert.deepEqual(d.result.payload.content,{...v.before.business.payload.content,...v.text_draft.fields});
  await root().getByText('正文已采用并保存为业务新修订。',{exact:true}).waitFor();await stableMobile(page);
  const saved=await get('/runs/'+id);assert.equal(saved.outcome,'draft');assert.equal(saved.oa_request.status,'adopted');assert.equal(saved.oa_request.result,null);
  adoptions.push({kind,fields:Object.keys(d.fields),new_business_version:d.result.version,operation_confirmed:true,approval_submitted:false});
 }
 const recordings=[await recordViewport(page,app,'oa-prose-'+(mobile?'mobile':'web'),async()=>{
  await adopt('contract');await page.waitForTimeout(600);await openRun(fixture.purchase_run_id);await adopt('purchase');
  await root().getByText('业务正文草稿',{exact:true}).scrollIntoViewIfNeeded();await page.waitForTimeout(600);
 })];
 await page.screenshot({path:path.join(app,'../docs/media/screenshots',prefix+'-after.png'),fullPage:false});
 checks.push('Contract and purchase adoption require actual human review, call the authenticated native API and save exactly one new business revision with all commercial facts preserved.');
 await button('收起任务进度').click();
 await select('本次秘书任务',fixture.profile_id,'协助业务申请 · controlled-test');await button('选择业务申请操作').click();await select('申请业务类型','claim','报销');
 const claim=await get('/claims/'+fixture.claim_business_id);
 if(mobile){await button('选择申请业务资料').click();await root().locator('.cp-row-title').getByText(claim.title,{exact:true}).click();}else await field('申请业务资料').selectOption(claim.id);
 await select('申请操作','text','起草业务正文');
 if(mobile)await root().getByText('用途说明',{exact:true}).click();else await page.getByLabel('用途说明',{exact:true}).check();
 await field('申请操作说明').fill('仅起草报销用途和备注，保留原费用明细');await button('将准确申请操作加入请求').click();await field('本次消息内容').fill('请依据我选择的准确报销资料整理用途和备注正文。');
 const [sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button('确认发送本次请求').click()]);
 assert.equal(sent.status(),202);const payload=sent.request().postDataJSON(),queued=(await sent.json()).data;
 assert.equal(payload.oa_request.tool,'draft_business_text');assert.deepEqual(payload.oa_request.arguments.fields,['notes','purpose']);assert.equal(payload.oa_request.arguments.business_id,claim.id);assert.equal(queued.oa_request.status,'authorized');assert.equal(queued.oa_request.text_draft,undefined);
 checks.push('The real claim selector freezes only notes/purpose and the current business revision; one actual send creates an authorized queued run without fabricating model execution.');
 await writeFile(path.join(evidence,'capture.json'),JSON.stringify({mobile,recordings,layouts,adoptions,synthetic_data:true,scope:'Authenticated native API and Web/Taro H5 prose drafting/adoption; controlled tool fixture, no external model, device, cloud or production acceptance.'},null,2)+'\n');
},'oa-prose');
