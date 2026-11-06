import { useInfiniteQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { checkedInventory, mergeInventory, inventoryHints, inventoryLabels, inventoryState, inventoryURL, type InventoryKind, type WorkInventory as Inventory } from '@copypaste/contracts/work-inventory';
import { api, type Me } from './api';
import { Busy, Dialog, Empty, ErrorNotice, date } from './components';
export function WorkInventory({kind,me,onClose}:{kind:InventoryKind;me:Me;onClose():void}) {
 const q=useInfiniteQuery({queryKey:['complete-work',kind,me.user.id,me.authorization_version],initialPageParam:'',queryFn:({pageParam,signal})=>api<Inventory>('/dashboard/work-items/'+kind+'?limit=20'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}).then(v=>checkedInventory(v,kind)),getNextPageParam:v=>v.next_cursor??undefined,gcTime:0,retry:false,refetchInterval:30000,refetchOnWindowFocus:'always'});
 let items:ReturnType<typeof mergeInventory>=[],failure:unknown=q.error;
 if(!failure&&!q.isFetching&&q.data){try{items=mergeInventory(q.data.pages,kind);}catch(e){failure=e;}}
 const head=q.data?.pages[0];
 return <Dialog title={inventoryLabels[kind]+' · 完整列表'} onClose={onClose}><p className="form-note">{inventoryHints[kind]} 列表变化时请从首页重新读取；查阅不会结束业务待办。</p><ErrorNotice error={failure}/>{q.isFetching?<Busy/>:!failure&&head&&<><p>当前可见 {head.count} 项 · 已列 {items.length} 项 · 截至 {date(head.as_of)}</p>{items.length?items.map(i=><Link className="todo-row" key={i.id} to={inventoryURL(kind,i,'web')} onClick={onClose}><div><strong>{i.title}</strong><small>{inventoryState(kind,i)}{i.due_on?' · '+i.due_on:''}</small></div><span aria-hidden="true">↗</span></Link>):<Empty title="当前没有符合条件的事项" detail="请按当前权限在各业务模块继续查阅。"/>}</>}{q.hasNextPage&&!failure&&<button className="button secondary" disabled={q.isFetching} onClick={()=>void q.fetchNextPage()}>继续读取后续事项</button>}<button className="button secondary" disabled={q.isFetching} onClick={()=>void q.refetch({cancelRefetch:true})}>重新核对当前列表</button>{Boolean(failure)&&<p className="notice neutral">请关闭后重新打开此分组，以当前权限和业务依据从首页读取。</p>}</Dialog>;
}
