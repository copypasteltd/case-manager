import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { workbenchHints, workbenchLabels, workbenchState, workbenchURL, type WorkbenchSummary } from '@copypaste/contracts/workbench';
import { Empty, Notice, Row, Section, date } from '../../ui';

export function BusinessWorkbench({ value, error }: { value?: WorkbenchSummary; error?: unknown }) {
  return <><Section title="业务待办"><Text className="cp-muted cp-small">仅统计当前有权查阅、且由你负责的事项。打开详情后核对最新资料再处理。</Text><Notice error={error} />{value && <Text className="cp-muted cp-small">截至 {date(value.as_of)} · 合同按香港日期，至 {value.contract_through}</Text>}</Section>{value?.groups.map(group => <Section key={group.kind} title={workbenchLabels[group.kind] + ' · ' + group.count}><Text className="cp-muted cp-small">{workbenchHints[group.kind]}</Text>{group.items.length ? group.items.map(item => <Row key={item.id} title={item.title} subtitle={workbenchState(item) + (item.due_on ? ' · ' + (group.kind === 'contracts' ? '到期 ' : '账期开始 ') + item.due_on : '')} onClick={() => void Taro.navigateTo({ url: workbenchURL(group.kind, item, 'mobile') })} />) : <Empty>当前没有符合条件的事项；只反映你当前可见的负责事项。</Empty>}{group.has_more && <View><Text className="cp-muted cp-small">仅列出前5条，请到对应业务列表继续查阅。</Text></View>}</Section>)}</>;
}
