import concurrent.futures
import contextlib
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("layout", Path(__file__).with_name("runtime_storage.py"))
layout = importlib.util.module_from_spec(spec)
spec.loader.exec_module(layout)


class RuntimeStorageTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        self.data = self.root / "network"
        self.data.mkdir()
        self.volume = self.root / "volume"
        self.volume.mkdir()
        self.home = self.data / "agents" / "one"
        self.volumes = [self.volume]
        original_fs = layout.filesystem_type
        original_mount = layout.mount_id

        def where(fd):
            return Path(os.readlink(f"/proc/self/fd/{fd}"))

        def filesystem(fd):
            value = where(fd)
            if value.is_relative_to(self.data):
                return "nfs"
            if any(value.is_relative_to(root) for root in self.volumes):
                return "ext4"
            return original_fs(fd)

        def mount(fd):
            value = where(fd)
            for root in self.volumes:
                if value.is_relative_to(root):
                    return str(root)
            return original_mount(fd)

        self.addCleanup(patch.stopall)
        patch.object(layout, "filesystem_type", side_effect=filesystem).start()
        patch.object(layout, "mount_id", side_effect=mount).start()

    def prepare(self):
        return layout.prepare(str(self.home), str(self.volume))

    def legacy(self):
        self.home.mkdir(parents=True)
        (self.home / "auth.json").write_text("credential-sentinel")
        (self.home / "installation_id").write_text("391b6b43-9c04-4ab2-b95c-ed970aedfd89")
        (self.home / "sessions").mkdir()
        (self.home / "sessions" / "thread.jsonl").write_text('{"preserve":"native session"}\n')
        (self.home / "tmp").mkdir()
        (self.home / "tmp" / ".lock").write_text("preserve old lock")

    def test_new_local_home_keeps_original_layout(self):
        home = self.root / "local" / "agent"
        self.assertEqual(layout.prepare(str(home), str(self.volume)), "local")
        self.assertTrue(home.is_dir())
        self.assertFalse(home.is_symlink())
        self.assertFalse((home / "tmp").exists())
        self.assertEqual(list(self.volume.iterdir()), [])

    def test_existing_local_home_keeps_native_data(self):
        home = self.root / "local"
        home.mkdir()
        (home / "tmp").mkdir()
        (home / "state.sqlite").write_text("unchanged")
        self.assertEqual(layout.prepare(str(home)), "local")
        self.assertEqual((home / "state.sqlite").read_text(), "unchanged")

    def test_new_network_home_uses_persistent_local_volume(self):
        self.assertEqual(self.prepare(), "isolated")
        target = self.home.resolve()
        self.assertTrue(target.is_relative_to(self.volume))
        (self.home / "auth.json").write_text("preserve")
        (self.home / "installation_id").write_text("stable")
        self.assertEqual(self.prepare(), "isolated")
        self.assertEqual(target, self.home.resolve())
        self.assertEqual((self.home / "auth.json").read_text(), "preserve")
        self.assertEqual((self.home / "installation_id").read_text(), "stable")

    def test_concurrent_preparation_publishes_one_home(self):
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda _: self.prepare(), range(32)))
        self.assertEqual(set(results), {"isolated"})
        self.assertEqual(len(list(self.volume.glob("*/*/"))), 1)

    def test_missing_volume_does_not_create_network_home(self):
        self.volume.rmdir()
        with self.assertRaisesRegex(layout.LayoutError, "native_volume_required"):
            self.prepare()
        self.assertFalse(self.home.exists())

    def test_rejects_ephemeral_or_network_volume(self):
        for kind in ("tmpfs", "overlay", "nfs", "nfs4", "fuse.unknown"):
            with self.subTest(kind=kind), patch.object(layout, "filesystem_type", return_value=kind):
                with self.assertRaisesRegex(layout.LayoutError, "native_volume_not_persistent_local"):
                    with layout.native_volume(str(self.volume)):
                        pass
        self.assertEqual(list(self.volume.iterdir()), [])

    def test_requires_separate_mount(self):
        with patch.object(layout, "mount_id", return_value="same"):
            with self.assertRaisesRegex(layout.LayoutError, "native_volume_not_persistent_local"):
                self.prepare()

    def test_rejects_unsafe_volume_permissions(self):
        self.volume.chmod(0o777)
        with self.assertRaisesRegex(layout.LayoutError, "unsafe_local_permissions"):
            self.prepare()
        self.assertEqual(list(self.volume.iterdir()), [])

    def test_rejects_unknown_home_link(self):
        self.home.parent.mkdir()
        self.home.symlink_to(self.root / "other")
        with self.assertRaisesRegex(layout.LayoutError, "native_volume_identity_mismatch"):
            self.prepare()
        self.assertFalse((self.root / "other").exists())

    def test_rejects_symlinked_ancestor(self):
        outside = self.root / "outside"
        outside.mkdir()
        self.home.parent.symlink_to(outside)
        with self.assertRaises(OSError):
            self.prepare()
        self.assertEqual(list(outside.iterdir()), [])

    def test_rejects_symlinked_volume(self):
        self.volume.rmdir()
        self.volume.symlink_to(self.root)
        with self.assertRaises(OSError):
            self.prepare()

    def test_refuses_replacement_empty_volume(self):
        self.prepare()
        link = os.readlink(self.home)
        shutil.rmtree(self.volume)
        self.volume.mkdir()
        with self.assertRaisesRegex(layout.LayoutError, "native_volume_identity_mismatch"):
            self.prepare()
        self.assertEqual(os.readlink(self.home), link)

    def test_refuses_missing_native_state(self):
        self.prepare()
        shutil.rmtree(self.home.resolve())
        with self.assertRaisesRegex(layout.LayoutError, "native_state_missing"):
            self.prepare()
        self.assertFalse(self.home.exists())

    def test_refuses_changed_home_identity(self):
        self.prepare()
        marker = self.home / layout.MARKER
        marker.write_text('{"version":1,"home":"some-other-agent"}')
        with self.assertRaisesRegex(layout.LayoutError, "invalid_storage_identity"):
            self.prepare()

    def test_refuses_replaced_volume_identity_file(self):
        self.prepare()
        marker = self.volume / ".memoh-volume.json"
        marker.unlink()
        marker.symlink_to(self.root / "outside")
        with self.assertRaises(OSError):
            self.prepare()

    def test_legacy_network_home_requires_drain_without_mutation(self):
        self.legacy()
        before = self.tree(self.home)
        with self.assertRaisesRegex(layout.LayoutError, "native_home_requires_drain"):
            self.prepare()
        self.assertEqual(self.tree(self.home), before)
        self.assertEqual(list(self.volume.iterdir()), [])

    def test_offline_migration_preserves_wal_identity_credentials_and_sessions(self):
        self.legacy()
        # A committed WAL from an exited writer must migrate without locking
        # the original NFS database. os._exit avoids automatic WAL checkpoint.
        code = "import sqlite3,os; c=sqlite3.connect(%r); c.execute('PRAGMA journal_mode=WAL'); c.execute('CREATE TABLE x(v)'); c.execute('INSERT INTO x VALUES (42)'); c.commit(); os._exit(0)" % str(self.home / "state_5.sqlite")
        subprocess.run([sys.executable, "-c", code], check=True)
        self.assertTrue((self.home / "state_5.sqlite-wal").exists())
        before = self.tree(self.home)
        self.assertEqual(layout.migrate_drained(str(self.home), str(self.volume)), "migrated")
        self.assertEqual(self.prepare(), "isolated")
        archives = list(self.home.parent.glob("one.before-native-storage-*"))
        self.assertEqual(len(archives), 1)
        self.assertEqual(self.tree(archives[0]), before)
        self.assertFalse((self.home / "tmp").exists())
        self.assertEqual((self.home / "auth.json").read_text(), "credential-sentinel")
        self.assertEqual((self.home / "installation_id").read_text(), "391b6b43-9c04-4ab2-b95c-ed970aedfd89")
        with contextlib.closing(sqlite3.connect(self.home / "state_5.sqlite")) as db:
            self.assertEqual(db.execute('SELECT v FROM x').fetchall(), [(42,)])

    def test_migration_keeps_nested_tmp_directories(self):
        self.legacy()
        folder = self.home / "plugin" / "tmp"
        folder.mkdir(parents=True)
        (folder / "keep").write_text("keep")
        layout.migrate_drained(str(self.home), str(self.volume))
        self.assertEqual((self.home / "plugin/tmp/keep").read_text(), "keep")

    def test_migration_rejects_running_owner(self):
        self.legacy()
        child = subprocess.Popen([sys.executable, "-c", "import time;time.sleep(30)"], env={**os.environ, "CODEX_HOME": str(self.home)})
        try:
            with self.assertRaisesRegex(layout.LayoutError, "native_home_in_use"):
                layout.migrate_drained(str(self.home), str(self.volume))
        finally:
            child.kill()
            child.wait()
        self.assertFalse(self.home.is_symlink())

    def test_migration_rejects_links_and_preserves_source(self):
        self.legacy()
        (self.home / "outside").symlink_to(self.root)
        with self.assertRaisesRegex(layout.LayoutError, "unsupported_migration_entry"):
            layout.migrate_drained(str(self.home), str(self.volume))
        self.assertFalse(self.home.is_symlink())
        self.assertEqual((self.home / "auth.json").read_text(), "credential-sentinel")

    def test_missing_anchor_does_not_reuse_orphaned_native_state(self):
        self.prepare()
        (self.home / "state_5.sqlite").write_text("do not overwrite")
        target = self.home.resolve()
        self.home.unlink()
        with self.assertRaisesRegex(layout.LayoutError, "orphaned_native_state"):
            self.prepare()
        self.assertEqual((target / "state_5.sqlite").read_text(), "do not overwrite")

    def test_rejects_noncanonical_path(self):
        for home in ("relative", str(self.home) + "/../one", "//data/home"):
            with self.assertRaisesRegex(layout.LayoutError, "unsafe_path"):
                layout.prepare(home, str(self.volume))

    def test_migration_rejects_external_sqlite_home(self):
        self.legacy()
        (self.home / "config.toml").write_text('sqlite_home = "/elsewhere"\n')
        with self.assertRaisesRegex(layout.LayoutError, "unsupported_migration_entry"):
            layout.migrate_drained(str(self.home), str(self.volume))
        self.assertFalse(self.home.is_symlink())
        self.assertEqual((self.home / "auth.json").read_text(), "credential-sentinel")

    def test_corrupt_database_does_not_publish_migration(self):
        self.legacy()
        (self.home / "state_5.sqlite").write_bytes(b"invalid database")
        before = self.tree(self.home)
        with self.assertRaises(sqlite3.Error):
            layout.migrate_drained(str(self.home), str(self.volume))
        self.assertEqual(self.tree(self.home), before)
        self.assertFalse(self.home.is_symlink())
        self.assertEqual(list(self.home.parent.glob("*.before-native-storage-*")), [])

    def test_legacy_local_tmp_link_can_recreate_only_transient_target(self):
        home = self.root / "local"
        home.mkdir()
        temp_parent = self.root / "temporary"
        temp_parent.mkdir()
        expected = temp_parent / f"memoh-codex-runtime-{os.geteuid()}" / hashlib.sha256(str(home).encode()).hexdigest()
        (home / "tmp").symlink_to(expected)
        self.assertFalse(expected.exists())
        self.assertEqual(layout.prepare_local_tmp(str(home), str(temp_parent)), "local")
        self.assertTrue(expected.is_dir())
        self.assertEqual(os.readlink(home / "tmp"), str(expected))

    def test_legacy_local_tmp_rejects_arbitrary_link(self):
        home = self.root / "local"
        home.mkdir()
        (home / "tmp").symlink_to(self.root)
        with self.assertRaisesRegex(layout.LayoutError, "unexpected_tmp_link"):
            layout.prepare_local_tmp(str(home))

    @staticmethod
    def tree(root):
        return {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob("*") if p.is_file()}


if __name__ == "__main__":
    unittest.main()
