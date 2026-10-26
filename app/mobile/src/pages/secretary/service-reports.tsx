import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { Action, Busy, Notice, Section } from '../../ui';

export function ServiceReport({ conversation:c, source, onClose }:{conversation:Schemas['Conversation'];source:Schemas['AgentSourceSnapshot'];onClose():void}){
 const [offset,setOffset]=useState(0),[view,setView]=useState<Schemas['ConversationContextView']>(),[error,setError]=useState<unknown>(),scope=useRef<Scope>();
 useDidHide(()=>{scope.current?.close();setView(undefined);setError(undefined);onClose();});
 useEffect(()=>{const current=new Scope();scope.current=current;setView(undefined);setError(undefined);
  void client.request<Schemas['ConversationContextView']>('/conversations/'+c.id+'/context-sources/service/'+source.id+'?context_security_version='+c.context_security_version+'&version_id='+source.version_id+'&offset='+offset+'&max_chars=4000',{scope:current}).then(value=>{
   if(value.source_sha256!==source.sha256||value.version_id!==source.version_id||value.conversation_revision_id!==c.current_revision_id)throw new MobileError('SOURCE_CHANGED','来源或会话范围已变化，请重新核对当前权限。');
   if(!current.closed)setView(value);
  }).catch(cause=>{if(!current.closed)setError(cause);});return()=>current.close();
 },[c.id,c.current_revision_id,c.context_security_version,source.id,source.version_id,source.sha256,offset]);
 return <Section title='固定状态快照'><View className='cp-pad'><Text className='cp-notice cp-paragraph'>这是读取时刻的历史证据。请核对 captured_at、时间窗和截断标记；实时状态请到服务状态页面核对。</Text><Notice error={error}/>{!view&&!error&&<Busy/>}{view&&<><Text className='cp-row-title'>{view.title}</Text><Text className='cp-paragraph' selectable>{view.text}</Text>{offset>0&&<Action secondary onClick={()=>setOffset(0)}>回到快照第一段</Action>}{view.next_offset!==null&&<Action secondary onClick={()=>setOffset(view.next_offset!)}>读取快照下一段</Action>}</>}<Action secondary onClick={onClose}>收起状态快照</Action></View></Section>;
}
