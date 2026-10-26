import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ArrowRight, Check, RotateCcw, X } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Page, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';

type StoredFile = Schemas['File'];
type FileVersion = Schemas['FileVersion'];
type Restoration = Schemas['HistoryRestore'];
const labels: Record<Restoration['state'], string> = {
  queued: '等待复制', copying: '正在复制历史内容', verifying: '正在校验候选', retry_wait: '等待自动重试',
  ready: '待确认提交', committed: '已生成正式版本', cancelled: '已取消', failed: '恢复未完成',
};
const bytes = (value: string) => BigInt(value).toLocaleString('zh-HK') + ' 字节';
function useCommandKey() {
  const previous = useRef({ body: '', key: '' });
  return (body: unknown) => {
    const encoded = JSON.stringify(body);
    if (previous.current.body !== encoded) previous.current = { body: encoded, key: crypto.randomUUID() };
    return previous.current.key;
  };
}
async function refreshFile(cache: ReturnType<typeof useQueryClient>, fileID: string) {
  await Promise.all([
    cache.invalidateQueries({ queryKey: ['file', fileID] }),
    cache.invalidateQueries({ queryKey: ['file-versions', fileID] }),
    cache.invalidateQueries({ queryKey: ['history-restores', fileID] }),
    cache.invalidateQueries({ queryKey: ['files'] }),
  ]);
}
function blocked(error: unknown) {
  return error instanceof ApiError && ['VERSION_CONFLICT', 'BASE_VERSION_CONFLICT', 'RESTORE_TARGET_CHANGED',
    'RESTORE_EXPIRED', 'AUTHORIZATION_CHANGED', 'RESTORE_STATE_CONFLICT'].includes(error.code);
}

export function RestoreVersionButton({ file, version }: { file: StoredFile; version: FileVersion }) {
  const [open, setOpen] = useState(false);
  if (!file.can_restore_history || version.id === file.current_version_id) return null;
  const available = version.availability === 'available' && ['clean', 'waived'].includes(version.scan_state);
  return <>
    <button className="button secondary" disabled={!available} onClick={() => setOpen(true)}
      title={available ? undefined : '此历史版本缺少可用内容及扫描或放行依据。'}><RotateCcw size={16} />恢复为新版本</button>
    {open && <CreateRestoration file={file} version={version} onClose={() => setOpen(false)} />}
  </>;
}
function CreateRestoration({ file, version, onClose }: { file: StoredFile; version: FileVersion; onClose: () => void }) {
  const [base] = useState({ file, version }), [reason, setReason] = useState('');
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({
    mutationFn: async () => {
      const body: Schemas['HistoryRestoreCreate'] = { expected_version: base.file.version, source_version_id: base.version.id, reason };
      await ensureDownloadSession();
      return api<Restoration>('/files/' + base.file.id + '/history-restores', { method: 'POST', body, key: key(body) });
    },
    onSuccess: async () => { await refreshFile(cache, base.file.id); onClose(); document.getElementById('history-restoration')?.scrollIntoView({ block: 'start' }); },
  });
  return <Dialog title="恢复历史内容为新版本" onClose={onClose} busy={mutation.isPending}>
    <form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
      <p>将「{base.file.name}」的内容版本 {base.version.seq} 复制为独立候选；现有内容和全部历史继续保留。</p>
      <dl className="file-facts"><div><dt>恢复大小</dt><dd>{bytes(base.version.size_bytes)}</dd></div>
        <div><dt>来源状态</dt><dd>{base.version.scan_state === 'waived' ? '经扫描例外放行' : '扫描通过'}</dd></div>
        <div className="wide"><dt>已固定的来源 SHA-256</dt><dd><code>{base.version.sha256}</code></dd></div></dl>
      <p className="notice neutral">服务端复制和完整校验可能需要时间。候选就绪后，需要在本页再次明确提交，才会生成新的正式版本。</p>
      {base.version.scan_state === 'waived' && <p className="notice warning">来源仍属例外放行。恢复会核对原批准是否仍有效，并保留原状态；复制校验不代表重新通过安全扫描。</p>}
      <ErrorNotice error={mutation.error} />
      {blocked(mutation.error) && <p className="notice neutral">文件或权限已改变。填写内容保留供核对，请关闭窗口并读取最新资料后重新发起。</p>}
      <Field label="恢复原因"><textarea required maxLength={200} rows={3} value={reason} onChange={event => setReason(event.target.value)} autoFocus /></Field>
      <div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>返回核对</button>
        <button className="button primary" disabled={mutation.isPending || blocked(mutation.error)}>创建恢复候选</button></div>
    </form>
  </Dialog>;
}

