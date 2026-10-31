import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {notificationBrowser,waitOTP} from './completed_upload_retirement_browser_common.mjs';
import {assertNoCredentials,recordViewport} from './completed_upload_retirement_recording.mjs';

const app=fileURLToPath(new URL('../',import.meta.url));
await notificationBrowser(false,async({page,fixture,checks,evidence})=>{
 await page.goto(fixture.origin);
 await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);
 await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);
 await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
 await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));
 await page.getByRole('button',{name:'验证并继续',exact:true}).click();
 await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 await page.goto(fixture.origin+'/files/folders/'+fixture.folder_id);
 const workspace=page.getByRole('region',{name:'已完成上传草稿',exact:true});
 await workspace.getByRole('button',{name:'盘点未提交的上传',exact:true}).click();
 const radio=workspace.getByRole('radio',{name:/选择Unsubmitted new file.bin/});
 await radio.waitFor();await radio.check();await workspace.scrollIntoViewIfNeeded();
 const prefix='completed-upload-retirement-web',screenshots=[];
 async function shot(suffix){await assertNoCredentials(page);const file='docs/media/screenshots/'+prefix+'-'+suffix+'.png';await mkdir(path.dirname(path.join(app,'..',file)),{recursive:true});const bytes=await page.screenshot({path:path.join(app,'..',file)});screenshots.push({file,sha256:createHash('sha256').update(bytes).digest('hex')});}
 await shot('inventory');
 checks.push('Authenticated folder inventory exposes the exact new-file upload and both known completed objects without inventing a formal file.');
 await workspace.getByLabel('原件清理原因',{exact:true}).fill('Synthetic human138: retire the exact expired unsubmitted upload and all known completed candidates.');
 await shot('selection');
 let plan;
 const recording=await recordViewport(page,app,prefix,async()=>{
  const pending=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname.endsWith('/completed-draft-retirement-plans'));
  await workspace.getByRole('button',{name:'建立原件观察清单',exact:true}).click();
  const response=await pending;assert.equal(response.status(),201);plan=(await response.json()).data;
  assert.equal(plan.snapshot.kind,'uncommitted_draft');assert.equal(plan.snapshot.folder_id,fixture.folder_id);assert.equal(plan.snapshot.target_id,fixture.upload_id);assert.equal(plan.snapshot.file_id,undefined);assert.equal(plan.items.length,2);assert.equal(plan.state,'observing');
  await workspace.getByText(plan.snapshot_sha256,{exact:true}).waitFor();
  await page.reload();await page.getByRole('button',{name:'盘点未提交的上传',exact:true}).click();
  await page.getByRole('button',{name:'查阅原件清单',exact:true}).first().click();
  await page.getByText(plan.snapshot_sha256,{exact:true}).waitFor();
  await page.getByRole('region',{name:'原件清单详情',exact:true}).scrollIntoViewIfNeeded();
 });
 await shot('plan');
 const after=await page.evaluate(async id=>{const r=await fetch('/v1/object-retirement-plans/'+id);if(!r.ok)throw new Error('Durable read failed');return(await r.json()).data;},plan.id);
 assert.deepEqual(after,plan);
 checks.push('A visible human selection and reason commit one fixed folder/upload revision with ordered upload and promotion objects.');
 checks.push('Reload and independent authenticated readback retain the immutable plan and mandatory observation period; no deletion is executed in this browser.');
 await writeFile(path.join(evidence,'business-proof.json'),JSON.stringify({plan_id:plan.id,folder_id:fixture.folder_id,upload_id:fixture.upload_id,snapshot_sha256:plan.snapshot_sha256,object_count:plan.items.length,new_file_without_formal_identity:true,durable_readback:true,real_cloud:false,cloud_operations_in_browser:false,fixture_scope:fixture.fixture_scope},null,2)+'\n');
 await writeFile(path.join(evidence,'media.json'),JSON.stringify({screenshots,recording,credentials_excluded:true,synthetic_data:true},null,2)+'\n');
},'completed-upload-retirement');
