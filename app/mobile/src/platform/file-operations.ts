import type { Schemas } from './session';
export function historyCommitAccepted(value:Schemas['FileCommitResult'],run:Schemas['HistoryRestore']):boolean{
 return value.committed===true&&value.file.id===run.file_id&&value.file.current_version_id===value.file_version.id&&value.file_version.file_id===run.file_id&&value.file_version.sha256===run.sha256&&value.file_version.size_bytes===run.size_bytes&&value.file_version.id!==run.source_version_id&&value.file_version.seq>run.base_seq;
}
export function exactScanReport(report:Schemas['UploadScanReport'],source:{id:string;scan_result_id:string|null;sha256:string|null;size_bytes:string}):boolean{
 return source.sha256!==null&&report.id===source.scan_result_id&&report.upload_id===source.id&&report.sha256===source.sha256&&report.size_bytes===source.size_bytes;
}
