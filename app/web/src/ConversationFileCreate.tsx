import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { makeFileCreate, type FileCreate } from '@copypaste/contracts/file-update';
import { api, type Page, type Schemas } from './api';
import { Busy, ErrorNotice, Field } from './components';

export function ConversationFileCreate({ conversation:c, disabled, value, onChange }: { conversation:Schemas['Conversation']; disabled:boolean; value?:FileCreate; onChange(value?:FileCreate):void }) {
  const [open,setOpen]=useState(false), [folderID,setFolderID]=useState(''), [name,setName]=useState(''), [media,setMedia]=useState<'text/plain'|'text/markdown'>('text/markdown'), [error,setError]=useState<unknown>();
  const options=useInfiniteQuery({ queryKey:['file-create-folders',c.id], enabled:open, initialPageParam:'', queryFn:({pageParam,signal})=>api<Page<Schemas['Folder']>>('/folders?limit=20'+(c.project_id?'&project_id='+c.project_id:'')+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}), getNextPageParam:p=>p.next_cursor??undefined,gcTime:0 });
  const current=useQuery({queryKey:['file-create-folder',c.id,folderID],enabled:open&&!!folderID,queryFn:({signal})=>api<Schemas['Folder']>('/folders/'+folderID,{signal}),gcTime:0});
  const children=useInfiniteQuery({queryKey:['file-create-children',c.id,folderID],enabled:open&&!!folderID,initialPageParam:'',queryFn:({pageParam,signal})=>api<Page<Schemas['Folder']>>('/folders?limit=20&parent_id='+folderID+(c.project_id?'&project_id='+c.project_id:'')+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}),getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
  const folders=options.data?.pages.flatMap(p=>p.items).filter(f=>f.project_id===c.project_id&&f.state==='active')??[];
  const folder=current.error?undefined:current.data;
  return <section className="conversation-source-preview"><h3>本次准确文件新建</h3>{value?<><p>{value.name} · 目录修订 {value.expected_folder_version}</p><p className="form-note">发送后只授权在所选目录创建这一文件；重名会停止，请在消息中说明正文要求。</p><button type="button" className="button secondary" disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次文件新建</button></>:!open?<button type="button" className="button secondary" disabled={disabled} onClick={()=>setOpen(true)}>选择新建文件位置</button>:<>
    <ErrorNotice error={error??options.error??current.error??children.error}/>
    <Field label="新文件资料目录"><select disabled={disabled} value={folderID} onChange={e=>setFolderID(e.target.value)}><option value="">请选择目录</option>{folders.map(f=><option key={f.id} value={f.id}>{f.name}</option>)}{folder&&!folders.some(f=>f.id===folder.id)&&<option value={folder.id}>{folder.name}</option>}</select></Field>
    {options.hasNextPage&&<button type="button" className="button secondary" disabled={disabled||options.isFetchingNextPage} onClick={()=>void options.fetchNextPage()}>加载更多目录</button>}
    {current.isFetching&&folderID&&<Busy label="正在核对目标目录"/>}{folder&&<><p>目标：{folder.name} · 修订 {folder.version}</p>{folder.parent_id&&<button type="button" className="button secondary" disabled={disabled} onClick={()=>setFolderID(folder.parent_id!)}>返回上级目录</button>}{children.data?.pages.flatMap(p=>p.items).filter(f=>f.project_id===c.project_id&&f.state==='active').map(f=><button type="button" className="button secondary" key={f.id} disabled={disabled} onClick={()=>setFolderID(f.id)}>进入目录：{f.name}</button>)}{children.hasNextPage&&<button type="button" className="button secondary" disabled={disabled||children.isFetchingNextPage} onClick={()=>void children.fetchNextPage()}>加载更多子目录</button>}</>}
    <Field label="新文件格式"><select disabled={disabled} value={media} onChange={e=>setMedia(e.target.value as typeof media)}><option value="text/markdown">Markdown（.md／.markdown）</option><option value="text/plain">文本（.txt）</option></select></Field>
    <Field label="新文件名称"><input disabled={disabled} maxLength={255} value={name} onChange={e=>setName(e.target.value)} placeholder="例如：项目会议纪要.md"/></Field>
    <p className="form-note">选择后需明确发送正文要求。新建正文最多48KiB，草稿经校验和扫描后才成为正式文件；已有资料不会被覆盖。</p>
    <button type="button" className="button secondary" disabled={disabled||!folder||current.isFetching||!name.trim()} onClick={()=>{try{onChange(makeFileCreate(folder!,name,media,c.project_id));setOpen(false);}catch(cause){setError(cause);}}}>将准确文件新建加入请求</button>
    <button type="button" className="button secondary" disabled={disabled} onClick={()=>setOpen(false)}>收起文件新建</button>
  </>}</section>;
}
