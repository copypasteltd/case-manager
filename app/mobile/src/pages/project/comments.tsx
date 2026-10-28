import { Text, View } from '@tarojs/components';
import { useEffect, useId, useState } from 'react';
import Taro from '@tarojs/taro';
import { taskCommentPayload, taskCommentReceipt, type TaskComment } from '@copypaste/contracts/task-comment';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { client } from '../../platform/taro';
import { Scope, type Page as ResultPage, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Notice, Row, Section, date } from '../../ui';
import { TextField } from '../business/controls';
import { SourceCard, SourcePicker } from '../knowledge/sources';
type Task = Schemas['Task'];
type Source = Schemas['ResourceRef'];
type Candidate = Schemas['MentionCandidate'];

export function TaskComments({ task, me, focusID = '' }: { task: Task; me: Schemas['Me']; focusID?: string }) {
  const focusAnchor = 'task-comment-focus-' + useId().replace(/[^a-zA-Z0-9_-]/g, '');
  const [positionNotice, setPositionNotice] = useState(false);
  const project = useRecord<Schemas['Project']>('/projects/' + task.project_id);
  const members = useRecord<Schemas['ProjectMembers']>('/projects/' + task.project_id + '/members');
  const comments = useCollection<TaskComment>('/tasks/' + task.id + '/comments?limit=20');
  const validFocus = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(focusID);
  const focus = useRecord<TaskComment>(validFocus ? '/comments/' + focusID : null);
  const [creating, setCreating] = useState(false), [editing, setEditing] = useState<TaskComment>(), [history, setHistory] = useState<TaskComment>();
  const readonly = !project.value || !members.value || ['closing', 'archived', 'cancelled'].includes(project.value.state) || members.value.memberships?.find(m => m.user_id === me.user.id)?.project_role === 'viewer';
  function done() { setCreating(false); setEditing(undefined); comments.reload(); }
  useEffect(() => {
    if (!focus.value || focus.value.resource_type !== 'task' || focus.value.resource_id !== task.id || !project.value || !members.value) return;
    let closed = false;
    const timer = setTimeout(() => { void Taro.pageScrollTo({ selector: '#' + focusAnchor, duration: 150 }).catch(() => { if (!closed) setPositionNotice(true); }); }, 80);
    return () => { closed = true; clearTimeout(timer); };
  }, [focus.value?.id, project.value?.id, members.value, task.id, focusAnchor]);
  const render = (c: TaskComment, exact = false) => <View id={exact ? focusAnchor : undefined} key={(exact ? 'exact:' : '') + c.id} className='cp-pad'>
    <Text className='cp-row-title'>{c.author_display_name}</Text><Text className='cp-muted cp-small'>{date(c.created_at)} · 评论修订 {c.revision}{exact ? ' · 通知指定讨论' : ''}</Text>
    <Text selectable className='cp-paragraph'>{c.body}</Text>
    {!!c.mentioned_user_ids?.length && <Text className='cp-muted cp-small'>明确提及 {c.mentioned_user_ids.map(id => members.value?.items.find(m => m.id === id)?.display_name ?? '有权项目成员').join('、')}</Text>}
    {c.mentions_filtered && <Text className='cp-muted cp-small'>部分原提及当前不可查阅；文字修改保留原记录。</Text>}
    {c.attachment_versions.map(id => <SourceCard key={id} versionID={id} />)}
    {!readonly && c.author_id === me.user.id && <Action secondary onClick={() => { setEditing(c); setCreating(false); }}>修改我的任务评论</Action>}
    <Action secondary onClick={() => setHistory(c)}>查看任务评论历史</Action>
  </View>;
  return <Section title='任务讨论与提及'>
    <Text className='cp-muted cp-small cp-pad'>明确选择的有权项目成员会收到站内提醒。正文中的 @文字不会自动通知；修改文字保留固定附件和原提及，评论修订保留历史。</Text>
    <Notice error={project.error ?? members.error ?? comments.error ?? focus.error} />
    {focusID && !validFocus && <Text className='cp-notice'>讨论地址无效，请从通知中心重新打开。</Text>}
    {focus.loading && <Busy />}
    {positionNotice && <Text className='cp-notice'>指定讨论已加载，当前页面未能自动定位，请向下查阅“通知指定讨论”。</Text>}
    {focus.value && (focus.value.resource_type === 'task' && focus.value.resource_id === task.id ? render(focus.value, true) : <Text className='cp-notice'>指定讨论与当前任务不一致。</Text>)}
    {comments.loading ? <Busy /> : comments.items.length ? comments.items.map(c => render(c)) : !comments.error && <Empty>此任务暂无讨论。</Empty>}
    {comments.next && <Action secondary loading={comments.moreBusy} onClick={() => void comments.more()}>更多任务讨论</Action>}
    <Action secondary onClick={comments.reload}>重新读取任务讨论</Action>
    {!readonly && <Action secondary onClick={() => { setCreating(true); setEditing(undefined); }}>新增任务评论</Action>}
    {(creating || editing) && <TaskCommentEditor key={editing?.id ?? 'new'} task={task} me={me} before={editing} onDone={done} onClose={() => { setCreating(false); setEditing(undefined); }} />}
    {history && <TaskCommentHistory comment={history} onClose={() => setHistory(undefined)} />}
  </Section>;
}

