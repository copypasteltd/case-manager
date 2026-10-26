import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Schemas } from './api';
import { Busy, Dialog, ErrorNotice, Field } from './components';
import { FileVersionPicker, useCommandKey } from './BusinessSelectors';
import { money, lifecycleNames } from './ContractDisplay';
import { paymentTimestamp } from './PaymentDisplay';
import { FixedBillingFiles } from './SubscriptionDisplay';
import { MFAConfirmation } from './MFAConfirmation';

type Preview = Schemas['ContractClosurePreview'];
type Snapshot = Schemas['ContractClosureSnapshot'];
type Action = Schemas['ContractClosureInput']['action'];
const actionNames: Record<Action, string> = { completed: '登记合同完成', terminated: '登记合同终止' };
export const closureBlockerNames: Record<Preview['blockers'][number], string> = {
  contract_not_effective: '合同尚未生效', restricted: '当前不能查阅完整依据', plan_unknown: '收付款计划尚未明确',
  node_amount_difference: '计划节点与净收付存在差额', unattributed_refunds: '存在尚未归属的实际退款',
  node_review_required: '节点缺少对当前依据的人工核对', conditions_unconfirmed: '约定条件尚未全部核实满足',
  cost_unsettled: '关联合同费用尚未结清', supplement_unresolved: '仍有待处理的补充协议', supplement_effective: '仍有生效的补充协议，须先分别处理'
};

export function ContractClosure({ contract }: { contract: Schemas['Contract'] }) {
  const active = Boolean(contract.active_revision_id), terminal = ['completed', 'terminated'].includes(contract.lifecycle);
  const query = useQuery({ queryKey: ['contract-closure-preview', contract.id], queryFn: ({ signal }) => api<Preview>('/contracts/' + contract.id + '/closure-preview', { signal }), enabled: active });
  const record = useQuery({ queryKey: ['contract-closure', contract.id], queryFn: ({ signal }) => api<Schemas['ContractClosure']>('/contracts/' + contract.id + '/closure', { signal }), enabled: terminal && !query.error && query.data?.visibility === 'complete' });
  const [action, setAction] = useState<Action | null>(null), view = query.error ? undefined : query.data;
  if (!active) return null;
  return <section className="panel party-detail contract-closure" aria-label="合同完成与终止">
    <div className="section-heading"><h2>完成与终止</h2><span className="badge neutral">{lifecycleNames[contract.lifecycle]}</span></div>
    <p className="form-note">依据实际条款和凭证人工登记法律阶段。未结事项和真实收付款继续保留，后续财务更正可单独登记。</p><ErrorNotice error={query.error} />
    {query.isPending ? <Busy label="正在核对合同结束依据" /> : view && <>
      {view.visibility !== 'complete' || !view.basis ? <p className="notice neutral">当前不能查阅完整的节点、结算或补充协议依据。未结情况未知。</p> : <>
        {view.blockers.length ? <div className="notice warning"><strong>当前仍须处理的事项</strong><ul>{view.blockers.map(blocker => <li key={blocker}>{closureBlockerNames[blocker]}</li>)}</ul></div> : <p className="notice neutral">当前依据未发现节点差额、未结费用或待处理补充协议，仍须本人核实条款与完成凭证。</p>}
        <ClosureFacts basis={view.basis} />
        {terminal && <><ErrorNotice error={record.error} />{record.isPending ? <Busy label="正在读取合同结束记录" /> : !record.error && record.data && <ClosureRecord value={record.data} />}</>}
        {!terminal && (view.can_complete || view.can_terminate) && <div className="post-actions">{view.can_complete && <button className="button primary" onClick={() => setAction('completed')}>登记合同完成</button>}{view.can_terminate && <button className="button secondary" onClick={() => setAction('terminated')}>登记合同终止</button>}</div>}
      </>}
    </>}
    {action && view?.visibility === 'complete' && view.basis && <ClosureDialog contract={contract} preview={view} action={action} onClose={() => setAction(null)} />}
  </section>;
}

