import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { useRef, useState } from 'react';
import { mergeNotifications, notificationCategories, notificationCount, notificationDestination, notificationReadInput, type Notification } from '@copypaste/contracts/notifications';
import { client } from '../../platform/taro';
import { usePage } from '../../platform/page';
import { useCommand } from '../../platform/command';
import { MobileError, type Page as ResultPage, type Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Notice, Page, Section, State, date } from '../../ui';
import { SelectField } from '../business/controls';
import { NotificationDelivery } from './delivery';
import { ContractReminders } from './reminders';
import { NotificationTelegram } from './telegram';

export default function NotificationsPage() {
  const filters = useRef({ state: 'all', category: '' });
  const [state, setState] = useState('all'), [category, setCategory] = useState('');
  const [extra, setExtra] = useState<Notification[]>([]), [cursor, setCursor] = useState<string | null>(), [moreBusy, setMoreBusy] = useState(false), [error, setError] = useState<unknown>(), [opening, setOpening] = useState(false);
  const flight = useRef(false);
  const path = (next?: string) => '/notifications?state=' + filters.current.state + '&category=' + filters.current.category + '&limit=20' + (next ? '&cursor=' + encodeURIComponent(next) : '');
  const page = usePage(async scope => {
    setExtra([]); setCursor(undefined); setError(undefined); setMoreBusy(false); setOpening(false); flight.current = false;
    const [list, summary] = await Promise.all([client.request<ResultPage<Notification>>(path(), { scope }), client.request<Schemas['NotificationSummary']>('/notifications/summary', { scope })]);
    return { list, summary };
  });
  useDidHide(() => { setExtra([]); setCursor(undefined); setError(undefined); setOpening(false); flight.current = false; });
  const value = page.result?.value, next = cursor === undefined ? value?.list.next_cursor : cursor;
  const items = value && !error ? mergeNotifications([...value.list.items, ...extra]) : [];
  async function more() {
    const scope = page.scope; if (!next || !scope || scope.closed || flight.current) return;
    flight.current = true; setMoreBusy(true); setError(undefined);
    try { const value = await client.request<ResultPage<Notification>>(path(next), { scope }); if (!scope.closed) { setExtra(old => [...old, ...value.items]); setCursor(value.next_cursor); } }
    catch (cause) { if (!scope.closed) { setError(cause); setExtra([]); } }
    finally { if (!scope.closed) { flight.current = false; setMoreBusy(false); } }
  }
  async function open(item: Notification) {
    const scope = page.scope; if (!scope || scope.closed || flight.current) return;
    flight.current = true; setOpening(true); setError(undefined);
    try {
      const current = await client.request<Notification>('/notifications/' + item.id, { scope });
      if (scope.closed) return;
      if (current.id !== item.id) throw new MobileError('INVALID_RESPONSE', '通知与所选记录不一致。');
      await Taro.navigateTo({ url: notificationDestination(current.target).mobile });
    } catch (cause) { if (!scope.closed) { setError(cause); setExtra([]); } }
    finally { if (!scope.closed) { flight.current = false; setOpening(false); } }
  }
  return <Page title='通知中心' eyebrow='WORKSPACE / NOTIFICATIONS' home>
    <Text className='cp-muted cp-paragraph'>查看与你相关的工作进展。标记已读后，相关业务仍须在资料中处理。</Text>
    <SelectField label='处理范围' value={state} options={[[ 'all', '全部通知' ], [ 'unread', '仅未读' ], [ 'attention', '仍需处理' ]]} disabled={page.loading || opening} onChange={value => { setState(value); filters.current.state = value; void page.reload(); }} />
    <SelectField label='通知类型' value={category} options={[[ '', '全部类型' ], ...Object.entries(notificationCategories)]} disabled={page.loading || opening} onChange={value => { setCategory(value); filters.current.category = value; void page.reload(); }} />
    <Notice error={page.error ?? error} />{page.loading ? <Busy /> : value && <>
      <Section title='通知概览'><Fact label='未读'><Text>{notificationCount(value.summary.unread_count, value.summary.count_complete)}</Text></Fact><Fact label='仍需处理'><Text>{notificationCount(value.summary.attention_count, value.summary.count_complete)}</Text></Fact><View className='cp-pad'><Text className='cp-muted cp-small'>截至 {date(value.summary.as_of)}{!value.summary.count_complete && ' · + 表示至少这些条，计数待进一步核对。'}</Text></View></Section>
      <Section title='通知列表'>{items.length ? items.map(item => <NotificationCard key={item.id + ':' + item.delivery_seq + ':' + item.read_seq} item={item} me={page.result!.me} disabled={opening} onOpen={() => void open(item)} onRead={() => void page.reload()} />) : !error && <Empty>当前没有符合条件的通知。</Empty>}</Section>
      {next && !error && <Action secondary loading={moreBusy} onClick={() => void more()}>加载更早通知</Action>}
      {page.result!.me.actions.includes('notification.delivery.read') && <NotificationDelivery me={page.result!.me} />}
      <NotificationTelegram me={page.result!.me} />
      {page.result!.me.actions.includes('notification.policy.read') && <ContractReminders me={page.result!.me} />}
    </>}
    <Action secondary loading={page.loading} disabled={opening} onClick={() => void page.reload()}>刷新通知</Action>
  </Page>;
}
function NotificationCard({ item, me, disabled, onOpen, onRead }: { item: Notification; me: Schemas['Me']; disabled: boolean; onOpen(): void; onRead(): void }) {
  const command = useCommand<Notification>(onRead, [me.user.id, me.authorization_version, 'notification-read', item.id, item.delivery_seq].join(':'), value => value.id === item.id && value.read_seq >= item.delivery_seq && value.read_seq <= value.delivery_seq);
  return <View className='cp-pad cp-paragraph'><Text className='cp-section-title'>{item.title}</Text><Text className='cp-muted cp-small'>{item.read ? '已读' : '未读'} · {item.attention_required ? '仍需处理' : '查看进展'}</Text><Text className='cp-paragraph'>{item.resource_label}</Text><State value={item.current_state} /><Text className='cp-muted cp-small'>{date(item.updated_at)}</Text><Notice error={command.error} /><Action disabled={disabled || command.busy} onClick={onOpen}>打开相关资料</Action>{!item.read && <Action secondary disabled={disabled || command.blocked} loading={command.busy} onClick={() => void command.submit('/notifications/' + item.id + '/read', notificationReadInput(item))}>标记已读</Action>}</View>;
}
