import type { TelegramPolicy, TelegramReceipt, TelegramDetails } from '../src/notification-telegram';
const id=(n:number)=>'10000000-0000-4000-8000-'+String(n).padStart(12,'0');
export function personalNoticeFixture(){
  const created_at='2026-10-31T01:00:00Z';
  const policy:TelegramPolicy={id:id(1),bot_id:'123456789',version:2,enabled:true,categories:['approval','task'],binding_id:id(2),binding_version:1,reason:'Synthetic employee consent',created_at};
  const receipt:TelegramReceipt={id:id(3),bot_id:policy.bot_id,source_delivery_id:id(4),thread_id:id(5),policy_id:policy.id,state:'delivery_unknown',version:3,attempts:1,reason_code:'provider_outcome_unknown',next_attempt_at:created_at,payload_sha256:'a'.repeat(64),message_id:null,created_at,updated_at:created_at};
  const details:TelegramDetails={receipt,events:[
    {id:id(6),version:1,state:'pending',attempts:0,reason_code:'queued',payload_sha256:null,message_id:null,actor_id:null,reason:null,created_at},
    {id:id(7),version:2,state:'sending',attempts:1,reason_code:'attempt_prepared',payload_sha256:receipt.payload_sha256,message_id:null,actor_id:null,reason:null,created_at},
    {id:id(8),version:3,state:receipt.state,attempts:1,reason_code:receipt.reason_code,payload_sha256:receipt.payload_sha256,message_id:null,actor_id:null,reason:null,created_at},
  ]};
  return {policy,receipt,details};
}
