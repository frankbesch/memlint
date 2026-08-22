#!/bin/bash
# Runs the command stored in file $1, captures full output to a sibling .log,
# and prints back only what matters. Short output (<=30 lines) is returned
# whole. Exit status of the original command is preserved.
set -u
cmdfile="$1"
log="${cmdfile}.log"
bash "$cmdfile" >"$log" 2>&1
status=$?
lines=$(wc -l < "$log" | tr -d ' ')
if [ "$lines" -le 30 ]; then
  cat "$log"
else
  grep -E -n -- '--- FAIL|^FAIL|^ERROR|[Ee]rror:|panic:|warning|not ok|✗|undefined:|cannot ' "$log" | head -30
  echo "  ..."
  tail -8 "$log"
fi
echo "[quiet-run: exit $status, $lines lines total, full log: $log]"
exit $status
