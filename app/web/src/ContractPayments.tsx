import { useQuery } from '@tanstack/react-query';
import { Plus, Trash2 } from 'lucide-react';
import { api, type Page, type Schemas } from './api';
import { Busy, ErrorNotice, Field, date } from './components';

type Plan = Schemas['ContractPaymentSchedule'];
type Item = Schemas['ContractPaymentPlanItem'];
const amount = (value: string | null, currency: string | null) => value === null ? '金额待确认' : (currency ?? '币种待确认') + ' ' + value.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');

export function PaymentScheduleEditor({ value, currency, onChange }: { value: Plan | null; currency: string | null; onChange: (value: Plan | null) => void }) {
  function add() { if (!value || value.items.length >= 100) return; onChange({ ...value, items: [...value.items, { id: crypto.randomUUID(), title: '', amount: null, due_date: null, conditions: '' }] }); }
  function change(id: string, update: Partial<Item>) { if (value) onChange({ ...value, items: value.items.map(item => item.id === id ? { ...item, ...update } : item) }); }
  return <section className="contract-editor-section payment-schedule-editor"><h3>收付款计划</h3><Field label="计划情况" hint="计划随合同修订送审。草稿可保留未知金额、日期；送审前须补全已列节点。"><select value={value?.mode ?? 'unknown'} onChange={event => { const mode = event.target.value; onChange(mode === 'unknown' ? null : { mode: mode as Plan['mode'], reason: '', items: mode === 'scheduled' ? [{ id: crypto.randomUUID(), title: '', amount: null, due_date: null, conditions: '' }] : [] }); }}><option value="unknown">尚未整理</option><option value="scheduled">按节点收款或付款</option><option value="no_payment">明确无收付款义务</option></select></Field>
    {value && <><Field label={value.mode === 'no_payment' ? '无收付款义务依据' : '计划说明'}><textarea required={value.mode === 'no_payment'} maxLength={20000} rows={2} value={value.reason} onChange={event => onChange({ ...value, reason: event.target.value })} /></Field>{value.mode === 'scheduled' && <><p className="form-note">各节点使用合同币种 {currency ?? '（待确认）'}，合计须与合同总额一致。</p>{value.items.map((item, index) => <fieldset className="payment-plan-item" key={item.id}><legend>收付款节点 {index + 1}</legend><Field label={'节点 ' + (index + 1) + ' 名称'}><input required maxLength={200} value={item.title} onChange={event => change(item.id, { title: event.target.value })} /></Field><div className="form-grid"><Field label={'节点 ' + (index + 1) + ' 金额'}><input inputMode="decimal" maxLength={30} value={item.amount ?? ''} onChange={event => change(item.id, { amount: event.target.value || null })} /></Field><Field label={'节点 ' + (index + 1) + ' 到期日'}><input type="date" value={item.due_date ?? ''} onChange={event => change(item.id, { due_date: event.target.value || null })} /></Field></div><Field label={'节点 ' + (index + 1) + ' 满足条件'}><textarea rows={2} maxLength={20000} value={item.conditions} onChange={event => change(item.id, { conditions: event.target.value })} /></Field><button type="button" className="text-button" disabled={value.items.length === 1} onClick={() => onChange({ ...value, items: value.items.filter(other => other.id !== item.id) })}><Trash2 size={16} />移除此节点</button></fieldset>)}<button type="button" className="button secondary" disabled={value.items.length >= 100} onClick={add}><Plus size={16} />添加收付款节点</button></>}</>}
  </section>;
}

export function PaymentScheduleFacts({ schedule, currency }: { schedule?: Plan | null; currency: string | null }) {
  return <section className="contract-attachment"><h3>收付款计划</h3>{!schedule ? <p className="muted">尚未整理收付款计划。</p> : <>{schedule.mode === 'no_payment' ? <p>明确无收付款义务</p> : <ol className="payment-plan-list">{schedule.items.map(item => <li key={item.id}><div className="section-heading"><h4>{item.title}</h4><strong>{amount(item.amount, currency)}</strong></div><p>到期日：{date(item.due_date)}</p>{item.conditions && <p className="payment-conditions">{item.conditions}</p>}</li>)}</ol>}{schedule.reason && <p className="payment-conditions">{schedule.reason}</p>}</>}</section>;
}

export function ContractMilestones({ contract }: { contract: Schemas['Contract'] }) {
  const query = useQuery({ queryKey: ['contract-milestones', contract.id, contract.version], queryFn: ({ signal }) => api<Page<Schemas['ContractMilestone']>>('/contracts/' + contract.id + '/milestones', { signal }), enabled: contract.lifecycle === 'effective' });
  if (contract.lifecycle !== 'effective') return null;
  return <section className="panel party-detail"><div className="section-heading"><h2>履约节点</h2><span className="muted">按生效合同计划固定</span></div><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取履约节点" /> : !query.error && (!query.data?.items.length ? <p className="muted">{contract.payment_schedule?.mode === 'no_payment' ? '本合同无收付款节点。' : '生效修订未列收付款节点。'}</p> : <ol className="payment-plan-list">{query.data.items.map(item => <li key={item.id}><div className="section-heading"><h3>{item.title}</h3><strong>{item.direction === 'receivable' ? '应收' : '应付'} {amount(item.amount, item.currency)}</strong></div><p>到期日：{date(item.due_date)}</p>{item.conditions && <p className="payment-conditions">{item.conditions}</p>}</li>)}</ol>)}</section>;
}
