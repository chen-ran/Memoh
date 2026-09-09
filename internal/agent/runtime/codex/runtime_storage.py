"""Keep Codex's native home on a workspace-local, persistent filesystem.

Normal startup never migrates an existing network home. --migrate-drained is an
operator-only offline operation, after all owners have stopped admitting work.
"""
import contextlib
import hashlib
import fcntl
import json
import os
import shutil
import sqlite3
import stat
import sys
import tomllib
import uuid

LOCAL_FILESYSTEMS = frozenset(("ext2", "ext3", "ext4", "xfs", "btrfs", "tmpfs", "ramfs", "overlay", "aufs", "zfs", "f2fs"))
PERSISTENT_FILESYSTEMS = LOCAL_FILESYSTEMS - {"tmpfs", "ramfs", "overlay", "aufs"}
NATIVE_ROOT = "/var/lib/memoh/codex-state"
DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
FILE_FLAGS = os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK
MARKER = ".memoh-native-home.json"


class LayoutError(Exception):
    pass


def mount_id(fd):
    with open(f"/proc/self/fdinfo/{fd}", encoding="ascii") as info:
        value = next((line.split()[1] for line in info if line.startswith("mnt_id:")), None)
    if value is None:
        raise LayoutError("unknown_filesystem")
    return value


def filesystem_type(fd):
    ident = mount_id(fd)
    with open("/proc/self/mountinfo", encoding="utf-8") as mounts:
        for line in mounts:
            fields = line.split()
            if fields[0] == ident:
                return fields[fields.index("-") + 1]
    raise LayoutError("unknown_filesystem")


def is_local(fd):
    return filesystem_type(fd) in LOCAL_FILESYSTEMS


@contextlib.contextmanager
def directory(path, create=False):
    if not os.path.isabs(path) or os.path.normpath(path) != path or path.startswith("//"):
        raise LayoutError("unsafe_path")
    fd = os.open("/", DIR_FLAGS)
    try:
        for part in path.split("/")[1:]:
            if not part:
                continue
            if create:
                try:
                    os.mkdir(part, mode=0o700, dir_fd=fd)
                except FileExistsError:
                    pass
            child = os.open(part, DIR_FLAGS, dir_fd=fd)
            os.close(fd)
            fd = child
        yield fd
    finally:
        os.close(fd)


def owned(fd, private=False):
    info = os.fstat(fd)
    mode = stat.S_IMODE(info.st_mode)
    if info.st_uid != os.geteuid() or (mode != 0o700 if private else mode & 0o022):
        raise LayoutError("unsafe_local_permissions")


@contextlib.contextmanager
def private_child(parent, name):
    try:
        os.mkdir(name, mode=0o700, dir_fd=parent)
    except FileExistsError:
        pass
    fd = os.open(name, DIR_FLAGS, dir_fd=parent)
    try:
        owned(fd, private=True)
        if not is_local(fd):
            raise LayoutError("runtime_root_not_local")
        yield fd
    finally:
        os.close(fd)


def read_json(parent, name):
    fd = os.open(name, FILE_FLAGS, dir_fd=parent)
    with os.fdopen(fd, "rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
            raise LayoutError("invalid_storage_identity")
        data = source.read(4097)
        if len(data) > 4096:
            raise LayoutError("invalid_storage_identity")
    try:
        value = json.loads(data)
    except (ValueError, UnicodeError):
        raise LayoutError("invalid_storage_identity") from None
    if not isinstance(value, dict):
        raise LayoutError("invalid_storage_identity")
    return value


def create_json(parent, name, value):
    """Publish complete contents without replacing an existing identity."""
    temporary = ".memoh-publish-" + uuid.uuid4().hex
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600, dir_fd=parent)
    try:
        with os.fdopen(fd, "wb") as target:
            target.write(json.dumps(value, sort_keys=True).encode() + b"\n")
            target.flush()
            os.fsync(target.fileno())
        try:
            os.link(temporary, name, src_dir_fd=parent, dst_dir_fd=parent, follow_symlinks=False)
        except FileExistsError:
            pass
        os.fsync(parent)
    finally:
        os.unlink(temporary, dir_fd=parent)


@contextlib.contextmanager
def native_volume(root):
    try:
        with directory(root) as fd, directory(os.path.dirname(root)) as parent:
            owned(fd)
            if filesystem_type(fd) not in PERSISTENT_FILESYSTEMS or mount_id(fd) == mount_id(parent):
                raise LayoutError("native_volume_not_persistent_local")
            create_json(fd, ".memoh-volume.json", {"version": 1, "id": str(uuid.uuid4())})
            identity = read_json(fd, ".memoh-volume.json")
            try:
                valid = identity.get("version") == 1 and str(uuid.UUID(identity["id"])) == identity["id"]
            except (KeyError, ValueError, TypeError, AttributeError):
                valid = False
            if not valid:
                raise LayoutError("invalid_storage_identity")
            lock = os.open(".memoh-prepare.lock", os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK, 0o600, dir_fd=fd)
            try:
                lock_info = os.fstat(lock)
                if not stat.S_ISREG(lock_info.st_mode) or lock_info.st_uid != os.geteuid() or lock_info.st_mode & 0o077:
                    raise LayoutError("invalid_storage_identity")
                # This lock is on the verified persistent LOCAL filesystem.
                fcntl.flock(lock, fcntl.LOCK_EX)
                yield fd, identity["id"]
            finally:
                os.close(lock)
    except FileNotFoundError:
        raise LayoutError("native_volume_required") from None


