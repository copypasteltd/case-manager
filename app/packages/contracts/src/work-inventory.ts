import type { components } from './api';
import { workbenchKinds, workbenchLabels, workbenchHints, workbenchState, workbenchURL, type WorkbenchItem } from './workbench';
import { followupKinds, followupLabels, followupHints, followupState, followupURL, type FollowupItem } from './followups';
type S = components['schemas'];
export type WorkInventory = S['WorkInventory'];
export type InventoryItem = S['WorkInventoryItem'];
export const taskInventoryKinds = ['my_tasks','my_overdue_tasks','participating_tasks','unassigned_tasks'] as const;
export const inventoryKinds = [...workbenchKinds, ...followupKinds, ...taskInventoryKinds] as const;
export type InventoryKind = typeof inventoryKinds[number];
export const inventoryLabels = { ...workbenchLabels, ...followupLabels, my_tasks:'我的未完成任务',my_overdue_tasks:'我的逾期任务',participating_tasks:'我参与的任务',unassigned_tasks:'可安排的未分配任务' };
export const inventoryHints = { ...workbenchHints, ...followupHints, my_tasks:'本人负责且当前可读的全部未完成任务。',my_overdue_tasks:'本人负责、当前可读且实际截止时刻已过的任务。',participating_tasks:'明确参与且当前可读的未完成任务；参与关系不授予权限。',unassigned_tasks:'当前可读且有权编辑和分配的未分配任务；打开原任务人工安排。' };
const uuid = (v:unknown):v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
const clock = (v:unknown):v is string => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(v) && Number.isFinite(Date.parse(v));
const invalid = () => new Error('完整待办列表尚未核实，请从首页重新读取。');
export function checkedInventory(v:WorkInventory,kind:InventoryKind):WorkInventory {
 if(!v || v.scope !== 'my_readable_inventory' || v.kind !== kind || !inventoryKinds.includes(kind) || !clock(v.as_of) || !/^[a-f0-9]{64}$/.test(v.basis_sha256) || !Number.isSafeInteger(v.count) || v.count<0 || v.count>5000 || !Array.isArray(v.items) || v.items.length>50 || v.items.length>v.count || v.count>0 && v.items.length===0 || v.next_cursor!==null && (typeof v.next_cursor!=='string' || !v.next_cursor || v.next_cursor.length>2048) || v.next_cursor!==null && v.count<=v.items.length) throw invalid();
 if(new Set(v.items.map(i=>i?.id?.toLowerCase())).size!==v.items.length) throw invalid();
 for(const i of v.items) {
  if(!i || !uuid(i.id) || !Number.isSafeInteger(i.version) || i.version<1 || i.revision_id!==null&&!uuid(i.revision_id) || i.related_id!==null&&!uuid(i.related_id) || i.project_id!==null&&!uuid(i.project_id) || !clock(i.updated_at) || typeof i.title!=='string' || !i.title.trim() || [...i.title].length>200) throw invalid();
  if(taskInventoryKinds.includes(kind as typeof taskInventoryKinds[number])) {
   if(!uuid(i.project_id)||i.revision_id!==null||i.related_id!==null||!['todo','in_progress','in_review','blocked'].includes(i.state)||i.due_on!==null&&!/^\d{4}-\d{2}-\d{2}$/.test(i.due_on))throw invalid();
  } else if(followupKinds.includes(kind as typeof followupKinds[number])) {
   if(i.related_id!==null||i.due_on!==null) throw invalid();
   if(kind==='requests_to_revise'&&(!uuid(i.revision_id)||!['draft','returned'].includes(i.state)))throw invalid();
   if(kind==='deliverables_to_submit'&&(!uuid(i.project_id)||!['draft','changes_requested'].includes(i.state)))throw invalid();
   if(kind==='deliverables_to_review'&&(!uuid(i.project_id)||!uuid(i.revision_id)||i.state!=='submitted'))throw invalid();
  } else {
   if(!uuid(i.revision_id)||i.project_id!==null)throw invalid();
   const states:Record<string,string[]>={approvals:['active'],contracts:['overdue','due_soon'],subscription_periods:['awaiting_confirmation'],incidents:['open','investigating','monitoring'],file_attention:['file_unavailable','replica_attention','replica_pending'],resource_attention:['resource_critical','resource_warning','resource_unconfirmed']};
   if(!states[kind]?.includes(i.state))throw invalid();
   if(kind==='contracts'||kind==='subscription_periods') {
    if(typeof i.due_on!=='string'||!/^\d{4}-\d{2}-\d{2}$/.test(i.due_on)||!Number.isFinite(Date.parse(i.due_on+'T00:00:00Z'))||new Date(i.due_on+'T00:00:00Z').toISOString().slice(0,10)!==i.due_on)throw invalid();
   } else if(i.due_on!==null)throw invalid();
   if((kind==='approvals'||kind==='subscription_periods')&&!uuid(i.related_id))throw invalid();
   if(['contracts','incidents','file_attention'].includes(kind)&&i.related_id!==null)throw invalid();
  }
 }
 return v;
}
export function mergeInventory(pages:WorkInventory[],kind:InventoryKind):InventoryItem[] {
 if(!pages.length)return [];
 const head=checkedInventory(pages[0],kind); const all:InventoryItem[]=[];
 for(let p=0;p<pages.length;p++) {const v=checkedInventory(pages[p],kind);if(v.basis_sha256!==head.basis_sha256||v.count!==head.count||p>0&&pages[p-1].next_cursor===null)throw invalid();all.push(...v.items);}
 if(new Set(all.map(i=>i.id.toLowerCase())).size!==all.length||all.length>head.count||pages.at(-1)!.next_cursor===null&&all.length!==head.count)throw invalid();
 for(let p=1;p<all.length;p++){const a=all[p-1],b=all[p];if((a.due_on??'9999-99-99')>(b.due_on??'9999-99-99')||a.due_on===b.due_on&&(Date.parse(a.updated_at)<Date.parse(b.updated_at)||Date.parse(a.updated_at)===Date.parse(b.updated_at)&&a.id.toLowerCase()<b.id.toLowerCase()))throw invalid();}
 return all;
}
export function inventoryURL(kind:InventoryKind,item:InventoryItem,client:'web'|'mobile'):string {
 if(taskInventoryKinds.includes(kind as typeof taskInventoryKinds[number])) { if(!uuid(item.id)||!uuid(item.project_id))throw invalid(); return (client==='web'?'/tasks/':'/pages/detail/index?kind=task&id=')+item.id; }
 if(followupKinds.includes(kind as typeof followupKinds[number]))return followupURL(kind as typeof followupKinds[number],item as FollowupItem,client);
 return workbenchURL(kind as typeof workbenchKinds[number],item as WorkbenchItem,client);
}
export function inventoryState(kind:InventoryKind,item:InventoryItem):string {
 if(taskInventoryKinds.includes(kind as typeof taskInventoryKinds[number]))return ({todo:'待开始',in_progress:'进行中',in_review:'待验收',blocked:'受阻'} as Record<string,string>)[item.state]??'请核对';
 return followupKinds.includes(kind as typeof followupKinds[number])?followupState(item as FollowupItem):workbenchState(item as WorkbenchItem);
}
