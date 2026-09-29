#!/usr/bin/env python3
"""Sync the WSL working copy to the Windows working copy (one way, incremental).

Only files needed to build/test are copied: VCS data, secrets, agent scratch
dirs, build caches and local outputs are excluded (see EXCLUDE_DIRS /
EXCLUDE_NAMES / EXCLUDE_FILES). By default only new or newer files are copied;
nothing is ever deleted unless --mirror --yes is given.

Usage:
  scripts/sync-wsl-to-windows.py [--dest DIR] [--dry-run] [--mirror --yes]
  scripts/sync-wsl-to-windows.py --reverse [--dest DIR] [--dry-run]

--reverse copies Windows -> WSL (e.g. bring back release artifacts built on
Windows). Same rules: only new/newer files, never deletes unless --mirror.

Environment:
  WIN_PROJET  overrides the default Windows side directory.
"""

import argparse
import fnmatch
import os
import shutil
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
SRC_ROOT = os.path.dirname(SCRIPT_DIR)
DEFAULT_DEST = r"/mnt/c/Users/Administrator/Documents/Projets/projet"
MARKERS = ("AGENTS.md", "Makefile")

# Relative dirs (from repo root) never synced.
EXCLUDE_DIRS = {
    ".git",
    ".secrets",
    ".agents",
    ".cache",
    ".tools",
    "installer/build",
    "output",
}
# Dir names never synced, wherever they appear.
EXCLUDE_NAMES = {
    "__pycache__",
    ".pytest_cache",
    "node_modules",
    "target",
}
# File name patterns never synced.
EXCLUDE_FILES = {
    "*.pyc",
    "*Zone.Identifier*",
}
# mtime epsilon (filesystem granularity) in seconds.
MTIME_EPS = 1.0


def excluded_dir(rel):
    parts = rel.split(os.sep)
    if rel in EXCLUDE_DIRS:
        return True
    return any(p in EXCLUDE_NAMES for p in parts)


def excluded_file(rel):
    base = os.path.basename(rel)
    return any(fnmatch.fnmatchcase(base, pat) for pat in EXCLUDE_FILES)


def walk_files(root):
    """Yield relative paths of regular files under root, honouring excludes."""
    stack = [""]
    while stack:
        rel = stack.pop()
        full = os.path.join(root, rel) if rel else root
        try:
            with os.scandir(full) as it:
                entries = list(it)
        except OSError as exc:
            print(f"WARN: cannot scan {full}: {exc}", file=sys.stderr)
            continue
        for entry in entries:
            try:
                child = entry.name if not rel else rel + os.sep + entry.name
                if entry.is_symlink():
                    print(f"WARN: skip symlink {child}", file=sys.stderr)
                    continue
                if entry.is_dir(follow_symlinks=False):
                    if not excluded_dir(child):
                        stack.append(child)
                elif entry.is_file(follow_symlinks=False):
                    if not excluded_file(child):
                        yield child
            except OSError as exc:
                print(f"WARN: skip {entry.path}: {exc}", file=sys.stderr)


def check_dest(write_root, walk_root):
    if not os.path.isdir(write_root):
        return f"destination is not a directory: {write_root}"
    for marker in MARKERS:
        if not os.path.isfile(os.path.join(write_root, marker)):
            return f"destination lacks marker {marker}: {write_root} (refusing to sync)"
    real_write = os.path.realpath(write_root)
    real_walk = os.path.realpath(walk_root)
    if real_write == real_walk or real_write.startswith(real_walk + os.sep):
        return "destination is inside the source tree (refusing to sync)"
    if real_walk.startswith(real_write + os.sep):
        return "source is inside the destination tree (refusing to sync)"
    return None


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--dest", default=os.environ.get("WIN_PROJET", DEFAULT_DEST))
    ap.add_argument("--dry-run", action="store_true", help="report only, copy nothing")
    ap.add_argument("--mirror", action="store_true",
                    help="also delete dest files absent from source (needs --yes)")
    ap.add_argument("--yes", action="store_true", help="confirm --mirror deletions")
    ap.add_argument("--reverse", action="store_true",
                    help="copy Windows -> WSL instead of WSL -> Windows")
    args = ap.parse_args()

    if args.mirror and not args.yes:
        print("ERROR: --mirror needs --yes", file=sys.stderr)
        return 2
    walk_root, write_root = (args.dest, SRC_ROOT) if args.reverse else (SRC_ROOT, args.dest)
    err = check_dest(write_root, walk_root)
    if err:
        print(f"ERROR: {err}", file=sys.stderr)
        return 2

    copied, identical, skipped_newer, failed = 0, 0, 0, 0
    bytes_copied = 0
    shown = 0
    src_set = set()
    for rel in walk_files(walk_root):
        src_set.add(rel)
        src = os.path.join(walk_root, rel)
        dst = os.path.join(write_root, rel)
        try:
            st_src = os.stat(src)
        except OSError as exc:
            print(f"WARN: stat {rel}: {exc}", file=sys.stderr)
            failed += 1
            continue
        try:
            st_dst = os.stat(dst)
            exists = True
        except FileNotFoundError:
            exists = False
        action = None
        if not exists:
            action = "new"
        elif st_dst.st_mtime > st_src.st_mtime + MTIME_EPS:
            skipped_newer += 1
            continue
        elif st_src.st_mtime > st_dst.st_mtime + MTIME_EPS or st_src.st_size != st_dst.st_size:
            action = "update"
        else:
            identical += 1
            continue
        if args.dry_run:
            if shown < 50:
                print(f"[{action}] {rel}")
                shown += 1
            copied += 1
            bytes_copied += st_src.st_size
            continue
        try:
            os.makedirs(os.path.dirname(dst) or write_root, exist_ok=True)
            shutil.copy2(src, dst)
        except OSError as exc:
            print(f"WARN: copy {rel}: {exc}", file=sys.stderr)
            failed += 1
            continue
        copied += 1
        bytes_copied += st_src.st_size

    deleted, del_failed = 0, 0
    if args.mirror:
        for rel in walk_files(write_root):
            if rel in src_set:
                continue
            # Never touch excluded paths on the destination side either.
            if excluded_dir(os.path.dirname(rel)) or excluded_file(rel):
                continue
            dst = os.path.join(write_root, rel)
            if args.dry_run:
                if shown < 100:
                    print(f"[delete] {rel}")
                    shown += 1
                deleted += 1
                continue
            try:
                os.remove(dst)
                deleted += 1
            except OSError as exc:
                print(f"WARN: delete {rel}: {exc}", file=sys.stderr)
                del_failed += 1
        # Prune directories left empty (best effort, shallow errors ignored).
        if not args.dry_run:
            for dirpath, dirnames, filenames in os.walk(write_root, topdown=False):
                if dirpath == write_root:
                    continue
                if not dirnames and not filenames:
                    try:
                        os.rmdir(dirpath)
                    except OSError:
                        pass

    mode = "DRY-RUN " if args.dry_run else ""
    print(f"{mode}sync {walk_root} -> {write_root}")
    print(f"{mode}copied={copied} ({bytes_copied / 1048576:.1f} MiB) "
          f"identical={identical} dest_newer_skipped={skipped_newer} failed={failed} "
          f"deleted={deleted} delete_failed={del_failed}")
    return 1 if (failed or del_failed) else 0


if __name__ == "__main__":
    sys.exit(main())
