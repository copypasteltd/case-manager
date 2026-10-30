import { Text, View } from '@tarojs/components';
import { useRouter } from '@tarojs/taro';
import { useState, useSyncExternalStore } from 'react';
import { client } from '../../platform/taro';
import { nativeUploads, uploadTransfer } from '../../platform/uploads';
import { identifier, go, usePage } from '../../platform/page';
import { useCollection } from '../../platform/collection';
import { useCommand, CommandNotice } from '../../platform/command';
import { type Schemas, MobileError } from '../../platform/session';
import type { NativeUploadSource, Upload } from '../../platform/upload-transfer';
import { ChoiceList, TextField } from '../business/controls';
import { Action, Busy, Check, Empty, Fact, Notice, Page, Row, Section, State, bytes, date } from '../../ui';

type Data = { sources: NativeUploadSource[]; capability: Schemas['UploadCapabilities']; folder: Schemas['Folder'] | null; file: Schemas['File'] | null };
const localStates: Record<string, string> = { selected: '尚未指定目标', prepared: '目标已固定', paused: '本机已暂停', transferring: '本机传输中', needs_source: '需要重新选择原文件', uncertain: '结果待核对', awaiting_verification: '等待服务端处理', waiting_session: '需要重新登录' };
const phases: Record<string, string> = { idle: '尚未开始', checking: '核对身份与原文件', creating: '建立上传会话', reconciling: '核对云端已有分片', signing: '取得本次分片许可', transferring: '原生传输中', acknowledging: '记录分片回执', finalizing: '提交服务端校验', server_processing: '等待服务端处理', paused: '本机已暂停', uncertain: '结果待核对', waiting_session: '需要重新登录' };
const remoteStates: Record<string, string> = { created: '准备上传中', uploading: '等待或正在上传', paused: '已暂停', finalizing: '正在冻结上传源', verifying: '校验／扫描／整理中', ready: '正式候选已就绪', committed: '已提交正式版本', blocked: '已阻断', expired: '会话已过期', aborted: '已终止', failed: '未能完成' };
export default function UploadView() {
  const params = useRouter().params;
  const page = usePage<Data>(async (scope, me) => {
    const folderID = params.folder ? identifier(params.folder) : null, fileID = params.file ? identifier(params.file) : null;
    await nativeUploads.scope(me.user.id);
    const [sources, capability, folder, file] = await Promise.all([nativeUploads.list(), client.request<Schemas['UploadCapabilities']>('/storage/upload-capabilities', { scope }),
      folderID ? client.request<Schemas['Folder']>('/folders/' + folderID, { scope }) : null, fileID ? client.request<Schemas['File']>('/files/' + fileID, { scope }) : null]);
    if (file && (file.folder_id !== folderID || !params.base || file.current_version_id !== identifier(params.base))) throw new MobileError('UPLOAD_BASE_CHANGED', '文件当前版本已变化，请从文件详情重新进入上传。');
    return { sources, capability, folder, file };
  });
  return <Page title='上传队列' home><Notice error={page.error} />{page.loading ? <Busy /> : page.result && <Workspace key={page.result.me.user.id + ':' + client.epoch} data={page.result.value} owner={page.result.me.user.id} reload={() => void page.reload()} />}<Action secondary loading={page.loading} onClick={() => void page.reload()}>刷新队列与真实状态</Action></Page>;
}
function Workspace({ data, owner, reload }: { data: Data; owner: string; reload(): void }) {
  const transfer = useSyncExternalStore(uploadTransfer.subscribe, uploadTransfer.snapshot);
  const queue = useCollection<Upload>('/uploads?state=all&limit=20');
  const [folder, setFolder] = useState<Schemas['Folder'] | null>(data.folder), [choosingFolder, setChoosingFolder] = useState(false);
  const [selected, setSelected] = useState<NativeUploadSource>(), [name, setName] = useState('');
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [confirmed, setConfirmed] = useState(false);
  async function action(task: () => Promise<void>) {
    if (busy) return; setBusy(true); setError(undefined);
    try { await task(); } catch (cause) { setError(cause); } finally { setBusy(false); }
  }
  const refresh = () => { queue.reload(); reload(); };
  async function start(source: NativeUploadSource) {
    await action(async () => { await uploadTransfer.start(source.handle, owner); refresh(); });
  }
  return <>
    <Notice error={error ?? transfer.error} />
    {!data.capability.control_ready && <View className='cp-notice'><Text>存储上传接入尚未就绪，不能新建上传。现有任务可以查看真实状态。</Text>{data.capability.reason_code && <Text className='cp-muted cp-small'>原因 {data.capability.reason_code}</Text>}</View>}
    <Text className='cp-muted cp-small'>上限120 GB。切页保留本机任务；系统终止后需重新登录并手动继续，首先核对云端分片。原文件须支持持久授权和随机读取，设备不复制整份文件。</Text>
    {transfer.handle && <Section title='本次传输'><View className='cp-pad'><Text>{phases[transfer.phase] ?? '状态待核对'}</Text><Fact label='本机已传（尚待完整对账）'><Text>{bytes(String(Math.min(transfer.size, transfer.transferred + transfer.currentPartBytes)))} / {bytes(String(transfer.size))}</Text></Fact><Fact label='云端已核对'><Text>{bytes(String(transfer.confirmed))}</Text></Fact>{transfer.running && <Action secondary onClick={() => uploadTransfer.pause()}>暂停本机传输</Action>}</View></Section>}
    <Section title={data.file ? '固定基础版的新内容' : '新建文件上传'}><View className='cp-pad'>
      {folder ? <Fact label='目标目录'><Text>{folder.name}{!folder.can_upload && ' · 当前不可上传'}</Text></Fact> : <Text>请先选择目标目录。</Text>}
      {data.file && <><Fact label='业务文件'><Text>{data.file.name}</Text></Fact><Fact label='固定基础版本'><Text selectable>{data.file.current_version_id}</Text></Fact></>}
      {!data.folder && !data.file && <Action secondary disabled={busy || transfer.running} onClick={() => setChoosingFolder(true)}>选择目标目录</Action>}
      {choosingFolder && <ChoiceList<Schemas['Folder']> title='选择可上传目录' path='/folders?limit=20' label={value => value.name} eligible={value => value.can_upload} onSelect={value => { setFolder(value); setChoosingFolder(false); setConfirmed(false); }} onClose={() => setChoosingFolder(false)} />}
      <Action secondary disabled={busy || transfer.running || !data.capability.control_ready || !folder?.can_upload} onClick={() => void action(async () => { const value = await nativeUploads.select(); setSelected(value); setName(value.name); setConfirmed(false); })}>从系统文件选择器选择原件</Action>
      {selected && <><Fact label='所选原文件'><Text>{selected.name} · {bytes(selected.size_bytes)}</Text></Fact><TextField name='upload-name' label='文件名称' value={name} onChange={value => { setName(value); setConfirmed(false); }} disabled={busy || transfer.running} max={255} /><Check checked={confirmed} disabled={busy || transfer.running} onChange={setConfirmed}>我已核对原文件、目标目录{data.file ? '及固定基础版本' : ''}。后续将按此目标建立会话。</Check><Action disabled={!confirmed || !folder?.can_upload || busy || transfer.running || !name.trim()} onClick={() => void action(async () => {
        if (!folder) return; await nativeUploads.prepare(selected.handle, { folder_id: folder.id, name: name.trim(), ...(data.file?.current_version_id ? { file_id: data.file.id, base_version_id: data.file.current_version_id } : {}) }); refresh();
      })}>固定目标并保存上传准备</Action></>}
    </View></Section>
    <Section title='本机恢复记录'>{data.sources.length ? data.sources.map(source => <View className='cp-pad' key={source.handle}><Text className='cp-row-title'>{source.name}</Text><Text>{bytes(source.size_bytes)} · {localStates[source.state] ?? '待核对'}</Text>{source.upload_id && <Text className='cp-muted cp-small' selectable>上传会话 {source.upload_id}</Text>}
      {source.create_body && <Text className='cp-muted cp-small'>上传目标已固定；核对创建结果时沿用原操作，不另建会话。</Text>}
      {!source.create_body && !source.upload_id && <Action secondary disabled={busy || transfer.running} onClick={() => { setSelected(source); setName(source.name); setConfirmed(false); }}>为这份原件指定目标</Action>}
      {(!source.has_source || source.upload_id) && <Action secondary disabled={busy || transfer.running} onClick={() => void action(async () => { await nativeUploads.select(source.handle); refresh(); })}>重新选择原文件／修复读取授权</Action>}
      {source.has_source && (source.create_body || source.upload_id) && <Action disabled={busy || transfer.running || !data.capability.control_ready} onClick={() => void start(source)}>{source.upload_id ? '核对云端后继续' : '建立或核对原上传会话'}</Action>}
      <Action secondary disabled={busy || transfer.running} onClick={() => void action(async () => { await nativeUploads.forget(source.handle); refresh(); })}>释放本机记录与文件授权</Action>
    </View>) : <Empty>没有保存的本机原文件。服务端已有会话仍在下方队列中。</Empty>}</Section>
    <Section title='服务端本人上传'><Notice error={queue.error} />{queue.loading ? <Busy /> : queue.items.length ? queue.items.map(upload => <ServerUpload key={upload.id + ':' + upload.version} upload={upload} disabled={busy || transfer.running} reload={refresh} source={data.sources.find(source => source.upload_id === upload.id)} attach={value => void action(async () => { const picked = await nativeUploads.select(); await nativeUploads.bind(picked.handle, value); refresh(); })} />) : <Empty>没有符合当前权限的上传会话。</Empty>}{queue.next && <Action secondary loading={queue.moreBusy} onClick={() => void queue.more()}>加载更多上传</Action>}</Section>
  </>;
}
function ServerUpload({ upload, disabled, reload, source, attach }: { upload: Upload; disabled: boolean; reload(): void; source?: NativeUploadSource; attach(value: Upload): void }) {
  const [commit, setCommit] = useState(false), [abort, setAbort] = useState(false);
  const command = useCommand<Record<string, unknown>>(() => reload(), 'android-upload:' + upload.id + ':' + upload.version);
  return <View className='cp-pad'><Text className='cp-row-title'>{upload.name}</Text><Text>{remoteStates[upload.state] ?? upload.state} · {bytes(upload.size_bytes)}</Text><Text className='cp-muted cp-small'>会话期限 {date(upload.expires_at)} · 扫描 <State value={upload.scan_state} /></Text><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    {upload.state === 'uploading' && !source && (upload.source_fingerprint.startsWith('android-saf-v1:') || upload.source_fingerprint.startsWith('office-artifact-v1:') ? <Action secondary disabled={disabled || command.busy} onClick={() => attach(upload)}>重新选择原件并关联这个会话</Action> : <Text className='cp-muted cp-small'>该会话的原文件指纹来自 Web，请在原上传端继续。</Text>)}
    {upload.state === 'ready' && upload.sha256 && <><Fact label='已核对的完整 SHA-256'><Text className='cp-code' selectable>{upload.sha256}</Text></Fact><Text className='cp-muted cp-small'>正式提交后主端与 R2 副本状态分别展示；副本仍可能排队或失败。</Text><Check checked={commit} onChange={setCommit} disabled={disabled || command.busy || command.blocked}>我确认提交这份准确候选{upload.base_version_id ? '，使用原固定基础版本' : ''}为正式内容。</Check><Action disabled={!commit || disabled || command.blocked} loading={command.busy} onClick={() => void command.submit('/uploads/' + upload.id + '/commit', { expected_version: upload.version, sha256: upload.sha256, ...(upload.base_version_id ? { base_version_id: upload.base_version_id } : {}) })}>确认提交正式版本</Action></>}
    {upload.result_file_id && <Row title='查看正式文件与副本状态' onClick={() => go('file', upload.result_file_id!)} />}
    {!['committed', 'aborted', 'expired'].includes(upload.state) && <><Check checked={abort} onChange={setAbort} disabled={disabled || command.busy || command.blocked}>我确认终止这个上传会话；此操作不是暂停本机传输。</Check><Action secondary disabled={!abort || disabled || command.blocked} loading={command.busy} onClick={() => void command.submit('/uploads/' + upload.id + '/abort', { expected_version: upload.version, action: 'abort', reason: '本人在Android上传队列明确终止此会话' })}>终止服务端上传会话</Action></>}
  </View>;
}
