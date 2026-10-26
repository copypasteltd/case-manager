import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { go, identifier, projectResourceForm, projectSection, usePage } from '../../platform/page';
import { projectReadOnly } from '../../platform/project-input';
import { currentResourceInput, resourceActionLabels, resourceEditable, resourceLabels, resourcePayload, resourcePlural, resourceReceipt, resourceTransition, resourceTransitionReceipt, resourceTransitions, type ResourceInput, type ResourceItem, type ResourceKind } from '../../platform/project-resource-input';
import { MobileError, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { SelectField, TextField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Section, State, date } from '../../ui';

const severities = [['low', '低'], ['medium', '中'], ['high', '高'], ['critical', '严重']] as const;
type ResourceContext = { project: Schemas['Project']; members: Schemas['ProjectMembers']; item?: ResourceItem; kind: ResourceKind };
const writable = (project: Schemas['Project'], members: Schemas['ProjectMembers'], me: Schemas['Me']) => !projectReadOnly(project) && members.memberships?.find(member => member.user_id === me.user.id)?.project_role !== 'viewer';
const problem = (cause: unknown) => new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对本次事项资料。');

export default function ResourceFormPage({ kind, projectID, resourceID }: { kind?: string; projectID?: string; resourceID?: string }) {
  const [saved, setSaved] = useState<ResourceItem>();
  const validKind = kind === 'milestone' || kind === 'risk' || kind === 'issue' ? kind : undefined;
  const page = usePage<ResourceContext>(async scope => {
    setSaved(undefined);
    if (!validKind) throw new MobileError('INVALID_RESOURCE', '事项类型无效，请从所属项目重新打开。');
    const id = identifier(projectID), project = await client.request<Schemas['Project']>('/projects/' + id, { scope });
    const members = await client.request<Schemas['ProjectMembers']>('/projects/' + id + '/members', { scope });
    const item = resourceID ? await client.request<ResourceItem>('/' + resourcePlural[validKind] + '/' + identifier(resourceID), { scope }) : undefined;
    if (item && item.project_id !== id) throw new MobileError('INVALID_RESOURCE', '事项与所属项目不匹配，请重新打开。');
    return { project, members, item, kind: validKind };
  });
  const value = page.result?.value, me = page.result?.me;
  const allowed = value && me && writable(value.project, value.members, me) && me.actions.includes(value.kind + (value.item ? '.update' : '.create')) && (!value.item || resourceEditable(value.kind, value.item));
  const label = validKind ? resourceLabels[validKind] : '项目事项';
  return <Page title={(resourceID ? '编辑' : '新建') + label} home><Notice error={page.error} />{page.loading ? <Busy /> : value && me && <>
    {saved ? <Section title={label + '已保存'}><Fact label='标题'><Text>{saved.title}</Text></Fact><Fact label='当前状态'><State value={saved.state} /></Fact><Fact label='当前版本'><Text>{saved.version}</Text></Fact><Fact label='目标时间'><Text>{date(saved.due_at)}</Text></Fact><View className='cp-pad'><Action onClick={() => go(value.kind, saved.id)}>查看已保存事项</Action><Action secondary onClick={() => projectSection(value.project.id, resourcePlural[value.kind])}>返回项目事项列表</Action></View></Section>
      : allowed ? <ResourceEditor key={value.kind + ':' + (value.item?.id ?? value.project.id) + ':' + (value.item?.version ?? value.project.version) + ':' + me.authorization_version} {...value} me={me} onCommitted={setSaved} /> : <Empty>当前权限、项目角色或事项阶段不允许新建或编辑。</Empty>}
    <Action secondary loading={page.loading} onClick={() => void page.reload()}>重新读取当前权限与事项</Action>
    <Action secondary onClick={() => projectSection(value.project.id, resourcePlural[value.kind])}>查看项目列表核对结果</Action>
  </>}</Page>;
}

function ResourceEditor({ kind, project, members, item, me, onCommitted }: { kind: ResourceKind; project: Schemas['Project']; members: Schemas['ProjectMembers']; item?: ResourceItem; me: Schemas['Me']; onCommitted(value: ResourceItem): void }) {
  const draft = useFormDraft(me, { kind: item ? 'resource-edit' : 'resource-create', id: kind + ':' + (item?.id ?? project.id), version: item?.version ?? project.version }, 'save', { resource: currentResourceInput(project, item) });
  const input = draft.fields.resource ?? currentResourceInput(project, item), [error, setError] = useState<unknown>();
  const command = useCommand<ResourceItem>(value => { draft.forget(); onCommitted(value); }, draft.binding, value => resourceReceipt(value, kind, input, project, item));
  const disabled = command.busy || command.blocked, label = resourceLabels[kind], owners = members.items.filter(user => user.status === 'active');
  function change(key: keyof ResourceInput, value: string) { setError(undefined); draft.update({ resource: { ...input, [key]: value } }); }
  return <Section title={'核对' + label + '资料'}><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-muted cp-small'>{project.name}{item ? ' · 事项基础版本 ' + item.version : ' · 新记录按实际回执核对'}</Text>
    <TextField name='resource-title' label={label + '标题'} value={input.title} onChange={value => change('title', value)} disabled={disabled} />
    <SelectField label='事项负责人' value={input.owner} options={[[ '', '请选择实际有效项目成员' ], ...(!owners.some(user => user.id === input.owner) && input.owner ? [[ input.owner, '原负责人不可选，请重新核对' ] as const] : []), ...owners.map(user => [user.id, user.display_name] as const)]} onChange={value => change('owner', value)} disabled={disabled} />
    <TextField name='resource-due' label={(kind === 'milestone' ? '目标时间' : '截止时间') + '（香港时间）'} value={input.due} onChange={value => change('due', value)} disabled={disabled} max={40} hint={'例如2028-02-29 18:00' + (kind === 'milestone' ? '，必填' : item?.due_at ? '；已有时间可修改，暂不支持清空' : '；可留空')} />
    {kind !== 'milestone' && <><TextField name='resource-description' label='情况说明' value={input.description} onChange={value => change('description', value)} disabled={disabled} multiline max={20000} />
      <SelectField label='影响程度' value={input.severity} options={severities} onChange={value => change('severity', value)} disabled={disabled} />
      {kind === 'risk' ? <><SelectField label='发生概率' value={input.probability} options={[[ '', '未设置' ], ...severities.slice(0, 3)]} onChange={value => change('probability', value)} disabled={disabled} /><TextField name='resource-mitigation' label='应对措施' value={input.mitigation} onChange={value => change('mitigation', value)} disabled={disabled} multiline max={20000} /></> : <TextField name='resource-resolution' label='解决说明' value={input.resolution} onChange={value => change('resolution', value)} disabled={disabled} multiline max={20000} />}
    </>}
    <Action loading={command.busy} disabled={disabled} onClick={() => { try { const value = resourcePayload(kind, input, project, members, item); setError(undefined); void command.submit(value.path, value.body, value.method); } catch (cause) { setError(problem(cause)); } }}>确认保存{label}</Action>
  </View></Section>;
}

export function ResourceActions({ kind, item, project, members, me, onCommitted }: { kind: ResourceKind; item: ResourceItem; project: Schemas['Project']; members: Schemas['ProjectMembers']; me: Schemas['Me']; onCommitted(): void }) {
  const [action, setAction] = useState<string>();
  if (!writable(project, members, me) || !me.actions.includes(kind + '.update')) return null;
  const choices = resourceTransitions(kind, item).filter(value => value !== 'realized' || me.actions.includes('issue.create'));
  return <Section title={resourceLabels[kind] + '跟进'}><View className='cp-pad'>
    {resourceEditable(kind, item) && <Action secondary onClick={() => projectResourceForm(kind, project.id, item.id)}>编辑{resourceLabels[kind]}资料</Action>}
    {kind === 'risk' && item.state === 'realized' && <Text className='cp-notice'>此风险已发生，后续请在实际关联问题中跟进。</Text>}
    {choices.map(value => <Action secondary key={value} onClick={() => setAction(action === value ? undefined : value)}>{resourceActionLabels[value]}</Action>)}
    {action && <ResourceTransitionEditor key={item.id + ':' + item.version + ':' + action} {...{ kind, item, project, me, action, onCommitted }} />}
  </View></Section>;
}

function ResourceTransitionEditor({ kind, item, project, me, action, onCommitted }: { kind: ResourceKind; item: ResourceItem; project: Schemas['Project']; me: Schemas['Me']; action: string; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: 'resource-transition', id: kind + ':' + item.id + ':' + action, version: item.version }, action, { body: 'resolution' in item ? item.resolution ?? '' : '' });
  const [error, setError] = useState<unknown>();
  const resolution = draft.fields.body ?? '';
  const command = useCommand<ResourceItem>(() => { draft.forget(); onCommitted(); }, draft.binding, value => resourceTransitionReceipt(value, kind, item, action, resolution));
  const disabled = command.busy || command.blocked;
  return <View><DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Text className='cp-row-title'>{resourceActionLabels[action]}</Text><Text className='cp-paragraph'>{item.title} · 基础版本 {item.version}</Text>
    {action === 'achieve' && <Text className='cp-notice'>须存在已完成的关联任务或已验收交付，且全部关联事项均已完成。保存时重新核对。</Text>}
    {action === 'realized' && <Text className='cp-notice'>确认后创建同项目的关联问题，保留当前标题、说明、负责人、影响及期限。该风险之后只读，请在新问题中继续处理。</Text>}
    {kind === 'issue' && ['resolved', 'closed'].includes(action) && <TextField name='transition-resolution' label='本次解决说明' value={resolution} onChange={body => { setError(undefined); draft.update({ body }); }} multiline max={20000} disabled={disabled} />}
    <TextField name='transition-reason' label='事项变更原因' value={draft.fields.reason} onChange={reason => { setError(undefined); draft.update({ reason }); }} disabled={disabled} max={200} />
    <Action loading={command.busy} disabled={disabled} onClick={() => { try { const value = resourceTransition(kind, item, project, action, draft.fields.reason, resolution, me.actions); setError(undefined); void command.submit(value.path, value.body, value.method); } catch (cause) { setError(problem(cause)); } }}>确认{resourceActionLabels[action]}</Action>
  </View>;
}
