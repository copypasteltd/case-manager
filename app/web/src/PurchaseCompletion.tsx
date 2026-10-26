import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Schemas } from './api';
import { Busy, date, Dialog, ErrorNotice, Field } from './components';
import { money } from './ContractDisplay';
import { bytes, FileVersionPicker, useCommandKey, type FileReference } from './BusinessSelectors';
import { DownloadDialog, type DownloadVersion } from './DownloadDialog';
import { MFAConfirmation } from './MFAConfirmation';
import { refreshSpend } from './SpendEditor';

type Purchase = Schemas['Spend'];
type Preview = Schemas['PurchaseCompletionPreview'];
type Snapshot = Schemas['PurchaseCompletionSnapshot'];
type Completion = Schemas['PurchaseCompletion'];
type Mode = Schemas['PurchaseCompletionInput']['settlement_mode'];
const modes: Record<Mode, string> = { recorded_costs: '按已归集实际费用核对', no_payable_obligation: '经人工核实无应付义务' };
const blockers: Record<string, string> = {
  no_accepted_delivery: '尚无实际合格验收，请先登记交付依据。',
  order_unfulfilled: '有效订单仍有待收余量，请先登记验收或明确取消余量。',
  variance_in_review: '差异提案正在审批，请先完成处理。',
  cost_unconfirmed: '关联费用尚未确认，无法核实完整实际支出。',
  cost_unsettled: '已确认费用仍有未结金额，请先核对实际付款与分配。',
  unattributed_refund: '关联退款仍有未归属金额，请先处理退款依据。',
  currency_mismatch: '关联实际费用币种与采购不一致，请先核对。',
  restricted: '当前无法完整查阅核对资料，完成条件未知。',
  not_approved: '采购尚未获批。',
};
const receiptNames = { accepted: '通过验收', rejected: '拒收', returned: '来源退货' };
const costNames: Record<string, string> = { expense: '实际费用', reversal: '冲销费用' };
const stateNames: Record<string, string> = { draft: '草稿', confirmed: '已确认', reversed: '已冲销', recorded: '已登记', voided: '已作废', approved: '已批准', in_review: '审批中', rejected: '已驳回', withdrawn: '已撤回', effective: '已启用', requested: '已送审' };
const allocationNames: Record<string, string> = { allocation: '付款分配', refund: '退款分配', release: '解除分配', reversal: '冲销分配' };
const conflict = (error: unknown) => error instanceof ApiError && (error.status === 409 || error.status === 404 || ['AUTHORIZATION_CHANGED', 'FORBIDDEN'].includes(error.code));

export function PurchaseCompletion({ purchase }: { purchase: Purchase }) {
  const [reviewing, setReviewing] = useState<{ purchase: Purchase; preview: Preview } | null>(null);
  const approved = purchase.approval_state === 'approved';
  const preview = useQuery({ queryKey: ['spend', 'purchase', purchase.id, 'completion-preview', purchase.version], enabled: approved, queryFn: ({ signal }) => api<Preview>('/purchases/' + purchase.id + '/completion-preview', { signal }) });
  const current = preview.error ? undefined : preview.data;
  const history = useQuery({ queryKey: ['spend', 'purchase', purchase.id, 'completion', purchase.version, current?.fingerprint], enabled: approved && purchase.lifecycle === 'completed' && current?.visibility === 'complete', queryFn: ({ signal }) => api<Completion>('/purchases/' + purchase.id + '/completion', { signal }) });
  const completed = history.error ? undefined : history.data;
  if (!approved) return null;
  return <section className="panel party-detail purchase-completion" aria-label="采购完成与结算核对">
    <div className="section-heading"><div><h2>完成与结算核对</h2><p className="muted">固定交付、差异处置和实际结算的人工核实依据。</p></div><span className="badge neutral">{purchase.lifecycle === 'completed' ? '已登记采购完成' : '待明确核对'}</span></div>
    <ErrorNotice error={preview.error} />
    {preview.isPending ? <Busy /> : current && (current.visibility !== 'complete' || !current.basis ? <p className="notice neutral">当前无法完整查阅相关交付、差异或财务资料。完成条件未知，请联系业务负责人核对查阅范围。</p> : <>
      {purchase.lifecycle === 'completed' ? <><ErrorNotice error={history.error} />{history.isPending ? <Busy /> : completed && <CompletionHistory value={completed} purchase={purchase} />}
        {completed?.basis_state === 'changed' && <section aria-label="完成后的当前核对依据"><h3>完成后的当前依据</h3><Blockers value={current.blockers} /><CompletionFacts value={current.basis} purchase={purchase} /></section>}
      </> : <><Blockers value={current.blockers} /><CompletionFacts value={current.basis} purchase={purchase} />
        <p className="form-note">空费用列表仍需人工核实。批准金额、订单、验收、实际费用与付款分别留存；登记完成会保存本次完整依据。</p>
        {current.has_difference && <p className="notice neutral">核对集合包含未使用额度、取消余量、未启用提案或费用差异，登记时须说明处置。</p>}
        {current.can_complete && <button className="button primary" disabled={preview.isFetching} onClick={() => setReviewing({ purchase, preview: current })}>核对并登记采购完成</button>}
        {!current.can_complete && current.blockers.length === 0 && <p className="form-note">请由具备完整资料访问与采购完成权限的人员核对登记。</p>}
      </>}
    </>)}
    {reviewing && <CompletionDialog purchase={reviewing.purchase} preview={reviewing.preview} onClose={() => setReviewing(null)} />}
  </section>;
}