export function HistoryRestores({ file }: { file: StoredFile }) {
  const [clock, setClock] = useState(Date.now());
  const [action, setAction] = useState<{ run: Restoration; kind: 'commit' | 'cancel' } | null>(null);
  useEffect(() => { const timer = window.setInterval(() => setClock(Date.now()), 15_000); return () => clearInterval(timer); }, []);
  const runs = useInfiniteQuery({
    queryKey: ['history-restores', file.id], initialPageParam: '', enabled: file.can_view_history_restores,
    queryFn: ({ pageParam, signal }) => api<Page<Restoration>>('/files/' + file.id + '/history-restores?' +
      new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined, refetchInterval: 5000,
  });
  if (!file.can_view_history_restores) return null;
  const items = runs.data?.pages.flatMap(page => page.items) ?? [];
  return <section className="section-block history-restoration" id="history-restoration">
    <div className="section-heading"><h2>我的历史恢复任务</h2><small className="muted">候选校验与正式提交分别确认</small></div>
    <ErrorNotice error={runs.error} />
    {!runs.error && (runs.isPending ? <Busy label="正在读取历史恢复任务" /> : !items.length ?
      <div className="panel"><Empty title="尚无历史恢复任务" detail="在有权的历史版本上选择“恢复为新版本”，复制与校验完成后再明确提交。" /></div> :
      <div className="history-restore-list">{items.map(run => {
        const expired = Date.parse(run.expires_at) <= clock, finished = ['committed', 'cancelled', 'failed'].includes(run.state);
        return <article className="panel history-restore-card" key={run.id} data-restore-id={run.id}>
          <div className="section-heading"><h3>历史内容恢复</h3><span className={'badge ' + (run.state === 'committed' ? 'success' : ['failed', 'retry_wait'].includes(run.state) ? 'warning' : 'neutral')}>{labels[run.state]}</span></div>
          <p className="muted">{date(run.created_at)} · 原因：{run.reason}</p>
          <dl className="file-facts">
            <div><dt>来源内容版本</dt><dd className="restore-reference">内容版本 {run.source_seq}</dd></div>
            <div><dt>固定基础内容版本</dt><dd className="restore-reference">内容版本 {run.base_seq}</dd></div>
            <div><dt>云端确认的候选分片</dt><dd>{bytes(run.copied_bytes)} / {bytes(run.size_bytes)}</dd></div>
            <div><dt>目标完整校验进度</dt><dd>{bytes(run.verified_bytes)} / {bytes(run.size_bytes)}</dd></div>
            <div className="wide"><dt>来源 SHA-256</dt><dd><code>{run.sha256}</code></dd></div>
          </dl>
          {run.state === 'ready' && !expired && <p className="notice neutral">候选已校验，当前正式内容尚未改变。确认基础版本和来源摘要后提交；主端可用后，次端仍需独立复制和校验。</p>}
          {expired && !finished && <p className="notice warning">此恢复任务已超过有效期。请重新创建候选；保留的旧文件和版本不会因此删除。</p>}
          {run.error_code && <p className={'notice ' + (finished ? 'neutral' : 'warning')}>执行记录：{run.error_code}{run.state === 'retry_wait' ? ' · 服务端将按任务策略重试。' : ''}</p>}
          {run.state === 'committed' && run.result_version_id && <p className="muted">已生成正式版本，来源与基础版本均可在上方内容记录中核对。</p>}
          <div className="post-actions">
            {run.state === 'ready' && !expired && file.can_restore_history && <button className="button primary" onClick={() => setAction({ run, kind: 'commit' })}><Check size={16} />确认并生成正式版本</button>}
            {!finished && <button className="button secondary" onClick={() => setAction({ run, kind: 'cancel' })}><X size={16} />取消恢复任务</button>}
          </div>
        </article>;
      })}</div>)}
    {!runs.error && runs.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 项</span><button className="button secondary" disabled={runs.isFetchingNextPage} onClick={() => void runs.fetchNextPage()}>加载更多<ArrowRight size={15} /></button></div>}
    {action && <RestorationAction run={action.run} kind={action.kind} file={file} onClose={() => setAction(null)} />}
  </section>;
}
function RestorationAction({ run, kind, file, onClose }: { run: Restoration; kind: 'commit' | 'cancel'; file: StoredFile; onClose: () => void }) {
  const [base] = useState(run), [reason, setReason] = useState(''), [confirmed, setConfirmed] = useState(false);
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({
    mutationFn: async () => {
      const body: Schemas['HistoryRestoreCommit'] | Schemas['HistoryRestoreCancel'] = kind === 'commit' ?
        { expected_version: base.version, base_version_id: base.base_version_id, sha256: base.sha256 } : { expected_version: base.version, reason };
      await ensureDownloadSession();
      return api('/history-restores/' + base.id + '/' + kind, { method: 'POST', body, key: key(body) });
    },
    onSuccess: async () => { await refreshFile(cache, file.id); onClose(); },
  });
  return <Dialog title={kind === 'commit' ? '确认历史恢复的正式版本' : '取消历史恢复任务'} onClose={onClose} busy={mutation.isPending}>
    <form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
      <p>{kind === 'commit' ? '将已校验的候选发布为「' + file.name + '」的新版本。已有版本全部保留；当前文件发生变化时，服务端会拒绝覆盖。' :
        '停止此恢复任务并保留执行记录。旧文件和已有历史继续保留，已产生的候选另按保留制度清理。'}</p>
      <dl className="file-facts"><div className="wide"><dt>固定基础内容版本</dt><dd className="restore-reference">内容版本 {base.base_seq}</dd></div>
        <div className="wide"><dt>已确认的内容 SHA-256</dt><dd><code>{base.sha256}</code></dd></div></dl>
      <ErrorNotice error={mutation.error} />
      {blocked(mutation.error) && <p className="notice neutral">任务或文件已改变，候选和填写内容保留。请关闭窗口，重新核对当前资料；系统不会自动改用新的基础版本。</p>}
      {kind === 'cancel' ? <Field label="取消原因"><textarea required maxLength={200} rows={3} value={reason} onChange={event => setReason(event.target.value)} autoFocus /></Field> :
        <label className="checkbox-label"><input type="checkbox" required checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />我已核对来源摘要与基础内容版本，确认生成新的正式版本。</label>}
      <div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose} disabled={mutation.isPending}>返回核对</button>
        <button className={'button ' + (kind === 'commit' ? 'primary' : 'danger-outline')} disabled={mutation.isPending || blocked(mutation.error) || (kind === 'commit' && !confirmed)}>
          {kind === 'commit' ? '生成新的正式版本' : '确认取消恢复'}</button></div>
    </form>
  </Dialog>;
}
