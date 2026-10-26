import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen=page=>page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(page,name)=>screen(page).getByRole('button',{name,exact:true});

await notificationBrowser(true,async({page,context,fixture,checks})=>{
 let token;page.on('request',request=>{const value=request.headers().authorization;if(value)token=value;});
 await page.goto(fixture.origin);
 await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(fixture.alice.login);await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(fixture.alice.password);
 await button(page,'登录工作空间 ↗').click();await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));await button(page,'验证并继续 ↗').click();
 await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();
 await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();
 const messages=page.waitForResponse(response=>response.url().includes('/conversations/'+fixture.conversation_id+'/messages')&&response.request().method()==='GET');
 await page.goto(fixture.origin+'/#/pages/secretary/index?kind=conversation&id='+fixture.conversation_id+'&run='+fixture.completed_run_id);
 const historyResponse=await messages;assert.equal(historyResponse.status(),200);const history=(await historyResponse.json()).data;
 const source=history.items.find(m=>m.role==='assistant'&&m.visibility==='available')?.source_refs.find(s=>s.type==='agent_finance_report'&&s.id===fixture.finance_report_id);assert.ok(source,'real completed finance source must remain visible');
 const opened=page.waitForResponse(response=>new URL(response.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/context-sources/agent_finance_report/'+fixture.finance_report_id);
 await button(page,'查看固定财务快照').first().click();
 try { const response=await opened;assert.equal(response.status(),200,'actual finance reader response');await screen(page).getByText(/captured_at/).waitFor(); }
 catch(error){throw new Error(error.message+'; reader headings='+JSON.stringify(await screen(page).locator('.cp-section-title').allTextContents())+'; notices='+JSON.stringify(await screen(page).locator('.cp-notice').allTextContents()));}
 assert.match(await screen(page).textContent(),/原币种不合并/);
 checks.push({name:'Taro opens the actual completed monthly financial snapshot with its scope notice',passed:true});
 assert.ok(token);const response=await context.request.get(fixture.api_origin+'/v1/conversations/'+fixture.conversation_id+'/context-sources/agent_finance_report/'+fixture.finance_report_id+'?context_security_version=1&max_chars=10000',{headers:{Authorization:token}});
 assert.equal(response.status(),200);const view=(await response.json()).data,content=JSON.parse(view.text);
 assert.equal(view.source_sha256,source.sha256);assert.equal(content.view,'actual');assert.deepEqual(content.report.totals.map(t=>[t.currency,t.net_amount]),[['HKD','100.120000'],['USD','7.010000']]);
 await button(page,'收起财务快照').click();await button(page,'收起财务快照').waitFor({state:'hidden'});
 checks.push({name:'Taro verifies historical money strings against the current authorized source digest and closes the reader',passed:true});
},'finance-summary');
