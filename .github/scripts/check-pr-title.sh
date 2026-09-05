#!/usr/bin/env bash
# The pull request title becomes the commit on main (ADR-0013).
set -euo pipefail
title=${TITLE:?}
number=${NUMBER:?}

types='feat|fix|docs|chore|refactor|perf|test|build|ci|revert'
if [[ ! $title =~ ^($types)(\([a-z0-9-]+\))?!?:\ .+$ ]]; then
  echo "::error::not a Conventional Commit: $title"
  exit 1
fi

scope=${BASH_REMATCH[2]//[()]/}
if [[ -n $scope && $scope != repo && ! -d services/$scope ]]; then
  echo "::error::scope '$scope' is neither 'repo' nor a directory under services/"
  exit 1
fi

# GitHub appends the number to the squashed subject.
subject="$title (#$number)"
if (( ${#subject} > 100 )); then
  echo "::error::${#subject} characters once GitHub appends (#$number); limit is 100"
  exit 1
fi
echo "ok: $subject (${#subject} chars)"
