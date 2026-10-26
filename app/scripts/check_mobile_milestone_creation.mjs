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
  // Click one adjacent visible row per step. Large mouse drags can leave
  // the picker mask before its mouseup handler observes the desired row.
  const direction=Math.sign(desired-current);
  for(let index=current;index!==desired;index+=direction){
    await page.mouse.click(box.x+box.width/2,box.y+box.height/2+direction*item.height);
    await page.waitForTimeout(350);await stable(page);
  }
  await overlay.locator('.weui-picker__action').filter({hasText:/^确定$/}).click();await overlay.waitFor({state:'hidden'});await stable(page);assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'),String(desired),label+' must select '+text);
}

await notificationBrowser(true,async({page,context,fixture,checks})=>{
 let token;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))token=r.headers().authorization;});
 await page.goto(fixture.origin);await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(fixture.alice.login);await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(fixture.alice.password);await button(page,'登录工作空间 ↗').click();await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await button(page,'验证并继续 ↗').click();await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();
 async function request(route){assert.ok(token);const r=await context.request.get(fixture.api_origin+'/v1'+route,{headers:{Authorization:token}});assert.equal(r.status(),200,route);return(await r.json()).data;}
 await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-row-title').getByText('Exact project stage conversation',{exact:true}).click();
 const messages=await request('/conversations/'+fixture.conversation_id+'/messages?limit=100'),message=messages.items.find(m=>m.run_id===fixture.completed_run_id&&m.role==='user');assert.ok(message);
 await screen(page).locator('.cp-editor-row').first().waitFor();
 const titlePattern=new RegExp('^成员消息\\s*·\\s*'+message.seq+'$'),row=screen(page).locator('.cp-editor-row').filter({has:page.locator('.cp-row-title').filter({hasText:titlePattern})});
 for(let n=0;n<20&&await row.count()===0;n++){
  const earlier=button(page,'加载更早消息');assert.equal(await earlier.count(),1,'Completed run message missing: '+message.seq+'; rendered '+JSON.stringify(await screen(page).locator('.cp-editor-row .cp-row-title').allTextContents()));
  await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/messages'),earlier.click()]);await stable(page);
 }assert.equal(await row.count(),1);
 await row.getByRole('button',{name:'查看任务进度',exact:true}).click();await screen(page).getByText('本次更新已提交',{exact:true}).waitFor();
 const completed=await request('/runs/'+fixture.completed_run_id),milestone=await request('/milestones/'+fixture.milestone_id),project=await request('/projects/'+fixture.project_id),members=await request('/projects/'+fixture.project_id+'/members');assert.equal(completed.mutation.status,'committed');assert.equal(completed.mutation.resource_type,'project');assert.equal(completed.mutation.result.id,milestone.id);assert.equal(completed.mutation.result_version,1);const owner=members.items.find(m=>m.id===project.owner_id);assert.ok(owner);checks.push('Taro H5 displays actual committed milestone history after real MFA login.');
 const task=await request('/tasks/'+fixture.task_id);

 let sends=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sends++;});
 await select(page,'本次秘书任务','更新项目 · controlled-test');
 await button(page,'选择准确业务更新').click();await select(page,'本次更新方式','修改任务资料');await button(page,'选择本次更新任务').click();
 await screen(page).locator('.cp-row-title').getByText(task.title+' · 待处理',{exact:true}).click();await select(page,'本次修改字段','关联里程碑');
 await button(page,'选择关联里程碑').click();await screen(page).locator('.cp-row-title').getByText(milestone.title,{exact:true}).click();
 await screen(page).locator('input[aria-label="本次更新原因"]').fill('Explicitly associate this task only');await button(page,'将这项明确更新加入请求').click();assert.equal(sends,0);
 await screen(page).getByText(milestone.id,{exact:true}).last().waitFor();await button(page,'移除本次更新').click();checks.push('Taro selects an actual same-project milestone and previews one task association without sending.');
 await button(page,'选择准确业务更新').click();await select(page,'本次更新方式','创建里程碑');
 await screen(page).locator('input[aria-label="新里程碑标题"]').fill('Mobile reviewed milestone');await select(page,'新里程碑负责人',owner.display_name);
 await screen(page).locator('input[aria-label="新里程碑目标时刻"]').fill('2027-04-06T18:00:00+08:00');await screen(page).locator('input[aria-label="本次更新原因"]').fill('Explicit mobile creation');
 await button(page,'将这项明确更新加入请求').click();assert.equal(sends,0);checks.push('Taro previews the complete title, actual member, zoned due time and parent version before sending.');
 await screen(page).locator('textarea[aria-label="本次消息内容"]').fill('只创建我明确选择的里程碑，不自动关联任何任务。');
 const[sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button(page,'确认发送本次请求').click()]);
 assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;
 assert.deepEqual(input.mutation,{tool:'create_milestone',arguments:{id:project.id,expected_version:project.version,title:'Mobile reviewed milestone',owner_id:owner.id,due_at:'2027-04-06T18:00:00+08:00',reason:'Explicit mobile creation'}});
 assert.equal(run.mutation.status,'authorized');assert.equal(run.mutation.operation_id,null);checks.push('Taro confirmed send persists exactly one complete creation instruction without claiming completion.');
 const replay=await context.request.post(fixture.api_origin+'/v1/conversations/'+fixture.conversation_id+'/runs',{headers:{Authorization:token,'Idempotency-Key':sent.request().headers()['idempotency-key']},data:input});assert.equal(replay.status(),202);assert.equal((await replay.json()).data.id,run.id);
 await button(page,'明确取消这个任务').click();await screen(page).locator('input[aria-label="取消任务的原因"]').fill('End controlled creation check');
 const[cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),button(page,'确认请求取消').click()]);assert.equal(cancelled.status(),200);
 assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/tasks/'+task.id)).version,task.version);assert.equal((await request('/milestones/'+milestone.id)).version,milestone.version);assert.equal(sends,1);checks.push('Taro transport retry preserves the run; cancellation leaves existing milestone and task unchanged.');
},'milestone-creation');
