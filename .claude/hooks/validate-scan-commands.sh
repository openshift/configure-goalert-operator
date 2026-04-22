#!/usr/bin/env bash

set -euo pipefail

if ((BASH_VERSINFO[0] < 4)); then
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"bash 4+ required (found %s)"}}\n' "$BASH_VERSION"
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"jq is required but not found"}}'
  exit 0
fi

COMMAND=$(jq -e -r '.tool_input.command | select(type == "string")' 2>/dev/null) || COMMAND=""

if [[ -z "$COMMAND" ]]; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: "tool_input.command must be a non-empty string"
    }
  }'
  exit 0
fi

if (( ${#COMMAND} > 4096 )); then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: "Command exceeds maximum length (4096 characters)"
    }
  }'
  exit 0
fi

ORIGINAL_COMMAND="$COMMAND"

# Normalize: strip leading "env" and VAR=VALUE assignments so
# "FOO=bar env BAZ=qux golangci-lint run" matches the case block.
read -ra _NORM_TOKENS <<< "$COMMAND"
_STRIP=0
for _t in "${_NORM_TOKENS[@]}"; do
  if [[ "$_t" == "env" || "$_t" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
    _STRIP=$((_STRIP + 1))
  else
    break
  fi
done
if ((_STRIP > 0)); then
  TOKENS=("${_NORM_TOKENS[@]:$_STRIP}")
  COMMAND="${TOKENS[*]}"
  if [[ -z "$COMMAND" ]]; then
    jq -n --arg cmd "$ORIGINAL_COMMAND" '{
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "deny",
        permissionDecisionReason: ("Command empty after normalization: " + $cmd)
      }
    }'
    exit 0
  fi
else
  TOKENS=("${_NORM_TOKENS[@]}")
fi

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
)

# Regex validators for flag arguments (keyed by the flag)
validate_flag_arg() {
  local flag="$1"
  local value="$2"

  case "$flag" in
    -c|--config)
      [[ "$value" =~ ^[a-zA-Z0-9_./-]+\.(ya?ml)$ ]] && ! [[ "$value" =~ \.\. ]] && ! [[ "$value" =~ ^/ ]] && return 0 ;;
    --timeout)
      [[ "$value" =~ ^[0-9]+[smh]$ ]] && return 0 ;;
    --out-format)
      case "$value" in
        json|text|colored-line-number|line-number|tab|colored-tab|checkstyle|code-climate|html|junit-xml|github-actions|sarif|teamcity) return 0 ;;
      esac ;;
    --new-from-rev)
      (( ${#value} <= 128 )) && [[ "$value" =~ ^[A-Za-z0-9._/~^-]+$ ]] && ! [[ "$value" =~ \.\. ]] && return 0 ;;
    -show)
      [[ "$value" =~ ^[a-z]+$ ]] && return 0 ;;
  esac
  return 1
}

LOG_DIR="${HOME:-/tmp}/tmp"
if mkdir -p "$LOG_DIR" 2>/dev/null; then
  chmod 700 "$LOG_DIR" 2>/dev/null || true
else
  echo "validate-scan-commands: WARNING: cannot create log directory '$LOG_DIR'" >&2
fi
LOG_FILE="$LOG_DIR/scan-commands-$(printf '%(%Y-%m-%d)T' -1).log"
if ! (umask 077; touch "$LOG_FILE") 2>/dev/null; then
  echo "validate-scan-commands: WARNING: cannot create log file '$LOG_FILE' — audit logging disabled" >&2
fi

log() {
  echo "$(printf '%(%Y-%m-%dT%H:%M:%S%z)T' -1) decision=$1 command=$(printf '%q' "$ORIGINAL_COMMAND") reason=$(printf '%q' "$2")" >> "$LOG_FILE" 2>/dev/null || true
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

# Defense-in-depth: reject shell metacharacters before token validation.
# read -ra splits on whitespace without interpreting shell syntax, so
# metacharacters could end up embedded in tokens that pass the allowlist.
_METACHAR_RE='[;|&$`(){}<>!\\"'"'"']'
if [[ "$ORIGINAL_COMMAND" =~ $_METACHAR_RE ]]; then
  deny "Command contains shell metacharacter"
fi

# Validate environment-variable prefix values
for (( _i=0; _i<_STRIP; _i++ )); do
  _prefix="${_NORM_TOKENS[$_i]}"
  [[ "$_prefix" == "env" ]] && continue
  _name="${_prefix%%=*}"
  _val="${_prefix#*=}"
  if [[ "$_name" =~ ^(PATH|LD_PRELOAD|LD_LIBRARY_PATH|DYLD_.*|GOFLAGS|GOPROXY|GONOSUMCHECK|GONOSUMDB|GOPRIVATE|HOME)$ ]]; then
    deny "Dangerous environment variable '$_name' in prefix '$_prefix'"
  fi
  if ! [[ "$_val" =~ ^[a-zA-Z0-9_.-]*$ ]]; then
    deny "Unsafe value in environment variable prefix '$_prefix'"
  fi
done

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

  # Handle --flag=value combined form
  if [[ "$token" == --*=* ]]; then
    local_flag="${token%%=*}"
    local_val="${token#*=}"
    if [[ -z "${ALLOWED[$local_flag]+x}" ]]; then
      deny "Unknown flag '$local_flag' in token '$token'"
    fi
    if [[ "${ALLOWED[$local_flag]}" == "2" ]]; then
      if ! validate_flag_arg "$local_flag" "$local_val"; then
        deny "Value '$local_val' failed validation as argument to '$local_flag'"
      fi
    else
      deny "Flag '$local_flag' does not accept an argument but got '$local_val'"
    fi
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
