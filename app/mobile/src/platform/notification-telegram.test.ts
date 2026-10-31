import { describe, it } from 'node:test';
import { strictEqual, throws } from 'node:assert';
const expect=(v:unknown)=>({toBe:(other:unknown)=>strictEqual(v,other),toThrow:()=>throws(v as ()=>unknown)});
import { checkedTelegramSettings, checkedTelegramPolicyReceipt, checkedTelegramReconcileReceipt, checkedTelegramDetails, telegramPolicyInput, telegramReconcileInput } from '../../../packages/contracts/src/notification-telegram';
import { personalNoticeFixture } from '../../../packages/contracts/test/notification-telegram-fixture';
describe('personal Telegram notice boundaries',()=>{
  it('rejects response consent for a different Bot or notification set',()=>{
    const {policy}=personalNoticeFixture();const setting={bot_id:policy.bot_id,binding_active:true,effective:true,current:policy};
    const input=telegramPolicyInput(setting,true,['task'],'本人明确选择');
    expect(()=>checkedTelegramPolicyReceipt({...policy,version:3},input,policy.bot_id)).toThrow();
    expect(()=>checkedTelegramSettings({items:[{...setting,bot_id:'987654321'}]})).toThrow();
    expect(()=>telegramPolicyInput({...setting,binding_active:false},true,['task'],'本人明确选择')).toThrow();
  });
  it('never accepts a changed source or reset attempts in a reconciliation receipt',()=>{
    const {receipt}=personalNoticeFixture();const input=telegramReconcileInput(receipt,'retry','','核对后明确重试');
    const returned={...receipt,version:4,state:'retry_wait' as const,reason_code:'human_retry'};
    expect(checkedTelegramReconcileReceipt(returned,receipt,input).id).toBe(receipt.id);
    expect(()=>checkedTelegramReconcileReceipt({...returned,source_delivery_id:receipt.thread_id},receipt,input)).toThrow();
    expect(()=>checkedTelegramReconcileReceipt({...returned,attempts:0},receipt,input)).toThrow();
  });
  it('requires bounded exact message IDs and prevents exhausted retries',()=>{
    const {receipt}=personalNoticeFixture();expect(()=>telegramReconcileInput(receipt,'confirm_sent','9007199254740992','核对本人消息')).toThrow();
    expect(()=>telegramReconcileInput({...receipt,attempts:5},'retry','','核对本人消息')).toThrow();
    expect(telegramReconcileInput(receipt,'confirm_sent','9001','核对本人消息').message_id).toBe(9001);
  });
  it('rejects a truncated or inconsistent immutable receipt journal',()=>{
    const {details}=personalNoticeFixture();expect(checkedTelegramDetails(details).events.length).toBe(3);
    expect(()=>checkedTelegramDetails({...details,events:details.events.slice(1)})).toThrow();
    const altered=structuredClone(details);altered.events[2].reason_code='provider_confirmed';expect(()=>checkedTelegramDetails(altered)).toThrow();
  });
});
