#!/bin/bash
# Double-click in Finder, or run from Terminal.
cd "$(dirname "$0")"
clear
if [[ ! -x ./cs3 ]]; then
  chmod +x ./cs3 2>/dev/null || true
fi
exec ./cs3
