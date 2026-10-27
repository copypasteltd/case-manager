import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import ProjectBudgetComparison from './budget-comparison';
import { changeWriter } from '../../platform/change-input';
import { SelectField, TextField } from '../business/controls';
import { useCollection } from '../../platform/collection';
import { go, identifier, projectChangeForm, usePage } from '../../platform/page';
import { exactAmount, overviewState } from '../../platform/project-overview';
import type { Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, dateOnly } from '../../ui';

export default function ProjectBusinessPage({ projectID, section, focus }: { projectID: unknown; section: 'changes' | 'budgets'; focus?: string }) {
  const [comparison, setComparison] = useState(false);
  const page = usePage(async scope => {
    const id = identifier(projectID);
    if (focus) identifier(focus);
    const project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
    return { project, members };
  });
  const value = page.result?.value, project = value?.project;
  const canCreate = value && changeWriter(value.project, value.members, page.result!.me, 'change.create');
  return <Page title={section === 'budgets' ? '项目预算修订' : '项目变更'} home><Notice error={page.error} />{page.loading ? <Busy /> : project && <>
    <Section title={project.name}><View className='cp-pad'><Text className='cp-muted cp-small'>批准、实际实施、预算和发生支出分别核对。预算调整须通过完整项目变更提案。</Text><Action secondary onClick={() => go('project', project.id)}>返回项目资料</Action></View></Section>
    {canCreate && <Action onClick={() => projectChangeForm(project.id)}>{section === 'budgets' ? '提出完整预算调整' : '新建完整变更提案'}</Action>}
    {section === 'budgets' && <><Action secondary onClick={() => setComparison(old => !old)}>{comparison ? '收起预算对比' : '预算与实际支出对比'}</Action>{comparison && <ProjectBudgetComparison projectID={project.id} />}</>}
    {section === 'budgets' ? <Budgets key={project.id} projectID={project.id} focus={focus} /> : <Changes key={project.id} projectID={project.id} />}
  </>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新项目业务</Action></Page>;
}
function Changes({ projectID }: { projectID: string }) {
  const [search, setSearch] = useState(''), [state, setState] = useState(''), [filter, setFilter] = useState('');
  const list = useCollection<Schemas['Change']>('/projects/' + projectID + '/changes?limit=20' + filter);
  return <><Section title='查找项目变更'><View className='cp-pad'>
    <TextField name='change-search' label='查找变更名称' value={search} disabled={list.loading} onChange={setSearch} />
    <SelectField label='变更状态筛选' value={state} options={[[ '', '全部可读状态' ], [ 'draft', '草稿' ], [ 'in_review', '审批中' ], [ 'approved', '已批准尚未实施' ], [ 'rejected', '未通过' ], [ 'withdrawn', '已撤回' ], [ 'implemented', '已实施' ]]} disabled={list.loading} onChange={setState} />
    <Action secondary loading={list.loading} onClick={() => { setFilter((search.trim() ? '&q=' + encodeURIComponent(search.trim()) : '') + (state ? '&state=' + state : '')); list.reload(); }}>查找变更记录</Action>
  </View></Section><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <Section title='当前可读变更'>{list.items.length ? list.items.map(change => <Row key={change.id} title={change.revision.content.title} subtitle={overviewState(change.state) + ' · 提案修订 ' + change.revision.seq} onClick={() => go('change', change.id)} />) : <Empty>暂无当前可读变更。</Empty>}</Section>}<Text className='cp-muted cp-small'>当前载入 {list.items.length} 项，后续资料按页读取。</Text>{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多项目变更</Action>}<Action secondary onClick={list.reload}>重新读取变更列表</Action></>;
}
function Budgets({ projectID, focus }: { projectID: string; focus?: string }) {
  const [currency, setCurrency] = useState(''), [state, setState] = useState(''), [filter, setFilter] = useState('');
  const list = useCollection<Schemas['ProjectBudget']>('/projects/' + projectID + '/budgets?limit=20' + filter);
  return <><Section title='按币种与修订状态核对'><View className='cp-pad'>
    <TextField name='budget-currency' label='预算币种筛选' value={currency} max={3} disabled={list.loading} hint='三位大写币种，留空读取全部可读币种。' onChange={setCurrency} />
    <SelectField label='预算状态筛选' value={state} options={[[ '', '全部可读修订' ], [ 'draft', '提案草稿预算' ], [ 'approved', '当前批准预算' ], [ 'superseded', '已取代预算历史' ]]} disabled={list.loading} onChange={setState} />
    <Action secondary loading={list.loading} disabled={Boolean(currency) && !/^[A-Z]{3}$/.test(currency)} onClick={() => { setFilter((currency ? '&currency=' + currency : '') + (state ? '&state=' + state : '')); list.reload(); }}>筛选预算修订</Action>
  </View></Section><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && <>
    {focus && !list.items.some(budget => budget.id === focus) && <View className='cp-notice'><Text>所选预算修订尚未载入或当前不可读；有后续页时请继续读取，不以其他修订替代。</Text></View>}
    {list.items.length ? list.items.map(budget => <Section key={budget.id} title={budget.currency + ' · 预算修订 ' + budget.revision + (budget.id === focus ? ' · 所选修订' : '')}><Fact label='修订状态'><Text>{overviewState(budget.state)}</Text></Fact><Fact label='修订金额'><Text>{budget.currency} {exactAmount(budget.total)}</Text><Text className='cp-muted cp-small'>仅 approved 为当前批准预算；草稿与已取代修订单独核对。</Text></Fact>{budget.lines.map((line, index) => <Fact key={index} label={line.category}><Text>{dateOnly(line.period_start)} — {dateOnly(line.period_end)}</Text><Text>{budget.currency} {exactAmount(line.amount)}</Text></Fact>)}<Fact label='准确预算ID'><Text selectable className='cp-code'>{budget.id}</Text></Fact><Row title='查看来源变更' subtitle={'来源提案修订 ' + budget.change_revision_id} onClick={() => go('change', budget.change_id)} /></Section>) : <Empty>暂无当前可读预算修订。</Empty>}
    <Text className='cp-muted cp-small'>预算按各类别与期间核对，不与全期间费用直接相减。当前载入 {list.items.length} 项。</Text>
  </>}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多预算修订</Action>}<Action secondary onClick={list.reload}>重新读取预算列表</Action></>;
}
