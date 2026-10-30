import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { resourceNames, resourceReasons } from '@copypaste/contracts/host-resources';
import { api, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice } from './components';
import { statusTime } from './statusFreshness';

type Event = Schemas['ResourceHealthEvent'];
const changes: Record<Event['change_kind'], string> = { initial: '首次评估', configuration: '配置变更', responsibility: '负责人变更', state: '资源状态变更' };
export function ResourceHealthHistory({ service, authority, onClose }: { service: string; authority: string; onClose(): void }) {
  const path = '/services/' + service + '/resource-monitor/';
  const [proof, setProof] = useState<string>();
  const feed = useInfiniteQuery({ queryKey: ['resource-health-history', service, authority], initialPageParam: '', gcTime: 0, retry: false, queryFn: ({ pageParam, signal }) => api<{ items: Event[]; next_cursor: string | null }>(path + 'health-events?limit=10' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const evidence = useQuery({ queryKey: ['resource-health-proof', service, authority, proof], enabled: !!proof && !feed.error, gcTime: 0, retry: false, queryFn: ({ signal }) => api<Schemas['ResourceProof']>(path + 'observations/' + proof + '/evidence', { signal }) });
  return <Dialog title="资源状态记录" onClose={onClose}><p className="form-note">以下为当时的评估与责任记录，不代表当前主机状态。通知事件已创建也不表示已经投递。</p>
    <ErrorNotice error={feed.error} />{feed.isPending ? <Busy label="读取资源状态记录" /> : !feed.error && !feed.data?.pages[0]?.items.length ? <Empty title="尚无状态记录" detail="发布资源配置或收到完整指标后开始保留记录。" /> : !feed.error && feed.data?.pages.flatMap(page => page.items).map(row => <article className="status-source" key={row.id} data-testid="resource-health-event">
      <h3>记录 {row.sequence} · {resourceNames[row.state]}</h3><p>{changes[row.change_kind]} · {row.previous_state === 'unconfigured' ? '尚未配置' : resourceNames[row.previous_state]} → {resourceNames[row.state]}</p>
      <p>{resourceReasons[row.state_reason] ?? '依据当时完整指标评估'} · {row.attention_required ? '当时需要负责人处理' : '当时无资源待办'}</p>
      <p className="form-note">评估于 {statusTime(row.evaluated_at)} · 当时负责人 {row.recipient_id}</p><p className="form-note">配置修订 {row.source_revision_id}</p>
      <p>{row.notification_event_created ? '已创建站内通知事件；实际投递结果请在通知中心核对。' : '本次仅保留状态记录，不发送站内提醒。'}</p>
      {row.observation_id && <button className="text-button" onClick={() => setProof(row.observation_id!)}>核对该记录的原始签名</button>}
    </article>)}
    <ErrorNotice error={evidence.error} />{proof && evidence.isFetching && <Busy label="核对该记录的签名证据" />}{proof && evidence.data && !evidence.error && !feed.error && <p className="notice success">原始载荷摘要及 Ed25519 签名已校验：{evidence.data.id} · {evidence.data.payload_sha256}</p>}
    {feed.hasNextPage && !feed.error && <button className="button secondary" disabled={feed.isFetchingNextPage} onClick={() => void feed.fetchNextPage()}>加载更早状态记录</button>}
  </Dialog>;
}