def home_identity(home, volume):
    return {"version": 1, "home": home, "volume": volume}


def target_path(home, root, volume):
    return os.path.join(root, volume, hashlib.sha256(home.encode()).hexdigest())


def validate_home(fd, identity):
    owned(fd, private=True)
    if filesystem_type(fd) not in PERSISTENT_FILESYSTEMS:
        raise LayoutError("native_volume_not_persistent_local")
    try:
        actual = read_json(fd, MARKER)
    except FileNotFoundError:
        raise LayoutError("native_state_missing") from None
    if actual != identity:
        raise LayoutError("invalid_storage_identity")


def inspect_entry(parent, name):
    try:
        return os.stat(name, dir_fd=parent, follow_symlinks=False)
    except FileNotFoundError:
        return None


def prepare_local_tmp(home, temp_parent="/tmp"):
    """Compatibility with v0.1 managed tmp links on otherwise local homes."""
    root_name = f"memoh-codex-runtime-{os.geteuid()}"
    expected = os.path.join(temp_parent, root_name, hashlib.sha256(home.encode()).hexdigest())
    with directory(home) as home_fd:
        entry = inspect_entry(home_fd, "tmp")
        if entry is None:
            return "local"  # Codex creates its own tmp on this local home.
        if stat.S_ISDIR(entry.st_mode):
            with directory(home + "/tmp") as fd:
                if not is_local(fd):
                    raise LayoutError("legacy_tmp_requires_drain")
            return "local"
        if not stat.S_ISLNK(entry.st_mode) or os.readlink("tmp", dir_fd=home_fd) != expected:
            raise LayoutError("unexpected_tmp_link")
        with directory(temp_parent) as fd:
            if not is_local(fd):
                raise LayoutError("runtime_root_not_local")
            with private_child(fd, root_name) as root_fd:
                with private_child(root_fd, os.path.basename(expected)):
                    pass
        return "local"


def prepare(home, native_root=NATIVE_ROOT):
    if not os.path.isabs(home) or os.path.normpath(home) != home or home.startswith("//"):
        raise LayoutError("unsafe_path")
    parent_path, name = os.path.split(home)
    if not name:
        raise LayoutError("unsafe_path")
    with directory(parent_path, create=True) as parent:
        entry = inspect_entry(parent, name)
        if entry is None and is_local(parent):
            try:
                os.mkdir(name, mode=0o700, dir_fd=parent)
            except FileExistsError:
                pass
            entry = inspect_entry(parent, name)
        if entry is not None and stat.S_ISDIR(entry.st_mode):
            with directory(home) as fd:
                if is_local(fd):
                    return prepare_local_tmp(home)
            raise LayoutError("native_home_requires_drain")
        if entry is not None and not stat.S_ISLNK(entry.st_mode):
            raise LayoutError("unsafe_home_entry")
        with native_volume(native_root) as (volume_fd, volume):
            entry = inspect_entry(parent, name)
            target = target_path(home, native_root, volume)
            identity = home_identity(home, volume)
            if entry is not None:
                if os.readlink(name, dir_fd=parent) != target:
                    raise LayoutError("native_volume_identity_mismatch")
                try:
                    with directory(target) as fd:
                        validate_home(fd, identity)
                except FileNotFoundError:
                    raise LayoutError("native_state_missing") from None
                return "isolated"
            with private_child(volume_fd, volume) as generation:
                with private_child(generation, os.path.basename(target)) as fd:
                    existing = set(os.listdir(fd))
                    if existing - {MARKER}:
                        raise LayoutError("orphaned_native_state")
                    create_json(fd, MARKER, identity)
                    validate_home(fd, identity)
                os.fsync(generation)
            try:
                os.symlink(target, name, dir_fd=parent)
            except FileExistsError:
                if os.readlink(name, dir_fd=parent) != target:
                    raise LayoutError("native_home_publication_failed") from None
            os.fsync(parent)
            return "isolated"


def ensure_drained(home):
    """Local process check supplements, never replaces, the operator's drain."""
    expected = ("CODEX_HOME=" + home).encode()
    for name in os.listdir("/proc"):
        if not name.isdigit() or int(name) == os.getpid():
            continue
        try:
            with open(f"/proc/{name}/environ", "rb") as source:
                if expected in source.read(1024 * 1024).split(b"\0"):
                    raise LayoutError("native_home_in_use")
        except (FileNotFoundError, ProcessLookupError):
            pass
        except PermissionError:
            raise LayoutError("cannot_verify_local_processes") from None


