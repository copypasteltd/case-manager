import type { components } from './api';
type S=components['schemas'];
export type FixedAuthorization=Pick<S['RunCreate'],'scope_transfer'|'notification'>;
export const fixedDeliveryNotice='只执行本次核对的准确计划。范围变更将终止旧秘书授权；通知仅进入原群投递队列，实际送达需按投递账本核对。';
export function scopeAuthorization(p:S['ScopeTransfer'],reason:string,reviewed:boolean):FixedAuthorization{
 if(!reviewed||!reason.trim()||[...reason].length>200||p.state!=='pending'||!p.can_execute||!p.current_basis_matches||Date.parse(p.expires_at)<=Date.now()||p.snapshot.items.length<1||p.snapshot.items.length>100)throw new Error('请完整核对当前可执行范围计划、可见性清单和执行原因。');
 return {scope_transfer:{plan_id:p.id,expected_version:p.version,plan_hash:p.plan_hash,reviewed:true,authorized:true,reason}};
}
export function notificationFragment(p:S['TelegramDisclosurePreview'],start:string,max:string):string{
 if(!/^\d+$/.test(start)||!/^\d+$/.test(max))throw new Error('片段位置与长度必须为完整整数。');
 const a=Number(start),n=Number(max),body=[...p.body];
 if(!Number.isSafeInteger(a)||!Number.isSafeInteger(n)||n<1||n>2000||a<0||a>body.length||n>body.length-a||body.length!==p.total_chars)throw new Error('请选择准确结果中最多2000字的完整片段。');
 return body.slice(a,a+n).join('');
}
export function notificationAuthorization(p:S['TelegramDisclosurePreview'],start:string,max:string,reviewed:boolean):FixedAuthorization{
 notificationFragment(p,start,max);if(!reviewed)throw new Error('请明确核对将公开到原群的准确片段。');
 return {notification:{request_id:p.request_id,expected_version:p.run_version,body_sha256:p.body_sha256,start_char:Number(start),max_chars:Number(max),reviewed:true,disclosed_to_group:true}};
}
export function fixedDeliveryAccepted(run:S['Run'],input:S['RunCreate']):boolean{
 const expected=input.scope_transfer??input.notification;
 if(!expected)return !run.fixed_delivery;
 const actual=run.fixed_delivery;
 if(!actual||actual.run_id!==run.id||actual.kind!==(input.scope_transfer?'scope_transfer':'notification')||!actual.input||typeof actual.input!=='object')return false;
 return Object.entries(expected).every(([key,value])=>JSON.stringify((actual.input as Record<string,unknown>)[key])===JSON.stringify(value))&&Object.keys(actual.input).length===Object.keys(expected).length;
}
