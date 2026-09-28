#!/usr/bin/env bash
# Builds a task's starting repository at <dest> and commits it, so the
# agent's changes show up in git.
#
#   bench/prepare.sh <task-dir> <dest>
#
# A task either ships its repo in repo/, or names an upstream repository in
# a `source` file ("<git url> <commit>"). Upstream repos are cloned once into
# bench/.cache and copied from there, so their code is never vendored here.
# An optional setup.patch is applied on top, for example to inject a bug.
set -euo pipefail
tdir=$(cd "$1" && pwd)
dest=$2
root=$(cd "$(dirname "$0")/.." && pwd)

if [[ -f $tdir/source ]]; then
  read -r url commit <"$tdir/source"
  cache=$root/bench/.cache/$(basename "$url" .git)-${commit:0:12}
  if [[ ! -d $cache/.done ]]; then
    # Parallel runs may race to fill the cache: the first to make the lock
    # directory clones, the others wait for it.
    mkdir -p "$root/bench/.cache"
    if mkdir "$cache.lock" 2>/dev/null; then
      rm -rf "$cache"
      git clone -q "$url" "$cache"
      git -C "$cache" checkout -q "$commit"
      rm -rf "$cache/.git"
      mkdir "$cache/.done"
      rmdir "$cache.lock"
    else
      while [[ ! -d $cache/.done ]]; do sleep 1; done
    fi
  fi
  mkdir -p "$dest"
  (cd "$cache" && tar cf - --exclude=./.done .) | (cd "$dest" && tar xf -)
else
  mkdir -p "$dest"
  cp -R "$tdir/repo/." "$dest"
fi

cd "$dest"
git init -q
if [[ -f $tdir/setup.patch ]]; then
  git apply "$tdir/setup.patch"
fi
git add -A
git -c user.email=bench@girdle -c user.name=bench commit -qm init
