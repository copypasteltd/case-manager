import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';
import { copyFile, cp, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { loadSkills, KNOWN_TOOLS } from './skills.js';
import { verifyResult, type ProposedResult, type ExecutionReceipt } from './evidence.js';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const catalog = await loadSkills(root), id = randomUUID(), version = randomUUID();
const baseResult = (): ProposedResult => ({ status: 'completed', summary: '来源与业务执行结果已核对。', resource_refs: [], source_refs: [], operation_ids: [], missing_fields: [], next_action: '' });

test('contract-body creation accepts exact draft context but never grants human adoption tools', () => {
  const skill = catalog.get('file-update')!;
  const input = { folder_id: randomUUID(), expected_folder_version: 2, name: 'contract-body.md', media_type: 'text/markdown', instructions: 'Draft the complete body from exact facts; human adoption remains separate', contract_body: { contract_id: randomUUID(), expected_version: 3, base_revision_id: randomUUID(), content_hash: 'a'.repeat(64), reason: 'Use this exact draft' } };
  skill.validateInput(input);
  assert.equal(skill.manifest.version, '1.4.0');
  assert.equal(skill.manifest.state, 'draft');
  for (const field of ['contract_id', 'expected_version', 'base_revision_id', 'content_hash', 'reason']) {
    const changed = structuredClone(input); delete (changed.contract_body as Record<string, unknown>)[field];
    assert.throws(() => skill.validateInput(changed), /SKILL_INPUT_INVALID/);
  }
  for (const tool of ['adopt_contract_body', 'sign_contract', 'activate_contract', 'approve_request']) assert.throws(() => skill.authorizeTool(tool), /SKILL_TOOL_DENIED/);
});

test('deliverable draft creation requires all parent fields and a real current-run creation receipt',()=>{
 const skill=catalog.get('project-update')!,run={id:randomUUID(),authority_stamp:'deliverable-create'};
 const input={instructions:'Create exactly this reviewed draft',mutation:{tool:'create_deliverable',arguments:{id:randomUUID(),expected_version:7,name:'New delivery',acceptance_criteria:'Check fixed evidence',owner_id:randomUUID(),milestone_id:null,reason:'Reviewed draft creation'}}};
 skill.validateInput(input);skill.authorizeTool('create_deliverable');
 for(const field of ['name','acceptance_criteria','owner_id','milestone_id']){const altered=structuredClone(input);delete (altered.mutation.arguments as Record<string,unknown>)[field];assert.throws(()=>skill.validateInput(altered),/SKILL_INPUT_INVALID/);}
 const op=randomUUID(),ref={type:'deliverable',id:randomUUID()},source={type:'agent_mutation',id:run.id};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'create_deliverable',status:'succeeded',committed:true,operation_id:op,resource_refs:[ref],source_refs:[source]};
 const result={...baseResult(),resource_refs:[ref],source_refs:[source],operation_ids:[op]};
 assert.equal(verifyResult(skill,run,result,[receipt]).status,'completed');
 for(const altered of [{...receipt,run_id:randomUUID()},{...receipt,committed:false},{...receipt,operation_id:null}])assert.throws(()=>verifyResult(skill,run,result,[altered]),/RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
});

test('deliverable updates require every reviewed metadata field and genuine write evidence',()=>{
 const skill=catalog.get('project-update')!,run={id:randomUUID(),authority_stamp:'deliverable-update'};
 const input={instructions:'Use only the complete reviewed metadata',mutation:{tool:'update_deliverable',arguments:{id:randomUUID(),expected_version:3,name:'Delivery',acceptance_criteria:'Check actual results',owner_id:randomUUID(),milestone_id:null,reason:'Explicit unlink'}}};
 skill.validateInput(input);skill.authorizeTool('update_deliverable');
 for(const field of ['name','acceptance_criteria','owner_id','milestone_id']){const altered=structuredClone(input);delete (altered.mutation.arguments as Record<string,unknown>)[field];assert.throws(()=>skill.validateInput(altered),/SKILL_INPUT_INVALID/);}
 const op=randomUUID(),ref={type:'deliverable',id:input.mutation.arguments.id},source={type:'agent_mutation',id:run.id};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'update_deliverable',status:'succeeded',committed:true,operation_id:op,resource_refs:[ref],source_refs:[source]};
 const result={...baseResult(),resource_refs:[ref],source_refs:[source],operation_ids:[op]};
 assert.equal(verifyResult(skill,run,result,[receipt]).status,'completed');
 assert.throws(()=>verifyResult(skill,run,result,[{...receipt,committed:false}]),/RESULT_(OPERATION|COMPLETION)_UNVERIFIED/);
});

