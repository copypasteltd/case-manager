// Actual synthetic HTTP and native PostgreSQL policy publication; no mocks.
import assert from 'node:assert/strict';
import { createHash,randomUUID } from 'node:crypto';
import { mkdir,readFile,writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser,waitOTP } from './contract_reminders_browser_common.mjs';
import { mobileScreen,selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials,recordViewport } from './contract_reminders_recording.mjs';

const mobile=process.argv.includes('--mobile'),app=fileURLToPath(new URL('../',import.meta.url));
const sha=b=>createHash('sha256').update(b).digest('hex');
await notificationBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
  const root=()=>mobile?mobileScreen(page):page;
  const button=name=>root().getByRole('button',{name,exact:true});
  const field=name=>mobile?root().locator('input[aria-label="'+name+'"],textarea[aria-label="'+name+'"]'):page.getByLabel(name,{exact:true});
  const actor=fixture.alice;assert.ok(fixture.contract_reminders_fixture&&fixture.isolated_database);
  let bearer;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))bearer=r.headers().authorization;});
  await page.goto(fixture.origin);await field('内部账号').fill(actor.login);await field('密码').fill(actor.password);
  await button(mobile?'登录工作空间 ↗':'登录工作空间').click();await field('六位验证码').fill(await waitOTP({alice:actor}));
  await button(mobile?'验证并继续 ↗':'验证并继续').click();
  if(mobile)await root().locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();else await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  const lastStep=Math.floor(Date.now()/30000),origin=mobile?fixture.api_origin:fixture.origin;
  async function headers(){return mobile?{Authorization:bearer}:{Origin:fixture.origin,'X-CSRF-Token':(await context.cookies()).find(c=>c.name==='cp_csrf')?.value??''};}
  async function get(route){const response=await context.request.get(origin+'/v1'+route,{headers:await headers()});assert.equal(response.status(),200,route);return(await response.json()).data;}
  await page.goto(fixture.origin+(mobile?'/#/pages/notifications/index':'/notifications'));
  await root().getByText('合同到期提醒策略',{exact:true}).waitFor();
  const me=await get('/me');assert.ok(me.actions.includes('notification.policy.manage'));
  const initial=(await get('/notification-contract-policy')).current;assert.equal(initial.version,fixture.current_policy_version);
  await button('修改提醒策略').click();assert.equal(await button('确认发布提醒策略').isEnabled(),false);
  assert.equal((await get('/notification-contract-policy')).current.id,initial.id);
  checks.push('An explicitly authorized human opens current organization policy without publishing anything; no rule is automatically selected.');
  if(mobile){await field('验证器六位验证码').fill(await waitOTP({alice:{...actor,last_step:lastStep}}));await button('确认本人操作').click();await root().getByText('已通过本次两步验证，请继续核对并确认操作。',{exact:true}).waitFor();}
  await field('提前天数').fill('7,7');await field('策略变更原因').fill('Synthetic explicit policy review');
  if(mobile)await root().locator('.cp-check').filter({hasText:'已核对组织范围'}).click();else await page.getByLabel('已核对组织范围、时区、提前量及历史提醒处理方式。',{exact:true}).check();
  await button('确认发布提醒策略').click();await root().getByText('提前量须为互不重复的0至365天。',{exact:true}).waitFor();
  assert.equal((await get('/notification-contract-policy')).current.id,initial.id);
  checks.push('Actual form validation rejects duplicate thresholds while retaining input and the current published version; MFA and explicit confirmation are required.');
  const directory=path.join(app,'../docs/media/screenshots');await mkdir(directory,{recursive:true});
  const prefix='contract-reminders-'+(mobile?'mobile':'web'),shots=[],layouts=[];
  async function shot(suffix){await assertNoCredentials(page);const file=prefix+'-'+suffix+'.png';await page.screenshot({path:path.join(directory,file)});shots.push({file:'docs/media/screenshots/'+file,sha256:sha(await readFile(path.join(directory,file)))});}
  await button('确认发布提醒策略').scrollIntoViewIfNeeded();
  const recording=await recordViewport(page,app,prefix,async()=>{
    await shot('review');
    if(mobile)await selectMobile(page,'提醒开关','停用');else await field('提醒开关').selectOption('disabled');
    await field('提前天数').fill('');await field('策略变更原因').fill('Synthetic explicit stop of contract reminders');
    assert.equal(await button('确认发布提醒策略').isEnabled(),false);
    if(mobile)await root().locator('.cp-check').filter({hasText:'已核对组织范围'}).click();else await page.getByLabel('已核对组织范围、时区、提前量及历史提醒处理方式。',{exact:true}).check();
    await button('确认发布提醒策略').scrollIntoViewIfNeeded();
    const created=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/notification-contract-policy'&&r.request().method()==='POST');
    await button('确认发布提醒策略').click();const response=await created;assert.equal(response.status(),201);const receipt=(await response.json()).data;
    assert.equal(receipt.version,initial.version+1);assert.equal(receipt.enabled,false);assert.deepEqual(receipt.days_before,[]);assert.equal(receipt.created_by,me.user.id);
    await button('修改提醒策略').waitFor();await button('刷新提醒策略').scrollIntoViewIfNeeded();
    assert.equal((await get('/notification-contract-policy')).current.id,receipt.id);await shot('published');
    checks.push('A real UI submission appends one disabled immutable policy and the page reloads the persisted native PostgreSQL result.');
    const stale=await context.request.post(origin+'/v1/notification-contract-policy',{headers:{...await headers(),'Idempotency-Key':randomUUID()},data:{expected_version:initial.version,enabled:true,timezone:initial.timezone,days_before:[0,7],reason:'Synthetic stale concurrent publication'}});
    assert.equal(stale.status(),409);
    const history=await get('/notification-contract-policy-history?limit=50');const original=history.items.find(p=>p.id===initial.id);assert.deepEqual(original,initial);assert.equal(history.items.filter(p=>p.id===receipt.id).length,1);assert.equal((await get('/notification-contract-policy')).current.id,receipt.id);
    await root().getByText('策略发布历史',{exact:true}).evaluate(node=>node.scrollIntoView({block:'start'}));await shot('history');
    checks.push('The stale expected version is rejected, while published history retains the exact original and one new revision.');
    if(!mobile){await page.getByLabel('外观偏好',{exact:true}).selectOption('dark');await button('刷新提醒策略').scrollIntoViewIfNeeded();await shot('dark');await page.getByLabel('外观偏好',{exact:true}).selectOption('light');}
  });
  for(const width of mobile?[390]:[1440,390]){await page.setViewportSize({width,height:900});await button('刷新提醒策略').scrollIntoViewIfNeeded();const size=await(mobile?root():page.locator('html')).evaluate(node=>({width:node.clientWidth,scroll:node.scrollWidth}));assert.ok(size.scroll<=size.width+1);layouts.push({width,...size,passed:true});}
  checks.push('The existing Web and shared mobile layout contains the policy form and publication history without horizontal overflow; no device or external delivery acceptance is claimed.');
  await writeFile(path.join(evidence,'capture.json'),JSON.stringify({recordings:[recording],layouts,screenshots:shots,synthetic_data:true,real_browser_pixels:true,audio:false},null,2)+'\n');
},'contract-reminders');
