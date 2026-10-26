import { Text, View } from '@tarojs/components';
import Taro, { useDidHide, useDidShow } from '@tarojs/taro';
import { useEffect, useRef } from 'react';
import { go, identifier, usePage } from '../../platform/page';
import { currentOverview, exactAmount, overviewClock, overviewCollectionURL, overviewItemURL, overviewLabels, overviewState } from '../../platform/project-overview';
import type { Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section } from '../../ui';

export default function ProjectOverviewPage({ projectID }: { projectID: unknown }) {
  const page = usePage(async scope => {
    const id = identifier(projectID);
    const project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
    const overview = currentOverview(await client.request<Schemas['ProjectOverview']>('/projects/' + id + '/overview', { scope }), id, project.version);
    return { project, overview };
  });
  const timer = useRef<ReturnType<typeof setInterval>>();
  const stop = () => { if (timer.current) clearInterval(timer.current); timer.current = undefined; };
  useDidShow(() => { stop(); timer.current = setInterval(() => void page.reload(), 60000); });
  useDidHide(stop);
  useEffect(() => stop, []);
  const value = page.result?.value;
  return <Page title='项目业务概览' home><Notice error={page.error} />{page.loading ? <Busy /> : value && <>
    <Section title={value.project.name}><Fact label='当前项目'><Text>{value.project.project_no} · {overviewState(value.project.state)} · 版本 {value.overview.project_version}</Text></Fact><Fact label='读取快照'><Text>{overviewClock(value.overview.as_of)}</Text><Text className='cp-muted cp-small'>仅统计当前可读记录。结项核查与归档单独执行。</Text></Fact><View className='cp-pad'><Action secondary onClick={() => go('project', value.project.id)}>返回项目资料</Action></View></Section>
    <Section title='本项目发生支出'><View className='cp-pad'><Text className='cp-muted cp-small'>全期间、本项目实际分摊；费用与冲销按原币分列。草稿、付款、审批与订阅预测分别核对。</Text></View>
      {!value.overview.incurred.available ? <Empty>发生支出未核实 · 当前报表读取范围不足</Empty> : <>
        {value.overview.incurred.totals.length ? value.overview.incurred.totals.map(total => <Fact key={total.currency} label={total.currency + ' · ' + total.entry_count + ' 条记账记录'}><Text>费用原额 {exactAmount(total.expense_amount)}</Text><Text>冲销原额 {exactAmount(total.reversal_amount)}</Text><Text className='cp-row-title'>净发生额 {exactAmount(total.net_amount)}</Text></Fact>) : <Empty>暂无当前可读的已确认发生支出。</Empty>}
        <View className='cp-pad'><Text className='cp-muted cp-small'>未确认费用 {value.overview.incurred.unconfirmed_count} 项，未计入金额。冲销保留原费用与冲销记录，净额抵销。</Text></View>
      </>}
    </Section>
    <Section title='当前批准预算'>{!value.overview.groups.find(group => group.kind === 'budgets')?.available ? <Empty>预算未核实 · 当前读取范围不足</Empty> : value.overview.budgets.length ? value.overview.budgets.map(budget => <Row key={budget.budget_id} title={budget.currency + ' ' + exactAmount(budget.amount)} subtitle={'批准预算修订 ' + budget.revision} onClick={() => void Taro.navigateTo({ url: overviewItemURL(value.project.id, 'budgets', budget.budget_id) })} />) : <Empty>暂无当前批准预算。</Empty>}<View className='cp-pad'><Text className='cp-muted cp-small'>预算按批准期间独立展示；不同币种不相加，不据此推算剩余预算。</Text></View></Section>
    {value.overview.groups.map(group => <Section key={group.kind} title={overviewLabels[group.kind] + ' · ' + (group.available ? group.count : '—')}>
      {!group.available ? <Empty>未核实 · 当前读取范围不足</Empty> : <>
        <View className='cp-pad'><Text className='cp-muted cp-small'>当前全部可读记录{group.kind === 'files' ? ' · 有效目录中的文件' : group.kind === 'posts' ? ' · 不含已归档' : ''}</Text>{group.states.map(state => <Text className='cp-small' key={state.state}>{overviewState(state.state)} {state.count}</Text>)}</View>
        {group.items.length ? group.items.map(item => <Row key={item.id} title={item.title || '未命名记录'} subtitle={overviewState(item.state) + ' · ' + overviewClock(item.updated_at)} onClick={() => void Taro.navigateTo({ url: overviewItemURL(value.project.id, group.kind, item.id) })} />) : <Empty>暂无可读记录。</Empty>}
        {group.has_more && <View className='cp-pad'><Text className='cp-muted cp-small'>仅预览最近 {group.items.length} 项，共 {group.count} 项可读记录。</Text></View>}
        {overviewCollectionURL(value.project.id, group.kind) && <View className='cp-pad'><Action secondary onClick={() => void Taro.navigateTo({ url: overviewCollectionURL(value.project.id, group.kind)! })}>{'查看' + overviewLabels[group.kind]}</Action></View>}
        {group.kind === 'services' && <View className='cp-pad'><Text className='cp-muted cp-small'>这里展示服务配置；打开服务后核对当前健康状态。</Text></View>}
        {group.kind === 'payments' && <View className='cp-pad'><Text className='cp-muted cp-small'>按费用分配与合同现金归属关联；登记金额不计入发生支出。</Text></View>}
      </>}
    </Section>)}
  </>}{!page.loading && !value && <Empty>概览未核实，请重新读取。</Empty>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新项目概览</Action></Page>;
}
