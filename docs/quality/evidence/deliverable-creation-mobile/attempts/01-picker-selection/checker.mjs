import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen=page=>page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(page,name)=>screen(page).getByRole('button',{name,exact:true});

async function stable(page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});}
async function select(page,label,text){
  const picker=screen(page).locator('taro-picker-core[aria-label="'+label+'"]');await picker.click();const overlay=page.locator('.weui-picker:visible');await overlay.waitFor();const group=overlay.locator('.weui-picker__group');await group.waitFor();
  await group.evaluate(async node=>{if(node.componentOnReady)await node.componentOnReady();});await overlay.evaluate(async node=>{await Promise.all(node.getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})));});await stable(page);
  // Reopening the editor reloads its actual project members. Wait for that
  // response to populate the native wheel before choosing a member.
  await overlay.getByText(text,{exact:true}).waitFor();await stable(page);
  const items=await overlay.locator('.weui-picker__item').allTextContents(),desired=items.indexOf(text);assert.ok(desired>=0,label+' missing '+text);
  const current=Number(await picker.locator('input[type="hidden"]').first().getAttribute('value'))||0,box=await group.boundingBox(),item=await overlay.locator('.weui-picker__item').first().boundingBox();assert.ok(box&&item);
  // One row per drag keeps the mouseup inside the actual wheel mask.
  const direction=Math.sign(desired-current);
  for(let index=current;index!==desired;index+=direction){
    const x=box.x+box.width/2,y=box.y+box.height/2;
    await page.mouse.move(x,y);await page.mouse.down();
    for(let step=1;step<=5;step++){
      await page.mouse.move(x,y-direction*item.height*step/5);
      await stable(page);
    }
    await page.mouse.up();
    await page.waitForTimeout(350);await stable(page);
    const diagnostic=await group.evaluate(node=>{const content=node.querySelector('.weui-picker__content');const matrix=new DOMMatrixReadOnly(getComputedStyle(content).transform);const rect=node.getBoundingClientRect();return{translate:matrix.m42,group:{x:rect.x,y:rect.y,width:rect.width,height:rect.height},viewport:innerHeight,hit:document.elementFromPoint(rect.x+rect.width/2,rect.y+rect.height/2)?.className};});
    assert.equal(Math.round((102-diagnostic.translate)/item.height),index+direction,label+' wheel '+JSON.stringify({current,desired,item,diagnostic}));
  }
  await overlay.locator('.weui-picker__action').filter({hasText:/^确定$/}).click();await overlay.waitFor({state:'hidden'});await stable(page);assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'),String(desired),label+' must select '+text);
}

