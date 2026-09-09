"""Prepare Codex's helper-lock directory inside the target Linux workspace.

Sent over bridge stdin by runtime_tmp.go; no additional image file is required.
Only a missing tmp entry is created. Existing network-backed directories must
be drained and migrated by an operator, never renamed under a running Codex.
"""

import contextlib
import hashlib
import os
import stat
import sys


LOCAL_FILESYSTEMS = frozenset(
    ("ext2", "ext3", "ext4", "xfs", "btrfs", "tmpfs", "ramfs", "overlay", "aufs", "zfs", "f2fs")
)
DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC


class LayoutError(Exception):
    pass


def filesystem_type(fd):
    # Use the opened directory's mount ID, not a path-prefix match: bind mounts,
    # escaped mount names and nested mounts must resolve to the actual mount.
    with open(f"/proc/self/fdinfo/{fd}", encoding="ascii") as info:
        mount_id = next(
            (line.split()[1] for line in info if line.startswith("mnt_id:")), None
        )
    with open("/proc/self/mountinfo", encoding="utf-8") as mounts:
        for line in mounts:
            fields = line.split()
            if fields[0] == mount_id:
                return fields[fields.index("-") + 1]
    raise LayoutError("unknown_filesystem")


def is_local(fd):
    return filesystem_type(fd) in LOCAL_FILESYSTEMS


@contextlib.contextmanager
def directory(path):
    """Open existing absolute path components without traversing symlinks."""
    if not os.path.isabs(path) or os.path.normpath(path) != path:
        raise LayoutError("unsafe_path")
    fd = os.open("/", DIR_FLAGS)
    try:
        for part in path.split("/")[1:]:
            if not part:
                continue
            child = os.open(part, DIR_FLAGS, dir_fd=fd)
            os.close(fd)
            fd = child
        yield fd
    finally:
        os.close(fd)


@contextlib.contextmanager
def private_child(parent, name):
    try:
        os.mkdir(name, mode=0o700, dir_fd=parent)
    except FileExistsError:
        pass
    fd = os.open(name, DIR_FLAGS, dir_fd=parent)
    try:
        entry = os.fstat(fd)
        if entry.st_uid != os.geteuid() or stat.S_IMODE(entry.st_mode) != 0o700:
            raise LayoutError("unsafe_local_permissions")
        if not is_local(fd):
            raise LayoutError("runtime_root_not_local")
        yield fd
    finally:
        os.close(fd)


def inspect_tmp(home_fd, expected):
    try:
        entry = os.stat("tmp", dir_fd=home_fd, follow_symlinks=False)
    except FileNotFoundError:
        return "missing"
    if stat.S_ISLNK(entry.st_mode):
        if os.readlink("tmp", dir_fd=home_fd) != expected:
            raise LayoutError("unexpected_tmp_link")
        return "managed"
    if not stat.S_ISDIR(entry.st_mode):
        raise LayoutError("unsafe_tmp_entry")
    fd = os.open("tmp", DIR_FLAGS, dir_fd=home_fd)
    try:
        if not is_local(fd):
            raise LayoutError("legacy_tmp_requires_drain")
        return "local"
    finally:
        os.close(fd)


def prepare(home, temp_parent="/tmp"):
    # Same absolute link on every workspace generation, but different local
    # inodes. Never repoint a shared durable link to a per-process UUID: another
    # process could still be using the old target. Codex owns its own unique
    # arg0 subdirectories and their lifetime locks below this stable directory.
    root_name = f"memoh-codex-runtime-{os.geteuid()}"
    key = hashlib.sha256(home.encode("utf-8")).hexdigest()
    expected = os.path.join(temp_parent, root_name, key)
    with directory(home) as home_fd:
        state = inspect_tmp(home_fd, expected)
        if state == "local":
            return "local"
        with directory(temp_parent) as parent_fd:
            if not is_local(parent_fd):
                raise LayoutError("runtime_root_not_local")
            with private_child(parent_fd, root_name) as root_fd:
                with private_child(root_fd, key):
                    try:
                        os.symlink(expected, "tmp", dir_fd=home_fd)
                    except FileExistsError:
                        # Concurrent preparation is allowed only if the winner
                        # installed exactly our mapping (or a safe local dir).
                        pass
                    result = inspect_tmp(home_fd, expected)
                    if result not in ("managed", "local"):
                        raise LayoutError("tmp_publication_failed")
        return "isolated" if result == "managed" else "local"


if __name__ == "__main__":
    try:
        if len(sys.argv) != 2:
            raise LayoutError("invalid_arguments")
        print(prepare(sys.argv[1]))
    except LayoutError as error:
        print(str(error), file=sys.stderr)
        sys.exit(2)
    except (OSError, ValueError, StopIteration):
        # Never echo arbitrary filesystem contents or inherited environment.
        print("runtime_tmp_preparation_failed", file=sys.stderr)
        sys.exit(2)