test('milestone creation requires all fixed fields and a committed current-run receipt',()=>{
 const skill=catalog.get('project-update')!,run={id:randomUUID(),authority_stamp:'milestone-create'};
 const input={instructions:'Create only the explicitly selected milestone',mutation:{tool:'create_milestone',arguments:{id:randomUUID(),expected_version:7,reason:'Reviewed creation',title:'Release',owner_id:randomUUID(),due_at:'2027-03-05T18:00:00+08:00'}}};
 skill.validateInput(input);skill.authorizeTool('create_milestone');
 for(const field of ['title','owner_id','due_at']){const bad=structuredClone(input);delete (bad.mutation.arguments as Record<string,unknown>)[field];assert.throws(()=>skill.validateInput(bad),/SKILL_INPUT_INVALID/);}
 const op=randomUUID(),ref={type:'milestone',id:randomUUID()},source={type:'agent_mutation',id:run.id};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'create_milestone',status:'succeeded',committed:true,operation_id:op,resource_refs:[ref],source_refs:[source]};
 const result={...baseResult(),resource_refs:[ref],source_refs:[source],operation_ids:[op]};
 assert.equal(verifyResult(skill,run,result,[receipt]).status,'completed');
 for(const altered of [{...receipt,run_id:randomUUID()},{...receipt,committed:false},{...receipt,operation_id:null}])assert.throws(()=>verifyResult(skill,run,result,[altered]),/RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
});

test('monthly finance completion requires a cited successful finance snapshot from this run',()=>{
 const skill=catalog.get('subscription-review')!,run={id:randomUUID(),authority_stamp:'monthly-finance'};
 const snapshot={type:'agent_finance_report',id:randomUUID()},value:ProposedResult={...baseResult(),source_refs:[snapshot]};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'read_finance_summary',status:'succeeded',committed:false,operation_id:null,resource_refs:[],source_refs:[snapshot]};
 assert.equal(verifyResult(skill,run,value,[receipt]).status,'completed');
 for(const altered of [{...receipt,tool:'read_resource'},{...receipt,status:'pending' as const},{...receipt,run_id:randomUUID()},{...receipt,source_refs:[]}])assert.throws(()=>verifyResult(skill,run,value,[altered]),/RESULT_(READ|REFERENCE)_UNVERIFIED/);
 assert.throws(()=>verifyResult(skill,run,{...value,missing_fields:['缺少月份']},[receipt]),/RESULT_COMPLETION_INCOMPLETE/);
});

