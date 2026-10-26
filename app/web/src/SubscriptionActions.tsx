import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Schemas } from './api';
import { Dialog, ErrorNotice, Field } from './components';
import { FileVersionPicker, useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import { fromHongKongInput } from './PaymentDisplay';
import { money } from './ContractDisplay';
import { refreshSubscriptions, subscriptionBase, type Subscription } from './SubscriptionDisplay';

export type SubscriptionActionKind = Schemas['SubscriptionTransition']['action'] | 'price' | 'generate' | 'unexpected';
export const subscriptionActionNames: Record<SubscriptionActionKind, string> = {
  activate: '启用订阅', pause: '暂停后续预测', resume: '恢复后续预测', cancel: '登记内部取消',
  verify_vendor: '核实供应方状态', archive: '归档订阅', restore: '恢复历史核对',
  price: '登记新价格', generate: '生成账期预测', unexpected: '登记停订后异常账期',
};
export function billingMonth() {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit' }).formatToParts();
  return parts.find(p => p.type === 'year')!.value + '-' + parts.find(p => p.type === 'month')!.value;
}
export function billingMonthRange(month: string) {
  const [year, number] = month.split('-').map(Number);
  return { from: month + '-01', to: `${String(number === 12 ? year + 1 : year).padStart(4, '0')}-${String(number === 12 ? 1 : number + 1).padStart(2, '0')}-01` };
}
export function SubscriptionAction({ subscription, action, onClose }: { subscription: Subscription; action: SubscriptionActionKind; onClose: () => void }) {
  const [fixed] = useState(subscription), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const range = billingMonthRange(billingMonth());
  const [from, setFrom] = useState(range.from), [to, setTo] = useState(range.to), [effective, setEffective] = useState('');
  const [currency, setCurrency] = useState(fixed.prices.at(-1)?.currency ?? ''), [amount, setAmount] = useState(''), [plan, setPlan] = useState(''), [priceEnd, setPriceEnd] = useState('');
  const [vendor, setVendor] = useState<Subscription['vendor_state']>('unknown'), [checked, setChecked] = useState(''), [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]), [picker, setPicker] = useState(false);
  const [ack, setAck] = useState(false), [result, setResult] = useState<Schemas['SubscriptionGeneration'] | null>(null);
  const cache = useQueryClient(), commandKey = useCommandKey();
  const mutation = useMutation({
    mutationFn: async () => {
      const base = subscriptionBase(fixed, reason);
      let path = '/subscriptions/' + fixed.id, body: object;
      if (action === 'generate') { path += '/periods/generate'; body = { ...base, from, to } satisfies Schemas['SubscriptionGenerate']; }
      else if (action === 'unexpected') { path += '/periods/unexpected'; body = { ...base, start_date: effective } satisfies Schemas['SubscriptionUnexpectedPeriod']; }
      else if (action === 'price') { path += '/prices'; body = { ...base, effective_from: effective, effective_to: priceEnd || null, currency, amount, plan } satisfies Schemas['SubscriptionPriceChange']; }
      else {
        path += '/transition';
        body = { ...base, action, effective_on: action === 'cancel' ? effective || null : null, vendor_state: action === 'verify_vendor' ? vendor : '', vendor_checked_at: action === 'verify_vendor' ? fromHongKongInput(checked) : null, vendor_evidence_version_ids: action === 'verify_vendor' ? files.map(f => f.version_id) : [] } satisfies Schemas['SubscriptionTransition'];
      }
      return api<Subscription | Schemas['SubscriptionGeneration'] | Schemas['SubscriptionPeriod']>(path, { method: 'POST', body, key: commandKey(body) });
    },
    onSuccess: async value => { await refreshSubscriptions(cache); if (action === 'generate') setResult(value as Schemas['SubscriptionGeneration']); else onClose(); },
  });
  const conflict = mutation.error instanceof ApiError && [409, 412].includes(mutation.error.status);
  const needsAck = ['cancel', 'pause', 'restore'].includes(action);
  return <Dialog title={subscriptionActionNames[action]} onClose={onClose} busy={mutation.isPending}>
    <p><strong>{fixed.product}</strong></p><p className="form-note">固定本次打开时的资料与版本。操作需要当前有效的身份确认。</p>
    <ErrorNotice error={mutation.error} />{conflict && <p className="notice warning">输入已保留。请核对最新状态与条件后重新打开操作。</p>}
    {result ? <><p className={'notice ' + (result.coverage_complete ? 'neutral' : 'warning')}>已新增 {result.generated_ids.length} 个账期；已有 {result.existing_count} 个保持原预测。{result.coverage_complete ? '本次窗口价格完整。' : '仍有价格缺项，本次窗口不完整。'}</p>{result.missing_price_dates.length > 0 && <p>缺价格的收费日：{result.missing_price_dates.join('、')}</p>}<button className="button primary" onClick={onClose}>返回订阅</button></> :
      <form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}>
        <fieldset className="form-fieldset" disabled={mutation.isPending || conflict}>
          {action === 'generate' && <><p className="notice neutral">按订阅的 {fixed.timezone} 自然日生成预测；不登记实际费用或付款。已有账期保留原价格，单次最长366个自然日。</p><Field label="收费日从"><input required type="date" value={from} onChange={e => setFrom(e.target.value)} /></Field><Field label="收费日至（不含）"><input required type="date" value={to} onChange={e => setTo(e.target.value)} /></Field></>}
          {action === 'price' && <><p className="notice neutral">本次价格只在账期起点采用，不自动覆盖已有预测或按天摊分。</p><Field label="价格生效日"><input required type="date" value={effective} onChange={e => setEffective(e.target.value)} /></Field><Field label="价格结束日（不含，可留空）"><input type="date" value={priceEnd} onChange={e => setPriceEnd(e.target.value)} /></Field><Field label="原币币种"><input required pattern="[A-Z]{3}" maxLength={3} value={currency} onChange={e => setCurrency(e.target.value.toUpperCase())} /></Field><Field label="原币价格"><input required inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} /></Field><Field label="套餐说明"><input maxLength={200} value={plan} onChange={e => setPlan(e.target.value)} /></Field></>}
          {action === 'activate' && <p className="notice neutral">将启用内部账期与预测管理。请先核对完整供应方、负责人、日历和首期价格；此操作不会在供应方购买或续费。</p>}
          {action === 'cancel' && <><p className="notice warning">登记内部取消及停止正常预测的生效日。供应方是否停扣须另行核实。</p><Field label="取消生效日" hint={`按 ${fixed.timezone || '订阅时区'} 自然日；留空表示实际操作日。`}><input type="date" value={effective} onChange={e => setEffective(e.target.value)} /></Field></>}
          {action === 'pause' && <p className="notice warning">按订阅时区的实际操作日暂停后续正常预测。保留历史账期及真实费用；供应方实际状态另行核实。</p>}
          {action === 'resume' && <p className="notice neutral">按原始计费日历恢复后续预测，保留暂停区间与全部历史。</p>}
          {action === 'restore' && <p className="notice warning">恢复取消状态下的历史核对入口，保留原取消日期和配置；不恢复供应方服务或未来正常收费。</p>}
          {action === 'archive' && <p className="notice neutral">仅在取消已生效、供应方取消已核实、已存历史账期满足核对条件时可归档；系统会检查当前实际状态。</p>}
          {action === 'unexpected' && <><p className="notice warning">仅登记准确停订区间内已经到达的原日历账期；随后须关联真实费用，不据此自动生成扣款。</p><Field label="异常收费日"><input required type="date" value={effective} onChange={e => setEffective(e.target.value)} /></Field></>}
          {action === 'verify_vendor' && <><p className="notice neutral">保存实际核实状态及固定依据，内部生命周期保持独立。</p><Field label="供应方实际状态"><select value={vendor} onChange={e => setVendor(e.target.value as Subscription['vendor_state'])}><option value="unknown">当前未知</option><option value="active">已核实启用</option><option value="paused">已核实暂停</option><option value="cancelled">已核实取消</option></select></Field><Field label="实际核实时间（香港时间）"><input type="datetime-local" step="1" required={vendor !== 'unknown'} value={checked} onChange={e => setChecked(e.target.value)} /></Field><div className="reference-options">{files.map(file => <div className="reference-option" key={file.version_id}><strong>{file.name}</strong><small>固定版本 {file.seq}</small><button className="text-button" type="button" onClick={() => setFiles(files.filter(f => f.version_id !== file.version_id))}>移除此凭证</button></div>)}</div><button className="button secondary" type="button" disabled={files.length >= 20} onClick={() => setPicker(true)}>选择固定核实凭证</button></>}
          {action === 'price' && amount && currency && <p>本次登记 {money(amount, currency)}。</p>}
          <Field label="操作依据"><textarea required rows={3} maxLength={200} value={reason} onChange={e => setReason(e.target.value)} /></Field>
          {needsAck && <label className="check-field"><input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />我已核对本次操作范围与供应方实际状态的区别。</label>}
          <MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || needsAck && !ack || action === 'verify_vendor' && vendor !== 'unknown' && !files.length}>{mutation.isPending ? '正在登记' : subscriptionActionNames[action]}</button></div>
        </fieldset>
      </form>}
    {picker && <FileVersionPicker project={fixed.project_id} classification={fixed.classification} title="选择准确核实凭证" onClose={() => setPicker(false)} onSelect={file => { setFiles(current => current.some(f => f.version_id === file.version_id) ? current : [...current, file]); setPicker(false); }} />}
  </Dialog>;
}