function ClosureFacts({ basis }: { basis: Snapshot }) {
  return <div className="contract-finance-nodes">
    <p className="muted">收付依据：{basis.schedule_mode === null ? '尚未明确计划' : basis.schedule_mode === 'no_payment' ? '已明确无收付款义务' : '已固定收付款节点'}</p>
    {basis.nodes.map(node => <article className="contract-finance-node" key={node.id}><h3>{node.title}</h3><dl className="file-facts"><div><dt>固定计划 / 归属净收付</dt><dd>{money(node.amount, node.currency)} / {money(node.cash_amount, node.currency)}</dd></div><div><dt>当前依据与约定条件</dt><dd>{node.review_current ? '当前依据已核对' : '须核对当前依据'} · {node.conditions_satisfied === true ? '已核实满足' : '尚未核实全部满足'}</dd></div><div><dt>原付款未归属退款</dt><dd>{node.has_unattributed_refunds ? '有，须核实影响' : '未发现'}</dd></div></dl></article>)}
    {basis.costs.length > 0 && <details><summary>关联合同费用与结算（{basis.costs.length}项）</summary>{basis.costs.map(cost => <article className="cost-business-link" key={cost.id}><Link className="text-button" to={'/costs/' + cost.id}>核对来源费用</Link><p>{cost.state === 'reversed' ? '原费用已冲销' : '已确认费用 ' + money(cost.amount, cost.currency)} · 有效结算 {money(cost.paid_amount, cost.currency)}</p><small className="cost-source-key">费用修订 {cost.revision_id}</small></article>)}</details>}
    {basis.children.length > 0 && <details><summary>相关补充协议（{basis.children.length}项）</summary>{basis.children.map(child => <div className="cost-business-link" key={child.id}><Link className="text-button" to={'/contracts/' + child.id}>查看补充协议</Link><span>{lifecycleNames[child.lifecycle]} · 版本 {child.version}</span></div>)}</details>}
  </div>;
}

function ClosureRecord({ value }: { value: Schemas['ContractClosure'] }) {
  return <article className="party-revision"><h3>{value.action === 'completed' ? '合同完成记录' : '合同终止记录'}</h3>
    {value.basis_state === 'changed' && <p className="notice warning">登记后的财务、结算或补充协议依据已变化，须继续跟进。原法律阶段与当时记录继续保留。</p>}
    <dl className="file-facts"><div><dt>实际发生日期</dt><dd>{value.occurred_on}</dd></div><div><dt>登记人 / 登记时间</dt><dd>{value.actor_name} · {paymentTimestamp(value.created_at)}</dd></div><div><dt>登记依据</dt><dd>{value.reason}</dd></div></dl>
    <div className="party-notes"><h3>条款与材料核实说明</h3><p>{value.attestation}</p>{value.unsettled_disposition && <><h3>未结事项后续处理</h3><p>{value.unsettled_disposition}</p></>}</div>
    <details><summary>查阅登记时的固定依据</summary><ClosureFacts basis={value.basis} /></details><h3>固定结束凭证</h3><FixedBillingFiles ids={value.attachments.map(file => file.version_id)} files={value.attachments} />
  </article>;
}

