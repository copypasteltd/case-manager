import type { components } from './api';
type S=components['schemas'];type Entry=S['AgentOrganizationEntry'];type Action=S['AgentOrganizationAction'];
const uuid=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
const invalid=()=>new Error('请选择准确资料、目录和名称，核对同一项目及分类后再加入整理。');
export function organizationAction(kind:Action['kind'],source:Entry|undefined,target:Entry|undefined,name:string):Action {
 if(!source||!uuid(source.id)||!Number.isSafeInteger(source.version)||source.version<1||!['folder.create','folder.move','file.move'].includes(kind)||typeof name!=='string'||[...name].length>200)throw invalid();
 if(kind==='folder.create'&&(source.kind!=='folder'||!name.trim()||target)||kind==='folder.move'&&source.kind!=='folder'||kind==='file.move'&&source.kind!=='file')throw invalid();
 if(target&&(!uuid(target.id)||target.kind!=='folder'||!Number.isSafeInteger(target.version)||target.version<1||target.id===source.id||target.project_id!==source.project_id||target.classification!==source.classification))throw invalid();
 if(kind!=='folder.create'&&!target&&(!name.trim()||name===source.name))throw invalid();
 return {kind,id:source.id,expected_version:source.version,...(name.trim()&&(kind==='folder.create'||name!==source.name)?{name}:{}),...(kind!=='folder.create'&&target?{target_folder_id:target.id,expected_target_version:target.version}:{})};
}
export function organizationQueue(actions:Action[],next:Action):Action[] {
 if(actions.length>=10||actions.some(a=>a.id===next.id&&(a.kind!=='folder.create'||next.kind!=='folder.create'||a.name===next.name)))throw new Error('每份最多10项，同一资料或同名新目录不能重复加入。');
 if(actions.some(a=>a.kind==='folder.move'&&(next.id===a.id||next.target_folder_id===a.id)||next.kind==='folder.move'&&(a.id===next.id||a.target_folder_id===next.id)))throw new Error('本批整理的目录与其他动作重叠，请分开拟定。');
 return [...actions,next];
}
const canonical=(v:unknown):string=>JSON.stringify(v,(_k,x)=>x&&typeof x==='object'&&!Array.isArray(x)?Object.fromEntries(Object.keys(x).sort().map(k=>[k,x[k]])):x);
export function organizationReceipt(v:S['AgentOrganizationPlan'],c:S['Conversation'],body:S['AgentOrganizationPlanCreate'],actor:string):boolean {
 return !!v&&uuid(v.id)&&v.version===1&&v.conversation_id===c.id&&v.conversation_revision_id===c.current_revision_id&&v.context_security_version===body.context_security_version&&v.actor_id===actor&&v.project_id===c.project_id&&v.reason===body.reason&&v.state==='proposed'&&v.operation_id===null&&Array.isArray(v.results)&&v.results.length===0&&/^[a-f0-9]{64}$/.test(v.plan_hash)&&Array.isArray(v.preview)&&v.preview.length===body.actions.length&&v.preview.every((p,i)=>canonical(p.action)===canonical(body.actions[i]));
}
