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

export type SkillReleaseDetails = S['AgentSkillReleaseDetails'];
export type SkillReleaseInput = S['AgentSkillReleaseInput'];
export const releaseStages = {pilot:'试运行',published:'已人工评估发布',disabled:'已暂停执行'} as const;
export const releaseNotice = '发布记录绑定已安装的准确Skill摘要与本人评估运行。暂停会阻止后续执行核对，不代表进程已退出。回退须先由部署身份安装原已发布代码，再选择原发布记录；不恢复已停用配置，不撤销业务事实。';
export function skillReleaseInput(v:SkillReleaseDetails, stage:SkillReleaseInput['stage'], runs:string, rollback:string, reason:string, reviewed:boolean):SkillReleaseInput {
 if(!reviewed||v.profile.item.state!=='active')throw new Error('请核对当前活动配置及任务影响。');
 const ids=runs.split(/[\s,，]+/).filter(Boolean), uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
 if(ids.length>20||new Set(ids).size!==ids.length||ids.some(id=>!uuid.test(id))||stage==='published'&&!ids.length)throw new Error('发布需填写本人此准确配置已成功完成的评估运行，最多20项。');
 if(rollback&&!uuid.test(rollback))throw new Error('请选择准确原发布记录。');
 if(!reason.trim()||[...reason.trim()].length>200)throw new Error('请填写不超过200字的决定原因。');
 return {expected_version:v.version,profile_version:v.profile.item.version,skill_sha256:v.profile.item.skill_sha256,active_basis_sha256:v.profile.impact.active_basis_sha256,stage,assessment_run_ids:ids,...(rollback?{rollback_release_id:rollback}:{}),reviewed:true,reason:reason.trim()};
}

export type TelegramHandoff = S['TelegramHandoffInput'];
export type TelegramHandoffRequest = S['TelegramHandoffRequest'];
export function telegramHandoffInput(v:TelegramHandoffRequest,project:string|null,skill:string,reviewed:boolean):TelegramHandoff {
 if(!reviewed||!v.eligible||v.run_id||v.project_id!==project||v.skill_id!==skill||Date.parse(v.expires_at)<=Date.now())throw new Error('请核对当前可授权请求、相同项目范围及准确任务。');
 return {request_id:v.id,expected_version:v.version,request_sha256:v.request_sha256,reviewed:true};
}
export function telegramHandoffReceipt(run:S['Run'],input?:TelegramHandoff):boolean {
 if(!input)return !run.telegram_handoff;
 const v=run.telegram_handoff;return !!v&&v.request_id===input.request_id&&v.expected_version===input.expected_version&&v.request_sha256===input.request_sha256&&v.reviewed;
}
