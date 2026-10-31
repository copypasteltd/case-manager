import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, Check, Pause, Play, UploadCloud, X } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { confirmedBytes, sourceFingerprint, transferUpload, type UploadSession } from './upload';

type Runtime = { source?: File; controller?: AbortController; desired: boolean; paused: boolean; error?: unknown; bytes: number; snapshot?: UploadSession; keys: Map<string, string> };
type QueueContext = {
  items: UploadSession[]; runtime: Map<string, Runtime>; pending: boolean; error: unknown;
  capabilities?: Schemas['UploadCapabilities']; capabilitiesError: unknown;
  more: boolean; loadingMore: boolean; loadMore: () => void; refresh: () => void;
  create: (file: File, input: Schemas['UploadCreate'], key: string) => Promise<UploadSession>;
  resume: (upload: UploadSession, file?: File) => void; pause: (id: string) => void;
  command: <T>(upload: UploadSession, action: string, body: unknown) => Promise<T>;
};
const Context = createContext<QueueContext | null>(null);
export function useUploadQueue(): QueueContext {
  const value = useContext(Context); if (!value) throw new Error('Upload queue provider is required'); return value;
}
const terminal = new Set(['committed', 'aborted', 'expired']);
export const uploadStateNames: Record<string, string> = { created: '等待准备上传', uploading: '等待本机传输', paused: '已暂停', finalizing: '等待固定上传源', verifying: '服务端校验与整理中', blocked: '扫描或策略阻断', ready: '已校验，等待正式提交', committed: '已建立正式版本', expired: '会话已过期', aborted: '已终止', failed: '执行失败' };
export function bytes(value: string | number): string {
  const size = Number(value);
  if (!Number.isFinite(size)) return '未知';
  return size >= 1e9 ? (size / 1e9).toLocaleString('zh-HK', { maximumFractionDigits: 2 }) + ' GB' : size >= 1e6 ? (size / 1e6).toLocaleString('zh-HK', { maximumFractionDigits: 2 }) + ' MB' : size.toLocaleString('zh-HK') + ' 字节';
}

