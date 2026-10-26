import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { useCollection } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import { channelInput, channelPayload, channelReceipt, telegramSkills, type Channel, type ChannelInput } from '../../platform/telegram-input';
import type { Schemas } from '../../platform/session';
import { SelectField, TextField } from '../business/controls';
import { Action, Busy, Check, Empty, Fact, Notice, Section, date } from '../../ui';

type Page<T> = { items: T[]; next_cursor: string | null };
export function TelegramChannels({ bots, authorization }: { bots: Schemas['TelegramBot'][]; authorization: number }) {
  const list = useCollection<Channel>('/telegram/channels?limit=20');
  const [editing, setEditing] = useState<Channel | 'new'>(), [history, setHistory] = useState<Channel>();
  function close() { setEditing(undefined); setHistory(undefined); list.reload(); }
  return <Section title='群与话题入口管理'><View className='cp-pad'>
    <Text className='cp-notice cp-paragraph'>仅管理准确Bot、群号与话题的接收范围。新入口先暂停；业务正文只在有权门户显示，群通知只含阶段与登录链接。</Text>
    <Notice error={list.error} />{editing ? <ChannelEditor key={(editing === 'new' ? 'new' : editing.id + ':' + editing.version) + ':' + authorization} current={editing === 'new' ? undefined : editing} bots={bots} onClose={close} />
      : history ? <ChannelHistory current={history} onClose={() => setHistory(undefined)} /> : <>
        <Action disabled={!bots.length || list.loading || !!list.error} onClick={() => setEditing('new')}>新增暂停的TG群入口</Action>
        {list.loading ? <Busy /> : list.items.map(value => <View className='cp-editor-row' key={value.id}><ChannelFacts value={value} />
          <Action secondary onClick={() => setEditing(value)}>编辑入口：{value.label}</Action><Action secondary onClick={() => setHistory(value)}>查看历史：{value.label}</Action></View>)}
        {!list.loading && !list.error && !list.items.length && <Empty>尚无群与话题入口。</Empty>}
        {list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多TG群入口</Action>}
        <Action secondary onClick={list.reload}>重新读取TG群入口列表</Action>
      </>}
  </View></Section>;
}
function ChannelFacts({ value }: { value: Channel }) {
  return <><Text className='cp-row-title'>{value.label}</Text><Fact label='接收与修订'><Text>{value.state === 'active' ? '接收已启用' : '接收已暂停'} · 第{value.version}版</Text></Fact>
    <Fact label='准确Bot／群号／话题'><Text selectable>{value.bot_id} ／ {value.chat_id} ／ {value.topic_id}</Text></Fact>
    <Fact label='项目范围'><Text selectable>{value.project_id ?? '公司范围'}</Text></Fact>
    <Fact label='允许任务'><Text>{value.allowed_skill_ids.map(id => telegramSkills[id]).join('、')}</Text></Fact>
    <Fact label='默认任务'><Text>{value.default_skill_id ? telegramSkills[value.default_skill_id] : '未设置，多任务须明确命令'}</Text></Fact>
    <Fact label='阶段与登录链接通知'><Text>{value.delivery_enabled ? '允许，实际送达另查回执' : '不允许'}</Text></Fact></>;
}
function ChannelEditor({ current, bots, onClose }: { current?: Channel; bots: Schemas['TelegramBot'][]; onClose(): void }) {
  const [input, setInput] = useState(() => channelInput(current)), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [checked, setChecked] = useState(false), [error, setError] = useState<unknown>();
  const [picking, setPicking] = useState(false), [projectName, setProjectName] = useState(current?.project_id ? '已配置项目，请重新核对当前可见范围' : '公司范围');
  const command = useCommand<Channel>(onClose, 'telegram-channel:' + (current ? current.id + ':' + current.version : 'new'), async value => {
    const body = channelPayload(input, current, reason, fresh, checked);
    if (!channelReceipt(value, body, current)) return false;
    const history = await client.request<Page<Schemas['TelegramChannelRevision']>>('/telegram/channels/' + value.id + '/history?limit=1');
    const latest = history.items[0];
    return latest?.channel_id === value.id && latest.version === value.version && latest.source === 'portal'
      && latest.reason === body.reason && channelReceipt(latest.snapshot, body, current);
  });
  const disabled = command.busy || command.blocked;
  function change<K extends keyof ChannelInput>(key: K, value: ChannelInput[K]) { setInput(old => ({ ...old, [key]: value })); setChecked(false); }
  async function save() { try { const body = channelPayload(input, current, reason, fresh, checked); setError(undefined); await command.submit(current ? '/telegram/channels/' + current.id : '/telegram/channels', body, current ? 'PATCH' : 'POST'); } catch (e) { setError(e); } }
  if (picking) return <ProjectChoice onChoose={value => { change('project', value?.id ?? ''); setProjectName(value?.name ?? '公司范围'); setPicking(false); }} onClose={() => setPicking(false)} />;
  return <><Text className='cp-row-title'>{current ? '编辑准确TG群入口' : '新建暂停的TG群入口'}</Text><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    {current ? <><Fact label='不可更换的准确位置'><Text selectable>{current.bot_id} ／ {current.chat_id} ／ {current.topic_id}</Text></Fact><Fact label='采用入口版本'><Text>{current.version}</Text></Fact></>
      : <><SelectField label='接收Bot' value={input.bot} options={[['', '请明确选择Bot'], ...bots.map(b => [b.bot_id, '@' + b.username] as const)]} disabled={disabled} onChange={v => change('bot', v)} />
        <TextField name='tg-chat' label='准确数字群号' value={input.chat} max={17} disabled={disabled} onChange={v => change('chat', v)} hint='保留负号；不使用群名或用户名。' />
        <TextField name='tg-topic' label='准确数字话题号' value={input.topic} max={10} disabled={disabled} onChange={v => change('topic', v)} hint='无话题请明确填0。' /></>}
    <TextField name='tg-label' label='TG入口名称' value={input.label} disabled={disabled} onChange={v => change('label', v)} />
    <Fact label='本次项目范围'><Text>{projectName}</Text>{input.project && <Text className='cp-code' selectable>{input.project}</Text>}</Fact>
    <Action secondary disabled={disabled} onClick={() => setPicking(true)}>选择TG项目范围</Action>
    {(Object.keys(telegramSkills) as (keyof typeof telegramSkills)[]).map(id => <Check key={id} checked={input.allowed.includes(id)} disabled={disabled} onChange={yes => {
      setInput(old => ({ ...old, allowed: yes ? [...old.allowed, id] : old.allowed.filter(v => v !== id), defaultSkill: !yes && old.defaultSkill === id ? '' : old.defaultSkill })); setChecked(false);
    }}>{'允许任务：' + telegramSkills[id]}</Check>)}
    <SelectField label='TG默认任务' value={input.defaultSkill} options={[['', '未设置；多任务须明确命令'], ...input.allowed.map(id => [id, telegramSkills[id]] as const)]} disabled={disabled} onChange={v => change('defaultSkill', v)} />
    {current && <SelectField label='TG接收状态' value={input.state} options={[['paused', '暂停接收'], ['active', '启用接收']]} disabled={disabled} onChange={v => change('state', v as Channel['state'])} />}
    <Check checked={input.delivery} disabled={disabled} onChange={v => change('delivery', v)}>允许向准确群与话题发送阶段和登录链接</Check>
    <TextField name='tg-config-reason' label='TG配置原因' value={reason} disabled={disabled} onChange={setReason} />
    <Text className='cp-notice cp-paragraph'>更换群或话题须新建入口；保存与启用须本人近期两步验证。服务器仍核对当前权限，接收和通知配置不授予业务读写或群正文披露。</Text>
    <Check checked={checked} disabled={disabled} onChange={setChecked}>我已核对准确Bot、群、话题、项目范围和全部允许任务</Check>
    <MFAConfirmation onFresh={setFresh} disabled={disabled} />
    <Action disabled={disabled || !fresh || !checked || !reason.trim()} loading={command.busy} onClick={() => void save()}>{current ? '保存准确TG入口配置' : '创建暂停的TG群入口'}</Action>
    <Action secondary disabled={command.busy} onClick={onClose}>{command.blocked ? '关闭表单并读取最新TG入口' : '关闭TG入口表单'}</Action>
  </>;
}
function ProjectChoice({ onChoose, onClose }: { onChoose(value?: Schemas['Project']): void; onClose(): void }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState('');
  const list = useCollection<Schemas['Project']>('/projects?limit=20' + (query ? '&q=' + encodeURIComponent(query) : ''));
  return <><Text className='cp-row-title'>选择TG项目范围</Text><Notice error={list.error} /><TextField name='tg-project-q' label='查找TG可选项目' value={search} disabled={list.loading} onChange={setSearch} />
    <Action secondary disabled={list.loading} onClick={() => { setQuery(search.trim()); list.reload(); }}>查找TG可选项目</Action>
    <Action secondary onClick={() => onChoose()}>明确使用公司范围</Action>
    {list.loading ? <Busy /> : list.items.map(project => <Action secondary key={project.id} onClick={() => onChoose(project)}>选择项目：{project.name}</Action>)}
    {list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更多TG可选项目</Action>}
    {!list.loading && !list.error && !list.items.length && <Empty>没有当前可选项目。</Empty>}
    <Action secondary onClick={onClose}>返回TG入口表单</Action></>;
}
function ChannelHistory({ current, onClose }: { current: Channel; onClose(): void }) {
  const list = useCollection<Schemas['TelegramChannelRevision']>('/telegram/channels/' + current.id + '/history?limit=20');
  return <><Text className='cp-row-title'>准确TG入口修订历史</Text><Notice error={list.error} />{list.loading ? <Busy /> : list.items.map(revision => <View className='cp-editor-row' key={revision.id}>
    <Fact label='历史记录'><Text>第{revision.version}版 · {date(revision.created_at)}</Text></Fact><Fact label='变更来源'><Text>{({ portal: '门户配置', migration: '群标识变化', membership: '群成员变化' })[revision.source]}</Text></Fact>
    <Fact label='当时原因'><Text selectable>{revision.reason}</Text></Fact><ChannelFacts value={revision.snapshot} />
  </View>)}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早TG入口历史</Action>}<Action secondary onClick={onClose}>返回TG入口列表</Action></>;
}
