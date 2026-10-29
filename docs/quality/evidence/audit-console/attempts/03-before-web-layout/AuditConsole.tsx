import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { auditFields, auditKey, auditPath, auditLabels as labels, auditFilterSummary, auditExportStates, checkedAuditEvent, checkedAuditExport, checkedAuditExportUpdate, checkedAuditPage, currentAuditFilter, readAuditBytes, validateAuditFilter, type AuditEvent, type AuditExport, type AuditPage, type AuditChunk } from '@copypaste/contracts/audit';
import { api, ApiError, ensureDownloadSession, type Me, type Page } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field, date } from './components';
import './financial-reports.css';

export default function AuditConsole({ me }: { me: Me }) {
  return me.actions.includes('audit.read') ? <AuditWorkspace key={me.user.id + ':' + me.authorization_version} me={me} /> : <Empty title="没有审计查询权限" detail="请由管理员明确授予组织级审计查询权限。" />;
}

function AuditWorkspace({ me }: { me: Me }) {
  const [draft, setDraft] = useState(currentAuditFilter), [applied, setApplied] = useState(currentAuditFilter);
  const [advanced, setAdvanced] = useState(false), [cursors, setCursors] = useState(['']), [historyCursors, setHistoryCursors] = useState(['']);
  const [detail, setDetail] = useState<string>(), [reason, setReason] = useState(''), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(''), [uncertain, setUncertain] = useState(false);
  const command = useRef<{ body: string; key: string }>(), lifetime = useRef(new AbortController()), flight = useRef(false);
  useEffect(() => { const current = new AbortController(); lifetime.current = current; return () => current.abort(); }, []);
  const signal = () => lifetime.current.signal;
  const path = auditPath(applied, cursors.at(-1));
  const events = useQuery({ queryKey: ['audit-events', me.user.id, me.authorization_version, path], queryFn: async ({ signal }) => checkedAuditPage(await api<AuditPage>(path, { signal }), applied) });
  const historyPath = '/audit/exports?limit=10' + (historyCursors.at(-1) ? '&cursor=' + encodeURIComponent(historyCursors.at(-1)!) : '');
  const history = useQuery({ queryKey: ['audit-exports', me.user.id, me.authorization_version, historyPath], enabled: me.actions.includes('audit.export'), queryFn: async ({ signal }) => { const p = await api<Page<AuditExport>>(historyPath, { signal }); p.items.forEach(item => checkedAuditExport(item)); return p; } });
  const dirty = JSON.stringify(draft) !== JSON.stringify(applied);
  function apply() {
    try { validateAuditFilter(draft); setApplied({ ...draft }); setCursors(['']); setDetail(undefined); setError(undefined); if (auditKey(draft) === auditKey(applied)) void events.refetch(); } catch (cause) { setError(cause); }
  }
  async function create() {
    if (flight.current || dirty || !reason.trim()) return;
    flight.current = true;
    try {
      setBusy('create'); setError(undefined);
      const body = { filters: applied, reason: reason.trim() }, encoded = JSON.stringify(body);
      if (!command.current || command.current.body !== encoded) command.current = { body: encoded, key: crypto.randomUUID() };
      checkedAuditExport(await api<AuditExport>('/audit/exports', { method: 'POST', body, key: command.current.key, signal: signal() }), applied);
      if (signal().aborted) return;
      command.current = undefined; setUncertain(false); setReason(''); setHistoryCursors(['']); await history.refetch();
    } catch (cause) { if (!signal().aborted) { setError(cause); setUncertain(!(cause instanceof ApiError) || cause.status === 0 || cause.status >= 500); } } finally { flight.current = false; if (!signal().aborted) setBusy(''); }
  }
  async function download(record: AuditExport) {
    if (flight.current) return;
    flight.current = true;
    let bytes: Uint8Array | undefined;
    const active = () => signal().throwIfAborted();
    try {
      setBusy(record.id); setError(undefined); await ensureDownloadSession(signal()); active();
      const fresh = checkedAuditExportUpdate(await api<AuditExport>('/audit/exports/' + record.id, { signal: signal() }), record);
      bytes = await readAuditBytes(fresh, (offset, length) => api<AuditChunk>('/audit/exports/' + record.id + '/chunks?offset=' + offset + '&length=' + length, { signal: signal() }), async value => Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', value.slice().buffer as ArrayBuffer)), b => b.toString(16).padStart(2, '0')).join(''), active);
      const current = await api<Me>('/me', { signal: signal() }); active();
      if (current.user.id !== me.user.id || current.authorization_version !== me.authorization_version || !current.actions.includes('audit.export')) throw new Error('权限已变化，请重新读取审计工作区。');
      const final = checkedAuditExportUpdate(await api<AuditExport>('/audit/exports/' + record.id, { signal: signal() }), fresh);
      if (!final.can_download || final.csv_sha256 !== fresh.csv_sha256) throw new Error('固定导出已失效，请重新生成。');
      active(); const url = URL.createObjectURL(new Blob([bytes.slice().buffer as ArrayBuffer], { type: 'text/csv;charset=utf-8' }));
      const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'audit-' + record.id + '.csv'; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (cause) { if (!signal().aborted) { setError(cause); void history.refetch(); } } finally { flight.current = false; bytes?.fill(0); if (!signal().aborted) setBusy(''); }
  }
  return <div className="audit-console">
    <div className="page-heading"><div><span className="eyebrow">OPERATIONS / AUDIT TRAIL</span><h1>审计记录<span className="accent">.</span></h1><p>按准确操作线索查询，保留可核对的固定导出。</p></div></div>
    <section className="panel report-sources"><h2>查询条件</h2><p className="muted">组织级操作元数据；业务正文仍按原业务权限访问。时间按香港时区，结束日期不包含在内。</p>
      <form onSubmit={event => { event.preventDefault(); apply(); }}><div className="form-grid">
        <Field label="开始日期"><input type="date" required value={draft.from.slice(0, 10)} disabled={!!busy || uncertain} onChange={e => setDraft({ ...draft, from: e.target.value + 'T00:00:00+08:00' })} /></Field>
        <Field label="结束日期（不含）"><input type="date" required value={draft.to.slice(0, 10)} disabled={!!busy || uncertain} onChange={e => setDraft({ ...draft, to: e.target.value + 'T00:00:00+08:00' })} /></Field>
        {(['action', 'result', 'channel'] as const).map(key => <Field key={key} label={labels[key]} hint="准确匹配；留空包含全部。"><input value={draft[key]} maxLength={128} disabled={!!busy || uncertain} onChange={e => setDraft({ ...draft, [key]: e.target.value.trim() })} /></Field>)}
      </div><button className="text-button" type="button" aria-expanded={advanced} onClick={() => setAdvanced(!advanced)}>{advanced ? '收起更多线索' : '按人员、资源或任务筛选'}</button>
        {advanced && <div className="form-grid">{auditFields.filter(key => !['action', 'result', 'channel'].includes(key)).map(key => <Field key={key} label={labels[key]} hint={key === 'resource_type' || key === 'request_id' ? '准确技术标识' : '完整UUID标识'}><input value={draft[key]} maxLength={128} disabled={!!busy || uncertain} onChange={e => setDraft({ ...draft, [key]: e.target.value.trim() })} /></Field>)}</div>}
        <div className="dialog-actions"><button className="button primary" disabled={!!busy || uncertain || events.isFetching}>应用查询</button><button type="button" className="button secondary" disabled={!!busy || events.isFetching} onClick={() => void events.refetch()}>刷新当前结果</button></div>
      </form>{dirty && <p className="form-note" role="status">筛选尚未应用，列表仍显示上次查询结果。</p>}
    </section><ErrorNotice error={error ?? events.error} />
    {events.isPending ? <Busy label="正在查询审计记录" /> : !events.error && <><p className="form-note">已应用：{auditFilterSummary(applied)}</p><section className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="审计操作记录"><table><thead><tr><th>时间 / 来源</th><th>动作 / 结果</th><th>操作者 / 资源</th><th>操作</th></tr></thead><tbody>{events.data?.items.map(item => <tr key={item.id}><td>{date(item.occurred_at)}<small className="binding-id">{item.channel}</small></td><td><strong>{item.action}</strong><small className="binding-id">{item.result}</small></td><td><small className="binding-id">{item.actor_id ?? '系统执行'}<br />{item.resource_type} · {item.resource_id ?? '无对象标识'}</small></td><td><button className="text-button" onClick={() => setDetail(item.id)}>查看详情</button></td></tr>)}</tbody></table></div>{events.data?.items.length === 0 && <Empty title="没有匹配的审计记录" detail="可调整时间或准确标识后重新查询。" />}</section><div className="report-pagination"><button className="button secondary" disabled={cursors.length === 1 || events.isFetching} onClick={() => setCursors(cursors.slice(0, -1))}>上一页记录</button><button className="button secondary" disabled={!events.data?.next_cursor || events.isFetching} onClick={() => setCursors([...cursors, events.data!.next_cursor!])}>下一页记录</button></div></>}
    {me.actions.includes('audit.export') && <section className="panel report-sources"><h2>固定导出</h2><p className="muted">按已应用条件固定全部匹配事件，最多10,000条、8 MiB，有效24小时。超出范围会明确拒绝；导出与下载需要近期两步验证。CSV只含操作元数据，建议按文本导入。</p>
      <Field label="导出用途"><input required maxLength={200} value={reason} disabled={!!busy || uncertain} onChange={e => setReason(e.target.value)} /></Field>
      <div className="dialog-actions"><button className="button primary" disabled={!!busy || dirty || !reason.trim()} onClick={() => void create()}>{busy === 'create' ? '正在生成…' : uncertain ? '使用相同请求核对导出结果' : '按当前条件生成 CSV'}</button><button className="button secondary" disabled={!!busy || history.isFetching} onClick={() => void history.refetch()}>刷新我的导出</button></div>
      {uncertain && <p className="form-note" role="status">请求未确认成功，输入已保留。核对导出记录后，可使用相同请求编号重试。</p>}<ErrorNotice error={history.error} />
      {history.isPending ? <Busy label="读取导出历史" /> : !history.error && history.data?.items.map(item => <article key={item.id} className="report-export-row"><div><strong>{auditExportStates[item.state]} · {item.row_count} 条记录</strong><p>{date(item.created_at)} · 到期 {date(item.expires_at)}</p><p className="binding-id">{auditFilterSummary(item.filters)}</p><small className="binding-id">{item.id}<br />SHA-256 {item.csv_sha256}</small></div>{item.can_download && <button className="button secondary" disabled={!!busy} onClick={() => void download(item)}>{busy === item.id ? '正在校验…' : '校验并下载 CSV'}</button>}</article>)}
      {!history.error && history.data?.items.length === 0 && <p className="muted">尚无本人生成的固定导出。</p>}
      <div className="report-pagination"><button className="button secondary" disabled={historyCursors.length === 1 || history.isFetching} onClick={() => setHistoryCursors(historyCursors.slice(0, -1))}>上一页导出</button><button className="button secondary" disabled={!history.data?.next_cursor || history.isFetching} onClick={() => setHistoryCursors([...historyCursors, history.data!.next_cursor!])}>下一页导出</button></div>
    </section>}
    {detail && <AuditDetails id={detail} authority={[me.user.id, me.authorization_version]} close={() => setDetail(undefined)} />}
  </div>;
}

function AuditDetails({ id, authority, close }: { id: string; authority: (string | number)[]; close(): void }) {
  const query = useQuery({ queryKey: ['audit-event', ...authority, id], queryFn: async ({ signal }) => checkedAuditEvent(await api<AuditEvent>('/audit/events/' + id, { signal }), id) });
  const e = query.error ? undefined : query.data;
  return <Dialog title="审计记录详情" onClose={close}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : e && <dl className="file-facts">
    {([['事件标识', e.id], ['发生时间', date(e.occurred_at)], ['动作 / 结果', e.action + ' / ' + e.result], ['来源渠道', e.channel], ['操作者', e.actor_id], ['执行者', e.executor_id], ['资源类型', e.resource_type], ['资源标识', e.resource_id], ['资源版本', e.resource_version], ['请求编号', e.request_id], ['秘书任务', e.run_id], ['后台任务', e.job_id], ['变更前摘要', e.before_digest], ['变更后摘要', e.after_digest]] as const).map(([label, value]) => <div key={label}><dt>{label}</dt><dd className="binding-id">{value ?? '未记录'}</dd></div>)}
  </dl>}<p className="form-note">通用视图不返回自由填写的原因、会话标识或业务正文。</p><div className="dialog-actions"><button className="button primary" onClick={close}>关闭详情</button></div></Dialog>;
}
