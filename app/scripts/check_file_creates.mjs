import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';

await notificationBrowser(false,async({page,context,fixture,checks})=>{
  await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();
  await page.getByLabel('六位验证码').fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
  await page.goto(fixture.origin+'/conversations/'+fixture.conversation_id+'?run='+fixture.completed_run_id);
  await page.getByRole('heading',{name:'文件新建实际结果',exact:true}).waitFor();
  async function request(route){const response=await context.request.get(fixture.origin+'/v1'+route);assert.equal(response.status(),200);return(await response.json()).data;}
  const original=await request('/runs/'+fixture.completed_run_id),created=original.file_create.result;
  assert.equal(original.file_create.status,'committed');assert.equal(created.file_version.seq,1);assert.equal(created.file_version.base_version_id,null);assert.equal(created.file_version.scan_state,'clean');assert.equal(created.file_version.source,'agent');
  assert.match(await page.locator('body').innerText(),/香港🙂：明确下一项工作/);
  checks.push('Web renders a genuinely committed new file, its generated text and first scanned formal version.');
  assert.equal(created.file_version.replicas[1].state,'queued');await page.getByText('第二存储副本按文件详情的实际复制校验状态显示。',{exact:true}).waitFor();
  checks.push('The page preserves the true pending second replica instead of claiming two-cloud completion.');
  let sends=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sends++;});
  await page.getByLabel('本次秘书任务',{exact:true}).selectOption(fixture.profile_id);await page.getByRole('button',{name:'选择新建文件位置',exact:true}).click();
  await page.getByLabel('新文件资料目录',{exact:true}).selectOption(fixture.folder_id);await page.getByText(/目标：.+修订/).waitFor();
  const folder=await request('/folders/'+fixture.folder_id),name='Web待生成会议纪要.md';
  await page.getByLabel('新文件名称',{exact:true}).fill(name);await page.getByRole('button',{name:'将准确文件新建加入请求',exact:true}).click();
  assert.equal(sends,0);assert.equal(await page.getByRole('button',{name:'选择准确文件更新',exact:true}).isDisabled(),true);
  checks.push('Directory and filename selection fixes one new-file request without sending or mixing an existing-file update.');
  await page.getByLabel('本次消息内容',{exact:true}).fill('新建会议纪要，只记录已核实事项。');
  const[sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);
  assert.equal(sent.status(),202);const body=sent.request().postDataJSON(),run=(await sent.json()).data;
  assert.deepEqual(body.file_create,{folder_id:fixture.folder_id,expected_folder_version:folder.version,name,media_type:'text/markdown'});assert.equal(body.file_update,undefined);assert.equal(run.file_create.status,'authorized');assert.equal(run.file_create.operation_id,null);assert.deepEqual(run.file_create.drafts,[]);
  checks.push('One real Web request pins directory revision, name and format and receives authorization without a formal file.');
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(m=>m.run_id===run.id&&m.role==='user');assert.equal(saved.source_refs[0].type,'agent_file_create');assert.equal(saved.source_refs[0].sha256,run.file_create.intent_sha256);
  checks.push('The sender message stores the exact immutable new-file intent digest.');
  const csrf=(await context.cookies()).find(c=>c.name==='cp_csrf')?.value;
  const repeated=await context.request.post(fixture.origin+'/v1/conversations/'+fixture.conversation_id+'/runs',{headers:{Origin:fixture.origin,'X-CSRF-Token':csrf,'Idempotency-Key':sent.request().headers()['idempotency-key']},data:body});
  assert.equal(repeated.status(),202);assert.equal((await repeated.json()).data.id,run.id);
  checks.push('Rechecking the identical sender request returns the same run and exact creation scope.');
  await page.getByLabel('取消任务的原因',{exact:true}).fill('结束本地新建请求验证');const[cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);
  assert.equal(cancelled.status(),200);assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/files?limit=100&project_id='+fixture.project_id)).items.some(f=>f.name===name),false);assert.equal(sends,1);
  checks.push('Cancelling the queued request produces a real cancellation receipt and creates no formal file.');
},'secretary-file-creates');