test('knowledge writes require real current-run commits while ordinary briefings remain read-only',()=>{
 const skill=catalog.get('project-briefing')!,run={id:randomUUID(),authority_stamp:'post-authority',prepared_context:JSON.stringify({authorized_post_write:{input:{tool:'draft_post'}}})};
 const source={type:'agent_post_write',id:run.id},post={type:'post',id:randomUUID(),version_id:randomUUID()},op=randomUUID();
 const value:ProposedResult={...baseResult(),resource_refs:[post],source_refs:[source],operation_ids:[op]};
 for(const tool of ['draft_post','publish_post']){
  const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool,status:'succeeded',committed:true,operation_id:op,resource_refs:[post],source_refs:[source]};
  skill.validateOutput(value);assert.equal(verifyResult(skill,run,value,[receipt]).status,'completed');
  assert.throws(()=>verifyResult(skill,run,value,[{...receipt,committed:false,operation_id:null}]),/RESULT_(OPERATION|COMPLETION)_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,value,[{...receipt,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
 }
 const read:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'read_resource',status:'succeeded',committed:false,operation_id:null,resource_refs:[],source_refs:[source]};
 assert.throws(()=>verifyResult(skill,run,{...baseResult(),source_refs:[source]},[read]),/RESULT_COMPLETION_UNVERIFIED/);
});

test('OA completion requires the exact current-run saved draft or real submission receipt',()=>{
 const skill=catalog.get('oa-assist')!,run={id:randomUUID(),authority_stamp:'oa-authority'};
 const source={type:'agent_oa_request',id:run.id},request={type:'request',id:randomUUID()},op=randomUUID();
 const value:ProposedResult={...baseResult(),resource_refs:[request],source_refs:[source],operation_ids:[op]};
 for(const tool of ['draft_request','submit_request']){
  const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool,status:'succeeded',committed:true,operation_id:op,resource_refs:[request],source_refs:[source]};
  skill.validateOutput(value);assert.equal(verifyResult(skill,run,value,[receipt]).status,'completed');
  assert.throws(()=>verifyResult(skill,run,value,[{...receipt,committed:false,operation_id:null}]),/RESULT_(OPERATION|COMPLETION)_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,value,[{...receipt,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
 }
 assert.throws(()=>verifyResult(skill,run,value,[]),/RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
});

test('new text files require exact current-run formal creation evidence; staging and proposed content stay pending',()=>{
  const skill=catalog.get('file-update')!,run={id:randomUUID(),authority_stamp:'creation-authority'};
  skill.validateInput({folder_id:randomUUID(),expected_folder_version:3,name:'会议纪要.md',media_type:'text/markdown',instructions:'新建并记录已核实事项。'});
  const intent={type:'agent_file_create',id:run.id},file={type:'file',id:randomUUID(),version_id:randomUUID()},operation=randomUUID();
  const value:ProposedResult={...baseResult(),resource_refs:[file],source_refs:[intent],operation_ids:[operation]};
  const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'commit_file_create',status:'succeeded',committed:true,operation_id:operation,resource_refs:[file],source_refs:[intent]};
  skill.validateOutput(value);assert.equal(verifyResult(skill,run,value,[receipt]).status,'completed');
  for(const tool of ['recover_file_draft','stage_file_create','propose_file_create','inspect_operation'])assert.throws(()=>verifyResult(skill,run,value,[{...receipt,tool,committed:false,operation_id:null}]),/RESULT_(OPERATION|COMPLETION)_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,value,[{...receipt,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
});

test('organization completion requires its exact current-run commit receipt rather than a proposed plan', () => {
  const skill=catalog.get('information-organize')!,run={id:randomUUID(),authority_stamp:'organization-authority'};
  const plan={type:'agent_organization_plan',id:randomUUID()},folder={type:'folder',id:randomUUID()},operation=randomUUID();
  const value:ProposedResult={...baseResult(),source_refs:[plan],resource_refs:[folder],operation_ids:[operation]};
  const committed:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'commit_organization',status:'succeeded',committed:true,operation_id:operation,source_refs:[plan],resource_refs:[folder]};
  skill.validateOutput(value);assert.equal(verifyResult(skill,run,value,[committed]).status,'completed');
  for(const tool of ['propose_organization','read_resource','inspect_operation']) assert.throws(()=>verifyResult(skill,run,value,[{...committed,tool,committed:false,status:'pending',operation_id:null}]),/RESULT_(OPERATION|COMPLETION)_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,value,[{...committed,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
});

test('conversation requests accept partial business intent without accepting caller identity paths or grants', () => {
  for (const skill of catalog.values()) {
    skill.validateInput({ request: '请整理当前项目；缺少必要资料时说明缺项。', context_project_id: id });
    for (const extra of [{ actor_id: id }, { approved: true }, { task_credential: 'private' }, { local_path: 'C:/private' }]) assert.throws(() => skill.validateInput({ request: '整理资料', ...extra }), /SKILL_INPUT_INVALID/);
    assert.throws(() => skill.validateInput({ request: '' }), /SKILL_INPUT_INVALID/);
  }
});

test('all eight bounded packages load while identity injection and unsupported operations are rejected', () => {
  assert.equal(catalog.size, 8); const skill = catalog.get('file-update')!;
  const input = { file_id: id, base_version_id: version, format: 'text/markdown', instructions: '更正已核实的项目日期。' };
  skill.validateInput(input);
  for (const extra of [{ actor_id: id }, { local_path: 'C:/private.env' }, { object_key: 'private/object' }, { url: 'https://invalid.example' }]) assert.throws(() => skill.validateInput({ ...input, ...extra }), /SKILL_INPUT_INVALID/);
  assert.throws(() => skill.validateInput({ ...input, format: 'application/pdf' }), /SKILL_INPUT_INVALID/);
  assert.throws(() => skill.authorizeTool('decide_approval'), /SKILL_TOOL_DENIED/);
  assert.throws(() => skill.authorizeTool('send_authorized_notification'), /SKILL_TOOL_DENIED/);
  assert.throws(() => skill.assertRunnable(skill.sha256, KNOWN_TOOLS), /SKILL_NOT_PUBLISHED/);
});

test('bounded report windows reject impossible dates and reversed ranges; native output schemas contain no external references', () => {
  const skill = catalog.get('project-briefing')!, input = { project_id: id, window: { from: '2026-10-01T00:00:00Z', to: '2026-11-01T00:00:00Z' } };
  skill.validateInput(input);
  for (const from of ['2026-02-30T00:00:00Z', '2026-10-01T24:00:00Z', '2026-10-01T00:00:00+25:00']) assert.throws(() => skill.validateInput({ ...input, window: { ...input.window, from } }), /SKILL_INPUT_INVALID/);
  assert.throws(() => skill.validateInput({ ...input, window: { from: input.window.to, to: input.window.from } }), /SKILL_WINDOW_INVALID/);
  assert.throws(() => skill.validateInput({ ...input, window: { from: input.window.to, to: input.window.to } }), /SKILL_WINDOW_INVALID/);
  for (const item of catalog.values()) { assert.equal(item.modelOutputSchema.type, 'object'); assert.ok(!JSON.stringify(item.modelOutputSchema).includes('"$ref"')); }
});

test('formal completion needs the current run and current authority ledger evidence', () => {
  const skill = catalog.get('file-update')!, run = { id: randomUUID(), authority_stamp: 'current-authority' }, operation = randomUUID();
  const value = baseResult(); value.operation_ids = [operation]; value.resource_refs = [{ type: 'file', id, version_id: version }];
  const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool: 'commit_file_update', operation_id: operation, status: 'succeeded', committed: true, resource_refs: value.resource_refs, source_refs: [] };
  assert.equal(verifyResult(skill, run, value, [receipt]).status, 'completed');
  assert.throws(() => verifyResult(skill, run, value, [{ ...receipt, run_id: randomUUID() }]), /UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, value, [{ ...receipt, authority_stamp: 'revoked-authority' }]), /UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, value, [{ ...receipt, committed: false }]), /UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, { ...value, operation_ids: [randomUUID()] }, [receipt]), /UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, { ...value, resource_refs: [{ type: 'file', id, version_id: randomUUID() }] }, [receipt]), /UNVERIFIED/);
  const noCommit = baseResult(); assert.throws(() => verifyResult(skill, run, noCommit, []), /RESULT_COMPLETION_UNVERIFIED/);
});

test('a staged file draft and its fixed intent remain pending without a formal version receipt', () => {
  const skill = catalog.get('file-update')!, run = { id: randomUUID(), authority_stamp: 'file-draft-authority' };
  const intent = { type: 'agent_file_update', id: run.id }, draft = { type: 'agent_file_draft', id: randomUUID() };
  const value: ProposedResult = { ...baseResult(), status: 'pending', resource_refs: [draft], source_refs: [intent], next_action: '核对后台扫描与准确草稿计划，正式版本尚未提交。' };
  const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool: 'create_file_draft', operation_id: null, status: 'succeeded', committed: false, resource_refs: [draft], source_refs: [intent] };
  assert.equal(verifyResult(skill, run, value, [receipt]).status, 'pending');
  assert.throws(() => verifyResult(skill, run, { ...value, status: 'completed' }, [receipt]), /RESULT_COMPLETION_UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, value, [{ ...receipt, source_refs: [{ ...intent, id: randomUUID() }] }]), /UNVERIFIED/);
});

test('a pending project proposal cites its exact intent without claiming a committed operation', () => {
  const skill=catalog.get('project-create')!,run={id:randomUUID(),authority_stamp:'plan-authority'},plan={type:'agent_plan',id:randomUUID()};
  const value:ProposedResult={...baseResult(),status:'pending',resource_refs:[plan],source_refs:[plan],next_action:'请本人核对并批准这份计划。'};
  const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'propose_project',operation_id:null,status:'pending',committed:false,resource_refs:[plan],source_refs:[plan]};
  assert.equal(verifyResult(skill,run,value,[receipt]).status,'pending');
  assert.throws(()=>verifyResult(skill,run,{...value,status:'completed'},[receipt]),/RESULT_COMPLETION_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,{...value,operation_ids:[randomUUID()]},[receipt]),/RESULT_OPERATION_UNVERIFIED/);
});

