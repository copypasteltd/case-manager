import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
await notificationBrowser(false,async({page,context,fixture,checks})=>{
 await page.goto(fixture.origin);await page.getByLabel('内部账号',{exact:true}).fill(fixture.alice.login);await page.getByLabel('密码',{exact:true}).fill(fixture.alice.password);await page.getByRole('button',{name:'登录工作空间',exact:true}).click();await page.getByLabel('六位验证码').fill(await waitOTP(fixture));await page.getByRole('button',{name:'验证并继续',exact:true}).click();await page.getByRole('heading',{name:/工作从这里开始/}).waitFor();
 async function request(route){const r=await context.request.get(fixture.origin+'/v1'+route);assert.equal(r.status(),200,route);return(await r.json()).data;}
 await page.goto(fixture.origin+'/conversations/'+fixture.conversation_id+'?run='+fixture.completed_run_id);await page.getByRole('heading',{name:'本次更新已提交',exact:true}).waitFor();
 const completed=await request('/runs/'+fixture.completed_run_id),milestone=await request('/milestones/'+fixture.milestone_id),project=await request('/projects/'+fixture.project_id);assert.equal(completed.mutation.resource_type,'project');assert.equal(completed.mutation.result.id,milestone.id);assert.equal(completed.mutation.result_version,1);assert.equal(completed.mutation.status,'committed');assert.equal(completed.outcome,'completed');checks.push('Web displays a genuine created milestone ID, child version and original operation receipt.');
 const members=await request('/projects/'+fixture.project_id+'/members'),owner=members.items.find(item=>item.id===project.owner_id),task=await request('/tasks/'+fixture.task_id);assert.ok(owner);

 let sends=0;page.on('request',r=>{if(new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.method()==='POST')sends++;});
 await page.getByLabel('本次秘书任务',{exact:true}).selectOption(fixture.profile_id);
 await page.getByRole('button',{name:'选择准确业务更新',exact:true}).click();await page.getByLabel('本次更新方式',{exact:true}).selectOption('update_task');
 await page.getByLabel('本次更新任务',{exact:true}).selectOption(task.id);await page.getByLabel('本次修改字段',{exact:true}).selectOption('milestone_id');await page.getByLabel('关联里程碑的新值',{exact:true}).selectOption(milestone.id);
 await page.getByLabel('本次更新原因',{exact:true}).fill('Explicitly associate this task only');await page.getByRole('button',{name:'将这项明确更新加入请求',exact:true}).click();assert.equal(sends,0);
 await page.getByText('关联里程碑 → '+milestone.id,{exact:true}).waitFor();await page.getByRole('button',{name:'移除本次更新',exact:true}).click();checks.push('Web selects an actual same-project milestone and previews one task association without sending.');
 await page.getByRole('button',{name:'选择准确业务更新',exact:true}).click();await page.getByLabel('本次更新方式',{exact:true}).selectOption('create_milestone');
 await page.getByLabel('新里程碑标题',{exact:true}).fill('Web reviewed milestone');await page.getByLabel('新里程碑负责人',{exact:true}).selectOption(owner.id);await page.getByLabel('新里程碑目标时刻',{exact:true}).fill('2027-04-06T18:00:00+08:00');await page.getByLabel('本次更新原因',{exact:true}).fill('Explicit Web creation');
 await page.getByRole('button',{name:'将这项明确更新加入请求',exact:true}).click();assert.equal(sends,0);checks.push('Web previews complete creation fields and the parent project version before sending.');
 await page.getByLabel('本次消息内容',{exact:true}).fill('只创建我明确选择的里程碑，不自动关联任何任务。');
 const[sent]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/runs'&&r.request().method()==='POST'),page.getByRole('button',{name:'确认发送本次请求',exact:true}).click()]);
 assert.equal(sent.status(),202);const input=sent.request().postDataJSON(),run=(await sent.json()).data;
 assert.deepEqual(input.mutation,{tool:'create_milestone',arguments:{id:project.id,expected_version:project.version,title:'Web reviewed milestone',owner_id:owner.id,due_at:'2027-04-06T18:00:00+08:00',reason:'Explicit Web creation'}});
 assert.equal(run.mutation.status,'authorized');assert.equal(run.mutation.operation_id,null);checks.push('Web confirmed send persists the complete exact creation instruction with no completion claim.');
 const csrf=(await context.cookies()).find(c=>c.name==='cp_csrf')?.value,replay=await context.request.post(fixture.origin+'/v1/conversations/'+fixture.conversation_id+'/runs',{headers:{Origin:fixture.origin,'X-CSRF-Token':csrf,'Idempotency-Key':sent.request().headers()['idempotency-key']},data:input});assert.equal(replay.status(),202);assert.equal((await replay.json()).data.id,run.id);
 await page.getByLabel('取消任务的原因',{exact:true}).fill('End controlled creation check');const[cancelled]=await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/runs/'+run.id+'/cancel'),page.getByRole('button',{name:'确认请求取消',exact:true}).click()]);assert.equal(cancelled.status(),200);
 assert.equal((await request('/runs/'+run.id)).state,'cancelled');assert.equal((await request('/tasks/'+task.id)).version,task.version);assert.equal((await request('/milestones/'+milestone.id)).version,milestone.version);assert.equal(sends,1);checks.push('Web retry preserves the run; cancellation leaves the existing milestone and task unchanged.');
},'milestone-creation');
