import assert from 'node:assert/strict';
import { notificationBrowser, waitOTP } from './notification_browser_common.mjs';
const screen=page=>page.locator('.taro_page_show.taro_page_stationed:not(.taro_page_shade) .cp-page').last();
const button=(page,name)=>screen(page).getByRole('button',{name,exact:true});
async function stable(page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});}

await notificationBrowser(true,async({page,context,fixture,checks})=>{
 let token;page.on('request',request=>{if(request.headers().authorization?.startsWith('Bearer '))token=request.headers().authorization;});
 await page.goto(fixture.origin);
 await screen(page).getByRole('textbox',{name:'内部账号',exact:true}).fill(fixture.alice.login);
 await screen(page).getByRole('textbox',{name:'密码',exact:true}).fill(fixture.alice.password);
 await button(page,'登录工作空间 ↗').click();
 await screen(page).locator('input[aria-label="六位验证码"]').fill(await waitOTP(fixture));
 await button(page,'验证并继续 ↗').click();
 await screen(page).locator('.cp-title').filter({hasText:'工作从这里开始'}).waitFor();
 await screen(page).locator('.cp-row-title').getByText('公司秘书',{exact:true}).click();
 const [messages]=await Promise.all([page.waitForResponse(response=>new URL(response.url()).pathname==='/v1/conversations/'+fixture.conversation_id+'/messages'),page.goto(fixture.origin+'/#/pages/secretary/index?kind=conversation&id='+fixture.conversation_id+'&run='+fixture.completed_run_id)]);
 assert.equal(messages.status(),200,'actual message history request');
 const history=(await messages.json()).data;
 assert.ok(history.items.some(message=>message.role==='assistant'&&message.visibility==='available'&&message.source_refs.some(source=>source.type==='service'&&source.version_id===fixture.service_report_id)),'completed report must remain visible under current source authority');
 await button(page,'查看固定状态快照').click();
 await screen(page).getByText(/captured_at/).waitFor();
 assert.ok(token);const response=await context.request.get(fixture.api_origin+'/v1/conversations/'+fixture.conversation_id+'/context-sources/service/'+fixture.service_id+'?context_security_version=1&max_chars=10000&version_id='+fixture.service_report_id,{headers:{Authorization:token}});
 assert.equal(response.status(),200);const view=(await response.json()).data,content=JSON.parse(view.text);
 assert.equal(view.version_id,fixture.service_report_id);assert.equal(content.state_as_of_capture.observed_state,'down');assert.ok(!view.text.includes('DO_NOT_DISCLOSE'));
 checks.push({name:'Taro opens the real immutable service report with its original failure facts',passed:true});
 await button(page,'收起状态快照').click();
 await button(page,'核对并选择上下文资料').click();
 await select(page,'上下文资料类型','服务状态资料');
 await screen(page).locator('.cp-row-title').getByText('Summary business API',{exact:true}).click();
 await button(page,'核对并预览资料').click();
 await button(page,'将这份准确资料加入本次请求').waitFor();
 checks.push({name:'Taro service picker uses the actual permission checked source endpoint',passed:true});
},'service-summary');

async function select(page, label, text) {
  const picker = screen(page).locator('taro-picker-core[aria-label="' + label + '"]'); await picker.click(); const overlay = page.locator('.weui-picker:visible'); await overlay.waitFor();
  const group = overlay.locator('.weui-picker__group'); await group.waitFor();
  // The panel animates in from outside the viewport. Wait for its actual
  // hydration and animation before measuring and using the real mouse wheel.
  await group.evaluate(async node => { if (node.componentOnReady) await node.componentOnReady(); });
  await overlay.evaluate(async node => { await Promise.all(node.getAnimations({ subtree: true }).map(animation => animation.finished.catch(() => {}))); });
  await stable(page); const items = await overlay.locator('.weui-picker__item').allTextContents(), desired = items.indexOf(text); assert.ok(desired >= 0, label + ' missing ' + text);
  const current = Number(await picker.locator('input[type="hidden"]').first().getAttribute('value')) || 0, box = await group.boundingBox(), item = await overlay.locator('.weui-picker__item').first().boundingBox(); assert.ok(box && item);
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  if (desired !== current) { await page.mouse.move(x, y); await page.mouse.down(); for (let step = 1; step <= 8; step++) { await page.mouse.move(x, y - (desired - current) * item.height * step / 8); await stable(page); } await page.mouse.up(); await stable(page); }
  await overlay.locator('.weui-picker__action').filter({ hasText: /^确定$/ }).click(); await overlay.waitFor({ state: 'hidden' }); await stable(page); assert.equal(await picker.locator('input[type="hidden"]').first().getAttribute('value'), String(desired), label + ' did not select ' + text);
}
