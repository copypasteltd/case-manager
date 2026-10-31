import type { components } from './api';
import type { OABusiness, OAKind, OARequest, OARequestView } from './oa-request';
type S = components['schemas'];
export const oaTextNames: Record<string,string> = { renewal_terms:'续约条款', termination_terms:'终止条款', purpose:'用途说明', notes:'备注' };
const linePattern = /^line:([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}):description$/;
const object = (v:unknown):Record<string,unknown> => v&&typeof v==='object'&&!Array.isArray(v)?v as Record<string,unknown>:{};
const lines = (content:unknown) => {const values=object(content).lines;return Array.isArray(values)?values.map(object):[];};
export function oaTextFields(kind:OAKind,content?:unknown):string[] {
 const base=kind==='contract'?['renewal_terms','termination_terms','notes']:['purpose','notes'];
 if(kind==='contract')return base;
 const entries=lines(content);
 if(entries.length>100)return base;
 return [...base,...entries.filter(l=>typeof l.id==='string'&&uuid(l.id)&&entries.filter(v=>v.id===l.id).length===1).map(l=>'line:'+l.id+':description')];
}
export function oaTextName(field:string,content?:unknown):string {
 if(oaTextNames[field])return oaTextNames[field];
 const id=linePattern.exec(field)?.[1],entries=lines(content),index=entries.findIndex(l=>l.id===id);
 if(!id||index<0)return '明细说明（需重新核对）';
 const text=entries[index].description;
 return '明细 '+(index+1)+' · '+(typeof text==='string'&&text.trim()?[...text.trim()].slice(0,60).join(''):'未填写说明');
}
function proseLimit(field:string) {return linePattern.test(field)?1000:field==='purpose'?4000:20000;}
function patchContent(content:unknown,fields:Record<string,string>):Record<string,unknown> {
 const result={...object(content)};
 for(const [field,text] of Object.entries(fields)) {
  const id=linePattern.exec(field)?.[1];
  if(id)result.lines=lines(result).map(l=>l.id===id?{...l,description:text}:l);
  else result[field]=text;
 }
 return result;
}
const digest = (v:string) => /^[a-f0-9]{64}$/.test(v);
const uuid = (v:string) => /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v);
const canonical = (v:unknown):string => Array.isArray(v)?'['+v.map(canonical).join(',')+']':v&&typeof v==='object'?'{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonical((v as Record<string,unknown>)[k])).join(',')+'}':JSON.stringify(v);
export function makeOAText(kind:OAKind,b:OABusiness&{can_edit?:boolean},project:string|null,fields:string[],reason:string):OARequest {
 if(!b.can_edit||b.project_id!==project||b.lifecycle!=='draft'||!['not_submitted','returned','withdrawn'].includes(b.approval_state)||!uuid(b.id)||!uuid(b.current_revision_id)||!digest(b.content_hash)||!Number.isSafeInteger(b.version)||b.version<1)throw new Error('请选择当前会话范围内可编辑的准确业务草稿。');
 if(!fields.length||fields.length>102||new Set(fields).size!==fields.length||fields.some(f=>!oaTextFields(kind,b).includes(f)))throw new Error('请选择可起草的正文字段。');
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
 if(v.tool!=='draft_business_text'||!d||!Array.isArray(d.source_refs)||!d.source_refs.some(s=>s.type==='agent_oa_request'&&s.id===v.run_id&&s.sha256===v.intent_sha256&&s.resource_version===1)||d.source_refs.some(s=>!uuid(s.id)||!digest(s.sha256)||!Number.isSafeInteger(s.resource_version)||s.resource_version<1)||!digest(d.draft_sha256)||!a.fields?.length||a.fields.length>102||new Set(a.fields).size!==a.fields.length||!['drafted','adopted'].includes(v.status)||Object.keys(d.fields).length!==a.fields.length||a.fields.some(f=>!oaTextFields(v.kind,v.before.business.payload.content).includes(f)||typeof d.fields[f]!=='string'||!d.fields[f].trim()||d.fields[f].includes('\0')||[...d.fields[f]].length>proseLimit(f)||((f==='purpose'||linePattern.test(f))&&d.fields[f]!==d.fields[f].trim())))return false;
 if(v.status==='drafted')return d.operation_id===null&&d.result===null;
 const r=d.result,b=v.before.business;if(!r||!d.operation_id||!uuid(d.operation_id)||r.id!==b.id||r.kind!==b.kind||r.version!==b.version+1||r.current_revision_id===b.current_revision_id||!digest(r.content_hash)||r.project_id!==b.project_id||r.classification!==b.classification||r.lifecycle!=='draft')return false;
 return canonical(r.payload.content)===canonical(patchContent(b.payload.content,d.fields));
}
export function oaTextOriginal(v:OARequestView,field:string):string {
 const content=v.before.business.payload.content,id=linePattern.exec(field)?.[1];
 const text=id?lines(content).find(l=>l.id===id)?.description:object(content)[field];return typeof text==='string'?text:'';
}
export function makeOATextAdopt(v:OARequestView,reviewed:boolean,reason:string):S['AgentOATextAdopt'] {
 if(!reviewed||v.status!=='drafted'||!oaTextProven(v))throw new Error('请先核对原文与全部正文建议。');
 const why=reason.trim();if(!why||[...why].length>200)throw new Error('请填写1至200字采用依据。');
 return {draft_sha256:v.text_draft!.draft_sha256,reviewed:true,reason:why};
}
