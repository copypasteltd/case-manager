import { RuntimeError, type SkillPackage } from './skills.js';

export interface ResourceReference { type: string; id: string; version_id?: string }
export interface ProposedResult {
  status: 'draft' | 'pending' | 'awaiting_input' | 'completed' | 'conflict' | 'denied' | 'failed';
  summary: string; resource_refs: ResourceReference[]; source_refs: ResourceReference[];
  operation_ids: string[]; missing_fields: string[]; next_action: string;
}
// Receipts come from the trusted execution ledger, separately from model output.
export interface ExecutionReceipt {
  run_id: string; authority_stamp: string; tool: string; operation_id: string | null;
  status: 'succeeded' | 'pending' | 'conflict' | 'denied' | 'failed'; committed: boolean;
  resource_refs: ResourceReference[]; source_refs: ResourceReference[];
}
const key = (ref: ResourceReference) => [ref.type, ref.id, ref.version_id ?? ''].join('/');

export function verifyResult(skill: SkillPackage, run: { id: string; authority_stamp: string; prepared_context?: string }, proposed: unknown, receipts: readonly ExecutionReceipt[]): ProposedResult {
  skill.validateOutput(proposed); const value = proposed as ProposedResult;
  const current = receipts.filter(receipt => receipt.run_id === run.id && receipt.authority_stamp === run.authority_stamp && skill.manifest.allowed_tools.includes(receipt.tool));
  const references = new Set(current.filter(receipt => receipt.status === 'succeeded' || receipt.status === 'pending').flatMap(receipt => [...receipt.resource_refs, ...receipt.source_refs]).map(key));
  if ([...value.resource_refs, ...value.source_refs].some(ref => !references.has(key(ref)))) throw new RuntimeError('RESULT_REFERENCE_UNVERIFIED');
  for (const id of value.operation_ids) {
    const receipt = current.find(item => item.operation_id === id);
    if (!receipt || (value.status === 'completed' && (receipt.status !== 'succeeded' || !receipt.committed))) throw new RuntimeError('RESULT_OPERATION_UNVERIFIED');
  }
  if (value.status === 'completed') {
    if (value.missing_fields.length) throw new RuntimeError('RESULT_COMPLETION_INCOMPLETE');
    if (skill.manifest.skill_id === 'subscription-review' && !current.some(receipt => ['read_finance_summary', 'read_subscription_review', 'read_finance_record'].includes(receipt.tool) && receipt.status === 'succeeded' && receipt.source_refs.some(ref => ref.type === 'agent_finance_report' && value.source_refs.some(cited => key(cited) === key(ref))))) throw new RuntimeError('RESULT_READ_UNVERIFIED');
    let completionTools=skill.manifest.completion_tools;
    if(skill.manifest.skill_id==='project-briefing'){
      let context:unknown;try{context=JSON.parse(run.prepared_context??'{}');}catch{throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');}
      if(!(context && typeof context==='object' && 'authorized_post_write' in context && context.authorized_post_write!=null))completionTools=[];
    }
    if (completionTools.length) {
      if (!current.some(receipt => receipt.status === 'succeeded' && receipt.committed && receipt.operation_id !== null && value.operation_ids.includes(receipt.operation_id) && completionTools.includes(receipt.tool))) throw new RuntimeError('RESULT_COMPLETION_UNVERIFIED');
    } else if (!current.some(receipt => receipt.status === 'succeeded') || !value.source_refs.length) throw new RuntimeError('RESULT_READ_UNVERIFIED');
  }
  return structuredClone(value);
}
