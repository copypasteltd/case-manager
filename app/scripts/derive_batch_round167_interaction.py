"""Make a readable GIF from the immutable actual viewport recording, locally."""
import hashlib,json,shutil,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
GROUP=sys.argv[1]
assert GROUP.startswith('batch-round-167-') and all(c.isalnum() or c=='-' for c in GROUP)
E=ROOT/'docs/quality/evidence'/GROUP
M=ROOT/'docs/media/batch-round-167'/GROUP
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
source=M/'screen-recording.mp4';target=M/'interaction.gif';record=E/'interaction-media.json'
if target.exists() or record.exists():raise SystemExit('Preserve earlier media; choose an explicit new artifact.')
proof=json.loads((E/'encoded-media.json').read_text(encoding='utf-8'))
assert proof['passed'] and any(i['sha256']==sha(source) for i in proof['artifacts'] if i['file'].endswith('.mp4'))
ffmpeg=shutil.which('ffmpeg');assert ffmpeg
subprocess.run([ffmpeg,'-hide_banner','-loglevel','error','-i',str(source),'-filter_complex','fps=4,scale=1280:-1:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3','-loop','0',str(target)],check=True,capture_output=True)
v={'passed':target.stat().st_size>1000,'source_video':source.relative_to(ROOT).as_posix(),'source_video_sha256':sha(source),'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target),'bytes':target.stat().st_size,'actual_viewport_recording_conversion':True,'screenshot_tour_unchanged':True,'source_script_sha256':sha(Path(__file__)),'synthetic_data':True,'production_acceptance':False}
record.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':v['passed'],'bytes':v['bytes']}))
