import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { MobileError, type Schemas } from '../../platform/session';
import { memberRoleNames, memberPayload, ownerPayload, projectReadOnly, projectVersionReceipt } from '../../platform/project-input';
import { SelectField, TextField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Section } from '../../ui';

type Project = Schemas['Project'];
export function ProjectMembers({ project, members, me, onCommitted }: { project: Project; members: Schemas['ProjectMembers']; me: Schemas['Me']; onCommitted(): void }) {
  const [mode, setMode] = useState<'member' | 'owner'>();
  const currentRole = members.memberships?.find(item => item.user_id === me.user.id)?.project_role;
  const canManage = !projectReadOnly(project) && currentRole !== 'viewer' && me.actions.includes('project.manage_members');
  return <>
    <Section title='当前成员'>{members.items.length ? members.items.map(user => <Fact label={memberRoleNames[members.memberships?.find(item => item.user_id === user.id)?.project_role ?? 'member']} key={user.id}><Text>{user.display_name}</Text>{user.id === project.owner_id && <Text className='cp-muted cp-small'>负责本项目</Text>}</Fact>) : <Empty>暂无可见成员。</Empty>}</Section>
    {canManage && <Section title='成员与负责人安排'><View className='cp-pad'><Text className='cp-small cp-paragraph'>成员角色不等于业务权限。角色变更会撤销受影响成员的旧登录；未闭环事项须按原规则处理后才能移除。</Text><Action secondary onClick={() => setMode(mode === 'member' ? undefined : 'member')}>配置或移除项目成员</Action>{me.actions.includes('project.update') && <Action secondary onClick={() => setMode(mode === 'owner' ? undefined : 'owner')}>移交项目负责人</Action>}
      {mode && <MemberEditor key={project.id + ':' + project.version + ':' + mode} mode={mode} project={project} members={members} me={me} onCommitted={onCommitted} />}
    </View></Section>}
  </>;
}

function MemberEditor({ mode, project, members, me, onCommitted }: { mode: 'member' | 'owner'; project: Project; members: Schemas['ProjectMembers']; me: Schemas['Me']; onCommitted(): void }) {
  const directory = useRecord<Schemas['ProjectMembers']>('/projects/' + project.id + '/member-candidates');
  const draft = useFormDraft(me, { kind: mode === 'owner' ? 'project-owner' : 'project-member', id: project.id, version: project.version }, 'member', { owner: '' });
  const [error, setError] = useState<unknown>();
  const userID = draft.fields.owner ?? '', action = draft.fields.action;
  const command = useCommand<Project>(() => { draft.forget(); onCommitted(); }, draft.binding, value => projectVersionReceipt(value, project, mode === 'owner' ? userID : project.owner_id));
  const disabled = command.busy || command.blocked || directory.loading || !directory.value;
  const candidates = directory.value?.items.filter(user => user.status === 'active' && user.id !== project.owner_id && (mode === 'owner' || user.id !== me.user.id)) ?? [];
  const selected = candidates.find(user => user.id === userID), existing = members.memberships?.find(item => item.user_id === userID);
  return <View><DraftNotice restored={draft.restored} /><Notice error={directory.error ?? error ?? command.error} /><CommandNotice blocked={command.blocked} />{directory.loading ? <Busy /> : directory.value && <>
    <Text className='cp-muted cp-small'>项目基础版本 {project.version}</Text>
    <SelectField label={mode === 'owner' ? '接任项目负责人' : '待配置项目成员'} value={userID} options={[[ '', '请选择实际有效用户' ], ...candidates.map(user => [user.id, user.display_name] as const)]} onChange={owner => draft.update({ owner })} disabled={disabled} />
    {userID && !selected && <Text className='cp-notice'>原选择不再是当前有效候选，请重新选择。</Text>}
    {selected && <Fact label='当前项目角色'><Text>{existing ? memberRoleNames[existing.project_role] : '尚非项目成员'}</Text></Fact>}
    {mode === 'member' ? <><SelectField label='本次成员安排' value={action} options={[[ 'member', '设为项目成员' ], [ 'manager', '设为项目管理员' ], [ 'viewer', '设为只读成员' ], [ 'remove', '移除项目成员' ]]} onChange={value => draft.update({ action: value })} disabled={disabled} /><TextField name='project-member-reason' label='成员变更原因' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={disabled} max={200} /></> : <Text className='cp-notice cp-paragraph'>确认后，所选用户成为项目负责人，原负责人保留项目管理员身份；已有业务职责不自动移交。</Text>}
    <Action loading={command.busy} disabled={disabled || !selected} onClick={() => { try { if (!selected) throw new Error('请重新选择实际有效用户。'); setError(undefined); void command.submit('/projects/' + project.id + (mode === 'owner' ? '' : '/members'), mode === 'owner' ? ownerPayload(project, userID) : memberPayload(project, members, me, userID, action, draft.fields.reason), mode === 'owner' ? 'PATCH' : 'POST'); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对本次项目资料。')); } }}>{mode === 'owner' ? '确认移交项目负责人' : '确认本次成员变更'}</Action>
  </>}{Boolean(directory.error) && <Action secondary onClick={onCommitted}>重新读取项目及可配置成员</Action>}</View>;
}
