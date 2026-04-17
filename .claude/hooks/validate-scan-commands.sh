#!/usr/bin/env bash

set -euo pipefail

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command')

# Only validate commands starting with our scan tools
case "$COMMAND" in
  golangci-lint\ *|govulncheck\ *|which\ golangci-lint|which\ govulncheck|golangci-lint|govulncheck)
    ;;
  *)
    exit 0
    ;;
esac

# Token allowlist
# 1 = standalone token (no argument follows)
# 2 = flag that consumes the next token (validated by regex)
declare -A ALLOWED=(
  # Commands
  [golangci-lint]=1
  [govulncheck]=1
  [which]=1

  # Subcommands
  [run]=1
  [version]=1

  # Standalone flags
  [--version]=1
  [-version]=1

  # Flags that take a validated argument
  [-c]=2
  [--config]=2
  [--timeout]=2
  [--out-format]=2
  [--new-from-rev]=2
  [-show]=2

  # Known standalone values
  [./...]=1
  [json]=1
  [line-number]=1
  [colored-line-number]=1
  [tab]=1
  [verbose]=1
  [traces]=1
  [.golangci.yaml]=1
  [boilerplate/openshift/golang-osd-operator/golangci.yml]=1
)

# Regex validators for flag arguments (keyed by the flag)
validate_flag_arg() {
  local flag="$1"
  local value="$2"

  case "$flag" in
    -c|--config)
      [[ "$value" =~ ^[a-zA-Z0-9_./-]+\.(ya?ml)$ ]] && return 0 ;;
    --timeout)
      [[ "$value" =~ ^[0-9]+[smh]$ ]] && return 0 ;;
    --out-format)
      [[ "$value" =~ ^[a-z-]+$ ]] && return 0 ;;
    --new-from-rev)
      [[ "$value" =~ ^[a-f0-9]+$ ]] && return 0 ;;
    -show)
      [[ "$value" =~ ^[a-z]+$ ]] && return 0 ;;
  esac
  return 1
}

log() {
  echo "$(date -Iseconds) decision=$1 command='$COMMAND' reason='$2'" >> /tmp/scan-commands.log
}

deny() {
  log "deny" "$1"
  jq -n --arg reason "$1" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $reason
    }
  }'
  exit 0
}

# Split command into tokens
read -ra TOKENS <<< "$COMMAND"

skip_next=false
pending_flag=""

for i in "${!TOKENS[@]}"; do
  token="${TOKENS[$i]}"

  if $skip_next; then
    if ! validate_flag_arg "$pending_flag" "$token"; then
      deny "Token '$token' failed validation as argument to '$pending_flag'"
    fi
    skip_next=false
    pending_flag=""
    continue
  fi

  if [[ -z "${ALLOWED[$token]+x}" ]]; then
    deny "Unknown token '$token' in command: $COMMAND"
  fi

  if [[ "${ALLOWED[$token]}" == "2" ]]; then
    skip_next=true
    pending_flag="$token"
  fi
done

if $skip_next; then
  deny "Flag '$pending_flag' requires an argument but command ended"
fi

# All tokens validated
log "allow" "All tokens validated against scan command allowlist"
jq -n '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "allow",
    permissionDecisionReason: "All tokens validated against scan command allowlist"
  }
}'
