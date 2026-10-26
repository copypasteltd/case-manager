import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { makePostDraft, makePostPublish, postTypes, postWriteReceipt, postWriteScopeAllowed, type PostWrite } from '@copypaste/contracts/post-write';
import { api, type Schemas, type Page } from './api';
import { Busy, classificationNames, ErrorNotice, Field } from './components';
import Markdown from './Markdown';

type Props={conversation:Schemas['Conversation'];disabled:boolean;value?:PostWrite;references:Schemas['ResourceRef'][];onChange(value?:PostWrite):void};
export function ConversationPostWrite({conversation:c,disabled,value,references,onChange}:Props){
 const [open,setOpen]=useState(false),[mode,setMode]=useState<'new'|'revise'|'publish'>('new'),[id,setID]=useState(''),[title,setTitle]=useState(''),[type,setType]=useState('project_log'),[classification,setClassification]=useState(c.project_id?'project_restricted':'internal'),[why,setWhy]=useState(''),[reviewed,setReviewed]=useState(false),[error,setError]=useState<unknown>();
 const list=useInfiniteQuery({queryKey:['secretary-posts',c.id],initialPageParam:'',enabled:open&&mode!=='new',queryFn:({pageParam,signal})=>api<Page<Schemas['Post']>>('/posts?limit=20'+(c.project_id?'&project_id='+c.project_id:'')+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}),getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
 const current=useQuery({queryKey:['secretary-post',c.id,id],enabled:open&&mode!=='new'&&!!id,queryFn:({signal})=>api<Schemas['Post']>('/posts/'+id,{signal}),gcTime:0});
 const p=current.error?undefined:current.data;
 useEffect(()=>{setReviewed(false);setError(undefined);if(p){setTitle(p.title);setType(p.type??'project_log');setClassification(p.classification??'internal');}},[id,p?.version,p?.content_hash]);
 if(c.type!=='private')return <p className='form-note'>知识写入请进入本人私密会话。</p>;
 const attachments=references.filter(r=>r.type==='file'&&!!r.version_id);
 return <section className='conversation-source-preview'><h3>本次知识草稿或内部发布</h3>{value?<><p>{value.tool==='publish_post'?'发布已核对的准确修订':value.post_id?'生成并保存指定文档的草稿修订':'生成并保存新的知识草稿'}</p><p className='form-note'>已固定本次目标；正文写作要求请填写在发给秘书的请求中。</p><button type='button' className='button secondary' disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次知识操作</button></>:!open?<button type='button' className='button secondary' disabled={disabled} onClick={()=>setOpen(true)}>选择知识草稿或发布</button>:<>
 <ErrorNotice error={error??list.error??current.error}/>
 <Field label='知识操作'><select value={mode} disabled={disabled} onChange={e=>{setMode(e.target.value as typeof mode);setID('');setReviewed(false);}}><option value='new'>新建知识草稿</option><option value='revise'>修订知识草稿</option><option value='publish'>核对后内部发布</option></select></Field>
 {mode!=='new'&&<><Field label='知识文档'><select value={id} disabled={disabled} onChange={e=>setID(e.target.value)}><option value=''>请选择知识文档</option>{list.data?.pages.flatMap(p=>p.items).filter(p=>p.project_id===c.project_id&&p.state!=='archived').map(p=><option key={p.id} value={p.id}>{p.title}</option>)}</select></Field>{list.hasNextPage&&<button className='button secondary' type='button' disabled={disabled||list.isFetchingNextPage} onClick={()=>void list.fetchNextPage()}>加载更多知识文档</button>}{current.isFetching&&<Busy/>}</>}
 {mode!=='publish'&&<><Field label='知识标题'><input maxLength={200} value={title} disabled={disabled} onChange={e=>setTitle(e.target.value)}/></Field><Field label='知识类型'><select value={type} disabled={disabled||mode==='revise'} onChange={e=>setType(e.target.value)}>{Object.entries(postTypes).map(([key,label])=><option key={key} value={key}>{label}</option>)}</select></Field><Field label='知识分类'><select value={classification} disabled={disabled||mode==='revise'} onChange={e=>setClassification(e.target.value)}>{['internal','project_restricted'].filter(v=>postWriteScopeAllowed(v,c.classification)&&!!(c.project_id||v==='internal')).map(v=><option key={v} value={v}>{classificationNames[v]}</option>)}</select></Field><p className='form-note'>本次附件使用下方已选择的准确文件版本，共 {attachments.length} 项；仅保存草稿，原发布版保持有效。</p></>}
 {p&&<><p>文档修订 {p.version} · {p.state} · {classificationNames[p.classification??'']}</p><p className='form-note'>正文摘要：{p.content_hash}</p><Link to={'/knowledge/'+p.id}>查看知识详情与历史</Link></>}
 {mode==='publish'&&p&&<><h4>本次完整内部发布内容</h4><h4>{p.title}</h4><Markdown body={p.body??''}/><p>摘要：{p.summary||'（无）'}</p><p>标签：{p.tags?.join('、')||'（无）'}</p><ul>{p.source_refs?.map(r=><li key={r.id+(r.version_id??'')}>文件 {r.id} · 固定版本 {r.version_id??'未固定，不能发布'}</li>)}</ul><label><input type='checkbox' checked={reviewed} disabled={disabled} onChange={e=>setReviewed(e.target.checked)}/>我已核对完整正文、摘要、标签、分类和固定附件，确认内部发布</label></>}
 <Field label='知识操作说明'><input maxLength={200} value={why} disabled={disabled} onChange={e=>setWhy(e.target.value)}/></Field>
 <button type='button' className='button secondary' disabled={disabled||!why.trim()||(mode!=='new'&&(!p||current.isFetching))||(mode==='publish'&&!reviewed)} onClick={()=>{try{onChange(mode==='publish'?makePostPublish(p!,c.project_id,why):makePostDraft(c.project_id,type,title,classification,attachments,why,mode==='revise'?p:undefined));setOpen(false);}catch(e){setError(e);}}}>将知识操作加入本次请求</button><button type='button' className='button secondary' disabled={disabled} onClick={()=>setOpen(false)}>收起知识选择</button>
 </>}</section>;
}
export function PostWriteReceipt({run}:{run:Schemas['Run']}){
 const v=run.post_write!,p=v.result;
 return <section className='conversation-source-preview'><h4>知识执行记录</h4>{postWriteReceipt(run)&&p?<><p>{v.input.tool==='publish_post'?'已发布准确内部修订':'知识草稿已保存，发布需另行核对'} · {p.title}</p><p>文档修订 {p.version} · 操作 {v.operation_id}</p><Link to={'/knowledge/'+p.id}>查阅完整知识内容</Link></>:<p>已记录本次目标，请以真实保存记录核对执行结果。</p>}</section>;
}