function Blockers({ value }: { value: string[] }) {
  return value.length > 0 ? <div className="notice neutral" aria-label="采购完成待处理事项"><p>登记前仍需处理：</p><ul>{value.map(code => <li key={code}>{blockers[code] ?? '存在尚未处理的核对事项，请核对完整资料。'}</li>)}</ul></div> : null;
}

function CompletionFacts({ value, purchase }: { value: Snapshot; purchase: Purchase }) {
  const description = (id: string) => value.orders.flatMap(order => order.lines).find(line => line.purchase_line_id === id)?.description ?? purchase.lines.find(line => line.id === id)?.description ?? '执行明细 ' + id;
  return <div className="completion-facts">
    <dl className="file-facts"><div><dt>原批准修订</dt><dd>{value.revision_id}</dd></div><div><dt>执行依据</dt><dd>{value.execution_plan_revision_id ? '已启用差异修订 ' + value.execution_plan_revision_id : '原采购商业批准'}</dd></div><div><dt>执行总额</dt><dd>{money(value.execution_amount, value.currency)}</dd></div><div><dt>原批准内容指纹</dt><dd>{value.content_hash}</dd></div></dl>
    <details className="completion-section" open><summary>交付与额度 · {value.lines.length} 行</summary><div className="fulfillment-summary">{value.lines.map(line => <article className="spend-line" key={line.line_id}><h4>{description(line.line_id)}</h4><dl className="file-facts"><div><dt>执行数量</dt><dd>{line.approved_quantity}</dd></div><div><dt>占用数量</dt><dd>{line.reserved_quantity}</dd></div><div><dt>净验收数量</dt><dd>{line.net_accepted_quantity}</dd></div><div><dt>有效订单待收数量</dt><dd>{line.outstanding_quantity}</dd></div><div><dt>执行额度</dt><dd>{money(line.approved_amount, value.currency)}</dd></div><div><dt>占用金额</dt><dd>{money(line.reserved_amount, value.currency)}</dd></div><div><dt>占用税额／折扣</dt><dd>{money(line.reserved_tax, value.currency)} ／ {money(line.reserved_discount, value.currency)}</dd></div></dl></article>)}</div></details>
    <details className="completion-section"><summary>固定订单 · {value.orders.length} 张</summary>{value.orders.length === 0 ? <p className="muted">此集合尚无订单。</p> : value.orders.map(order => <article className="purchase-order-card" key={order.id}><h4>{order.external_ref}</h4><p>{order.cancelled ? '已取消余量，保留净验收' : '有效订单'} · {order.ordered_on} · {money(order.amount, value.currency)}</p><p className="form-note">供应商固定修订 {order.supplier_revision_id} · 采购版本 {order.purchase_version} · 订单执行依据 {order.execution_plan_revision_id ?? '原批准'}</p>{order.lines.map(line => <div className="spend-line" key={line.id}><h4>{line.description}</h4><dl className="file-facts"><div><dt>订购数量／单位</dt><dd>{line.quantity} ／ {line.unit}</dd></div><div><dt>净验收数量</dt><dd>{line.net_accepted_quantity}</dd></div><div><dt>固定单价</dt><dd>{money(line.unit_price, value.currency)}</dd></div><div><dt>税额／折扣</dt><dd>{money(line.tax_amount, value.currency)} ／ {money(line.discount_amount, value.currency)}</dd></div><div><dt>订单行金额</dt><dd>{money(line.amount, value.currency)}</dd></div></dl></div>)}</article>)}</details>
    <details className="completion-section"><summary>验收与来源退货 · {value.receipts.length} 批</summary>{value.receipts.length === 0 ? <p className="muted">此集合尚无验收记录。</p> : value.receipts.map(receipt => <article className="purchase-receipt-card" key={receipt.id}><h4>{receipt.received_on} · 采购版本 {receipt.purchase_version}</h4><p className="form-note">固定订单 {receipt.order_id}</p>{receipt.lines.map(line => <div className="purchase-receipt-line" key={line.id}><strong>{receiptNames[line.result]} {line.quantity}</strong><p>{line.reason}</p><p className="form-note">订单明细 {line.order_line_id}{line.reverses_line_id && ' · 原验收来源 ' + line.reverses_line_id}</p></div>)}</article>)}</details>
    <details className="completion-section"><summary>差异提案 · {value.variances.length} 份</summary>{value.variances.length === 0 ? <p className="muted">此集合没有差异提案。</p> : value.variances.map(item => <article className="party-revision" key={item.id}><h4><Link className="text-button" to={'/purchase-variances/' + item.id}>{item.title} ↗</Link></h4><p>{stateNames[item.approval_state] ?? item.approval_state} · {item.lifecycle === 'effective' ? '已有明确启用记录' : '尚未启用'}</p><p className="form-note">提案版本 {item.version} · 固定修订 {item.revision_id} · 依据采购版本 {item.basis_purchase_version} · {item.content_hash}</p></article>)}</details>
    <details className="completion-section" open><summary>实际费用与结算 · {value.costs.length} 条</summary>{value.costs.length === 0 ? <p className="muted">此集合未归集实际费用，仍需经办人核实是否存在应付义务。</p> : value.costs.map(cost => <article className="party-revision" key={cost.id}><h4><Link className="text-button" to={'/costs/' + cost.id}>{cost.title} ↗</Link></h4><dl className="file-facts"><div><dt>费用性质／状态</dt><dd>{costNames[cost.entry_kind] ?? cost.entry_kind} ／ {stateNames[cost.state] ?? cost.state}</dd></div><div><dt>实际金额</dt><dd>{money(cost.amount, cost.currency)}</dd></div><div><dt>有效已结金额</dt><dd>{money(cost.paid_amount, cost.currency)}</dd></div><div><dt>固定修订／版本</dt><dd>{cost.revision_id} ／ {cost.version}</dd></div></dl></article>)}</details>
    <details className="completion-section"><summary>付款、退款与分配 · {value.payments.length} 条</summary>{value.payments.length === 0 ? <p className="muted">此集合没有关联资金记录。</p> : value.payments.map(payment => <article className="party-revision" key={payment.id}><h4><Link className="text-button" to={'/payments/' + payment.id}>查阅固定资金记录 ↗</Link></h4><dl className="file-facts"><div><dt>资金性质／状态</dt><dd>{payment.record_kind === 'reversal' ? '冲销记录' : payment.direction === 'refund' ? '退款' : '付款'} ／ {stateNames[payment.state] ?? payment.state}</dd></div><div><dt>实际金额</dt><dd>{money(payment.amount, payment.currency)}</dd></div><div><dt>未归属退款</dt><dd>{money(payment.unattributed_refund_amount, payment.currency)}</dd></div><div><dt>固定修订／版本</dt><dd>{payment.revision_id} ／ {payment.version}</dd></div>{payment.refund_of_id && <div><dt>原付款来源</dt><dd>{payment.refund_of_id}</dd></div>}</dl>{payment.allocations.map(item => <p className="form-note" key={item.id}>{allocationNames[item.kind] ?? item.kind} · {money(item.amount, payment.currency)} · {item.effect === 1 ? '增加有效分配' : '减少有效分配'} · 费用 {item.cost_id} · 固定费用修订 {item.cost_revision_id}</p>)}</article>)}</details>
  </div>;
}

