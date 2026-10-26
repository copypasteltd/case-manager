import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { Textarea } from '../../platform/components';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { go } from '../../platform/page';
import type { Schemas } from '../../platform/session';
import { TaskActions } from '../detail/mutations';
import { ResourceActions } from './resource-forms';
import { Action, Busy, Empty, Fact, Field, Notice, Row, Section, date } from '../../ui';

const severity = { low: '低', medium: '中', high: '高', critical: '严重' };
export function ProjectResource({ kind, item, me, onCommitted }: { kind: 'milestone' | 'risk' | 'issue'; item: Schemas['Milestone'] | Schemas['Risk'] | Schemas['Issue']; me: Schemas['Me']; onCommitted(): void }) {
  const members = useRecord<Schemas['ProjectMembers']>('/projects/' + item.project_id + '/members');
  const project = useRecord<Schemas['Project']>('/projects/' + item.project_id);
  const owner = members.value?.items.find(user => user.id === item.owner_id)?.display_name;
  const risk = kind === 'risk' ? item as Schemas['Risk'] : undefined, issue = kind === 'issue' ? item as Schemas['Issue'] : undefined;
  return <><Notice error={members.error} /><Section title={kind === 'milestone' ? '里程碑资料' : kind === 'risk' ? '风险资料' : '问题资料'}>
    <Fact label='负责人'><Text>{owner ?? (members.loading ? '正在读取姓名…' : item.owner_id ? '当前姓名不可查阅' : '未分配')}</Text></Fact>
    <Fact label='截止时间'><Text>{date(item.due_at)}</Text>{'overdue' in item && item.overdue && <Text className='cp-notice'>已逾期</Text>}</Fact>
    {'description' in item && <Fact label='说明'><Text className='cp-paragraph'>{item.description || '暂未填写。'}</Text></Fact>}
    {'severity' in item && <Fact label='严重程度'><Text>{item.severity ? severity[item.severity] : '未设置'}</Text></Fact>}
    {risk && <><Fact label='发生概率'><Text>{risk.probability ? severity[risk.probability] : '未设置'}</Text></Fact><Fact label='缓解措施'><Text className='cp-paragraph'>{risk.mitigation || '暂未填写。'}</Text></Fact>{risk.issue_id && <View className='cp-pad'><Action secondary onClick={() => go('issue', risk.issue_id!)}>查看已发生的问题</Action></View>}</>}
    {issue && <Fact label='处理结果'><Text className='cp-paragraph'>{issue.resolution || '暂未填写。'}</Text></Fact>}
  </Section><Notice error={project.error} />{project.loading || members.loading ? <Busy /> : project.value && members.value && <ResourceActions {...{ kind, item, me, onCommitted }} project={project.value} members={members.value} />}
    <Action secondary onClick={() => go('project', item.project_id)}>查看所属项目</Action></>;
}

export function TaskCollaboration({ task, me, onCommitted }: { task: Schemas['Task']; me: Schemas['Me']; onCommitted(): void }) {
  const project = useRecord<Schemas['Project']>('/projects/' + task.project_id), members = useRecord<Schemas['ProjectMembers']>('/projects/' + task.project_id + '/members');
  const dependencies = useRecord<Schemas['TaskDependencies']>('/tasks/' + task.id + '/dependencies');
  const readonly = !project.value || !members.value || ['closing', 'archived', 'cancelled'].includes(project.value.state) || members.value.memberships?.find(member => member.user_id === me.user.id)?.project_role === 'viewer';
  return <>
    <Notice error={project.error ?? members.error} />
    <Section title='任务责任与前置条件'><Fact label='任务负责人'><Text>{members.value?.items.find(user => user.id === task.assignee_id)?.display_name ?? (task.assignee_id ? '当前姓名不可查阅' : '未分配')}</Text></Fact><Notice error={dependencies.error} />{dependencies.loading ? <Busy /> : dependencies.value?.items.length ? dependencies.value.items.map(edge => <Row key={edge.predecessor_id} title={edge.title} state={edge.state} subtitle='前置任务' onClick={() => go('task', edge.predecessor_id)} />) : !dependencies.error && <Empty>未设置前置任务。</Empty>}</Section>
    {project.loading || members.loading ? <Busy /> : readonly ? <View className='cp-notice'><Text>当前以只读方式查阅任务。</Text></View> : <TaskActions key={task.id + ':' + task.version} task={task} me={me} onCommitted={onCommitted} />}
    <TaskComments task={task} me={me} readonly={readonly} />
  </>;
}

