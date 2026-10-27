import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Search as SearchIcon, ArrowUpRight, FileText } from 'lucide-react';
import { checkedSearch, checkedFileText, searchPath, fileTextPath, resourceNames, textStates, type SearchHit, type SearchType } from '@copypaste/contracts/search';
import { api, type Me } from './api';
import { Busy, Empty, ErrorNotice, date } from './components';

export default function SearchPage({ me }: { me: Me }) {
  const [params, setParams] = useSearchParams();
  const q = params.get('q') ?? '', type = (params.get('type') ?? '') as SearchType;
  const [draft, setDraft] = useState(q), [draftType, setDraftType] = useState<SearchType>(type in resourceNames ? type : ''), [localError, setError] = useState<unknown>();
  const [preview, setPreview] = useState<SearchHit>();
  const search = useInfiniteQuery({ queryKey: ['unified-search', me.user.id, me.authorization_version, q, type], initialPageParam: '', enabled: !!q,
    queryFn: async ({ pageParam, signal }) => checkedSearch(await api(searchPath(q, type, pageParam), { signal })), getNextPageParam: last => last.next_cursor ?? undefined, staleTime: 0 });
  const items = search.data?.pages.flatMap(page => page.items) ?? [];
  function apply(event: React.FormEvent) { event.preventDefault(); try { searchPath(draft, draftType); setError(undefined); setPreview(undefined); setParams({ q: draft.trim(), ...(draftType ? { type: draftType } : {}) }); } catch (e) { setError(e); } }
  return <><div className="page-heading"><div><span className="eyebrow">FIND THE RIGHT CONTEXT</span><h1>搜索工作资料<span className="accent">.</span></h1><p className="muted">查找项目、任务、知识记录、目录和文件正文。</p></div></div>
    <form className="panel search-form" onSubmit={apply}><label className="search-field"><SearchIcon size={19} aria-hidden="true" /><input aria-label="搜索工作资料" placeholder="输入名称或正文中的词句" value={draft} maxLength={200} onChange={e => setDraft(e.target.value)} /></label><label className="search-kind"><span className="sr-only">资源类型</span><select aria-label="资源类型" value={draftType} onChange={e => setDraftType(e.target.value as SearchType)}><option value="">所有资料</option>{Object.entries(resourceNames).map(([key, name]) => <option key={key} value={key}>{name}</option>)}</select></label><button className="button primary" type="submit">搜索资料</button></form>
    <ErrorNotice error={localError ?? search.error} />
    {!q ? <Empty title="从一个词句开始" detail="文件正文按当前正式版本检索；你也可以按名称查找尚未提取正文的文件。" /> : search.isLoading ? <Busy /> : !search.error && <><div className="section-heading"><h2>“{q}”的搜索结果</h2><button className="button ghost" disabled={search.isFetching} onClick={() => { setPreview(undefined); void search.refetch(); }}>重新检索</button></div>{!items.length ? <Empty title="没有找到可见资料" detail="尝试其他词句或资源类型；正文尚未提取的文件可按名称查找。" /> : <div className="search-results">{items.map(hit => <article className="panel search-hit" key={hit.resource.type + '/' + hit.resource.id}><div className="search-hit-heading"><span className="eyebrow">{resourceNames[hit.resource.type as keyof typeof resourceNames]}</span><small className="muted">{hit.match_types.map(m => ({ name: '名称', body: '正文', tags: '标签' })[m]).join(' · ')}命中</small></div><h3><Link to={resourceURL(hit)}>{hit.title}<ArrowUpRight size={17} aria-hidden="true" /></Link></h3>{hit.snippet && <p className="search-snippet">{hit.snippet.replace(/\s+/gu,' ')}</p>}<div className="search-hit-footer"><small className="muted">更新 {date(hit.updated_at)}{hit.text_state && ' · ' + textStates[hit.text_state]}{hit.indexed_at && ' · 提取 ' + date(hit.indexed_at)}</small>{hit.resource.type === 'file' && hit.resource.version_id && <button className="button secondary" onClick={() => setPreview(hit)}><FileText size={15} />查阅此版本正文</button>}</div></article>)}</div>}{search.hasNextPage && <button className="button secondary" disabled={search.isFetchingNextPage} onClick={() => void search.fetchNextPage()}>{search.isFetchingNextPage ? '正在读取' : '更多结果'}</button>}</>}
    {preview?.resource.version_id && <TextPreview key={preview.resource.id + preview.resource.version_id} hit={preview} close={() => setPreview(undefined)} />}
  </>;
}
function resourceURL(h: SearchHit) { return h.resource.type === 'project' ? '/projects/' + h.resource.id : h.resource.type === 'task' ? '/projects/' + h.project_id : h.resource.type === 'post' ? '/knowledge/' + h.resource.id : h.resource.type === 'folder' ? '/files/folders/' + h.resource.id : '/files/' + h.resource.id; }
function TextPreview({ hit, close }: { hit: SearchHit; close(): void }) {
  const file = hit.resource.id, version = hit.resource.version_id!;
  const [offsets, setOffsets] = useState([0]); const offset = offsets[offsets.length - 1];
  const text = useQuery({ queryKey: ['file-text', file, version, offset], queryFn: async ({ signal }) => checkedFileText(await api(fileTextPath(file, version, offset), { signal }), file, version, offset), staleTime: 0, gcTime: 0 });
  return <section className="panel file-text-preview" aria-label="固定版本正文"><div className="section-heading"><h2>{hit.title} · 正文</h2><button className="button ghost" onClick={close}>关闭正文</button></div><p className="muted">固定版本 <code>{version}</code></p><ErrorNotice error={text.error} />{text.isLoading ? <Busy /> : text.data && <>{text.data.state === 'ready' ? <><small className="muted">提取 {date(text.data.indexed_at)} · 字符 {offset + 1}–{offset + [...text.data.text].length}／{text.data.total_characters}</small><pre className="file-text-body">{text.data.text || '此版本没有可显示的正文。'}</pre></> : <p>{textStates[text.data.state]}。可打开文件详情查阅原件。{text.data.error_code && <small className="muted"> {text.data.error_code}</small>}</p>}<div className="form-actions">{offsets.length > 1 && <button className="button secondary" onClick={() => setOffsets(old => old.slice(0, -1))}>上一段正文</button>}{text.data.next_offset !== null && <button className="button secondary" onClick={() => setOffsets(old => [...old, text.data!.next_offset!])}>下一段正文</button>}<Link className="button secondary" to={'/files/' + file}>打开文件详情</Link></div></>}</section>;
}
