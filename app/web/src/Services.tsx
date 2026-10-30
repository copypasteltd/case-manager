import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity, ArrowLeft, ArrowUpRight, Plus, RefreshCw, Search } from 'lucide-react';
import { api, type Me, type Page, type Schemas } from './api';
import { ProjectScope } from './BusinessScope';
import { Busy, classificationNames, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';
import { isStatusConflict, sourceReasonNames, StatusBadge, StatusConflict, StatusNavigation, StatusOwner, useStatusClock, useStatusKey } from './StatusShared';
import { stampStatus, statusIsCurrent, statusNames, statusTime, statusTone, type StatusSnapshot } from './statusFreshness';
import { ServiceHistory } from './StatusHistory';
import { HostResources } from './HostResources';

type Service = Schemas['Service'];
type Source = Schemas['ServiceSource'];
async function readService(id: string, signal?: AbortSignal): Promise<StatusSnapshot> {
  const started = performance.now();
  return stampStatus(await api<Service>('/services/' + id, { signal }), started);
}
async function refresh(cache: ReturnType<typeof useQueryClient>, id: string) {
  await Promise.all([cache.invalidateQueries({ queryKey: ['services'] }), cache.invalidateQueries({ queryKey: ['service', id] }), cache.invalidateQueries({ queryKey: ['service-history', id] })]);
}
export function ServicesPage({ me }: { me: Me }) {
  const [search, setSearch] = useState(''), [q, setQuery] = useState(''), [lifecycle, setLifecycle] = useState('active'), [create, setCreate] = useState(false);
  const now = useStatusClock();
  const query = useInfiniteQuery({ queryKey: ['services', q, lifecycle], initialPageParam: '', enabled: me.actions.includes('service.read'), refetchInterval: 5000,
    queryFn: async ({ pageParam, signal }) => {
      const started = performance.now();
      const page = await api<Page<Service>>('/services?' + new URLSearchParams({ q, lifecycle, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal });
      return { ...page, items: page.items.map(value => stampStatus(value, started)) };
    }, getNextPageParam: page => page.next_cursor ?? undefined });
  const entries = query.error ? [] : [...new Map(query.data?.pages.flatMap(page => page.items).map(item => [item.value.id, item])).values()];
  if (!me.actions.includes('service.read')) return <Empty title="没有服务状态权限" detail="请联系负责人核对服务范围和授权。" />;
  return <><div className="page-heading"><div><span className="eyebrow">OPERATIONS / SERVICE HEALTH</span><h1>服务状态</h1><p>依据有效观测判断状态，保留每一步处置记录。</p></div><div className="heading-actions"><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}><RefreshCw size={16} />刷新</button>{me.actions.includes('service.manage') && <button className="button primary" onClick={() => setCreate(true)}><Plus size={17} />登记服务</button>}</div></div><StatusNavigation />
    <form className="party-filters" onSubmit={event => { event.preventDefault(); setQuery(search.trim()); }}><Field label="搜索服务"><input type="search" maxLength={200} value={search} onChange={event => setSearch(event.target.value)} /></Field><button className="button secondary"><Search size={16} />搜索</button><Field label="目录状态"><select value={lifecycle} onChange={event => setLifecycle(event.target.value)}><option value="active">使用中</option><option value="archived">已归档</option><option value="">全部</option></select></Field></form>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取服务状态" /> : !query.error && !entries.length ? <Empty title="尚无符合条件的服务" detail="登记服务并配置可信监控来源后，才会产生健康状态。" /> : !!entries.length && <div className="panel party-list">{entries.map(snapshot => { const item = snapshot.value; return <Link className="party-row status-service-row" key={item.id} to={'/services/' + item.id}><Activity size={21} /><div><h2>{item.name}</h2><p>{item.host_label || '尚未填写主机名称'} · {classificationNames[item.classification]} · {item.sources.length} 个观测来源</p><small className="muted">评估于 {statusTime(item.evaluated_at)} · 香港时间</small></div><StatusBadge snapshot={snapshot} now={now} /><ArrowUpRight size={18} /></Link>; })}</div>}
    {query.hasNextPage && !query.error && <div className="list-footer"><span>已显示 {entries.length} 项</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多服务</button></div>}
    <p className="form-note">页面自动重读状态；结果到期后显示“待重新确认”。外部探测正常仅说明对应服务可用性。</p>{create && <ServiceEditor me={me} onClose={() => setCreate(false)} />}
  </>;
}
export function ServiceDetail({ me }: { me: Me }) {
  const { id = '' } = useParams(), [edit, setEdit] = useState(false), [source, setSource] = useState<Source | 'new' | null>(null), [maintenance, setMaintenance] = useState<Schemas['MaintenanceWindow'] | 'new' | null>(null);
  const [history, setHistory] = useState<'observations' | 'maintenance' | Source | null>(null), now = useStatusClock();
  const query = useQuery({ queryKey: ['service', id], queryFn: ({ signal }) => readService(id, signal), refetchInterval: 5000 });
  const snapshot = query.error ? undefined : query.data, value = snapshot?.value;
  if (!snapshot || !value) return <><Link className="back-link" to="/services"><ArrowLeft size={16} />服务目录</Link><ErrorNotice error={query.error} />{query.isPending && <Busy label="正在读取服务资料" />}</>;
  const current = statusIsCurrent(snapshot, now);
  return <><Link className="back-link" to="/services"><ArrowLeft size={16} />服务目录</Link><div className="page-heading"><div><span className="eyebrow">SERVICE / CURRENT EVIDENCE</span><h1>{value.name}</h1><p>{value.description || '尚未补充服务说明'}</p></div><div className="heading-actions"><StatusBadge snapshot={snapshot} now={now} /><button className="button secondary" disabled={query.isFetching} onClick={() => void query.refetch()}>刷新状态</button>{value.can_manage && <button className="button primary" onClick={() => setEdit(true)}>编辑服务</button>}</div></div>
    {!current && <p className="notice warning" role="status">上次评估已到重读时刻，当前状态待重新确认。下方保留的是上次取得的证据。</p>}
    <section className="panel party-detail"><dl className="file-facts"><div><dt>目录状态</dt><dd>{value.lifecycle === 'active' ? '使用中' : '已归档'}</dd></div><div><dt>主机名称</dt><dd>{value.host_label || '未填写'}</dd></div><div><dt>端点名称</dt><dd>{value.endpoint_label || '未填写'}</dd></div><div><dt>资料分类</dt><dd>{classificationNames[value.classification]}</dd></div><div><dt>评估时间（香港）</dt><dd>{statusTime(value.evaluated_at)}</dd></div><div><dt>最迟重读（香港）</dt><dd>{statusTime(value.recheck_at)}</dd></div><div><dt>配置版本</dt><dd>第 {value.version} 版</dd></div><div><dt>未覆盖观测状态</dt><dd>{statusNames[value.observed_state]}（上次评估）</dd></div></dl>{value.project_id && <Link className="text-button" to={'/projects/' + value.project_id}>查看所属项目 <ArrowUpRight size={16} /></Link>}</section>
    <section className="section-block status-section"><div className="section-heading"><div><span className="eyebrow">MONITORING EVIDENCE</span><h2>观测来源</h2></div>{value.can_manage && value.lifecycle === 'active' && <button className="button secondary" onClick={() => setSource('new')}><Plus size={16} />登记来源</button>}</div>{!value.sources.length ? <div className="panel"><Empty title="尚无监控数据" detail="来源登记不会自动安装监控。请由可信采集适配器发送签名观测。" /></div> : <div className="status-source-grid">{value.sources.map(item => <article className="panel status-source" key={item.id}><div className="section-heading compact"><h3>{item.name}</h3><span className={'badge ' + (current ? statusTone[item.state] : 'neutral')}>{current ? statusNames[item.state] : '待重新确认'}</span></div><p className="muted">{item.enabled ? '已启用' : '已停用'} · 来源第 {item.version} 版 · {item.provider}</p>{item.state_reason && <p>{sourceReasonNames[item.state_reason] ?? '状态需核对'}</p>}<dl className="file-facts"><div><dt>观测于（香港）</dt><dd>{statusTime(item.projection?.observed_at)}</dd></div><div><dt>接收于（香港）</dt><dd>{statusTime(item.projection?.received_at)}</dd></div><div><dt>连续确认</dt><dd>{item.failure_count} 次失败 / {item.recovery_count} 次恢复</dd></div><div><dt>新鲜度</dt><dd>{item.stale_after_seconds} 秒</dd></div></dl><div className="heading-actions"><button className="text-button" onClick={() => setHistory(item)}>查看修订</button>{value.can_manage && value.lifecycle === 'active' && <button className="text-button" onClick={() => setSource(item)}>发布来源修订</button>}</div></article>)}</div>}</section>
    <section className="section-block status-section"><div className="section-heading"><h2>观测与维护记录</h2><div className="heading-actions"><button className="button secondary" onClick={() => setHistory('observations')}>查看观测事实</button><button className="button secondary" onClick={() => setHistory('maintenance')}>查看维护记录</button>{value.can_manage && value.lifecycle === 'active' && <button className="button primary" onClick={() => setMaintenance('new')}>安排维护</button>}</div></div><p className="muted">维护仅覆盖展示状态，原始观测继续保存。窗口结束后按当前有效证据重新判断。</p></section>
    <HostResources key={id + ':' + me.user.id + ':' + me.authorization_version} service={id} me={me} />
    {edit && <ServiceEditor me={me} service={value} onClose={() => setEdit(false)} />}
    {source && <SourceEditor service={value} source={source === 'new' ? undefined : source} onClose={() => setSource(null)} />}
    {maintenance && <MaintenanceEditor service={value} window={maintenance === 'new' ? undefined : maintenance} onClose={() => setMaintenance(null)} />}
    {history && <ServiceHistory service={value} kind={history} onClose={() => setHistory(null)} onCancelMaintenance={window => { setHistory(null); setMaintenance(window); }} />}
  </>;
}
function ServiceEditor({ me, service, onClose }: { me: Me; service?: Service; onClose: () => void }) {
  const [base] = useState(service), [name, setName] = useState(service?.name ?? ''), [owner, setOwner] = useState(service?.owner_id ?? me.user.id);
  const [host, setHost] = useState(service?.host_label ?? ''), [endpoint, setEndpoint] = useState(service?.endpoint_label ?? ''), [description, setDescription] = useState(service?.description ?? ''), [lifecycle, setLifecycle] = useState<Service['lifecycle']>(service?.lifecycle ?? 'active');
  const [project, setProject] = useState(''), [classification, setClassification] = useState('internal'), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const key = useStatusKey(), cache = useQueryClient(), navigate = useNavigate();
  const mutation = useMutation({ mutationFn: () => {
    const content = { name, owner_id: owner, host_label: host, endpoint_label: endpoint, description };
    const body: Schemas['ServiceCreate'] | Schemas['ServicePatch'] = base ? { ...content, lifecycle, expected_version: base.version, reason } : { ...content, project_id: project || null, classification: classification as Service['classification'], reason };
    return api<Service>('/services' + (base ? '/' + base.id : ''), { method: base ? 'PATCH' : 'POST', body, key: key(body) });
  }, onSuccess: async value => { await refresh(cache, value.id); onClose(); if (!base) navigate('/services/' + value.id); } });
  return <Dialog title={base ? '编辑服务资料' : '登记服务'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><StatusConflict error={mutation.error} />
    <Field label="服务名称"><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} /></Field>
    {!base && me.actions.includes('project.read') && <ProjectScope project={project} setProject={value => { setProject(value); setOwner(me.user.id); }} classification={classification} setClassification={setClassification} />}
    <StatusOwner me={me} project={base?.project_id ?? project} owner={owner} setOwner={setOwner} currentOwner={base?.owner_id} />
    <Field label="主机展示名称"><input maxLength={200} value={host} onChange={event => setHost(event.target.value)} /></Field><Field label="端点展示名称" hint="仅作展示；实际探测由可信监控配置管理。"><input maxLength={500} value={endpoint} onChange={event => setEndpoint(event.target.value)} /></Field>
    <Field label="服务说明"><textarea rows={4} maxLength={10000} value={description} onChange={event => setDescription(event.target.value)} /></Field>
    {base && <Field label="目录状态"><select value={lifecycle} onChange={event => setLifecycle(event.target.value as Service['lifecycle'])}><option value="active">使用中</option><option value="archived">归档并停止接收观测</option></select></Field>}
    <Field label={base ? '变更原因' : '登记原因'}><textarea required maxLength={200} rows={2} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} />
    {base && <p className="form-note">本次基于配置第 {base.version} 版。项目、分类与创建人保持该服务的固定范围。</p>}
    <div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || mutation.isPending || isStatusConflict(mutation.error)}>{base ? '确认保存服务' : '确认登记服务'}</button></div></form></Dialog>;
}
function SourceEditor({ service, source, onClose }: { service: Service; source?: Source; onClose: () => void }) {
  const [base] = useState(service), [original] = useState(source), [name, setName] = useState(source?.name ?? ''), [provider, setProvider] = useState<Source['provider']>(source?.provider ?? 'adapter'), [external, setExternal] = useState(source?.external_ref ?? ''), [publicKey, setPublicKey] = useState(source?.public_key ?? ''), [enabled, setEnabled] = useState(source?.enabled ?? true);
  const [failure, setFailure] = useState(source?.failure_count ?? 3), [recovery, setRecovery] = useState(source?.recovery_count ?? 2), [interval, setIntervalValue] = useState(source?.interval_seconds ?? 60), [stale, setStale] = useState(source?.stale_after_seconds ?? 180), [latency, setLatency] = useState(source?.latency_limit_ms ?? 2000), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const key = useStatusKey(), cache = useQueryClient();
  const mutation = useMutation({ mutationFn: () => {
    const body: Schemas['ServiceSourceInput'] = { name, provider, external_ref: external, public_key: publicKey.trim(), enabled, failure_count: failure, recovery_count: recovery, interval_seconds: interval, stale_after_seconds: stale, latency_limit_ms: latency, expected_version: base.version, expected_source_version: original?.version ?? 0, base_revision_id: original?.revision_id ?? null, reason };
    return api<Service>('/services/' + base.id + '/sources' + (original ? '/' + original.id + '/revisions' : ''), { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refresh(cache, base.id); onClose(); } });
  return <Dialog title={source ? '发布来源修订' : '登记观测来源'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><StatusConflict error={mutation.error} /><p><strong>{base.name}</strong> · 配置第 {base.version} 版</p><p className="form-note">发布后从“状态未知”重新积累观测。来源类型仅为登记，不会自动接通采集平台。</p>
    <Field label="来源名称"><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} /></Field><Field label="来源类型"><select value={provider} onChange={event => setProvider(event.target.value as Source['provider'])}><option value="adapter">可信采集适配器</option><option value="uptime_kuma">Uptime Kuma（须另接适配器）</option><option value="prometheus">Prometheus（须另接适配器）</option></select></Field>
    <Field label="外部来源参考" hint="填写可公开的监控编号；不要粘贴密钥或签名链接。"><input required maxLength={200} value={external} onChange={event => setExternal(event.target.value)} /></Field><Field label="Ed25519 公钥（Base64）" hint="只填写 32 字节公钥的标准 Base64；私钥由独立采集端保管。"><textarea required rows={2} maxLength={44} value={publicKey} onChange={event => setPublicKey(event.target.value)} /></Field>
    <div className="form-grid"><Field label="连续失败次数"><input required type="number" min={1} max={10} step={1} value={failure} onChange={event => setFailure(Number(event.target.value))} /></Field><Field label="连续恢复次数"><input required type="number" min={1} max={10} step={1} value={recovery} onChange={event => setRecovery(Number(event.target.value))} /></Field><Field label="探测间隔（秒）"><input required type="number" min={15} max={60} step={1} value={interval} onChange={event => setIntervalValue(Number(event.target.value))} /></Field><Field label="过期时长（秒）"><input required type="number" min={interval * 3} max={180} step={1} value={stale} onChange={event => setStale(Number(event.target.value))} /></Field><Field label="时延阈值（毫秒）"><input required type="number" min={1} max={120000} step={1} value={latency} onChange={event => setLatency(Number(event.target.value))} /></Field></div>
    <label className="checkbox-row"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用此来源</label><Field label="发布原因"><textarea required rows={2} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || mutation.isPending || isStatusConflict(mutation.error)}>确认发布来源</button></div></form></Dialog>;
}
function MaintenanceEditor({ service, window, onClose }: { service: Service; window?: Schemas['MaintenanceWindow']; onClose: () => void }) {
  const [base] = useState(service), [original] = useState(window), [start, setStart] = useState(''), [end, setEnd] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const key = useStatusKey(), cache = useQueryClient();
  const mutation = useMutation({ mutationFn: () => {
    const body: Schemas['MaintenanceInput'] | Schemas['MaintenanceCancel'] = original ? { expected_version: base.version, reason } : { expected_version: base.version, start_at: new Date(start + '+08:00').toISOString(), end_at: new Date(end + '+08:00').toISOString(), reason };
    return api<Service>('/services/' + base.id + '/maintenance-windows' + (original ? '/' + original.id + '/cancel' : ''), { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refresh(cache, base.id); onClose(); } });
  return <Dialog title={original ? '取消维护窗口' : '安排维护窗口'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} /><StatusConflict error={mutation.error} /><p><strong>{base.name}</strong> · 配置第 {base.version} 版</p>{original ? <p>{statusTime(original.start_at)} — {statusTime(original.end_at)}（香港时间）。取消将追加记录并保留原窗口。</p> : <><p className="form-note">按香港时间填写，最长 31 天。维护期间仍保留原始观测。</p><Field label="开始时间（香港 UTC+8）"><input required type="datetime-local" value={start} onChange={event => setStart(event.target.value)} /></Field><Field label="结束时间（香港 UTC+8）"><input required type="datetime-local" min={start || undefined} value={end} onChange={event => setEnd(event.target.value)} /></Field></>}
    <Field label={original ? '取消原因' : '维护原因'}><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>返回</button><button className="button primary" disabled={!fresh || mutation.isPending || isStatusConflict(mutation.error)}>{original ? '确认取消维护' : '确认安排维护'}</button></div></form></Dialog>;
}
