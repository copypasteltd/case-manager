import { useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { checkedCostPaymentBasis, costPaymentBody, costPaymentClaimLines, costPaymentPath, costPaymentReceipt, costPaymentSeed, type CostPaymentBasis, type CostPaymentText } from '@copypaste/contracts/cost-payment';
import { api, ApiError, type Me, type Schemas } from './api';
import { Busy, Dialog, ErrorNotice, Field } from './components';
import { FileVersionPicker, PartyPicker, useCommandKey, type FileReference } from './BusinessSelectors';
import { ApprovedClaimPicker } from './PaymentEditor';
import { MFAConfirmation } from './MFAConfirmation';
import { refreshPayments } from './PaymentDisplay';
import { costPaymentOutcomeUncertain } from './cost-payment-outcome';

export function CostPaymentEntry({ cost, me }: { cost: Schemas['Cost']; me: Me }) {
  const [open, setOpen] = useState(false);
  if (cost.state !== 'confirmed' || cost.entry_kind !== 'expense' || !['cost.read', 'payment.read', 'payment.record'].every(v => me.actions.includes(v))) return null;
  return <><button className="button primary" onClick={() => setOpen(true)}>登记付款并核销费用</button>{open && <CostPaymentDialog key={cost.id + ':' + me.user.id + ':' + me.authorization_version} id={cost.id} me={me} onClose={() => setOpen(false)} />}</>;
}
function CostPaymentDialog({ id, me, onClose }: { id: string; me: Me; onClose(): void }) {
  const query = useQuery({ queryKey: ['cost-payment-basis', id, me.user.id, me.authorization_version], gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: false,
    queryFn: async ({ signal }) => checkedCostPaymentBasis(await api<unknown>(costPaymentPath(id), { signal }), id) });
  const basis = query.error ? undefined : query.data;
  if (!basis) return <Dialog title="核对付款费用依据" onClose={onClose}>{query.isPending ? <Busy /> : <ErrorNotice error={query.error} />}</Dialog>;
  return <CostPaymentForm key={basis.cost.version + ':' + basis.cost.current_revision_id + ':' + basis.cost.content_hash} basis={basis} me={me} onClose={onClose} />;
}
function CostPaymentForm({ basis, me, onClose }: { basis: CostPaymentBasis; me: Me; onClose(): void }) {
  const c = basis.cost, navigate = useNavigate(), cache = useQueryClient(), commandKey = useCommandKey();
  const [text, setText] = useState(() => costPaymentSeed(basis, 'manual:' + crypto.randomUUID()));
  const [party, setParty] = useState<Schemas['SpendSupplier'] | null>(null), [partyName, setPartyName] = useState('');
  const [claim, setClaim] = useState<Schemas['Spend'] | null>(null), [line, setLine] = useState(''), [files, setFiles] = useState<FileReference[]>([]);
  const [actual, setActual] = useState(false), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false);
  const [picker, setPicker] = useState<'party' | 'file' | 'claim' | null>(null), [localError, setLocalError] = useState<unknown>();
  const received = useRef(false);
  function uncheck() { setActual(false); setReviewed(false); }
  function change<K extends keyof CostPaymentText>(name: K, value: CostPaymentText[K]) { uncheck(); setText(v => ({ ...v, [name]: value })); }
  const mutation = useMutation({ mutationFn: async () => {
    received.current = false;
    if (!fresh) throw new Error('请先验证本人操作。');
    const body = costPaymentBody(basis, text, party, files.map(v => v.version_id), claim, line, actual, reviewed);
    const receipt = await api<unknown>('/payments/from-cost', { method: 'POST', body, key: commandKey(body) }); received.current = true;
    if (!costPaymentReceipt(receipt, basis, body, me.user.id)) throw new Error('COST_PAYMENT_RECEIPT_UNCERTAIN');
    return receipt;
  }, onSuccess: async receipt => { await refreshPayments(cache); await Promise.all(['project-overview', 'cost-report', 'cost-payment-basis'].map(name => cache.invalidateQueries({ queryKey: [name] }))); navigate('/payments/' + receipt.payment.id); onClose(); } });
  const uncertain = !!mutation.error && costPaymentOutcomeUncertain(mutation.error, received.current), blocked = uncertain || mutation.error instanceof ApiError && [403, 404, 409, 412].includes(mutation.error.status);
  const lines = claim ? costPaymentClaimLines(basis, claim) : [];
  const field = (name: keyof CostPaymentText, label: string, max: number, hint?: string) => <Field label={label} hint={hint}><input aria-label={label} required={!['external', 'notes'].includes(name)} maxLength={max} value={text[name]} onChange={e => change(name, e.target.value)} /></Field>;
  return <Dialog title="登记付款并核销费用" onClose={onClose}><form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}>
    <section className="notice neutral"><strong>{c.title}</strong><p>固定费用版本 {c.version}，原费用 {c.currency} {c.amount}。实际付款总额与本次核销额分别填写；剩余款保留在资金记录。此操作只登记已发生的事实。</p><p>归属：{c.project_id ? '费用所属项目' : '公司'} · {c.classification}。费用及批准报销明细均按当前余额重新核对。</p></section>
    {uncertain ? <p className="notice warning">本次付款操作结果尚未完整核对。请关闭并按永久资金来源标识查阅，勿更换标识重复登记。</p> : <ErrorNotice error={mutation.error} />}<ErrorNotice error={localError} />
    {blocked && <p className="notice warning">请关闭并核对最新费用、权限和本次资金记录后继续。</p>}
    <fieldset className="form-fieldset" disabled={mutation.isPending || blocked}>
      {field('title', '付款标题', 200)}{field('source', '永久资金来源标识', 199)}
      <div className="form-grid">{field('date', '实际付款香港时间', 19, '格式 2025-02-10T09:30:00，须已实际发生。')}<Field label="实际付款币种"><select value={text.currency} onChange={e => { change('currency', e.target.value); setText(v => ({ ...v, costAmount: '', rate: '', quoteSource: '' })); }}>{['HKD', 'CNY', 'USD', 'EUR', 'JPY', 'GBP'].map(v => <option key={v}>{v}</option>)}</select></Field>{field('amount', '实际付款总额', 30)}{field('allocation', '本次核销付款额', 30, '填写付款币种金额，可小于实际付款总额。')}{field('category', '付款类别', 50)}{field('method', '实际付款方式', 100)}{field('external', '外部流水标识', 200)}</div>
      {text.currency !== c.currency && <section className="form-section"><h3>人工核对异币核销</h3>{field('costAmount', '本次费用原币核销额', 30, c.currency!)}{field('rate', '每1费用原币兑换付款币种', 25)}{field('quoteSource', '实际汇率来源', 200)}</section>}
      <section className="form-section"><h3>准确收款依据</h3><Field label="实际收款类型"><select value={text.payee} onChange={e => { change('payee', e.target.value); setParty(null); setPartyName(''); setClaim(null); setLine(''); }}><option value="supplier">费用供应方</option><option value="employee">已批准员工垫付报销</option></select></Field>
        {text.payee === 'supplier' ? <><p>{partyName || '请明确选择费用同一供应方的准确版本'}</p><button className="button secondary" type="button" onClick={() => setPicker('party')}>选择实际收款供应方</button></> : <><p>{claim ? claim.title + ' · 收款员工 ' + claim.creator_id : '请选择费用已关联的准确批准报销'}</p><button className="button secondary" type="button" onClick={() => setPicker('claim')}>选择批准垫付报销</button>{claim && <Field label="批准垫付明细"><select value={line} onChange={e => { uncheck(); setLine(e.target.value); }}><option value="">明确选择一笔明细</option>{lines.map(v => <option key={v.id} value={v.id}>{v.description} · {claim.currency} {v.declared_amount}</option>)}</select></Field>}</>}
      </section>
      <section className="form-section"><h3>固定付款凭证</h3>{files.map(f => <div className="cost-selected-file" key={f.version_id}><span>{f.name} · 版本 {f.seq}</span><button className="text-button" type="button" onClick={() => { uncheck(); setFiles(v => v.filter(x => x.version_id !== f.version_id)); }}>移除付款凭证</button></div>)}<button className="button secondary" type="button" disabled={files.length >= 20} onClick={() => setPicker('file')}>选择付款凭证版本</button></section>
      <Field label="付款备注"><textarea rows={3} maxLength={20000} value={text.notes} onChange={e => change('notes', e.target.value)} /></Field>{field('reason', '付款核销登记原因', 200)}
      <section className="form-section"><label><input type="checkbox" checked={actual} onChange={e => setActual(e.target.checked)} />我已核对实际付款时间、总额、币种、收款方及凭证</label><label><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />我已核对本笔费用、核销金额、报销或异币依据</label><MFAConfirmation onFresh={setFresh} /></section>
      <div className="dialog-actions"><button className="button primary" disabled={mutation.isPending || !fresh || !actual || !reviewed}>{mutation.isPending ? '正在登记并核销' : '确认实际付款并核销'}</button></div>
    </fieldset></form>
    {picker === 'party' && <PartyPicker project={c.project_id} classification={c.classification} onClose={() => setPicker(null)} onSelect={v => { if (v.id !== c.counterparty!.party_id) { setLocalError(new Error('收款方须与本笔费用供应方一致。')); return; } uncheck(); setLocalError(undefined); setParty({ party_id: v.id, revision_id: v.current_revision_id }); setPartyName(v.name); setPicker(null); }} />}
    {picker === 'claim' && <ApprovedClaimPicker currency={c.currency} onClose={() => setPicker(null)} onSelect={v => { if (!costPaymentClaimLines(basis, v).length) { setLocalError(new Error('此报销没有与本笔费用相符的已关联批准垫付明细。')); setPicker(null); return; } uncheck(); setLocalError(undefined); setClaim(v); setLine(''); setPicker(null); }} />}
    {picker === 'file' && <FileVersionPicker title="选择付款凭证版本" project={c.project_id} classification={c.classification} onClose={() => setPicker(null)} onSelect={v => { if (!files.some(x => x.version_id === v.version_id)) { uncheck(); setFiles(f => [...f, v]); } setPicker(null); }} />}
  </Dialog>;
}
