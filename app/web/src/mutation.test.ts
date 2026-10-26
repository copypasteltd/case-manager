import { describe, expect, it } from 'vitest';
import { makeDeliverableCreation, makeDeliverableUpdate, makeMilestoneCreation, makeMutation, mutationChanges, mutationReceipt, type MutationView } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';

const id='11111111-1111-4111-8111-111111111111',runID='22222222-2222-4222-8222-222222222222';
const target={id,version:7};
const input=makeMutation('update_project',target,'name','Changed name',false,'Explicit rename');
const view:MutationView={run_id:runID,tool:input.tool,arguments:input.arguments,resource_type:'project',resource_id:id,base_version:7,intent_sha256:'a'.repeat(64),before:{id,version:7,name:'Before'},status:'authorized',operation_id:null,result_version:null,result:null};
const run={id:runID,mutation:view} as Schemas['Run'];

it('deliverable creation fixes a complete parent instruction with explicit association or null',()=>{
 const intent=makeDeliverableCreation(target,'New delivery','Review actual fixed results',runID,null,'Create only this draft');
 expect(intent).toEqual({tool:'create_deliverable',arguments:{id,expected_version:7,name:'New delivery',acceptance_criteria:'Review actual fixed results',owner_id:runID,milestone_id:null,reason:'Create only this draft'}});
 expect(()=>makeMutation('create_deliverable',target,'name','Partial',false,'Reason')).toThrow();
 for(const values of [['','Criteria',runID,null],['Delivery',' ',runID,null],['Delivery','Criteria','',null],['Delivery','Criteria',runID,'bad-id']] as const)expect(()=>makeDeliverableCreation(target,values[0],values[1],values[2],values[3],'Reason')).toThrow();
});
it('a new deliverable receipt has its own ID and version one without a human revision',()=>{
 const intent=makeDeliverableCreation(target,'New delivery','Review actual fixed results',runID,null,'Create only this draft');
 const child={id:'33333333-3333-4333-8333-333333333333',version:1,revision_id:null};
 const receipt:MutationView={...view,tool:intent.tool,arguments:intent.arguments,status:'committed',operation_id:runID,result_version:1,result:child};
 expect(mutationReceipt({...run,mutation:receipt},intent)).toBe(true);
 for(const result of [{...child,id},{...child,version:8},{...child,revision_id:runID},{id:child.id,version:1},{...child,state:'accepted'}])expect(mutationReceipt({...run,mutation:{...receipt,result}},intent)).toBe(false);
 expect(mutationReceipt({...run,mutation:{...receipt,resource_type:'deliverable'}},intent)).toBe(false);
});

it('complete deliverable metadata preserves explicit null and never permits partial authority',()=>{
 const update=makeDeliverableUpdate(target,'Reviewed delivery','Verify actual evidence',runID,null,'Explicit unlink');
 expect(update.arguments).toEqual({id,expected_version:7,name:'Reviewed delivery',acceptance_criteria:'Verify actual evidence',owner_id:runID,milestone_id:null,reason:'Explicit unlink'});
 expect(mutationChanges(update).find(item=>item.field==='milestone_id')?.value).toBe('清空');
 expect(()=>makeMutation('update_deliverable',target,'name','Partial',false,'Reason')).toThrow();
 for(const values of [['','Criteria',runID,null],['Delivery',' ',runID,null],['Delivery','Criteria','',null],['Delivery','Criteria',runID,'bad-id']] as const) expect(()=>makeDeliverableUpdate(target,values[0],values[1],values[2],values[3],'Reason')).toThrow();
});
it('deliverable acknowledgement must preserve the human revision and exact metadata intent',()=>{
 const input=makeDeliverableUpdate(target,'Reviewed delivery','Verify actual evidence',runID,null,'Explicit unlink');
 const receipt:MutationView={...view,tool:input.tool,resource_type:'deliverable',arguments:input.arguments,before:{id,version:7,current_revision_id:runID},status:'committed',operation_id:runID,result_version:8,result:{id,version:8,revision_id:runID}};
 expect(mutationReceipt({...run,mutation:receipt},input)).toBe(true);
 for(const change of [{result:{id,version:8,revision_id:null}},{result:{id,version:8}},{arguments:{...input.arguments,milestone_id:runID}},{resource_type:'task'}])expect(mutationReceipt({...run,mutation:{...receipt,...change}} as Schemas['Run'],input)).toBe(false);
});

it('milestone creation distinguishes its parent version from a genuine new child receipt',()=>{
 const intent=makeMilestoneCreation(target,'  Release review  ',runID,'2027-03-05T18:00:00+08:00','Create reviewed milestone');
 const before={id,version:7};
 const child={id:'33333333-3333-4333-8333-333333333333',project_id:id,version:1,state:'open',title:'Release review',owner_id:runID,due_at:'2027-03-05T10:00:00Z'};
 const receipt:MutationView={...view,tool:'create_milestone',arguments:intent.arguments,before,status:'committed',result_version:1,operation_id:runID,result:child};
 expect(mutationReceipt({...run,mutation:receipt},intent)).toBe(true);
 for(const result of [{...child,id},{...child,project_id:runID},{...child,version:8},{...child,owner_id:id},{...child,title:'Wrong title'},{...child,due_at:'2027-03-05T11:00:00Z'}])expect(mutationReceipt({...run,mutation:{...receipt,result}},intent)).toBe(false);
 expect(()=>makeMilestoneCreation(target,'Title','','2027-03-05T18:00:00+08:00','Reason')).toThrow();
 expect(()=>makeMilestoneCreation(target,'Title',runID,'2027-03-05T18:00:00','Reason')).toThrow();
 expect(()=>makeMutation('create_milestone',target,'title','Partial',false,'Reason')).toThrow();
});
it('task milestone association requires an actual ID and explicit null to unlink',()=>{
 expect(makeMutation('update_task',target,'milestone_id',runID,false,'Associate').arguments.milestone_id).toBe(runID);
 expect(makeMutation('update_task',target,'milestone_id','',true,'Unlink').arguments.milestone_id).toBeNull();
 expect(()=>makeMutation('update_task',target,'milestone_id','',false,'Associate')).toThrow();
});

