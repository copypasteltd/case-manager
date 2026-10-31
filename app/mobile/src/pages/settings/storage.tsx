import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { providers, states, jobStates, blocks, retryNotice, storagePath, storageHistoryPath, storageRetryInput, checkedStorageReceipt, type StorageItem, type StorageDetails, type StorageReceipt } from '@copypaste/contracts/storage-operations';
import { usePage } from '../../platform/page';
import { useCollection, useRecord } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import { TextField, SelectField } from '../business/controls';
import { Action, Busy, Empty, Fact, Notice, Page, Section, date } from '../../ui';

const options = (v: Record<string, string>) => [['', '全部'], ...Object.entries(v)] as [string, string][];
export default function StorageOperations() {
  const page = usePage(async (_scope, me) => me.actions.includes('storage.operations.read'));
  return <Page title='文件复制运维' eyebrow='FILES / REPLICATION'><Notice error={page.error} />{page.loading ? <Busy /> : page.result?.value ? <Workspace key={page.result.me.user.id + ':' + page.result.me.authorization_version} /> : <Empty>当前账号没有组织文件复制运维查阅权限。</Empty>}<Action secondary onClick={() => void Taro.navigateBack().catch(() => Taro.redirectTo({ url: '/pages/settings/index' }))}>返回我的工作空间</Action></Page>;
}
function Workspace() {
  const [provider, setProvider] = useState(''), [state, setState] = useState('failed'), [job, setJob] = useState(''), [path, setPath] = useState(storagePath('', 'failed'));
  return <><Section title='副本查询'><View className='cp-pad'><Text className='cp-muted cp-paragraph'>此处查阅组织运维元数据；查看文件和重试仍使用当前文件权限。</Text><SelectField label='存储端' value={provider} options={options(providers)} onChange={setProvider} disabled={false} /><SelectField label='副本状态' value={state} options={options(states)} onChange={setState} disabled={false} /><SelectField label='当前任务' value={job} options={options(jobStates)} onChange={setJob} disabled={false} /><Action onClick={() => setPath(storagePath(provider, state, job))}>应用复制查询条件</Action></View></Section><Inventory key={path} path={path} /></>;
}
function Inventory({ path }: { path: string }) {
  const list = useCollection<StorageItem>(path), [id, setID] = useState<string>(), [revision, setRevision] = useState(0), [receipt, setReceipt] = useState<StorageReceipt>();
  return <><Section title='副本与任务台账'><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && (list.items.length ? list.items.map(v => <View className='cp-pad' key={v.id}><Text className='cp-row-title'>{providers[v.provider]} · {states[v.state]}</Text><Text className='cp-paragraph'>{v.is_current ? '当前文件版本' : '历史文件版本'} · 副本修订 {v.version} · {date(v.updated_at)}</Text>{v.job && <Text className='cp-paragraph'>{jobStates[v.job.state]} · 尝试 {v.job.attempt}／{v.job.max_attempts} 次</Text>}<Text className='cp-code cp-small'>{v.id}</Text><Action secondary onClick={() => { setReceipt(undefined); setID(v.id); }}>核对副本与人工记录</Action></View>) : <Empty>当前条件没有副本记录。</Empty>)}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早副本</Action>}<Action secondary onClick={() => { setID(undefined); list.reload(); }}>刷新副本台账</Action></Section>{receipt && <Text className='cp-notice cp-paragraph'>人工重试已排队，固定回执 {receipt.id}；后续复制结果以当前副本状态为准。</Text>}{id && <Detail key={id + ':' + revision} id={id} onClose={() => setID(undefined)} onSaved={v => { setReceipt(v); list.reload(); setRevision(n => n + 1); }} />}</>;
}
function Detail({ id, onClose, onSaved }: { id: string; onClose(): void; onSaved(v: StorageReceipt): void }) {
  const detail = useRecord<StorageDetails>('/storage/replicas/' + id), [editing, setEditing] = useState(false), v = detail.value;
  return <Section title='副本详情与人工重试'><Notice error={detail.error} />{detail.loading ? <Busy /> : !detail.error && v && <><Fact label={providers[v.item.provider] + ' · ' + states[v.item.state]}><Text>{v.item.is_current ? '当前文件版本' : '历史文件版本'} · 副本修订 {v.item.version}</Text><Text className='cp-code cp-small'>文件 {v.item.file_id} ／版本 {v.item.file_version_id}</Text><Text>来源副本{v.item.source_available ? '具有核验记录' : '当前不可核验'} · 原件状态 {v.item.availability}</Text>{v.item.job && <Text>{jobStates[v.item.job.state]} · 尝试 {v.item.job.attempt}／{v.item.job.max_attempts} 次</Text>}{v.item.error_code && <Text>故障代码：{v.item.error_code}</Text>}</Fact><Text className='cp-muted cp-paragraph'>{blocks[v.retry_block]} {retryNotice}</Text>{v.can_open && <Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/files/detail?id=' + v.item.file_id })}>打开有权查看的文件</Action>}{v.can_retry && !editing && <Action onClick={() => setEditing(true)}>核对并安排复制重试</Action>}{editing && <Retry value={v} onClose={() => setEditing(false)} onSaved={onSaved} />}</>}<History id={id} /><Action secondary onClick={onClose}>关闭副本详情</Action></Section>;
}
function History({ id }: { id: string }) {
  const list = useCollection<StorageReceipt>(storageHistoryPath(id));
  return <View className='cp-pad'><Text className='cp-row-title'>固定人工重试记录</Text><Notice error={list.error} />{list.loading ? <Busy /> : !list.error && (list.items.length ? list.items.map(v => <Fact key={v.id} label={'副本修订 ' + v.expected_version + ' → ' + v.result_version}><Text>{date(v.created_at)} · {states[v.before.state]} → 当时排队</Text><Text className='cp-code cp-small'>回执 {v.id}</Text><Text className='cp-small'>经办人 {v.actor_id}；当前状态另行核对。</Text></Fact>) : <Text className='cp-muted'>尚无组织运维人工重试记录；普通文件重试和工作器历史继续保留原审计。</Text>)}{list.next && <Action secondary loading={list.moreBusy} onClick={() => void list.more()}>加载更早人工记录</Action>}</View>;
}
function Retry({ value, onClose, onSaved }: { value: StorageDetails; onClose(): void; onSaved(v: StorageReceipt): void }) {
  const [reason, setReason] = useState(''), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false), [error, setError] = useState<unknown>();
  const command = useCommand<StorageReceipt>(onSaved, 'storage-retry:' + value.item.id + ':' + value.item.version + ':' + value.item.basis_sha256, v => { checkedStorageReceipt(v, value); return true; });
  return <View className='cp-pad'><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} /><Text className='cp-notice cp-paragraph'>{retryNotice}</Text><Action secondary disabled={command.busy || command.blocked} onClick={() => setReviewed(v => !v)}>{reviewed ? '已核对' : '核对'}：准确文件版本、副本和当前任务</Action><TextField name='storage-retry-reason' label='复制重试原因' value={reason} onChange={setReason} disabled={command.busy || command.blocked} /><MFAConfirmation onFresh={setFresh} disabled={command.busy || command.blocked} /><Action loading={command.busy} disabled={!fresh || !reviewed || !reason.trim() || command.blocked} onClick={() => { setError(undefined); try { void command.submit('/storage/replicas/' + value.item.id + '/retry', storageRetryInput(value, reason, reviewed)); } catch (cause) { setError(cause); } }}>确认安排复制重试</Action><Action secondary disabled={command.busy} onClick={onClose}>关闭重试核对</Action></View>;
}
