import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { useCollection } from '../../platform/collection';
import { FixedFiles } from '../../platform/fixed-files';
import { financialRoutes, type FinancialKind } from '../../platform/financial-input';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Notice, Section, date } from '../../ui';

type Revision = Schemas['CostRevision'] | Schemas['PaymentRevision'];
type Event = Schemas['CostEvent'] | Schemas['PaymentEvent'];
export function FinancialHistory({ kind, id }: { kind: FinancialKind; id: string }) {
  const revisions = useCollection<Revision>(financialRoutes[kind] + '/' + id + '/revisions?limit=20');
  const events = useCollection<Event>(financialRoutes[kind] + '/' + id + '/events?limit=20');
  const [selected, setSelected] = useState<string>();
  return <>
    <Section title='财务修订历史'><Notice error={revisions.error} />{revisions.loading ? <Busy /> : revisions.items.length ? revisions.items.map(item => <View className='cp-pad cp-fixed-file' key={item.id}><Text className='cp-row-title'>修订 {item.seq} · {item.title}</Text><Text>{date(item.created_at)} · {item.currency} {item.amount}</Text><Text className='cp-paragraph'>{item.reason}</Text><Text className='cp-code'>{item.content_hash}</Text><Action secondary onClick={() => setSelected(selected === item.id ? undefined : item.id)}>{selected === item.id ? '收起准确修订' : '核对准确修订'}</Action>{selected === item.id && <><Fact label='固定业务日期'><Text>{'occurred_on' in item ? item.occurred_on ?? '未填写' : item.paid_at ?? '未填写'}</Text></Fact><Fact label='固定往来方'><Text>{item.counterparty_evidence?.name ?? '当前不可读或尚未填写'}</Text></Fact><Fact label='固定说明'><Text className='cp-paragraph'>{item.notes || '未填写'}</Text></Fact>{'allocations' in item && item.allocations.map((line, index) => <Fact key={index} label={line.project_id ? '项目 ' + line.project_id : '公司费用份额'}><Text>{item.currency} {line.amount}</Text></Fact>)}<FixedFiles files={item.attachments} /></>}</View>) : !revisions.error && <Empty>暂无当前可读修订。</Empty>}{revisions.next && <Action secondary loading={revisions.moreBusy} onClick={() => void revisions.more()}>加载更早财务修订</Action>}</Section>
    <Section title='财务操作历史'><Notice error={events.error} />{events.loading ? <Busy /> : events.items.length ? events.items.map(item => <Fact key={item.id} label={'版本 ' + item.version + ' · ' + item.kind}><Text>{item.state} · {date(item.created_at)}</Text><Text className='cp-paragraph'>{item.reason}</Text><Text className='cp-code'>{item.revision_id}</Text></Fact>) : !events.error && <Empty>暂无当前可读操作记录。</Empty>}{events.next && <Action secondary loading={events.moreBusy} onClick={() => void events.more()}>加载更早财务操作</Action>}</Section>
  </>;
}
