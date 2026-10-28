import { oaTextAccepted } from './oa-text';
import type { components } from './api';
type S = components['schemas'];
export type OARequest = S['AgentOARequestInput'];
export type OARequestView = S['AgentOARequestView'];
export type OAKind = 'contract' | 'purchase' | 'claim';
export type OABusiness = Pick<S['Contract'], 'id'|'title'|'version'|'current_revision_id'|'content_hash'|'project_id'|'request_id'|'can_prepare_request'|'lifecycle'|'approval_state'>;
export const oaKindNames: Record<OAKind,string> = {contract:'合同',purchase:'采购',claim:'报销'};
export const oaPaths: Record<OAKind,string> = {contract:'/contracts',purchase:'/purchases',claim:'/claims'};
const id = (v:string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v);
const hash = (v:string) => /^[0-9a-f]{64}$/.test(v);
function reasonText(v:string) { if(!v.trim() || [...v.trim()].length>200) throw new Error('请填写1至200字操作说明。'); return v.trim(); }
function target(b:OABusiness, project:string|null) { if(b.project_id!==project||!id(b.id)||!id(b.current_revision_id)||!hash(b.content_hash)||!Number.isSafeInteger(b.version)||b.version<1) throw new Error('请选择当前会话范围内的准确业务修订。'); }
export function makeOADraft(kind:OAKind,b:OABusiness,r:S['Request']|undefined,project:string|null,reason:string):OARequest {
 target(b,project);
 if(!b.can_prepare_request || !['draft'].includes(b.lifecycle) || !['not_submitted','returned','withdrawn','rejected'].includes(b.approval_state)) throw new Error('此业务当前不能准备申请。');
 if(b.request_id && (!r||r.id!==b.request_id||r.business_id!==b.id||r.kind!==kind||!r.can_revise||!['draft','returned','withdrawn'].includes(r.state))) throw new Error('请先核对本人可修订的准确申请。');
 if(!b.request_id && r) throw new Error('申请与业务关联已变化，请重新选择。');
 return {tool:'draft_request',arguments:{kind,business_id:b.id,expected_business_version:b.version,business_revision_id:b.current_revision_id,business_content_hash:b.content_hash,reason:reasonText(reason),...(r?{request_id:r.id,expected_version:r.version}:{})}};
}
export function makeOASubmit(kind:OAKind,b:OABusiness,r:S['Request'],p:S['SubmissionPreview'],project:string|null,reason:string):OARequest {
 target(b,project);
 if(r.id!==b.request_id||r.business_id!==b.id||r.kind!==kind||r.project_id!==project||!r.can_submit||p.request_id!==r.id||p.request_version!==r.version||p.revision_id!==r.current_revision_id||p.content_hash!==r.content_hash||p.payload.business_revision_id!==b.current_revision_id||p.payload.business_content_hash!==b.content_hash||!hash(p.preview_hash)||!id(p.workflow.id)||!p.workflow.steps.length) throw new Error('申请、业务修订或审批预览已变化，请重新核对。');
 return {tool:'submit_request',arguments:{request_id:r.id,expected_version:r.version,revision_id:r.current_revision_id,content_hash:r.content_hash,workflow_definition_id:p.workflow.id,preview_hash:p.preview_hash,reason:reasonText(reason)}};
}
export function oaRequestAccepted(run:S['Run'],input?:OARequest):boolean {
 const v=run.oa_request;
 if(!input)return !v;
 if(input.tool==='draft_business_text')return oaTextAccepted(run,input);
 if(!v||run.mutation||run.file_update||run.file_create||v.run_id!==run.id||v.tool!==input.tool||!hash(v.intent_sha256)||Object.keys(input.arguments).length!==Object.keys(v.arguments).length||Object.entries(input.arguments).some(([k,value])=>(v.arguments as Record<string,unknown>)[k]!==value))return false;
 if(v.before.business.id!==v.business_id||v.before.business.kind!==v.kind)return false;
 if(input.tool==='draft_request'&&(v.kind!==input.arguments.kind||v.business_id!==input.arguments.business_id||v.request_id!==(input.arguments.request_id??null)))return false;
 if(input.tool==='submit_request'&&v.request_id!==input.arguments.request_id)return false;
 return v.status==='authorized'?v.operation_id===null&&v.result===null:oaRequestReceipt(run,input);
}
export function oaRequestReceipt(run:S['Run'],input?:OARequest):boolean {
 const v=run.oa_request, r=v?.result;
 if(v?.tool==='draft_business_text')return false;
 if(!v||!r||v.run_id!==run.id||v.status!=='committed'||!hash(v.intent_sha256)||!v.operation_id||!id(v.operation_id)||r.kind!==v.kind||r.business_id!==v.business_id||r.applicant_id!==run.actor_id||r.project_id!==v.before.business.project_id||r.payload.business_id!==v.business_id||r.payload.business_revision_id!==v.before.business.current_revision_id||r.payload.business_content_hash!==v.before.business.content_hash||r.version!==((v.arguments as {expected_version?:number}).expected_version??0)+1) return false;
 if(input&&(input.tool!==v.tool||Object.entries(input.arguments).some(([k,value])=>(v.arguments as Record<string,unknown>)[k]!==value))) return false;
 if(v.tool==='draft_request') return r.state==='draft'&&r.active_instance_id===null&&(!v.request_id||v.request_id===r.id);
 const a=v.arguments as S['AgentOASubmitArguments'];
 return r.id===a.request_id&&r.state==='in_review'&&!!r.active_instance_id&&r.workflow?.id===a.workflow_definition_id&&!!r.steps.length&&r.steps.every(step=>step.instance_id===r.active_instance_id&&step.request_id===r.id&&step.revision_id===r.current_revision_id)&&r.steps.some(step=>step.state==='active');
}
