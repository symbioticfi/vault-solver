"""Ensure an acknowledged gap only silences the one live state it names."""

import importlib.util
import pathlib
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("drift", pathlib.Path(__file__).with_name("check-schema-drift.py"))
drift = importlib.util.module_from_spec(spec)
spec.loader.exec_module(drift)

VENDORED = '{"paths": {"/a": {}}}'
LIVE = '{"paths": {"/b": {}}}'


class ExpectedDriftTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = pathlib.Path(directory.name)
        (root / "openapi").mkdir()
        (root / "openapi/x.json").write_text(VENDORED)
        patcher = mock.patch.object(drift, "ROOT", root)
        patcher.start()
        self.addCleanup(patcher.stop)

    def check(self, live, expected=None):
        source = {"name": "x", "vendored": "openapi/x.json", "mode": "auto", "fetch": "json",
                  "url": "invalid://unused", "refresh": "make x"}
        if expected:
            source["expectedDrift"] = expected
        report = []
        with mock.patch.object(drift, "fetch_live", return_value=live):
            drifted = drift.check(source, report)
        return drifted, "\n".join(report)

    def live_digest(self, live):
        return drift.digest(drift.normalize(live, "json"))

    def test_states(self):
        acknowledged = {"reason": "pending deploy", "liveDigest": self.live_digest(LIVE)}
        other = {"reason": "pending deploy", "liveDigest": self.live_digest('{"other": 1}')}
        cases = [
            ("in sync", VENDORED, None, False, "up to date"),
            ("unacknowledged", LIVE, None, True, "DRIFTED"),
            ("acknowledged live state", LIVE, acknowledged, False, "pending deploy"),
            ("live moved on", LIVE, other, True, "Live changed since the acknowledged"),
            ("stale acknowledgement", VENDORED, acknowledged, True, "stale"),
        ]
        for name, live, expected, want_drift, want_text in cases:
            with self.subTest(name):
                drifted, report = self.check(live, expected)
                self.assertEqual(drifted, want_drift, report)
                self.assertIn(want_text, report)

    def test_digest_ignores_key_order(self):
        self.assertEqual(self.live_digest('{"a": 1, "b": 2}'), self.live_digest('{"b": 2, "a": 1}'))


if __name__ == "__main__":
    unittest.main()
