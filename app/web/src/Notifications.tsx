import { useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { mergeNotifications, notificationCategories, notificationCount, notificationDestination, notificationReadInput, type Notification } from '@copypaste/contracts/notifications';
import { api, type Me, type Page } from './api';
import { Busy, date, Empty, ErrorNotice, Field, State } from './components';
import { useNotificationSummary } from './NotificationIndicator';
import NotificationDelivery from './NotificationDelivery';
import ContractReminders from './ContractReminders';
import NotificationTelegram from './NotificationTelegram';

export default function NotificationsPage({ me }: { me: Me }) {
  const [state, setState] = useState('all'), [category, setCategory] = useState(''), [opening, setOpening] = useState<string>(), [openError, setOpenError] = useState<unknown>();
  const cache = useQueryClient(), navigate = useNavigate(), summary = useNotificationSummary(me);
  const keys = useRef(new Map<string, string>());
  const query = useInfiniteQuery({ queryKey: ['notifications', me.user.id, me.authorization_version, state, category], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Notification>>('/notifications?' + new URLSearchParams({ state, category, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 30000, gcTime: 0 });
  const items = query.error ? [] : mergeNotifications(query.data?.pages.flatMap(page => page.items) ?? []), counts = summary.error ? undefined : summary.data;
  const refresh = () => Promise.all([cache.invalidateQueries({ queryKey: ['notifications', me.user.id] }), cache.invalidateQueries({ queryKey: ['notifications-summary', me.user.id] })]);
  const read = useMutation({ mutationFn: async (item: Notification) => {
    const body = notificationReadInput(item), signature = item.id + ':' + body.delivery_seq;
    if (!keys.current.has(signature)) keys.current.set(signature, crypto.randomUUID());
    const result = await api<Notification>('/notifications/' + item.id + '/read', { method: 'POST', body, key: keys.current.get(signature) });
    notificationReadInput(result);
    if (result.id !== item.id || result.read_seq < body.delivery_seq) throw new Error('已读结果尚未核对，请刷新通知。');
    keys.current.delete(signature); return result;
  }, onSettled: refresh });
  async function open(item: Notification) {
    if (opening) return; setOpening(item.id); setOpenError(undefined);
    try { const current = await api<Notification>('/notifications/' + item.id); if (current.id !== item.id) throw new Error('通知与所选记录不一致。'); navigate(notificationDestination(current.target).web); }
    catch (cause) { setOpenError(cause); await refresh(); }
    finally { setOpening(undefined); }
  }
  return <><div className="page-heading"><div><span className="eyebrow">WORKSPACE / NOTIFICATIONS</span><h1>通知中心</h1><p className="muted">查看当前与你相关的工作进展，打开资料后继续处理。</p></div><button className="button secondary" onClick={() => void refresh()} disabled={query.isFetching}>刷新通知</button></div>
    {counts && <section className="panel conversation-summary" aria-label="通知概览"><p>未读 <strong>{notificationCount(counts.unread_count, counts.count_complete)}</strong> · 仍需处理 <strong>{notificationCount(counts.attention_count, counts.count_complete)}</strong></p><p className="form-note">截至 {date(counts.as_of)}。已读仅表示你已查看通知，业务是否完成以相关资料为准。{!counts.count_complete && '计数已达核对上限，带 + 的数字表示至少这些条。'}</p></section>}
    <div className="party-filters"><Field label="处理范围"><select value={state} onChange={e => setState(e.target.value)}><option value="all">全部通知</option><option value="unread">仅未读</option><option value="attention">仍需处理</option></select></Field><Field label="通知类型"><select value={category} onChange={e => setCategory(e.target.value)}><option value="">全部类型</option>{Object.entries(notificationCategories).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></Field></div>
    <ErrorNotice error={query.error ?? summary.error ?? read.error ?? openError} />{query.isPending ? <Busy label="正在读取通知" /> : !query.error && <section className="panel conversation-summary" aria-label="通知列表">{items.length ? items.map(item => <article className="conversation-source-preview" key={item.id} data-notification-id={item.id}><div className="section-heading"><h2>{item.title}</h2><span className={'badge ' + (item.read ? 'neutral' : 'info')}>{item.read ? '已读' : '未读'}</span></div><p>{item.resource_label}</p><p><State state={item.current_state} /> {item.attention_required ? '仍需处理' : '查看进展'} · {date(item.updated_at)}</p><div className="form-actions"><button className="button" disabled={!!opening} onClick={() => void open(item)}>{opening === item.id ? '正在核对…' : '打开相关资料'}</button>{!item.read && <button className="button secondary" disabled={read.isPending} onClick={() => read.mutate(item)}>标记已读</button>}</div></article>) : <Empty title="当前没有符合条件的通知" detail="新通知会随业务进展进入这里。" />}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早通知</button>}</section>}
    <NotificationTelegram me={me} />
    {me.actions.includes('notification.delivery.read') && <NotificationDelivery me={me} />}
    {me.actions.includes('notification.policy.read') && <ContractReminders me={me} />}
  </>;
}