def copy_tree(source, target, skip_tmp=True):
    """Copy an offline tree, never following source links or special files."""
    for name in os.listdir(source):
        if skip_tmp and name == "tmp":
            continue
        info = os.stat(name, dir_fd=source, follow_symlinks=False)
        if stat.S_ISDIR(info.st_mode):
            os.mkdir(name, mode=0o700, dir_fd=target)
            a = os.open(name, DIR_FLAGS, dir_fd=source)
            b = os.open(name, DIR_FLAGS, dir_fd=target)
            try:
                copy_tree(a, b, skip_tmp=False)
                os.fchmod(b, stat.S_IMODE(info.st_mode) & 0o777)
                os.fsync(b)
            finally:
                os.close(a)
                os.close(b)
        elif stat.S_ISREG(info.st_mode):
            a = os.open(name, FILE_FLAGS, dir_fd=source)
            b = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600, dir_fd=target)
            with os.fdopen(a, "rb") as reader, os.fdopen(b, "wb") as writer:
                current = os.fstat(reader.fileno())
                if not stat.S_ISREG(current.st_mode) or (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
                    raise LayoutError("migration_source_changed")
                shutil.copyfileobj(reader, writer, 1024 * 1024)
                writer.flush()
                os.fchmod(writer.fileno(), stat.S_IMODE(info.st_mode) & 0o777)
                os.fsync(writer.fileno())
                after = os.fstat(reader.fileno())
                if (after.st_size, after.st_mtime_ns) != (current.st_size, current.st_mtime_ns):
                    raise LayoutError("migration_source_changed")
        else:
            # Unknown links could put a lock/database back on NFS. An operator
            # must resolve them explicitly rather than having migration follow.
            raise LayoutError("unsupported_migration_entry")


def validate_databases(target):
    # No process may write the source during this operation. Copying its stable
    # DB + WAL set first avoids acquiring any SQLite/NLM lock on the NFS source.
    for name in os.listdir(target):
        if not name.endswith(".sqlite"):
            continue
        path = os.path.join(target, name)
        with contextlib.closing(sqlite3.connect(path, timeout=5)) as connection:
            if connection.execute("PRAGMA quick_check").fetchall() != [("ok",)]:
                raise LayoutError("invalid_native_database")
            if connection.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()[0] != 0:
                raise LayoutError("invalid_native_database")
        fd = os.open(path, FILE_FLAGS)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)


def migrate_drained(home, native_root=NATIVE_ROOT):
    ensure_drained(home)
    with directory(home) as source, directory(os.path.dirname(home)) as parent:
        if is_local(source):
            raise LayoutError("migration_not_required")
        if inspect_entry(source, MARKER) is not None:
            raise LayoutError("unsupported_migration_entry")
        config = inspect_entry(source, "config.toml")
        if config is not None:
            config_fd = os.open("config.toml", FILE_FLAGS, dir_fd=source)
            with os.fdopen(config_fd, "rb") as config_file:
                if not stat.S_ISREG(os.fstat(config_file.fileno()).st_mode):
                    raise LayoutError("unsupported_migration_entry")
                if tomllib.load(config_file).get("sqlite_home"):
                    raise LayoutError("unsupported_migration_entry")
        original = os.fstat(source)
        with native_volume(native_root) as (volume_fd, volume):
            target = target_path(home, native_root, volume)
            identity = home_identity(home, volume)
            with private_child(volume_fd, volume) as generation:
                name = os.path.basename(target)
                if inspect_entry(generation, name) is not None:
                    raise LayoutError("migration_target_exists")
                staging = ".migration-" + uuid.uuid4().hex
                with private_child(generation, staging) as out:
                    copy_tree(source, out)
                    create_json(out, MARKER, identity)
                    os.fsync(out)
                validate_databases(os.path.join(native_root, volume, staging))
                ensure_drained(home)
                current = os.stat(os.path.basename(home), dir_fd=parent, follow_symlinks=False)
                if (current.st_dev, current.st_ino) != (original.st_dev, original.st_ino):
                    raise LayoutError("migration_source_changed")
                os.rename(staging, name, src_dir_fd=generation, dst_dir_fd=generation)
                os.fsync(generation)
                # Keep the full old tree, including tmp, for explicit rollback.
                archive = os.path.basename(home) + ".before-native-storage-" + uuid.uuid4().hex
                os.rename(os.path.basename(home), archive, src_dir_fd=parent, dst_dir_fd=parent)
                os.fsync(parent)
                os.symlink(target, os.path.basename(home), dir_fd=parent)
                os.fsync(parent)
    return "migrated"


if __name__ == "__main__":
    try:
        if len(sys.argv) == 3 and sys.argv[1] == "--migrate-drained":
            print(migrate_drained(sys.argv[2]))
        elif len(sys.argv) == 2:
            print(prepare(sys.argv[1]))
        else:
            raise LayoutError("invalid_arguments")
    except LayoutError as error:
        print(str(error), file=sys.stderr)
        sys.exit(2)
    except (OSError, ValueError, sqlite3.Error):
        print("runtime_storage_preparation_failed", file=sys.stderr)
        sys.exit(2)
