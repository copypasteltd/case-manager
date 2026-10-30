import { useRef, useState, type FormEvent } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, ArrowUpRight, Folder, FolderPlus, Search, Pencil, Trash2, RotateCcw, ShieldCheck, Download } from 'lucide-react';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { DownloadDialog } from './DownloadDialog';
import { MultipartCleanup } from './MultipartCleanup';
import { UploadButton } from './UploadQueue';
import { OfficeAuthorButton } from './OfficeAuthor';
import { HistoryRestores, RestoreVersionButton } from './HistoryRestores';
import { useRetention, RetentionPanel, RetentionVersionActions } from './Retention';
import FileComparisonPanel from './FileComparison';
import { ScopeTransfers } from './ScopeTransfers';
import { BatchFileList, FileBatchHistory } from './FileBatches';
import { checkedReplicaRetry, replicaRetryInput } from '@copypaste/contracts/file-replica';

type Directory = Schemas['Folder'];
type StoredFile = Schemas['File'];
type FileVersion = Schemas['FileVersion'];
const folderURL = (id?: string | null) => id ? '/files/folders/' + id : '/files';
const stateLabels: Record<string, string> = { pending: '待扫描', clean: '扫描通过', blocked: '已阻断', unscannable: '无法扫描', waived: '经授权放行', available: '主端可用', quarantined: '已隔离', missing: '缺失', queued: '等待复制', copying: '复制中', verifying: '校验中', verified: '已校验', retry_wait: '等待重试', failed: '复制失败' };
function useCommandKey() {
  const last = useRef({ body: '', key: '' });
  return (body: unknown) => {
    const content = JSON.stringify(body);
    if (last.current.body !== content) last.current = { body: content, key: crypto.randomUUID() };
    return last.current.key;
  };
}
function LoadMore({ count, more, busy, load }: { count: number; more: boolean; busy: boolean; load: () => void }) {
  return more ? <div className="list-footer"><span>已显示 {count} 项</span><button className="button secondary" disabled={busy} onClick={load}>加载更多</button></div> : null;
}

