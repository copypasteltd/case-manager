import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { notificationBrowser, waitOTP } from './retirement_reauthorization_browser_common.mjs';
import { assertNoCredentials, recordViewport } from './retirement_reauthorization_recording.mjs';

const app = fileURLToPath(new URL('../', import.meta.url));
await notificationBrowser(false, async ({ page, fixture, checks, evidence }) => {
 await page.goto(fixture.origin);
 await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);
 await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);
 await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
 await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));
 await page.getByRole('button',{name:'验证并继续',exact:true}).click();
 await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 const url=fixture.origin+'/files/'+fixture.file_id;
 async function openPlan(){
  await page.getByRole('button',{name:'盘点历史与草稿原件',exact:true}).click();
  await page.getByRole('button',{name:'查阅原件清单',exact:true}).first().click();
  await page.getByText(fixture.snapshot_sha256,{exact:true}).waitFor();
 }
 async function readPlan(){
  return page.evaluate(async id=>{const r=await fetch('/v1/object-retirement-plans/'+id);if(!r.ok)throw new Error('Durable plan read failed');return (await r.json()).data;},fixture.plan_id);
 }
 await page.goto(url);await openPlan();
 const before=await readPlan();
 assert.equal(before.items.filter(v=>v.state==='succeeded').length,1);
 assert.equal(before.items.filter(v=>v.state==='blocked').length,1);
 assert.equal(before.authorization,null);
 const prefix='retirement-reauthorization-web',screenshots=[];
 async function shot(suffix){
  await assertNoCredentials(page);
  const file='docs/media/screenshots/'+prefix+'-'+suffix+'.png';
  await mkdir(path.dirname(path.join(app,'..',file)),{recursive:true});
  const bytes=await page.screenshot({path:path.join(app,'..',file)});
  screenshots.push({file,sha256:createHash('sha256').update(bytes).digest('hex')});
 }
 await page.getByRole('heading',{name:/已批准 · 1\/2项已确认/}).scrollIntoViewIfNeeded();
 await shot('stopped');
 checks.push('Authenticated current Web session reads a durable partial plan: one confirmed result, one blocked result, immutable first approval.');
 await page.getByRole('button',{name:'重新授权剩余原件',exact:true}).click();
 const decision=page.getByRole('region',{name:'确认重新授权并继续',exact:true});
 await decision.getByLabel('原件操作依据',{exact:true}).fill('Synthetic human136: explicitly adopt only the stopped remaining original.');
 await decision.getByRole('checkbox',{name:'我已核对原件范围、摘要及不可逆影响',exact:true}).check();
 await decision.getByText('最近已完成两步验证。请继续核对内容，再确认本次操作。',{exact:true}).waitFor();
 await decision.scrollIntoViewIfNeeded();await shot('review');
 let committed;
 const recording=await recordViewport(page,app,prefix,async()=>{
  const pending=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname.endsWith('/reauthorize'));
  await decision.getByRole('button',{name:'确认重新授权并继续',exact:true}).click();
  const response=await pending;assert.equal(response.status(),202);committed=(await response.json()).data;
  assert.equal(committed.authorization.items.length,1);
  assert.equal(committed.authorization.sequence,1);
  assert.equal(committed.items.filter(v=>v.state==='queued'&&v.generation===2).length,1);
  assert.deepEqual(committed.items.find(v=>v.state==='succeeded'),before.items.find(v=>v.state==='succeeded'));
  assert.equal(committed.approved_at,before.approved_at);assert.equal(committed.approval_reason,before.approval_reason);
  await page.getByText(/最近重新授权：第1次/).waitFor();
  await page.reload();await openPlan();
  await page.getByRole('button',{name:'查阅重新授权记录',exact:true}).click();
  await page.getByRole('region',{name:'重新授权记录',exact:true}).getByText(/第1次重新授权/).waitFor();
  await page.getByRole('region',{name:'重新授权记录',exact:true}).scrollIntoViewIfNeeded();
 });
 await shot('history');
 const after=await readPlan();assert.deepEqual(after,committed);
 checks.push('Explicit human acknowledgement and fresh MFA commit one renewed grant and exactly one next-generation queued item through the actual Web command.');
 checks.push('Reload and independent durable readback retain the first approval, confirmed original, fixed source digest and immutable renewed authorization history.');
 const proof={plan_id:fixture.plan_id,snapshot_sha256:before.snapshot_sha256,authorization_id:after.authorization.id,sequence:after.authorization.sequence,confirmed_originals_preserved:true,remaining_selected:1,new_generation:2,durable_readback:true,first_approval_unchanged:true,cloud_operations_in_browser:false,real_cloud:false,fixture_scope:fixture.fixture_scope};
 await writeFile(path.join(evidence,'business-proof.json'),JSON.stringify(proof,null,2)+'\n');
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({screenshots,recording,credentials_excluded:true,synthetic_data:true},null,2)+'\n');
},'retirement-reauthorization');
