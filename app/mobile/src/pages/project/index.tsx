import { View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { Input } from '../../platform/components';
import { useCollection } from '../../platform/collection';
import { go, identifier, projectResourceForm, usePage } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Field, Notice, Page, Row, Section, State, date } from '../../ui';
import ProjectSetupPage from './setup';
import { ProjectMembers } from './members';
import ResourceFormPage from './resource-forms';
import ProjectOverviewPage from './overview';
import ProjectBusinessPage from './business';
import TaskFormPage from './task-forms';
import ProjectCloseoutPage from './closeout';

const titles = { milestones: '项目里程碑', risks: '项目风险', issues: '项目问题', members: '项目成员', deliverables: '项目交付' };
type Kind = keyof typeof titles;
type Item = Schemas['Milestone'] | Schemas['Risk'] | Schemas['Issue'] | Schemas['Deliverable'];
const singular = { milestones: 'milestone', risks: 'risk', issues: 'issue', deliverables: 'deliverable' };

export default function ProjectSectionPage() {
  const params = useRouter().params;
  if (params.section === 'setup') return <ProjectSetupPage projectID={params.id} />;
  if (params.section === 'resource-form') return <ResourceFormPage kind={params.kind} projectID={params.id} resourceID={params.resource_id} />;
  if (params.section === 'task-form') return <TaskFormPage projectID={params.id} taskID={params.task_id} parentID={params.parent_id} />;
  if (params.section === 'closeout') return <ProjectCloseoutPage projectID={params.id} />;
  if (params.section === 'overview') return <ProjectOverviewPage projectID={params.id} />;
  if (params.section === 'changes' || params.section === 'budgets') return <ProjectBusinessPage projectID={params.id} section={params.section} focus={params.focus} />;
  return <ProjectRecordSection />;
}

function ProjectRecordSection() {
  const params = useRouter().params, kind = params.section as Kind;
  const page = usePage(async scope => {
    if (!Object.prototype.hasOwnProperty.call(titles, kind)) throw new MobileError('INVALID_RESOURCE', '项目资料地址无效，请从项目重新打开。');
    const id = identifier(params.id);
    const project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
    return { project, members };
  });
  const value = page.result?.value;
  return <Page title={titles[kind] ?? '项目资料'} home><Notice error={page.error} />{page.loading ? <Busy /> : value && <>
    <Section title={value.project.name}><Fact label='项目阶段'><State value={value.project.state} /></Fact><View className='cp-pad'><Action secondary onClick={() => go('project', value.project.id)}>查看项目概况</Action></View></Section>
    {kind === 'members' ? <ProjectMembers key={value.project.id + ':' + value.project.version + ':' + page.result!.me.authorization_version} project={value.project} members={value.members} me={page.result!.me} onCommitted={() => void page.reload()} />
      : <ProjectItems key={value.project.id + ':' + kind} kind={kind} project={value.project} members={value.members} me={page.result!.me} />}
  </>}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新项目资料</Action>
  </Page>;
}

function ProjectItems({ kind, project, members, me }: { kind: Exclude<Kind, 'members'>; project: Schemas['Project']; members: Schemas['ProjectMembers']; me: Schemas['Me'] }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState('');
  const collection = useCollection<Item>('/projects/' + project.id + '/' + kind + '?limit=20' + (query ? '&q=' + encodeURIComponent(query) : ''));
  const viewer = members.memberships?.find(member => member.user_id === me.user.id)?.project_role === 'viewer';
  const canCreate = !viewer && !['closing', 'archived', 'cancelled'].includes(project.state) && me.actions.includes(singular[kind] + '.create');
  return <>
    {canCreate && (kind === 'deliverables' ? <Action onClick={() => void Taro.navigateTo({ url: '/pages/deliverable/index?project_id=' + project.id })}>新建项目交付</Action> : <Action onClick={() => projectResourceForm(singular[kind] as 'milestone' | 'risk' | 'issue', project.id)}>新建{({ milestones: '里程碑', risks: '风险', issues: '问题' })[kind]}</Action>)}
    <Field label='查找项目资料'><Input ariaLabel='查找项目资料' className='cp-input' value={search} maxlength={200} onInput={event => setSearch(event.detail.value)} /><Action secondary loading={collection.loading} onClick={() => { setQuery(search.trim()); collection.reload(); }}>查找项目资料</Action></Field>
    <Notice error={collection.error} />{collection.loading ? <Busy /> : <Section title={titles[kind]}>{collection.items.length ? collection.items.map(item => <Row key={item.id} title={'name' in item ? item.name : item.title} state={item.state} subtitle={'acceptance_criteria' in item ? item.protocol_version === 0 ? '旧记录 · 来源待核实' : '固定文件版本与人工验收' : 'due_at' in item ? '截止 ' + date(item.due_at) : undefined} onClick={() => go(singular[kind], item.id)} />) : !collection.error && <Empty>暂无符合条件的可见资料。</Empty>}</Section>}
    {Boolean(collection.error) && <Action secondary onClick={collection.reload}>重新读取列表</Action>}
    {collection.next && <Action secondary loading={collection.moreBusy} onClick={() => void collection.more()}>加载更多项目资料</Action>}
  </>;
}