export function FilesPage({ me }: { me: Me }) {
  const { folderID = '' } = useParams(); const [params] = useSearchParams();
  const projectID = params.get('project_id') ?? '';
  const [search, setSearch] = useState(''), [state, setState] = useState('active'), [creating, setCreating] = useState(false), [moving, setMoving] = useState<Directory | null>(null);
  const folder = useQuery({ queryKey: ['folder', folderID], enabled: !!folderID, queryFn: ({ signal }) => api<Directory>('/folders/' + folderID, { signal }) });
  const directories = useInfiniteQuery({
    queryKey: ['folders', folderID, projectID, search], initialPageParam: '', enabled: !folderID || !!folder.data,
    queryFn: ({ pageParam, signal }) => api<Page<Directory>>('/folders?' + new URLSearchParams({ ...(folderID ? { parent_id: folderID } : {}), ...(projectID ? { project_id: projectID } : {}), q: search, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const files = useInfiniteQuery({
    queryKey: ['files', folderID, projectID, search, state], initialPageParam: '', enabled: !folderID || !!folder.data,
    queryFn: ({ pageParam, signal }) => api<Page<StoredFile>>('/files?' + new URLSearchParams({ ...(folderID ? { folder_id: folderID } : {}), ...(projectID ? { project_id: projectID } : {}), q: search, state, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  if (folderID && folder.isPending) return <Busy label="正在读取资料目录" />;
  if (folderID && !folder.data) return <><Link className="back-link" to="/files"><ArrowLeft size={16} />返回文件中心</Link><ErrorNotice error={folder.error} /></>;
  const folders = directories.data?.pages.flatMap(page => page.items) ?? [], items = files.data?.pages.flatMap(page => page.items) ?? [];
  return <><Link className="back-link" to={folderURL(folder.data?.parent_id)}><ArrowLeft size={16} />{folderID ? folder.data?.parent_id ? '返回上级目录' : '返回文件中心' : '文件中心'}</Link>
    <div className="page-heading"><div><span className="eyebrow">EVERY FILE HAS ITS PLACE</span><h1>{folder.data?.name ?? '文件与资料'}<span className="accent">.</span></h1><p>{folder.data ? classificationNames[folder.data.classification] + ' · 按权限管理的资料目录' : '整理公司与项目资料，追踪版本和备份进度。'}</p></div>
      <div className="post-actions"><UploadButton folder={folder.data} /><OfficeAuthorButton folder={folder.data} />{((folder.data?.can_create) || (!folderID && !projectID && me.actions.includes('folder.create'))) && <button className="button primary" onClick={() => setCreating(true)}><FolderPlus size={17} />新建目录</button>}</div></div>
    {folder.data?.project_id && <p className="filter-caption"><Link className="text-button" to={'/projects/' + folder.data.project_id}>查看关联项目<ArrowUpRight size={15} /></Link></p>}
    {folder.data && me.actions.includes('file.retention.manage') && <MultipartCleanup key={folderID} folder={folder.data} me={me} />}
    {folder.data && !folder.error && <ScopeTransfers kind="folder" resource={folder.data} me={me} />}
    <div className="toolbar"><form className="search-field" onSubmit={event => { event.preventDefault(); setSearch(String(new FormData(event.currentTarget).get('q') ?? '').trim()); }}><Search size={18} /><input name="q" maxLength={200} aria-label="搜索文件与目录" placeholder="搜索当前范围的名称" /><button type="submit" aria-label="搜索"><ArrowRight size={17} /></button></form><select aria-label="文件状态" value={state} onChange={event => setState(event.target.value)}><option value="active">当前文件</option><option value="trashed">回收站</option></select></div>
    <ErrorNotice error={folder.error ?? directories.error ?? files.error} />
    {state === 'active' && <section className="section-block"><div className="section-heading"><h2>资料目录</h2><small className="muted">仅显示获授权的目录</small></div>{directories.isPending ? <Busy label="正在读取目录" /> : folders.length ? <div className="directory-grid">{folders.map(item => <div className="panel directory-card" key={item.id}><Link to={folderURL(item.id)}><Folder size={24} /><div><strong>{item.name}</strong><small>{classificationNames[item.classification]}</small></div><ArrowUpRight size={17} /></Link>{item.can_update && <button className="icon-button" aria-label={'修改目录 ' + item.name} onClick={() => setMoving(item)}><Pencil size={16} /></button>}</div>)}</div> : !directories.error && <div className="panel"><Empty title="当前没有可见的子目录" detail={folderID ? '可在此整理项目资料。' : '项目创建时会生成资料根目录，也可以新建公司资料目录。'} /></div>}<LoadMore count={folders.length} more={!!directories.hasNextPage} busy={directories.isFetchingNextPage} load={() => void directories.fetchNextPage()} /></section>}
    <section className="section-block"><div className="section-heading"><h2>{state === 'trashed' ? '回收站文件' : folderID ? '当前目录文件' : '最近更新的文件'}</h2></div>{files.isPending ? <Busy label="正在读取文件" /> : !files.error ? <BatchFileList key={[folderID, projectID, search, state, me.user.id, me.authorization_version].join(':')} items={items} me={me} /> : !files.error && <div className="panel"><Empty title={state === 'trashed' ? '当前范围没有回收站文件' : '当前范围没有文件'} detail="获授权且符合筛选条件的文件会在这里显示。" /></div>}<LoadMore count={items.length} more={!!files.hasNextPage} busy={files.isFetchingNextPage} load={() => void files.fetchNextPage()} /></section>
    {creating && <CreateDirectory parent={folder.data} onClose={() => setCreating(false)} />}
    {moving && <MoveResource kind="folder" resource={moving} onClose={() => setMoving(null)} />}
  </>;
}

function CreateDirectory({ parent, onClose }: { parent?: Directory; onClose: () => void }) {
  const cache = useQueryClient(), key = useCommandKey();
  const [name, setName] = useState('');
  const mutation = useMutation({
    mutationFn: () => { const body = { name, classification: parent?.classification ?? 'internal', ...(parent ? { parent_id: parent.id } : {}) }; return api('/folders', { method: 'POST', body, key: key(body) }); },
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: ['folders'] }); onClose(); },
  });
  return <Dialog title="新建资料目录" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><Field label="目录名称" hint="名称保留原样；大小写和全角等价名称视为重复。"><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} autoFocus /></Field><p className="muted">位置：{parent?.name ?? '公司资料根目录'} · {classificationNames[parent?.classification ?? 'internal']}</p><div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending}>创建目录</button></div></form></Dialog>;
}

function FolderNavigator({ projectID, classification, excludeID, onChoose }: { projectID: string | null; classification: string; excludeID?: string; onChoose: (folder: Directory) => void }) {
  const [path, setPath] = useState<Directory[]>([]);
  const current = path.at(-1);
  const query = useInfiniteQuery({
    queryKey: ['folder-picker', projectID, current?.id], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Directory>>('/folders?' + new URLSearchParams({ ...(current ? { parent_id: current.id } : {}), ...(projectID ? { project_id: projectID } : {}), limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const entries = query.data?.pages.flatMap(page => page.items).filter(item => item.id !== excludeID && item.project_id === projectID) ?? [];
  return <div className="folder-picker"><div className="folder-picker-heading"><strong>选择目标目录</strong>{path.length > 0 && <button className="text-button" type="button" onClick={() => setPath(value => value.slice(0, -1))}><ArrowLeft size={15} />上一级</button>}</div><p className="muted">{path.map(item => item.name).join(' / ') || '资料根目录'}</p><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取可用目录" /> : entries.map(item => <div className="folder-option" key={item.id}><button className="text-button" type="button" onClick={() => setPath(value => [...value, item])}><Folder size={17} /><span>{item.name}</span></button><button className="text-button" type="button" disabled={!item.can_create || item.classification !== classification} aria-label={'选用目录 ' + item.name} onClick={() => onChoose(item)}>选用</button></div>)}{!query.isPending && !entries.length && <p className="muted">没有可见的子目录。</p>}<LoadMore count={entries.length} more={!!query.hasNextPage} busy={query.isFetchingNextPage} load={() => void query.fetchNextPage()} /></div>;
}

function MoveResource({ kind, resource, onClose }: { kind: 'file' | 'folder'; resource: StoredFile | Directory; onClose: () => void }) {
  const [base] = useState(resource), [name, setName] = useState(resource.name), [reason, setReason] = useState(''), [target, setTarget] = useState<Directory | null>(null), [choosing, setChoosing] = useState(false);
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({
    mutationFn: () => { const body = { expected_version: base.version, new_name: name, reason, ...(target ? { target_folder_id: target.id } : {}) }; return api('/' + (kind === 'folder' ? 'folders' : 'files') + '/' + base.id + '/move', { method: 'POST', body, key: key(body) }); },
    onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['folders'] }), cache.invalidateQueries({ queryKey: ['files'] }), cache.invalidateQueries({ queryKey: [kind, base.id] }), cache.invalidateQueries({ queryKey: ['folder-picker'] })]); onClose(); },
    onError: error => { if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') void cache.invalidateQueries({ queryKey: [kind, base.id] }); },
  });
  const conflict = mutation.error instanceof ApiError && mutation.error.code === 'VERSION_CONFLICT';
  const changed = name !== base.name || !!target;
  return <Dialog title={kind === 'folder' ? '修改资料目录' : '修改文件位置与名称'} onClose={onClose} busy={mutation.isPending}><form onSubmit={(event: FormEvent) => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">请关闭此窗口，比较最新资料后重新发起修改。当前填写内容保留供核对。</p>}<Field label={kind === 'folder' ? '目录名称' : '文件名称'}><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} /></Field><div className="field"><span>目标目录</span><p>{target?.name ?? '保留当前位置'}</p><button type="button" className="text-button" onClick={() => setChoosing(value => !value)}>{choosing ? '收起目录选择' : '选择其他目录'}<ArrowRight size={15} /></button>{target && <button type="button" className="text-button" onClick={() => setTarget(null)}>保留原位置</button>}</div>{choosing && <FolderNavigator projectID={base.project_id} classification={base.classification} excludeID={kind === 'folder' ? base.id : undefined} onChoose={value => { setTarget(value); setChoosing(false); }} />}<p className="muted">移动限于相同项目和分类；文件内容及历史版本保持可追踪。</p><Field label="变更原因"><textarea required maxLength={200} rows={2} value={reason} onChange={event => setReason(event.target.value)} /></Field><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending || conflict || !changed}>保存变更</button></div></form></Dialog>;
}

export function FileDetail() {
  const { id = '' } = useParams(); const [moving, setMoving] = useState(false), [action, setAction] = useState<'trash' | 'restore' | null>(null);
  const file = useQuery({ queryKey: ['file', id], queryFn: ({ signal }) => api<StoredFile>('/files/' + id, { signal }) });
  const me = useQuery({ queryKey: ['me'], queryFn: ({ signal }) => api<Me>('/me', { signal }) });
  const retention = useRetention(id, !!file.data);
  const history = useInfiniteQuery({ queryKey: ['file-versions', id], initialPageParam: '', enabled: !!file.data, queryFn: ({ pageParam, signal }) => api<Page<FileVersion>>('/files/' + id + '/versions?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined,
    refetchInterval: query => query.state.data?.pages.some(page => page.items.some(version => version.replicas.some(replica => ['queued', 'copying', 'verifying', 'retry_wait'].includes(replica.state)))) ? 5000 : false });
  if (file.isPending) return <Busy label="正在读取文件资料" />;
  if (!file.data) return <><Link className="back-link" to="/files"><ArrowLeft size={16} />返回文件中心</Link><ErrorNotice error={file.error} /></>;
  const value = file.data, versions = history.data?.pages.flatMap(page => page.items) ?? [];
  return <><Link className="back-link" to={folderURL(value.folder_id)}><ArrowLeft size={16} />返回所在目录</Link><ErrorNotice error={file.error} /><div className="page-heading detail-heading"><div><span className="eyebrow">A TRACEABLE RECORD</span><h1>{value.name}</h1><div className="project-subtitle"><span className="badge neutral">{value.state === 'trashed' ? '回收站' : '当前文件'}</span><span>{classificationNames[value.classification]}</span><span>资料版本 {value.version} · {date(value.updated_at)}</span></div></div><div className="post-actions"><UploadButton file={value} /><OfficeAuthorButton file={value} />{value.can_move && <button className="button secondary" onClick={() => setMoving(true)}><Pencil size={16} />修改位置与名称</button>}{value.can_trash && <button className="button danger-outline" onClick={() => setAction('trash')}><Trash2 size={16} />移入回收站</button>}{value.can_restore && <button className="button primary" onClick={() => setAction('restore')}><RotateCcw size={16} />恢复至原目录</button>}</div></div>
    {value.legal_hold && <div className="notice neutral"><ShieldCheck size={18} /><span>此文件处于保留约束中。</span></div>}
    {me.data && !me.error && !file.error && <ScopeTransfers kind="file" resource={value} me={me.data} />}
    {me.data && !me.error && !file.error && <FileBatchHistory key={value.id + me.data.authorization_version} fileID={value.id} me={me.data} />}
    <section className="section-block"><div className="section-heading"><h2>内容版本与存储记录</h2><small className="muted">原文件与来源信息按版本保留</small></div><ErrorNotice error={history.error} />{history.isPending ? <Busy label="正在读取文件版本" /> : versions.length ? <div className="file-versions">{versions.map(version => <VersionCard key={version.id} version={version} current={version.id === value.current_version_id} canRetry={value.can_update} active={value.state === 'active'} file={value} canManageRetention={!!retention.data?.can_manage && !retention.error} />)}</div> : !history.error && <div className="panel"><Empty title="尚无正式内容版本" detail="完成上传和服务端校验的版本才会出现在这里。" /></div>}<LoadMore count={versions.length} more={!!history.hasNextPage} busy={history.isFetchingNextPage} load={() => void history.fetchNextPage()} /></section>
    <FileComparisonPanel key={value.id + ':' + value.version} file={value} versions={versions} more={!!history.hasNextPage} loading={history.isFetchingNextPage} loadMore={() => void history.fetchNextPage()} unavailable={!!file.error || !!history.error} />
    <HistoryRestores file={value} />
    <RetentionPanel file={value} overview={retention.data} loading={retention.isPending} error={retention.error} />
    {moving && <MoveResource kind="file" resource={value} onClose={() => setMoving(false)} />}{action && <FileStateAction file={value} action={action} onClose={() => setAction(null)} />}
  </>;
}

function VersionCard({ version, current, canRetry, active, file, canManageRetention }: { version: FileVersion; current: boolean; canRetry: boolean; active: boolean; file: StoredFile; canManageRetention: boolean }) {
  const [retry, setRetry] = useState<Schemas['FileReplica'] | null>(null);
  const [download, setDownload] = useState(false);
  const downloadReason = !active ? '先从回收站恢复，才能下载此版本。' : version.availability !== 'available' || !['clean', 'waived'].includes(version.scan_state) ? '此版本尚未具备可用内容与扫描或放行条件。' : !version.replicas.some(replica => replica.state === 'verified') ? '暂缺具有完整校验证据的下载副本。' : '';
  const canDownload = active && version.availability === 'available' && ['clean', 'waived'].includes(version.scan_state) && version.replicas.some(replica => replica.state === 'verified');
  return <article className="panel version-card"><div className="section-heading"><h3>内容版本 {version.seq}{current && <span className="badge neutral">当前正式版</span>}</h3><small className="muted">{date(version.created_at)}</small></div><div className="version-actions"><RestoreVersionButton file={file} version={version} /><RetentionVersionActions file={file} version={version} canManage={canManageRetention} /><button className="button secondary" onClick={() => setDownload(true)} disabled={!canDownload} title={downloadReason || undefined}><Download size={16} />下载此内容版本</button>{downloadReason && <small className="muted">{downloadReason}</small>}</div><dl className="file-facts"><div><dt>原文件名</dt><dd>{version.original_name ?? '历史记录未提供'}</dd></div><div><dt>大小</dt><dd>{BigInt(version.size_bytes).toLocaleString('zh-HK')} 字节</dd></div><div><dt>媒体类型</dt><dd>{version.media_type}</dd></div><div><dt>扫描与主端</dt><dd>{stateLabels[version.scan_state]} · {stateLabels[version.availability]}</dd></div><div className="wide"><dt>SHA-256</dt><dd><code>{version.sha256}</code></dd></div><div><dt>来源</dt><dd>{({ upload: "上传", restore: "历史恢复" } as Record<string,string>)[version.source] ?? version.source}</dd></div><div><dt>双云状态</dt><dd>{version.replica_state ? stateLabels[version.replica_state] : '尚无副本记录'}</dd></div></dl><div className="replica-grid">{(['oss', 'r2'] as const).map(provider => {
    const replicas = version.replicas.filter(item => item.provider === provider);
    return <div className="replica-status" key={provider}><strong>{provider === 'oss' ? '阿里云 OSS' : 'Cloudflare R2'}</strong>{replicas.length ? replicas.map(replica => <div key={replica.id}><span className={'badge ' + (replica.state === 'verified' ? 'success' : ['failed', 'missing'].includes(replica.state) ? 'warning' : 'neutral')}>{stateLabels[replica.state]}</span><small>校验时间：{date(replica.verified_at)}</small>{replica.error_code && <small>状态代码：{replica.error_code}</small>}{canRetry && ['failed', 'missing'].includes(replica.state) && <button className="text-button" onClick={() => setRetry(replica)}>重新安排副本复制<ArrowRight size={15} /></button>}</div>) : <small>尚无副本记录</small>}</div>;
  })}</div>{version.scan_state === 'waived' && <p className="notice warning">此版本经扫描例外审核放行，尚未取得完整安全扫描通过的结论。</p>}{download && <DownloadDialog version={version} onClose={() => setDownload(false)} />}{retry && <ReplicaRetryDialog replica={retry} fileID={version.file_id} onClose={() => setRetry(null)} />}</article>;
}

function ReplicaRetryDialog({ replica, fileID, onClose }: { replica: Schemas['FileReplica']; fileID: string; onClose: () => void }) {
  const [reason, setReason] = useState(''); const [base] = useState(replica);
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({
    mutationFn: async () => { const body = replicaRetryInput(base, reason); return checkedReplicaRetry(await api<Schemas['FileReplica']>('/file-replicas/' + base.id + '/retry', { method: 'POST', body, key: key(body) }), base); },
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: ['file-versions', fileID] }); onClose(); },
  });
  const conflict = mutation.error instanceof ApiError && mutation.error.code === 'VERSION_CONFLICT';
  return <Dialog title="重新安排副本复制" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><p className="muted">重新复制此内容版本，并校验目标完整摘要。排队成功后仍需等待复制完成。</p>{conflict && <p className="notice neutral">副本状态已改变，请关闭此窗口并刷新存储记录后核对。</p>}<Field label="重试原因"><textarea required maxLength={200} rows={3} value={reason} onChange={event => setReason(event.target.value)} autoFocus /></Field><div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending || conflict}>安排复制</button></div></form></Dialog>;
}

function FileStateAction({ file, action, onClose }: { file: StoredFile; action: 'trash' | 'restore'; onClose: () => void }) {
  const [reason, setReason] = useState(''); const [base] = useState(file);
  const cache = useQueryClient(), key = useCommandKey();
  const title = action === 'trash' ? '移入回收站' : '恢复至原目录';
  const mutation = useMutation({
    mutationFn: () => { const body = { expected_version: base.version, reason, ...(action === 'trash' ? { action: 'trash' } : { mode: 'untrash' }) }; return api('/files/' + base.id + '/' + action, { method: 'POST', body, key: key(body) }); },
    onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['file', base.id] }), cache.invalidateQueries({ queryKey: ['files'] })]); onClose(); },
  });
  return <Dialog title={title} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><p className="muted">{action === 'trash' ? '文件将从当前列表移至回收站，内容和历史版本继续保留。' : '恢复原名称和原目录；如名称已占用，先处理冲突再重试。'}</p><Field label="操作原因"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} autoFocus /></Field><div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className={'button ' + (action === 'trash' ? 'danger' : 'primary')} disabled={mutation.isPending}>{title}</button></div></form></Dialog>;
}
