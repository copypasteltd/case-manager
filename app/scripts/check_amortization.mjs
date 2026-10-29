import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {notificationBrowser,waitOTP} from './amortization_browser_common.mjs';
import {mobileScreen} from './mobile_picker_browser.mjs';
import {assertNoCredentials,recordViewport} from './amortization_recording.mjs';

const mobile=process.argv.includes('--mobile'),app=fileURLToPath(new URL('../',import.meta.url));
await notificationBrowser(mobile,async({page,context,fixture,checks,evidence})=>{
  const root=()=>mobile?mobileScreen(page):page,button=name=>root().getByRole('button',{name,exact:true});
  const field=name=>mobile?root().locator('input[aria-label="'+name+'"]'):page.getByLabel(name,{exact:true});
  let bearer;page.on('request',request=>{if(request.headers().authorization?.startsWith('Bearer '))bearer=request.headers().authorization;});
  await page.goto(fixture.origin);await field('内部账号').fill(fixture.alice.login);await field('密码').fill(fixture.alice.password);
  await button(mobile?'登录工作空间 ↗':'登录工作空间').click();await field('六位验证码').fill(await waitOTP(fixture));await button(mobile?'验证并继续 ↗':'验证并继续').click();
  if(mobile)await root().locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();else await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  fixture.alice.last_step=Math.floor(Date.now()/30000);
  const origin=mobile?fixture.api_origin:fixture.origin;
  async function get(route){const response=await context.request.get(origin+'/v1'+route,mobile?{headers:{Authorization:bearer}}:{});assert.equal(response.status(),200,route);return(await response.json()).data;}
  const route='/costs/'+fixture.cost_id,planPath=route+'/amortization';
  const source=await get(route);
  await page.goto(fixture.origin+(mobile?'/#/pages/finance/index?kind=cost&id='+fixture.cost_id:route));
  await button('查阅摊销计划').click();await root().getByText(/未发布，未自动采用任何规则/).waitFor();
  assert.equal((await get(planPath)).version,0);checks.push('No implicit plan or rule is created by reading the original confirmed expense.');
  await button('编制或停用摊销计划').click();
  if(mobile)await button('所涉月份均分（首尾月同权）').click();else await field('摊销规则').selectOption('monthly_equal');
  await field('服务起始日').fill('2025-01-01');await field('服务结束日（不含）').fill('2025-04-01');await field('计划操作依据').fill('Synthetic human review: three service months with project and company shares');
  await button('预览月度摊销').click();await root().getByText('待发布预览 · 第 1 版',{exact:true}).waitFor();
  assert.equal((await get(planPath)).version,0);assert.equal((await get(route)).version,source.version);
  checks.push('Explicit monthly preview conserves each project/company share and leaves source version and active plans unchanged.');
  if(mobile){
    await field('验证器六位验证码').fill(await waitOTP(fixture));await button('确认本人操作').click();await root().getByText('已通过本次两步验证，请继续核对并确认操作。',{exact:true}).waitFor();
    await button('确认重列当前计划口径并保留历史').click();
  }else await page.getByLabel('我确认该操作将重列当前摊销计划视图，历史版本及已完成导出保留',{exact:true}).check();
  await assertNoCredentials(page);
  const prefix='cost-amortization-'+(mobile?'mobile':'web'),screenshots=path.join(app,'../docs/media/screenshots');await mkdir(screenshots,{recursive:true});
  const shots=[],layouts=[];
  async function screenshot(suffix){await assertNoCredentials(page);const name=prefix+'-'+suffix+'.png';await page.screenshot({path:path.join(screenshots,name)});shots.push({file:'docs/media/screenshots/'+name,sha256:createHash('sha256').update(await readFile(path.join(screenshots,name))).digest('hex')});}
  const recording=await recordViewport(page,app,prefix,async()=>{
    await button('确认发布摊销计划').scrollIntoViewIfNeeded();await screenshot('preview');
    await button('确认发布摊销计划').click();await root().getByText(/第 1 版 · 有效/).waitFor();
    const plan=await get(planPath);assert.equal(plan.version,1);assert.equal(plan.current.state,'active');assert.equal(plan.current.content.periods.length,3);
    assert.deepEqual(plan.current.content.periods.map(p=>p.amount),['33.330000','33.340000','33.330000']);assert.equal((await get(route)).content_hash,source.content_hash);
    checks.push('Human publication requires acknowledgement and recent MFA, returns an immutable attributed version, and preserves the original cost hash.');
    await button('查看第 1 版依据').scrollIntoViewIfNeeded();await screenshot('published');await button('查看第 1 版依据').click();
    await root().getByText('SHA-256：'+plan.current.content_sha256,{exact:true}).waitFor();await screenshot('history');
    checks.push('The published plan can be reopened with exact revision, author, reason, original shares and SHA-256 evidence.');
    await page.goto(fixture.origin+(mobile?'/#/pages/reports/index':'/reports/expenses'));
    await button('费用摊销').click();
    if(!await field(mobile?'报表起始日':'起始日（含）').isVisible())await button(mobile?'调整筛选条件':'调整筛选').click();
    await field(mobile?'报表起始日':'起始日（含）').fill('2025-01-01');await field(mobile?'报表结束日':'结束日（不含）').fill('2025-04-02');await button('查询报表').click();
    await root().getByText(/按当前最新有效计划重列视图/).waitFor();
    const report=await get('/finance/reports?kind=amortization&from=2025-01-01&to=2025-04-02&limit=100');
    assert.equal(report.summary.basis,'latest_active_schedules');assert.equal(report.summary.totals[0].net_amount,'100.000000');
    const own=report.items.filter(v=>v.resource_id===fixture.cost_id);assert.equal(own.length,3);assert.ok(own.every(v=>v.amortization.plan_id===plan.current.id&&v.source_type==='cost_amortization'));
    checks.push('The independent amortization report exposes only authorized current plans and retains the exact original expense/plan/month references.');
    await screenshot('report');
  });
  for(const width of mobile?[390]:[1440,900,390]){
    await page.setViewportSize({width,height:mobile?900:1100});await page.waitForTimeout(200);
    const layout=await page.evaluate(()=>({client:document.documentElement.clientWidth,scroll:document.documentElement.scrollWidth}));assert.ok(layout.scroll<=layout.client+2,JSON.stringify(layout));layouts.push({width,...layout});
  }
  checks.push('Authenticated report layouts stay within the Web/portable viewport; wide schedule tables scroll inside their own container.');
  await writeFile(path.join(evidence,'capture.json'),JSON.stringify({recordings:[recording],layouts,screenshots:shots,synthetic_data:true,real_browser_pixels:true,audio:false},null,2)+'\n');
},'cost-amortization');
