import concurrent.futures
import importlib.util
import os
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "runtime_tmp", Path(__file__).with_name("runtime_tmp.py")
)
layout = importlib.util.module_from_spec(spec)
spec.loader.exec_module(layout)


class RuntimeTmpTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        self.home = self.root / "data" / "agent"
        self.home.mkdir(parents=True)
        self.local = self.root / "local"
        self.local.mkdir()

    def prepare(self, home=None):
        return layout.prepare(str(home or self.home), str(self.local))

    def test_preserves_durable_files_and_is_idempotent(self):
        for name in ("auth.json", "config.toml", "state.sqlite", "sessions"):
            (self.home / name).write_text("unchanged")
        self.assertEqual(self.prepare(), "isolated")
        target = (self.home / "tmp").resolve()
        self.assertTrue(target.is_relative_to(self.local))
        self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o700)
        self.assertEqual(self.prepare(), "isolated")
        self.assertEqual(target, (self.home / "tmp").resolve())
        for name in ("auth.json", "config.toml", "state.sqlite", "sessions"):
            self.assertEqual((self.home / name).read_text(), "unchanged")

    def test_distinct_agents_do_not_share_tmp(self):
        other = self.root / "data" / "other-agent"
        other.mkdir()
        self.prepare()
        self.prepare(other)
        self.assertNotEqual(os.readlink(self.home / "tmp"), os.readlink(other / "tmp"))

    def test_concurrent_preparation_keeps_one_mapping(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda _: self.prepare(), range(32)))
        self.assertEqual(set(results), {"isolated"})
        self.assertEqual(len(list(self.local.iterdir())), 1)

    def test_keeps_existing_local_tmp(self):
        (self.home / "tmp").mkdir()
        (self.home / "tmp" / ".lock").write_text("owned by another process")
        self.assertEqual(self.prepare(), "local")
        self.assertFalse((self.home / "tmp").is_symlink())
        self.assertEqual((self.home / "tmp" / ".lock").read_text(), "owned by another process")
        self.assertEqual(list(self.local.iterdir()), [])

    def test_rejects_legacy_network_tmp_without_touching_it(self):
        (self.home / "tmp").mkdir()
        (self.home / "tmp" / ".lock").write_text("must not touch")
        with patch.object(layout, "is_local", return_value=False):
            with self.assertRaisesRegex(layout.LayoutError, "legacy_tmp_requires_drain"):
                self.prepare()
        self.assertEqual((self.home / "tmp" / ".lock").read_text(), "must not touch")
        self.assertEqual(list(self.local.iterdir()), [])

    def test_rejects_network_runtime_parent_before_creating_entries(self):
        with patch.object(layout, "is_local", return_value=False):
            with self.assertRaisesRegex(layout.LayoutError, "runtime_root_not_local"):
                self.prepare()
        self.assertEqual(list(self.local.iterdir()), [])
        self.assertFalse((self.home / "tmp").is_symlink())

    def test_rejects_unknown_filesystem(self):
        with patch.object(layout, "filesystem_type", return_value="fuse.unknown"):
            with self.assertRaisesRegex(layout.LayoutError, "runtime_root_not_local"):
                self.prepare()

    def test_rejects_arbitrary_tmp_symlink(self):
        outside = self.root / "outside"
        outside.mkdir()
        (self.home / "tmp").symlink_to(outside)
        with self.assertRaisesRegex(layout.LayoutError, "unexpected_tmp_link"):
            self.prepare()
        self.assertEqual(list(outside.iterdir()), [])
        self.assertEqual(list(self.local.iterdir()), [])

    def test_rejects_dangling_arbitrary_tmp_symlink(self):
        (self.home / "tmp").symlink_to(self.root / "missing")
        with self.assertRaisesRegex(layout.LayoutError, "unexpected_tmp_link"):
            self.prepare()
        self.assertFalse((self.root / "missing").exists())

    def test_recreates_only_its_own_missing_local_target(self):
        self.prepare()
        target = (self.home / "tmp").resolve()
        target.rmdir()
        self.assertTrue((self.home / "tmp").is_symlink())
        self.assertEqual(self.prepare(), "isolated")
        self.assertEqual(target, (self.home / "tmp").resolve())
        self.assertTrue(target.is_dir())

    def test_rejects_runtime_parent_symlink(self):
        outside = self.root / "outside"
        outside.mkdir()
        (self.local / f"memoh-codex-runtime-{os.geteuid()}").symlink_to(outside)
        with self.assertRaises(OSError):
            self.prepare()
        self.assertEqual(list(outside.iterdir()), [])
        self.assertFalse((self.home / "tmp").is_symlink())

    def test_rejects_replaced_target_symlink(self):
        self.prepare()
        target = (self.home / "tmp").resolve()
        target.rmdir()
        outside = self.root / "outside"
        outside.mkdir()
        target.symlink_to(outside)
        with self.assertRaises(OSError):
            self.prepare()
        self.assertEqual(list(outside.iterdir()), [])

    def test_rejects_runtime_parent_with_unsafe_permissions(self):
        parent = self.local / f"memoh-codex-runtime-{os.geteuid()}"
        parent.mkdir(mode=0o777)
        parent.chmod(0o777)
        with self.assertRaisesRegex(layout.LayoutError, "unsafe_local_permissions"):
            self.prepare()
        self.assertEqual(stat.S_IMODE(parent.stat().st_mode), 0o777)
        self.assertEqual(list(parent.iterdir()), [])

    def test_rejects_symlink_in_home_ancestors(self):
        alias = self.root / "alias"
        alias.symlink_to(self.home.parent)
        with self.assertRaises(OSError):
            self.prepare(alias / self.home.name)
        self.assertEqual(list(self.home.iterdir()), [])

    def test_rejects_tmp_file(self):
        (self.home / "tmp").write_text("keep")
        with self.assertRaisesRegex(layout.LayoutError, "unsafe_tmp_entry"):
            self.prepare()
        self.assertEqual((self.home / "tmp").read_text(), "keep")

    def test_rejects_noncanonical_path(self):
        with self.assertRaisesRegex(layout.LayoutError, "unsafe_path"):
            layout.prepare(str(self.home) + "/../agent", str(self.local))
        with self.assertRaisesRegex(layout.LayoutError, "unsafe_path"):
            layout.prepare("relative", str(self.local))

    def test_mount_id_lookup_uses_real_opened_directory(self):
        with layout.directory(str(self.local)) as fd:
            self.assertIn(layout.filesystem_type(fd), layout.LOCAL_FILESYSTEMS)


if __name__ == "__main__":
    unittest.main()