describe('one explicit L1 update and exact public receipt',()=>{
  it('pins milestone fields and project stages without enabling archive, null dates or unrelated edits',()=>{
    for(const [tool,field,value] of [['update_milestone','title','Milestone title'],['update_milestone','owner_id',id],['update_milestone','due_at','2027-01-03T18:00:00+08:00'],['transition_milestone','action','achieve'],['transition_project','action','close']] as const) {
      const m=makeMutation(tool,target,field,value,false,'Explicit stage or milestone decision');
      expect(m.arguments).toEqual({id,expected_version:7,reason:'Explicit stage or milestone decision',[field]:value});
    }
    for(const [tool,field,value,clear] of [['transition_project','action','archive',false],['update_milestone','due_at','',true],['update_milestone','owner_id','',true],['update_milestone','owner_id','not-a-member-id',false],['update_milestone','project_id',id,false],['transition_milestone','action','complete',false]] as const) expect(()=>makeMutation(tool,target,field,value,clear,'Chosen action')).toThrow();
    expect(mutationChanges(makeMutation('transition_project',target,'action','cancel',false,'Cancel project'))[0].value).toBe('取消项目');
  });
  it('rejects mismatched milestone resources and committed states or content while accepting equivalent timezones',()=>{
    const operation='33333333-3333-4333-8333-333333333333';
    for(const [tool,action,state,kind] of [['transition_project','hold','on_hold','project'],['transition_milestone','achieve','achieved','milestone']] as const) {
      const intent=makeMutation(tool,target,'action',action,false,'Exact state');
      const receipt:MutationView={...view,tool,resource_type:kind,arguments:intent.arguments,status:'committed',operation_id:operation,result_version:8,result:{id,version:8,state}};
      expect(mutationReceipt({...run,mutation:receipt},intent)).toBe(true);
      expect(mutationReceipt({...run,mutation:{...receipt,resource_type:'task'}},intent)).toBe(false);
      expect(mutationReceipt({...run,mutation:{...receipt,result:{id,version:8,state:'cancelled'}}},intent)).toBe(false);
    }
    const intent=makeMutation('update_milestone',target,'due_at','2027-01-03T18:00:00+08:00',false,'Exact instant');
    const receipt:MutationView={...view,tool:intent.tool,resource_type:'milestone',arguments:intent.arguments,status:'committed',operation_id:operation,result_version:8,result:{id,version:8,due_at:'2027-01-03T10:00:00Z'}};
    expect(mutationReceipt({...run,mutation:receipt},intent)).toBe(true);
    expect(mutationReceipt({...run,mutation:{...receipt,result:{id,version:8,due_at:'2027-01-03T11:00:00Z'}}},intent)).toBe(false);
  });
  it('preserves the exact selected field mask and distinguishes clearing dates from omission',()=>{
    expect(Object.keys(input.arguments).sort()).toEqual(['expected_version','id','name','reason']);
    const clear=makeMutation('update_project',target,'due_date','',true,'Clear selected date');
    expect(clear.arguments.due_date).toBeNull();expect(clear.arguments).not.toHaveProperty('start_date');
    expect(mutationChanges(clear)).toEqual([{field:'due_date',label:'计划完成日期',value:'清空'}]);
  });
  it('rejects unexposed permission and acceptance edits, impossible dates and missing explicit values',()=>{
    for(const [tool,field] of [['update_project','owner_id'],['update_project','classification'],['update_task','review_required']] as const) expect(()=>makeMutation(tool,target,field,'value',false,'Explicit edit')).toThrow();
    expect(()=>makeMutation('update_project',target,'due_date','2026-02-30',false,'Explicit date')).toThrow();
    expect(()=>makeMutation('transition_task',target,'action','approve',false,'Explicit action')).toThrow();
    expect(()=>makeMutation('update_task',target,'title','',false,'Explicit name')).toThrow();
    expect(()=>makeMutation('update_task',target,'title','New title',false,'')).toThrow();
  });
  it('keeps the request unresolved if server acknowledgement changes a target, version, field or intent',()=>{
    expect(mutationReceipt(run,input)).toBe(true);
    for(const change of [{run_id:id},{base_version:8},{resource_id:runID},{tool:'update_task'},{intent_sha256:''},{arguments:{...input.arguments,scope:'Added field'}},{before:{id,version:8}}]) expect(mutationReceipt({...run,mutation:{...view,...change}} as Schemas['Run'],input)).toBe(false);
    expect(mutationReceipt(run)).toBe(false);
  });
  it('accepts committed acknowledgement only with the corresponding real resource and next version',()=>{
    const committed:MutationView={...view,status:'committed',operation_id:'33333333-3333-4333-8333-333333333333',result_version:8,result:{id,version:8,name:'Changed name'}};
    expect(mutationReceipt({...run,mutation:committed},input)).toBe(true);
    for(const change of [{operation_id:null},{result_version:9},{result:{id:runID,version:8}},{result:null}]) expect(mutationReceipt({...run,mutation:{...committed,...change}},input)).toBe(false);
  });
});
