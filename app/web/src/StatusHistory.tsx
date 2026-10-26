import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice } from './components';
import { sourceReasonNames } from './StatusShared';
import { statusTime } from './statusFreshness';

type Props = { service: Schemas['Service']; kind: 'observations' | 'maintenance' | Schemas['ServiceSource']; onClose: () => void; onCancelMaintenance: (window: Schemas['MaintenanceWindow']) => void };
export function ServiceHistory({ service, kind, onClose, onCancelMaintenance }: Props) {
  return <Dialog title={kind === 'observations' ? '观测事实' : kind === 'maintenance' ? '维护记录' : '来源修订历史'} onClose={onClose}><p><strong>{service.name}</strong></p>{kind === 'observations' ? <Observations service={service} /> : kind === 'maintenance' ? <Maintenance service={service} onCancel={onCancelMaintenance} /> : <Revisions service={service} source={kind} />}</Dialog>;
}
function Observations({ service }: { service: Schemas['Service'] }) {
  const [proofID, setProofID] = useState('');
  const query = useInfiniteQuery({ queryKey: ['service-history', service.id, 'observations'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['ServiceObservation']>>('/services/' + service.id + '/observations?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><p className="form-note">按接收时间倒序。迟到事实会留存，只有标记为“已应用”的样本参与当时的状态投影。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !items.length && !query.error ? <Empty title="尚无观测事实" detail="可信来源发送首条有效签名观测后会在此留存。" /> : items.map(item => <article className="status-history-entry" key={item.id}><div className="section-heading compact"><strong>{item.healthy ? '探测成功' : '探测失败'} · 序号 {item.sequence}</strong><span className={'badge ' + (item.applied ? 'info' : 'neutral')}>{item.applied ? '已应用' : '仅保留事实'}</span></div><p>观测于 {statusTime(item.observed_at)}（香港）</p><p>接收于 {statusTime(item.received_at)}（香港）</p><p>{item.latency_ms === null ? '未取得时延' : item.latency_ms + ' ms'}{item.ignored_reason && ' · ' + sourceReasonNames[item.ignored_reason]}</p><button className="text-button" onClick={() => setProofID(proofID === item.id ? '' : item.id)}>{proofID === item.id ? '收起验证材料' : '查看并验证原始签名'}</button>{proofID === item.id && <ObservationProof serviceID={service.id} observationID={item.id} />}</article>)}{query.hasNextPage && !query.error && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早观测</button>}</>;
}
function decodeBase64(value: string): Uint8Array<ArrayBuffer> {
  const text = atob(value), result = new Uint8Array(text.length);
  for (let index = 0; index < text.length; index++) result[index] = text.charCodeAt(index);
  return result;
}
function ObservationProof({ serviceID, observationID }: { serviceID: string; observationID: string }) {
  const query = useQuery({ queryKey: ['service-history', serviceID, 'proof', observationID], queryFn: async ({ signal }) => {
    const value = await api<Schemas['ServiceObservationProof']>('/services/' + serviceID + '/observations/' + observationID + '/evidence', { signal });
    if (value.id !== observationID) throw new Error('验证材料与所选观测不符，请重新核对。');
    let verification = '当前浏览器无法完成签名校验，请导出验证材料后核对。';
    try {
      const payload = decodeBase64(value.signed_payload), prefix = new TextEncoder().encode('CP-STATUS-V1\n' + value.source_id.toLowerCase() + '\n');
      const message = new Uint8Array(prefix.length + payload.length); message.set(prefix); message.set(payload, prefix.length);
      const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', payload))].map(byte => byte.toString(16).padStart(2, '0')).join('');
      const key = await crypto.subtle.importKey('raw', decodeBase64(value.public_key), 'Ed25519', false, ['verify']);
      const signed = await crypto.subtle.verify('Ed25519', key, decodeBase64(value.signature), message);
      verification = signed && digest === value.payload_sha256 ? '签名及原始字节摘要校验通过。此结果只证明该条历史事实，不代表当前健康状态。' : '签名或摘要校验未通过，请停止使用这条材料并联系负责人核对。';
    } catch { /* Display an explicit unsupported/unconfirmed outcome, never success. */ }
    return { value, verification };
  } });
  function download() {
    if (!query.data || query.error) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(query.data.value, null, 2)], { type: 'application/json' }));
    const link = document.createElement('a'); link.href = url; link.download = 'observation-' + observationID + '.json'; document.body.appendChild(link); link.click(); link.remove(); setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return <div className="status-proof"><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在取得准确原始材料" /> : !query.error && query.data && <><p role="status">{query.data.verification}</p><dl className="file-facts"><div><dt>原始字节 SHA-256</dt><dd className="status-digest">{query.data.value.payload_sha256}</dd></div><div><dt>来源修订</dt><dd className="status-digest">{query.data.value.source_revision_id}</dd></div></dl><button className="button secondary" onClick={download}>导出这条验证材料</button></>}</div>;
}
function Maintenance({ service, onCancel }: { service: Schemas['Service']; onCancel: Props['onCancelMaintenance'] }) {
  const query = useInfiniteQuery({ queryKey: ['service-history', service.id, 'maintenance'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['MaintenanceWindow']>>('/services/' + service.id + '/maintenance-windows?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !items.length && !query.error ? <Empty title="尚无维护记录" detail="安排或取消维护时会保存不可改写的记录。" /> : items.map(item => <article className="status-history-entry" key={item.id}><div className="section-heading compact"><strong>{statusTime(item.start_at)} — {statusTime(item.end_at)}</strong><span className="badge neutral">{item.cancelled_at ? '已取消' : '原定窗口'}</span></div><p>香港时间 · {item.reason}</p>{item.cancelled_at ? <p>取消于 {statusTime(item.cancelled_at)} · {item.cancellation_reason}</p> : service.can_manage && service.lifecycle === 'active' && <button className="text-button" onClick={() => onCancel(item)}>取消此维护窗口</button>}</article>)}{query.hasNextPage && !query.error && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早窗口</button>}</>;
}
function Revisions({ service, source }: { service: Schemas['Service']; source: Schemas['ServiceSource'] }) {
  const query = useInfiniteQuery({ queryKey: ['service-history', service.id, 'source', source.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['ServiceSourceRevision']>>('/services/' + service.id + '/sources/' + source.id + '/revisions?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><p className="form-note">{source.name} · 按修订序号倒序，历史公钥用于验证对应历史观测。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !items.length && !query.error ? <Empty title="尚无来源修订" detail="来源发布后的准确配置会在此留存。" /> : items.map(item => <article className="status-history-entry" key={item.id}><div className="section-heading compact"><strong>第 {item.seq} 版 · {item.name}</strong><span className="badge neutral">{item.is_current ? '当前修订' : '历史修订'}</span></div><p>{item.enabled ? '启用' : '停用'} · {item.provider} · {statusTime(item.created_at)}（香港）</p><p>{item.reason}</p><dl className="file-facts"><div><dt>外部参考</dt><dd>{item.external_ref}</dd></div><div><dt>探测/过期</dt><dd>{item.interval_seconds} / {item.stale_after_seconds} 秒</dd></div><div><dt>连续失败/恢复</dt><dd>{item.failure_count} / {item.recovery_count} 次</dd></div><div><dt>时延阈值</dt><dd>{item.latency_limit_ms} ms</dd></div><div><dt>公开验证密钥</dt><dd className="status-digest">{item.public_key}</dd></div></dl></article>)}{query.hasNextPage && !query.error && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早修订</button>}</>;
}