function TaskComments({ task, me, readonly }: { task: Schemas['Task']; me: Schemas['Me']; readonly: boolean }) {
  const comments = useCollection<Schemas['Comment']>('/tasks/' + task.id + '/comments?limit=20');
  const [editing, setEditing] = useState<string>(), [history, setHistory] = useState<string>();
  const [composeVersion, setComposeVersion] = useState(0);
  const canWrite = !readonly && me.actions.includes('comment.write');
  return <Section title='任务讨论'><View className='cp-pad'><Notice error={comments.error} />
    {canWrite && <CommentEditor key={task.id + ':' + task.version + ':' + composeVersion} task={task} me={me} onCommitted={() => { setComposeVersion(value => value + 1); comments.reload(); }} />}
    {comments.loading ? <Busy /> : comments.items.length ? comments.items.map(comment => <View className='cp-comment' key={comment.id}>
      <Text className='cp-row-title'>{comment.author_display_name}</Text><Text className='cp-muted cp-small'>{date(comment.created_at)} · 修订 {comment.revision}</Text>
      <Text className='cp-paragraph' selectable>{comment.body}</Text>
      {!!comment.attachment_versions.length && <Text className='cp-muted cp-small'>固定附件 {comment.attachment_versions.length} 项；修改文字将保留原附件版本。</Text>}
      {canWrite && comment.author_id === me.user.id && <Action secondary onClick={() => setEditing(editing === comment.id ? undefined : comment.id)}>修改我的评论</Action>}
      {comment.revision > 1 && <Action secondary onClick={() => setHistory(history === comment.id ? undefined : comment.id)}>查看评论修订历史</Action>}
      {editing === comment.id && canWrite && <CommentEditor key={comment.id + ':' + comment.revision} task={task} comment={comment} me={me} onCommitted={() => { setEditing(undefined); comments.reload(); }} />}
      {history === comment.id && <CommentHistory id={comment.id} />}
    </View>) : !comments.error && <Empty>还没有讨论记录。</Empty>}
    {Boolean(comments.error) && <Action secondary onClick={comments.reload}>重新读取讨论</Action>}
    {comments.next && <Action secondary loading={comments.moreBusy} onClick={() => void comments.more()}>加载更多讨论</Action>}
  </View></Section>;
}

function CommentEditor({ task, comment, me, onCommitted }: { task: Schemas['Task']; comment?: Schemas['Comment']; me: Schemas['Me']; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: comment ? 'comment-edit' : 'comment-create', id: comment?.id ?? task.id, version: comment?.revision ?? task.version }, 'comment', { body: comment?.body ?? '' });
  const command = useCommand(() => { draft.forget(); onCommitted(); }, draft.binding), body = draft.fields.body ?? '';
  const disabled = command.busy || command.blocked;
  return <View><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    <Field label={comment ? '修改评论文字' : '写一条评论'}><Textarea ariaLabel={comment ? '修改评论文字' : '任务评论'} className='cp-textarea' value={body} maxlength={20000} disabled={disabled} onInput={event => draft.update({ body: event.detail.value })} /></Field>
    <Action loading={command.busy} disabled={disabled || !body.trim()} onClick={() => void command.submit(comment ? '/comments/' + comment.id : '/tasks/' + task.id + '/comments', comment ? { expected_revision: comment.revision, body: body.trim(), attachment_versions: comment.attachment_versions } satisfies Schemas['CommentPatch'] : { body: body.trim() } satisfies Schemas['CommentCreate'], comment ? 'PATCH' : 'POST')}>{comment ? '保存评论修订' : '发布任务评论'}</Action>
  </View>;
}

function CommentHistory({ id }: { id: string }) {
  const history = useCollection<Schemas['Comment']>('/comments/' + id + '/history?limit=20');
  return <View><Notice error={history.error} />{history.loading ? <Busy /> : history.items.map(comment => <Fact key={comment.id} label={'修订 ' + comment.revision + ' · ' + date(comment.created_at)}><Text className='cp-paragraph' selectable>{comment.body}</Text></Fact>)}{Boolean(history.error) && <Action secondary onClick={history.reload}>重新读取评论历史</Action>}{history.next && <Action secondary loading={history.moreBusy} onClick={() => void history.more()}>加载更多评论历史</Action>}</View>;
}
