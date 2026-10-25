"""Release policy contract tests. GitHub and git calls are simulated; no writes."""
import contextlib
import copy
import importlib.util
import io
import json
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch

MODULE_PATH = Path(__file__).with_name('publish-releases.py')
SPEC = importlib.util.spec_from_file_location('catalogdb_publish_releases', MODULE_PATH)
publisher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publisher)

COMMIT = 'a' * 40
REPOSITORY = 'example/catalogdb'


def release(version, *, prerelease=True, latest=None):
    record = {
        'version': version, 'commit': COMMIT, 'title': 'CatalogDB ' + version,
        'body': 'Release description', 'prerelease': prerelease, 'assets': [],
    }
    if latest is not None:
        record['make_latest'] = latest
    return record


class ReleasePolicyTests(unittest.TestCase):
    def publish(self, releases, existing):
        """Exercise the real publish loop, capturing exact GitHub payloads."""
        states = {record['id']: copy.deepcopy(record) for record in existing}
        calls = []

        def api(path, method='GET', payload=None):
            calls.append((path, method, copy.deepcopy(payload)))
            if method == 'POST':
                created = {'id': 100, 'assets': [], **copy.deepcopy(payload)}
                states[created['id']] = created
                return copy.deepcopy(created)
            release_id = int(path.rsplit('/', 1)[-1])
            if method == 'PATCH':
                states[release_id].update(copy.deepcopy(payload))
            return copy.deepcopy(states[release_id])

        manifest = {'repository': REPOSITORY, 'releases': releases}
        ref = SimpleNamespace(returncode=0, stdout=(COMMIT + '\n').encode())
        with patch.dict(publisher.os.environ, {'GITHUB_REPOSITORY': REPOSITORY}), \
                patch.object(publisher, 'run', return_value=json.dumps([existing]).encode()) as run, \
                patch.object(publisher.subprocess, 'run', return_value=ref), \
                patch.object(publisher, 'api', side_effect=api), \
                contextlib.redirect_stdout(io.StringIO()):
            publisher.publish(manifest)
        # No upload or tag rewrite is needed: existing tags identify the commits.
        self.assertEqual(run.call_count, 1)
        return calls, states

    def test_latest_only_applies_to_new_release_and_draft_is_never_latest(self):
        historical = release('0.12.0')
        requested = release('0.13.0', prerelease=False, latest='true')
        before = copy.deepcopy([historical, requested])
        calls, states = self.publish([historical, requested], [{
            'id': 12, 'tag_name': 'v0.12.0', 'draft': False,
            'prerelease': True, 'assets': [],
        }])
        old_patch = next(payload for path, method, payload in calls
                         if method == 'PATCH' and path.endswith('/12'))
        self.assertNotIn('make_latest', old_patch)
        self.assertNotIn('prerelease', old_patch)
        self.assertTrue(states[12]['prerelease'])
        created = next(payload for _, method, payload in calls if method == 'POST')
        self.assertEqual(created['make_latest'], 'false')
        self.assertTrue(created['draft'])
        self.assertFalse(created['prerelease'])
        self.assertNotIn('target_commitish', created)
        new_patch = next(payload for path, method, payload in calls
                         if method == 'PATCH' and path.endswith('/100'))
        self.assertEqual(new_patch['make_latest'], 'true')
        self.assertFalse(states[100]['draft'])
        self.assertFalse(states[100]['prerelease'])
        self.assertEqual([historical, requested], before)

    def test_explicit_none_is_an_ordinary_release_not_a_prerelease(self):
        calls, _ = self.publish([release('0.13.0', prerelease=False, latest='false')], [{
            'id': 13, 'tag_name': 'v0.13.0', 'draft': False,
            'prerelease': False, 'assets': [],
        }])
        self.assertFalse(any(method == 'POST' for _, method, _ in calls))
        metadata = next(payload for _, method, payload in calls if method == 'PATCH')
        self.assertEqual(metadata['make_latest'], 'false')
        self.assertIs(metadata['prerelease'], False)

    def test_absent_setting_preserves_existing_ordinary_release_label(self):
        calls, _ = self.publish([release('0.13.0', prerelease=False)], [{
            'id': 13, 'tag_name': 'v0.13.0', 'draft': False,
            'prerelease': False, 'assets': [], 'make_latest': 'true',
        }])
        metadata = next(payload for _, method, payload in calls if method == 'PATCH')
        self.assertNotIn('make_latest', metadata)
        self.assertNotIn('prerelease', metadata)

    def test_historical_label_manually_changed_in_github_is_not_reset(self):
        # A historical manifest can still say prerelease after the owner has
        # changed its GitHub label. The new policy applies to future entries.
        calls, states = self.publish([release('0.12.0')], [{
            'id': 12, 'tag_name': 'v0.12.0', 'draft': False,
            'prerelease': False, 'assets': [], 'make_latest': 'true',
        }])
        metadata = next(payload for _, method, payload in calls if method == 'PATCH')
        self.assertNotIn('make_latest', metadata)
        self.assertNotIn('prerelease', metadata)
        self.assertFalse(states[12]['prerelease'])
        self.assertEqual(states[12]['make_latest'], 'true')

    def test_impossible_latest_policy_is_rejected_before_any_github_call(self):
        manifest = {'repository': REPOSITORY, 'releases': [
            release('0.12.0'), release('0.13.0', latest='true'),
        ]}
        with patch.dict(publisher.os.environ, {'GITHUB_REPOSITORY': REPOSITORY}), \
                patch.object(publisher, 'run') as run, patch.object(publisher, 'api') as api:
            with self.assertRaisesRegex(ValueError, 'prerelease'):
                publisher.publish(manifest)
            run.assert_not_called()
            api.assert_not_called()

    def test_invalid_api_values_are_rejected_instead_of_coerced(self):
        for value in [True, False, None, 0, 1, 'legacy', 'TRUE', '']:
            record = release('0.13.0', prerelease=False)
            record['make_latest'] = value
            with self.subTest(value=value), self.assertRaises(ValueError):
                publisher.latest_setting(record)


if __name__ == '__main__':
    unittest.main()
