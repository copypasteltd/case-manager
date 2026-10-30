import { Text, View } from '@tarojs/components';
import { useRouter } from '@tarojs/taro';
import { useRef, useState } from 'react';
import { checkedCostPaymentBasis, costPaymentBody, costPaymentClaimLines, costPaymentPath, costPaymentReceipt, costPaymentRoute, costPaymentSeed, type CostPaymentBasis, type CostPaymentText } from '@copypaste/contracts/cost-payment';
import { CommandNotice, DraftNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { go, usePage } from '../../platform/page';
import { type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Fact, Notice, Page, Section } from '../../ui';
import { ChoiceList, FileChoice, PartyChoice, SelectField, TextField } from '../business/controls';

export function CostPaymentPage() {
  const params = useRouter().params;
  const page = usePage(async scope => { const id = costPaymentRoute(params); return checkedCostPaymentBasis(await client.request<unknown>(costPaymentPath(id), { scope }), id); });
  const basis = page.result?.value;
  return <Page title="登记付款并核销费用" home><Notice error={page.error} />{page.loading ? <Busy /> : basis && page.result && <CostPaymentForm key={basis.cost.id + ':' + basis.cost.version + ':' + basis.cost.content_hash + ':' + page.result.me.user.id + ':' + page.result.me.authorization_version} basis={basis} me={page.result.me} />}</Page>;
}
function CostPaymentForm({ basis, me }: { basis: CostPaymentBasis; me: Schemas['Me'] }) {
  const c = basis.cost;
  const draft = useFormDraft(me, { kind: 'cost-payment', id: c.id, version: c.version, revision: c.current_revision_id, hash: c.content_hash }, 'recordPayment', { costPayment: costPaymentSeed(basis, 'manual:' + client.operationKey()) });
  const text = draft.fields.costPayment!;
  const [party, setParty] = useState<Schemas['SpendSupplier'] | null>(null), [partyName, setPartyName] = useState('');
  const [claim, setClaim] = useState<Schemas['Spend'] | null>(null), [line, setLine] = useState(''), [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]);
  const [actual, setActual] = useState(false), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const [picker, setPicker] = useState<'party' | 'claim' | 'file'>();
  const submitted = useRef<Schemas['CostPaymentRecord']>();
  const command = useCommand<Schemas['CostPaymentReceipt']>(receipt => { draft.forget(); go('payment', receipt.payment.id); }, draft.binding, value => !!submitted.current && costPaymentReceipt(value, basis, submitted.current, me.user.id));
  const disabled = command.busy || command.blocked, lines = claim ? costPaymentClaimLines(basis, claim) : [];
  function uncheck() { setActual(false); setReviewed(false); }
  function change<K extends keyof CostPaymentText>(key: K, value: CostPaymentText[K]) { uncheck(); draft.update({ costPayment: { ...text, [key]: value } }); }
  async function submit() {
    setError(undefined);
    try { if (!fresh) throw new Error('请先验证本人操作。'); const body = costPaymentBody(basis, text, party, files.map(v => v.version_id), claim, line, actual, reviewed); submitted.current = body; await command.submit('/payments/from-cost', body); }
    catch (e) { setError(e); }
  }
  const field = (key: keyof CostPaymentText, label: string, max: number, hint?: string) => <TextField name={'cost-payment-' + key} label={label} value={text[key]} onChange={v => change(key, v)} max={max} hint={hint} disabled={disabled} />;
  return <><DraftNotice restored={draft.restored} /><CommandNotice blocked={command.blocked} /><Notice error={command.error ?? error} />
    <Section title="准确费用依据"><Fact label={c.title}><Text>{c.currency} {c.amount} · 费用版本 {c.version}</Text><Text>{c.project_id ? '费用所属项目' : '公司'} · {c.classification}</Text></Fact><View className="cp-pad"><Text className="cp-muted">分别填写真实付款总额与本次核销额。剩余款保留在资金记录；费用及报销余额由服务重新核对。本操作登记已发生事实。</Text></View></Section>
    <Section title="实际付款与核销"><View className="cp-pad">
      {field('title', '付款标题', 200)}{field('source', '永久资金来源标识', 199)}{field('date', '实际付款香港时间', 19, '格式 2025-02-10T09:30:00，须已实际发生。')}
      <SelectField label="实际付款币种" value={text.currency} options={['HKD', 'CNY', 'USD', 'EUR', 'JPY', 'GBP'].map(v => [v, v] as const)} onChange={v => { uncheck(); draft.update({ costPayment: { ...text, currency: v, costAmount: '', rate: '', quoteSource: '' } }); }} disabled={disabled} />
      {field('amount', '实际付款总额', 30)}{field('allocation', '本次核销付款额', 30, '填写付款币种金额，可小于实际付款总额。')}{field('category', '付款类别', 50)}{field('method', '实际付款方式', 100)}{field('external', '外部流水标识', 200)}
      {text.currency !== c.currency && <>{field('costAmount', '本次费用原币核销额', 30, c.currency!)}{field('rate', '每1费用原币兑换付款币种', 25)}{field('quoteSource', '实际汇率来源', 200)}</>}
      <SelectField label="实际收款类型" value={text.payee} options={[['supplier', '费用供应方'], ['employee', '已批准员工垫付报销']]} onChange={v => { change('payee', v); setParty(null); setPartyName(''); setClaim(null); setLine(''); }} disabled={disabled} />
      {text.payee === 'supplier' ? <><Text>{partyName || '请明确选择费用同一供应方的准确版本'}</Text><Action secondary disabled={disabled} onClick={() => setPicker('party')}>选择实际收款供应方</Action></> : <><Text>{claim ? claim.title + ' · 收款员工 ' + claim.creator_id : '请选择费用已关联的准确批准报销'}</Text><Action secondary disabled={disabled} onClick={() => setPicker('claim')}>选择批准垫付报销</Action>{claim && <SelectField label="批准垫付明细" value={line} options={[['', '明确选择一笔明细'], ...lines.map(v => [v.id, v.description + ' · ' + claim.currency + ' ' + v.declared_amount] as const)]} onChange={v => { uncheck(); setLine(v); }} disabled={disabled} />}</>}
      {files.map(v => <View key={v.version_id}><Text>{v.name} · 版本 {v.seq}</Text><Action secondary disabled={disabled} onClick={() => { uncheck(); setFiles(f => f.filter(x => x.version_id !== v.version_id)); }}>移除付款凭证</Action></View>)}<Action secondary disabled={disabled || files.length >= 20} onClick={() => setPicker('file')}>选择付款凭证版本</Action>
      {field('notes', '付款备注', 20000)}{field('reason', '付款核销登记原因', 200)}
      <SelectField label="我已核对实际付款时间、总额、币种、收款方及凭证" value={actual ? 'yes' : 'no'} options={[['no', '尚未核对'], ['yes', '已核对实际付款']]} onChange={v => setActual(v === 'yes')} disabled={disabled} />
      <SelectField label="我已核对本笔费用、核销金额、报销或异币依据" value={reviewed ? 'yes' : 'no'} options={[['no', '尚未核对'], ['yes', '已核对核销依据']]} onChange={v => setReviewed(v === 'yes')} disabled={disabled} />
      <MFAConfirmation onFresh={setFresh} disabled={disabled} /><Action loading={command.busy} disabled={disabled || !fresh || !actual || !reviewed} onClick={() => void submit()}>确认实际付款并核销</Action>
    </View></Section>
    {picker === 'party' && <PartyChoice project={c.project_id ?? ''} classification={c.classification} onClose={() => setPicker(undefined)} onSelect={v => { if (v.id !== c.counterparty!.party_id) { setError(new Error('收款方须与本笔费用供应方一致。')); return; } uncheck(); setError(undefined); setParty({ party_id: v.id, revision_id: v.current_revision_id }); setPartyName(v.name); setPicker(undefined); }} />}
    {picker === 'claim' && <ChoiceList<Schemas['Spend']> title="选择批准垫付报销" path="/claims?approval_state=approved" label={v => v.title + ' · ' + v.currency + ' ' + v.amount} eligible={v => costPaymentClaimLines(basis, v).length > 0} onClose={() => setPicker(undefined)} onSelect={v => { uncheck(); setClaim(v); setLine(''); setPicker(undefined); }} />}
    {picker === 'file' && <FileChoice project={c.project_id ?? ''} classification={c.classification} onClose={() => setPicker(undefined)} onSelect={v => { if (!files.some(x => x.version_id === v.version_id)) { uncheck(); setFiles(f => [...f, v]); } setPicker(undefined); }} />}
    <Action secondary disabled={command.busy} onClick={() => go('cost', c.id)}>返回准确费用</Action>
  </>;
}
