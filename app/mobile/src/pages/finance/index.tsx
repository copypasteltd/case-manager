import { CostPaymentPage } from './cost-payment';
import { SubscriptionCommands, SubscriptionHistory, SubscriptionOperationPage } from './subscriptions';
import { CostAmortization } from './amortization';
import { FinancialHistory } from './history';
import { PaymentFees } from './payment-fees';
import { FinancialCommands, FinanceOperationPage } from './operations';
import { Text, View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useCollection } from '../../platform/collection';
import { FixedFiles } from '../../platform/fixed-files';
import { go, identifier, usePage } from '../../platform/page';
import { exactAmount, financialKind, financialReadPath, overviewState } from '../../platform/project-overview';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, dateOnly } from '../../ui';
import ProjectChangePage from '../project/change';
import { overviewClock } from '../../platform/project-overview';

type Record = { kind: 'cost'; value: Schemas['Cost'] } | { kind: 'payment'; value: Schemas['Payment'] } | { kind: 'subscription'; value: Schemas['Subscription'] } | { kind: 'change'; value: Schemas['Change'] };
export default function FinanceDetailPage() {
  const params = useRouter().params;
  return params.action === 'recordPayment' ? <CostPaymentPage /> : params.kind === 'subscription' && params.action ? <SubscriptionOperationPage /> : params.kind === 'change' ? <ProjectChangePage changeID={params.id} /> : params.action ? <FinanceOperationPage /> : <FinancialRecordPage />;
}
function FinancialRecordPage() {
  const params = useRouter().params;
  const page = usePage<Record>(async scope => {
    const kind = financialKind(params.kind), id = identifier(params.id), path = financialReadPath(kind, id);
    const value = await client.request<Record['value']>(path, { scope });
    if (value.id !== id) throw new MobileError('INVALID_RESPONSE', '业务资料与所选记录不一致，请重新读取。');
    // Each path has an independently authorized response contract. Narrowing
    // only occurs after choosing that fixed path, never from a model/tool URL.
    return { kind, value } as Record;
  });
  const record = page.result?.value, value = record?.value;
  const title = !record ? '业务资料查阅' : record.kind === 'subscription' ? record.value.product : record.kind === 'change' ? record.value.revision.content.title : record.value.title;
  return <Page title={title} home><Notice error={page.error} />{page.loading ? <Busy /> : record && <>
    <Section title='当前资料'><Fact label='业务状态'><Text>{overviewState(record.value.state)} · 版本 {record.value.version}</Text></Fact><Fact label='更新时刻'><Text>{overviewClock(record.value.updated_at)}</Text></Fact><Fact label='当前修订'><Text className='cp-code' selectable>{record.value.current_revision_id}</Text></Fact><View className='cp-pad'><Text className='cp-muted cp-small'>发生支出、资金记录、预算、审批额度与订阅预测分别判断。</Text></View></Section>
    {(record.kind === 'cost' || record.kind === 'payment') && page.result && <FinancialCommands kind={record.kind} value={record.value} me={page.result.me} />}
    {record.kind === 'cost' && <><CostDetails value={record.value} />{page.scope && <CostAmortization key={record.value.id + ':' + record.value.version} cost={record.value} scope={page.scope} />}</>}
    {record.kind === 'payment' && <><PaymentDetails value={record.value} />{page.scope && page.result && <PaymentFees payment={record.value} me={page.result.me} scope={page.scope} refresh={() => void page.reload()} />}</>}
    {record.kind === 'subscription' && <><SubscriptionCommands value={record.value} /><SubscriptionDetails value={record.value} /><SubscriptionHistory id={record.value.id} /></>}
    {record.kind === 'change' && <ChangeDetails value={record.value} />}
    {(record.kind === 'cost' || record.kind === 'payment') && <FinancialHistory kind={record.kind} id={record.value.id} />}
    {value?.project_id && <Action secondary onClick={() => go('project', value.project_id!)}>查看所属项目</Action>}
  </>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新业务资料</Action></Page>;
}
function Evidence({ files }: { files: Schemas['BusinessFileReference'][] }) {
  return <Section title='固定版本凭证'>{files.length ? <View className='cp-pad'><FixedFiles files={files} /></View> : <Empty>暂无当前可读的固定版本凭证。</Empty>}</Section>;
}
function CostDetails({ value }: { value: Schemas['Cost'] }) {
  const currency = value.currency ?? '币种未核实';
  return <>
    <Section title={value.entry_kind === 'reversal' ? '冲销费用原额' : '费用原额'}><Fact label='发生日期'><Text>{dateOnly(value.occurred_on)}</Text></Fact><Fact label='原币金额'><Text>{currency} {exactAmount(value.amount)}</Text></Fact><Fact label='费用分类'><Text>{value.category || '未填写'}</Text></Fact><Fact label='固定交易方'><Text>{value.counterparty_evidence?.name ?? '尚未核实'}</Text></Fact>{value.base_currency && <Fact label='记录的本位币金额'><Text>{value.base_currency} {exactAmount(value.base_amount)}</Text></Fact>}<Fact label='说明'><Text className='cp-paragraph'>{value.notes || '未填写'}</Text></Fact>{value.state === 'draft' && <View className='cp-notice'><Text>这是未确认费用，尚未计入发生支出。</Text></View>}{value.reverses_id && <Row title='查看被冲销原费用' subtitle='冲销与原记录分别保留，不重复计费' onClick={() => go('cost', value.reverses_id!)} />}{value.reversal_id && <Row title='查看对应冲销记录' onClick={() => go('cost', value.reversal_id!)} />}</Section>
    <Section title='实际分摊'>{value.allocations.length ? value.allocations.map((share, index) => share.project_id ? <Row key={index} title={currency + ' ' + exactAmount(share.amount)} subtitle='项目费用份额 · 打开对应项目核对' onClick={() => go('project', share.project_id!)} /> : <Fact key={index} label='公司费用份额'><Text>{currency} {exactAmount(share.amount)}</Text></Fact>) : <Empty>尚未填写分摊。</Empty>}<View className='cp-pad'><Text className='cp-muted cp-small'>费用原额不是每个项目的发生支出；项目概览只统计对应项目的实际分摊。</Text></View></Section>
    <Section title='资金结算核对'>{value.settlement_visibility === 'complete' ? <><Fact label='已结算金额'><Text>{currency} {exactAmount(value.paid_amount)}</Text></Fact><Fact label='待结算金额'><Text>{currency} {exactAmount(value.outstanding_amount)}</Text></Fact><Fact label='结算状态'><Text>{(value.settlement_state ? { unsettled: '未结算', partial: '部分结算', settled: '已结清' }[value.settlement_state] : '尚未核实')}</Text></Fact></> : <Empty>{value.settlement_visibility === 'not_confirmed' ? '费用尚未确认，结算情况未核实。' : '结算读取范围不足，金额与余额未核实。'}</Empty>}</Section>
    <Section title='关联业务'>{value.links.length ? value.links.map(link => <Fact key={link.id} label={link.title ?? '关联记录标题当前不可读'}><Text className='cp-muted cp-small'>固定来源修订 {link.source_revision_id}</Text><Text className='cp-paragraph'>{link.reason}</Text>{link.title && <Action secondary onClick={() => link.source_type === 'subscription_period' ? void Taro.navigateTo({ url: '/pages/subscription-period/index?id=' + identifier(link.source_id) }) : go(link.source_type, link.source_id)}>查看关联业务当前资料</Action>}</Fact>) : <Empty>暂无当前可读的业务关联。</Empty>}</Section>
    <Evidence files={value.attachments} />
  </>;
}
function PaymentDetails({ value }: { value: Schemas['Payment'] }) {
  const currency = value.currency ?? '币种未核实';
  return <>
    <Section title='资金记录'><Fact label='方向与类型'><Text>{value.direction ? { outgoing: '支出', incoming: '收入', refund: '退款', transfer: '内部转账' }[value.direction] : '尚未核实'} · {value.record_kind === 'reversal' ? '冲销记录' : '原资金记录'}</Text></Fact><Fact label='实际时刻'><Text>{value.paid_at ? overviewClock(value.paid_at) : '尚未登记'}</Text></Fact><Fact label='业务日期'><Text>{dateOnly(value.paid_on)}</Text></Fact><Fact label='原币金额'><Text>{currency} {exactAmount(value.amount)}</Text></Fact><Fact label='固定交易方'><Text>{value.counterparty_evidence?.name ?? '尚未核实'}</Text></Fact><Fact label='付款方式与外部流水'><Text>{value.method_label || '未填写'}</Text><Text selectable>{value.external_ref || '未填写'}</Text></Fact><Fact label='说明'><Text className='cp-paragraph'>{value.notes || '未填写'}</Text></Fact><View className='cp-notice'><Text>登记是已有资金事实的记录，不会自动发起银行付款，也不增加第二份发生支出。</Text></View>{value.refund_fx && <Fact label='异币退款实收依据'><Text>原本金 {value.refund_fx.original_amount} {value.refund_fx.original_currency}</Text><Text>每1原币兑换 {value.refund_fx.rate} {value.currency} · {value.refund_fx.source}</Text><Text>现金按实收币种记录；原付款扣减原本金；不回转费用或合同归属。</Text></Fact>}{value.refund_of_id && <Row title='查看退款对应原资金记录' onClick={() => go('payment', value.refund_of_id!)} />}{value.reverses_id && <Row title='查看被冲销原资金记录' onClick={() => go('payment', value.reverses_id!)} />}{value.reversal_id && <Row title='查看资金冲销记录' onClick={() => go('payment', value.reversal_id!)} />}</Section>
    <Section title='分配与退款'><Fact label='有效分配净额'><Text>{currency} {exactAmount(value.effective_allocated_amount)}</Text></Fact><Fact label='已退款金额'><Text>{currency} {exactAmount(value.refunded_amount)}</Text></Fact><Fact label='合同退款预留'><Text>{currency} {exactAmount(value.contract_return_reserved_amount)}</Text></Fact><Fact label='尚未分配金额'><Text>{currency} {exactAmount(value.unallocated_amount)}</Text></Fact></Section>
    <Allocations key={value.id + ':' + value.version} paymentID={value.id} currency={currency} />
    <Evidence files={value.attachments} />
  </>;
}
function Allocations({ paymentID, currency }: { paymentID: string; currency: string }) {
  const list = useCollection<Schemas['PaymentAllocation']>('/payments/' + paymentID + '/allocations?limit=20');
  return <Section title='当前可读分配流水'><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <>{list.items.length ? list.items.map(item => <Fact key={item.id} label={{ allocation: '分配', release: '释放', refund: '退款分配', reversal: '冲销分配' }[item.kind]}><Text>{item.effect === -1 ? '减少' : '增加'} {currency} {exactAmount(item.amount)}</Text><Text className='cp-muted cp-small'>原分配可用余额 {exactAmount(item.remaining_amount)} · {overviewClock(item.created_at)}</Text>{item.cost_currency && <Text>费用原币 {item.cost_amount} {item.cost_currency} · 原币剩余 {item.remaining_cost_amount}</Text>}{item.fx && <Text className='cp-paragraph'>每1 {item.cost_currency}兑换 {item.fx.rate} {item.payment_currency} · {item.fx.quoted_on} · {item.fx.source}</Text>}{item.cost_shares?.map((share, index) => <Text className='cp-small' key={'cost-' + index}>{share.project_id ? '项目' : '公司'}费用原币份额 {share.amount} {item.cost_currency}</Text>)}<Text className='cp-paragraph'>{item.reason}</Text><Action secondary onClick={() => go('cost', item.cost_id)}>查看对应费用当前资料</Action><Text className='cp-muted cp-small'>固定费用修订 {item.cost_revision_id}</Text>{item.shares.map((share, index) => <Text className='cp-small' key={index}>{share.project_id ? '项目份额' : '公司份额'} {currency} {exactAmount(share.amount)}</Text>)}</Fact>) : <Empty>暂无当前可读分配流水。</Empty>}<View className='cp-pad'><Text className='cp-muted cp-small'>当前载入 {list.items.length} 项；释放与冲销作为独立流水保留。</Text></View></>}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多分配流水</Action>}<Action secondary onClick={list.reload}>重新读取分配流水</Action></Section>;
}
function SubscriptionDetails({ value }: { value: Schemas['Subscription'] }) {
  return <>
    <Section title='订阅配置'><Fact label='供应商'><Text>{value.supplier_evidence?.name ?? '尚未核实'}</Text></Fact><Fact label='负责人'><Text>{value.owner_display_name ?? '当前不可用'}{!value.owner_available && ' · 当前不可用'}</Text></Fact><Fact label='计费周期与时区'><Text>{value.cycle ? { month: '每月', year: '每年', custom: '自定义天数' }[value.cycle] : '尚未设置'}{value.cycle === 'custom' ? ' · ' + value.custom_interval_days + ' 天' : ''} · {value.timezone}</Text></Fact><Fact label='首次计费与下次计费'><Text>{dateOnly(value.first_charge_date)} / {dateOnly(value.next_charge_date)}</Text></Fact><Fact label='续费方式'><Text>{({ automatic: '自动续费', manual: '人工续费', '': '尚未设置' }[value.renewal_mode])}</Text></Fact><Fact label='开始与试用结束'><Text>{dateOnly(value.start_date)} / {dateOnly(value.trial_until)}</Text></Fact><Fact label='取消生效日期'><Text>{value.cancel_effective_on ? dateOnly(value.cancel_effective_on) : '尚未安排取消'}</Text></Fact><Fact label='供应商侧核对'><Text>{overviewState(value.vendor_state)} · {value.vendor_checked_at ? overviewClock(value.vendor_checked_at) : '尚未核对'}</Text></Fact><Fact label='备注'><Text className='cp-paragraph'>{value.notes || '未填写'}</Text></Fact><View className='cp-notice'><Text>内部取消与供应商实际停费分别核对。价格和账期是预测，不自动成为费用或付款。</Text></View></Section>
    <Section title='价格期间'>{value.prices.length ? value.prices.map((price, index) => <Fact key={index} label={price.plan || '订阅价格'}><Text>{price.currency} {exactAmount(price.amount)}</Text><Text>{dateOnly(price.effective_from)} — {price.effective_to ? dateOnly(price.effective_to) : '持续有效'}</Text></Fact>) : <Empty>尚未配置价格。</Empty>}</Section>
    <Periods key={value.id + ':' + value.version} subscriptionID={value.id} />
    <Evidence files={value.attachments} />
  </>;
}
function Periods({ subscriptionID }: { subscriptionID: string }) {
  const list = useCollection<Schemas['SubscriptionPeriod']>('/subscriptions/' + subscriptionID + '/periods?limit=20');
  return <Section title='当前可读账期'><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <>{list.items.length ? list.items.map(period => <Row key={period.id} title={dateOnly(period.start_date) + ' — ' + dateOnly(period.end_date)} subtitle={overviewState(period.state) + ' · 预计 ' + period.currency + ' ' + exactAmount(period.amount)} onClick={() => void Taro.navigateTo({ url: '/pages/subscription-period/index?id=' + period.id })} />) : <Empty>暂无已生成的可读账期；不据此判断没有订阅义务。</Empty>}<View className='cp-pad'><Text className='cp-muted cp-small'>当前载入 {list.items.length} 项，按服务器已生成账期核对。</Text></View></>}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多订阅账期</Action>}<Action secondary onClick={list.reload}>重新读取订阅账期</Action></Section>;
}
function ChangeDetails({ value }: { value: Schemas['Change'] }) {
  const revision = value.revision, content = revision.content;
  return <>
    <Section title='范围与时间提案'><Fact label='审批与实施'><Text>审批：{overviewState(value.approval_state)}</Text><Text>{value.implementation ? '已实施 · ' + overviewClock(value.implementation.created_at) : '尚未实施；批准不等于已改变项目'}</Text></Fact><Fact label='准确提案修订'><Text>第 {revision.seq} 修订 · 项目基线版本 {revision.basis.project_version}</Text><Text className='cp-code' selectable>{revision.content_hash}</Text></Fact><Fact label='拟实施完整范围'><Text className='cp-paragraph'>{content.scope}</Text></Fact><Fact label='范围变化说明'><Text className='cp-paragraph'>{content.scope_delta || '未填写'}</Text></Fact><Fact label='时间影响说明'><Text className='cp-paragraph'>{content.schedule_delta || '未填写'}</Text></Fact><Fact label='拟实施起止日期'><Text>{dateOnly(content.start_date)} — {dateOnly(content.due_date)}</Text></Fact><Fact label='提案原因'><Text className='cp-paragraph'>{revision.reason}</Text></Fact>{value.request_id && <Row title='查看对应审批申请' subtitle='由当前有权人员核对本人步骤' onClick={() => go('request', value.request_id!)} />}</Section>
    <Section title='预算提案'><Fact label='审批计价金额'><Text>{content.currency} {exactAmount(revision.approval_amount)}</Text><Text className='cp-muted cp-small'>各项正向增加合计；不是实际发生支出。</Text></Fact><Fact label='预算净变化'><Text>{content.currency} {exactAmount(revision.budget_delta)}</Text></Fact>{content.budget_lines === null ? <Empty>本提案保持预算不变。</Empty> : content.budget_lines.length ? content.budget_lines.map((line, index) => <Fact key={index} label={line.category}><Text>{dateOnly(line.period_start)} — {dateOnly(line.period_end)}</Text><Text>{content.currency} {exactAmount(line.amount)}</Text></Fact>) : <Empty>本提案明确清空该币种预算；批准后仍须单独实施。</Empty>}</Section>
    <Evidence files={revision.attachments} />
  </>;
}
