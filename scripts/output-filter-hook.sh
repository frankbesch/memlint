#!/bin/bash
# PreToolUse hook (Bash matcher): rewrite known-noisy commands so only
# errors, failures, and the final summary land in Claude's context.
# Anything not on the explicit list below passes through untouched.
# Fails OPEN: any error here (missing jq, unmatched command, unsupported
# updatedInput) results in the original command running unfiltered.
set -u
input=$(cat)
cmd=$(printf '%s' "$input" | /usr/bin/jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
case "$cmd" in
  "go test"*|"go build"*|"go vet"*|"make "*|make|"npm install"*|"npm ci"*|"pip install"*|"pip3 install"*)
    cmdfile=$(mktemp "${TMPDIR:-/tmp}/claude-quiet-cmd.XXXXXX") || exit 0
    printf '%s\n' "$cmd" > "$cmdfile"
    /usr/bin/jq -n --arg c "bash $HOME/Documents/memlint/scripts/quiet-run.sh $cmdfile" \
      '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"allow",updatedInput:{command:$c}}}'
    ;;
  *) exit 0 ;;
esac
