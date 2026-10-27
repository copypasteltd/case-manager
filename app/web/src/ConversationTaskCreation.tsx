import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { makeTaskCreation, type Mutation } from '@copypaste/contracts/mutation';
import { api, type Page, type Schemas } from './api';
import { ErrorNotice, Field } from './components';

export function ConversationTaskCreation({ project, members, disabled, onChange }: {
  project: Schemas['Project']; members?: Schemas['ProjectMembers']; disabled: boolean; onChange(value: Mutation): void;
}) {
  const [title, setTitle] = useState(''), [body, setBody] = useState(''), [assignee, setAssignee] = useState('');
  const [parent, setParent] = useState(''), [milestone, setMilestone] = useState(''), [priority, setPriority] = useState('');
  const [review, setReview] = useState(''), [start, setStart] = useState(''), [due, setDue] = useState('');
  const [startMode, setStartMode] = useState('clear'), [dueMode, setDueMode] = useState('clear'), [reason, setReason] = useState('');
  const [error, setError] = useState<unknown>();
  const parents = useInfiniteQuery({ queryKey: ['task-create-parents', project.id], initialPageParam: '', gcTime: 0,
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['Task']>>('/projects/' + project.id + '/tasks?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const milestones = useInfiniteQuery({ queryKey: ['task-create-milestones', project.id], initialPageParam: '', gcTime: 0,
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['Milestone']>>('/projects/' + project.id + '/milestones?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const dates = [['开始时刻', startMode, setStartMode, start, setStart], ['截止时刻', dueMode, setDueMode, due, setDue]] as const;
  return <>
    <ErrorNotice error={error ?? parents.error ?? milestones.error} />
    <Field label="任务标题"><input value={title} maxLength={400} disabled={disabled} onChange={e => setTitle(e.target.value)} /></Field>
    <Field label="完整任务说明"><textarea rows={4} value={body} maxLength={40000} disabled={disabled} onChange={e => setBody(e.target.value)} /></Field>
    <Field label="任务负责人"><select value={assignee} disabled={disabled} onChange={e => setAssignee(e.target.value)}><option value="">明确暂不分配负责人</option>{members?.items.filter(m => m.status === 'active').map(m => <option key={m.id} value={m.id}>{m.display_name}</option>)}</select></Field>
    <Field label="父任务"><select value={parent} disabled={disabled || parents.isFetching} onChange={e => setParent(e.target.value)}><option value="">独立任务，不设置父任务</option>{parents.data?.pages.flatMap(p => p.items).filter(t => !['done', 'cancelled'].includes(t.state)).map(t => <option key={t.id} value={t.id}>{t.title}</option>)}</select></Field>
    {parents.hasNextPage && <button type="button" className="button secondary" disabled={disabled || parents.isFetchingNextPage} onClick={() => void parents.fetchNextPage()}>加载更多父任务</button>}
    <Field label="关联里程碑"><select value={milestone} disabled={disabled || milestones.isFetching} onChange={e => setMilestone(e.target.value)}><option value="">明确不关联里程碑</option>{milestones.data?.pages.flatMap(p => p.items).filter(m => m.state === 'open' || m.state === 'missed').map(m => <option key={m.id} value={m.id}>{m.title}</option>)}</select></Field>
    {milestones.hasNextPage && <button type="button" className="button secondary" disabled={disabled || milestones.isFetchingNextPage} onClick={() => void milestones.fetchNextPage()}>加载更多关联里程碑</button>}
    {dates.map(([label, mode, setMode, value, setValue]) => <div key={label}><Field label={label + '处理'}><select value={mode} disabled={disabled} onChange={e => setMode(e.target.value)}><option value="clear">明确不设置</option><option value="set">设置准确时刻</option></select></Field>{mode === 'set' && <Field label={label}><input value={value} disabled={disabled} placeholder="2027-03-05T18:00:00.123456+08:00" onChange={e => setValue(e.target.value)} /></Field>}</div>)}
    <Field label="优先级"><select value={priority} disabled={disabled} onChange={e => setPriority(e.target.value)}><option value="">请明确选择优先级</option><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></Field>
    <Field label="需要人工验收"><select value={review} disabled={disabled} onChange={e => setReview(e.target.value)}><option value="">请明确选择验收要求</option><option value="true">需要人工验收</option><option value="false">无需人工验收</option></select></Field>
    <Field label="本次创建原因"><input value={reason} maxLength={400} disabled={disabled} onChange={e => setReason(e.target.value)} /></Field>
    <p className="form-note">请核对全部九项资料。发送后只创建一个待处理任务，新版本从1开始。父任务表示子任务归属；前置依赖和人工验收请在任务页面办理。</p>
    <button type="button" className="button secondary" disabled={disabled} onClick={() => { try {
      if (review !== 'true' && review !== 'false') throw new Error('请明确选择人工验收要求。');
      onChange(makeTaskCreation(project, { title, body, assignee_id: assignee || null, parent_id: parent || null, milestone_id: milestone || null, start_at: startMode === 'clear' ? null : start, due_at: dueMode === 'clear' ? null : due, priority, review_required: review === 'true' }, reason));
    } catch (cause) { setError(cause); } }}>将这项明确更新加入请求</button>
  </>;
}
