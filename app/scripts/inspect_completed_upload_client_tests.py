"""Seal Vitest's actual JSON reporter output from the selected completed-upload run."""
from datetime import datetime
import hashlib,json
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
E=APP.parent/'docs/quality/evidence/completed-upload-retirement'
def main():
 target=E/'web-test-results.json';assert not target.exists()
 record=json.loads((E/'web-tests-1.json').read_text(encoding='utf-8'))
 source=APP/'web/.vitest/json/output.json'
 raw=source.read_bytes();data=json.loads(raw)
 start=datetime.fromisoformat(record['started_at']).timestamp()
 assert record['passed'] and start<=source.stat().st_mtime<=start+record['elapsed_seconds']+2
 events=[{'file':Path(suite['name']).relative_to(APP).as_posix(),'name':row['fullName'],'status':row['status']} for suite in data['testResults'] for row in suite['assertionResults']]
 assert events and data['numFailedTests']==0 and data['numPendingTests']==0 and len(events)==data['numPassedTests'] and all(v['status']=='passed' for v in events)
 result={'passed':True,'execution_record_sha256':hashlib.sha256((E/'web-tests-1.json').read_bytes()).hexdigest(),'raw_report_sha256':hashlib.sha256(raw).hexdigest(),'source_sha256':record['source_sha256'],'tests':{k:data[k] for k in ('numPassedTests','numFailedTests','numPendingTests')},'events':events,'reporter':'Vitest configured JSON output file, not console JSON','inspection_only':True,'tests_rerun':False}
 target.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result['tests']))
if __name__=='__main__':main()
