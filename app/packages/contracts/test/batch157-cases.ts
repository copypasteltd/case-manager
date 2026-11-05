import assert from 'node:assert/strict';
import { skillReleaseInput, telegramHandoffInput, telegramHandoffReceipt, type SkillReleaseDetails, type TelegramHandoffRequest } from '../src/agent-runtime';
import { disclosureInput, type DisclosurePreview } from '../src/telegram-operations';
const id = (n:number) => `10000000-0000-4000-8000-${String(n).padStart(12,'0')}`;
const release = {version:0,profile:{item:{state:'active',version:3,skill_sha256:'a'.repeat(64)},impact:{active_basis_sha256:'b'.repeat(64)}},history:[]} as unknown as SkillReleaseDetails;
const request = {id:id(1),version:2,project_id:id(2),skill_id:'file-update',request_sha256:'c'.repeat(64),expires_at:'2099-01-01T00:00:00Z',eligible:true,run_id:null} as unknown as TelegramHandoffRequest;
const preview = {run_version:9,body_sha256:'d'.repeat(64),total_chars:4,body:'合成🙂例'} as unknown as DisclosurePreview;
export const batch157Cases: Array<readonly [string,()=>void]> = [
 ['publication requires explicit review, exact active package and real assessment identities',()=>{
  assert.throws(()=>skillReleaseInput(release,'published','','','发布',true));
  assert.throws(()=>skillReleaseInput(release,'published',id(3),'','发布',false));
  assert.throws(()=>skillReleaseInput(release,'published',id(3)+','+id(3),'','发布',true));
  assert.throws(()=>skillReleaseInput({...release,profile:{...release.profile,item:{...release.profile.item,state:'retired'}}},'pilot','','','试行',true));
  const value=skillReleaseInput(release,'published',id(3),id(4),' 完成评估 ',true);
  assert.equal(value.expected_version,0);assert.equal(value.profile_version,3);assert.equal(value.skill_sha256,release.profile.item.skill_sha256);assert.deepEqual(value.assessment_run_ids,[id(3)]);assert.equal(value.reason,'完成评估');assert.equal(value.rollback_release_id,id(4));
 }],
 ['TG handoff cannot cross project, task, ownership state, expiry or reviewed request basis',()=>{
  const value=telegramHandoffInput(request,id(2),'file-update',true);
  assert.equal(value.expected_version,2);assert.equal(value.request_sha256,request.request_sha256);
  for(const v of [{...request,eligible:false},{...request,run_id:id(8)},{...request,expires_at:'2000-01-01T00:00:00Z'}])assert.throws(()=>telegramHandoffInput(v,id(2),'file-update',true));
  assert.throws(()=>telegramHandoffInput(request,id(9),'file-update',true));assert.throws(()=>telegramHandoffInput(request,id(2),'project-update',true));assert.throws(()=>telegramHandoffInput(request,id(2),'file-update',false));
  const run={telegram_handoff:value} as Parameters<typeof telegramHandoffReceipt>[0];assert.equal(telegramHandoffReceipt(run,value),true);assert.equal(telegramHandoffReceipt(run,{...value,request_sha256:'e'.repeat(64)}),false);assert.equal(telegramHandoffReceipt(run),false);
 }],
 ['group disclosure selects exact Unicode characters and requires independent public disclosure consent',()=>{
  const v=disclosureInput(preview,'1','2',true,true);assert.equal([...preview.body].slice(v.start_char,v.start_char+v.max_chars).join(''),'成🙂');assert.equal(v.expected_version,9);assert.equal(v.body_sha256,preview.body_sha256);
  for(const [start,count] of [['-1','2'],['1','0'],['0','2001'],['3','2'],['01','1'],['0','1.5']])assert.throws(()=>disclosureInput(preview,start,count,true,true));
  assert.throws(()=>disclosureInput(preview,'0','1',false,true));assert.throws(()=>disclosureInput(preview,'0','1',true,false));
 }],
];