function CompletionHistory({ value, purchase }: { value: Completion; purchase: Purchase }) {
  const [download, setDownload] = useState<DownloadVersion | null>(null);
  return <section aria-label="不可变采购完成记录">
    <p className={'notice ' + (value.basis_state === 'changed' ? 'warning' : 'neutral')}>{value.basis_state === 'changed' ? '完成时依据已变化。以下保留当时核实集合，请另外处理后续交付、费用或退款义务。' : '已保存当时的完整核实依据。后续事项仍按各自实际凭证登记。'}</p>
    <dl className="file-facts"><div><dt>实际完成日期</dt><dd>{value.occurred_on}</dd></div><div><dt>登记人／时间</dt><dd>{value.actor_name} ／ {date(value.created_at)}</dd></div><div><dt>核实方式</dt><dd>{modes[value.settlement_mode]}</dd></div><div><dt>固定采购版本</dt><dd>{value.purchase_version}</dd></div><div><dt>核对集合指纹</dt><dd>{value.fingerprint}</dd></div></dl>
    <h3>人工核实说明</h3><p className="completion-prose">{value.attestation}</p>{value.difference_disposition && <><h3>差异处置</h3><p className="completion-prose">{value.difference_disposition}</p></>}<p className="completion-prose">登记依据：{value.reason}</p>
    <h3>完成时固定集合</h3><CompletionFacts value={value.basis} purchase={purchase} />
    <section className="contract-attachment"><h3>完成核实凭证</h3>{value.attachments.map(file => <div className="reference-summary" key={file.version_id}><div><strong>{file.name}</strong><p>固定版本 {file.seq} · {bytes(file.size_bytes)}</p><div className="post-actions"><button type="button" className="text-button" onClick={() => setDownload({ id: file.version_id, seq: file.seq, size_bytes: file.size_bytes, sha256: file.sha256, original_name: file.name, scan_state: file.scan_state })}>下载完成凭证</button><Link className="text-button" to={'/files/' + file.file_id}>查看来源文件</Link></div></div></div>)}</section>
    {download && <DownloadDialog version={download} onClose={() => setDownload(null)} />}
  </section>;
}

