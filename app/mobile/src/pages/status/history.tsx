import { Text, View } from '@tarojs/components';
import { useMemo, useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import type { Schemas } from '../../platform/session';
import { observationReasons, statusTime } from '../../platform/status';
import { verifyStatusProof } from '../../platform/status-proof';
import { Action, Busy, Empty, Fact, Notice, Section } from '../../ui';

export function Observations({ serviceID }: { serviceID: string }) {
  const query = useCollection<Schemas['ServiceObservation']>('/services/' + serviceID + '/observations?limit=20');
  const [selected, setSelected] = useState<string>();
  return <Section title='观测事实与原始签名'><View className='cp-pad'><Text className='cp-muted cp-small cp-paragraph'>按接收时间倒序。迟到或过期样本保留为事实，不能作为当前健康结论。</Text><Notice error={query.error} />
    {query.loading ? <Busy /> : query.items.map(item => <View className='cp-editor-row' key={item.id}>
      <Text className='cp-row-title'>{item.healthy ? '探测成功' : '探测失败'} · 序号 {item.sequence}</Text>
      <Text className='cp-paragraph'>{item.applied ? '已应用于当时状态' : '仅保留事实'}{item.ignored_reason && ' · ' + observationReasons[item.ignored_reason]}</Text>
      <Fact label='观测时间'><Text>{statusTime(item.observed_at)}</Text></Fact><Fact label='接收时间'><Text>{statusTime(item.received_at)}</Text></Fact><Fact label='时延'><Text>{item.latency_ms === null ? '未取得时延' : item.latency_ms + ' ms'}</Text></Fact>
      <Action secondary onClick={() => setSelected(selected === item.id ? undefined : item.id)}>{selected === item.id ? '收起验证材料' : '查看并验证原始签名'}</Action>
      {selected === item.id && <Proof key={item.id} serviceID={serviceID} observation={item} />}
    </View>)}
    {!query.loading && !query.error && !query.items.length && <Empty>尚无可信来源观测，服务状态保持未知。</Empty>}
    {query.next && <Action secondary loading={query.moreBusy} onClick={() => void query.more()}>加载更早观测</Action>}
    <Action secondary onClick={query.reload}>重新读取观测事实</Action>
  </View></Section>;
}
function Proof({ serviceID, observation }: { serviceID: string; observation: Schemas['ServiceObservation'] }) {
  const query = useRecord<Schemas['ServiceObservationProof']>('/services/' + serviceID + '/observations/' + observation.id + '/evidence');
  const value = query.value, verified = useMemo(() => value ? verifyStatusProof(value, observation) : false, [value, observation]);
  return <View className='cp-status-proof'><Notice error={query.error} />{query.loading ? <Busy /> : value && <>
    <Text className={'cp-paragraph ' + (verified ? 'cp-status-verified' : 'cp-field-error')}>{verified ? '签名及原始字节摘要校验通过。' : '签名、摘要或所选观测匹配未通过，请停止使用此材料并联系负责人核对。'}</Text>
    <Text className='cp-muted cp-small cp-paragraph'>此结果只验证所选历史事实与服务器返回的历史公钥，不代表当前健康状态。</Text>
    <Fact label='原始字节 SHA-256'><Text selectable className='cp-code'>{value.payload_sha256}</Text></Fact>
    <Fact label='历史来源修订'><Text selectable className='cp-code'>{value.source_revision_id}</Text></Fact>
    <Fact label='历史公开验证密钥'><Text selectable className='cp-code'>{value.public_key}</Text></Fact>
    <Fact label='原始字节 Base64'><Text selectable className='cp-code'>{value.signed_payload}</Text></Fact>
    <Fact label='Ed25519 签名'><Text selectable className='cp-code'>{value.signature}</Text></Fact>
  </>}</View>;
}
export function Maintenance({ serviceID }: { serviceID: string }) {
  const query = useCollection<Schemas['MaintenanceWindow']>('/services/' + serviceID + '/maintenance-windows?limit=20');
  return <Section title='维护窗口历史'><View className='cp-pad'><Text className='cp-muted cp-small cp-paragraph'>以下为原定窗口与取消记录。是否正在覆盖观测，以重新读取的当前状态为准。</Text><Notice error={query.error} />
    {query.loading ? <Busy /> : query.items.map(item => <View className='cp-editor-row' key={item.id}>
      <Text className='cp-row-title'>{item.cancelled_at ? '已取消的窗口' : '原定维护窗口'}</Text>
      <Fact label='开始'><Text>{statusTime(item.start_at)}</Text></Fact><Fact label='结束'><Text>{statusTime(item.end_at)}</Text></Fact>
      <Fact label='维护原因'><Text>{item.reason}</Text></Fact>
      {item.cancelled_at && <><Fact label='取消时间'><Text>{statusTime(item.cancelled_at)}</Text></Fact><Fact label='取消原因'><Text>{item.cancellation_reason}</Text></Fact></>}
    </View>)}
    {!query.loading && !query.error && !query.items.length && <Empty>尚无维护窗口记录。</Empty>}
    {query.next && <Action secondary loading={query.moreBusy} onClick={() => void query.more()}>加载更早窗口</Action>}
    <Action secondary onClick={query.reload}>重新读取维护记录</Action>
  </View></Section>;
}
export function Revisions({ serviceID, source }: { serviceID: string; source: Schemas['ServiceSource'] }) {
  const query = useCollection<Schemas['ServiceSourceRevision']>('/services/' + serviceID + '/sources/' + source.id + '/revisions?limit=20');
  return <Section title={source.name + ' · 来源修订'}><View className='cp-pad'><Notice error={query.error} />
    {query.loading ? <Busy /> : query.items.map(item => <View className='cp-editor-row' key={item.id}>
      <Text className='cp-row-title'>第 {item.seq} 版 · {item.is_current ? '当前修订' : '历史修订'}</Text><Text className='cp-paragraph'>{item.name} · {item.enabled ? '启用' : '停用'}</Text>
      <Fact label='发布于'><Text>{statusTime(item.created_at)}</Text></Fact><Fact label='发布原因'><Text>{item.reason}</Text></Fact>
      <Fact label='探测／过期阈值'><Text>{item.interval_seconds}／{item.stale_after_seconds} 秒</Text></Fact><Fact label='连续失败／恢复'><Text>{item.failure_count}／{item.recovery_count} 次</Text></Fact>
      <Fact label='时延阈值'><Text>{item.latency_limit_ms} ms</Text></Fact><Fact label='外部参考'><Text>{item.external_ref}</Text></Fact><Fact label='历史公开验证密钥'><Text selectable className='cp-code'>{item.public_key}</Text></Fact>
      <Fact label='准确修订'><Text selectable className='cp-code'>{item.id}</Text></Fact>
    </View>)}
    {!query.loading && !query.error && !query.items.length && <Empty>尚无可见来源修订。</Empty>}
    {query.next && <Action secondary loading={query.moreBusy} onClick={() => void query.more()}>加载更早来源修订</Action>}
    <Action secondary onClick={query.reload}>重新读取来源历史</Action>
  </View></Section>;
}
