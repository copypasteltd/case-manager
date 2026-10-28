import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, date } from './components';

type Binding = Schemas['MiniappBinding'];
const names = { wechat: '微信小程序', douyin: '抖音小程序' };
export function MiniappBindings() {
  const cache = useQueryClient(), [selected, setSelected] = useState<Binding | null>(null);
  const bindings = useInfiniteQuery({ queryKey: ['own-miniapp-bindings'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Schemas['MiniappBindingListResponse']['data']>('/me/miniapp-bindings?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: value => value.next_cursor ?? undefined });
  const rows = bindings.data?.pages.flatMap(page => page.items) ?? [];
  async function refresh() { await cache.invalidateQueries({ queryKey: ['own-miniapp-bindings'] }); }
  return <section className="section-block" aria-label="我的小程序绑定"><div className="section-heading"><h2>我的小程序绑定</h2><button className="button secondary" disabled={bindings.isFetching} onClick={() => void refresh()}>刷新绑定</button></div><p className="muted">首次绑定请在对应小程序使用内部账号登录，进入账号安全并确认当前密码。已设置验证器的账号还须完成最近10分钟内的两步验证。Web 可查看和解除本人的绑定。</p><ErrorNotice error={bindings.error} />{bindings.isPending ? <Busy /> : !rows.length ? <Empty title="暂无平台身份绑定" detail="进入公司的微信或抖音小程序，在账号安全中确认绑定。" /> : <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="平台身份绑定记录"><table><thead><tr><th>平台 / AppID</th><th>状态</th><th>绑定时间</th><th>操作</th></tr></thead><tbody>{rows.map(item => <tr key={item.id} data-binding-id={item.id}><td><strong>{names[item.platform]}</strong><br /><small>{item.app_id}</small></td><td>{item.revoked_at ? '已解绑' : '已绑定'}{item.revoked_at && <><br /><small>{date(item.revoked_at)}</small></>}</td><td>{date(item.created_at)}</td><td>{!item.revoked_at && <button className="text-button danger-text" onClick={() => setSelected(item)}>解除绑定</button>}</td></tr>)}</tbody></table></div></div>}{bindings.hasNextPage && <button className="button secondary" disabled={bindings.isFetchingNextPage} onClick={() => void bindings.fetchNextPage()}>加载更多绑定</button>}{selected && <RevokeBinding item={selected} onClose={() => setSelected(null)} onDone={async () => { setSelected(null); await refresh(); await cache.invalidateQueries({ queryKey: ['own-sessions'] }); }} />}</section>;
}
function RevokeBinding({ item, onClose, onDone }: { item: Binding; onClose(): void; onDone(): Promise<void> }) {
  const [key] = useState(() => crypto.randomUUID());
  const revoke = useMutation({ mutationFn: () => api<Schemas['MiniappBinding']>('/me/miniapp-bindings/' + item.id + '/revoke', { method: 'POST', key, body: { expected_version: item.version, reason: '本人解除不再使用的平台身份绑定' } }), onSuccess: onDone });
  return <Dialog title="解除平台身份绑定" busy={revoke.isPending} onClose={onClose}><p>{names[item.platform]} · {item.app_id}。确认后，该绑定产生的登录、刷新凭证和待完成验证会立即失效。绑定历史会保留。</p><ErrorNotice error={revoke.error} /><div className="dialog-actions"><button className="button secondary" disabled={revoke.isPending} onClick={onClose}>取消</button><button className="button danger" disabled={revoke.isPending} onClick={() => revoke.mutate()}>确认解绑</button></div></Dialog>;
}
