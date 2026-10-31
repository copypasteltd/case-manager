import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { checkedFollowups, followupHints, followupLabels, followupState, followupURL, type FollowupSummary } from '@copypaste/contracts/followups';
import { api, type Me } from './api';
import { Busy, Empty, ErrorNotice, date } from './components';

export function Followups({ me }: { me: Me }) {
  const query = useQuery({ queryKey: ['business-followups', me.user.id, me.authorization_version], queryFn: async ({ signal }) => checkedFollowups(await api<FollowupSummary>('/dashboard/followups', { signal })), gcTime: 0, retry: false, refetchInterval: 30000, refetchOnWindowFocus: 'always' });
  const value = query.error || query.isFetching ? undefined : query.data;
  return <section className="section-block" aria-label="申请与交付跟进">
    <div className="section-heading"><div><span className="eyebrow">KEEP WORK MOVING</span><h2>申请与交付跟进</h2></div><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新申请与交付跟进</button></div>
    <p className="muted">按当前权限核对需要推进的申请和交付。此处只提供入口，业务结果以详情中的正式回执为准。</p>
    <ErrorNotice error={query.error} />{query.isFetching && <Busy label="正在核对申请与交付" />}
    {value && <><small className="muted">截至 {date(value.as_of)}</small><div className="project-grid">{value.groups.map(group => <section className="panel" key={group.kind} aria-label={followupLabels[group.kind]}><div className="section-heading"><h3>{followupLabels[group.kind]}</h3><strong aria-label={followupLabels[group.kind] + '数量'}>{group.count.toLocaleString()}</strong></div><p className="muted">{followupHints[group.kind]}</p>{group.items.length ? group.items.map(item => <Link className="todo-row" key={item.id} to={followupURL(group.kind, item, 'web')}><div><strong>{item.title}</strong><small>{followupState(item)}</small></div><span aria-hidden="true">↗</span></Link>) : <Empty title="当前没有符合条件的事项" detail="修订、送审或验收后，刷新核对当前状态。" />}{group.has_more && <p className="muted">仅列出最近更新的5条，共{group.count.toLocaleString()}条。请到对应业务或项目列表继续查阅。</p>}</section>)}</div></>}
  </section>;
}
