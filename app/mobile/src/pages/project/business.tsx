import { Text, View } from '@tarojs/components';
import { useCollection } from '../../platform/collection';
import { go, identifier, usePage } from '../../platform/page';
import { exactAmount, overviewState } from '../../platform/project-overview';
import type { Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, dateOnly } from '../../ui';

export default function ProjectBusinessPage({ projectID, section, focus }: { projectID: unknown; section: 'changes' | 'budgets'; focus?: string }) {
  const page = usePage(async scope => {
    const id = identifier(projectID);
    if (focus) identifier(focus);
    return client.request<Schemas['Project']>('/projects/' + id, { scope });
  });
  const project = page.result?.value;
  return <Page title={section === 'budgets' ? '项目预算修订' : '项目变更'} home><Notice error={page.error} />{page.loading ? <Busy /> : project && <>
    <Section title={project.name}><View className='cp-pad'><Text className='cp-muted cp-small'>只读核对当前有权资料。批准、实际实施、预算和发生支出分别判断。</Text><Action secondary onClick={() => go('project', project.id)}>返回项目资料</Action></View></Section>
    {section === 'budgets' ? <Budgets key={project.id} projectID={project.id} focus={focus} /> : <Changes key={project.id} projectID={project.id} />}
  </>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新项目业务</Action></Page>;
}
function Changes({ projectID }: { projectID: string }) {
  const list = useCollection<Schemas['Change']>('/projects/' + projectID + '/changes?limit=20');
  return <><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <Section title='当前可读变更'>{list.items.length ? list.items.map(change => <Row key={change.id} title={change.revision.content.title} subtitle={overviewState(change.state) + ' · 提案修订 ' + change.revision.seq} onClick={() => go('change', change.id)} />) : <Empty>暂无当前可读变更。</Empty>}</Section>}<Text className='cp-muted cp-small'>当前载入 {list.items.length} 项，后续资料按页读取。</Text>{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多项目变更</Action>}<Action secondary onClick={list.reload}>重新读取变更列表</Action></>;
}
function Budgets({ projectID, focus }: { projectID: string; focus?: string }) {
  const list = useCollection<Schemas['ProjectBudget']>('/projects/' + projectID + '/budgets?limit=20');
  return <><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <>
    {focus && !list.items.some(budget => budget.id === focus) && <View className='cp-notice'><Text>所选预算修订尚未载入或当前不可读；有后续页时请继续读取，不以其他修订替代。</Text></View>}
    {list.items.length ? list.items.map(budget => <Section key={budget.id} title={budget.currency + ' · 预算修订 ' + budget.revision + (budget.id === focus ? ' · 所选修订' : '')}><Fact label='修订状态'><Text>{overviewState(budget.state)}</Text></Fact><Fact label='修订金额'><Text>{budget.currency} {exactAmount(budget.total)}</Text><Text className='cp-muted cp-small'>仅 approved 为当前批准预算；草稿与已取代修订单独核对。</Text></Fact>{budget.lines.map((line, index) => <Fact key={index} label={line.category}><Text>{dateOnly(line.period_start)} — {dateOnly(line.period_end)}</Text><Text>{budget.currency} {exactAmount(line.amount)}</Text></Fact>)}<Fact label='准确预算ID'><Text selectable className='cp-code'>{budget.id}</Text></Fact><Row title='查看来源变更' subtitle={'来源提案修订 ' + budget.change_revision_id} onClick={() => go('change', budget.change_id)} /></Section>) : <Empty>暂无当前可读预算修订。</Empty>}
    <Text className='cp-muted cp-small'>预算按各类别与期间核对，不与全期间费用直接相减。当前载入 {list.items.length} 项。</Text>
  </>}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多预算修订</Action>}<Action secondary onClick={list.reload}>重新读取预算列表</Action></>;
}
