import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { makeTaskDependency, mutationValue, type Mutation } from '@copypaste/contracts/mutation';
import { api, type Page, type Schemas } from './api';
import { Busy, ErrorNotice, Field } from './components';

export function ConversationTaskDependency({ task, disabled, onChange }: { task: Schemas['Task']; disabled: boolean; onChange(value: Mutation): void }) {
  const [selected, setSelected] = useState(''), [action, setAction] = useState(''), [reason, setReason] = useState(''), [error, setError] = useState<unknown>();
  const choices = useInfiniteQuery({ queryKey: ['dependency-predecessor-options', task.project_id], initialPageParam: '', gcTime: 0,
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['Task']>>('/projects/' + task.project_id + '/tasks?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: p => p.next_cursor ?? undefined });
  const predecessor = useQuery({ queryKey: ['dependency-predecessor', selected], enabled: !!selected, gcTime: 0, staleTime: 0, refetchOnWindowFocus: false,
    queryFn: ({ signal }) => api<Schemas['Task']>('/tasks/' + selected, { signal }) });
  const edges = useQuery({ queryKey: ['dependency-current', task.id, task.version], gcTime: 0, staleTime: 0,
    queryFn: ({ signal }) => api<Schemas['TaskDependencies']>('/tasks/' + task.id + '/dependencies', { signal }) });
  const current = predecessor.error ? undefined : predecessor.data;
  const busy = disabled || predecessor.isFetching || edges.isFetching || !edges.data || !current;
  return <>
    <ErrorNotice error={error ?? choices.error ?? predecessor.error ?? edges.error} />
    <p className="form-note">当前选中的任务是后续任务。新增依赖后，须先完成前置任务才能开始后续任务。每次仅调整一条关系。</p>
    <Field label="前置任务"><select value={selected} disabled={disabled || choices.isPending} onChange={e => { setSelected(e.target.value); setError(undefined); }}><option value="">请明确选择同项目另一项任务</option>{choices.data?.pages.flatMap(p => p.items).filter(t => t.id !== task.id).map(t => <option key={t.id} value={t.id}>{t.title} · {mutationValue(t.state, 'state')}</option>)}</select></Field>
    {choices.hasNextPage && <button type="button" className="button secondary" disabled={disabled || choices.isFetchingNextPage} onClick={() => void choices.fetchNextPage()}>加载更多前置任务</button>}
    {predecessor.isFetching && <Busy label="正在核对前置任务版本" />}
    {current && !predecessor.isFetching && <p>前置任务：{current.title} · 基础版本 {current.version} · {mutationValue(current.state, 'state')}</p>}
    {current && edges.data && <p>当前关系：{edges.data.items.some(item => item.predecessor_id === current.id) ? '存在依赖' : '没有依赖'}</p>}
    <Field label="依赖动作"><select value={action} disabled={disabled} onChange={e => { setAction(e.target.value); setError(undefined); }}><option value="">请明确选择</option><option value="add">新增依赖</option><option value="remove">解除依赖</option></select></Field>
    <Field label="本次依赖更新原因"><input maxLength={400} value={reason} disabled={disabled} onChange={e => { setReason(e.target.value); setError(undefined); }} /></Field>
    <p className="form-note">发送前核对两个任务及其版本。任务或关系变化后，请重新选择；循环依赖及任务状态继续由业务规则核对。</p>
    <button type="button" className="button secondary" disabled={busy} onClick={() => { try {
      if (!current || !edges.data) throw new Error('请读取两个任务和当前依赖。');
      onChange(makeTaskDependency(task, current, action, reason, edges.data.items.map(item => item.predecessor_id)));
    } catch (cause) { setError(cause); } }}>将这项明确更新加入请求</button>
  </>;
}
