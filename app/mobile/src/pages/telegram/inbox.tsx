import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { telegramInboxStates as states, telegramInboxSkills as skills, telegramInboxSources as sources, telegramInboxReason as reason, telegramInboxPath, telegramInboxWithdrawInput, type TelegramInboxOperation as Item, type TelegramInboxDetails as Details, type TelegramInboxEvent as Event } from '@copypaste/contracts/telegram-inbox';
import { usePage } from '../../platform/page';
import { useCollection, useRecord } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import { TextField, SelectField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Section, date } from '../../ui';

export default function TelegramInbox() {
  const page = usePage(async (_scope, me) => me.actions.includes('telegram.inbox.read'));
  return <Page title='组织入站调度' eyebrow='TELEGRAM / INBOX'><Notice error={page.error} />{page.loading ? <Busy /> : page.result?.value ? <Workspace canWrite={page.result.me.actions.includes('telegram.inbox.withdraw')} /> : !page.error && <Empty>当前账号没有组织TG入站查阅权限。</Empty>}<Action secondary onClick={() => void Taro.redirectTo({ url: '/pages/telegram/index' })}>返回本人TG入口</Action></Page>;
}
function Workspace({ canWrite }: { canWrite: boolean }) {
  const [state, setState] = useState(''), [channel, setChannel] = useState(''), [skill, setSkill] = useState(''), [path, setPath] = useState(telegramInboxPath());
  return <><Section title='入站查询条件'><View className='cp-pad'><Text className='cp-notice cp-paragraph'>台账展示路由与状态；正文和秘书结果由本人查阅。已安装配置不代表模型或调度器正在运行。</Text><SelectField label='入站状态' value={state} options={[["", "全部状态"], ...Object.entries(states)]} onChange={setState} disabled={false} /><SelectField label='任务类型' value={skill} options={[["", "全部任务"], ...Object.entries(skills)]} onChange={setSkill} disabled={false} /><TextField name='inbox-channel' label='群入口编号（可选）' value={channel} onChange={setChannel} max={36} disabled={false} /><Action onClick={() => setPath(telegramInboxPath(state, channel, skill))}>应用入站查询条件</Action></View></Section><InboxList key={path} path={path} canWrite={canWrite} /></>;
}
function InboxList({ path, canWrite }: { path: string; canWrite: boolean }) {
  const list = useCollection<Item>(path), [selected, setSelected] = useState<string>(), [revision, setRevision] = useState(0);
  return <><Section title='入站请求台账'><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && (list.items.length ? list.items.map(v => <View className='cp-pad' key={v.id}><Text className='cp-row-title'>{v.channel_label} · {states[v.state]}</Text><Text className='cp-paragraph'>{v.requested_skill_id ? skills[v.requested_skill_id] : '原请求未保留任务选择'} · {date(v.received_at)}</Text><Text className='cp-paragraph'>{reason(v.reason_code)}</Text><Text className='cp-small cp-paragraph'>原群 {v.chat_id} ／话题 {v.topic_id}</Text><Action secondary onClick={() => setSelected(v.id)}>查看入站详情</Action></View>) : <Empty>当前条件没有入站请求。</Empty>)}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早入站请求</Action>}<Action secondary onClick={() => { setSelected(undefined); list.reload(); }}>刷新入站台账</Action></Section>{selected && <InboxDetail key={selected + ':' + revision} id={selected} canWrite={canWrite} onClose={() => setSelected(undefined)} onSaved={() => { list.reload(); setRevision(v => v + 1); }} />}</>;
}
function InboxDetail({ id, canWrite, onClose, onSaved }: { id: string; canWrite: boolean; onClose(): void; onSaved(): void }) {
  const query = useRecord<Details>('/telegram/inbox-operations/' + id), [editing, setEditing] = useState(false), v = query.value?.item;
  return <Section title='入站详情与调度历史'><Notice error={query.error} />{query.loading ? <Busy /> : !query.error && v && <><Fact label={states[v.state] + ' · ' + reason(v.reason_code)}><Text className='cp-code cp-small'>{v.id} · 第 {v.version} 版</Text><Text>原群 {v.chat_id} ／话题 {v.topic_id} · Bot {v.bot_id}</Text><Text>请求人编号：{v.requester_id}</Text><Text>接收于 {date(v.received_at)}</Text><Text>入口版本：请求 {v.request_channel_version} ／当前 {v.current_channel_version}</Text><Text>{v.scope_current ? '当前身份与入口匹配' : '当前身份或入口已变化'}</Text><Text>下次可调度：{date(v.dispatch_after)}</Text><Text>排队有效期至：{date(v.dispatch_expires_at)}</Text><Text>指定任务配置：{v.profile_configured ? '已安装' : '未安装'}</Text>{v.run_id && <Text>秘书任务：{v.run_state} · {v.run_id}</Text>}</Fact><History id={id} />{v.withdrawable && canWrite && !editing && <Action onClick={() => setEditing(true)}>撤回此排队请求</Action>}{editing && <Withdraw key={v.id + ':' + v.version} value={v} onClose={() => setEditing(false)} onSaved={onSaved} />}</>}<Action secondary onClick={onClose}>关闭入站详情</Action></Section>;
}
function History({ id }: { id: string }) {
  const list = useCollection<Event>('/telegram/inbox-operations/' + id + '/history?limit=20');
  return <><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && list.items.map(e => <Fact key={e.id} label={'第 ' + e.version + ' 版 · ' + states[e.state]}><Text>{date(e.created_at)} · {sources[e.source]}</Text><Text>{reason(e.reason_code)}</Text>{e.actor_id && <Text className='cp-small'>人工经办：{e.actor_id}</Text>}</Fact>)}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早调度历史</Action>}</>;
}
function Withdraw({ value, onClose, onSaved }: { value: Item; onClose(): void; onSaved(): void }) {
  const [note, setNote] = useState(''), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand(onSaved, 'telegram-inbox:' + value.id + ':' + value.version);
  return <View className='cp-pad'><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} /><Text className='cp-notice cp-paragraph'>请求 {value.id} · 第 {value.version} 版。撤回后不再创建秘书任务；已发出的群通知保留实际记录。已创建的任务需从原任务流程处理。</Text><Action secondary disabled={command.busy || command.blocked} onClick={() => setReviewed(v => !v)}>{reviewed ? '已确认' : '确认'}：已核对准确请求并确认停止后续调度</Action><TextField name='inbox-withdraw-reason' label='撤回原因' value={note} onChange={setNote} disabled={command.busy || command.blocked} max={200} /><MFAConfirmation onFresh={setFresh} disabled={command.busy || command.blocked} /><Action loading={command.busy} disabled={!fresh || !reviewed || !note.trim() || command.blocked} onClick={() => { setError(undefined); try { void command.submit('/telegram/inbox-operations/' + value.id + '/withdraw', telegramInboxWithdrawInput(value, note, reviewed)); } catch (cause) { setError(cause); } }}>确认撤回排队请求</Action><Action secondary disabled={command.busy} onClick={onClose}>关闭撤回</Action></View>;
}
