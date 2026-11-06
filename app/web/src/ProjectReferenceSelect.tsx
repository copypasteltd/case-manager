import { useInfiniteQuery } from '@tanstack/react-query';
import { api, type Me, type Page, type Schemas, type Task } from './api';
import { Busy, ErrorNotice } from './components';

type Reference = Task | Schemas['Milestone'];
export function ProjectReferenceSelect({ projectID, kind, value, onChange, me, exclude = [], disabled = false, allowTerminal = false }: {
  projectID: string; kind: 'tasks' | 'milestones'; value: string; onChange(value: string): void; me: Me; exclude?: string[]; disabled?: boolean; allowTerminal?: boolean;
}) {
  const query = useInfiniteQuery({ queryKey: ['project-reference', projectID, kind, me.user.id, me.authorization_version], initialPageParam: '', gcTime: 0, retry: false,
    queryFn: ({ pageParam, signal }) => api<Page<Reference>>('/projects/' + projectID + '/' + kind + '?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined });
  const items = [...new Map((query.data?.pages.flatMap(page => page.items) ?? []).map(item => [item.id, item])).values()];
  const visible = items.filter(item => !exclude.includes(item.id) && (allowTerminal || item.id === value || !['done', 'cancelled', 'achieved'].includes(item.state)));
  return <div><ErrorNotice error={query.error} />{query.isFetching && <Busy label="正在读取可选关联" />}
    <select aria-label={kind === 'tasks' ? '任务关联' : '里程碑关联'} value={value} disabled={disabled || query.isFetching || !!query.error} onChange={event => onChange(event.target.value)}>
      <option value="">不关联</option>{value && !visible.some(item => item.id === value) && <option value={value}>保留当前关联（请在详情核对）</option>}
      {!query.error && visible.map(item => <option key={item.id} value={item.id}>{item.title} · v{item.version}</option>)}
    </select>{query.hasNextPage && <button type="button" className="text-button" disabled={disabled || query.isFetching || !!query.error} onClick={() => void query.fetchNextPage()}>继续读取可选关联</button>}
    <small className="muted">已读取 {items.length} 项{query.hasNextPage ? '，后续仍有记录' : '，已到列表末尾'}。保存时重新核对当前业务约束。</small>
  </div>;
}