test('successful plan discovery proves only observation and cannot claim a created project', () => {
  const skill=catalog.get('project-create')!,run={id:randomUUID(),authority_stamp:'discovery-authority'},plan={type:'agent_plan',id:randomUUID()};
  const value:ProposedResult={...baseResult(),status:'pending',resource_refs:[plan],source_refs:[plan],next_action:'请核对准确计划及当前批准状态。'};
  for(const tool of ['search_resources','read_resource','inspect_operation']){
    const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool,operation_id:null,status:'succeeded',committed:false,resource_refs:[],source_refs:[plan]};
    assert.equal(verifyResult(skill,run,value,[receipt]).status,'pending');
    assert.throws(()=>verifyResult(skill,run,{...value,status:'completed'},[receipt]),/RESULT_COMPLETION_UNVERIFIED/);
    assert.throws(()=>verifyResult(skill,run,{...value,operation_ids:[randomUUID()]},[receipt]),/RESULT_OPERATION_UNVERIFIED/);
    assert.throws(()=>verifyResult(skill,run,{...value,source_refs:[{type:'agent_plan',id:randomUUID()}]},[receipt]),/RESULT_REFERENCE_UNVERIFIED/);
  }
});

test('a read briefing requires genuine source evidence and cannot invent a sent notification', () => {
  const skill = catalog.get('project-briefing')!, run = { id: randomUUID(), authority_stamp: 'authority' }, value = baseResult();
  assert.throws(() => verifyResult(skill, run, value, []), /RESULT_READ_UNVERIFIED/);
  value.source_refs = [{ type: 'project', id }];
  const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool: 'read_resource', operation_id: null, status: 'succeeded', committed: false, resource_refs: [], source_refs: value.source_refs };
  assert.equal(verifyResult(skill, run, value, [receipt]).source_refs.length, 1);
  assert.throws(() => verifyResult(skill, run, { ...value, delivery_status: 'sent' }, [receipt]), /SKILL_OUTPUT_INVALID/);
});

