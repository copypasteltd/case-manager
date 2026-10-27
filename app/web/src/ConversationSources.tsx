import { assertIndexedContext } from '@copypaste/contracts/indexed-context';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { ArrowRight, Search } from 'lucide-react';
import { api, type Page, type Project, type Schemas, type Task } from './api';
import { Busy, Empty, ErrorNotice, Field } from './components';

type Conversation = Schemas['Conversation'];
type ContextView = Schemas['ConversationContextView'];
type SourceType = Exclude<ContextView['type'], 'agent_finance_report'>;
type Choice = { id: string; title: string };
const sourceNames: Record<ContextView['type'], string> = { project: '项目说明', task: '项目任务', post: '知识文档', file: '固定文件正文', service: '服务状态资料', agent_finance_report: '固定财务快照' };

export function ConversationSources({ conversation: c, onSelect }: { conversation: Conversation; onSelect?(value: ContextView): void }) {
  const [kind, setKind] = useState<SourceType>(c.project_id ? 'project' : 'post');
  const [search, setSearch] = useState(''), [q, setQ] = useState(''), [source, setSource] = useState(c.project_id ?? '');
  const [versionID,setVersionID]=useState('');
  const pendingRead = useRef<AbortController | null>(null);
  useEffect(() => () => pendingRead.current?.abort(), []);
  const sources = useInfiniteQuery({
    queryKey: ['conversation-source-options', c.id, c.context_security_version, kind, q],
    initialPageParam: '',
    queryFn: async ({ pageParam, signal }): Promise<Page<Choice>> => {
      if (kind === 'project') {
        if (!c.project_id) return { items: [], next_cursor: null };
        const project = await api<Project>('/projects/' + c.project_id, { signal });
        return { items: [{ id: project.id, title: project.name }], next_cursor: null };
      }
      const query = new URLSearchParams({ q, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) });
      if (kind === 'task') {
        if (!c.project_id) return { items: [], next_cursor: null };
        return api<Page<Task>>('/projects/' + c.project_id + '/tasks?' + query, { signal });
      }
      if (c.project_id) query.set('project_id', c.project_id);
      else query.set('company_only','true');
      if(kind==='service'){
        const page=await api<Page<Schemas['Service']>>('/services?'+query,{signal});
        return {items:page.items.filter(item=>item.project_id===c.project_id).map(item=>({id:item.id,title:item.name})),next_cursor:page.next_cursor};
      }
      if(kind==='file'){
        const page=await api<Page<Schemas['File']>>('/files?'+query,{signal});
        return {items:page.items.filter(file=>file.project_id===c.project_id).map(file=>({id:file.id,title:file.name})),next_cursor:page.next_cursor};
      }
      const page = await api<Page<Schemas['Post']>>('/posts?' + query, { signal });
      return { items: page.items.filter(post => post.project_id === c.project_id).map(post => ({ id: post.id, title: post.title })), next_cursor: page.next_cursor };
    },
    getNextPageParam: page => page.next_cursor ?? undefined,
    enabled: c.state === 'active',
  });
  const choices = sources.error ? [] : [...new Map(sources.data?.pages.flatMap(page => page.items).map(item => [item.id, item])).values()];
  const versions=useInfiniteQuery({queryKey:['conversation-file-versions',c.id,c.context_security_version,source],initialPageParam:'',enabled:kind==='file'&&!!source&&c.state==='active',queryFn:({pageParam,signal})=>api<Page<Schemas['FileVersion']>>('/files/'+source+'/versions?limit=20'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}),getNextPageParam:page=>page.next_cursor??undefined});
  const preview = useMutation({
    gcTime: 0,
    mutationFn: async ({ offset, previous }: { offset: number; previous?: ContextView }) => {
      pendingRead.current?.abort();
      const controller = new AbortController();
      pendingRead.current = controller;
      const query = new URLSearchParams({ context_security_version: String(c.context_security_version), offset: String(offset), max_chars: '4000' });
      if (previous?.version_id) query.set('version_id', previous.version_id);
      else if(kind==='file'){if(!versionID)throw new Error('请先选择准确文件版本。');query.set('version_id',versionID);}
      try {
        const view = await api<ContextView>('/conversations/' + c.id + '/context-sources/' + kind + '/' + source + '?' + query, { signal: controller.signal });
        assertIndexedContext(view);
        if(kind==='file' && view.version_id!==(previous?.version_id??versionID)) throw new Error('文件版本已变化，请重新选择。');
        if (previous && view.source_sha256 !== previous.source_sha256) throw new Error('资料已更新，请从第一段重新核对。');
        return view;
      } finally {
        if (pendingRead.current === controller) pendingRead.current = null;
      }
    },
  });
  const view = preview.isSuccess ? preview.data : undefined;
  const reset = () => preview.reset();
  if (c.state !== 'active') return <section className="panel conversation-summary"><h2>准备给秘书的资料</h2><p className="muted">恢复会话后，可核对资料的阅读范围。</p></section>;
  return <section className="panel conversation-summary" aria-labelledby="conversation-sources-title">
    <div className="section-heading"><h2 id="conversation-sources-title">准备给秘书的资料</h2><span className="badge neutral">范围核对</span></div>
    <p className="muted">选择当前会话范围内的资料，核对所有成员共同可读、且允许秘书处理的内容。</p>
    <form className="conversation-source-controls" onSubmit={event => { event.preventDefault(); setSource(''); reset(); setQ(search.trim()); }}>
      <Field label="资料类型"><select value={kind} disabled={preview.isPending} onChange={event => { const value = event.target.value as SourceType; setKind(value); setVersionID(''); setSource(value === 'project' ? c.project_id ?? '' : ''); setSearch(''); setQ(''); reset(); }}>{c.project_id && <><option value="project">项目说明</option><option value="task">项目任务</option></>}<option value="post">知识文档</option><option value="file">固定文件正文</option><option value="service">服务状态资料</option></select></Field>
      {kind !== 'project' && <><Field label="查找资料"><input type="search" maxLength={200} value={search} onChange={event => setSearch(event.target.value)} disabled={preview.isPending} placeholder="输入资料标题" /></Field><button className="button secondary" disabled={preview.isPending}><Search size={16} />查找</button></>}
    </form>
    <ErrorNotice error={sources.error} />{sources.isPending && <Busy label="正在读取有权选择的资料" />}
    {!sources.isPending && !sources.error && <><Field label="选择资料"><select value={source} disabled={preview.isPending} onChange={event => { setSource(event.target.value); setVersionID(''); reset(); }}><option value="">请选择{sourceNames[kind]}</option>{choices.map(item => <option key={item.id} value={item.id}>{item.title}</option>)}</select></Field>{!choices.length && <p className="form-note">当前页没有此范围内的资料。{sources.hasNextPage ? '可继续加载，查找其他资料。' : '可在对应业务模块新增后再选择。'}</p>}{sources.hasNextPage && <button className="text-button" disabled={sources.isFetchingNextPage || preview.isPending} onClick={() => void sources.fetchNextPage()}>继续加载资料</button>}</>}
    <p className="form-note">共享会话可能显示已发布版本；任何成员无法阅读的资料均不展示。后续任务提交时会再次核对权限。</p>
    {kind==='service'&&<p className="form-note">这里核对服务资料。选择事件摘要任务后，秘书可读取带采集时间的观测、维护与共同授权事件；旧摘要不代表实时状态。</p>}
    {kind==='file'&&source&&<><p className="form-note">请选择准确正式版本。支持已扫描的UTF-8文本／Markdown／CSV／JSON（1MiB内），以及已完成正文提取的DOCX（原件8MiB内）。提取仅含主文档文字，原件可在文件入口查阅。</p><ErrorNotice error={versions.error}/>{versions.isPending?<Busy label="正在读取文件版本"/>:<Field label="准确文件版本"><select value={versionID} disabled={preview.isPending} onChange={event=>{setVersionID(event.target.value);reset();}}><option value="">请明确选择版本</option>{versions.data?.pages.flatMap(page=>page.items).map(version=><option key={version.id} value={version.id}>版本 {version.seq} · {version.media_type} · {version.size_bytes} 字节 · {version.scan_state}</option>)}</select></Field>}{versions.hasNextPage&&<button className="button secondary" disabled={versions.isFetchingNextPage||preview.isPending} onClick={()=>void versions.fetchNextPage()}>更多文件版本</button>}</>}
    <div className="post-actions"><button className="button primary" disabled={!source || !choices.some(item => item.id === source) || preview.isPending || kind==='file'&&!versionID} onClick={() => preview.mutate({ offset: 0 })}>{preview.isPending ? '正在核对资料' : '核对并预览'}</button></div>
    <ErrorNotice error={preview.error} />{preview.isPending && <Busy label="正在核对会话成员与资料权限" />}
    {view && <div className="conversation-source-preview" aria-live="polite"><div className="section-heading"><h3>{view.title}</h3><span className="badge neutral">{sourceNames[view.type]} · 第 {view.resource_version} 版</span></div><p className="form-note">{view.total_chars ? `当前片段：第 ${view.offset + 1}—${view.offset + [...view.text].length} 个字符，共 ${view.total_chars} 个字符。` : '当前资料没有正文。'}</p>{view.text_index && <div className="form-note"><strong>固定版本派生正文</strong><p>提取器 {view.text_index.extractor} · 索引于 {new Date(view.text_index.indexed_at!).toLocaleString()}</p><p>正文摘要 <code>{view.text_index.text_sha256}</code></p><p>仅包含已提取文字；版式、图片与嵌入对象请查阅原件。</p></div>}{view.text ? <pre>{view.text}</pre> : <Empty title="没有正文内容" detail="可在原资料中补充内容后重新核对。" />}{view.evidence.length > 0 && <><h4>资料所依据的文件版本</h4><ul>{view.evidence.map(file => <li key={file.version_id}><Link to={'/files/' + file.file_id}>{file.name}</Link><span className="muted"> · 第 {file.seq} 版</span></li>)}</ul></>}{view.next_offset !== null && <button className="button secondary" disabled={preview.isPending} onClick={() => preview.mutate({ offset: view.next_offset!, previous: view })}>核对下一段<ArrowRight size={16} /></button>}{onSelect && <button className="button secondary" disabled={preview.isPending} onClick={() => onSelect(view)}>将这份准确资料加入本次请求</button>}<p className="form-note">以上为不可信资料内容。预览核对共同阅读范围，不自动发送请求或授予操作权限。</p></div>}
  </section>;
}
