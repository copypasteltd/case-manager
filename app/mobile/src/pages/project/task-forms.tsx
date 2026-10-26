import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { browse, go, identifier, projectTaskForm, usePage } from '../../platform/page';
import { currentTaskInput, dependencyPayload, dependencyProof, dependencyTaskReceipt, taskEditable, taskPayload, taskReceipt, taskWritable, type TaskInput } from '../../platform/task-input';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, State } from '../../ui';

type Task = Schemas['Task'];
type Context = { project: Schemas['Project']; members: Schemas['ProjectMembers']; task?: Task; parent?: Task };
const priorities = [['low', '低'], ['normal', '普通'], ['high', '高'], ['urgent', '紧急']] as const;
const problem = (cause: unknown) => new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对本次任务资料。');

async function readContext(projectID: string | undefined, scope: Scope): Promise<Context> {
  const id = identifier(projectID), project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
  const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
  return { project, members };
}

export function ProjectTaskList({ projectID }: { projectID?: string }) {
  const page = usePage(scope => readContext(projectID, scope)), value = page.result?.value, me = page.result?.me;
  return <Page title='项目任务' home><Notice error={page.error} />{page.loading ? <Busy /> : value && me && <>
    <Section title={value.project.name}><Fact label='项目阶段'><State value={value.project.state} /></Fact></Section>
    <TaskList key={value.project.id + ':' + value.project.version + ':' + me.authorization_version} {...value} me={me} />
    <Action secondary onClick={() => go('project', value.project.id)}>查看所属项目</Action>
  </>}<Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取项目权限与任务</Action></Page>;
}

function TaskList({ project, members, me }: Context & { me: Schemas['Me'] }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState(''), [state, setState] = useState('');
  const tasks = useCollection<Task>('/projects/' + project.id + '/tasks?limit=20' + (query ? '&q=' + encodeURIComponent(query) : '') + (state ? '&state=' + state : ''));
  return <>
    {taskWritable(project, members, me) && me.actions.includes('task.create') && <Action onClick={() => projectTaskForm(project.id)}>新建任务</Action>}
    <Section title='查找项目任务'><View className='cp-pad'>
      <TextField name='task-search' label='任务标题关键词' value={search} onChange={setSearch} disabled={tasks.loading} />
      <SelectField label='任务状态' value={state} options={[[ '', '全部状态' ], ['todo', '待开始'], ['in_progress', '进行中'], ['in_review', '待验收'], ['blocked', '受阻'], ['done', '已完成'], ['cancelled', '已取消']]} onChange={setState} disabled={tasks.loading} />
      <Action secondary loading={tasks.loading} onClick={() => { setQuery(search.trim()); tasks.reload(); }}>查找任务</Action>
    </View></Section>
    <Notice error={tasks.error} />{tasks.loading ? <Busy /> : <Section title='当前可读任务'>{tasks.items.length ? tasks.items.map(task => <Row key={task.id} title={task.title} state={task.state} subtitle={(priorities.find(value => value[0] === task.priority)?.[1] ?? '普通') + '优先级' + (task.parent_id ? ' · 子任务' : '')} onClick={() => go('task', task.id)} />) : !tasks.error && <Empty>没有符合条件的可读任务。</Empty>}</Section>}
    {Boolean(tasks.error) && <Action secondary onClick={tasks.reload}>重新读取任务列表</Action>}
    {tasks.next && <Action secondary loading={tasks.moreBusy} onClick={() => void tasks.more()}>加载更多任务</Action>}
  </>;
}

