import type { DeliveryJob, RetryReceipt } from '../src/notifications';
export const notificationRetryIDs = ['13ab9051-f2b0-4e4c-819a-5f65a0d12001','13ab9051-f2b0-4e4c-819a-5f65a0d12002','13ab9051-f2b0-4e4c-819a-5f65a0d12003','13ab9051-f2b0-4e4c-819a-5f65a0d12004','13ab9051-f2b0-4e4c-819a-5f65a0d12005'];
export function notificationRetryFixture() {
  const [source,child,event,actor,receiptID]=notificationRetryIDs;
  const job:DeliveryJob={job_id:source,kind:'notification.deliver.v1',state:'dead',safe_code:'SYNTHETIC_FAILURE',attempt:4,max_attempts:4,first_failed_at:'2026-10-29T01:00:00Z',updated_at:'2026-10-29T01:10:00Z',available_at:'2026-10-29T01:11:00Z',revision:'b'.repeat(64),business_event_id:event,event_type:'task.created',can_retry:true,retry_block:'ready',retry_id:null,retry_job_id:null,parent_job_id:null,result:null};
  const receipt:RetryReceipt={id:receiptID,source_job_id:source,retry_job_id:child,business_event_id:event,created_by:actor,created_at:'2026-10-29T01:12:00Z'};
  return {job,receipt,source,child,event,actor};
}
