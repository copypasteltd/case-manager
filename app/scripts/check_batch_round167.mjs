import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { batchBrowser, waitOTP } from './batch_round_browser167_common.mjs';
import { assertNoCredentials, recordViewport } from './batch_round167_recording.mjs';
const app=fileURLToPath(new URL('../',import.meta.url)), mobile=process.argv.includes('--mobile');
await batchBrowser(mobile,async({page,fixture,checks,evidence})=>{
  await page.goto(fixture.origin);
  if(mobile){
    await page.locator('input[aria-label="内部账号"]').fill(fixture.alice.login);await page.locator('input[aria-label="密码"]').fill(fixture.alice.password);await page.getByRole('button',{name:/登录工作空间/}).click();await page.locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await page.getByRole('button',{name:/验证并继续/}).click();await page.locator('.taro_page_show .cp-title').filter({hasText:'工作从这里开始'}).last().waitFor();
  }else{await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码',{exact:true}).fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();}
  const screen=()=>mobile?page.locator('.taro_page_show').last():page.locator('main');
  await page.goto(fixture.origin+(mobile?'/#/pages/settings/index?section=members':'/administration'));
  const pending=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/users/'+fixture.invited_user_id+'/invitation'&&r.status()===200);
  if(mobile){
    await screen().getByRole('button',{name:'查看邀请 · '+fixture.invited_user_name,exact:true}).click();
    await screen().getByRole('button',{name:'核对并查看当前邀请',exact:true}).click();
  }else{
    await screen().getByRole('row').filter({hasText:fixture.invited_user_name}).getByRole('button',{name:'查看邀请',exact:true}).click();
  }
  const old=(await(await pending).json()).data;
  assert.equal(old.invitation_id,fixture.original_invitation_id);assert.equal(old.activation_token,undefined);
  const panel=()=>mobile?screen():page.getByRole('dialog');
  await panel().getByText(/此邀请已/).waitFor();
  const egroup=process.env.CP_BROWSER_EVIDENCE_GROUP,root=path.dirname(app.replace(/[\\/]$/,'')),media=path.join(root,'docs/media/batch-round-167',egroup);
  await mkdir(media,{recursive:true});const shots=[];
  const shot=async name=>{await assertNoCredentials(page);const file=path.join(media,name+'.png'),bytes=await page.screenshot({path:file,fullPage:!mobile,caret:'hide'});shots.push({file:path.relative(root,file).replaceAll('\\','/'),sha256:createHash('sha256').update(bytes).digest('hex'),credentials_excluded:true});};
  let receipt;
  const recording=await recordViewport(page,app,egroup,async()=>{
    await page.waitForTimeout(450);await shot(mobile?'mobile-expired-invitation':'web-expired-invitation');
    checks.push('actual authorized API exposes expired invitation metadata without an activation token');
    await panel().getByRole('button',{name:'重新签发邀请',exact:true}).click();
    const reason=mobile?panel().locator('textarea[aria-label="重发原因"]'):panel().getByLabel('重发原因',{exact:true});
    await reason.fill('原邀请已过期，重新交付给协作成员');
    const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/users/'+fixture.invited_user_id+'/invitation-reissues'&&r.request().method()==='POST');
    await panel().getByRole('button',{name:'确认重发邀请',exact:true}).click();
    const result=await response;assert.equal(result.status(),201);receipt=(await result.json()).data;
    assert.equal(receipt.user.id,old.user.id);assert.equal(receipt.user.version,old.user.version+1);assert.equal(receipt.replaces_id,old.invitation_id);assert.notEqual(receipt.invitation_id,old.invitation_id);
    await panel().getByText('已重新签发，原邀请码已失效。',{exact:true}).waitFor();
    checks.push('ordinary human reissue preserves the invited user and replaces the exact reviewed invitation');
    assert.equal(await panel().getByRole('button',{name:'显示邀请码',exact:true}).count(),1);
    await page.waitForTimeout(350);await shot(mobile?'mobile-reissued-invitation':'web-reissued-invitation');
    checks.push('new activation capability remains hidden throughout actual screenshot and viewport recording');
  });
  await writeFile(path.join(evidence,'business-proof.json'),JSON.stringify({passed:true,synthetic_data:true,user_id:old.user.id,previous_invitation_id:old.invitation_id,invitation_id:receipt.invitation_id,before_version:old.user.version,after_version:receipt.user.version,expired_metadata_only:true,plaintext_excluded:true},null,2)+'\n');
  await writeFile(path.join(evidence,'media.json'),JSON.stringify({passed:true,synthetic_data:true,authenticated_capture:true,credentials_excluded:true,screenshots:shots,recording},null,2)+'\n');
});
