import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { checkedWorkbench, workbenchHints, workbenchLabels, workbenchState, workbenchURL, type WorkbenchSummary } from '@copypaste/contracts/workbench';
import { api, type Me } from './api';
import { Busy, Empty, ErrorNotice, date } from './components';

export function BusinessWorkbench({ me }: { me: Me }) {
  const query = useQuery({ queryKey: ['business-workbench', me.user.id, me.authorization_version], queryFn: async ({ signal }) => checkedWorkbench(await api<WorkbenchSummary>('/dashboard/business', { signal })), gcTime: 0, retry: false, refetchInterval: 30000, refetchOnWindowFocus: 'always' });
  const value = query.error || query.isFetching ? undefined : query.data;
  return <section className="section-block" aria-label="业务待办汇总">
    <div className="section-heading"><div><span className="eyebrow">YOUR BUSINESS DESK</span><h2>业务待办</h2></div><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新业务待办</button></div>
    <p className="muted">汇总当前可读、由你负责的业务及你创建的文件存储待办。打开详情核对最新资料；处理后刷新确认。</p>
    <ErrorNotice error={query.error} />{query.isFetching && <Busy label="正在核对业务待办" />}
    {value && <><small className="muted">截至 {date(value.as_of)} · 合同日期按香港时间，至 {value.contract_through}</small><div className="project-grid">{value.groups.map(group => <section className="panel" key={group.kind} aria-label={workbenchLabels[group.kind]}><div className="section-heading"><h3>{workbenchLabels[group.kind]}</h3><strong aria-label={workbenchLabels[group.kind] + '数量'}>{group.count.toLocaleString()}</strong></div><p className="muted">{workbenchHints[group.kind]}</p>{group.items.length ? group.items.map(item => <Link className="todo-row" key={item.id} to={workbenchURL(group.kind, item, 'web')}><div><strong>{item.title}</strong><small>{workbenchState(item)}{item.due_on ? ' · ' + (group.kind === 'contracts' ? '到期 ' : '账期开始 ') + item.due_on : ''}</small></div><span aria-hidden="true">↗</span></Link>) : <Empty title="当前没有符合条件的事项" detail="此数量只反映你当前可见的负责事项。" />}{group.has_more && <p className="muted">仅列出前5条，请到对应业务列表继续查阅。</p>}</section>)}</div></>}
  </section>;
}
