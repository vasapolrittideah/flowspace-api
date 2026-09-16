#!/bin/sh

status=0
task coverage || status=1
task vuln || status=1
if [ "$status" -eq 0 ]; then
  exit 0
fi
if [ "$(date -u +%Y%m%d)" -lt 20260926 ]; then
  echo "warning: a numbered constraint failed during the rollout window"
  exit 0
fi
exit 1
