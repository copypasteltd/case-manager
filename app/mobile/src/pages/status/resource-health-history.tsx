import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { resourceNames, resourceReasons } from '@copypaste/contracts/host-resources';
import { useCollection, useRecord } from '../../platform/collection';
import type { Schemas } from '../../platform/session';
import { statusTime } from '../../platform/status';
import { Action, Busy, Empty, Fact, Notice, Section } from '../../ui';

type Event = Schemas['ResourceHealthEvent'];
const changes: Record<Event['change_kind'], string> = { initial: '首次评估', configuration: '配置变更', responsibility: '负责人变更', state: '资源状态变更' };
export function ResourceHealthHistory({ service, onClose }: { service: string; onClose(): void }) {
  const path = '/services/' + service + '/resource-monitor/', [proof, setProof] = useState<string>();
  const feed = useCollection<Event>(path + 'health-events?limit=10');
  const evidence = useRecord<Schemas['ResourceProof']>(proof ? path + 'observations/' + proof + '/evidence' : null);
  return <Section title='资源状态记录'><View className='cp-pad'><Text className='cp-notice cp-paragraph'>以下为当时的评估与责任记录，不代表当前主机状态。通知事件已创建也不表示已经投递。</Text>
    <Notice error={feed.error} />{feed.loading ? <Busy /> : !feed.items.length && !feed.error ? <Empty>尚无状态记录。</Empty> : !feed.error && feed.items.map(row => <View key={row.id} className='cp-editor-row'>
      <Text className='cp-row-title'>记录 {row.sequence} · {resourceNames[row.state]}</Text><Text className='cp-paragraph'>{changes[row.change_kind]} · {row.previous_state === 'unconfigured' ? '尚未配置' : resourceNames[row.previous_state]} → {resourceNames[row.state]}</Text>
      <Text className='cp-paragraph'>{resourceReasons[row.state_reason] ?? '依据当时完整指标评估'} · {row.attention_required ? '当时需要负责人处理' : '当时无资源待办'}</Text>
      <Fact label='评估时间'><Text>{statusTime(row.evaluated_at)}</Text></Fact><Fact label='当时负责人'><Text>{row.recipient_id}</Text></Fact><Fact label='配置修订'><Text>{row.source_revision_id}</Text></Fact>
      <Text className='cp-paragraph'>{row.notification_event_created ? '已创建站内通知事件；实际投递结果请在通知中心核对。' : '本次仅保留状态记录，不发送站内提醒。'}</Text>
      {row.observation_id && <Action secondary onClick={() => setProof(row.observation_id!)}>核对该记录的原始签名</Action>}
    </View>)}
    <Notice error={evidence.error} />{proof && evidence.loading ? <Busy /> : proof && evidence.value && !evidence.error && <Text className='cp-notice cp-paragraph'>原始载荷摘要及 Ed25519 签名已校验：{evidence.value.id} · {evidence.value.payload_sha256}</Text>}
    {feed.next && !feed.error && <Action secondary loading={feed.moreBusy} onClick={() => void feed.more()}>加载更早状态记录</Action>}<Action secondary onClick={onClose}>返回主机资源</Action>
  </View></Section>;
}
