import { useState } from 'react';
import type { InventoryKind } from '@copypaste/contracts/work-inventory';
import { CompleteInventory } from './inventory';
import { Text } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { followupHints, followupLabels, followupState, followupURL, type FollowupSummary } from '@copypaste/contracts/followups';
import { Action, Empty, Notice, Row, Section, date } from '../../ui';

export function Followups({ value, error }: { value?: FollowupSummary; error?: unknown }) {
  const [inventory,setInventory]=useState<InventoryKind>();
  if(inventory) return <CompleteInventory key={inventory} kind={inventory} onClose={()=>setInventory(undefined)}/>;
  return <><Section title='申请与交付跟进'><Text className='cp-muted cp-small'>按当前权限核对申请和交付；修订、提交与独立验收仍在详情办理。</Text><Notice error={error}/>{value && <Text className='cp-muted cp-small'>截至 {date(value.as_of)}</Text>}</Section>{value?.groups.map(group => <Section key={group.kind} title={followupLabels[group.kind] + ' · ' + group.count}><Text className='cp-muted cp-small'>{followupHints[group.kind]}</Text>{group.items.length ? group.items.map(item => <Row key={item.id} title={item.title} subtitle={followupState(item)} onClick={() => void Taro.navigateTo({ url: followupURL(group.kind, item, 'mobile') })}/>) : <Empty>当前没有符合条件的事项；处理后返回首页核对。</Empty>}<Action secondary onClick={()=>setInventory(group.kind)}>查阅此分组完整列表</Action></Section>)}</>;
}
