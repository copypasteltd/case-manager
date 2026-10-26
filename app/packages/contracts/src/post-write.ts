import type { components } from './api';
type S=components['schemas'];
export type PostWrite=S['AgentPostWriteInput'];
export const postTypes:Record<string,string>={project_log:'项目日志',meeting_note:'会议纪要',sop:'操作规范',article:'内部文章'};
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{64}$/.test(v);
const id=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v);
const equal=(a:unknown,b:unknown):boolean=>JSON.stringify(normalize(a))===JSON.stringify(normalize(b));
function normalize(v:unknown):unknown{if(Array.isArray(v))return v.map(normalize);if(v&&typeof v==='object')return Object.fromEntries(Object.entries(v).sort(([a],[b])=>a.localeCompare(b)).map(([k,x])=>[k,normalize(x)]));return v;}
const rank:Record<string,number>={internal:1,project_restricted:2,confidential:3};
function reason(v:string){v=v.trim();if(!v||[...v].length>200)throw new Error('请填写1至200字操作说明。');return v;}
function base(p:S['Post'],project:string|null){if(p.project_id!==project||!id(p.id)||!id(p.current_revision_id)||!hash(p.content_hash)||!Number.isSafeInteger(p.version)||p.version<1||p.state==='archived')throw new Error('请选择当前会话范围内的完整文档修订。');return {post_id:p.id,expected_version:p.version,base_revision_id:p.current_revision_id,content_hash:p.content_hash};}
export function makePostDraft(project:string|null,type:string,title:string,classification:string,refs:S['ResourceRef'][],why:string,p?:S['Post']):PostWrite{
 title=title.trim();if(!postTypes[type]||!title||[...title].length>200||!['internal','project_restricted'].includes(classification)||(!project&&classification!=='internal')||refs.length>20)throw new Error('请核对文档类型、标题、分类及最多20项附件。');
 const seen=new Set<string>();for(const r of refs){if(r.type!=='file'||!id(r.id)||!id(r.version_id)||seen.has(r.version_id!))throw new Error('知识附件须为不重复的准确正式文件版本。');seen.add(r.version_id!);}
 if(p&&(!p.can_edit||p.type!==type||p.classification!==classification))throw new Error('本次草稿只能修订可编辑的原文档，不能改变类型或分类。');
 return {tool:'draft_post',...(p?base(p,project):{}),type:type as S['AgentPostDraftInput']['type'],title,classification:classification as S['AgentPostDraftInput']['classification'],source_refs:refs,reason:reason(why)};
}
export function makePostPublish(p:S['Post'],project:string|null,why:string):PostWrite{
 if(!p.can_publish||!p.has_unpublished_changes||!p.body?.trim()||!['internal','project_restricted'].includes(p.classification??'')||p.source_refs?.some(r=>!id(r.version_id)))throw new Error('请选择有权内部发布、正文完整且附件已固定的未发布修订。');
 return {tool:'publish_post',...base(p,project),reason:reason(why)};
}
export function postWriteAccepted(run:S['Run'],input?:PostWrite):boolean{
 const v=run.post_write;if(!input)return !v;
 const comparable=(i:PostWrite)=>i.tool==='draft_post'?{...i,source_refs:i.source_refs??[]}:i;
 if(!v||run.mutation||run.file_update||run.file_create||run.oa_request||v.run_id!==run.id||!hash(v.intent_sha256)||!equal(comparable(input),comparable(v.input)))return false;
 return v.status==='authorized'?v.operation_id===null&&v.result===null:postWriteReceipt(run);
}
export function postWriteReceipt(run:S['Run']):boolean{
 const v=run.post_write,p=v?.result;if(!v||!p||v.run_id!==run.id||v.status!=='committed'||!id(v.operation_id)||!hash(v.intent_sha256)||!hash(p.content_hash)||!id(p.current_revision_id)||p.project_id!==v.before.scope.project_id||p.version!==(v.input.expected_version??0)+1||('post_id'in v.input&&v.input.post_id!==p.id))return false;
 if(v.input.tool==='publish_post')return p.state==='published'&&p.published_revision_id===p.current_revision_id&&p.current_revision_id===v.input.base_revision_id&&p.content_hash===v.input.content_hash;
 if(p.title!==v.input.title||p.type!==v.input.type||p.classification!==v.input.classification||!equal(p.source_refs??[],v.input.source_refs??[]))return false;
 return v.before.post?p.state===v.before.post.state&&p.published_revision_id===v.before.post.published_revision_id:p.state==='draft'&&p.published_revision_id===null&&p.author_id===run.actor_id;
}
export function postWriteScopeAllowed(classification:string,conversationClass:string):boolean{return !!rank[classification]&&rank[classification]<=rank[conversationClass];}
