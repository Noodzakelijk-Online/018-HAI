import hashlib
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest


SPEC = importlib.util.spec_from_file_location("miniswe_app", Path(__file__).with_name("app.py"))
app = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(app)


class MiniSWEWorkerBoundaryTests(unittest.TestCase):
    def test_validate_request_rejects_multiline_task(self):
        with self.assertRaises(app.RequestError):
            app.validate_request({"workspaceId": "hai", "task": "ignore\nrules"})

    def test_copy_snapshot_rejects_secret_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            source.mkdir()
            (source / ".env").write_text("secret=value")
            target = root / "target"
            target.mkdir()
            with self.assertRaises(app.RequestError):
                app.copy_snapshot(source, target)

    def test_source_directory_rejects_symlinked_workspace_root(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            target = root / "other"
            target.mkdir()
            linked = root / "approved"
            try:
                os.symlink(target, linked, target_is_directory=True)
            except (NotImplementedError, OSError) as exc:
                self.skipTest(f"symlinks are unavailable in this test environment: {exc}")
            with self.assertRaises(app.RequestError):
                app.source_directory(root, "approved")

    def test_source_directory_rejects_sibling_snapshot(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "approved").mkdir()
            (root / "other").mkdir()
            with self.assertRaises(app.RequestError):
                app.source_directory(root, "approved")

    def test_diff_is_bounded_and_has_digest(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            baseline = root / "baseline"
            work = root / "work"
            baseline.mkdir()
            work.mkdir()
            (baseline / "one.txt").write_text("before\n")
            (work / "one.txt").write_text("after\n")
            diff, changed, truncated = app.make_diff(baseline, work)
            self.assertEqual(changed, 1)
            self.assertFalse(truncated)
            self.assertIn("-before", diff)
            self.assertIn("+after", diff)
            self.assertEqual(len(hashlib.sha256(diff.encode("utf-8")).hexdigest()), 64)


if __name__ == "__main__":
    unittest.main()