test('L1 project updates require a genuine current-run business write receipt, never the fixed intent or inspection alone', () => {
  const skill=catalog.get('project-update')!,run={id:randomUUID(),authority_stamp:'mutation-authority'},intent={type:'agent_mutation',id:randomUUID()},target={type:'task',id:randomUUID()},operation=randomUUID();
  const value:ProposedResult={...baseResult(),source_refs:[intent],resource_refs:[target],operation_ids:[operation]};
  const write:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'update_task',operation_id:operation,status:'succeeded',committed:true,resource_refs:[target],source_refs:[intent]};
  assert.equal(verifyResult(skill,run,value,[write]).status,'completed');
  for(const tool of ['read_resource','inspect_operation']) {
    const observed:ExecutionReceipt={...write,tool,operation_id:null,committed:false};
    assert.throws(()=>verifyResult(skill,run,{...value,operation_ids:[]},[observed]),/RESULT_COMPLETION_UNVERIFIED/);
  }
  for(const changed of [{...write,run_id:randomUUID()},{...write,authority_stamp:'old-authority'}]) assert.throws(()=>verifyResult(skill,run,value,[changed]),/RESULT_REFERENCE_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,value,[{...write,committed:false}]),/RESULT_OPERATION_UNVERIFIED/);
});

test('exact project stage inputs validate real calendar dates, field masks and permitted transitions', () => {
 const skill=catalog.get('project-update')!,argumentsBase={id:randomUUID(),expected_version:1,reason:'确认项目安排'};
 const input=(tool:string,fields:Record<string,unknown>)=>({mutation:{tool,arguments:{...argumentsBase,...fields}},instructions:'按本次已确认的字段执行'});
 skill.validateInput(input('update_project',{start_date:'2028-02-29'}));
 skill.validateInput(input('update_milestone',{owner_id:randomUUID(),due_at:'2027-01-03T12:00:00+08:00'}));
 skill.validateInput(input('transition_project',{action:'hold'}));
 skill.validateInput(input('transition_milestone',{action:'achieve'}));
 for(const value of [input('update_project',{start_date:'2027-02-29'}),input('update_project',{start_date:'0000-01-01'}),
   input('update_milestone',{due_at:null}),input('update_milestone',{owner_id:'someone'}),
   input('update_milestone',{title:'标题',state:'achieved'}),input('transition_project',{action:'archive'}),
   input('transition_milestone',{action:'close'})]) assert.throws(()=>skill.validateInput(value),/SKILL_INPUT_INVALID/);
});

