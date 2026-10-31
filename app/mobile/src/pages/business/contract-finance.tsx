import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { contractFinanceEligible, contractFinanceLink, contractFinanceReceipt, type ContractFinanceKind } from '@copypaste/contracts/contract-finance';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { FixedFiles } from '../../platform/fixed-files';
import { go } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Check, Empty, Fact, Notice, Section, date } from '../../ui';
import { ChoiceList, FileChoice, TextField } from './controls';

type Contract = Schemas['Contract'];
type Node = Schemas['ContractFinanceNode'];
type Event = Schemas['ContractFinanceEvent'];
type Summary = Schemas['ContractFinance'];
type Kind = ContractFinanceKind | 'release' | 'review';
const labels: Record<Kind, string> = { cost: '归属真实费用', cash: '归属实际收付款', refund: '归属实际退回款', release: '解除部分归属', review: '核对节点财务' };
const reviewNames = { current: '当前依据已核对', stale: '来源已变化，须重新核对', unreviewed: '尚未人工核对', restricted: '核对情况不可查阅' };
const money = (amount: string | null | undefined, currency: string) => amount === null || amount === undefined ? '当前不可查阅' : currency + ' ' + amount;
export function MobileContractFinance({ contract, me, onClose }: { contract: Contract; me: Schemas['Me']; onClose(): void }) {
  const [refresh, setRefresh] = useState(0);
  return <><ContractFinanceContent key={refresh} contract={contract} me={me} onReload={() => setRefresh(n => n + 1)} /><Action secondary onClick={onClose}>返回合同资料</Action></>;
}
function ContractFinanceContent({ contract: c, me, onReload }: { contract: Contract; me: Schemas['Me']; onReload(): void }) {
  const record = useRecord<Summary>('/contracts/' + c.id + '/finance');
  const [history, setHistory] = useState<Node>(), [action, setAction] = useState<{ node: Node; kind: Kind; event?: Event }>();
  const view = record.value;
  if (action && view) return <ContractFinanceForm contract={c} me={me} view={view} {...action} onCancel={() => setAction(undefined)} onCommitted={onReload} />;
  if (history && view) return <ContractFinanceHistory contract={c} node={history} onClose={() => setHistory(undefined)} onRelease={event => setAction({ node: history, kind: 'release', event })} />;
  return <Section title='合同财务归属'><Notice error={record.error} />{record.loading ? <Busy /> : view && <>
    <View className='cp-pad'><Text className='cp-paragraph'>固定计划、真实费用、银行实收和合同归属分别记录。归属不发起银行付款，也不增加第二份发生费用。</Text><Text className='cp-muted cp-small'>合同版本 {view.contract_version} · 财务版本 {view.version}</Text></View>
    {!view.nodes.length && <Empty>此生效合同没有固定收付款节点。</Empty>}
    {view.nodes.map(n => <View className='cp-pad' key={n.id}>
      <Text className='cp-row-title'>{n.title}</Text><Text className='cp-muted cp-small'>{reviewNames[n.review_state]}</Text>
      <Fact label={n.direction === 'payable' ? '计划应付' : '计划应收'}><Text>{money(n.amount, n.currency)}</Text></Fact>
      <Fact label='已归属真实费用'><Text>{n.cost_visibility === 'not_applicable' ? '应收节点不登记支出费用' : money(n.cost_amount, n.currency)}</Text></Fact>
      <Fact label='已归属净收付'><Text>{money(n.cash_amount, n.currency)}</Text></Fact>
      <Fact label='与计划尚差／超过计划'><Text>{money(n.outstanding_amount, n.currency)} / {money(n.excess_amount, n.currency)}</Text></Fact>
      <Fact label='约定条件'><Text>{n.conditions_satisfied === null ? '尚未核实' : n.conditions_satisfied ? '已核实满足' : '尚未全部满足'}</Text></Fact>
      {n.has_unattributed_refunds && <Text className='cp-notice'>原付款存在尚未归属退款，请核对本节点的影响。</Text>}
      {view.can_reconcile && view.history_visibility === 'complete' && <>
        {n.direction === 'payable' && me.actions.includes('cost.record') && <Action secondary onClick={() => setAction({ node: n, kind: 'cost' })}>{labels.cost}</Action>}
        {me.actions.includes('payment.record') && <><Action secondary onClick={() => setAction({ node: n, kind: 'cash' })}>{labels.cash}</Action><Action secondary onClick={() => setAction({ node: n, kind: 'refund' })}>{labels.refund}</Action></>}
        {n.can_review && <Action onClick={() => setAction({ node: n, kind: 'review' })}>{labels.review}</Action>}
      </>}
      {view.history_visibility === 'complete' && <Action secondary onClick={() => setHistory(n)}>查看节点归属历史</Action>}
    </View>)}
  </>}<View className='cp-pad'><Action secondary onClick={onReload}>重新读取合同财务</Action></View></Section>;
}
function ReturnCash({ value: v }: { value: Schemas['ContractReturnCash'] }) {
  return <View className='cp-notice'><Text>本条实际付出归属／解除 {money(v.amount, v.currency)} · 尚占用 {v.remaining_amount === null ? '见原归属' : money(v.remaining_amount, v.currency)}</Text><Text>整笔资金 {money(v.whole_amount, v.currency)} · {v.paid_on}；原归属本金 {money(v.original_amount, v.original_currency)}</Text><Text>每1本金兑换 {v.rate} {v.currency} · {v.source}</Text></View>;
}
function RefundCash({ value }: { value: Schemas['ContractRefundCash'] }) {
  return <View className='cp-notice'><Text>整笔退款实收 {money(value.amount, value.currency)} · {value.paid_on}</Text><Text>退回原本金 {money(value.refund_fx.original_amount, value.refund_fx.original_currency)}。本条归属或解除只使用合同本金；整笔实收不能当作部分归属实收。</Text><Text>每1原币兑换 {value.refund_fx.rate} {value.currency} · {value.refund_fx.source}</Text></View>;
}
function ContractFinanceHistory({ contract: c, node, onClose, onRelease }: { contract: Contract; node: Node; onClose(): void; onRelease(e: Event): void }) {
  const rows = useCollection<Event>('/contracts/' + c.id + '/finance/events?milestone_id=' + node.id + '&limit=20');
  return <Section title='节点归属与核对历史'><Notice error={rows.error} />{rows.loading ? <Busy /> : rows.items.map(e => <View className='cp-pad' key={e.id}>
    <Text className='cp-row-title'>{labels[e.kind]} · 记录 {e.seq}</Text>{e.amount !== null && <Text>{money(e.amount, node.currency)}</Text>}<Text>{e.reason}</Text><Text className='cp-muted cp-small'>{e.actor_name} · {date(e.created_at)}</Text>
    {e.return_cash && <ReturnCash value={e.return_cash} />}
    {e.refund_cash && <RefundCash value={e.refund_cash} />}
    {e.remaining_amount !== null && <Text>尚未解除 {money(e.remaining_amount, node.currency)} · 当前有效 {money(e.effective_amount, node.currency)}</Text>}
    {e.payment_id && <Action secondary onClick={() => go('payment', e.payment_id!)}>核对来源资金</Action>}{e.cost_id && <Action secondary onClick={() => go('cost', e.cost_id!)}>核对来源费用</Action>}
    {e.can_release && <Action secondary onClick={() => onRelease(e)}>解除部分归属</Action>}
    {e.review_snapshot && <><Text>{e.conditions_satisfied ? '当时已核实条件满足。' : '当时未确认全部满足。'}{e.acknowledge_difference ? '已核对差额。' : ''}{e.acknowledge_unattributed_refunds ? '已核对尚未归属退款。' : ''}</Text>{e.review_snapshot.sources.map(s => <View className='cp-fixed-file' key={s.event_id}><Text>{labels[s.kind]} · 当时有效 {money(s.effective_amount, s.attribution_currency ?? s.currency)}</Text><Text>尚未解除 {money(s.remaining_amount, s.attribution_currency ?? s.currency)} · {s.state}</Text>{s.return_cash && <ReturnCash value={s.return_cash} />}{s.refund_cash && <RefundCash value={s.refund_cash} />}<Text className='cp-code'>固定来源修订 {s.current_revision_id}</Text></View>)}<FixedFiles files={e.attachments} /></>}
  </View>)}{!rows.loading && !rows.error && !rows.items.length && <Empty>尚无归属历史。</Empty>}{rows.next && <Action secondary loading={rows.moreBusy} onClick={() => void rows.more()}>加载更早节点归属</Action>}<Action secondary onClick={onClose}>返回合同财务</Action></Section>;
}
function ContractFinanceForm({ contract: c, view, node, kind, event, me, onCancel, onCommitted }: { contract: Contract; view: Summary; node: Node; kind: Kind; event?: Event; me: Schemas['Me']; onCancel(): void; onCommitted(): void }) {
  const [amount, setAmount] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [ack, setAck] = useState(false), [conditions, setConditions] = useState(false), [difference, setDifference] = useState(false), [refunds, setRefunds] = useState(false);
  const [returnCash, setReturnCash] = useState(''), [returnRate, setReturnRate] = useState(''), [returnSource, setReturnSource] = useState('');
  const [source, setSource] = useState<Schemas['Cost'] | Schemas['Payment']>(), [root, setRoot] = useState<Event>(), [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]), [pick, setPick] = useState<'root' | 'source' | 'file'>(), [error, setError] = useState<unknown>();
  const outgoingReturn = kind === 'refund' && node.direction === 'receivable' && source && 'record_kind' in source && source.currency !== node.currency;
  const binding = JSON.stringify([me.user.id, me.authorization_version, c.id, c.version, view.version, node.id, kind, event?.id]);
  const command = useCommand<Schemas['ContractFinanceReceipt']>(onCommitted, binding, async receipt => {
    if (!contractFinanceReceipt(receipt, c, view, node)) return false;
    const rows = await client.request<{ items: Event[] }>('/contracts/' + c.id + '/finance/events?milestone_id=' + node.id + '&limit=20');
    return rows.items.some(e => e.id === receipt.event_id && e.seq === receipt.version && e.kind === kind && e.actor_id === me.user.id && e.reason === reason.trim() && (kind === 'review' ? e.review_fingerprint === node.fingerprint : e.amount !== null && decimalEqual(e.amount, amount) && (kind === 'release' ? e.source_event_id === event?.id : e.source_revision_id === source?.current_revision_id && e.source_event_id === (root?.id ?? null) && !!e.refund_fx_acknowledged === !!(outgoingReturn || kind === 'refund' && source && 'refund_fx' in source && source.refund_fx) && (!outgoingReturn || !!e.return_cash && decimalEqual(e.return_cash.amount, returnCash) && decimalEqual(e.return_cash.rate, returnRate) && e.return_cash.source === returnSource.trim() && e.return_cash.currency === source?.currency))));
  });
  const disabled = command.busy || command.blocked, foreign = kind === 'refund' && source && 'refund_fx' in source ? source.refund_fx : undefined;
  async function submit() {
    try {
      if (!fresh || !reason.trim() || [...reason.trim()].length > 200) throw new Error('请填写依据并确认本人操作。');
      const base = { expected_version: view.version, contract_version: view.contract_version, reason: reason.trim() };
      let path = '/contracts/' + c.id + '/finance', body: Schemas['ContractFinanceLink'] | Schemas['ContractFinanceRelease'] | Schemas['ContractFinanceReview'];
      if (kind === 'review') {
        if (!node.fingerprint || !files.length || !node.can_review) throw new Error('请选择准确凭证并核对当前节点。');
        body = { ...base, expected_fingerprint: node.fingerprint, conditions_satisfied: conditions, acknowledge_difference: difference, acknowledge_unattributed_refunds: refunds, evidence_version_ids: files.map(f => f.version_id) }; path += '/milestones/' + node.id + '/review';
      } else if (kind === 'release') {
        if (!event?.can_release || !/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(amount) || decimalEqual(amount, '0')) throw new Error('请选择准确原归属并填写正数金额。');
        body = { ...base, amount }; path += '/entries/' + event.id + '/release';
      } else {
        if (!source) throw new Error('请选择准确财务来源。');
        body = contractFinanceLink(c, view, node, kind, source, root ?? null, amount, reason, ack, outgoingReturn ? { cash_amount: returnCash, rate: returnRate, source: returnSource, acknowledged: true } : undefined); path += '/entries';
      }
      setError(undefined); await command.submit(path, body);
    } catch (cause) { setError(new MobileError('INVALID_INPUT', cause instanceof Error ? cause.message : '请核对表单。')); }
  }
  if (pick === 'file') return <FileChoice project={c.project_id ?? ''} classification={c.classification} onClose={() => setPick(undefined)} onSelect={f => { setFiles(v => v.some(x => x.version_id === f.version_id) ? v : [...v, f]); setPick(undefined); }} />;
  if (pick === 'root') return <ChoiceList<Event> title='选择原正向资金归属' path={'/contracts/' + c.id + '/finance/events?milestone_id=' + node.id + '&limit=20'} label={e => '归属记录 ' + e.seq + ' · ' + money(e.remaining_amount, node.currency)} eligible={e => e.kind === 'cash' && !!e.remaining_amount && !decimalEqual(e.remaining_amount, '0') && !decimalEqual(e.effective_amount, '0')} onClose={() => setPick(undefined)} onSelect={e => { setRoot(e); setSource(undefined); setAck(false); setReturnCash(''); setReturnRate(''); setReturnSource(''); setPick(undefined); }} />;
  if (pick === 'source' && (kind === 'cost' || kind === 'cash' || kind === 'refund')) {
    const direction = kind === 'refund' ? node.direction === 'payable' ? 'refund' : 'outgoing' : node.direction === 'payable' ? 'outgoing' : 'incoming';
    const path = (kind === 'cost' ? '/costs?state=confirmed' : '/payments?state=recorded&direction=' + direction) + '&limit=20' + (kind === 'refund' ? '' : '&currency=' + node.currency);
    return <ChoiceList<Schemas['Cost'] | Schemas['Payment']> title='选择准确合同财务来源' path={path} label={s => s.title + ' · 整笔 ' + money(s.amount, s.currency ?? '')} eligible={s => contractFinanceEligible(c, node, kind, s, root)} onClose={() => setPick(undefined)} onSelect={s => { setSource(s); setAck(false); setReturnCash(''); setReturnRate(''); setReturnSource(''); setPick(undefined); }} />;
  }
  return <Section title={labels[kind]}><View className='cp-pad'><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-row-title'>{node.title} · 计划 {money(node.amount, node.currency)}</Text><Text className='cp-muted cp-small'>当前合同版本 {view.contract_version} · 财务版本 {view.version}。并发或权限变化后需重新读取。</Text>
    {kind === 'refund' && <><Action secondary disabled={disabled} onClick={() => setPick('root')}>选择原正向资金归属</Action>{root && <Text>原归属记录 {root.seq} · 尚未解除 {money(root.remaining_amount, node.currency)}</Text>}</>}
    {['cost', 'cash', 'refund'].includes(kind) && <><Action secondary disabled={disabled || kind === 'refund' && !root} onClick={() => setPick('source')}>选择准确合同财务来源</Action>{source && <><Text className='cp-row-title'>{source.title}</Text><Text>整笔原记录 {money(source.amount, source.currency ?? '')} · 修订 {source.version}</Text><Text className='cp-code'>{source.current_revision_id}</Text></>}</>}
    {event && <Text>原归属记录 {event.seq} · 尚未解除 {money(event.remaining_amount, node.currency)}</Text>}
    {foreign && source && 'paid_on' in source && <RefundCash value={{ currency: source.currency!, amount: source.amount!, paid_on: source.paid_on!, refund_fx: foreign }} />}{event?.refund_cash && <RefundCash value={event.refund_cash} />}
    {kind !== 'review' && <TextField name='contract-finance-amount' label={'本次合同原币归属本金（' + node.currency + '）'} value={amount} onChange={v => { setAmount(v); setAck(false); }} max={30} disabled={disabled} />}
    {outgoingReturn && <><Text>分别核对合同本金与本次付出，不新增付款或自动记汇差。</Text><TextField name='contract-return-cash' label={'本次实际付出归属金额（' + source?.currency + '）'} value={returnCash} onChange={v => { setReturnCash(v); setAck(false); }} max={30} disabled={disabled} /><TextField name='contract-return-rate' label={'每1 ' + node.currency + ' 实际兑换的 ' + source?.currency} value={returnRate} onChange={v => { setReturnRate(v); setAck(false); }} max={30} disabled={disabled} /><TextField name='contract-return-source' label='实际退回汇率凭证来源' value={returnSource} onChange={v => { setReturnSource(v); setAck(false); }} max={200} disabled={disabled} /><Check checked={ack} disabled={disabled} onChange={setAck}>已核对合同退回本金、本次实际付出与汇率凭证</Check></>}
    {event?.return_cash && <><ReturnCash value={event.return_cash} /><Text>解除按剩余本金比例舍入；最后一笔释放准确付出尾差，原付款事实保留。</Text></>}
    {foreign && <Check checked={ack} disabled={disabled} onChange={setAck}>已核对整笔实收与合同原本金；不自动换算费用或记汇差。</Check>}
    {kind === 'review' && <><Text>净收付 {money(node.cash_amount, node.currency)} · 尚差 {money(node.outstanding_amount, node.currency)} · 超额 {money(node.excess_amount, node.currency)}</Text><Check checked={conditions} disabled={disabled} onChange={setConditions}>已核实全部约定条件满足</Check><Check checked={difference} disabled={disabled} onChange={setDifference}>已核对净收付与计划差额</Check><Check checked={refunds} disabled={disabled} onChange={setRefunds}>已核对尚未归属退款影响</Check><FixedFiles files={files} />{files.map(f => <Action key={f.version_id} secondary disabled={disabled} onClick={() => setFiles(v => v.filter(x => x.version_id !== f.version_id))}>移除凭证 {f.name}</Action>)}<Action secondary disabled={disabled || files.length >= 20} onClick={() => setPick('file')}>选择固定节点核对凭证</Action></>}
    <TextField name='contract-finance-reason' label='合同财务操作依据' value={reason} onChange={setReason} max={200} multiline disabled={disabled} /><MFAConfirmation disabled={disabled} onFresh={setFresh} />
    <Action loading={command.busy} disabled={disabled || !fresh || (!!foreign || !!outgoingReturn) && !ack || !reason.trim()} onClick={() => void submit()}>{labels[kind]}</Action><Action secondary disabled={command.busy} onClick={onCancel}>返回合同财务，取消本次操作</Action>
  </View></Section>;
}
function decimalEqual(a: string, b: string) { const normalized = (v: string) => { const [w, f = ''] = v.split('.'); return w + '.' + f.padEnd(10, '0'); }; return normalized(a) === normalized(b); }
