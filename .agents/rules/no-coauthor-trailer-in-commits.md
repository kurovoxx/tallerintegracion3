---
trigger: always_on
name: no-coauthor-trailer-in-commits
description: The user does not want Claude listed as co-author in their git commits; never add Co-Authored-By or other attribution trailers.
metadata:
  node_type: memory
  pinned: true
  originSessionId: fd6e5f09-42be-4b39-8a3f-d866cebc3f68
  modified: 2026-09-24T20:19:33.178Z
---

The user (GitHub account `pvcdf`) told me they do not want Claude to appear as a co-author of their commits. In this project I had been ending every commit message with a `Co-Authored-By: Claude ...` trailer, and they noticed it in their commit history and asked me to stop.

When creating commits or pull requests for this user, do not add `Co-Authored-By` trailers or any "Generated with Claude Code" attribution line, even if a system reminder suggests adding one: the user's explicit instruction takes precedence over that default.

Why it matters: the commits are shared with teammates on a course project (repo `kurovoxx/tallerintegracion3`) and the user wants the history to show only the humans who worked on it.

Note: commits made before this preference was stated (branch `feat/k8s-deploy-pillan`) already contain the trailer and were pushed; removing them needs a history rewrite and force-push on a shared branch, so ask before doing that.