function ClosureDialog({ contract, preview, action, onClose }: { contract: Schemas['Contract']; preview: Preview; action: Action; onClose: () => void }) {
  const [fixed] = useState({ contract, preview }), [occurred, setOccurred] = useState(''), [attestation, setAttestation] = useState(''), [reason, setReason] = useState('');
  const [terms, setTerms] = useState(false), [acknowledge, setAcknowledge] = useState(false), [disposition, setDisposition] = useState(''), [fresh, setFresh] = useState(false);
  const [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]), [picker, setPicker] = useState(false);
  const cache = useQueryClient(), key = useCommandKey(), unresolved = fixed.preview.blockers.length > 0;
  const mutation = useMutation({ mutationFn: () => {
    if (!fixed.preview.basis || !fixed.preview.fingerprint) throw new Error('当前结束依据不可用。');
    const body: Schemas['ContractClosureInput'] = { expected_version: fixed.preview.contract_version, revision_id: fixed.preview.basis.revision_id,
      expected_finance_version: fixed.preview.basis.finance_version, expected_fingerprint: fixed.preview.fingerprint, action, occurred_on: occurred,
      terms_verified: terms, attestation, acknowledge_unsettled: acknowledge, unsettled_disposition: disposition, evidence_version_ids: files.map(file => file.version_id), reason };
    return api<Schemas['ContractClosureReceipt']>('/contracts/' + fixed.contract.id + '/closure', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await cache.invalidateQueries({ predicate: query => ['contract', 'contracts', 'contract-history', 'contract-closure-preview', 'contract-closure', 'contract-finance', 'contract-finance-events'].includes(String(query.queryKey[0])) }); onClose(); } });
  const conflict = mutation.error instanceof ApiError && [409, 412].includes(mutation.error.status);
  return <Dialog title={actionNames[action]} onClose={onClose} busy={mutation.isPending}><p><strong>{fixed.contract.title}</strong></p><p className="form-note">本次固定打开表单时的合同、节点、费用结算和补充协议依据。终止保留未结事项，不自动产生收付款。</p><ErrorNotice error={mutation.error} />{conflict && <p className="notice warning">输入已保留。依据或权限已变化，请核对当前资料后重新打开本次登记。</p>}
    <form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><fieldset className="form-fieldset contract-finance-action" disabled={mutation.isPending || conflict}>
      <Field label="实际完成或终止日期" hint="按真实发生日期填写，须介于合同生效日和香港当前日期之间。"><input required type="date" value={occurred} onChange={event => setOccurred(event.target.value)} /></Field>
      <ClosureFacts basis={fixed.preview.basis!} />
      {unresolved && <section className="form-section"><h3>本次固定未结事项</h3><ul>{fixed.preview.blockers.map(blocker => <li key={blocker}>{closureBlockerNames[blocker]}</li>)}</ul><label className="check-field"><input type="checkbox" checked={acknowledge} onChange={event => setAcknowledge(event.target.checked)} />本人确认上述未结事项在终止后仍须处理。</label><Field label="各项未结事项后续处理"><textarea required rows={4} maxLength={20000} value={disposition} onChange={event => setDisposition(event.target.value)} /></Field></section>}
      <label className="check-field"><input type="checkbox" checked={terms} onChange={event => setTerms(event.target.checked)} />本人已核实准确合同条款、实际履行情况和本次凭证。</label><Field label="条款与材料核实说明"><textarea required rows={4} maxLength={20000} value={attestation} onChange={event => setAttestation(event.target.value)} /></Field>
      <section className="form-section"><h3>准确结束凭证（1—20项）</h3>{files.map(file => <div className="reference-option" key={file.version_id}><strong>{file.name}</strong><small>固定版本 {file.seq}</small><button type="button" className="text-button" onClick={() => setFiles(values => values.filter(value => value.version_id !== file.version_id))}>移除此结束凭证</button></div>)}<button type="button" className="button secondary" disabled={files.length >= 20} onClick={() => setPicker(true)}>选择固定合同结束凭证</button></section>
      <Field label="登记原因"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} />
      <div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || conflict || !fresh || !terms || !files.length || unresolved && (!acknowledge || !disposition.trim())}>{mutation.isPending ? '正在登记' : actionNames[action]}</button></div>
    </fieldset></form>
    {picker && <FileVersionPicker project={fixed.contract.project_id} classification={fixed.contract.classification} title="选择准确合同结束凭证" onClose={() => setPicker(false)} onSelect={file => { setFiles(values => values.some(value => value.version_id === file.version_id) ? values : [...values, file]); setPicker(false); }} />}
  </Dialog>;
}
