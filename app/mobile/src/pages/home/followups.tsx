import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { followupHints, followupLabels, followupState, followupURL, type FollowupSummary } from '@copypaste/contracts/followups';
import { Empty, Notice, Row, Section, date } from '../../ui';

export function Followups({ value, error }: { value?: FollowupSummary; error?: unknown }) {
  return <><Section title='申请与交付跟进'><Text className='cp-muted cp-small'>按当前权限核对申请和交付；修订、提交与独立验收仍在详情办理。</Text><Notice error={error}/>{value && <Text className='cp-muted cp-small'>截至 {date(value.as_of)}</Text>}</Section>{value?.groups.map(group => <Section key={group.kind} title={followupLabels[group.kind] + ' · ' + group.count}><Text className='cp-muted cp-small'>{followupHints[group.kind]}</Text>{group.items.length ? group.items.map(item => <Row key={item.id} title={item.title} subtitle={followupState(item)} onClick={() => void Taro.navigateTo({ url: followupURL(group.kind, item, 'mobile') })}/>) : <Empty>当前没有符合条件的事项；处理后返回首页核对。</Empty>}{group.has_more && <View><Text className='cp-muted cp-small'>仅列出最近更新的5条，共{group.count}条，请到对应业务或项目列表查阅。</Text></View>}</Section>)}</>;
}
