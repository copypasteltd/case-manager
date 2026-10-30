"""Encode actual authenticated business workbench captures after owned resources close."""
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
from PIL import Image
from encode_repository_showcase import run, sha

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/workbench'


def main():
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--feature', choices=('workbench', 'operations', 'docx', 'project-business', 'business-cost', 'cost-payment', 'file-batch', 'document-text', 'office-authoring', 'host-resources', 'resource-notifications'), default='workbench')
    args = parser.parse_args()
    prefix = 'resource-notifications' if args.feature == 'resource-notifications' else 'host-resources' if args.feature == 'host-resources' else 'office-authoring' if args.feature == 'office-authoring' else 'document-text' if args.feature == 'document-text' else 'file-batch' if args.feature == 'file-batch' else 'cost-payment' if args.feature == 'cost-payment' else 'business-cost' if args.feature == 'business-cost' else 'project-business' if args.feature == 'project-business' else 'docx-text' if args.feature == 'docx' else 'operations-workbench' if args.feature == 'operations' else 'workbench'
    evidence = EVIDENCE.parent / prefix
    evidence.mkdir(parents=True, exist_ok=True)
    ffmpeg, ffprobe = shutil.which('ffmpeg'), shutil.which('ffprobe')
    if not ffmpeg or not ffprobe:
        raise SystemExit('Native ffmpeg and ffprobe are required.')
    reports = []
    for client in ('web', 'mobile'):
        browser = EVIDENCE.parent / (prefix + '-' + client)
        result = json.loads((browser / 'result.json').read_text(encoding='utf-8'))
        capture = json.loads((browser / 'media.json').read_text(encoding='utf-8'))
        assert result['passed'] and result['source_unchanged'] and result['build_source_unchanged']
        assert result['owned_browser_closed'] and result['native_fixture_exit_code'] == 0 and result['owned_review_server_stopped']
        assert result['isolated_database_cleanup_passed'] and not (APP / '.local/business-browser-fixture.json').exists()
        clip = capture['recording']
        source = (APP / clip['manifest']).resolve()
        assert source.is_relative_to((APP / '.local/repository-showcase').resolve())
        assert sha(source) == clip['manifest_sha256']
        manifest = json.loads(source.read_text(encoding='utf-8'))
        assert manifest['credentials_excluded'] and manifest['synthetic_data'] and manifest['real_browser_pixels'] and not manifest['audio']
        frames = manifest['frames']
        assert len(frames) == clip['frame_count'] and len(frames) > 1
        frozen = evidence / (client + '-capture-manifest.json')
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
        name = prefix + '-' + client
        mp4, gif = ROOT / ('docs/media/videos/' + name + '.mp4'), ROOT / ('docs/media/gifs/' + name + '.gif')
        common = [ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', str(timed)]
        run(common + ['-an', '-vf', 'fps=10,scale=trunc(iw/2)*2:trunc(ih/2)*2', '-c:v', 'libx264', '-preset', 'fast', '-crf', '23', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(mp4)])
        # GIFs are disclosed screenshot tours, so responsive and user-scope
        # differences remain visible even when a recorded overview is still.
        width = 390 if client == 'mobile' else 960
        tour = ['ffconcat version 1.0']
        for index, shot in enumerate(capture['screenshots']):
            original = ROOT / shot['file']
            assert sha(original) == shot['sha256']
            picture = Image.open(original).convert('RGB')
            picture.thumbnail((width, 1000), Image.Resampling.LANCZOS)
            canvas = Image.new('RGB', (width, 1000), '#f7f7f7')
            canvas.paste(picture, ((width - picture.width) // 2, (1000 - picture.height) // 2))
            name_in_tour = 'gif-tour-' + str(index) + '.png'
            canvas.save(source.parent / name_in_tour)
            tour += [f"file '{name_in_tour}'", 'duration 2.000000']
        assert len(capture['screenshots']) == 3
        tour += [f"file '{name_in_tour}'"]
        tour_file = source.parent / 'gif-tour.ffconcat'
        tour_file.write_text('\n'.join(tour) + '\n', encoding='utf-8')
        graph = ('[0:v]fps=4,split[a][b];'
            '[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3')
        run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-f', 'concat', '-safe', '0', '-i', str(tour_file), '-an', '-filter_complex', graph, '-t', '6', '-final_delay', '25', '-loop', '0', str(gif)])
        gif_probe = json.loads(run([ffprobe, '-v', 'error', '-show_entries', 'format=duration:stream=width,height,nb_frames', '-of', 'json', str(gif)]))
        assert abs(float(gif_probe['format']['duration']) - 6) < 0.3
        probe = json.loads(run([ffprobe, '-v', 'error', '-show_entries', 'format=duration:stream=codec_name,width,height,nb_frames', '-of', 'json', str(mp4)]))
        assert len(probe['streams']) == 1 and probe['streams'][0]['codec_name'] == 'h264'
        assert abs(float(probe['format']['duration']) - manifest['duration_ms'] / 1000) < 0.35
        assert all(file.stat().st_size < 15 * 1024 * 1024 for file in (mp4, gif))
        artifacts = [{'file': file.relative_to(ROOT).as_posix(), 'sha256': sha(file), 'bytes': file.stat().st_size} for file in (mp4, gif)]
        reports.append({'client': client, 'source_sha256': result['source_sha256'], 'build_source_sha256': result['build_source_sha256'],
            'capture_manifest': frozen.relative_to(ROOT).as_posix(), 'capture_manifest_sha256': sha(frozen),
            'capture_frame_count': len(frames), 'capture_duration_ms': manifest['duration_ms'],
            'gif_mode': 'actual_screenshot_tour', 'gif_is_screen_recording': False,
            'gif_frame_duration_ms': 2000, 'gif_screenshots': capture['screenshots'],
            'gif_probe': gif_probe, 'probe': probe, 'artifacts': artifacts})
    value = {'passed': True, 'encoded_at': datetime.now(timezone.utc).isoformat(), 'encoder_script_sha256': sha(Path(__file__)),
        'ffmpeg_sha256': sha(Path(ffmpeg)), 'ffprobe_sha256': sha(Path(ffprobe)), 'recordings': reports,
        'scope': 'Silent MP4 recordings use actual authenticated synthetic viewport frames. GIFs are labelled tours of three actual screenshots (scenario labels retained in each feature capture), two seconds each. No real model or device claim.'}
    if args.feature in ('operations', 'docx', 'project-business', 'business-cost', 'cost-payment', 'file-batch', 'document-text', 'office-authoring', 'host-resources', 'resource-notifications'):
        value['scope'] = 'Silent MP4 records the actual authenticated home refresh after the owned copy worker verifies real synthetic bytes. GIF is an explicit tour of three actual screenshots: initial work, queued retry and completed work. No physical device, real cloud or production acceptance.'
        value['artifacts'] = [item for report in reports for item in report['gif_screenshots'] + report['artifacts']]
    if args.feature == 'office-authoring':
        value['scope'] = 'Actual authenticated synthetic Word/Excel/PPT content authoring on Web and exact-version readonly content on shared H5. MP4 records actual viewport pixels after MFA; GIF explicitly tours three actual screenshots, two seconds each. HTTP tests separately cover full upload/hash/scan/promotion/formal commit. No device, real cloud, Office layout or production acceptance.'
    if args.feature == 'document-text':
        value['scope'] = 'Actual authenticated synthetic PDF/image OCR preview, exact-version comparison and secretary-source reads on Web/H5. MP4 uses actual viewport frames after MFA. GIF explicitly tours three actual screenshots, two seconds each; not a continuous recording. Native extraction is real; object storage is synthetic. No device, cloud, model, production or arbitrary OCR accuracy acceptance.'
        value['artifacts'] = [item for report in reports for item in report['gif_screenshots'] + report['artifacts']]
    if args.feature == 'cost-payment':
        value['scope'] = 'Actual authenticated synthetic cost-to-payment and settlement on Web/H5: partial supplier cash, reviewed foreign cash and exact approved employee advance. MP4 uses real viewport pixels after MFA; GIF explicitly tours three actual screenshots. No bank transfer, credentials, physical device, cloud, model or production acceptance.'
    if args.feature == 'file-batch':
        value['scope'] = 'Actual authenticated synthetic fixed-inventory file move, trash and restore on Web/H5. MP4 records the first batch move execution and receipt using real viewport pixels after MFA; GIF explicitly tours three actual action receipt screenshots. Formal content heads and history remain unchanged. No credentials, physical device, real cloud, model or production acceptance.'
    if args.feature == 'business-cost':
        value['scope'] = 'Actual authenticated synthetic approved-business expense recording and ordinary source association. MP4 records the first source after MFA with real viewport pixels; GIF is a disclosed tour of three actual screenshots. Three source kinds run through the actual Web/H5 UI. No credentials, device/cloud/model/production acceptance.'
    if args.feature == 'project-business':
        value['scope'] = 'Actual authenticated synthetic project-scoped six-module draft creation and mobile subscription period review. MP4 uses real viewport frames; GIF is a labelled three-screenshot tour, not a realtime recording. No device/cloud/model acceptance.'
    if args.feature == 'docx':
        value['scope'] = 'Silent MP4 records actual authenticated synthetic DOCX search, story previews and exact-version comparison. GIF is a disclosed tour of three real screenshots: revisions/table, referenced stories and comparison. No physical device, real cloud or model acceptance.'
    if args.feature == 'resource-notifications':
        value['scope'] = 'Actual authenticated synthetic resource alert to owned workbench, in-app notification, read acknowledgement, live recovery and immutable history on Web/H5. MP4 records actual post-MFA viewport pixels; GIF explicitly tours three actual screenshots, two seconds each. No device, external alert delivery, real target, cloud or production acceptance.'
    (evidence / 'media.json').write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'recordings': len(reports), 'artifacts': sum(len(item['artifacts']) for item in reports)}))


if __name__ == '__main__':
    main()
