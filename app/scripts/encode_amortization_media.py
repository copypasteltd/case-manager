"""Encode only the actual authenticated concern captures after their owned resources are closed."""
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
from encode_repository_showcase import run, sha

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/cost-amortization'


def main():
    ffmpeg, ffprobe = shutil.which('ffmpeg'), shutil.which('ffprobe')
    if not ffmpeg or not ffprobe:
        raise SystemExit('Native ffmpeg and ffprobe are required.')
    reports = []
    for client in ('web', 'mobile'):
        browser = EVIDENCE.parent / ('cost-amortization-' + client)
        result = json.loads((browser / 'result.json').read_text(encoding='utf-8'))
        capture = json.loads((browser / 'capture.json').read_text(encoding='utf-8'))
        assert result['passed'] and result['source_unchanged'] and result['build_source_unchanged']
        assert result['owned_browser_closed'] and result['native_fixture_exit_code'] == 0 and result['owned_review_server_stopped']
        assert result['isolated_database_cleanup_passed'] and not (APP / '.local/business-browser-fixture.json').exists()
        assert len(capture['recordings']) == 1
        clip = capture['recordings'][0]
        source = (APP / clip['manifest']).resolve()
        assert source.is_relative_to((APP / '.local/repository-showcase').resolve())
        assert sha(source) == clip['manifest_sha256']
        manifest = json.loads(source.read_text(encoding='utf-8'))
        assert manifest['credentials_excluded'] and manifest['synthetic_data'] and manifest['real_browser_pixels'] and not manifest['audio']
        frames = manifest['frames']
        assert len(frames) == clip['frame_count'] and len(frames) > 1
        frozen = EVIDENCE / (client + '-capture-manifest.json')
        if frozen.exists() and frozen.read_bytes() != source.read_bytes():
            raise SystemExit('An existing capture manifest differs; archive that completed attempt first.')
        frozen.write_bytes(source.read_bytes())
        concat = ['ffconcat version 1.0']
        for index, frame in enumerate(frames):
            file = source.parent / frame['file']
            assert file.parent == source.parent and sha(file) == frame['sha256']
            end = frames[index + 1]['at_ms'] if index + 1 < len(frames) else manifest['duration_ms']
            start = frame['at_ms'] if index else 0
            concat += [f"file '{file.name}'", f'duration {max(1, end - start) / 1000:.6f}']
        concat.append(f"file '{frames[-1]['file']}'")
        timed = source.parent / 'timed.ffconcat'
        timed.write_text('\n'.join(concat) + '\n', encoding='utf-8')
        name = 'cost-amortization-' + client
        mp4, gif = ROOT / ('docs/media/videos/' + name + '.mp4'), ROOT / ('docs/media/gifs/' + name + '.gif')
        common = [ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', str(timed)]
        run(common + ['-an', '-vf', 'fps=10,scale=trunc(iw/2)*2:trunc(ih/2)*2', '-c:v', 'libx264', '-preset', 'fast', '-crf', '23', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(mp4)])
        speed = max(1.0, manifest['duration_ms'] / 15000)
        width = 390 if client == 'mobile' else 960
        graph = (f'[0:v]setpts=PTS/{speed:.6f},fps=6,scale={width}:-2:flags=lanczos,split[a][b];'
            '[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3')
        run(common + ['-an', '-filter_complex', graph, '-loop', '0', str(gif)])
        probe = json.loads(run([ffprobe, '-v', 'error', '-show_entries', 'format=duration:stream=codec_name,width,height,nb_frames', '-of', 'json', str(mp4)]))
        assert len(probe['streams']) == 1 and probe['streams'][0]['codec_name'] == 'h264'
        assert abs(float(probe['format']['duration']) - manifest['duration_ms'] / 1000) < 0.35
        assert all(file.stat().st_size < 15 * 1024 * 1024 for file in (mp4, gif))
        artifacts = [{'file': file.relative_to(ROOT).as_posix(), 'sha256': sha(file), 'bytes': file.stat().st_size} for file in (mp4, gif)]
        reports.append({'client': client, 'source_sha256': result['source_sha256'], 'build_source_sha256': result['build_source_sha256'],
            'capture_manifest': frozen.relative_to(ROOT).as_posix(), 'capture_manifest_sha256': sha(frozen),
            'capture_frame_count': len(frames), 'capture_duration_ms': manifest['duration_ms'],
            'gif_speed_multiplier': speed, 'probe': probe, 'artifacts': artifacts})
    value = {'passed': True, 'encoded_at': datetime.now(timezone.utc).isoformat(), 'encoder_script_sha256': sha(Path(__file__)),
        'ffmpeg_sha256': sha(Path(ffmpeg)), 'ffprobe_sha256': sha(Path(ffprobe)), 'recordings': reports,
        'scope': 'Actual authenticated synthetic Web/Taro H5 viewport frames; silent clips; no real model or device claim.'}
    (EVIDENCE / 'media.json').write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'recordings': len(reports), 'artifacts': sum(len(item['artifacts']) for item in reports)}))


if __name__ == '__main__':
    main()