export default function TaskFormPage({ projectID, taskID, parentID }: { projectID?: string; taskID?: string; parentID?: string }) {
  const [saved, setSaved] = useState<Task>();
  const page = usePage(async scope => {
    setSaved(undefined);
    if (taskID && parentID) throw new MobileError('INVALID_RESOURCE', '编辑任务与新建子任务地址不能混用。');
    const value = await readContext(projectID, scope);
    const task = taskID ? await client.request<Task>('/tasks/' + identifier(taskID), { scope }) : undefined;
    const parent = parentID ? await client.request<Task>('/tasks/' + identifier(parentID), { scope }) : undefined;
    if ((task && task.project_id !== value.project.id) || (parent && (parent.project_id !== value.project.id || !taskEditable(parent)))) throw new MobileError('INVALID_RESOURCE', '任务与项目不匹配，或父任务已结束，请从所属项目重新打开。');
    return { ...value, task, parent };
  });
  const value = page.result?.value, me = page.result?.me;
  const allowed = value && me && taskWritable(value.project, value.members, me) && me.actions.includes(value.task ? 'task.update' : 'task.create') && (!value.task || taskEditable(value.task));
  return <Page title={taskID ? '编辑任务' : parentID ? '新建子任务' : '新建任务'} home><Notice error={page.error} />{page.loading ? <Busy /> : value && me && <>
    {saved ? <Section title='任务已保存'><Fact label='任务名称'><Text>{saved.title}</Text></Fact><Fact label='实际版本'><Text>{saved.version}</Text></Fact><Fact label='当前状态'><State value={saved.state} /></Fact><View className='cp-pad'><Action onClick={() => go('task', saved.id)}>查看已保存任务</Action></View></Section>
      : allowed ? <TaskEditor key={(value.task?.id ?? value.project.id) + ':' + (value.task?.version ?? value.project.version) + ':' + me.authorization_version + ':' + (parentID ?? '')} {...value} me={me} onCommitted={setSaved} /> : <Empty>当前权限、项目角色或任务阶段不允许创建或编辑。</Empty>}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取当前权限与任务</Action>
    <Action secondary onClick={() => browse('tasks', { project_id: value.project.id })}>查看任务列表核对结果</Action>
  </>}</Page>;
}

