// Actual synthetic HTTP, leased worker and browser interactions; no response substitution.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './notification_retry_browser_common.mjs';
import { mobileScreen } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './notification_retry_recording.mjs';

const mobile=process.argv.includes('--mobile'),app=fileURLToPath(new URL('../',import.meta.url));
const sha=b=>createHash('sha256').update(b).digest('hex');
await notificationBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
  const root=()=>mobile?mobileScreen(page):page;
  const button=name=>root().getByRole('button',{name,exact:true});
  const field=name=>mobile?root().locator('input[aria-label="'+name+'"]'):page.getByLabel(name,{exact:true});
  const actor=fixture.alice;assert.ok(fixture.notification_retry_fixture&&fixture.isolated_database);
  let bearer;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))bearer=r.headers().authorization;});
  await page.goto(fixture.origin);await field('内部账号').fill(actor.login);await field('密码').fill(actor.password);
  await button(mobile?'登录工作空间 ↗':'登录工作空间').click();await field('六位验证码').fill(await waitOTP({alice:actor}));
  await button(mobile?'验证并继续 ↗':'验证并继续').click();
  if(mobile)await root().locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();else await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  const lastStep=Math.floor(Date.now()/30000),origin=mobile?fixture.api_origin:fixture.origin;
  async function headers(){return mobile?{Authorization:bearer}:{Origin:fixture.origin,'X-CSRF-Token':(await context.cookies()).find(c=>c.name==='cp_csrf')?.value??''};}
  async function get(route){const r=await context.request.get(origin+'/v1'+route,{headers:await headers()});assert.equal(r.status(),200,route);return(await r.json()).data;}
  await page.goto(fixture.origin+(mobile?'/#/pages/notifications/index':'/notifications'));
  await root().getByText('投递异常与重试',{exact:true}).waitFor();
  const me=await get('/me');assert.ok(me.actions.includes('notification.delivery.read')&&me.actions.includes('notification.delivery.retry'));
  const source=await get('/notification-delivery-jobs/'+fixture.failed_job_id);assert.equal(source.state,'dead');assert.equal(source.can_retry,true);
  const card=mobile?root().locator('.cp-paragraph').filter({hasText:source.job_id}):root().locator('article').filter({has:root().getByText(source.job_id,{exact:true})}).last();
  await card.waitFor();assert.equal(await card.count(),1);
  await card.getByRole('button',{name:'核对任务与重试',exact:true}).click();
  await root().getByText('所选投递任务',{exact:true}).waitFor();await button('确认创建重试').waitFor();
  assert.equal(await button('确认创建重试').isEnabled(),false);
  checks.push('An explicitly granted human sees technical failures and a fixed revision; a retry is not submitted merely by opening the record.');
  if(mobile){
    const code=await waitOTP({alice:{...actor,last_step:lastStep}});
    await field('验证器六位验证码').fill(code);await button('确认本人操作').click();
    await root().getByText('已通过本次两步验证，请继续核对并确认操作。',{exact:true}).waitFor();
  }
  const screenshots=path.join(app,'../docs/media/screenshots');await mkdir(screenshots,{recursive:true});
  const prefix='notification-retry-'+(mobile?'mobile':'web'),shots=[],layouts=[];
  async function shot(suffix){await assertNoCredentials(page);const file=prefix+'-'+suffix+'.png';await page.screenshot({path:path.join(screenshots,file)});shots.push({file:'docs/media/screenshots/'+file,sha256:sha(await readFile(path.join(screenshots,file)))});}
  await button('确认创建重试').scrollIntoViewIfNeeded();
  const recording=await recordViewport(page,app,prefix,async()=>{
    await shot('failure');
    if(mobile){await root().locator('textarea[aria-label="人工重试原因"],input[aria-label="人工重试原因"]').fill('Synthetic diagnosed notification retry');await root().locator('.cp-check').filter({hasText:'已核对失败原因'}).click();}
    else {await page.getByLabel('人工重试原因',{exact:true}).fill('Synthetic diagnosed notification retry');await page.getByLabel('已核对失败原因；按原事件重新检查当前接收人，不修改原始业务。',{exact:true}).check();}
    assert.equal(await button('确认创建重试').isEnabled(),true);
    const created=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/notification-delivery-jobs/'+source.job_id+'/retry'&&r.request().method()==='POST');
    await button('确认创建重试').click();const response=await created;assert.equal(response.status(),201);const receipt=(await response.json()).data;
    assert.equal(receipt.source_job_id,source.job_id);assert.equal(receipt.business_event_id,source.business_event_id);assert.equal(receipt.created_by,me.user.id);
    const parent=await get('/notification-delivery-jobs/'+source.job_id);assert.equal(parent.state,'dead');assert.equal(parent.attempt,source.attempt);assert.equal(parent.updated_at,source.updated_at);assert.equal(parent.retry_job_id,receipt.retry_job_id);assert.equal(parent.can_retry,false);
    checks.push('Explicit reason, confirmation and recent MFA append one retry receipt and fresh child without changing the terminal source or original event.');
    let result;
    for(let i=0;i<60;i++){result=await get('/notification-delivery-jobs/'+receipt.retry_job_id);if(result.state==='succeeded')break;await page.waitForTimeout(200);}
    assert.equal(result.state,'succeeded');assert.equal(result.result.outcome,'delivered');assert.equal(result.result.delivered,1);
    await root().getByText(/已写入站内信 1 条/).waitFor();await root().getByText(/已写入站内信 1 条/).scrollIntoViewIfNeeded();await shot('result');
    checks.push('The real leased notification service commits a recipient inbox result; the page reports in-app writes separately from employee reads and TG sends.');
    const history=await get('/notification-delivery-retries?limit=50');assert.equal(history.items.filter(r=>r.source_job_id===source.job_id).length,1);
    await button('查看原失败').first().click();await root().getByText('已创建后续任务，请查看其结果',{exact:true}).waitFor();
    assert.equal(await button('确认创建重试').count(),0);await button('查看已创建的后续任务').scrollIntoViewIfNeeded();await shot('history');
    checks.push('Retry history keeps original and child links, while an already retried source offers no second creation action.');
    if(!mobile){await page.getByLabel('外观偏好',{exact:true}).selectOption('dark');await shot('dark');await page.getByLabel('外观偏好',{exact:true}).selectOption('light');}
  });
  for(const width of mobile?[390]:[1440,900,390]){await page.setViewportSize({width,height:mobile?900:1100});await page.waitForTimeout(200);const layout=await page.evaluate(()=>({client:document.documentElement.clientWidth,scroll:document.documentElement.scrollWidth}));assert.ok(layout.scroll<=layout.client+2,JSON.stringify(layout));layouts.push({width,...layout});}
  checks.push('Reviewed viewports fit the notification and recovery controls; no device, live TG or cloud acceptance is claimed.');
  await writeFile(path.join(evidence,'capture.json'),JSON.stringify({recordings:[recording],layouts,screenshots:shots,synthetic_data:true,real_browser_pixels:true,audio:false},null,2)+'\n');
},'notification-retry');