test('milestone and project stage completion requires the actual current-run command, not an observation', () => {
 const skill=catalog.get('project-update')!,run={id:randomUUID(),authority_stamp:'stage-authority'},source={type:'agent_mutation',id:run.id},operation=randomUUID();
 for(const tool of ['transition_project','update_milestone','transition_milestone']) {
  const target={type:tool==='transition_project'?'project':'milestone',id:randomUUID()};
  const result:ProposedResult={...baseResult(),source_refs:[source],resource_refs:[target],operation_ids:[operation]};
  const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool,status:'succeeded',committed:true,operation_id:operation,source_refs:[source],resource_refs:[target]};
  assert.equal(verifyResult(skill,run,result,[receipt]).status,'completed');
  assert.throws(()=>verifyResult(skill,run,{...result,operation_ids:[]},[{...receipt,tool:'inspect_operation',committed:false,operation_id:null}]),/RESULT_COMPLETION_UNVERIFIED/);
  assert.throws(()=>verifyResult(skill,run,result,[{...receipt,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
 }
});

test('release hashes bind linked instructions and unpublished or changed tools cannot enter a live run', async () => {
  const local = path.join(root, 'app/.local'); await mkdir(local, { recursive: true }); const temporary = await mkdtemp(path.join(local, 'skill-release-'));
  try {
    await cp(path.join(root, 'app/skills'), path.join(temporary, 'app/skills'), { recursive: true });
    await cp(path.join(root, 'docs/skills'), path.join(temporary, 'docs/skills'), { recursive: true });
    const manifestPath = path.join(temporary, 'app/skills/file-update/manifest.json'), manifest = JSON.parse(await readFile(manifestPath, 'utf8')); manifest.state = 'published';
    await writeFile(manifestPath, JSON.stringify(manifest));
    const released = (await loadSkills(temporary)).get('file-update')!; released.assertRunnable(released.sha256, KNOWN_TOOLS);
    assert.throws(() => released.assertRunnable(released.sha256, new Set(['read_resource'])), /SKILL_TOOLS_UNAVAILABLE/);
    await writeFile(path.join(temporary, 'docs/skills/execution-boundaries.md'), 'Changed trusted boundary');
    const changed = (await loadSkills(temporary)).get('file-update')!; assert.notEqual(changed.sha256, released.sha256);
    assert.throws(() => changed.assertRunnable(released.sha256, KNOWN_TOOLS), /SKILL_RELEASE_CHANGED/);
    manifest.instructions_ref = '../outside/SKILL.md'; await writeFile(manifestPath, JSON.stringify(manifest));
    await assert.rejects(loadSkills(temporary), /SKILL_MANIFEST_INVALID/);
    await copyFile(path.join(root, 'app/skills/file-update/manifest.json'), manifestPath);
    manifest.allowed_tools.push('execute_sql'); manifest.instructions_ref = 'file-update/SKILL.md'; await writeFile(manifestPath, JSON.stringify(manifest));
    await assert.rejects(loadSkills(temporary), /SKILL_TOOL_POLICY_INVALID/);
    const readonlyPath = path.join(temporary, 'app/skills/incident-summary/manifest.json'), readonlyManifest = JSON.parse(await readFile(readonlyPath, 'utf8'));
    await copyFile(path.join(root, 'app/skills/file-update/manifest.json'), manifestPath);
    readonlyManifest.allowed_tools.push('commit_file_update'); await writeFile(readonlyPath, JSON.stringify(readonlyManifest));
    await assert.rejects(loadSkills(temporary), /SKILL_TOOL_POLICY_INVALID/);
  } finally {
    const target = await realpath(temporary), expected = await realpath(local);
    if (path.dirname(target) !== expected || !path.basename(target).startsWith('skill-release-')) throw new Error('Refusing cleanup outside owned test directory');
    await rm(target, { recursive: true, force: true });
  }
});


test('subscription discovery never proves a detailed review; actual detail and record receipts must match this run and cited snapshot',()=>{
 const skill=catalog.get('subscription-review')!,run={id:randomUUID(),authority_stamp:'subscription-review'};
 const ref={type:'agent_finance_report',id:randomUUID()},value={...baseResult(),source_refs:[ref]};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'list_subscriptions',status:'succeeded',committed:false,operation_id:null,resource_refs:[],source_refs:[ref]};
 assert.throws(()=>verifyResult(skill,run,value,[receipt]),/RESULT_READ_UNVERIFIED/);
 for(const tool of ['read_subscription_review','read_finance_record']){
  const detail={...receipt,tool};assert.equal(verifyResult(skill,run,value,[detail]).status,'completed');
  for(const altered of [{...detail,run_id:randomUUID()},{...detail,authority_stamp:'old'},{...detail,status:'pending' as const}])assert.throws(()=>verifyResult(skill,run,value,[altered]),/RESULT_(READ|REFERENCE)_UNVERIFIED/);
 }
});
test('explicit subscription detail input accepts actual dates and UUID but rejects arbitrary scopes or invalid calendar dates',()=>{
 const skill=catalog.get('subscription-review')!,input={scope:{project_ids:[randomUUID()],include_company:false},subscription_id:randomUUID(),from:'2025-01-01',to:'2025-02-01'};
 skill.validateInput(input);
 for(const changed of [{...input,subscription_id:'a title'},{...input,from:'2025-02-30'},{...input,sql:'select *'},{...input,from:null}])assert.throws(()=>skill.validateInput(changed),/SKILL_INPUT_INVALID/);
});

test('OA prose remains a draft until a separate human adoption',()=>{
 const skill=catalog.get('oa-assist')!,run={id:randomUUID(),authority_stamp:'oa-prose-authority'};
 const source={type:'agent_oa_request',id:run.id};
 const input={tool:'draft_business_text',arguments:{kind:'contract',business_id:randomUUID(),expected_business_version:1,business_revision_id:randomUUID(),business_content_hash:'a'.repeat(64),fields:['renewal_terms','notes'],reason:'Draft selected prose'}};
 skill.validateInput(input);
 const value:ProposedResult={...baseResult(),status:'draft',resource_refs:[],source_refs:[source],operation_ids:[],next_action:'Human review and adoption'};
 const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'draft_business_text',status:'succeeded',committed:false,operation_id:null,resource_refs:[],source_refs:[source]};
 assert.equal(verifyResult(skill,run,value,[receipt]).status,'draft');
 assert.throws(()=>verifyResult(skill,run,{...value,status:'completed'},[receipt]),/RESULT_COMPLETION_UNVERIFIED/);
 assert.throws(()=>verifyResult(skill,run,value,[{...receipt,run_id:randomUUID()}]),/RESULT_REFERENCE_UNVERIFIED/);
 assert.throws(()=>skill.validateInput({...input,arguments:{...input.arguments,fields:['amount']}}),/SKILL_INPUT_INVALID/);
});
