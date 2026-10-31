import type { components } from './api';
type S = components['schemas'];
export type ContractFinanceKind = 'cost' | 'cash' | 'refund';
const id = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const hash = /^[a-f0-9]{64}$/;
function units(value: unknown): bigint {
  if (typeof value !== 'string' || !/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(value)) throw new Error('金额或来源无效，请重新核对。');
  const [whole, fraction = ''] = value.split('.'); return BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, '0'));
}
export function contractFinanceEligible(c: S['Contract'], n: S['ContractFinanceNode'], kind: ContractFinanceKind, source: S['Cost'] | S['Payment'], root?: S['ContractFinanceEvent'] | null) {
  if (source.project_id !== c.project_id || source.counterparty?.party_id !== c.counterparty_id) return false;
  if ('entry_kind' in source) return kind === 'cost' && n.direction === 'payable' && source.state === 'confirmed' && source.entry_kind === 'expense' && source.currency === n.currency && source.links.some(l => l.source_type === 'contract' && l.source_id === c.id && l.source_revision_id === c.active_revision_id);
  if (kind === 'cost' || source.state !== 'recorded' || source.record_kind !== 'payment' || source.employee_id !== null) return false;
  if (kind === 'cash') return source.currency === n.currency && source.direction === (n.direction === 'payable' ? 'outgoing' : 'incoming');
  if (!root || root.contract_id !== c.id || root.milestone_id !== n.id || root.kind !== 'cash' || units(root.remaining_amount) <= 0n || units(root.effective_amount) <= 0n) return false;
  return n.direction === 'payable'
    ? source.direction === 'refund' && source.refund_of_id === root.payment_id && (source.currency === n.currency && !source.refund_fx || source.currency !== n.currency && source.refund_fx?.original_currency === n.currency)
    : source.currency === n.currency && source.direction === 'outgoing' && source.refund_of_id === null && !source.refund_fx;
}
export function contractFinanceLink(c: S['Contract'], view: S['ContractFinance'], node: S['ContractFinanceNode'], kind: ContractFinanceKind, source: S['Cost'] | S['Payment'], root: S['ContractFinanceEvent'] | null, amount: string, reason: string, acknowledged: boolean): S['ContractFinanceLink'] {
  if (!view.can_reconcile || view.history_visibility !== 'complete' || view.contract_id !== c.id || view.contract_version !== c.version || !view.nodes.some(n => n.id === node.id) || !id.test(source.id) || !id.test(source.current_revision_id) || !hash.test(source.content_hash) || !contractFinanceEligible(c, node, kind, source, root) || units(amount) <= 0n || !reason.trim() || [...reason.trim()].length > 200) throw new Error('合同、来源或归属依据已变化，请核对最新资料。');
  const foreign = kind === 'refund' && 'refund_fx' in source && !!source.refund_fx;
  if (foreign && !acknowledged) throw new Error('请核实整笔实收与本次合同原本金分别使用的币种。');
  return { expected_version: view.version, contract_version: view.contract_version, reason: reason.trim(), milestone_id: node.id, kind, source_id: source.id, source_version: source.version, source_revision_id: source.current_revision_id, source_content_hash: source.content_hash, source_event_id: kind === 'refund' ? root!.id : null, amount, ...(foreign ? { refund_fx_acknowledged: true } : {}) };
}
export function contractFinanceReceipt(v: S['ContractFinanceReceipt'], c: S['Contract'], view: S['ContractFinance'], node: S['ContractFinanceNode']) {
  return !!v && v.contract_id === c.id && v.milestone_id === node.id && Number.isSafeInteger(v.version) && v.version === view.version + 1 && typeof v.event_id === 'string' && id.test(v.event_id);
}