await notificationBrowser(true,async({page,context,fixture,checks})=>{
 let token;page.on('request',r=>{if(r.headers().authorization?.startsWith('Bearer '))token=r.headers().authorization;});
 await page.goto(fixture.origin);await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(fixture.alice.login);await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(fixture.alice.password);await button(page,'登录工作空间 ↗').click();await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await button(page,'验证并继续 ↗').click();await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();
 async function request(route){assert.ok(token);const r=await context.request.get(fixture.api_origin+'/v1'+route,{headers:{Authorization:token}});assert.equal(r.status(),200,route);return(await r.json()).data;}
 await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();await screen(page).locator('.cp-row-title').getByText('Exact deliverable creation conversation',{exact:true}).click();
 const messages=await request('/conversations/'+fixture.conversation_id+'/messages?limit=100'),message=messages.items.find(m=>m.run_id===fixture.completed_run_id&&m.role==='user');assert.ok(message);
 await screen(page).locator('.cp-editor-row').first().waitFor();
 const titlePattern=new RegExp('^成员消息\\s*·\\s*'+message.seq+'$'),row=screen(page).locator('.cp-editor-row').filter({has:page.locator('.cp-row-title').filter({hasText:titlePattern})});
 for(let n=0;n<20&&await row.count()===0;n++){
  const earlier=button(page,'加载更早消息');assert.equal(await earlier.count(),1,'Completed run message missing: '+message.seq+'; rendered '+JSON.stringify(await screen(page).locator('.cp-editor-row .cp-row-title').allTextContents()));
  await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/messages'),earlier.click()]);await stable(page);
 }assert.equal(await row.count(),1);
 await row.getByRole('button',{name:'查看任务进度',exact:true}).click();await screen(page).getByText('本次更新已提交',{exact:true}).waitFor();

 const completed=await request('/runs/'+fixture.completed_run_id),delivery=await request('/deliverables/'+fixture.deliverable_id),project=await request('/projects/'+fixture.project_id),milestone=await request('/milestones/'+fixture.milestone_id),members=await request('/projects/'+fixture.project_id+'/members');assert.equal(completed.mutation.resource_type,'project');assert.equal(completed.mutation.resource_id,project.id);assert.equal(completed.mutation.result.version,1);assert.equal(delivery.state,'accepted');assert.ok(delivery.current_revision_id);assert.equal(completed.mutation.result.id,delivery.id);assert.equal(completed.mutation.status,'committed');assert.equal(completed.mutation.result.revision_id,null);assert.equal(completed.outcome,'completed');checks.push('Taro reads a genuine creation receipt and its independently accepted human revision after actual MFA login.');
 const owner=members.items.find(m=>m.id===delivery.owner_id&&m.status==='active');assert.ok(owner);
 const initial=await request('/projects/'+project.id+'/deliverables?limit=20');assert.equal(initial.next_cursor,null);const before=initial.items.map(item=>item.id).sort();
 let sends=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sends++;});
 await select(page,'本次秘书任务','更新项目 · controlled-test');await button(page,'选择准确业务更新').click();await select(page,'本次更新方式','创建交付草稿');
 await screen(page).locator('input[aria-label="交付名称"]').fill('Mobile planned delivery');await screen(page).locator('textarea[aria-label="验收准则"]').fill('Check every actual fixed result.');await select(page,'交付负责人',owner.display_name);await screen(page).locator('input[aria-label="本次更新原因"]').fill('Create complete mobile draft metadata');await button(page,'将这项明确更新加入请求').click();assert.equal(sends,0);await screen(page).getByText('清空',{exact:true}).last().waitFor();checks.push('Taro previews a complete new draft with explicit null without creating it.');
 await button(page,'移除本次更新').click();await button(page,'选择准确业务更新').click();
 await screen(page).locator('input[aria-label="交付名称"]').fill('Mobile planned delivery');await screen(page).locator('textarea[aria-label="验收准则"]').fill('Check every actual fixed result.');await select(page,'交付负责人',owner.display_name);await button(page,'选择交付关联里程碑').click();await screen(page).locator('.cp-row-title').getByText(milestone.title,{exact:true}).click();await screen(page).locator('input[aria-label="本次更新原因"]').fill('Create complete mobile draft metadata');await button(page,'将这项明确更新加入请求').click();await screen(page).getByText(milestone.id,{exact:true}).last().waitFor();assert.equal(sends,0);checks.push('Taro selects a real same-project milestone and retains every reviewed creation field.');
 await screen(page).locator('textarea[aria-label="本次消息内容"]').fill('仅创建已核对的交付草稿，不提交或验收。');const[sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),button(page,'确认发送本次请求').click()]);assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;
 assert.deepEqual(input.mutation,{tool:'create_deliverable',arguments:{id:project.id,expected_version:project.version,name:'Mobile planned delivery',acceptance_criteria:'Check every actual fixed result.',owner_id:owner.id,milestone_id:milestone.id,reason:'Create complete mobile draft metadata'}});assert.equal(run.mutation.operation_id,null);assert.equal(run.mutation.status,'authorized');checks.push('Taro send fixes the exact parent version and complete new-draft instruction.');
 const replay=await context.request.post(fixture.api_origin+'/v1/conversations/'+fixture.conversation_id+'/runs',{headers:{Authorization:token,'Idempotency-Key':sent.request().headers()['idempotency-key']},data:input});assert.equal(replay.status(),202);assert.equal((await replay.json()).data.id,run.id);await button(page,'明确取消这个任务').click();await screen(page).locator('input[aria-label="取消任务的原因"]').fill('End controlled metadata check');const[cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),button(page,'确认请求取消').click()]);assert.equal(cancelled.status(),200);assert.equal((await request('/runs/'+run.id)).state,'cancelled');const after=await request('/projects/'+project.id+'/deliverables?limit=20');assert.equal(after.next_cursor,null);assert.deepEqual(after.items.map(item=>item.id).sort(),before);assert.equal(sends,1);checks.push('Taro retry preserves the run and cancellation creates no additional draft.');
},'deliverable-creation');
