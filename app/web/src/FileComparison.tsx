import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { officeTextNotice } from '@copypaste/contracts/search';
import { checkedComparison, comparisonEligible, comparisonMeaning, comparisonPath, type ComparisonSelection, type ComparisonView, type FileComparison } from '@copypaste/contracts/file-comparison';
import { api, type Schemas } from './api';
import { Busy, ErrorNotice, Field } from './components';

type Request = { selection: ComparisonSelection; offset: number; previous?: FileComparison };
export default function FileComparisonPanel({ file, versions, more, loading, loadMore, unavailable }: {
  file: Schemas['File']; versions: Schemas['FileVersion'][]; more: boolean; loading: boolean; loadMore(): void; unavailable: boolean;
}) {
  const [baseID, setBaseID] = useState(''), [targetID, setTargetID] = useState(''), [view, setView] = useState<ComparisonView>('changes');
  const [request, setRequest] = useState<Request>(), [inputError, setInputError] = useState<unknown>();
  const result = useQuery({ queryKey: ['file-text-comparison', file.id, file.version, request?.selection.base.id, request?.selection.target.id, request?.selection.view, request?.offset, request?.previous?.comparison_sha256], enabled: !!request && !unavailable && file.state === 'active', gcTime: 0, retry: false,
    queryFn: async ({ signal }) => checkedComparison(await api<FileComparison>(comparisonPath(request!.selection, request!.offset, request!.previous), { signal }), request!.selection, request!.offset, request!.previous),
    refetchInterval: 30000, refetchIntervalInBackground: false });
  const value = !unavailable && !result.error && request && result.data;
  const reset = () => { setRequest(undefined); setInputError(undefined); };
  const eligible = versions.filter(comparisonEligible);
  function compare() {
    try {
      const selection = { file, base: versions.find(v => v.id === baseID)!, target: versions.find(v => v.id === targetID)!, view };
      comparisonPath(selection); setRequest({ selection, offset: 0 }); setInputError(undefined);
      if (request && request.offset === 0 && request.selection.base.id === baseID && request.selection.target.id === targetID && request.selection.view === view) void result.refetch();
    } catch (error) { setRequest(undefined); setInputError(error); }
  }
  return <section className="panel file-comparison" aria-label="文件版本正文比较"><h2>比较两个内容版本</h2>
    <p className="muted">选择准确正式版本，核对完整提取正文的行差异。文本、Markdown、CSV、JSON及DOCX、XLSX、PPTX正文可参与；排版、图片和批注须查阅原件。</p>
    <div className="party-filters"><Field label="比较基础版本"><select value={baseID} disabled={unavailable || file.state !== 'active'} onChange={e => { setBaseID(e.target.value); reset(); }}><option value="">请选择基础版本</option>{eligible.map(v => <option key={v.id} value={v.id}>内容版本 {v.seq} · {v.id.slice(0, 8)}</option>)}</select></Field>
      <Field label="比较目标版本"><select value={targetID} disabled={unavailable || file.state !== 'active'} onChange={e => { setTargetID(e.target.value); reset(); }}><option value="">请选择目标版本</option>{eligible.map(v => <option key={v.id} value={v.id}>内容版本 {v.seq} · {v.id.slice(0, 8)}</option>)}</select></Field>
      <Field label="差异显示范围"><select value={view} onChange={e => { setView(e.target.value as ComparisonView); reset(); }}><option value="changes">差异与前后文</option><option value="all">全部正文</option></select></Field></div>
    <div className="form-actions"><button className="button" disabled={!baseID || !targetID || unavailable || file.state !== 'active' || result.isFetching} onClick={compare}>比较所选版本</button>
      <button className="button secondary" disabled={!baseID || !targetID || result.isFetching} onClick={() => { setBaseID(targetID); setTargetID(baseID); reset(); }}>交换比较方向</button>
      {more && <button className="button secondary" disabled={loading} onClick={loadMore}>加载更多可选历史版本</button>}</div>
    {eligible.length < 2 && <p className="form-note">当前加载的可选文本版本不足两个；可继续加载历史版本，或查阅原件及正文提取状态。</p>}
    <ErrorNotice error={inputError ?? result.error} />{request && result.isPending && <Busy label="正在核对两个准确版本" />}
    {value && request && <div aria-label="版本正文差异"><h3>内容版本 {value.base.seq} → {value.target.seq}</h3><p>{comparisonMeaning(value)}</p>{value.extractor !== 'text-v1' && <p className="form-note">{officeTextNotice}</p>}
      <p>新增 {value.added_lines} 行 · 删除 {value.removed_lines} 行 · 相同 {value.unchanged_lines} 行</p>
      <p className="form-note">{value.view === 'all' ? '全部正文按页展示。' : '展示全部差异及前后各3行，省略的相同行另行标明。'}末尾换行单独核对。{value.algorithm === 'bounded-replacement-v1' && '变化较多，使用完整中段替换呈现；新增／删除行数可能包含中段内相同的行。'}</p>
      <details><summary>核对准确版本与内容摘要</summary><p className="comparison-digest">基础原件 {value.base.source_sha256}<br />基础正文 {value.base.text_sha256}<br />目标原件 {value.target.source_sha256}<br />目标正文 {value.target.text_sha256}<br />本次比较 {value.comparison_sha256}</p></details>
      <div className="comparison-rows" aria-label="正文差异行">{value.rows.map((row, index) => <div className={'comparison-row comparison-' + row.kind} key={value.offset + index} data-diff-kind={row.kind}>
        {row.kind === 'skip' ? <p className="comparison-skip">省略 {row.count} 行相同正文 · 基础第 {row.base_line} 行／目标第 {row.target_line} 行起</p> : <><div className="comparison-coordinates">{row.kind === 'insert' ? '+' : row.kind === 'delete' ? '−' : '='} 基础 {row.base_line ?? '—'} · 目标 {row.target_line ?? '—'}</div><pre>{row.text || '\u00a0'}</pre>{!row.newline && <small className="muted">此行后无末尾换行</small>}</>}
      </div>)}</div>
      {value.total_rows === 0 && <p>两个版本的提取正文均为空。</p>}
      <div className="form-actions"><button className="button secondary" disabled={value.offset === 0 || result.isFetching} onClick={() => setRequest({ selection: request.selection, offset: Math.max(0, value.offset - 60), previous: value })}>上一页正文差异</button>
        <span className="muted">第 {value.total_rows ? value.offset + 1 : 0}～{value.offset + value.rows.length} 项，共 {value.total_rows} 项</span>
        <button className="button secondary" disabled={value.next_offset === null || result.isFetching} onClick={() => setRequest({ selection: request.selection, offset: value.next_offset!, previous: value })}>下一页正文差异</button></div>
    </div>}
  </section>;
}
