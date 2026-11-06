"""Exercise actual split/reassembly bytes and corrupted/missing-part rejection."""
import copy
import hashlib
import io
import json
from pathlib import Path
import tempfile
import zipfile
from release_archive_parts164 import package_archive, verify_parts, assemble


def main():
    root = Path(__file__).resolve().parents[2]
    checked = []
    with tempfile.TemporaryDirectory(prefix='archive164-', dir=root/'app/.local') as temporary:
        folder = Path(temporary)
        original = folder/'original.zip'
        with zipfile.ZipFile(original, 'x') as archive:
            archive.writestr('docs/media/a.txt', '完整媒体字节🙂' * 100)
            archive.writestr('docs/media/b.bin', bytes(range(256)) * 11)
        manifest = package_archive(original, folder, 'complete-media.zip', 1024)
        output = assemble(folder, manifest, folder/'restored.zip')
        assert output.read_bytes() == original.read_bytes()
        with zipfile.ZipFile(output) as archive:
            assert archive.testzip() is None and len(archive.namelist()) == 2
        checked.append('actual_multi_part_zip_reassembled_byte_for_byte')
        path = folder/manifest['parts'][0]['name']; saved = path.read_bytes()
        for data in (saved[:-1], bytes([saved[0] ^ 1]) + saved[1:]):
            path.write_bytes(data)
            try:
                verify_parts(folder, manifest)
                raise AssertionError('Changed part accepted')
            except ValueError:
                pass
        path.write_bytes(saved); checked.append('truncated_and_equal_size_corrupted_parts_rejected')
        for change in ('missing', 'order', 'traversal', 'hash'):
            wrong = copy.deepcopy(manifest)
            if change == 'missing': wrong['parts'][0]['name'] = 'absent.001'
            if change == 'order': wrong['parts'].reverse()
            if change == 'traversal': wrong['parts'][0]['name'] = '../outside.001'
            if change == 'hash': wrong['archive_sha256'] = '0' * 64
            try:
                verify_parts(folder, wrong)
                raise AssertionError('Invalid archive manifest accepted')
            except ValueError:
                pass
            checked.append(change+'_rejected')
        try:
            assemble(folder, manifest, output)
            raise AssertionError('Existing output overwritten')
        except FileExistsError:
            checked.append('existing_output_preserved')
    evidence = root/'docs/quality/evidence/batch-round-163/media-parts-check.json'
    assert not evidence.exists()
    result = {'passed': True, 'checks': checked, 'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'helper_sha256': hashlib.sha256((Path(__file__).parent/'release_archive_parts164.py').read_bytes()).hexdigest(), 'full_goal_complete': False, 'virtualization_used': False}
    evidence.write_text(json.dumps(result, indent=2)+'\n', encoding='utf8')
    print(json.dumps({'passed': True, 'checks': len(checked)}))


if __name__ == '__main__':
    main()
