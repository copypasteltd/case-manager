"""Encode authenticated, synthetic batch captures after all owned resources close."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
from PIL import Image
from encode_repository_showcase import run, sha
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('groups',nargs='+')
    args=parser.parse_args()
    ffmpeg,ffprobe=shutil.which('ffmpeg'),shutil.which('ffprobe')
    assert ffmpeg and ffprobe
    for group in args.groups:
        assert group.startswith('batch-round-166-') and all(c.isalnum() or c=='-' for c in group)
        evidence=ROOT/'docs/quality/evidence'/group
        report=json.loads((evidence/'result.json').read_text(encoding='utf-8'))
        capture=json.loads((evidence/'media.json').read_text(encoding='utf-8'))
        assert all(report[k] for k in ('passed','source_unchanged','build_source_unchanged','owned_browser_closed','owned_review_server_stopped','isolated_database_cleanup_passed'))
        assert report['native_fixture_exit_code']==0 and not (APP/'.local/business-browser-fixture.json').exists()
        output=ROOT/'docs/media/batch-round-166'/group
        target=evidence/'encoded-media.json'
        if target.exists():raise ValueError('Preserve completed media attempts.')
        source=(APP/capture['recording']['manifest']).resolve()
        assert source.is_relative_to((APP/'.local/repository-showcase').resolve()) and sha(source)==capture['recording']['manifest_sha256']
        manifest=json.loads(source.read_text(encoding='utf-8'))
        assert manifest['credentials_excluded'] and manifest['synthetic_data'] and manifest['real_browser_pixels'] and not manifest['audio']
        frames=manifest['frames'];assert len(frames)>=5
        concat=['ffconcat version 1.0']
        for i,frame in enumerate(frames):
            file=source.parent/frame['file'];assert file.parent==source.parent and sha(file)==frame['sha256']
            end=frames[i+1]['at_ms'] if i+1<len(frames) else manifest['duration_ms']
            concat += ["file '"+file.name+"'",f"duration {max(1,end-(frame['at_ms'] if i else 0))/1000:.6f}"]
        concat += ["file '"+frames[-1]['file']+"'"]
        timed=source.parent/'timed.ffconcat';timed.write_text('\n'.join(concat)+'\n',encoding='utf-8')
        mp4=output/'screen-recording.mp4';gif=output/'screenshot-tour.gif'
        run([ffmpeg,'-hide_banner','-loglevel','error','-y','-f','concat','-safe','0','-i',str(timed),'-an','-vf','fps=10,scale=trunc(iw/2)*2:trunc(ih/2)*2','-c:v','libx264','-preset','fast','-crf','23','-pix_fmt','yuv420p','-movflags','+faststart',str(mp4)])
        tour=['ffconcat version 1.0']
        for i,shot in enumerate(capture['screenshots']):
            original=ROOT/shot['file'];assert sha(original)==shot['sha256']
            image=Image.open(original).convert('RGB');image.thumbnail((960,1000),Image.Resampling.LANCZOS)
            canvas=Image.new('RGB',(960,1000),'#f7f7f7');canvas.paste(image,((960-image.width)//2,(1000-image.height)//2))
            name=f'gif-tour-{i}.png';canvas.save(source.parent/name);tour += ["file '"+name+"'",'duration 2.000000']
        assert capture['screenshots'];tour += ["file '"+name+"'"]
        tour_file=source.parent/'gif-tour.ffconcat';tour_file.write_text('\n'.join(tour)+'\n',encoding='utf-8')
        graph='[0:v]fps=4,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3'
        run([ffmpeg,'-hide_banner','-loglevel','error','-y','-f','concat','-safe','0','-i',str(tour_file),'-an','-filter_complex',graph,'-t',str(2*len(capture['screenshots'])),'-final_delay','25','-loop','0',str(gif)])
        artifacts=[{'file':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'size_bytes':p.stat().st_size} for p in (mp4,gif)]
        (evidence/'capture-manifest.json').write_bytes(source.read_bytes())
        result={'passed':True,'encoded_at':datetime.now(timezone.utc).isoformat(),'group':group,'encoder_sha256':sha(Path(__file__)),'mp4_mode':'actual_authenticated_silent_viewport_recording','gif_mode':'actual_screenshot_tour','synthetic_data':True,'controlled_model':False,'model_exercised':False,'real_cloud':False,'production_acceptance':False,'screenshots':capture['screenshots'],'artifacts':artifacts,'probe':json.loads(run([ffprobe,'-v','error','-show_entries','format=duration:stream=codec_name,width,height,nb_frames','-of','json',str(mp4)]))}
        target.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
        print(json.dumps({'group':group,'passed':True,'screenshots':len(capture['screenshots']),'artifacts':len(artifacts)}),flush=True)
if __name__=='__main__':main()