function CompletionDialog({ purchase, preview, onClose }: { purchase: Purchase; preview: Preview; onClose: () => void }) {
  const [base] = useState(purchase), [fixed] = useState(preview);
  const [occurred, setOccurred] = useState(''), [mode, setMode] = useState<Mode | ''>('');
  const [attestation, setAttestation] = useState(''), [disposition, setDisposition] = useState(''), [reason, setReason] = useState('');
  const [delivery, setDelivery] = useState(false), [costs, setCosts] = useState(false), [differences, setDifferences] = useState(false), [fresh, setFresh] = useState(false);
  const [files, setFiles] = useState<FileReference[]>([]), [picker, setPicker] = useState(false);
  const failure = useRef<HTMLDivElement>(null), cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => {
    if (!mode || !fixed.fingerprint) throw new Error('请先明确核实方式与准确依据。');
    const body: Schemas['PurchaseCompletionInput'] = { expected_version: base.version, revision_id: base.current_revision_id, content_hash: base.content_hash, expected_fingerprint: fixed.fingerprint, occurred_on: occurred, delivery_verified: delivery, costs_verified: costs, differences_verified: differences, settlement_mode: mode, attestation, difference_disposition: disposition, reason, evidence_version_ids: files.map(file => file.version_id) };
    return api<Schemas['PurchaseCompletionCommandReceipt']>('/purchases/' + base.id + '/completion', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refreshSpend(cache); onClose(); } });
  useEffect(() => { if (mutation.error) failure.current?.focus(); }, [mutation.error]);
  const ready = fresh && delivery && costs && differences && !!mode && !!occurred && !!attestation.trim() && !!reason.trim() && files.length > 0 && (!fixed.has_difference || !!disposition.trim()) && !conflict(mutation.error) && !mutation.isPending;
  return <Dialog title="核对并登记采购完成" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); if (ready) mutation.mutate(); }}>
    <h3>{base.title}</h3><p className="form-note">请逐项核对固定集合并作出人工声明。登记后，后续来源退货及财务变化继续单独留存。</p>
    <p className="record-code">固定采购版本 {base.version} · 核对指纹 {fixed.fingerprint}</p>
    {fixed.basis && <CompletionFacts value={fixed.basis} purchase={base} />}
    <div ref={failure} tabIndex={-1} aria-label="采购完成登记结果"><ErrorNotice error={mutation.error} />{conflict(mutation.error) && <p className="notice neutral">采购、财务依据或授权已变化，未按旧依据登记。填写内容仍保留，请关闭后核对最新完整集合。</p>}</div>
    <fieldset disabled={mutation.isPending} className="completion-attestation"><legend>人工核实与完成依据</legend>
      <Field label="实际采购完成日期"><input type="date" required value={occurred} onChange={event => setOccurred(event.target.value)} /></Field>
      <Field label="结算核实方式"><select required value={mode} onChange={event => setMode(event.target.value as Mode | '')}><option value="">请明确选择核实方式</option>{fixed.settlement_modes.map(value => <option key={value} value={value}>{modes[value]}</option>)}</select></Field>
      <Field label="完整费用归集与结算核实说明"><textarea required rows={4} maxLength={20000} value={attestation} onChange={event => setAttestation(event.target.value)} /></Field>
      <p className="form-note">说明核对过的实际交付、费用归集和结算凭证。选择无应付义务时，请说明如何核实，包括非零订单承诺的处理。</p>
      <Field label={fixed.has_difference ? '差异处置说明（必填）' : '差异处置说明'}><textarea required={fixed.has_difference} rows={4} maxLength={20000} value={disposition} onChange={event => setDisposition(event.target.value)} /></Field>
      <Field label="本次采购完成登记依据"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>
      <section className="contract-attachment"><h3>完成核实凭证</h3><p className="form-note">固定1至20个已正式提交的准确文件版本。</p>{files.map(file => <div className="reference-summary" key={file.version_id}><div><strong>{file.name}</strong><p>固定版本 {file.seq}</p></div><button type="button" className="text-button" onClick={() => setFiles(items => items.filter(item => item.version_id !== file.version_id))}>移除完成凭证</button></div>)}<button type="button" className="button secondary" disabled={files.length >= 20} onClick={() => setPicker(true)}>添加完成凭证版本</button></section>
      <label className="purchase-line-choice"><input type="checkbox" checked={delivery} onChange={event => setDelivery(event.target.checked)} />已核实实际交付、净验收和全部有效订单余量</label>
      <label className="purchase-line-choice"><input type="checkbox" checked={costs} onChange={event => setCosts(event.target.checked)} />已核实完整实际费用归集、付款和退款义务</label>
      <label className="purchase-line-choice"><input type="checkbox" checked={differences} onChange={event => setDifferences(event.target.checked)} />已核实全部差异提案、未使用额度及其处置</label>
    </fieldset>
    <MFAConfirmation onFresh={setFresh} />
    <div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!ready}>确认登记采购完成</button></div>
    {picker && <FileVersionPicker title="选择采购完成凭证版本" project={base.project_id} classification={base.classification} onSelect={file => { setFiles(items => [...items.filter(item => item.version_id !== file.version_id), file]); setPicker(false); }} onClose={() => setPicker(false)} />}
  </form></Dialog>;
}
