import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type Schemas } from './api';
import { Busy, ErrorNotice } from './components';

export function ConversationServiceReport({ conversation: c, source }: { conversation: Schemas['Conversation']; source: Schemas['AgentSourceSnapshot'] }) {
  const [open,setOpen]=useState(false),[offset,setOffset]=useState(0);
  const query=useQuery({queryKey:['service-report',c.id,c.context_security_version,source.id,source.version_id,source.sha256,offset],enabled:open&&!!source.version_id,gcTime:0,retry:false,
    queryFn:async({signal})=>{
      const view=await api<Schemas['ConversationContextView']>('/conversations/'+c.id+'/context-sources/service/'+source.id+'?context_security_version='+c.context_security_version+'&version_id='+source.version_id+'&offset='+offset+'&max_chars=4000',{signal});
      if(view.source_sha256!==source.sha256||view.version_id!==source.version_id||view.conversation_revision_id!==c.current_revision_id)throw new Error('来源或会话范围已变化，请重新核对当前权限。');
      return view;
    }});
  const view=query.isSuccess&&!query.isFetching?query.data:undefined;
  return <><button className="button secondary" onClick={()=>{setOpen(!open);setOffset(0);}}>{open?'收起状态快照':'查看固定状态快照'}</button>{open&&<section className="context-preview"><h3>固定状态快照</h3><p className="notice neutral">这是读取时刻的历史证据。请以 captured_at、时间窗和截断标记判断覆盖范围；实时状态请到服务状态页面核对。</p><ErrorNotice error={query.error}/>{query.isPending||query.isFetching?<Busy/>:view&&<><p>{view.title} · 快照 {source.version_id}</p><pre>{view.text}</pre>{offset>0&&<button className="button secondary" onClick={()=>setOffset(0)}>回到快照第一段</button>}{view.next_offset!==null&&<button className="button secondary" onClick={()=>setOffset(view.next_offset!)}>读取快照下一段</button>}</>}</section>}</>;
}
