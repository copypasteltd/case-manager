import { Text, View } from '@tarojs/components';
import { useEffect, useRef, useState } from 'react';
import { sha256 } from '@noble/hashes/sha2.js';
import { auditFields, auditPath, auditLabels as labels, auditFilterSummary, auditExportStates, checkedAuditEvent, checkedAuditExport, checkedAuditExportUpdate, checkedAuditPage, currentAuditFilter, readAuditBytes, validateAuditFilter, type AuditEvent, type AuditExport, type AuditPage, type AuditChunk } from '@copypaste/contracts/audit';
import { Input } from '../../platform/components';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Page as ResultPage, type Schemas } from '../../platform/session';
import { reportFileAvailable, saveReportCSV } from '../../platform/report-file';
import { Action, Busy, Empty, Fact, Field, Notice, Section, date } from '../../ui';

const safe = (cause: unknown) => cause instanceof MobileError ? cause : new MobileError('AUDIT_CHECK_FAILED', cause instanceof Error ? cause.message : '审计资料暂时无法读取。');
function useResource<T>(path: string | null, parent: Scope, revision: number, check: (v: T) => T) {
  const [value, setValue] = useState<T>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(Boolean(path)), validate = useRef(check); validate.current = check;
  useEffect(() => {
    const scope = new Scope(), unbind = parent.own(() => scope.close()); setValue(undefined); setError(undefined); setLoading(Boolean(path));
    if (path) void client.request<T>(path, { scope }).then(v => { const checked = validate.current(v); if (!scope.closed) setValue(checked); }).catch(e => { if (!scope.closed) setError(safe(e)); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => { scope.close(); unbind(); };
  }, [path, parent, revision]);
  return { value, error, loading };
}

export function AuditWorkspace({ me, scope }: { me: Schemas['Me']; scope: Scope }) {
  const [draft, setDraft] = useState(currentAuditFilter), [applied, setApplied] = useState(currentAuditFilter), [advanced, setAdvanced] = useState(false), [filtering, setFiltering] = useState(true);
  const [revision, setRevision] = useState(0), [historyRevision, setHistoryRevision] = useState(0), [cursors, setCursors] = useState(['']), [historyCursors, setHistoryCursors] = useState(['']);
  const [selected, setSelected] = useState<string>(), [reason, setReason] = useState(''), [busy, setBusy] = useState(''), [error, setError] = useState<unknown>(), [uncertain, setUncertain] = useState(false), flight = useRef(false), command = useRef<{ body: string; key: string }>();
  const events = useResource<AuditPage>(me.actions.includes('audit.read') ? auditPath(applied, cursors[cursors.length - 1]) : null, scope, revision, p => checkedAuditPage(p, applied));
  const history = useResource<ResultPage<AuditExport>>(me.actions.includes('audit.export') ? '/audit/exports?limit=10' + (historyCursors[historyCursors.length - 1] ? '&cursor=' + encodeURIComponent(historyCursors[historyCursors.length - 1]) : '') : null, scope, historyRevision, p => { p.items.forEach(e => checkedAuditExport(e)); return p; });
  const detail = useResource<AuditEvent>(selected ? '/audit/events/' + selected : null, scope, revision, e => checkedAuditEvent(e, selected));
  const dirty = JSON.stringify(draft) !== JSON.stringify(applied);
  if (!me.actions.includes('audit.read')) return <Empty>当前没有组织级审计查询权限。</Empty>;
  function apply() {
    try { validateAuditFilter(draft); setApplied({ ...draft }); setCursors(['']); setSelected(undefined); setFiltering(false); setRevision(v => v + 1); setError(undefined); } catch (cause) { setError(safe(cause)); }
  }
  async function create() {
    if (flight.current || dirty || !reason.trim() || scope.closed) return;
    flight.current = true; setBusy('create'); setError(undefined);
    try {
      const body = { filters: applied, reason: reason.trim() }, encoded = JSON.stringify(body);
      if (!command.current || command.current.body !== encoded) command.current = { body: encoded, key: client.operationKey() };
      checkedAuditExport(await client.request<AuditExport>('/audit/exports', { method: 'POST', body, key: command.current.key, scope }), applied);
      if (!scope.closed) { command.current = undefined; setReason(''); setUncertain(false); setHistoryCursors(['']); setHistoryRevision(v => v + 1); }
    } catch (cause) { if (!scope.closed) { setError(safe(cause)); setUncertain(!(cause instanceof MobileError) || cause.status === 0 || cause.status >= 500); } } finally { flight.current = false; if (!scope.closed) setBusy(''); }
  }
  async function download(record: AuditExport) {
    if (flight.current || scope.closed) return;
    flight.current = true; setBusy(record.id); setError(undefined); let bytes: Uint8Array | undefined;
    const active = () => { if (scope.closed) throw new MobileError('REQUEST_CANCELLED', '审计读取已结束。'); };
    try {
      const fresh = checkedAuditExportUpdate(await client.request<AuditExport>('/audit/exports/' + record.id, { scope }), record);
      bytes = await readAuditBytes(fresh, (offset, length) => client.request<AuditChunk>('/audit/exports/' + record.id + '/chunks?offset=' + offset + '&length=' + length, { scope }), value => Array.from(sha256(value), b => b.toString(16).padStart(2, '0')).join(''), active);
      const current = await client.request<Schemas['Me']>('/me', { scope }); active();
      if (current.user.id !== me.user.id || current.authorization_version !== me.authorization_version || !current.actions.includes('audit.export')) throw new Error('权限已变化，请重新读取。');
      const final = checkedAuditExportUpdate(await client.request<AuditExport>('/audit/exports/' + record.id, { scope }), fresh);
      if (!final.can_download || final.csv_sha256 !== fresh.csv_sha256) throw new Error('固定导出已失效，请重新生成。');
      active(); await saveReportCSV(bytes, 'audit', record.id, scope);
    } catch (cause) { if (!scope.closed) { setError(safe(cause)); setHistoryRevision(v => v + 1); } } finally { bytes?.fill(0); flight.current = false; if (!scope.closed) setBusy(''); }
  }
  return <>
    <Text className="cp-muted cp-paragraph">查询组织级操作元数据。业务正文仍按原业务权限访问；自由填写的原因和会话标识不在通用视图中返回。</Text>
    <Notice error={error ?? events.error} /><Action secondary disabled={!!busy || uncertain} onClick={() => setFiltering(!filtering)}>{filtering ? '收起查询条件' : '调整查询条件'}</Action>
    {filtering && <Section title="查询条件"><Field label="开始日期（香港，YYYY-MM-DD）"><Input ariaLabel="审计开始日期" className="cp-input" value={draft.from.slice(0, 10)} disabled={!!busy || uncertain} onInput={e => setDraft({ ...draft, from: e.detail.value + 'T00:00:00+08:00' })} /></Field><Field label="结束日期（香港，不含，YYYY-MM-DD）"><Input ariaLabel="审计结束日期" className="cp-input" value={draft.to.slice(0, 10)} disabled={!!busy || uncertain} onInput={e => setDraft({ ...draft, to: e.detail.value + 'T00:00:00+08:00' })} /></Field>
      {(['action', 'result', 'channel'] as const).map(key => <Field key={key} label={labels[key]} hint="准确匹配，留空包含全部"><Input ariaLabel={'审计' + labels[key]} className="cp-input" value={draft[key]} maxlength={128} disabled={!!busy || uncertain} onInput={e => setDraft({ ...draft, [key]: e.detail.value.trim() })} /></Field>)}
      <Action secondary onClick={() => setAdvanced(!advanced)}>{advanced ? '收起更多线索' : '按人员、资源或任务筛选'}</Action>
      {advanced && auditFields.filter(key => !['action', 'result', 'channel'].includes(key)).map(key => <Field key={key} label={labels[key]} hint="准确技术标识；人员和资源使用完整UUID"><Input ariaLabel={'审计' + labels[key]} className="cp-input" value={draft[key]} maxlength={128} disabled={!!busy || uncertain} onInput={e => setDraft({ ...draft, [key]: e.detail.value.trim() })} /></Field>)}
      <Action disabled={!!busy || uncertain} onClick={apply}>应用查询</Action>
    </Section>}
    {dirty && <Text className="cp-notice">筛选尚未应用，列表仍显示上次查询结果。</Text>}
    <Action secondary disabled={!!busy || events.loading} onClick={() => setRevision(v => v + 1)}>刷新当前结果</Action>
    <Section title="操作记录"><Text className="cp-muted cp-paragraph">已应用：{auditFilterSummary(applied)}</Text>{events.loading ? <Busy /> : !events.error && events.value?.items.map(item => <Fact key={item.id} label={date(item.occurred_at)}><Text>{item.action} · {item.result}</Text><Text className="cp-muted cp-small">{item.channel} · {item.actor_id ?? '系统执行'}</Text><Text className="cp-code cp-small">{item.resource_type} · {item.resource_id ?? '无对象标识'}</Text><Action secondary onClick={() => setSelected(item.id)}>查看详情</Action></Fact>)}{!events.error && events.value?.items.length === 0 && <Empty>没有匹配的审计记录。</Empty>}<View className="cp-choice"><Action secondary disabled={cursors.length === 1 || events.loading} onClick={() => { setSelected(undefined); setCursors(cursors.slice(0, -1)); }}>上一页记录</Action><Action secondary disabled={!events.value?.next_cursor || events.loading} onClick={() => { setSelected(undefined); setCursors([...cursors, events.value!.next_cursor!]); }}>下一页记录</Action></View></Section>
    {selected && <Section title="审计记录详情"><Notice error={detail.error} />{detail.loading ? <Busy /> : !detail.error && detail.value && Object.entries({ '事件标识': detail.value.id, '时间': date(detail.value.occurred_at), '动作 / 结果': detail.value.action + ' / ' + detail.value.result, '来源渠道': detail.value.channel, '操作者': detail.value.actor_id, '执行者': detail.value.executor_id, '资源类型': detail.value.resource_type, '资源标识': detail.value.resource_id, '资源版本': detail.value.resource_version, '请求编号': detail.value.request_id, '秘书任务': detail.value.run_id, '后台任务': detail.value.job_id, '变更前摘要': detail.value.before_digest, '变更后摘要': detail.value.after_digest }).map(([label, value]) => <Fact key={label} label={label}><Text className="cp-code cp-small">{value ?? '未记录'}</Text></Fact>)}<Action secondary onClick={() => setSelected(undefined)}>关闭详情</Action></Section>}
    {me.actions.includes('audit.export') && <Section title="固定导出"><Text className="cp-muted cp-paragraph">固定已应用条件的全部匹配事件；最多10,000条、8 MiB，有效24小时。超出范围整单拒绝。生成与下载需要近期两步验证，CSV按文本导入。</Text><Field label="导出用途"><Input ariaLabel="审计导出用途" className="cp-input" value={reason} maxlength={200} disabled={!!busy || uncertain} onInput={e => setReason(e.detail.value)} /></Field><Action disabled={!!busy || dirty || !reason.trim()} loading={busy === 'create'} onClick={() => void create()}>{uncertain ? '使用相同请求核对导出结果' : '按当前条件生成 CSV'}</Action>{uncertain && <Text className="cp-notice">请求未确认成功，输入已保留。核对记录后可使用相同请求编号重试。</Text>}<Action secondary disabled={!!busy || history.loading} onClick={() => setHistoryRevision(v => v + 1)}>刷新我的导出</Action><Notice error={history.error} />{!reportFileAvailable() && <Text className="cp-muted cp-paragraph">本端可查询和生成固定导出。CSV文件请使用同一账号在Web下载。</Text>}
      {history.loading ? <Busy /> : !history.error && history.value?.items.map(item => <Fact key={item.id} label={auditExportStates[item.state] + ' · ' + item.row_count + ' 条记录'}><Text>{date(item.created_at)} · 到期 {date(item.expires_at)}</Text><Text className="cp-code cp-small">{auditFilterSummary(item.filters)}</Text><Text className="cp-code cp-small">{item.id}</Text><Text className="cp-code cp-small">SHA-256 {item.csv_sha256}</Text>{item.can_download && reportFileAvailable() && <Action secondary disabled={!!busy} loading={busy === item.id} onClick={() => void download(item)}>校验并打开 CSV</Action>}</Fact>)}{!history.error && history.value?.items.length === 0 && <Empty>尚无本人生成的固定导出。</Empty>}
      <View className="cp-choice"><Action secondary disabled={historyCursors.length === 1 || history.loading} onClick={() => setHistoryCursors(historyCursors.slice(0, -1))}>上一页导出</Action><Action secondary disabled={!history.value?.next_cursor || history.loading} onClick={() => setHistoryCursors([...historyCursors, history.value!.next_cursor!])}>下一页导出</Action></View>
    </Section>}
  </>;
}
