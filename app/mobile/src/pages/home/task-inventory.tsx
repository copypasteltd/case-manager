import { useState } from 'react';
import { taskInventoryKinds, inventoryLabels, type InventoryKind } from '@copypaste/contracts/work-inventory';
import type { Schemas } from '../../platform/session';
import { Action, Section } from '../../ui';
import { CompleteInventory } from './inventory';
export function TaskInventory({ value }: { value: Schemas['Dashboard'] }) {
 const [selected,setSelected]=useState<InventoryKind>();
 const counts=[value.my_open_tasks,value.my_overdue_tasks,value.participating_tasks,value.unassigned_tasks];
 if(selected)return <CompleteInventory key={selected} kind={selected} onClose={()=>setSelected(undefined)}/>;
 return <Section title="任务完整待办">{taskInventoryKinds.map((kind,i)=><Action key={kind} secondary onClick={()=>setSelected(kind)}>{inventoryLabels[kind]} · {counts[i]}</Action>)}</Section>;
}
