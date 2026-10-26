import { describe, expect, it } from 'vitest';
import type { Task } from './api';
import { distinctTasks, projectSchedule, scheduleTime, taskView } from './projectSchedule';

const task = (id: string, start?: string, due?: string, version = 1) => ({ id, title: id, version, start_at: start, due_at: due, state: 'todo' } as Task);
describe('Project task presentation facts', () => {
  it('does not duplicate a moved keyset item or overwrite an observed newer revision', () => {
    const old = task('a', undefined, undefined, 1), current: Task = { ...old, version: 3, state: 'blocked' }, middle = { ...old, version: 2 };
    expect(distinctTasks([{ items: [old, task('b')] }, { items: [current] }, { items: [middle] }])).toEqual([current, task('b')]);
  });
  it('accepts only supported public view selectors', () => {
    expect([null, '', 'unknown', '<script>'].map(taskView)).toEqual(['list', 'list', 'list', 'list']);
    expect(taskView('board')).toBe('board'); expect(taskView('timeline')).toBe('timeline');
  });
});
describe('Project schedule evidence', () => {
  it('keeps an entirely undated project without invented dates', () => {
    const result = projectSchedule([task('a'), task('b')]);
    expect(result.min).toBeNull(); expect(result.max).toBeNull(); expect(result.rows.map(row => row.kind)).toEqual(['undated', 'undated']);
  });
  it('represents a missing start or due as an explicitly labelled point', () => {
    const result = projectSchedule([task('start', '2026-10-01T12:00:00Z'), task('due', undefined, '2026-10-02T12:00:00Z')]);
    expect(result.rows.map(row => row.kind)).toEqual(['start-only', 'due-only']);
    expect(result.rows.every(row => row.width === 0 && row.left > 0 && row.left < 100)).toBe(true);
  });
  it('renders a zero-duration interval as a point with a finite padded domain', () => {
    const result = projectSchedule([task('a', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z')]);
    expect(result.rows[0].kind).toBe('point'); expect(result.rows[0].left).toBe(50); expect(result.max! - result.min!).toBe(7_200_000);
  });
  it('quarantines reversed and malformed dates from the domain', () => {
    const result = projectSchedule([task('reversed', '2026-10-02T00:00:00Z', '2026-10-01T00:00:00Z'), task('bad', 'invalid')]);
    expect(result.min).toBeNull(); expect(result.rows.every(row => row.kind === 'invalid' && row.width === 0)).toBe(true);
  });
  it('preserves exact instants across offsets and a Hong Kong midnight', () => {
    const result = projectSchedule([task('a', '2026-10-25T23:30:00+08:00', '2026-10-25T16:30:00Z')]);
    expect(result.rows[0].due! - result.rows[0].start!).toBe(3_600_000);
    expect(scheduleTime('2026-10-25T16:30:00Z')).toContain('26/10/2026');
    expect(scheduleTime('2026-10-25T16:30:00Z')).toContain('00:30');
  });
  it('sorts by available due/start while keeping undated tasks separate from real deadlines', () => {
    const result = projectSchedule([task('undated'), task('late', undefined, '2026-12-02T00:00:00Z'), task('early', '2026-10-01T00:00:00Z')]);
    expect(result.rows.map(row => row.task.id)).toEqual(['early', 'late', 'undated']);
  });
  it('keeps multi-century schedules bounded without generating a calendar grid', () => {
    const result = projectSchedule([task('long', '1800-01-01T00:00:00Z', '2500-01-01T00:00:00Z'), task('near', undefined, '2026-10-01T00:00:00Z')]);
    expect(result.rows.every(row => Number.isFinite(row.left) && row.left >= 0 && row.left + row.width <= 100)).toBe(true);
    expect(result.rows.find(row => row.task.id === 'long')!.width).toBeGreaterThan(90);
  });
  it('does not treat completion or cancellation as new planned dates', () => {
    const result = projectSchedule([{ ...task('done'), state: 'done' }, { ...task('cancelled'), state: 'cancelled' }]);
    expect(result.rows.map(row => row.kind)).toEqual(['undated', 'undated']); expect(result.min).toBeNull();
  });
  it('labels unknown and invalid instants instead of throwing or inventing a time', () => {
    expect(scheduleTime(null)).toBe('未设置'); expect(scheduleTime('invalid')).toBe('日期需核对');
    expect(scheduleTime(Number.NaN)).toBe('日期需核对'); expect(scheduleTime(Infinity)).toBe('日期需核对');
  });
});
