"""Encode hash-verified real browser captures; keep raw frames in ignored storage."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/repository-showcase'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(args):
    result = subprocess.run(args, capture_output=True, text=True, encoding='utf-8', errors='replace', timeout=180)
    if result.returncode:
        # No input credentials are accepted by this encoder.
        raise RuntimeError(result.stderr[-2500:])
    return result.stdout


def main():
    result = json.loads((EVIDENCE / 'result.json').read_text(encoding='utf-8'))
    assert result['passed'] and result['source_unchanged']
    assert result['owned_servers_closed'] and result['private_fixture_removed']
    ffmpeg, ffprobe = shutil.which('ffmpeg'), shutil.which('ffprobe')
    if not ffmpeg or not ffprobe:
        raise SystemExit('Native ffmpeg and ffprobe are required on PATH.')
    items = []
    for phase in ('web', 'mobile'):
        report = json.loads((EVIDENCE / f'{phase}.json').read_text(encoding='utf-8'))
        assert report['passed'] and report['owned_browser_closed'] and report['credentials_excluded']
        for clip in report['recordings']:
            manifest_path = APP / clip['manifest']
            assert sha(manifest_path) == clip['manifest_sha256']
            manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
            assert manifest['real_browser_pixels'] and manifest['credentials_excluded'] and not manifest['audio']
            frames = manifest['frames']
            assert len(frames) == clip['frame_count'] and len(frames) > 1
            concat = ['ffconcat version 1.0']
            for index, frame in enumerate(frames):
                path = manifest_path.parent / frame['file']
                assert path.parent == manifest_path.parent and sha(path) == frame['sha256']
                end = frames[index + 1]['at_ms'] if index + 1 < len(frames) else manifest['duration_ms']
                duration = max(1, end - (0 if index == 0 else frame['at_ms'])) / 1000
                concat.extend([f"file '{path.name}'", f'duration {duration:.6f}'])
            concat.append(f"file '{frames[-1]['file']}'")
            concat_path = manifest_path.parent / 'timed.ffconcat'
            concat_path.write_text('\n'.join(concat) + '\n', encoding='utf-8')
            name = clip['name']
            mp4 = ROOT / f'docs/media/videos/{name}.mp4'
            gif = ROOT / f'docs/media/gifs/{name}.gif'
            mp4.parent.mkdir(parents=True, exist_ok=True)
            gif.parent.mkdir(parents=True, exist_ok=True)
            common = [ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', str(concat_path)]
            run(common + ['-an', '-vf', 'fps=10,scale=trunc(iw/2)*2:trunc(ih/2)*2',
                          '-c:v', 'libx264', '-preset', 'fast', '-crf', '23', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(mp4)])
            # The UI automation itself is fast. Preserve short clips at normal speed.
            speed = max(1.0, manifest['duration_ms'] / 15000)
            width = 960 if phase == 'web' else 390
            graph = (f'[0:v]setpts=PTS/{speed:.6f},fps=6,scale={width}:-2:flags=lanczos,split[a][b];'
                     '[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3')
            run(common + ['-an', '-filter_complex', graph, '-loop', '0', str(gif)])
            probe = json.loads(run([ffprobe, '-v', 'error', '-show_entries',
                                   'format=duration:stream=codec_name,width,height,nb_frames,r_frame_rate', '-of', 'json', str(mp4)]))
            assert len(probe['streams']) == 1 and probe['streams'][0]['codec_name'] == 'h264'
            duration = float(probe['format']['duration'])
            assert abs(duration - manifest['duration_ms'] / 1000) < 0.35
            assert mp4.stat().st_size < 15 * 1024 * 1024 and gif.stat().st_size < 15 * 1024 * 1024
            items.append({'name': name, 'source_manifest_sha256': clip['manifest_sha256'],
                          'source_frame_count': len(frames), 'source_duration_ms': manifest['duration_ms'],
                          'mp4': {'file': mp4.relative_to(ROOT).as_posix(), 'sha256': sha(mp4), 'bytes': mp4.stat().st_size, 'probe': probe},
                          'gif': {'file': gif.relative_to(ROOT).as_posix(), 'sha256': sha(gif), 'bytes': gif.stat().st_size,
                                  'speed_factor': round(speed, 6), 'fps': 6, 'width': width},
                          'audio': False, 'synthetic_data': True, 'real_browser_pixels': True, 'real_device': False})
            print(json.dumps({'clip': name, 'mp4_bytes': mp4.stat().st_size, 'gif_bytes': gif.stat().st_size, 'duration': duration}))
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'capture_source_sha256': result['source_sha256'], 'capture_result_sha256': sha(EVIDENCE / 'result.json'),
              'encoder_sha256': sha(Path(__file__)), 'ffmpeg_version': run([ffmpeg, '-version']).splitlines()[0],
              'credentials_excluded': True, 'raw_frames_ignored': True, 'recordings': items}
    (EVIDENCE / 'media.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
