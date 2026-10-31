import type { components } from './api';
type S = components['schemas'];
export type Profile = S['AgentProfileOperation'];
export type Details = S['AgentProfileDetails'];
export type Retire = S['AgentProfileRetire'];
export const skills = { 'project-create': '新建项目', 'project-update': '更新项目', 'file-update': '文件更新', 'information-organize': '信息整理', 'oa-assist': 'OA辅助', 'subscription-review': '订阅审阅', 'project-briefing': '项目简报', 'incident-summary': '故障总结' } as const;
export const states = { active: '可供任务选择', retired: '已停用' } as const;
export const sources = { installed: '实际安装记录', retired: '实际停用记录', retained_baseline: '启用台账时的现状快照' } as const;
export const retirementNotice = '停用后不再接受此配置的新任务，现有任务在后续权限、续租或提交核对时停止或进入待核对；停用不代表进程已退出，已提交的业务事实不会回滚。此配置不能重新启用，后续需另行安装经过核验的新配置。';
export function runtimePath(state = '', skill = '') {
  return '/agent/runtime-profiles?' + [['limit', '20'], ['state', state], ['skill_id', skill]].filter(([, v]) => v).map(([k, v]) => k + '=' + encodeURIComponent(v)).join('&');
}
export function retireInput(v: Details, note: string, reviewed: boolean): Retire {
  if (v.item.state !== 'active' || !reviewed) throw new Error('请核对当前配置及活动任务的停用影响。');
  if (!note.trim() || [...note.trim()].length > 200) throw new Error('请填写不超过200字的停用原因。');
  return { expected_version: v.item.version, active_basis_sha256: v.impact.active_basis_sha256, reviewed_impact: true, reason: note.trim() };
}
