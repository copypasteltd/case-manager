import assert from 'node:assert/strict';
import { test } from 'node:test';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';
import { verifyResult, type ExecutionReceipt, type ProposedResult } from './evidence.js';
const catalog=await loadSkills(fileURLToPath(new URL('../../../',import.meta.url)));
const run={id:randomUUID(),authority_stamp:'indexed-file-current-authority',prepared_context:'{}'};
const ref={type:'file',id:randomUUID(),version_id:randomUUID()};
const receipt:ExecutionReceipt={run_id:run.id,authority_stamp:run.authority_stamp,tool:'read_resource',operation_id:null,status:'succeeded',committed:false,resource_refs:[],source_refs:[ref]};
const result:ProposedResult={status:'completed',summary:'Read fixed-version extracted main document text; images were not read.',resource_refs:[],source_refs:[ref],operation_ids:[],missing_fields:[],next_action:''};
test('indexed file summaries require current actual read evidence without a write claim',()=>{
 for(const name of ['project-briefing','incident-summary']) assert.deepEqual(verifyResult(catalog.get(name)!,run,result,[receipt]),result);
});
test('another version, authority or fabricated operation cannot complete an indexed summary',()=>{
 const skill=catalog.get('project-briefing')!;
 assert.throws(()=>verifyResult(skill,run,{...result,source_refs:[{...ref,version_id:randomUUID()}]},[receipt]),/RESULT_REFERENCE_UNVERIFIED/);
 for(const patch of [{run_id:randomUUID()},{authority_stamp:'stale-authority'},{status:'failed' as const}]) assert.throws(()=>verifyResult(skill,run,result,[{...receipt,...patch}]),/RESULT_(REFERENCE|READ)_UNVERIFIED/);
 assert.throws(()=>verifyResult(skill,run,{...result,operation_ids:[randomUUID()]},[receipt]),/RESULT_OPERATION_UNVERIFIED/);
});
