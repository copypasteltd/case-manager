import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, type Schemas } from './api';
import { Busy, ErrorNotice } from './components';

export function ConversationFinanceReport({ conversation: c, source }: { conversation: Schemas['Conversation']; source: Schemas['AgentSourceSnapshot'] }) {
  const [open,setOpen]=useState(false),[offset,setOffset]=useState(0);
  const query=useQuery({queryKey:['finance-report',c.id,c.context_security_version,source.id,source.version_id,source.sha256,offset],enabled:open,gcTime:0,retry:false,
    queryFn:async({signal})=>{
      const view=await api<Schemas['ConversationContextView']>('/conversations/'+c.id+'/context-sources/agent_finance_report/'+source.id+'?context_security_version='+c.context_security_version+'&offset='+offset+'&max_chars=4000',{signal});
      if(view.source_sha256!==source.sha256||view.version_id!==null||view.conversation_revision_id!==c.current_revision_id)throw new Error('来源或会话范围已变化，请重新核对当前权限。');
      return view;
    }});
  const view=query.isSuccess&&!query.isFetching?query.data:undefined;
  return <><button className="button secondary" onClick={()=>{setOpen(!open);setOffset(0);}}>{open?'收起财务快照':'查看固定财务快照'}</button>{open&&<section className="context-preview"><h3>固定财务快照</h3><p className="notice neutral">这是读取时刻的财务证据。请核对月份、分摊范围及覆盖说明；预测、发生支出和现金分别计量，原币种不合并。</p><ErrorNotice error={query.error}/>{query.isPending||query.isFetching?<Busy/>:view&&<><p>{view.title} · 快照 {source.id}</p><pre>{view.text}</pre>{offset>0&&<button className="button secondary" onClick={()=>setOffset(0)}>回到快照第一段</button>}{view.next_offset!==null&&<button className="button secondary" onClick={()=>setOffset(view.next_offset!)}>读取快照下一段</button>}</>}</section>}</>;
}