export function UploadProvider({ me, children }: { me: Me; children: ReactNode }) {
  const cache = useQueryClient(), queryKey = useMemo(() => ['uploads', me.user.id], [me.user.id]);
  const runtime = useRef(new Map<string, Runtime>()).current;
  const [revision, setRevision] = useState(0);
  const changed = useCallback(() => setRevision(value => value + 1), []);
  const query = useInfiniteQuery({
    queryKey, initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<UploadSession>>('/uploads?' + new URLSearchParams({ state: 'all', limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
    refetchInterval: 5000,
  });
  const capabilities = useQuery({ queryKey: ['upload-capabilities', me.user.id], queryFn: ({ signal }) => api<Schemas['UploadCapabilities']>('/storage/upload-capabilities', { signal }), refetchInterval: 30_000 });
  const runtimeFor = useCallback((id: string) => {
    let value = runtime.get(id);
    if (!value) { value = { desired: false, paused: false, bytes: 0, keys: new Map() }; runtime.set(id, value); }
    return value;
  }, [runtime]);
  const snapshot = useCallback((upload: UploadSession) => { runtimeFor(upload.id).snapshot = upload; changed(); }, [runtimeFor, changed]);
  const items = useMemo(() => {
    const values = new Map<string, UploadSession>();
    for (const page of query.data?.pages ?? []) for (const item of page.items) {
      const old = values.get(item.id); if (!old || item.version >= old.version) values.set(item.id, item);
    }
    for (const [id, local] of runtime) if (local.snapshot) {
      const current = values.get(id);
      if (current && (local.snapshot.version > current.version || (local.snapshot.version === current.version && local.controller))) values.set(id, local.snapshot);
    }
    return [...values.values()].sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id.localeCompare(a.id));
  }, [query.data, revision, runtime]);
  const command = useCallback(async <T,>(upload: UploadSession, action: string, body: unknown): Promise<T> => {
    const record = runtimeFor(upload.id), signature = action + '/' + JSON.stringify(body);
    let key = record.keys.get(signature);
    if (!key) { key = crypto.randomUUID(); record.keys.set(signature, key); }
    await ensureDownloadSession();
    const result = await api<T>('/uploads/' + upload.id + '/' + action, { method: 'POST', body, key });
    record.keys.delete(signature); return result;
  }, [runtimeFor]);
  const run = useCallback(async (upload: UploadSession) => {
    const record = runtimeFor(upload.id);
    if (!record.source || record.controller || !record.desired || upload.state !== 'uploading') return;
    // One upload at a time for this browser, with at most two cloud parts.
    if ([...runtime.values()].some(item => !!item.controller)) return;
    const controller = new AbortController(); record.controller = controller; record.paused = false; record.error = undefined; changed();
    try {
      await transferUpload({ upload, source: record.source, signal: controller.signal, origin: location.origin,
        ensureSession: () => ensureDownloadSession(controller.signal),
        command: <T,>(action: 'resume' | 'sign-parts' | 'ack-parts' | 'finalize', body: unknown) => command<T>(upload, action, body),
        snapshot, progress: value => { record.bytes = value; changed(); } });
      record.desired = false; record.source = undefined;
    } catch (error) {
      record.desired = false;
      if (!record.paused) record.error = error instanceof Error ? error : new Error('传输中断，请核对上传状态。');
    } finally { record.controller = undefined; changed(); void query.refetch(); }
  }, [runtimeFor, runtime, changed, command, snapshot, query.refetch]);
  useEffect(() => {
    for (const item of items) {
      const record = runtime.get(item.id);
      if (terminal.has(item.state) || Date.parse(item.expires_at) <= Date.now()) {
        if (record) { record.desired = false; record.controller?.abort(); record.source = undefined; }
      } else if (record?.desired && !record.controller && item.state === 'uploading') { void run(item); }
    }
  }, [items, runtime, run]);
  useEffect(() => {
    const before = (event: BeforeUnloadEvent) => {
      if ([...runtime.values()].some(item => item.controller || (item.source && item.desired))) { event.preventDefault(); event.returnValue = ''; }
    };
    window.addEventListener('beforeunload', before);
    return () => { window.removeEventListener('beforeunload', before); for (const record of runtime.values()) { record.desired = false; record.controller?.abort(); record.source = undefined; } };
  }, [runtime]);
  const value: QueueContext = {
    items, runtime, pending: query.isPending, error: query.error, capabilities: capabilities.data, capabilitiesError: capabilities.error,
    more: !!query.hasNextPage, loadingMore: query.isFetchingNextPage, loadMore: () => void query.fetchNextPage(),
    refresh: () => { void query.refetch(); void capabilities.refetch(); },
    create: async (file, input, key) => {
      await ensureDownloadSession();
      const upload = await api<UploadSession>('/uploads', { method: 'POST', body: input, key });
      const record = runtimeFor(upload.id); record.source = file; record.desired = true; record.snapshot = upload; changed();
      cache.setQueryData<InfiniteData<Page<UploadSession>, string>>(queryKey, current => current ? {
        ...current, pages: current.pages.map((page, index) => ({ ...page, items: index === 0 ? [upload, ...page.items.filter(item => item.id !== upload.id)] : page.items.filter(item => item.id !== upload.id) })),
      } : { pages: [{ items: [upload], next_cursor: null }], pageParams: [''] });
      void cache.invalidateQueries({ queryKey }); return upload;
    },
    command,
    resume: (upload, file) => {
      const record = runtimeFor(upload.id); if (file) record.source = file;
      record.desired = true; record.paused = false; record.error = undefined; changed();
      void run(upload);
    },
    pause: id => { const record = runtimeFor(id); record.desired = false; record.paused = true; record.controller?.abort(); changed(); },
  };
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function UploadIndicator() {
  const queue = useUploadQueue();
  const count = queue.items.filter(item => !terminal.has(item.state) && Date.parse(item.expires_at) > Date.now()).length;
  return <Link className="upload-indicator text-button" to="/uploads" aria-label={'上传队列' + (count ? '，' + count + ' 项进行中' : '')}><UploadCloud size={18} /><span>上传队列{count ? ' · ' + count : ''}</span></Link>;
}

export function UploadButton({ folder, file }: { folder?: Schemas['Folder']; file?: Schemas['File'] }) {
  const queue = useUploadQueue(), [open, setOpen] = useState(false);
  if (!(file ? file.can_update && file.current_version_id : folder?.can_upload)) return null;
  const reason = queue.capabilitiesError ? '暂时无法确认上传接入，请刷新后重试。' : !queue.capabilities ? '正在确认上传接入。' : !queue.capabilities.control_ready ? '上传接入尚未配置，请联系管理员。' : '';
  return <div className="upload-entry"><button className="button primary" onClick={() => setOpen(true)} disabled={!!reason}><UploadCloud size={17} />{file ? '上传新内容版本' : '上传文件'}</button>{reason && <small className="muted">{reason}</small>}{open && <CreateUpload folder={folder} file={file} onClose={() => setOpen(false)} />}</div>;
}

function CreateUpload({ folder, file, onClose }: { folder?: Schemas['Folder']; file?: Schemas['File']; onClose: () => void }) {
  const queue = useUploadQueue(), [source, setSource] = useState<File>(), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const last = useRef<{ signature: string; key: string }>();
  const [created, setCreated] = useState(false);
  async function submit() {
    if (!source) return;
    setBusy(true); setError(undefined);
    try {
      const input: Schemas['UploadCreate'] = { folder_id: file?.folder_id ?? folder!.id, name: source.name, size_bytes: String(source.size),
        media_type: (source.type || 'application/octet-stream').toLowerCase().split(';')[0].trim(), source_fingerprint: await sourceFingerprint(source),
        ...(file?.current_version_id ? { file_id: file.id, base_version_id: file.current_version_id } : {}) };
      const signature = JSON.stringify(input);
      if (last.current?.signature !== signature) last.current = { signature, key: crypto.randomUUID() };
      await queue.create(source, input, last.current!.key); setCreated(true);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return <Dialog title={file ? '上传新内容版本' : '上传文件'} onClose={onClose} busy={busy}>{created ? <><p className="notice success"><Check size={18} />已加入上传队列。页面切换期间会继续传输。</p><p className="muted">请保持浏览器运行，后续服务端校验和正式提交进度可在上传队列查看。</p><div className="dialog-actions"><button className="button secondary" onClick={onClose}>继续整理资料</button><Link className="button primary" to="/uploads" onClick={onClose}>查看上传队列<ArrowRight size={16} /></Link></div></> : <form onSubmit={event => { event.preventDefault(); void submit(); }}><ErrorNotice error={error} /><p className="muted">目标：{file?.name ?? folder?.name}。支持至 120 GB；文件分片直接传往存储，之后仍需校验与正式提交。</p>{file && <p className="notice neutral">以打开此窗口时的正式内容版本为基础。期间出现新版本会提示冲突，请核对后处理。</p>}<Field label="选择原文件" hint="刷新或关闭浏览器会停止本机传输；继续时需要重新选择相同原文件。"><input type="file" required disabled={busy} onChange={event => { setSource(event.target.files?.[0]); setError(undefined); }} /></Field>{source && <p className="muted">{source.name} · {bytes(source.size)}</p>}<div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose} disabled={busy}>取消</button><button className="button primary" disabled={!source || busy}>{busy ? '正在建立上传' : '加入上传队列'}</button></div></form>}</Dialog>;
}

export function UploadQueuePage() {
  const queue = useUploadQueue();
  return <><Link className="back-link" to="/files"><ArrowLeft size={16} />返回文件中心</Link><div className="page-heading"><div><span className="eyebrow">FROM LOCAL TO SHARED</span><h1>上传队列<span className="accent">.</span></h1><p>追踪本机传输、服务端校验和正式版本提交。</p></div><Link className="button secondary" to="/scan-waivers">扫描例外申请<ArrowRight size={16} /></Link></div>
    <div className="notice neutral"><UploadCloud size={18} /><span>页面切换会保留本机队列。关闭浏览器或设备休眠可能中断传输；继续时先核对云端分片。正式提交后，在文件版本记录中查看双云备份。</span></div>
    <ErrorNotice error={queue.error ?? queue.capabilitiesError} />{queue.capabilities && !queue.capabilities.control_ready && <p className="notice warning">上传接入尚未配置，当前不能建立新的上传。已有记录仍可查阅。</p>}
    <div className="section-heading"><h2>我的上传记录</h2><button className="text-button" onClick={queue.refresh}>刷新状态</button></div>{queue.pending ? <Busy label="正在读取上传记录" /> : !queue.items.length ? <div className="panel"><Empty title="当前没有可见的上传记录" detail="进入获授权的资料目录，选择文件并加入队列。" action={<Link className="button primary" to="/files">选择资料目录</Link>} /></div> : <div className="upload-records">{queue.items.map(item => <UploadCard key={item.id} upload={item} />)}</div>}
    {queue.more && <div className="list-footer"><span>已显示 {queue.items.length} 项</span><button className="button secondary" disabled={queue.loadingMore} onClick={queue.loadMore}>加载更多</button></div>}
  </>;
}

function UploadCard({ upload }: { upload: UploadSession }) {
  const queue = useUploadQueue(), local = queue.runtime.get(upload.id), [action, setAction] = useState<'abort' | 'commit' | 'rescan' | 'waivers' | null>(null), [report, setReport] = useState(false);
  const reselect = useRef<HTMLInputElement>(null);
  const expired = Date.parse(upload.expires_at) <= Date.now() && !terminal.has(upload.state), active = !upload.retirement_plan_id && !expired && !terminal.has(upload.state);
  const transferring = !!local?.controller, sent = local?.bytes ?? 0;
  let cloud = 0; try { cloud = confirmedBytes(upload); } catch {}
  const label = upload.retirement_plan_id ? '已批准清理，不可继续提交' : expired ? '会话已过期' : transferring ? '本机分片传输中' : local?.paused && ['created', 'uploading'].includes(upload.state) ? '本机传输已暂停' : local?.desired && upload.state === 'uploading' ? '等待本机传输名额' : uploadStateNames[upload.state] ?? upload.state;
  return <article className="panel upload-card" data-upload-id={upload.id}><div className="section-heading"><div><h3>{upload.name}</h3><small className="muted">{bytes(upload.size_bytes)} · {date(upload.created_at)}</small></div><span className={'badge ' + (upload.state === 'committed' ? 'success' : ['blocked', 'failed'].includes(upload.state) || expired ? 'warning' : 'neutral')}>{label}</span></div>
    <dl className="upload-facts"><div><dt>云端已确认分片</dt><dd>{upload.confirmed_parts.length} / {upload.part_count} · {bytes(cloud)}</dd></div><div><dt>完整源摘要进度</dt><dd>{bytes(upload.verified_bytes)} / {bytes(upload.size_bytes)}</dd></div><div><dt>本次传输已收到的分片回执</dt><dd>{bytes(sent)}</dd></div><div><dt>会话有效至</dt><dd>{new Date(upload.expires_at).toLocaleString('zh-HK', { timeZone: 'Asia/Hong_Kong' })}</dd></div></dl>
    {transferring && <progress className="download-progress" value={sent} max={Number(upload.size_bytes)} aria-label="本机已收到分片回执的字节数" />}
    <ErrorNotice error={local?.error} />{upload.error_code && <p className="notice warning">{terminal.has(upload.state) ? '上次执行记录' : '服务端当前未完成此阶段'}，状态代码：{upload.error_code}。请查看扫描报告或联系管理员核对执行记录。</p>}
    {upload.scan_state === 'waived' && <p className="notice warning">此内容经扫描例外放行，尚未取得完整安全扫描通过的结论。</p>}
    {!upload.retirement_plan_id && upload.state === 'ready' && <p className="muted">候选内容已校验。确认后建立正式版本，再独立排队复制到备份端。</p>}
    <div className="upload-actions"><Link className="text-button" to={'/files/folders/' + upload.folder_id}>查看目标目录<ArrowRight size={15} /></Link>
      {active && ['created', 'uploading'].includes(upload.state) && <>{transferring || local?.desired ? <button className="button secondary" onClick={() => queue.pause(upload.id)}><Pause size={15} />暂停本机传输</button> : <button className="button secondary" onClick={() => local?.source ? queue.resume(upload) : reselect.current?.click()}><Play size={15} />{local?.source ? '核对分片并继续' : '重选原文件并继续'}</button>}<input ref={reselect} type="file" className="sr-only" aria-label={'重新选择原文件 ' + upload.name} onChange={event => { const file = event.target.files?.[0]; if (file) queue.resume(upload, file); event.target.value = ''; }} /></>}
      {upload.scan_result_id && <button className="text-button" onClick={() => setReport(true)}>查看扫描报告</button>}
      {active && upload.state === 'blocked' && upload.scan_state === 'unscannable' && <button className="button secondary" onClick={() => setAction('waivers')}>申请扫描例外</button>}
      {active && upload.state === 'blocked' && <button className="text-button" onClick={() => setAction('rescan')}>重新安排扫描</button>}
      {active && upload.state === 'ready' && <button className="button primary" disabled={!upload.sha256 || !upload.scan_result_id || !['clean', 'waived'].includes(upload.scan_state)} onClick={() => setAction('commit')}><Check size={16} />确认建立正式版本</button>}
      {active && !transferring && <button className="text-button danger-text" onClick={() => setAction('abort')}><X size={15} />终止上传</button>}
      {upload.state === 'committed' && upload.result_file_id && <Link className="button secondary" to={'/files/' + upload.result_file_id}>查看正式版本与备份<ArrowRight size={16} /></Link>}
    </div>{action && <UploadAction upload={upload} action={action} onClose={() => setAction(null)} />}{report && <ScanReport upload={upload} onClose={() => setReport(false)} />}
  </article>;
}

function ScanReport({ upload, onClose }: { upload: UploadSession; onClose: () => void }) {
  const report = useQuery({ queryKey: ['upload-scan-report', upload.id, upload.scan_result_id], queryFn: ({ signal }) => api<Schemas['UploadScanReport']>('/uploads/' + upload.id + '/scan-report', { signal }) });
  return <Dialog title="固定源扫描报告" onClose={onClose}><ErrorNotice error={report.error} />{report.isPending ? <Busy label="正在读取扫描证据" /> : report.data && <><p className="notice neutral">扫描报告保留原始结论；例外放行不会将“无法扫描”改成“扫描通过”。</p><dl className="file-facts"><div><dt>原始结论</dt><dd>{{ clean: '扫描通过', blocked: '已阻断', unscannable: '无法扫描' }[report.data.state]}</dd></div><div><dt>结论代码</dt><dd>{report.data.code}</dd></div><div><dt>实际送检内容</dt><dd>{bytes(report.data.streamed_bytes)} / {bytes(report.data.size_bytes)}</dd></div><div><dt>引擎与签名库</dt><dd>{report.data.engine ?? '未提供'} · {report.data.engine_version ?? '未提供'} · {report.data.signature_revision ?? '未提供'}</dd></div><div className="wide"><dt>固定源 SHA-256</dt><dd><code>{report.data.sha256}</code></dd></div></dl></>}<div className="dialog-actions"><button className="button secondary" onClick={onClose}>关闭报告</button></div></Dialog>;
}

function UploadAction({ upload: original, action, onClose }: { upload: UploadSession; action: 'abort' | 'commit' | 'rescan' | 'waivers'; onClose: () => void }) {
  const queue = useUploadQueue(), cache = useQueryClient(), [upload] = useState(original);
  const [reason, setReason] = useState(''), [risk, setRisk] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [result, setResult] = useState<Schemas['ScanWaiver']>();
  const title = { abort: '终止上传', commit: '建立正式内容版本', rescan: '重新安排固定源扫描', waivers: '申请扫描例外' }[action];
  async function submit() {
    setBusy(true); setError(undefined);
    try {
      const body = action === 'commit' ? { expected_version: upload.version, sha256: upload.sha256, ...(upload.base_version_id ? { base_version_id: upload.base_version_id } : {}) } : action === 'waivers' ? { expected_version: upload.version, scan_report_id: upload.scan_result_id, sha256: upload.sha256, reason, acknowledged_risk: risk } : action === 'abort' ? { expected_version: upload.version, action: 'abort', reason } : { expected_version: upload.version };
      const value = await queue.command<Schemas['ScanWaiver']>(upload, action, body);
      queue.pause(upload.id); queue.refresh();
      await Promise.all([cache.invalidateQueries({ queryKey: ['files'] }), cache.invalidateQueries({ queryKey: ['file'] }), cache.invalidateQueries({ queryKey: ['file-versions'] }), cache.invalidateQueries({ queryKey: ['scan-waivers'] })]);
      if (action === 'waivers') setResult(value); else onClose();
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  const conflict = error instanceof ApiError && ['VERSION_CONFLICT', 'BASE_VERSION_CONFLICT', 'UPLOAD_STATE_CONFLICT', 'DUPLICATE_RESOURCE'].includes(error.code);
  return <Dialog title={title} busy={busy} onClose={onClose}>{result ? <><p className="notice success">例外申请已保存，等待独立审核人处理。</p><div className="dialog-actions"><button className="button secondary" onClick={onClose}>关闭</button><Link className="button primary" to={'/scan-waivers/' + result.id} onClick={onClose}>查看申请</Link></div></> : <form onSubmit={event => { event.preventDefault(); void submit(); }}><ErrorNotice error={error} />{conflict && <p className="notice neutral">资料已改变，当前填写内容保留供核对。请关闭窗口刷新状态后重新发起。</p>}<p><strong>{upload.name}</strong> · {bytes(upload.size_bytes)}</p>{action === 'commit' && <><p className="muted">将候选内容建立为正式版本，备份随后独立执行。若基础版本或目录名称发生冲突，本次候选保留等待处理。</p><p className="digest-line">SHA-256：<code>{upload.sha256}</code></p>{upload.scan_state === 'waived' && <p className="notice warning">内容经扫描例外放行，完整扫描未通过；正式版本将保留此标记。</p>}</>}{action === 'abort' && <p className="notice warning">终止后不再接受提交。已经签发的分片许可最长还可能有效五分钟；临时对象清理按后台保留制度执行。</p>}{action === 'rescan' && <p className="muted">对同一固定源重新安排扫描，未决例外会失效。排队成功后仍需等待真实扫描结果。</p>}{action === 'waivers' && <><p className="notice warning">例外仅限当前制度明确允许的类型、大小和无法扫描原因，需独立审核人最近验证身份后决定。恶意内容和严格受限分类不能通过此路径放行。</p><label className="checkbox-row"><input type="checkbox" required checked={risk} onChange={event => setRisk(event.target.checked)} />我已核对固定扫描报告，并知悉内容未完成安全扫描</label></>}{['abort', 'waivers'].includes(action) && <Field label="申请或操作原因"><textarea rows={3} required maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field>}<div className="dialog-actions"><button className="button secondary" type="button" disabled={busy} onClick={onClose}>取消</button><button className={'button ' + (action === 'abort' ? 'danger' : 'primary')} disabled={busy || conflict || (action === 'waivers' && !risk)}>{busy ? '正在提交' : title}</button></div></form>}</Dialog>;
}
