"""Decode actual captures for human visual review; never claim review automatically."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/contract-body'
OUT = ROOT / 'app/.local/repository-showcase/contract-body-visual-review'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sheet(items, name):
    canvas = Image.new('RGB', (480 * len(items), 1040), '#dedede')
    draw = ImageDraw.Draw(canvas)
    for index, (label, source) in enumerate(items):
        picture = Image.open(source).convert('RGB')
        picture.thumbnail((472, 1000), Image.Resampling.LANCZOS)
        canvas.paste(picture, (index * 480 + (480 - picture.width) // 2, 32))
        draw.text((index * 480 + 8, 8), label, fill='#111111')
    target = OUT / name
    canvas.save(target)
    return target


def main():
    media = json.loads((E / 'media.json').read_text(encoding='utf-8'))
    assert media['passed']
    ffmpeg = shutil.which('ffmpeg')
    assert ffmpeg
    OUT.mkdir(parents=True, exist_ok=True)
    report = {'reviewed': False, 'prepared_at': datetime.now(timezone.utc).isoformat(),
        'screenshots': [], 'gifs': [], 'gif_frames_reviewed': {},
        'recording_frames_reviewed': {}, 'findings': [],
        'scope': 'Prepared real screenshot and decoded video/GIF pixels; requires actual visual inspection before publication.'}
    for recording in media['recordings']:
        client = recording['client']
        capture = json.loads((E.parent / ('contract-body-' + client) / 'media.json').read_text(encoding='utf-8'))
        originals = []
        for shot in capture['screenshots']:
            file = ROOT / shot['file']
            assert sha(file) == shot['sha256']
            report['screenshots'].append(shot)
            originals.append((file.stem, file))
        print(sheet(originals, client + '-screenshots.png'))
        samples = []
        video = next(v for v in recording['artifacts'] if v['file'].endswith('.mp4'))
        duration = float(recording['probe']['format']['duration'])
        seconds = [0, round(max(0, duration - 0.4), 3)]
        for index, position in enumerate(seconds):
            frame = OUT / (client + '-mp4-' + str(index) + '.png')
            subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', '-ss', str(position),
                '-i', str(ROOT / video['file']), '-frames:v', '1', str(frame)], check=True, capture_output=True)
            samples.append(('actual MP4 t=' + str(position), frame))
        gif = next(v for v in recording['artifacts'] if v['file'].endswith('.gif'))
        report['gifs'].append({'file': gif['file'], 'sha256': sha(ROOT / gif['file'])})
        with Image.open(ROOT / gif['file']) as image:
            frames = [0, image.n_frames - 1]
            for index in frames:
                image.seek(index)
                file = OUT / (client + '-gif-' + str(index) + '.png')
                image.convert('RGB').save(file)
                samples.append(('actual GIF frame=' + str(index), file))
            report['gif_frames_reviewed'][client] = {'decoded_frames': frames,
                'frame_count': image.n_frames, 'private_decoded_directory': OUT.relative_to(ROOT).as_posix()}
        report['recording_frames_reviewed'][client] = {'file': video['file'], 'sha256': sha(ROOT / video['file']),
            'sampled_seconds': seconds}
        print(sheet(samples, client + '-decoded.png'))
    (OUT / 'review-draft.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
