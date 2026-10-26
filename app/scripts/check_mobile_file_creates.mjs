import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen=page=>page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(page,name)=>screen(page).getByRole('button',{name,exact:true});

async function stable(page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});}
async function select(page,label,text){
  const picker=screen(page).locator('taro-picker-core[aria-label="'+label+'"]');await picker.click();const overlay=page.locator('.weui-picker:visible');await overlay.waitFor();const group=overlay.locator('.weui-picker__group');await group.waitFor();
  await group.evaluate(async node=>{if(node.componentOnReady)await node.componentOnReady();});await overlay.evaluate(async node=>{await Promise.all(node.getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})));});await stable(page);
  const items=await overlay.locator('.weui-picker__item').allTextContents(),desired=items.indexOf(text);assert.ok(desired>=0,label+' missing '+text);
  const current=Number(await picker.locator('input[type="hidden"]').first().getAttribute('value'))||0,box=await group.boundingBox(),item=await overlay.locator('.weui-picker__item').first().boundingBox();assert.ok(box&&item);
  if(desired!==current){const x=box.x+box.width/2,y=box.y+box.height/2;await page.mouse.move(x,y);await page.mouse.down();for(let step=1;step<=8;step++){await page.mouse.move(x,y-(desired-current)*item.height*step/8);await stable(page);}await page.mouse.up();await stable(page);}
  await overlay.locator('.weui-picker__action').filter({hasText:/^确定$/}).click();await overlay.waitFor({state:'hidden'});await stable(page);assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'),String(desired));
}

await notificationBrowser(true,async({page,context,fixture,checks})=>{
  let token;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))token=r.headers().authorization;});
  await page.goto(fixture.origin);await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(fixture.alice.login);await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(fixture.alice.password);await button(page,'登录工作空间 ↗').click();
  await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await button(page,'验证并继续 ↗').click();await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();
  await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-row-title').getByText('准确文件新建',{exact:true}).click();
  async function request(route){assert.ok(token);const response=await context.request.get(fixture.api_origin+'/v1'+route,{headers:{Authorization:token}});assert.equal(response.status(),200);return(await response.json()).data;}
  await button(page,'查看任务进度').first().click();await screen(page).getByText('文件新建实际结果',{exact:true}).waitFor();
  const original=await request('/runs/'+fixture.completed_run_id);assert.equal(original.file_create.status,'committed');assert.equal(original.file_create.result.file_version.seq,1);assert.equal(original.file_create.result.file_version.base_version_id,null);assert.match(await screen(page).innerText(),/香港🙂：明确下一项工作/);
  checks.push('Taro H5 reads the real first formal file version and renders the generated text and creation receipt.');
  assert.equal(original.file_create.result.file_version.replicas[1].state,'queued');await screen(page).getByText('第二存储副本按文件详情的实际复制校验状态显示。',{exact:true}).waitFor();await button(page,'收起任务进度').click();
  checks.push('Mobile shows the actual pending second replica and does not present staging as a formal file.');
  let sends=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sends++;});
  await select(page,'本次秘书任务','新建或更新文件 · controlled-test');await button(page,'选择新建文件位置').click();const folder=await request('/folders/'+fixture.folder_id);
  await screen(page).locator('.cp-row-title').getByText(folder.name,{exact:true}).click();await screen(page).getByText(/目标：.+修订/).waitFor();const name='移动端待生成项目说明.md';
  await screen(page).getByRole('textbox',{name:'新文件名称',exact:true}).fill(name);await button(page,'将准确文件新建加入请求').click();assert.equal(sends,0);assert.equal(await button(page,'选择准确文件更新').isDisabled(),true);
  checks.push('Mobile explicitly chooses an authorized directory and new filename without sending or mixing an update.');
  await screen(page).getByRole('textbox',{name:'本次消息内容',exact:true}).fill('新建项目说明，保留已核实的事实与待补项。');
  const[sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button(page,'确认发送本次请求').click()]);
  assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;assert.deepEqual(input.file_create,{folder_id:folder.id,expected_folder_version:folder.version,name,media_type:'text/markdown'});assert.equal(input.file_update,undefined);assert.equal(run.file_create.operation_id,null);assert.equal(run.file_create.status,'authorized');
  checks.push('One actual mobile request fixes directory revision, filename and format while leaving the new file uncommitted.');
  const saved=(await request('/conversations/'+fixture.conversation_id+'/messages?limit=100')).items.find(m=>m.run_id===run.id&&m.role==='user');assert.equal(saved.source_refs[0].type,'agent_file_create');assert.equal(saved.source_refs[0].sha256,run.file_create.intent_sha256);
  checks.push('The mobile sender message stores the original creation intent digest.');
  await button(page,'明确取消这个任务').click();await screen(page).getByRole('textbox',{name:'取消任务的原因',exact:true}).fill('取消本地新建请求验证');const[cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),button(page,'确认请求取消').click()]);
  assert.equal(cancelled.status(),200);assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/files?limit=100&project_id='+fixture.project_id)).items.some(f=>f.name===name),false);assert.equal(sends,1);
  checks.push('Mobile cancellation prevents a formal new file while retaining the prior completed file and its history.');
},'secretary-file-creates');
