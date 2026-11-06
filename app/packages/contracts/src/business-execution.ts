import type { components } from './api';
type S = components['schemas'];
export function executionText(value:string,label:string,max=200):string { const v=value.trim(); if(!v||v.length>max)throw new Error(label+'需要填写，且不能超过'+max+'字。');return v; }
export function executionDate(value:string,label:string):string { const v=value.trim();if(!/^\d{4}-\d{2}-\d{2}$/.test(v)||new Date(v+'T00:00:00Z').toISOString().slice(0,10)!==v)throw new Error(label+'需要有效的年-月-日。');return v; }
export function executionDecimal(value:string,label:string,positive=false):string { const v=value.trim();if(!/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(v)||positive&&!/[1-9]/.test(v))throw new Error(label+'需要填写有效的'+(positive?'正':'非负')+'数，最多六位小数。');return v; }
export function executionBase(v:Pick<S['Spend'],'version'|'current_revision_id'|'content_hash'>,reason:string):S['PurchaseExecutionInput'] { return {expected_version:v.version,revision_id:v.current_revision_id,content_hash:v.content_hash,reason:executionText(reason,'操作依据')}; }
export function executionReceipt(value:unknown,id:string,version:number):boolean { if(!value||typeof value!=='object')return false;const v=value as S['Spend'];return v.id===id&&v.version===version+1&&typeof v.current_revision_id==='string'&&/^[a-f0-9]{64}$/.test(v.content_hash); }
export function executionFiles(files:readonly S['BusinessFileReference'][],min=1,max=20):string[] { const ids=files.map(x=>x.version_id);if(ids.length<min||ids.length>max||new Set(ids).size!==ids.length||files.some(f=>!f.file_id||!/^[a-f0-9]{64}$/.test(f.sha256)))throw new Error('请核对固定版本依据文件，数量为'+min+'至'+max+'份且不得重复。');return ids; }
export function orderLines(source:readonly S['SpendLine'][], rows:Readonly<Record<string,{quantity:string;price:string;tax:string;discount:string}>>):S['PurchaseOrderLineInput'][] { const lines=source.filter(x=>rows[x.id]!==undefined).map(x=>({purchase_line_id:x.id,quantity:executionDecimal(rows[x.id].quantity,'订购数量',true),unit_price:executionDecimal(rows[x.id].price,'订购单价'),tax_amount:executionDecimal(rows[x.id].tax,'税额'),discount_amount:executionDecimal(rows[x.id].discount,'折扣')}));if(!lines.length||lines.length>100||Object.keys(rows).some(id=>!source.some(l=>l.id===id)))throw new Error('请从当前执行基线选择1至100行订购明细。');return lines; }
export const completionBlockers:Record<string,string>={restricted:'有关联资料当前不可查阅',not_approved:'采购尚未获批准',no_accepted_delivery:'尚无净验收交付',order_unfulfilled:'仍有有效订单待交付',variance_in_review:'仍有差异提案在审批',cost_unconfirmed:'关联费用尚未确认',cost_unsettled:'关联费用尚未结清',unattributed_refund:'仍有退款原付款本金未归属',currency_mismatch:'关联费用币种需核对',contract_not_effective:'合同尚未生效',plan_unknown:'收付款安排尚未完整确认',node_amount_difference:'节点金额尚未核平',unattributed_refunds:'仍有未归属退款',node_review_required:'节点需要重新核对',conditions_unconfirmed:'节点条件尚未确认',supplement_unresolved:'附属合同尚未处理',supplement_effective:'仍有生效中的附属合同'};

// The editor emits complete content. Refuse a partial or stripped proposal;
// never fill missing commercial fields from a different revision.
export function executionSpendContent(v:unknown):S['SpendContent']&{reason:string} {
 if(!v||typeof v!=='object'||Array.isArray(v))throw new Error('请核对完整商业提案。');const x=v as Record<string,unknown>;
 const keys=['title','category','purpose','owner_id','currency','pricing_basis','rounding_mode','expected_delivery_date','period_start','period_end','budget_reference','supplier_candidates','quote_version_ids','lines','notes','reason'];
 if(keys.some(k=>x[k]===undefined)||!Array.isArray(x.lines)||!Array.isArray(x.supplier_candidates)||!Array.isArray(x.quote_version_ids)||x.lines.some(l=>!l||typeof l!=='object'||!('id' in l)||!('description' in l)))throw new Error('完整商业字段或固定明细尚未提供。');
 const result=Object.fromEntries(keys.map(k=>[k,x[k]]));return result as S['SpendContent']&{reason:string};
}
