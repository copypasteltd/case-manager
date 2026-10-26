import { Text, View } from '@tarojs/components';
import Taro, { useDidHide, useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { identifier, usePage } from '../../platform/page';
import { useCollection } from '../../platform/collection';
import { MFAConfirmation, CommandNotice, useCommand } from '../../platform/command';
import { Input } from '../../platform/components';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Field, Notice, Page, Row, Section, date } from '../../ui';

import TelegramAccess from './access';

type Delivery = Schemas['TelegramDelivery'];
const names: Record<Delivery['state'], string> = { pending: '等待发送', sending: '正在发送', retry_wait: '等待重试', sent: 'Bot已确认送达', delivery_unknown: '投递结果未知', confirmed_sent: '本人对账为已发送', failed: '发送失败', superseded: '已合并到后续阶段', suppressed: '已停止发送', abandoned: '本人已放弃重发' };
const phases = { accepted: '受理通知', progress: '进度通知', result: '结果通知' };
export default function TelegramPage() {
 const route = useRouter();
 return route.params.section === 'access' ? <TelegramAccess /> : <TelegramRequests />;
}
function TelegramRequests() {
 const route = useRouter(), requestID = route.params.request;
 const page = usePage(async scope => {
  if (!requestID) return { request: undefined, details: undefined };
  const id = identifier(requestID);
  const request = await client.request<Schemas['TelegramRequest']>('/me/telegram/requests/' + id, { scope });
  const details = await client.request<Schemas['TelegramDeliveryDetails']>('/me/telegram/requests/' + id + '/deliveries', { scope });
  return { request, details };
 });
 const [target, setTarget] = useState<Delivery>(), value = page.result?.value;
 useDidHide(() => setTarget(undefined));
 return <Page title='我的TG请求' eyebrow='TELEGRAM'><Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index?section=access' })}>本人绑定与TG入口管理</Action><Notice error={page.error} />{page.loading ? <Busy /> : value && (value.request ? <><Section title='本人请求'><View className='cp-pad'><Text className='cp-paragraph' selectable>{value.request.text}</Text><Fact label='接收时间'><Text>{date(value.request.received_at)}</Text></Fact><Fact label='秘书状态'><Text>{value.request.state === 'withdrawn' ? '已撤回' : value.request.run_state ?? '等待处理'}</Text></Fact>{value.request.conversation_id && <Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/secretary/index?kind=conversation&id=' + value.request!.conversation_id + (value.request!.run_id ? '&run=' + value.request!.run_id : '') })}>查看本人秘书会话与结果</Action>}</View></Section><Section title='群通知投递记录'><View className='cp-pad'><Text className='cp-notice cp-paragraph'>群内只发送阶段和登录链接。结果未知时不会自动重发，人工对账与Bot回执分别保留。</Text></View>{value.details?.deliveries.length ? value.details.deliveries.map(d => <View className='cp-pad' key={d.id}><Text className='cp-row-title'>{phases[d.phase]} · {names[d.state]}</Text><Text className='cp-paragraph'>已尝试 {d.attempts} 次 · {date(d.updated_at)}</Text>{d.message_id !== null && <Text className='cp-paragraph'>消息编号：{d.message_id}</Text>}{value.details!.events.filter(e => e.delivery_id === d.id).map(e => <Text className='cp-paragraph cp-small' key={e.id}>第 {e.version} 版 · {names[e.state]}{e.reason ? ' · 对账原因：' + e.reason : ''}</Text>)}{d.state === 'delivery_unknown' && <Action secondary onClick={() => setTarget(d)}>核对此通知是否已发送</Action>}</View>) : <Empty>尚无通知投递记录。请联系管理员核对群通知与发送进程。</Empty>}</Section>{target && <Reconcile key={target.id + ':' + target.version} requestID={value.request.id} value={target} onClose={() => setTarget(undefined)} onSaved={() => { setTarget(undefined); void page.reload(); }} />}<Action secondary onClick={() => { setTarget(undefined); void page.reload(); }}>刷新请求与投递记录</Action></> : <RequestList />)}<Action secondary onClick={() => void Taro.navigateBack().catch(() => Taro.reLaunch({ url: '/pages/settings/index' }))}>返回我的工作空间</Action></Page>;
}
function RequestList() {
 const list = useCollection<Schemas['TelegramRequest']>('/me/telegram/requests?limit=20');
 return <Section title='本人从TG发出的请求'><Notice error={list.error} />{list.loading ? <Busy /> : list.items.length ? list.items.map(r => <Row key={r.id} title={r.text} subtitle={date(r.received_at) + ' · ' + (r.run_state ?? r.state)} onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index?request=' + r.id })} />) : !list.error && <Empty>尚无已接收的本人请求。</Empty>}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早TG请求</Action>}</Section>;
}
function Reconcile({ requestID, value, onClose, onSaved }: { requestID: string; value: Delivery; onClose(): void; onSaved(): void }) {
 const [action, setAction] = useState<Schemas['TelegramDeliveryReconcile']['action']>('abandon'), [message, setMessage] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [confirmed, setConfirmed] = useState(false);
 const command = useCommand<Delivery>(onSaved, 'telegram-delivery:' + value.id + ':' + value.version);
 return <Section title='人工核对群通知'><View className='cp-pad'><Notice error={command.error} /><CommandNotice blocked={command.blocked} /><Text className='cp-notice cp-paragraph'>请先查看原群与话题。人工确认不等同于Bot回执；核对未发送后重试仍可能重复。</Text>{([['abandon', '保留未知事实，放弃重发'], ['confirm_sent', '本人已找到群内对应消息'], ['retry_confirmed_not_sent', '本人核对未发送，允许重试']] as const).map(([a, label]) => <Action secondary key={a} disabled={command.busy || command.blocked} onClick={() => { setAction(a); setConfirmed(false); }}>{a === action ? '已选择：' : ''}{label}</Action>)}{action === 'confirm_sent' && <Field label='对应群消息编号'><Input ariaLabel='对应群消息编号' type='number' value={message} onInput={event => setMessage(event.detail.value)} /></Field>}{action === 'retry_confirmed_not_sent' && <Action secondary onClick={() => setConfirmed(c => !c)}>{confirmed ? '已确认' : '确认'}：已核对原群，允许再次发送并接受可能重复</Action>}<Field label='对账原因'><Input ariaLabel='对账原因' value={reason} maxlength={200} onInput={event => setReason(event.detail.value)} /></Field><MFAConfirmation onFresh={setFresh} disabled={command.busy || command.blocked} /><Action loading={command.busy} disabled={command.blocked || !fresh || !reason.trim() || (action === 'retry_confirmed_not_sent' && !confirmed) || (action === 'confirm_sent' && (!/^[1-9][0-9]*$/.test(message) || !Number.isSafeInteger(Number(message))))} onClick={() => void command.submit('/me/telegram/requests/' + requestID + '/deliveries/' + value.id + '/reconcile', { expected_version: value.version, action, message_id: action === 'confirm_sent' ? Number(message) : null, reason })}>保存此次对账</Action><Action secondary disabled={command.busy} onClick={onClose}>关闭对账</Action></View></Section>;
}
