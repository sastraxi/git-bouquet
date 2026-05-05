# git-bouquet

Maintain a long-lived integration branch by merging a curated list of feature
branches into a fresh checkout of an upstream base and recording the result
as a single linear commit on the integration branch.

Designed for the case where you have many small PRs in flight that the
upstream maintainer will take a while to review, but you want a single
"everything I'm shipping" branch that contributors can run.

## Install

```sh
go install github.com/sastraxi/git-bouquet@latest
```

This puts a `git-bouquet` binary on your `PATH`, which makes it available as
`git bouquet` (git auto-discovers `git-*` executables).

## Usage

Add a `.bouquet.yaml` to the root of your repo:

```yaml
target: release/current   # branch to commit the snapshot to
base:   main              # branch to start each rebuild from
merge:                    # globs; expanded against local branch names
  - feat/*
  - test/*
  - "!feat/wip-*"           # gitignore-style negation drops matches
```

To gate releases on tests, run them yourself after `git bouquet start`
succeeds. If they fail, `git update-ref refs/heads/<target> <target>~1`
moves the branch back one rebuild.

Then:

```sh
git bouquet list                      # show what would be merged, in order
git bouquet start [--pull] [--sync]   # rebuild
git bouquet continue                  # after resolving a conflict
git bouquet abort                     # bail out
git bouquet status                    # where are we
```

### Flags on `start`

- `--pull`  fast-forward `base` and each leaf from their upstreams first.
- `--sync`  run `git town sync -s` on each leaf first (requires `git-town`).
- `--dry-run`  do everything except the final commit + branch update.

### Resolving conflicts

When `start` (or `continue`) stops at a conflict, the worktree is a normal
git working tree — just `cd` into it:

```sh
cd .git/bouquet/worktree
# edit conflicted files, then
git add <resolved files>
cd ../../..             # back to repo root
git bouquet continue
```

Don't `git commit` yourself — `bouquet continue` seals the in-progress
merge with the default message and resumes the loop. `rerere` records every
resolution, so the same conflict will replay automatically next time.

## Leaf constraints

Every leaf must be a **descendant of `base`** in git history (i.e. `base` is
an ancestor of the leaf's tip). `git bouquet start` enforces this and rejects
leaves that violate it. This guarantees that the three-way merge inputs seen
at each step are stable across rebuilds, which is what makes `rerere` replay
reliable.

When using `git-town`, the simplest way to satisfy this is to root every leaf
on `base` (or on another leaf that is itself rooted on `base`, transitively).
Leaves that were originally branched from `base` but have since been rebased
or fast-forwarded by `git town sync` continue to satisfy the constraint.

The **order** of leaves in `merge:` is your responsibility — it determines
which side of each conflict is "ours" vs "theirs," and changing the order
invalidates `rerere` cache entries.

## How it works

1. Resolve `merge:` globs against local branches; drop ancestors; sort
   deterministically within each glob pattern. Verify every leaf descends from
   `base`.
2. Create a detached worktree at `.git/bouquet/worktree/` checked out to
   `base`.
3. `git merge --no-ff` each leaf in turn. On conflict: stop, print
   instructions; resume with `git bouquet continue`.
4. Snapshot-commit the resulting tree onto `target`: parent is the previous
   `target` (or `base` on first run), tree is the worktree's tree. One commit
   per rebuild — `git log target` is your release log.
5. Clean up the worktree.

`rerere` is enabled at repo scope on first run.

## Why not just `git merge a b c...`?

Octopus merge aborts at the first manual-resolution conflict and produces a
many-parent merge commit. `git-bouquet` does sequential merges, which work
with `rerere` — each pairwise conflict is cached and replayed independently.
The result is one snapshot commit per rebuild.
