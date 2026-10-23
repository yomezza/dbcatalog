#!/usr/bin/env python3
"""Validate historical builds; publish them only with explicit --publish."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def run(*args, data=None):
    result = subprocess.run(args, input=data, capture_output=True, cwd=ROOT)
    if result.returncode:
        raise RuntimeError(f'{args[0]} failed: {result.stderr.decode(errors="replace")}')
    return result.stdout


def api(path, method='GET', payload=None):
    args = ['gh', 'api', path, '--method', method]
    data = None
    if payload is not None:
        args += ['--input', '-']
        data = json.dumps(payload).encode()
    try:
        return json.loads(run(*args, data=data))
    except RuntimeError as error:
        raise RuntimeError(f'{method} {path}: {error}') from error


def validate(manifest):
    versions = set()
    assets_count = 0
    for release in manifest['releases']:
        version, commit = release['version'], release['commit']
        if not re.fullmatch(r'\d+\.\d+\.\d+', version) or version in versions:
            raise ValueError(f'Invalid/duplicate version: {version}')
        versions.add(version)
        if not re.fullmatch(r'[0-9a-f]{40}', commit):
            raise ValueError('A full source commit is required')
        run('git', 'cat-file', '-e', commit + '^{commit}')
        for name in release['assets']:
            path = ROOT / name
            if Path(name).parent != Path('downloads') or not path.is_file():
                raise ValueError(f'Invalid asset: {name}')
            local = path.read_bytes()
            historical = run('git', 'show', f'{commit}:{name}')
            if local != historical:
                raise ValueError(f'Asset differs from source release: {name}')
            if name.endswith('.sha256'):
                checksum, filename = local.decode().strip().split(maxsplit=1)
                target = path.parent / filename.lstrip('*')
                if target.name != path.name.removesuffix('.sha256'):
                    raise ValueError(f'Unexpected checksum target: {name}')
                if hashlib.sha256(target.read_bytes()).hexdigest() != checksum:
                    raise ValueError(f'Checksum mismatch: {name}')
            elif name + '.sha256' not in release['assets']:
                raise ValueError(f'Missing checksum: {name}')
            assets_count += 1
    print(f'Validated {len(versions)} versions and {assets_count} assets', flush=True)


def publish(manifest):
    repo = manifest['repository']
    if os.environ.get('GITHUB_REPOSITORY', repo) != repo:
        raise ValueError('Repository does not match the release manifest')
    existing = json.loads(run('gh', 'api', '--paginate', '--slurp', f'repos/{repo}/releases'))
    by_tag = {r['tag_name']: r for page in existing for r in page}
    for release in manifest['releases']:
        tag = 'v' + release['version']
        # Checkout fetches tags. Never move a pre-existing tag to another source commit.
        ref = subprocess.run(['git', 'rev-parse', '--verify', f'refs/tags/{tag}^{{commit}}'], cwd=ROOT, capture_output=True)
        if ref.returncode == 0 and ref.stdout.decode().strip() != release['commit']:
            raise ValueError(f'Tag points to a different commit: {tag}')
        current = by_tag.get(tag)
        if current is None:
            current = api(f'repos/{repo}/releases', 'POST', {
                'tag_name': tag, 'target_commitish': release['commit'],
                'name': release['title'], 'body': release['body'],
                'draft': True, 'prerelease': release['prerelease'], 'make_latest': 'false',
            })
        remote_assets = {asset['name']: asset for asset in current['assets']}
        for name in release['assets']:
            path = ROOT / name
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            remote = remote_assets.get(path.name)
            if remote:
                if remote['size'] != path.stat().st_size:
                    raise ValueError(f'Refusing to replace existing asset: {tag}/{path.name}')
                actual = remote.get('digest')
                if not actual:
                    with tempfile.TemporaryDirectory() as directory:
                        run('gh', 'release', 'download', tag, '--repo', repo, '--pattern', path.name, '--dir', directory)
                        actual = 'sha256:' + hashlib.sha256((Path(directory) / path.name).read_bytes()).hexdigest()
                if actual != 'sha256:' + digest:
                    raise ValueError(f'Refusing to replace different asset: {tag}/{path.name}')
            else:
                run('gh', 'release', 'upload', tag, str(path), '--repo', repo)
        api(f'repos/{repo}/releases/{current["id"]}', 'PATCH', {
            'name': release['title'], 'body': release['body'],
            'draft': False, 'prerelease': release['prerelease'], 'make_latest': 'false',
        })
        published = api(f'repos/{repo}/releases/{current["id"]}')
        if published['draft']:
            raise ValueError(f'Release is still a draft: {tag}')
        uploaded = {asset['name']: asset for asset in published['assets']}
        for name in release['assets']:
            path = ROOT / name
            remote = uploaded.get(path.name)
            if remote is None or remote.get('state') != 'uploaded' or remote['size'] != path.stat().st_size:
                raise ValueError(f'Release asset is missing or incomplete: {tag}/{path.name}')
            digest = remote.get('digest')
            if digest and digest != 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest():
                raise ValueError(f'Uploaded checksum differs: {tag}/{path.name}')
        print(f'Published and verified https://github.com/{repo}/releases/tag/{tag}', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--publish', action='store_true')
    args = parser.parse_args()
    os.chdir(ROOT)
    manifest = json.loads((ROOT / 'packaging/releases/manifest.json').read_text())
    validate(manifest)
    if args.publish:
        try:
            publish(manifest)
        except Exception as error:
            # Surface API failures in public Actions annotations, even when logs
            # require sign-in. The message contains no token or authentication headers.
            message = str(error).replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
            print('::error title=Release publication failed::' + message, flush=True)
            raise
