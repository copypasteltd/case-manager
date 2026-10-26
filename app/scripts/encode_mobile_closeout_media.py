"""Encode the actual new mobile-closeout recording after verified native/browser cleanup."""
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
from encode_repository_showcase import run, sha

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/mobile-closeout'
BROWSER = EVIDENCE.parent / 'mobile-closeout-browser'


def main():
    result = json.loads((BROWSER / 'result.json').read_text(encoding='utf-8'))
    assert result['passed'] and result['source_unchanged'] and result['owned_browser_closed']
    assert result['native_fixture_exit_code'] == 0 and result['owned_review_server_stopped']
    ffmpeg, ffprobe = shutil.which('ffmpeg'), shutil.which('ffprobe')
    if not ffmpeg or not ffprobe:
        raise SystemExit('Native ffmpeg and ffprobe are required.')
    assert len(result['recordings']) == 1
    clip = result['recordings'][0]
    source = APP / clip['manifest']
    assert sha(source) == clip['manifest_sha256']
    manifest = json.loads(source.read_text(encoding='utf-8'))
    assert manifest['credentials_excluded'] and manifest['real_browser_pixels'] and not manifest['audio']
    frames = manifest['frames']
    assert len(frames) == clip['frame_count'] and len(frames) > 1
    concat = ['ffconcat version 1.0']
    for index, frame in enumerate(frames):
        path = source.parent / frame['file']
        assert path.parent == source.parent and sha(path) == frame['sha256']
        end = frames[index + 1]['at_ms'] if index + 1 < len(frames) else manifest['duration_ms']
        start = frame['at_ms'] if index else 0
        concat += [f"file '{path.name}'", f'duration {max(1, end - start) / 1000:.6f}']
    concat.append(f"file '{frames[-1]['file']}'")
    timed = source.parent / 'timed.ffconcat'
    timed.write_text('\n'.join(concat) + '\n', encoding='utf-8')
    mp4, gif = ROOT / 'docs/media/videos/mobile-project-closeout-workflow.mp4', ROOT / 'docs/media/gifs/mobile-project-closeout-workflow.gif'
    common = [ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', str(timed)]
    run(common + ['-an', '-vf', 'fps=10,scale=trunc(iw/2)*2:trunc(ih/2)*2', '-c:v', 'libx264', '-preset', 'fast', '-crf', '23', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(mp4)])
    speed = max(1.0, manifest['duration_ms'] / 15000)
    graph = (f'[0:v]setpts=PTS/{speed:.6f},fps=6,scale=390:-2:flags=lanczos,split[a][b];'
             '[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3')
    run(common + ['-an', '-filter_complex', graph, '-loop', '0', str(gif)])
    probe = json.loads(run([ffprobe, '-v', 'error', '-show_entries', 'format=duration:stream=codec_name,width,height,nb_frames', '-of', 'json', str(mp4)]))
    assert len(probe['streams']) == 1 and probe['streams'][0]['codec_name'] == 'h264'
    assert abs(float(probe['format']['duration']) - manifest['duration_ms'] / 1000) < 0.35
    assert all(p.stat().st_size < 15 * 1024 * 1024 for p in (mp4, gif))
    artifacts = [{'file': p.relative_to(ROOT).as_posix(), 'sha256': sha(p), 'bytes': p.stat().st_size} for p in (mp4, gif)]
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': result['source_sha256'],
              'browser_result_sha256': sha(BROWSER / 'result.json'), 'frame_manifest_sha256': clip['manifest_sha256'],
              'source_frames': len(frames), 'source_duration_ms': manifest['duration_ms'], 'gif_speed_factor': speed,
              'artifacts': artifacts, 'probe': probe, 'encoder_sha256': sha(Path(__file__)),
              'helper_sha256': sha(APP / 'scripts/encode_repository_showcase.py'), 'audio': False,
              'real_browser_pixels': True, 'synthetic_data': True, 'real_device': False, 'credentials_excluded': True}
    (EVIDENCE / 'media.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'duration': probe['format']['duration'], 'artifacts': artifacts}))


if __name__ == '__main__':
    main()
