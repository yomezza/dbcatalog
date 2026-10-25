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
    args = ['gh', 'api', path, '--method', method, '--include']
    data = None
    if payload is not None:
        args += ['--input', '-']
        data = json.dumps(payload).encode()
    result = subprocess.run(args, input=data, capture_output=True, cwd=ROOT)
    response = result.stdout.decode(errors='replace')
    parts = re.split(r'\r?\n\r?\n', response, maxsplit=1)
    if result.returncode:
        # Only permission-related response headers are exposed; never request headers.
        permissions = [line for line in parts[0].splitlines()
                       if line.lower().startswith(('x-accepted-github-permissions:', 'x-oauth-scopes:'))]
        detail = result.stderr.decode(errors='replace').strip()
        if permissions:
            detail += '; ' + '; '.join(permissions)
        raise RuntimeError(f'{method} {path}: {detail}')
    return json.loads(parts[1] if len(parts) == 2 else response)


def latest_setting(release):
    """An absent setting preserves an existing release's GitHub label.

    New ordinary releases can request make_latest: "true" (Latest) or "false"
    (None). Historical entries need no rewrite when this policy is introduced.
    """
    if 'make_latest' not in release:
        return None
    value = release['make_latest']
    if value not in ('true', 'false'):
        raise ValueError('make_latest must be the string "true" or "false"')
    if value == 'true' and release['prerelease']:
        raise ValueError('A prerelease cannot be marked Latest')
    return value


def validate(manifest):
    versions = set()
    assets_count = 0
    for release in manifest['releases']:
        latest_setting(release)
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
    # Reject an invalid policy before modifying even the first historical entry.
    for release in manifest['releases']:
        latest_setting(release)
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
            payload = {
                'tag_name': tag,
                'name': release['title'], 'body': release['body'],
                'draft': True, 'prerelease': release['prerelease'], 'make_latest': 'false',
            }
            # Existing version tags already identify the exact source commit.
            if ref.returncode != 0:
                payload['target_commitish'] = release['commit']
            current = api(f'repos/{repo}/releases', 'POST', payload)
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
        metadata = {
            'name': release['title'], 'body': release['body'],
            'draft': False,
        }
        requested_latest = latest_setting(release)
        # Old published entries have no new label policy. Preserve their actual
        # prerelease/Latest state, including a label changed manually in GitHub.
        if requested_latest is not None or current['draft']:
            metadata['prerelease'] = release['prerelease']
        if requested_latest is not None:
            metadata['make_latest'] = requested_latest
        api(f'repos/{repo}/releases/{current["id"]}', 'PATCH', metadata)
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
