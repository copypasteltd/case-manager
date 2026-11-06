import type {components} from './api';
type S=components['schemas'];
export type BusinessRevision=S['AgentBusinessRevisionInput'];
export type RevisionBasis=S['AgentBusinessRevisionBasis'];
export type BusinessKind=BusinessRevision['kind'];
export type PreparedBusinessRevision=S['PreparedBusinessRevision'];
export type PreparedDraftBody=S['ContractCreate']|S['SpendCreate']|S['SubscriptionCreate'];
export type PreparedRevisionBody=S['ContractPatch']|S['SpendPatch']|S['SubscriptionPatch'];
const uuid=(s:string)=>/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(s),hash=(s:string)=>/^[a-f0-9]{64}$/.test(s);
export const canonicalBusiness=(v:unknown):string=>Array.isArray(v)?'['+v.map(canonicalBusiness).join(',')+']':v&&typeof v==='object'?'{'+Object.keys(v).sort().map(k=>JSON.stringify(k)+':'+canonicalBusiness((v as Record<string,unknown>)[k])).join(',')+'}':JSON.stringify(v);
export const businessRoutes:Record<BusinessKind,string>={contract:'/contracts',purchase:'/purchases',claim:'/claims',subscription:'/subscriptions'};
const baseFields=new Set(['expected_version','base_revision_id','revision_id','content_hash','reason','source_key','project_id','classification','parent_contract_id']);
export function businessContent(body:unknown):Record<string,unknown>{if(!body||typeof body!=='object'||Array.isArray(body))throw new Error('请核对完整业务资料。');return Object.fromEntries(Object.entries(body).filter(([k])=>!baseFields.has(k)));}
export function makeBusinessRevision(b:RevisionBasis,body:PreparedRevisionBody,crossScopeReviewed:boolean):BusinessRevision{
 if(!uuid(b.id)||!uuid(b.revision_id)||!hash(b.content_hash)||!hash(b.scope_sha256)||!Number.isSafeInteger(b.version)||b.version<1||b.classification==='confidential'||b.cross_scope&&!crossScopeReviewed)throw new Error('请先核对当前准确业务版本与本次范围，跨项目或分类须单独确认。');
 const v=body as unknown as Record<string,unknown>;
 if(v.expected_version!==b.version||(b.kind==='subscription'?v.revision_id!==b.revision_id||v.content_hash!==b.content_hash:v.base_revision_id!==b.revision_id))throw new Error('表单所依据的业务版本已变化，请重新读取。');
 return{kind:b.kind,resource_id:b.id,expected_version:b.version,base_revision_id:b.revision_id,content_hash:b.content_hash,scope_sha256:b.scope_sha256,cross_scope_reviewed:crossScopeReviewed,arguments:body} as BusinessRevision;
}
export function preparedRevisionMatches(v:PreparedBusinessRevision,input:BusinessRevision):boolean{
 const i=v.input,b=v.basis;
 if(!i||!b||i.kind!==input.kind||i.resource_id!==input.resource_id||i.expected_version!==input.expected_version||i.base_revision_id!==input.base_revision_id||i.content_hash!==input.content_hash||i.scope_sha256!==input.scope_sha256||i.cross_scope_reviewed!==input.cross_scope_reviewed)return false;
 try{const again=makeBusinessRevision(b,i.arguments,i.cross_scope_reviewed);return canonicalBusiness(again)===canonicalBusiness(i);}catch{return false;}
}
export function businessRevisionReceipt(run:S['Run']):boolean{
 const v=run.business_revision,r=v?.result;if(!v||v.run_id!==run.id||!hash(v.intent_sha256)||v.status!=='committed'||!v.operation_id||!uuid(v.operation_id)||!r||r.kind!==v.input.kind||r.id!==v.input.resource_id||r.version!==v.input.expected_version+1||!uuid(r.revision_id)||r.revision_id===v.input.base_revision_id||!hash(r.content_hash)||r.state!=='draft'||r.project_id!==v.basis.project_id||r.classification!==v.basis.classification)return false;
 return canonicalBusiness(r.content)===canonicalBusiness(businessContent(v.input.arguments));
}
export function businessRevisionAccepted(run:S['Run'],input?:BusinessRevision):boolean{
 const v=run.business_revision;if(!input)return!v;
 if(!v||v.run_id!==run.id||!hash(v.intent_sha256)||canonicalBusiness(v.input)!==canonicalBusiness(input)||!preparedRevisionMatches({input:v.input,basis:v.basis},input)||run.business_draft||run.oa_request||run.mutation||run.file_update||run.file_create||run.post_write||run.contract_body)return false;
 return v.status==='authorized'?v.operation_id===null&&v.result===null:businessRevisionReceipt(run);
}
const labels:Record<string,string>={title:'标题',product:'订阅产品',direction:'收付方向',type:'合同类型',category:'业务类别',counterparty_id:'往来方',counterparty_revision_id:'往来方资料修订',owner_id:'负责人',currency:'币种',amount:'金额',tax_amount:'税额',start_date:'开始日期',end_date:'结束日期',renewal_terms:'续约条款',termination_terms:'终止条款',notes:'备注',draft_file_version_id:'合同正文固定版本',parent_contract_id:'主合同',payment_schedule:'收付款计划',lines:'明细',id:'明细标识',description:'说明',quantity:'数量',unit:'单位',unit_price:'单价',declared_amount:'申报金额',discount_amount:'折扣',expense_date:'费用日期',supplier:'供应商',party_id:'往来方',revision_id:'资料修订',receipt_version_ids:'凭证固定版本',missing_receipt_reason:'缺少凭证原因',duplicate_receipt_reason:'重复凭证说明',funding:'付款来源',purpose:'用途',pricing_basis:'计价方式',rounding_mode:'舍入方式',expected_delivery_date:'预计交付日期',period_start:'期间开始',period_end:'期间结束',budget_reference:'预算依据',supplier_candidates:'供应商候选',quote_version_ids:'报价固定版本',trial_until:'试用截止日',cycle:'周期',timezone:'账期时区',first_charge_date:'首次扣费日期',anchor_day:'计费锚点日',custom_interval_days:'自定义间隔天数',renewal_mode:'续订方式',payment_method_label:'付款方式',reminder_days:'提前提醒天数',vendor_state:'供应方状态',vendor_checked_at:'供应方核对时间',vendor_evidence_version_ids:'供应方凭据版本',prices:'价格计划',effective_from:'生效日期',effective_to:'结束日期',plan:'方案说明',due_date:'计划日期',percentage:'比例',kind:'计划类别',sequence:'顺序',name:'名称',expected_on:'预期日期',ratio:'比例',milestone:'节点',terms:'条款'};
export type BusinessFieldChange={label:string;before:string;after:string};
export function businessChanges(before:unknown,after:unknown):BusinessFieldChange[]{
 const rows:BusinessFieldChange[]=[];
 const text=(v:unknown)=>v===null||v===undefined?'未填写':v===''?'空白':typeof v==='boolean'?(v?'是':'否'):String(v);
 function walk(a:unknown,b:unknown,path:string[],depth:number):void{
  if(canonicalBusiness(a)===canonicalBusiness(b))return;
  if(depth>12||rows.length>6000)throw new Error('资料层级或数量超出完整预览范围，请在业务页继续整理。');
  if(Array.isArray(a)||Array.isArray(b)){const av=Array.isArray(a)?a:[],bv=Array.isArray(b)?b:[];for(let i=0;i<Math.max(av.length,bv.length);i++)walk(av[i],bv[i],[...path,String(i+1)],depth+1);if(!av.length&&!bv.length)rows.push({label:path.join(' / '),before:text(a),after:text(b)});return;}
  if(a&&typeof a==='object'||b&&typeof b==='object'){const av=a&&typeof a==='object'?a as Record<string,unknown>:{},bv=b&&typeof b==='object'?b as Record<string,unknown>:{};for(const k of [...new Set([...Object.keys(av),...Object.keys(bv)])])walk(av[k],bv[k],[...path,labels[k]??'其他资料（'+k+'）'],depth+1);return;}
  rows.push({label:path.join(' / '),before:text(a),after:text(b)});
 }
 walk(before,after,[],0);return rows;
}
