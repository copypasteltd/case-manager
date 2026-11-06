import type {components} from './api';
type S=components['schemas'];export type BusinessDraft=S['AgentBusinessDraftInput'];export type DraftKind=BusinessDraft['kind'];
export const draftNames:Record<DraftKind,string>={contract:'合同',purchase:'采购',claim:'报销',subscription:'订阅'};
const uuid=(v:string)=>/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v),hash=(v:string)=>/^[a-f0-9]{64}$/.test(v);
const canonical=(v:unknown):string=>Array.isArray(v)?'['+v.map(canonical).join(',')+']':v&&typeof v==='object'?'{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonical((v as Record<string,unknown>)[k])).join(',')+'}':JSON.stringify(v);
export function makeBusinessDraft(c:S['Conversation'],actor:string,kind:DraftKind,title:string,notes:string,reason:string,key:string):BusinessDraft{
 title=title.trim();reason=reason.trim();if(c.type!=='private'||c.creator_id!==actor||c.classification==='confidential'||!uuid(key)||!draftNames[kind]||!title||[...title].length>200||!reason||[...reason].length>200||notes.includes('\0')||[...notes].length>20000)throw new Error('请在本人非机密私聊中填写草稿标题、正文和1至200字操作依据。');
 const scope={project_id:c.project_id,classification:c.classification,reason};
 if(kind==='contract')return{kind,arguments:{...scope,title,direction:null,type:null,category:'',counterparty_id:null,counterparty_revision_id:null,owner_id:actor,currency:null,amount:null,tax_amount:null,start_date:null,end_date:null,renewal_terms:'',termination_terms:'',notes,draft_file_version_id:null,parent_contract_id:null,payment_schedule:null}};
 if(kind==='subscription')return{kind,arguments:{...scope,source_key:'agent-draft:'+key,product:title,supplier:null,owner_id:actor,category:'',start_date:null,trial_until:null,cycle:null,timezone:'',first_charge_date:null,anchor_day:null,custom_interval_days:null,renewal_mode:'',payment_method_label:'',reminder_days:[],vendor_state:'unknown',vendor_checked_at:null,vendor_evidence_version_ids:[],prices:[],notes}};
 return{kind,arguments:{...scope,title,category:'',purpose:'',owner_id:actor,currency:null,pricing_basis:'',rounding_mode:'',expected_delivery_date:null,period_start:null,period_end:null,budget_reference:'',supplier_candidates:[],quote_version_ids:[],lines:[],notes}};
}
export function businessDraftAccepted(run:S['Run'],input?:BusinessDraft):boolean{const v=run.business_draft;if(!input)return!v;if(!v||v.run_id!==run.id||!hash(v.intent_sha256)||v.input.kind!==input.kind||canonical(v.input.arguments)!==canonical(input.arguments)||run.oa_request||run.mutation||run.file_update||run.file_create||run.post_write||run.contract_body)return false;return v.status==='authorized'?v.operation_id===null&&v.result===null:businessDraftReceipt(run);}
export function businessDraftReceipt(run:S['Run']):boolean{
 const v=run.business_draft,r=v?.result;if(!v||v.run_id!==run.id||v.status!=='committed'||!v.operation_id||!uuid(v.operation_id)||!r||!uuid(r.id)||r.kind!==v.input.kind||r.version!==1||!uuid(r.revision_id)||!hash(r.content_hash)||r.state!=='draft')return false;
 const{project_id,classification,reason:_reason,...content}=v.input.arguments;
 if(r.project_id!==project_id||r.classification!==classification)return false;
 if(v.input.kind==='subscription'){const{source_key,...expected}=content as S['SubscriptionCreate'];return r.source_key===source_key&&r.title===expected.product&&canonical(r.content)===canonical(expected);}
 const{parent_contract_id:_parent,...expected}=content as S['ContractCreate'];return r.title===expected.title&&canonical(r.content)===canonical(expected);
}
export const businessDraftPath=(v:S['AgentBusinessDraftReceipt'])=>({contract:'/contracts/',purchase:'/purchases/',claim:'/claims/',subscription:'/subscriptions/'}[v.kind])+v.id;