function TaskEditor({ project, members, task, parent: initialParent, me, onCommitted }: Context & { me: Schemas['Me']; onCommitted(value: Task): void }) {
  const draft = useFormDraft(me, { kind: task ? 'task-edit' : 'task-create', id: task?.id ?? project.id + ':' + (initialParent?.id ?? 'root'), version: task?.version ?? project.version }, 'save', { task: currentTaskInput(task, initialParent) });
  const input = draft.fields.task ?? currentTaskInput(task, initialParent), [choice, setChoice] = useState<'parent' | 'milestone'>(), [error, setError] = useState<unknown>();
  const parent = useRecord<Task>(input.parent ? '/tasks/' + input.parent : null), milestone = useRecord<Schemas['Milestone']>(input.milestone ? '/milestones/' + input.milestone : null);
  const command = useCommand<Task>(value => { draft.forget(); onCommitted(value); }, draft.binding, value => taskReceipt(value, input, project, task));
  const disabled = command.busy || command.blocked;
  const referencePending = !!input.parent && input.parent !== task?.parent_id && parent.loading || !!input.milestone && input.milestone !== task?.milestone_id && milestone.loading;
  function change(key: keyof TaskInput, value: string) { setError(undefined); draft.update({ task: { ...input, [key]: value } }); }
  const assignees = members.items.filter(user => user.status === 'active' && (me.actions.includes('task.assign') || task || user.id === me.user.id));
  return <Section title='核对任务资料'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-muted cp-small'>{project.name}{task ? ' · 任务基础版本 ' + task.version : ' · 新任务按实际结果核对'}</Text>
    <TextField name='task-title' label='任务名称' value={input.title} onChange={value => change('title', value)} disabled={disabled} />
    <TextField name='task-body' label='描述与验收标准' value={input.body} onChange={value => change('body', value)} disabled={disabled} multiline max={20000} />
    <SelectField label='任务负责人' value={input.assignee} options={[[ '', '暂不分配' ], ...(!assignees.some(user => user.id === input.assignee) && input.assignee ? [[ input.assignee, '原负责人不可用，请核对' ] as const] : []), ...assignees.map(user => [user.id, user.display_name] as const)]} onChange={value => change('assignee', value)} disabled={disabled || !!task && !me.actions.includes('task.assign')} hint={!me.actions.includes('task.assign') ? task ? '当前身份可保留原负责人；重新分配需要额外权限。' : '当前身份可选择本人或暂不分配。' : undefined} />
    <SelectField label='任务优先级' value={input.priority} options={priorities} onChange={value => change('priority', value)} disabled={disabled} />
    <SelectField label='完成要求' value={input.review} options={[[ 'direct', '直接完成' ], ['required', '提交审核后完成' ]]} onChange={value => change('review', value)} disabled={disabled} />
    <TextField name='task-start' label='开始时间（香港时间）' value={input.start} onChange={value => change('start', value)} disabled={disabled} max={40} hint='例如2028-02-29 09:00；留空表示未排期。' />
    <TextField name='task-due' label='截止时间（香港时间）' value={input.due} onChange={value => change('due', value)} disabled={disabled} max={40} hint='留空表示没有截止时间；可明确清空已有时间。' />
    <Fact label='父任务'><Text>{!input.parent ? '无父任务' : parent.value?.project_id === project.id ? parent.value.title : parent.loading ? '正在核对…' : '关联资料当前不可读；原关联不会自动清空。'}</Text></Fact>
    <Action secondary disabled={disabled} onClick={() => setChoice(choice === 'parent' ? undefined : 'parent')}>选择父任务</Action>
    {input.parent && <Action secondary disabled={disabled} onClick={() => change('parent', '')}>解除父任务关联</Action>}
    <Fact label='里程碑'><Text>{!input.milestone ? '未关联' : milestone.value?.project_id === project.id ? milestone.value.title : milestone.loading ? '正在核对…' : '关联资料当前不可读；原关联不会自动清空。'}</Text></Fact>
    <Action secondary disabled={disabled} onClick={() => setChoice(choice === 'milestone' ? undefined : 'milestone')}>选择任务里程碑</Action>
    {input.milestone && <Action secondary disabled={disabled} onClick={() => change('milestone', '')}>解除任务里程碑关联</Action>}
    {choice === 'parent' && !disabled && <ChoiceList<Task> title='选择父任务' path={'/projects/' + project.id + '/tasks?limit=20'} label={value => value.title} selected={input.parent} eligible={value => value.project_id === project.id && value.id !== task?.id && taskEditable(value)} onSelect={value => { change('parent', value.id); setChoice(undefined); }} onClose={() => setChoice(undefined)} />}
    {choice === 'milestone' && !disabled && <ChoiceList<Schemas['Milestone']> title='选择任务里程碑' path={'/projects/' + project.id + '/milestones?limit=20'} label={value => value.title} selected={input.milestone} eligible={value => value.project_id === project.id && !['achieved', 'cancelled'].includes(value.state)} onSelect={value => { change('milestone', value.id); setChoice(undefined); }} onClose={() => setChoice(undefined)} />}
    <Action loading={command.busy} disabled={disabled || !!choice || referencePending} onClick={() => { try { const body = taskPayload(input, project, members, me, task, { parent: parent.value, milestone: milestone.value }); setError(undefined); void command.submit(task ? '/tasks/' + task.id : '/projects/' + project.id + '/tasks', body, task ? 'PATCH' : 'POST'); } catch (cause) { setError(problem(cause)); } }}>确认保存任务</Action>
  </View></Section>;
}

