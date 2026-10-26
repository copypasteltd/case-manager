import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Schemas } from './api';
import { Dialog, ErrorNotice, Field } from './components';
import { FileVersionPicker, useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import { money } from './ContractDisplay';
import { CostFacts } from './CostDisplay';
import { PaymentFacts } from './PaymentDisplay';
import { ContractCashRootPicker, ContractFinanceSourcePicker, isZeroAmount, type FinanceSource } from './ContractFinancePickers';

export type ContractFinanceAction = 'cost' | 'cash' | 'refund' | 'release' | 'review';
export const contractFinanceActionNames: Record<ContractFinanceAction, string> = { cost: '归属真实费用', cash: '归属实际收付款', refund: '归属实际退回款', release: '解除部分归属', review: '核对节点财务' };
export async function refreshContractFinance(cache: ReturnType<typeof useQueryClient>) {
  await cache.invalidateQueries({ predicate: query => ['contract-finance', 'contract-finance-events', 'contract-finance-source-picker', 'contract-closure-preview', 'contract-closure', 'payments', 'payment', 'payment-allocations', 'costs', 'cost', 'cash-report', 'claim-settlement'].includes(String(query.queryKey[0])) });
}
export function ContractFinanceActionDialog({ contract, view, node, action, event, onClose }: {
  contract: Schemas['Contract']; view: Schemas['ContractFinance']; node: Schemas['ContractFinanceNode']; action: ContractFinanceAction; event?: Schemas['ContractFinanceEvent']; onClose: () => void;
}) {
  const [fixed] = useState({ contract, view, node, event }), [reason, setReason] = useState(''), [amount, setAmount] = useState('');
  const [fresh, setFresh] = useState(false), [difference, setDifference] = useState(false), [refunds, setRefunds] = useState(false), [conditions, setConditions] = useState(false);
  const [source, setSource] = useState<FinanceSource | null>(null), [root, setRoot] = useState<Schemas['ContractFinanceEvent'] | null>(null);
  const [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]), [picker, setPicker] = useState<'source' | 'root' | 'file' | null>(null);
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => {
    const base = { expected_version: fixed.view.version, contract_version: fixed.view.contract_version, reason };
    let path = '/contracts/' + fixed.contract.id + '/finance', body: object;
    if (action === 'review') {
      if (!fixed.node.fingerprint) throw new Error('当前依据不可核对。');
      path += '/milestones/' + fixed.node.id + '/review';
      body = { ...base, expected_fingerprint: fixed.node.fingerprint, conditions_satisfied: conditions, acknowledge_difference: difference, acknowledge_unattributed_refunds: refunds, evidence_version_ids: files.map(f => f.version_id) } satisfies Schemas['ContractFinanceReview'];
    } else if (action === 'release') {
      if (!fixed.event) throw new Error('请先选择准确归属记录。');
      path += '/entries/' + fixed.event.id + '/release'; body = { ...base, amount } satisfies Schemas['ContractFinanceRelease'];
    } else {
      if (!source || action === 'refund' && !root) throw new Error('请先选择准确来源。');
      const value = source.value; path += '/entries';
      body = { ...base, milestone_id: fixed.node.id, kind: action, source_id: value.id, source_version: value.version, source_revision_id: value.current_revision_id, source_content_hash: value.content_hash, source_event_id: action === 'refund' ? root!.id : null, amount } satisfies Schemas['ContractFinanceLink'];
    }
    return api<Schemas['ContractFinanceReceipt']>(path, { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refreshContractFinance(cache); onClose(); } });
  const conflict = mutation.error instanceof ApiError && [409, 412].includes(mutation.error.status);
  const differs = !isZeroAmount(fixed.node.outstanding_amount) || !isZeroAmount(fixed.node.excess_amount);
  const disabled = mutation.isPending || conflict || !fresh || action === 'review' && (!files.length || differs && !difference || fixed.node.has_unattributed_refunds && !refunds) || ['cost', 'cash', 'refund'].includes(action) && !source;
  return <Dialog title={contractFinanceActionNames[action]} onClose={onClose} busy={mutation.isPending}>
    <p><strong>{fixed.node.title}</strong> · 计划 {money(fixed.node.amount, fixed.node.currency)}</p>
    <p className="form-note">{action === 'review' ? '本次固定当前费用、收付款、退款和凭证。来源后续变化须重新核对。' : action === 'release' ? '保留实际费用和资金，追加解除记录并保留原归属。' : '从既有真实记录中分配金额，保留来源；不新增费用或银行付款。'}</p>
    <ErrorNotice error={mutation.error} />{conflict && <p className="notice warning">输入已保留。来源、余额或版本已变化，请核对最新资料后重新打开。</p>}
    <form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}><fieldset className="form-fieldset contract-finance-action" disabled={mutation.isPending || conflict}>
      {action === 'refund' && <section className="form-section"><h3>原正向资金</h3>{root && <p>归属记录 {root.seq} · 当前剩余 {money(root.remaining_amount, fixed.node.currency)}</p>}<button type="button" className="button secondary" onClick={() => setPicker('root')}>选择原正向资金归属</button></section>}
      {['cost', 'cash', 'refund'].includes(action) && <section className="form-section"><h3>本次准确来源</h3>{source ? <><strong>{source.value.title}</strong>{source.type === 'cost' ? <CostFacts value={source.value} /> : <PaymentFacts value={source.value} />}</> : <p className="muted">尚未选择来源。</p>}<button type="button" className="button secondary" disabled={action === 'refund' && !root} onClick={() => setPicker('source')}>选择准确合同财务来源</button></section>}
      {action === 'release' && <p>原归属记录 {fixed.event?.seq} · 尚未解除 {money(fixed.event?.remaining_amount, fixed.node.currency)}</p>}
      {action !== 'review' && <Field label="本次原币归属金额" hint={'使用 ' + fixed.node.currency + ' 最小货币单位。'}><input required inputMode="decimal" maxLength={30} value={amount} onChange={e => setAmount(e.target.value)} /></Field>}
      {action === 'review' && <><dl className="file-facts"><div><dt>已归属净收付</dt><dd>{money(fixed.node.cash_amount, fixed.node.currency)}</dd></div><div><dt>与计划尚差</dt><dd>{money(fixed.node.outstanding_amount, fixed.node.currency)}</dd></div><div><dt>超过计划</dt><dd>{money(fixed.node.excess_amount, fixed.node.currency)}</dd></div></dl>
        <label className="check-field"><input type="checkbox" checked={conditions} onChange={e => setConditions(e.target.checked)} />已核实本节点约定条件均已满足。</label>
        {differs && <label className="check-field"><input type="checkbox" checked={difference} onChange={e => setDifference(e.target.checked)} />已核对净收付款与计划差额，并在依据中说明。</label>}
        {fixed.node.has_unattributed_refunds && <label className="check-field"><input type="checkbox" checked={refunds} onChange={e => setRefunds(e.target.checked)} />已核实尚未归属的退款对本节点的影响。</label>}
        <div className="reference-options">{files.map(file => <div className="reference-option" key={file.version_id}><strong>{file.name}</strong><small>固定版本 {file.seq}</small><button className="text-button" type="button" onClick={() => setFiles(files.filter(f => f.version_id !== file.version_id))}>移除此核对凭证</button></div>)}</div><button className="button secondary" type="button" disabled={files.length >= 20} onClick={() => setPicker('file')}>选择固定节点核对凭证</button>
      </>}
      <Field label="操作依据"><textarea required maxLength={200} rows={3} value={reason} onChange={e => setReason(e.target.value)} /></Field><MFAConfirmation onFresh={setFresh} />
      <div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose}>取消</button><button className="button primary" disabled={Boolean(disabled)}>{mutation.isPending ? '正在登记' : contractFinanceActionNames[action]}</button></div>
    </fieldset></form>
    {picker === 'root' && <ContractCashRootPicker contractID={fixed.contract.id} node={fixed.node} onClose={() => setPicker(null)} onSelect={value => { setRoot(value); setSource(null); setPicker(null); }} />}
    {picker === 'source' && (action === 'cost' || action === 'cash' || action === 'refund') && <ContractFinanceSourcePicker contract={fixed.contract} node={fixed.node} kind={action} original={root} onClose={() => setPicker(null)} onSelect={value => { setSource(value); setPicker(null); }} />}
    {picker === 'file' && <FileVersionPicker project={fixed.contract.project_id} classification={fixed.contract.classification} title="选择准确节点核对凭证" onClose={() => setPicker(null)} onSelect={file => { setFiles(values => values.some(f => f.version_id === file.version_id) ? values : [...values, file]); setPicker(null); }} />}
  </Dialog>;
}
