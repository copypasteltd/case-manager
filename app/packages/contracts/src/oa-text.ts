import type { components } from './api';
import type { OABusiness, OAKind, OARequest, OARequestView } from './oa-request';
type S = components['schemas'];
export const oaTextNames: Record<string,string> = { renewal_terms:'续约条款', termination_terms:'终止条款', purpose:'用途说明', notes:'备注' };
export const oaTextFields = (kind:OAKind) => kind==='contract'?['renewal_terms','termination_terms','notes']:['purpose','notes'];
const digest = (v:string) => /^[a-f0-9]{64}$/.test(v);
const uuid = (v:string) => /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v);
const canonical = (v:unknown):string => Array.isArray(v)?'['+v.map(canonical).join(',')+']':v&&typeof v==='object'?'{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonical((v as Record<string,unknown>)[k])).join(',')+'}':JSON.stringify(v);
export function makeOAText(kind:OAKind,b:OABusiness&{can_edit?:boolean},project:string|null,fields:string[],reason:string):OARequest {
 if(!b.can_edit||b.project_id!==project||b.lifecycle!=='draft'||!['not_submitted','returned','withdrawn'].includes(b.approval_state)||!uuid(b.id)||!uuid(b.current_revision_id)||!digest(b.content_hash)||!Number.isSafeInteger(b.version)||b.version<1)throw new Error('请选择当前会话范围内可编辑的准确业务草稿。');
 if(!fields.length||new Set(fields).size!==fields.length||fields.some(f=>!oaTextFields(kind).includes(f)))throw new Error('请选择可起草的正文字段。');
 const why=reason.trim();if(!why||[...why].length>200)throw new Error('请填写1至200字操作说明。');
 return {tool:'draft_business_text',arguments:{kind,business_id:b.id,expected_business_version:b.version,business_revision_id:b.current_revision_id,business_content_hash:b.content_hash,fields:[...fields] as S['AgentOATextArguments']['fields'],reason:why}};
}
export function oaTextAccepted(run:S['Run'],input:OARequest):boolean {
 const v=run.oa_request;if(input.tool!=='draft_business_text'||!v||v.tool!==input.tool||run.mutation||run.post_write||run.file_update||run.file_create||v.run_id!==run.id||!digest(v.intent_sha256)||v.operation_id!==null||v.result!==null||v.request_id!==null||canonical(v.arguments)!==canonical(input.arguments))return false;
 const a=input.arguments,b=v.before.business;
 if(v.kind!==a.kind||v.business_id!==a.business_id||b.id!==a.business_id||b.kind!==a.kind||b.version!==a.expected_business_version||b.current_revision_id!==a.business_revision_id||b.content_hash!==a.business_content_hash)return false;
 return v.status==='authorized'?!v.text_draft:oaTextProven(v);
}
export function oaTextProven(v:OARequestView):boolean {
 const d=v.text_draft,a=v.arguments as S['AgentOATextArguments'];
 if(v.tool!=='draft_business_text'||!d||!Array.isArray(d.source_refs)||!d.source_refs.some(s=>s.type==='agent_oa_request'&&s.id===v.run_id&&s.sha256===v.intent_sha256&&s.resource_version===1)||d.source_refs.some(s=>!uuid(s.id)||!digest(s.sha256)||!Number.isSafeInteger(s.resource_version)||s.resource_version<1)||!digest(d.draft_sha256)||!a.fields?.length||!['drafted','adopted'].includes(v.status)||Object.keys(d.fields).length!==a.fields.length||a.fields.some(f=>!oaTextFields(v.kind).includes(f)||typeof d.fields[f]!=='string'||!d.fields[f].trim()||[...d.fields[f]].length>(f==='purpose'?4000:20000)))return false;
 if(v.status==='drafted')return d.operation_id===null&&d.result===null;
 const r=d.result,b=v.before.business;if(!r||!d.operation_id||!uuid(d.operation_id)||r.id!==b.id||r.kind!==b.kind||r.version!==b.version+1||r.current_revision_id===b.current_revision_id||!digest(r.content_hash)||r.project_id!==b.project_id||r.classification!==b.classification||r.lifecycle!=='draft')return false;
 return canonical(r.payload.content)===canonical({...b.payload.content as Record<string,unknown>,...d.fields});
}
export function oaTextOriginal(v:OARequestView,field:string):string {
 const text=(v.before.business.payload.content as Record<string,unknown>)[field];return typeof text==='string'?text:'';
}
export function makeOATextAdopt(v:OARequestView,reviewed:boolean,reason:string):S['AgentOATextAdopt'] {
 if(!reviewed||v.status!=='drafted'||!oaTextProven(v))throw new Error('请先核对原文与全部正文建议。');
 const why=reason.trim();if(!why||[...why].length>200)throw new Error('请填写1至200字采用依据。');
 return {draft_sha256:v.text_draft!.draft_sha256,reviewed:true,reason:why};
}
