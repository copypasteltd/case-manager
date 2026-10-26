import type { Task } from './api';

export const taskLanes = [
  { key: 'todo', name: '待开始' }, { key: 'in_progress', name: '进行中' },
  { key: 'in_review', name: '待验收' }, { key: 'blocked', name: '受阻' },
  { key: 'done', name: '已完成' }, { key: 'cancelled', name: '已取消' },
] as const;
export type TaskView = 'list' | 'board' | 'timeline';
export function taskView(value: string | null): TaskView {
  return value === 'board' || value === 'timeline' ? value : 'list';
}

// A keyset page can overlap earlier pages when another employee edits a task.
// Keep one row per identity and the highest actually observed record version.
export function distinctTasks(pages: readonly { items: Task[] }[]): Task[] {
  const tasks = new Map<string, Task>();
  for (const page of pages) for (const task of page.items) {
    const previous = tasks.get(task.id);
    if (!previous || task.version > previous.version) tasks.set(task.id, task);
  }
  return [...tasks.values()];
}

export type ScheduleKind = 'range' | 'point' | 'start-only' | 'due-only' | 'undated' | 'invalid';
export type ScheduleRow = { task: Task; kind: ScheduleKind; start: number | null; due: number | null; left: number; width: number };
function instant(value?: string | null): number | null {
  if (!value) return null;
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : null;
}
export function projectSchedule(tasks: readonly Task[]) {
  const rows: ScheduleRow[] = tasks.map(task => {
    const start = instant(task.start_at), due = instant(task.due_at);
    const invalid = (!!task.start_at && start === null) || (!!task.due_at && due === null) || (start !== null && due !== null && due < start);
    const kind: ScheduleKind = invalid ? 'invalid' : start !== null && due !== null ? (start === due ? 'point' : 'range') : start !== null ? 'start-only' : due !== null ? 'due-only' : 'undated';
    return { task, kind, start, due, left: 0, width: 0 };
  });
  const dated = rows.filter(row => row.kind !== 'invalid' && row.kind !== 'undated');
  if (!dated.length) return { rows, min: null, max: null };
  // Avoid spreading an unbounded list into Math.min/max or creating one DOM
  // column per calendar day. Long projects still have one proportional track.
  let min = Infinity, max = -Infinity;
  for (const row of dated) {
    min = Math.min(min, row.start ?? row.due!);
    max = Math.max(max, row.due ?? row.start!);
  }
  const padding = Math.max((max - min) * .04, 3_600_000);
  min = Math.max(-8_640_000_000_000_000, min - padding);
  max = Math.min(8_640_000_000_000_000, max + padding);
  for (const row of dated) {
    row.left = Math.max(0, Math.min(100, ((row.start ?? row.due!) - min) / (max - min) * 100));
    row.width = row.kind === 'range' ? Math.max(0, Math.min(100 - row.left, (row.due! - row.start!) / (max - min) * 100)) : 0;
  }
  rows.sort((a, b) => {
    const aTime = a.kind === 'invalid' || a.kind === 'undated' ? Infinity : a.due ?? a.start!;
    const bTime = b.kind === 'invalid' || b.kind === 'undated' ? Infinity : b.due ?? b.start!;
    return (aTime === bTime ? 0 : aTime - bTime) || a.task.title.localeCompare(b.task.title, 'zh-HK') || a.task.id.localeCompare(b.task.id);
  });
  return { rows, min, max };
}

export function scheduleTime(value?: string | number | null): string {
  const time = typeof value === 'number' ? value : instant(value);
  if (time === null) return value ? '日期需核对' : '未设置';
  if (!Number.isFinite(time) || Math.abs(time) > 8_640_000_000_000_000) return '日期需核对';
  return new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(time);
}
