import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { usePage, identifier, go, projectSection } from '../../platform/page';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { useRecord } from '../../platform/collection';
import { MobileError, type Schemas } from '../../platform/session';
import { currentProjectInput, projectControlled, projectReadOnly, projectPayload, projectReceipt, type ProjectInput } from '../../platform/project-input';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Section, State } from '../../ui';

type Project = Schemas['Project'];
type Template = Schemas['ProjectTemplate'];

export function ProjectSetupEntry({ project, me }: { project: Project; me: Schemas['Me'] }) {
  const members = useRecord<Schemas['ProjectMembers']>(!projectReadOnly(project) && me.actions.includes('project.update') ? '/projects/' + project.id + '/members' : null);
  if (!members.value || members.value.memberships?.find(item => item.user_id === me.user.id)?.project_role === 'viewer') return <Notice error={members.error} />;
  return <Action secondary onClick={() => projectSection(project.id, 'setup')}>编辑项目资料</Action>;
}

export default function ProjectSetupPage({ projectID }: { projectID?: string }) {
  const [saved, setSaved] = useState<Project>();
  const page = usePage(async scope => {
    setSaved(undefined);
    if (!projectID) return undefined;
    const project = await client.request<Project>('/projects/' + identifier(projectID), { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + project.id + '/members', { scope });
    return { project, members };
  });
  const result = page.result, value = result?.value;
  const allowed = result && (value ? !projectReadOnly(value.project) && value.members.memberships?.find(m => m.user_id === result.me.user.id)?.project_role !== 'viewer' && result.me.actions.includes('project.update') : result.me.actions.includes('project.create'));
  return <Page title={projectID ? '编辑项目资料' : '新建项目'} home><Notice error={page.error} />{page.loading ? <Busy /> : result && <>
    {saved ? <Section title={projectID ? '项目资料已保存' : '项目已创建'}><Fact label='项目名称'><Text>{saved.name}</Text></Fact><Fact label='项目编号'><Text>{saved.project_no}</Text></Fact><Fact label='当前阶段'><State value={saved.state} /></Fact><Fact label='当前版本'><Text>{saved.version}</Text></Fact><Fact label='初始化情况'><Text>{({ pending: '初始化待处理', complete: '目录及模板结构已初始化', partial_failed: '部分初始化未完成，请核对结果' })[saved.initialization_state ?? 'pending']}</Text></Fact><View className='cp-pad'><Action onClick={() => go('project', saved.id)}>查看已保存项目</Action><Action secondary onClick={() => projectSection(saved.id, 'members')}>安排项目成员</Action></View></Section>
      : allowed ? <ProjectEditor key={(value?.project.id ?? 'new') + ':' + (value?.project.version ?? 0) + ':' + result.me.authorization_version} project={value?.project} me={result.me} onCommitted={setSaved} /> : <Empty>当前权限、项目角色或阶段不允许立项或编辑。</Empty>}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取当前权限与项目</Action>
  </>}<Action secondary onClick={() => go('projects')}>查看项目列表核对结果</Action></Page>;
}

function ProjectEditor({ project, me, onCommitted }: { project?: Project; me: Schemas['Me']; onCommitted(value: Project): void }) {
  const draft = useFormDraft(me, { kind: project ? 'project-edit' : 'project-create', id: project?.id ?? me.user.id, version: project?.version ?? 1 }, 'save', { project: currentProjectInput(project) });
  const input = draft.fields.project ?? currentProjectInput(project);
  const [template, setTemplate] = useState<Template>(), [picking, setPicking] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand<Project>(value => { draft.forget(); onCommitted(value); }, draft.binding, value => projectReceipt(value, input, me, project, template));
  const disabled = command.busy || command.blocked, controlled = project && projectControlled(project);
  function change(key: keyof ProjectInput, value: string) { draft.update({ project: { ...input, [key]: value } }); }
  if (picking) return <ChoiceList<Template> title='选择已发布项目模板' path='/project-templates?state=published&limit=20' label={value => value.name + ' · ' + value.key + ' · 修订 ' + value.revision} selected={template?.id} eligible={value => value.state === 'published'} onSelect={value => { setTemplate(value); setPicking(false); }} onClose={() => setPicking(false)} />;
  return <Section title={project ? '核对当前项目资料' : '填写项目立项资料'}><View className='cp-pad'>
    <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {project && <Text className='cp-muted cp-small'>{project.project_no} · 基础版本 {project.version}</Text>}
    <TextField name='project-name' label='项目名称' value={input.name} onChange={value => change('name', value)} disabled={disabled} max={200} />
    <TextField name='project-type' label='项目类型' value={input.type} onChange={value => change('type', value)} disabled={disabled} max={100} />
    {project ? <Fact label='项目访问范围'><Text>{({ internal: '公司内部', project_restricted: '项目受限', confidential: '机密' })[project.classification ?? 'internal']}</Text></Fact> : <SelectField label='项目访问范围' value={input.classification} options={[[ 'project_restricted', '项目受限' ], [ 'internal', '公司内部' ]]} onChange={value => change('classification', value)} disabled={disabled} />}
    {controlled && <View className='cp-notice'><Text>进行中项目的范围与日期在此只读，调整须经项目变更独立审批后实施。</Text></View>}
    <TextField name='project-scope' label='项目范围与目标' value={input.scope} onChange={value => change('scope', value)} disabled={disabled || Boolean(controlled)} multiline max={20000} />
    <TextField name='project-start' label='项目开始日期' value={input.start} onChange={value => change('start', value)} disabled={disabled || Boolean(controlled)} max={10} hint='YYYY-MM-DD；留空表示未安排' />
    <TextField name='project-due' label='项目截止日期' value={input.due} onChange={value => change('due', value)} disabled={disabled || Boolean(controlled)} max={10} hint='YYYY-MM-DD；可留空，需不早于开始日期' />
    {!project && <><Text className='cp-small cp-paragraph'>你将成为项目负责人。创建后可配置成员或按权限移交负责人。</Text><Action secondary disabled={disabled} onClick={() => setPicking(true)}>{template ? '重新选择固定模板修订' : '选择已发布项目模板'}</Action>{template && <><TemplateFacts template={template} /><Action secondary disabled={disabled} onClick={() => setTemplate(undefined)}>取消使用模板</Action></>}</>}
    <Action disabled={disabled} loading={command.busy} onClick={() => { try { const payload = projectPayload(input, me, project, template); setError(undefined); void command.submit('/projects' + (project ? '/' + project.id : ''), payload, project ? 'PATCH' : 'POST'); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对本次项目资料。')); } }}>{project ? '确认保存项目资料' : '确认创建项目'}</Action>
  </View></Section>;
}

function TemplateFacts({ template }: { template: Template }) {
  return <Section title='本次固定模板内容'><Fact label='模板'><Text>{template.name} · {template.key} · 修订 {template.revision}</Text></Fact><Fact label='说明'><Text>{template.description || '未填写说明'}</Text></Fact><Fact label='目录'><Text>标准目录：合同、交付、工作资料；{template.schema.folders.join('、') || '无额外目录'}</Text></Fact>{template.schema.tasks.map((task, i) => <Fact key={i} label={'模板任务 ' + (i + 1)}><Text>{task.title} · {task.priority} · {task.review_required ? '需要验收' : '直接完成'}</Text><Text className='cp-paragraph'>{task.body}</Text></Fact>)}{template.schema.milestones.map((item, i) => <Fact key={i} label={'模板里程碑 ' + (i + 1)}><Text>{item.title}</Text></Fact>)}<Fact label='固定内容摘要'><Text className='cp-code'>{template.content_hash}</Text></Fact><Text className='cp-small cp-paragraph'>仅使用这份已发布修订。停用或冲突后须重新核对，不自动替换模板。</Text></Section>;
}
