import { useState } from 'react';
import { taskInventoryKinds, inventoryLabels, type InventoryKind } from '@copypaste/contracts/work-inventory';
import type { Me, Schemas } from './api';
import { WorkInventory } from './WorkInventory';
export function TaskWorkbench({ value, me }: { value: Schemas['Dashboard']; me: Me }) {
  const [selected, setSelected] = useState<InventoryKind>();
  const counts = [value.my_open_tasks, value.my_overdue_tasks, value.participating_tasks, value.unassigned_tasks];
  return <section className="section-block"><h2>任务完整待办</h2><div className="task-actions">{taskInventoryKinds.map((kind, i) => <button className="button secondary" key={kind} onClick={() => setSelected(kind)}>{inventoryLabels[kind]} · {counts[i]}</button>)}</div><p className="muted">未分配列表只显示当前可分配、可编辑的任务。参与记录不增加资料访问权限。</p>{selected && <WorkInventory kind={selected} me={me} onClose={() => setSelected(undefined)} />}</section>;
}
