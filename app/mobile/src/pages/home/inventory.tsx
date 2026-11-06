import { useEffect, useRef, useState } from 'react';
import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { checkedInventory, mergeInventory, inventoryHints, inventoryLabels, inventoryState, inventoryURL, type InventoryKind, type WorkInventory } from '@copypaste/contracts/work-inventory';
import { Scope } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Notice, Row, Section } from '../../ui';
export function CompleteInventory({kind,onClose}:{kind:InventoryKind;onClose():void}) {
 const [pages,setPages]=useState<WorkInventory[]>([]),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(),[revision,setRevision]=useState(0);
 const owned=useRef<Scope>(),flight=useRef(false),last=pages[pages.length-1];
 useDidHide(()=>{owned.current?.close();setPages([]);});
 useEffect(()=>{const scope=new Scope();owned.current=scope;flight.current=false;setPages([]);setError(undefined);setLoading(true);
  void client.request<WorkInventory>('/dashboard/work-items/'+kind+'?limit=20',{scope}).then(v=>{if(!scope.closed)setPages([checkedInventory(v,kind)]);}).catch(e=>{if(!scope.closed)setError(e);}).finally(()=>{if(!scope.closed)setLoading(false);});
  const timer=setTimeout(()=>{scope.close();setPages([]);setError(new Error('请重新核对当前待办；旧列表已收起。'));},30000);
  return()=>{clearTimeout(timer);scope.close();};},[kind,revision,client.epoch]);
 async function more(){const scope=owned.current;if(!last?.next_cursor||!scope||scope.closed||flight.current)return;flight.current=true;setLoading(true);try{const v=checkedInventory(await client.request<WorkInventory>('/dashboard/work-items/'+kind+'?limit=20&cursor='+encodeURIComponent(last.next_cursor),{scope}),kind);const next=[...pages,v];mergeInventory(next,kind);if(!scope.closed)setPages(next);}catch(e){if(!scope.closed){setPages([]);setError(e);}}finally{if(!scope.closed){flight.current=false;setLoading(false);}}}
 let items:ReturnType<typeof mergeInventory>=[],fault=error;try{if(!fault&&!loading)items=mergeInventory(pages,kind);}catch(e){fault=e;}
 return <Section title={inventoryLabels[kind]+' · 完整列表'}><View className='cp-pad'><Text>{inventoryHints[kind]} 查阅不会结束业务待办。</Text><Notice error={fault}/>{loading?<Busy/>:!fault&&pages[0]&&<><Text>当前可见{pages[0].count}项 · 已列{items.length}项</Text>{items.length?items.map(i=><Row key={i.id} title={i.title} subtitle={inventoryState(kind,i)+(i.due_on?' · '+i.due_on:'')} onClick={()=>void Taro.navigateTo({url:inventoryURL(kind,i,'mobile')})}/>):<Empty>当前没有符合条件的事项。</Empty>}</>}{last?.next_cursor&&!fault&&<Action secondary disabled={loading} onClick={()=>void more()}>继续读取后续事项</Action>}<Action secondary disabled={loading} onClick={()=>setRevision(v=>v+1)}>从首页重新核对</Action><Action secondary onClick={onClose}>关闭完整列表</Action></View></Section>;
}
