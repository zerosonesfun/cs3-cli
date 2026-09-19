#!/usr/bin/env bash
# Double-click (if executable) or: ./open-cs3.sh
cd "$(dirname "$0")"
clear
chmod +x ./cs3 2>/dev/null || true
exec ./cs3
