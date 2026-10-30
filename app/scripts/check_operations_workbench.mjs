// Actual input, native API/PG and a byte-verifying owned synthetic copy worker.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { otp, workbenchBrowser } from './workbench_browser_common.mjs';
import { mobileScreen, selectMobile } from './mobile_picker_browser.mjs';
import { assertNoCredentials, recordViewport } from './workbench_recording.mjs';

const mobile = process.argv.includes('--mobile'), app = fileURLToPath(new URL('../', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
await workbenchBrowser(mobile, async ({ page, context, fixture, checks, evidence }) => {
  const root = () => mobile ? mobileScreen(page) : page;
  const field = label => mobile ? root().locator('input[aria-label="' + label + '"],textarea[aria-label="' + label + '"]') : page.getByLabel(label, { exact: true });
  const button = label => root().getByRole('button', { name: label, exact: true });
  let bearer, usedStep = fixture.alice.last_step;
  page.on('request', request => { if (request.headers().authorization?.startsWith('Bearer ')) bearer = request.headers().authorization; });
  async function nextOTP() {
    if (Math.floor(Date.now()/30000) <= usedStep) await page.waitForTimeout((usedStep+1)*30000-Date.now()+250);
    usedStep = Math.floor(Date.now()/30000);
    return otp(fixture.alice.mfa_secret, usedStep);
  }
  await page.goto(fixture.origin);
  await field('内部账号').fill(fixture.alice.login); await field('密码').fill(fixture.alice.password);
  await button(mobile ? '登录工作空间 ↗' : '登录工作空间').click();
  await field('六位验证码').fill(await nextOTP()); await button(mobile ? '验证并继续 ↗' : '验证并继续').click();
  await root().getByText('业务待办', { exact: true }).waitFor();
  const origin = mobile ? fixture.api_origin : fixture.origin;
  const headers = () => mobile ? { Authorization: bearer } : { Origin: fixture.origin };
  async function get(route) { const response = await context.request.get(origin+'/v1'+route, { headers: headers() }); assert.equal(response.status(), 200, route); return (await response.json()).data; }
  const group = (summary, kind) => { const value = summary.groups.find(g => g.kind === kind); assert.ok(value); return value; };
  const section = label => mobile ? root().locator('.cp-section').filter({ hasText: label }) : page.getByRole('region', { name: label, exact: true });
  const readyCount = async (label, count) => {
    if (mobile) await root().getByText(label+' · '+count, { exact: true }).waitFor();
    else await section(label).getByLabel(label+'数量', { exact: true }).filter({ hasText: String(count) }).waitFor();
  };
  const home = async () => {
    if (mobile) { await page.goBack(); await page.waitForURL(url => url.hash === '#/pages/home/index'); }
    else await page.goto(fixture.origin+'/');
    await root().getByText('业务待办', { exact: true }).waitFor();
  };
  const refresh = async () => {
    const result = page.waitForResponse(r => new URL(r.url()).pathname === '/v1/dashboard/business' && r.request().method() === 'GET' && r.status() === 200);
    await button(mobile ? '刷新工作台' : '刷新业务待办').click(); await result;
  };
  const current = await get('/dashboard/business');
  assert.equal(current.groups.length, 5);
  assert.equal(group(current,'incidents').count,1); assert.equal(group(current,'file_attention').count,1);
  assert.equal(group(current,'file_attention').items[0].state,'replica_attention');
  await readyCount('我负责的未结事件',1); await readyCount('我创建的文件存储待办',1);
  checks.push({ name: 'Authenticated home shows exact assigned incident and actual corrupt-copy work', passed: true });
  const prefix = 'operations-workbench-'+(mobile?'mobile':'web'), shots = [], directory = path.join(app,'../docs/media/screenshots');
  await mkdir(directory,{ recursive:true });
  async function shot(suffix) {
    await assertNoCredentials(page); const file = 'docs/media/screenshots/'+prefix+'-'+suffix+'.png';
    await page.screenshot({ path: path.join(app,'..',file),fullPage:false });
    shots.push({ file,sha256:sha(await readFile(path.join(app,'..',file))) });
  }
  await section('我负责的未结事件').scrollIntoViewIfNeeded(); await shot('summary');
  await section('我负责的未结事件').getByText(group(current,'incidents').items[0].title,{ exact:true }).click();
  await button(mobile?'追加事件进展':'更新处置进展').click();
  if (mobile) await selectMobile(page,'事件处理阶段','已解决'); else await field('更新后的进度').selectOption('resolved');
  await field(mobile?'本次时间线说明':'新增时间线记录').fill('已核对合成服务的恢复结果');
  await field(mobile?'恢复与解决说明':'恢复说明').fill('Synthetic recovery verified by the assigned human');
  await field(mobile?'操作依据':'本次变更原因').fill('工作台进入详情后显式关闭事件');
  if (mobile) { await field('验证器六位验证码').fill(await nextOTP()); await button('确认本人操作').click(); }
  const changed = page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/incidents/'+fixture.incident_id && r.request().method()==='PATCH');
  await button(mobile?'确认追加事件进展':'确认追加进展').click(); assert.equal((await changed).status(),200);
  assert.equal((await get('/incidents/'+fixture.incident_id)).state,'resolved');
  await home(); await readyCount('我负责的未结事件',0);
  assert.equal(group(await get('/dashboard/business'),'incidents').count,0);
  checks.push({ name:'Ordinary incident editor appends an authorized resolution and the next home read removes it', passed:true });

  await section('我创建的文件存储待办').getByText(group(current,'file_attention').items[0].title,{ exact:true }).click();
  await button('重新安排副本复制').click();
  await field('重试原因').fill('已修复合成目标存储，申请重新复制');
  if(mobile) await root().getByText('我已核对这个副本及重试原因',{ exact:true }).click();
  const retry = page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/file-replicas/'+fixture.replica_id+'/retry' && r.request().method()==='POST');
  await button('安排复制').click(); assert.equal((await retry).status(),200);
  if(mobile) await root().getByText('已核对副本重试结果',{ exact:true }).waitFor(); else await page.getByRole('dialog').waitFor({ state:'hidden' });
  await home(); await readyCount('我创建的文件存储待办',1);
  const queued = group(await get('/dashboard/business'),'file_attention'); assert.equal(queued.items[0].state,'replica_pending');
  await section('我创建的文件存储待办').scrollIntoViewIfNeeded(); await shot('pending');
  checks.push({ name:'Ordinary file retry pins its receipt and queued work remains visible until verification',passed:true });
  const recording = await recordViewport(page, app, prefix, async()=>{
    const verification = await context.request.post(fixture.api_origin+'/__fixture/operations/verify-copy',{ headers:{ Authorization:'Bearer '+fixture.stop_token } });
    assert.equal(verification.status(),204);
    await refresh(); await readyCount('我创建的文件存储待办',0);
    await section('我创建的文件存储待办').scrollIntoViewIfNeeded(); await page.waitForTimeout(2000);
  });
  const completed = await get('/dashboard/business');
  assert.equal(group(completed,'incidents').count,0); assert.equal(group(completed,'file_attention').count,0);
  const versions = await get('/files/'+fixture.file_id+'/versions');
  assert.ok(versions.items.find(v=>v.id===fixture.file_version_id)?.replicas.every(r=>r.state==='verified' && r.actual_sha256 && r.verified_at));
  await shot('complete');
  checks.push({ name:'Owned worker verifies actual synthetic bytes and refreshed home clears both completed groups',passed:true });
  await writeFile(path.join(evidence,'media.json'),JSON.stringify({screenshots:shots,recording,synthetic_data:true,credentials_excluded:true,gif_mode:'actual_screenshot_tour',no_real_cloud:true,no_real_device:true},null,2)+'\n');
},'operations-workbench');