function TaskCommentEditor({ task, me, before, onDone, onClose }: { task: Task; me: Schemas['Me']; before?: TaskComment; onDone(): void; onClose(): void }) {
  const draft = useFormDraft(me, { kind: before ? 'task-comment-edit' : 'task-comment-create', id: before?.id ?? task.id, version: before?.revision ?? task.version }, 'task-comment', { body: before?.body ?? '' });
  const body = draft.fields.body ?? '';
  const [sources, setSources] = useState<Source[]>([]), [picker, setPicker] = useState(false), [replaceFiles, setReplaceFiles] = useState(false), [replaceMentions, setReplaceMentions] = useState(false);
  const [selected, setSelected] = useState<Candidate[]>([]), [query, setQuery] = useState(''), [search, setSearch] = useState('');
  const candidates = useCollection<Candidate>(!before || replaceMentions ? '/tasks/' + task.id + '/mention-candidates?limit=20&q=' + encodeURIComponent(query) : null);
  const [preview, setPreview] = useState<ReturnType<typeof taskCommentPayload>>(), [confirmed, setConfirmed] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand<TaskComment>(() => { draft.forget(); onDone(); }, draft.binding, async value => {
    if (!preview || !taskCommentReceipt(value, task.id, me.user.id, preview, before)) return false;
    const scope = new Scope();
    try {
      const exact = await client.request<TaskComment>('/comments/' + value.id, { scope });
      const history = await client.request<ResultPage<TaskComment>>('/comments/' + value.id + '/history?limit=20', { scope });
      return taskCommentReceipt(exact, task.id, me.user.id, preview, before) && history.items.some(c => c.id === value.id && taskCommentReceipt(c, task.id, me.user.id, preview, before))
        && (!before || history.items.some(c => c.id === before.id && c.body === before.body && c.revision === before.revision));
    } finally { scope.close(); }
  });
  const disabled = command.busy || command.blocked;
  function reset() { setPreview(undefined); setConfirmed(false); }
  return <View className='cp-pad'><Text className='cp-section-title'>{before ? '修改任务评论 · 新修订' : '新增任务评论'}</Text><DraftNotice restored={draft.restored} />
    <TextField name='task-comment-body' label='任务评论文字' value={body} onChange={value => { draft.update({ body: value }); reset(); }} disabled={disabled} multiline max={40000} />
    {before && <><Check checked={replaceFiles} disabled={disabled} onChange={value => { setReplaceFiles(value); setSources([]); setPicker(false); reset(); }}>明确替换全部原附件（不选新附件表示清空）</Check>
      <Check checked={replaceMentions} disabled={disabled || before.mentions_filtered} onChange={value => { setReplaceMentions(value); setSelected([]); reset(); }}>明确替换全部原提及（不选成员表示清空）</Check>
      <Text className='cp-muted cp-small'>默认只修改文字。若原附件或成员当前不可核对，服务器会拒绝显式替换；可保留原关联继续修改文字。</Text></>}
    {(!before || replaceFiles) && <><Action secondary disabled={disabled} onClick={() => setPicker(!picker)}>{picker ? '收起任务附件选择' : '选择任务评论固定附件'}</Action>
      {picker && <SourcePicker projectID={task.project_id} value={sources} onChange={value => { setSources(value); reset(); }} disabled={disabled} fixedOnly max={20} />}
      {!picker && sources.map(source => <SourceCard key={source.version_id} source={source} />)}</>}
    {(!before || replaceMentions) && <Section title='明确选择提及成员'>
      <TextField name='task-mention-search' label='搜索有权项目成员' value={search} onChange={setSearch} max={200} disabled={disabled} />
      <Action secondary disabled={disabled} onClick={() => setQuery(search.trim())}>搜索提及成员</Action><Notice error={candidates.error} />
      <Text className='cp-muted cp-small'>已选 {selected.length}/20：{selected.map(c => c.display_name).join('、') || '无'}。仅新增提及产生提醒。</Text>
      {candidates.loading ? <Busy /> : candidates.items.length ? candidates.items.map(c => <Check key={c.id} checked={selected.some(item => item.id === c.id)} disabled={disabled || selected.length >= 20 && !selected.some(item => item.id === c.id)} onChange={value => { setSelected(old => value ? [...old.filter(item => item.id !== c.id), c] : old.filter(item => item.id !== c.id)); reset(); }}>{c.display_name}</Check>) : !candidates.error && <Empty>没有可选的其他有权成员。</Empty>}
      {candidates.next && <Action secondary disabled={disabled} loading={candidates.moreBusy} onClick={() => void candidates.more()}>更多提及成员</Action>}
      {!!selected.length && <Action secondary disabled={disabled} onClick={() => { setSelected([]); reset(); }}>清空本次提及选择</Action>}
    </Section>}
    <Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Action secondary disabled={disabled} onClick={() => { try { setError(undefined); setPreview(taskCommentPayload(task.id, body, before && !replaceFiles ? undefined : sources.map(s => s.version_id!), before && !replaceMentions ? undefined : selected.map(c => c.id), before)); setConfirmed(false); } catch (cause) { setError(cause); } }}>核对任务评论</Action>
    {preview && <Section title='核对本次任务评论'><Text selectable className='cp-paragraph'>{preview.body}</Text>
      <Text className='cp-muted cp-small'>固定附件：{preview.attachment_versions === undefined ? '保留全部原关联' : preview.attachment_versions.length + ' 个'}；提及：{preview.mentioned_user_ids === undefined ? '保留全部原关联' : selected.map(c => c.display_name).join('、') || '无'}。</Text>
      <Check checked={confirmed} disabled={disabled} onChange={setConfirmed}>已核对文字、固定附件及提及范围</Check>
      <Action loading={command.busy} disabled={disabled || !confirmed} onClick={() => void command.submit(before ? '/comments/' + before.id : '/tasks/' + task.id + '/comments', preview, before ? 'PATCH' : 'POST')}>{before ? '保存任务评论新修订' : '保存任务评论'}</Action>
    </Section>}
    <Action secondary disabled={command.busy} onClick={onClose}>关闭任务评论表单并核对讨论</Action>
  </View>;
}

function TaskCommentHistory({ comment, onClose }: { comment: TaskComment; onClose(): void }) {
  const history = useCollection<TaskComment>('/comments/' + comment.id + '/history?limit=20'), [selected, setSelected] = useState<TaskComment>();
  return <Section title='任务评论不可变历史'><Notice error={history.error} />{history.loading ? <Busy /> : history.items.map(c => <Row key={c.id} title={'评论修订 ' + c.revision} subtitle={date(c.created_at)} onClick={() => setSelected(c)} />)}
    {history.next && <Action secondary loading={history.moreBusy} onClick={() => void history.more()}>更多任务评论历史</Action>}
    {selected && <View className='cp-pad'><Text selectable className='cp-paragraph'>{selected.body}</Text><Text className='cp-muted cp-small'>当前可见提及 {selected.mentioned_user_ids?.length ?? 0} 人{selected.mentions_filtered ? '；部分原成员已过滤' : ''}</Text>{selected.attachment_versions.map(id => <SourceCard key={id} versionID={id} />)}</View>}
    <Action secondary onClick={onClose}>收起任务评论历史</Action>
  </Section>;
}