export function TaskManagement({ task, project, members, me, dependencies, onCommitted }: { task: Task; project: Schemas['Project']; members: Schemas['ProjectMembers']; me: Schemas['Me']; dependencies?: Schemas['TaskDependencies']; onCommitted(): void }) {
  const [adding, setAdding] = useState(false), [selected, setSelected] = useState<{ task: Task; action: 'add' | 'remove' }>();
  const writable = taskWritable(project, members, me), editable = writable && taskEditable(task) && me.actions.includes('task.update');
  const parent = useRecord<Task>(task.parent_id ? '/tasks/' + task.parent_id : null), milestone = useRecord<Schemas['Milestone']>(task.milestone_id ? '/milestones/' + task.milestone_id : null);
  return <>
    <Section title='任务资料与关联'><Fact label='优先级'><Text>{priorities.find(value => value[0] === task.priority)?.[1] ?? '未提供'}</Text></Fact>
      {task.parent_id && <View className='cp-pad'>{parent.value?.project_id === task.project_id ? <Action secondary onClick={() => go('task', parent.value!.id)}>查看父任务：{parent.value.title}</Action> : <Text className='cp-muted'>父任务当前不可读，请重新核对。</Text>}</View>}
      {task.milestone_id && <View className='cp-pad'>{milestone.value?.project_id === task.project_id ? <Action secondary onClick={() => go('milestone', milestone.value!.id)}>查看里程碑：{milestone.value.title}</Action> : <Text className='cp-muted'>关联里程碑当前不可读，请重新核对。</Text>}</View>}
      <View className='cp-pad'>{editable && <Action secondary onClick={() => projectTaskForm(project.id, task.id)}>编辑任务资料</Action>}{writable && taskEditable(task) && me.actions.includes('task.create') && <Action secondary onClick={() => projectTaskForm(project.id, undefined, task.id)}>新建子任务</Action>}</View>
    </Section>
    {editable && dependencies && <Section title='管理前置任务'><View className='cp-pad'><Text className='cp-muted cp-small'>只列出当前可读前置任务。新增、移除均需独立核对；完成与循环约束由原业务检查。</Text>
      <Action secondary onClick={() => { setAdding(!adding); setSelected(undefined); }}>添加前置任务</Action>
      {adding && <ChoiceList<Task> title='选择前置任务' path={'/projects/' + project.id + '/tasks?limit=20'} label={value => value.title} eligible={value => value.project_id === task.project_id && value.id !== task.id && !dependencies.items.some(edge => edge.predecessor_id === value.id)} onSelect={value => { setSelected({ task: value, action: 'add' }); setAdding(false); }} onClose={() => setAdding(false)} />}
      {dependencies.items.map(edge => <DependencyRemoval key={edge.predecessor_id} projectID={task.project_id} edge={edge} onSelect={value => { setSelected({ task: value, action: 'remove' }); setAdding(false); }} />)}
      {selected && <DependencyEditor key={task.id + ':' + task.version + ':' + selected.task.id + ':' + selected.action} {...{ task, project, me, onCommitted }} predecessor={selected.task} action={selected.action} />}
    </View></Section>}
  </>;
}

function DependencyRemoval({ projectID, edge, onSelect }: { projectID: string; edge: Schemas['TaskDependencies']['items'][number]; onSelect(value: Task): void }) {
  const value = useRecord<Task>('/tasks/' + edge.predecessor_id);
  return <Action secondary disabled={!value.value || value.value.project_id !== projectID || !!value.error} onClick={() => { if (value.value?.project_id === projectID) onSelect(value.value); }}>移除前置：{edge.title}</Action>;
}

function DependencyEditor({ task, project, me, predecessor, action, onCommitted }: { task: Task; project: Schemas['Project']; me: Schemas['Me']; predecessor: Task; action: 'add' | 'remove'; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: 'task-dependency', id: task.id + ':' + predecessor.id + ':' + action, version: task.version }, action);
  const [error, setError] = useState<unknown>();
  const command = useCommand<Task>(() => { draft.forget(); onCommitted(); }, draft.binding, async receipt => {
    if (!dependencyTaskReceipt(receipt, task)) return false;
    const scope = new Scope();
    try {
      const edges = await client.request<Schemas['TaskDependencies']>('/tasks/' + task.id + '/dependencies', { scope });
      const current = await client.request<Task>('/tasks/' + task.id, { scope });
      return dependencyProof(edges, current, receipt, predecessor, action);
    } finally { scope.close(); }
  });
  return <View className='cp-panel cp-pad'><Text className='cp-row-title'>{action === 'add' ? '确认添加前置任务' : '确认移除前置任务'}</Text><Text className='cp-paragraph'>{predecessor.title}</Text><State value={predecessor.state} /><Text className='cp-muted cp-small'>当前任务版本 {task.version}；只更改这一条依赖。</Text><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Action loading={command.busy} disabled={command.busy || command.blocked} onClick={() => { try { const body = dependencyPayload(task, predecessor, project, action); setError(undefined); void command.submit('/tasks/' + task.id + '/dependencies', body); } catch (cause) { setError(problem(cause)); } }}>确认{action === 'add' ? '添加' : '移除'}这一条前置任务</Action>
  </View>;
}
