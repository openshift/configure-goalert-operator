#!/usr/bin/env bash
# Claude Code PreToolUse hook for validating scan commands (golangci-lint, govulncheck).
# Authorization decisions exit 0 — allow/deny is communicated via JSON on stdout.
# For non-scan commands, the hook exits 0 with no output (passthrough).
# The hookSpecificOutput.permissionDecision field determines the outcome.
# If stdout is broken (closed pipe, /dev/full), the decision cannot be
# communicated, so the script exits 2 (treated as a framework error).

[[ "${BASH_SOURCE[0]}" == "${0}" ]] || return 1
_HANDLED=0
readonly _SIGNAL_DENY='{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Hook interrupted by signal"}}'
trap 'trap '\'''\'' TERM INT PIPE HUP QUIT; trap - ERR; [[ $_HANDLED == 1 ]] && exit 0; _HANDLED=1; printf "%s\n" "$_SIGNAL_DENY" 2>/dev/null && exit 0; exit 2' TERM INT PIPE HUP QUIT
set +e -u -o pipefail
export LC_ALL=C

readonly _ERR_DENY='{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Unexpected internal error"}}'
readonly _ERR_TRAP='trap '\'''\'' TERM INT PIPE HUP QUIT; trap - ERR; [[ $_HANDLED == 1 ]] && exit 0; _HANDLED=1; printf "%s\n" "$_ERR_DENY" 2>/dev/null && exit 0; exit 2'
trap "$_ERR_TRAP" ERR

if ((BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && ${BASH_VERSINFO[1]:-0} < 2))); then
  trap '' TERM INT PIPE HUP QUIT
  trap - ERR
  [[ $_HANDLED == 1 ]] && exit 0
  _HANDLED=1
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"bash 4.2+ required"}}\n' || exit 2
  exit 0
fi

_json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\t'/\\t}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\f'/\\f}"
  s="${s//$'\v'/\\u000B}"
  s="${s//$'\b'/\\b}"
  s="${s//$'\x1b'/\\u001B}"
  s="${s//$'\x7f'/\\u007F}"
  local cntrl_re='[[:cntrl:]]'
  if [[ "$s" =~ $cntrl_re ]]; then
    local i c out="" len=${#s}
    for (( i=0; i<len; i++ )); do
      c="${s:i:1}"
      if [[ "$c" =~ $cntrl_re ]]; then
        printf -v c '\\u%04X' "'$c" 2>/dev/null || c='\uFFFD'
      fi
      out+="$c"
    done
    s="$out"
  fi
  printf '%s' "$s"
}

# Audit logging is best-effort; failures are silently ignored to avoid
# blocking authorization decisions on log I/O errors.
# Log setup is deferred until after the passthrough check; calls before
# setup silently no-op.
log() {
  [[ -n "${_LOG_FD:-}" ]] || return 0
  printf '%(%Y-%m-%dT%H:%M:%S%z)T decision=%s command=%q reason=%q\n' -1 "$1" "$ORIGINAL_COMMAND" "$2" >&"$_LOG_FD" 2>/dev/null || true
}

deny() {
  trap '' TERM INT PIPE HUP QUIT
  trap - ERR
  [[ $_HANDLED == 1 ]] && exit 0
  _HANDLED=1
  local reason
  reason=$(_json_escape "$1") || reason="Internal error escaping denial reason"
  [[ -z "$reason" ]] && reason="Internal error: empty denial reason"
  local _deny_json
  if ! printf -v _deny_json '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}' "$reason"; then
    _deny_json='{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Internal error formatting denial"}}'
  fi
  log "deny" "$1"
  printf '%s\n' "$_deny_json" 2>/dev/null || exit 2
  exit 0
}

passthrough() {
  trap '' TERM INT PIPE HUP QUIT; trap - ERR; [[ $_HANDLED == 1 ]] && exit 0; _HANDLED=1; exit 0
}

ORIGINAL_COMMAND="<unavailable>"

_READ_TIMEOUT="${HOOK_READ_TIMEOUT:-2}"
if ! [[ "$_READ_TIMEOUT" =~ ^[0-9]{1,3}$ ]] || (( _READ_TIMEOUT < 1 || _READ_TIMEOUT > 300 )); then
  _READ_TIMEOUT=2
fi
readonly _READ_TIMEOUT
_READ_RC=0
_RAW_INPUT=""
IFS= read -r -d '' -t "$_READ_TIMEOUT" -n 1048577 _RAW_INPUT 2>/dev/null || _READ_RC=$?
if (( _READ_RC > 128 )); then
  deny "Input read timed out or interrupted (rc=$_READ_RC)"
elif (( _READ_RC != 0 && _READ_RC != 1 )); then
  deny "Input read failed (rc=$_READ_RC)"
fi
if (( ${#_RAW_INPUT} > 1048576 )); then
  deny "Input exceeds 1MB limit"
fi
# Broad pre-filter on raw input; the post-filter (case block below) does structured matching.
case "$_RAW_INPUT" in
  *golangci-lint*|*govulncheck*) ;;
  *) passthrough ;;
esac

if ! command -v jq >/dev/null 2>&1; then
  deny "Required dependency jq not found in PATH"
fi

_JQ_TIMEOUT="${HOOK_JQ_TIMEOUT:-5}"
if ! [[ "$_JQ_TIMEOUT" =~ ^[0-9]{1,3}$ ]] || (( _JQ_TIMEOUT < 1 || _JQ_TIMEOUT > 300 )); then
  _JQ_TIMEOUT=5
fi
readonly _JQ_TIMEOUT
_TIMEOUT_CMD=()
if command -v timeout >/dev/null 2>&1; then
  _TIMEOUT_CMD=(timeout "$_JQ_TIMEOUT")
elif command -v gtimeout >/dev/null 2>&1; then
  _TIMEOUT_CMD=(gtimeout "$_JQ_TIMEOUT")
fi
readonly _TIMEOUT_CMD
if (( ${#_TIMEOUT_CMD[@]} == 0 )); then
  deny "Required dependency timeout/gtimeout not found in PATH"
fi

COMMAND=""
_JQ_RC=0
_JQ_OUTPUT=$(printf '%s' "$_RAW_INPUT" | "${_TIMEOUT_CMD[@]}" jq -e -r '.tool_input.command | select(type == "string")' 2>/dev/null) || _JQ_RC=$?
if (( _JQ_RC == 0 )); then
  COMMAND="$_JQ_OUTPUT"
elif (( _JQ_RC == 1 )); then
  passthrough
elif (( _JQ_RC == 124 )); then
  deny "JSON parsing timed out"
elif (( _JQ_RC >= 125 )); then
  deny "jq infrastructure failure (rc=$_JQ_RC)"
else
  _VALIDATE_RC=0
  printf '%s' "$_RAW_INPUT" | "${_TIMEOUT_CMD[@]}" jq -e 'true' >/dev/null 2>&1 || _VALIDATE_RC=$?
  if (( _VALIDATE_RC == 0 )); then
    passthrough
  elif (( _VALIDATE_RC == 124 )); then
    deny "JSON validation timed out"
  elif (( _VALIDATE_RC >= 125 )); then
    deny "jq infrastructure failure (rc=$_VALIDATE_RC)"
  else
    deny "JSON parsing failed"
  fi
fi

ORIGINAL_COMMAND="$COMMAND"

if [[ -z "$COMMAND" ]]; then
  passthrough
fi

if (( ${#COMMAND} > 4096 )); then
  deny "Command exceeds maximum length (${#COMMAND} > 4096)"
fi

if [[ "$COMMAND" =~ [[:cntrl:]] ]]; then
  deny "Command contains control character"
fi

if [[ "$COMMAND" =~ [^\ -\~] ]]; then
  deny "Command contains non-ASCII characters"
fi

# Normalize: strip leading "env" and VAR=VALUE assignments so
# "FOO=bar env BAZ=qux golangci-lint run" matches the case block.
IFS=$' \t\n'
read -ra _NORM_TOKENS <<< "$COMMAND"
if (( ${#_NORM_TOKENS[@]} == 0 )); then
  passthrough
fi
if (( ${#_NORM_TOKENS[@]} > 100 )); then
  deny "Too many tokens (${#_NORM_TOKENS[@]} > 100)"
fi
readonly _ENV_PATH_RE='^(/usr)?/bin/env$'
readonly _ENV_VAR_RE='^[A-Za-z_][A-Za-z0-9_]*='
_STRIP=0
for _t in "${_NORM_TOKENS[@]}"; do
  if [[ "$_t" == "env" || "$_t" =~ $_ENV_PATH_RE || "$_t" =~ $_ENV_VAR_RE ]]; then
    _STRIP=$((_STRIP + 1))
  elif [[ "$_t" == -* && _STRIP -gt 0 ]]; then
    deny "Unsupported env flag '$_t' in command prefix"
  else
    break
  fi
done
if ((_STRIP > 0)); then
  if ((_STRIP >= ${#_NORM_TOKENS[@]})); then
    passthrough
  fi
  TOKENS=("${_NORM_TOKENS[@]:$_STRIP}")
  COMMAND="${TOKENS[*]}"
  if [[ -z "$COMMAND" ]]; then
    passthrough
  fi
fi
if ((_STRIP == 0)); then
  TOKENS=("${_NORM_TOKENS[@]}")
fi

# Only validate commands starting with our scan tools
case "$COMMAND" in
  golangci-lint\ *|govulncheck\ *|which\ golangci-lint|which\ govulncheck|golangci-lint|govulncheck)
    ;;
  *)
    passthrough
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
readonly ALLOWED

# Regex validators for flag arguments (keyed by the flag)
validate_flag_arg() {
  local flag="$1" value="$2"
  [[ -n "$flag" && -n "$value" ]] || return 1

  case "$flag" in
    -c|--config)
      [[ "$value" =~ ^[a-zA-Z0-9_.][a-zA-Z0-9_./-]*\.(ya?ml)$ ]] && ! [[ "$value" =~ \.\. ]] && ! [[ "$value" =~ /- ]] && return 0 ;;
    --timeout)
      [[ "$value" =~ ^[0-9]{1,4}[smh]$ ]] && return 0 ;;
    --out-format)
      case "$value" in
        json|text|colored-line-number|line-number|tab|colored-tab|checkstyle|code-climate|html|junit-xml|github-actions|sarif|teamcity) return 0 ;;
      esac ;;
    --new-from-rev)
      (( ${#value} <= 128 )) && [[ "$value" =~ ^[A-Za-z0-9._^][A-Za-z0-9._/~^-]*$ ]] && ! [[ "$value" =~ \.\. ]] && ! [[ "$value" =~ /~ ]] && return 0 ;;
    -show)
      [[ "$value" =~ ^[a-z]+$ ]] && return 0 ;;
  esac
  return 1
}

trap - ERR
if [[ -n "${HOME:-}" ]]; then
  LOG_DIR="$HOME/tmp"
  (umask 077; mkdir -p "$LOG_DIR") 2>/dev/null || true
  if [[ -d "$LOG_DIR" ]] && [[ -O "$LOG_DIR" ]] && ! [[ -L "$LOG_DIR" ]] && [[ -w "$LOG_DIR" ]]; then
    LOG_FILE="$LOG_DIR/scan-commands-$(printf '%(%Y-%m-%d)T' -1).log"
    if [[ -e "$LOG_FILE" ]] && [[ -L "$LOG_FILE" ]]; then
      :
    else
      _old_umask=$(umask); umask 077
      exec {_LOG_FD}>>"$LOG_FILE" 2>/dev/null || _LOG_FD=""
      umask "$_old_umask"
      if [[ -n "${_LOG_FD:-}" ]]; then
        if [[ -L "$LOG_FILE" ]] || ! [[ -O "$LOG_FILE" ]]; then
          exec {_LOG_FD}>&- 2>/dev/null; _LOG_FD=""
        fi
      fi
    fi
  fi
fi
trap "$_ERR_TRAP" ERR

# Validate environment-variable prefix values before the metacharacter
# check so each validation layer is independently responsible.
readonly _BLOCKED_ENV_RE='^(PATH|LD_.*|DYLD_.*|GOFLAGS|GOPROXY|GONOSUMCHECK|GONOSUMDB|GOPRIVATE|GOPATH|GOBIN|CGO_.*|HOME|BASH_ENV|ENV|IFS|GOROOT|GIT_.*|CDPATH|TMPDIR|GOTOOLCHAIN|GOMODCACHE|GONOPROXY|GOINSECURE|GOVCS|GOAUTH|HTTP_PROXY|HTTPS_PROXY|http_proxy|https_proxy|ALL_PROXY|all_proxy|NO_PROXY|no_proxy|SSL_CERT_DIR|SSL_CERT_FILE|GOWORK|GOEXPERIMENT|CC|CXX|AR|CFLAGS|LDFLAGS|PKG_CONFIG_PATH|GODEBUG|GOTELEMETRY|GOTELEMETRYDIR|GOCOVERDIR|GOGC|GOMEMLIMIT|GO111MODULE|SHELLOPTS|BASHOPTS|XDG_CONFIG_HOME|XDG_CONFIG_DIRS|PROMPT_COMMAND|TZ|PERL5LIB|PYTHONPATH|INPUTRC|LOCALDOMAIN|GLOBIGNORE|HOSTALIASES|RESOLV_HOST_CONF|RES_OPTIONS|GOENV|GOTMPDIR|GOCACHE|GOPACKAGESDRIVER|GCCGO|PKG_CONFIG|GCC_EXEC_PREFIX|CURL_CA_BUNDLE|EDITOR|VISUAL|NODE_OPTIONS|PERL5OPT|RUBYOPT|RUBYLIB|NODE_PATH|JAVA_TOOL_OPTIONS|_JAVA_OPTIONS|POSIXLY_CORRECT|BASH_XTRACEFD|EXECIGNORE|LIBRARY_PATH|OPENSSL_.*|PYTHONSTARTUP|PYTHONHOME|CPATH|C_INCLUDE_PATH|CPLUS_INCLUDE_PATH|MALLOC_CHECK_|MALLOC_PERTURB_|GLIBC_TUNABLES|GOLANGCI_LINT_.*|GOVULNDB|GOVULNCHECK_.*)$'
for (( _i=0; _i<_STRIP; _i++ )); do
  _prefix="${_NORM_TOKENS[$_i]}"
  [[ "$_prefix" == "env" || "$_prefix" =~ $_ENV_PATH_RE ]] && continue
  _name="${_prefix%%=*}"
  _val="${_prefix#*=}"
  if [[ "$_name" =~ $_BLOCKED_ENV_RE ]]; then
    deny "Dangerous environment variable '$_name' in prefix '$_prefix'"
  fi
  if ! [[ "$_val" =~ ^[a-zA-Z0-9_.-]*$ ]]; then
    deny "Unsafe value in environment variable prefix '$_prefix'"
  fi
done

# Defense-in-depth: reject shell metacharacters before token validation.
# read -ra splits on whitespace without interpreting shell syntax, so
# metacharacters could end up embedded in tokens that pass the allowlist.
readonly _METACHAR_RE='[][;|&$`(){}<>!\\"'"'"'#?*]'
if [[ "$ORIGINAL_COMMAND" =~ $_METACHAR_RE ]]; then
  deny "Command contains shell metacharacter"
fi

skip_next=false
pending_flag=""

for i in "${!TOKENS[@]}"; do
  token="${TOKENS[$i]}"

  if [[ "$skip_next" == true ]]; then
    if ! validate_flag_arg "$pending_flag" "$token"; then
      deny "Token '$token' failed validation as argument to '$pending_flag'"
    fi
    skip_next=false
    pending_flag=""
    continue
  fi

  # Handle --flag=value combined form
  if [[ "$token" == --*=* ]]; then
    _split_flag="${token%%=*}"
    _split_val="${token#*=}"
    if [[ -z "${ALLOWED[$_split_flag]+x}" ]]; then
      deny "Unknown flag '$_split_flag' in token '$token'"
    fi
    if [[ "${ALLOWED[$_split_flag]}" == "2" ]]; then
      if ! validate_flag_arg "$_split_flag" "$_split_val"; then
        deny "Value '$_split_val' failed validation as argument to '$_split_flag'"
      fi
    else
      deny "Flag '$_split_flag' does not accept an argument but got '$_split_val'"
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

if [[ "$skip_next" == true ]]; then
  deny "Flag '$pending_flag' requires an argument but command ended: $ORIGINAL_COMMAND"
fi

# All tokens validated
trap '' TERM INT PIPE HUP QUIT
trap - ERR
[[ $_HANDLED == 1 ]] && exit 0
_HANDLED=1
log "allow" "All tokens validated against scan command allowlist"
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"All tokens validated against scan command allowlist"}}\n' 2>/dev/null || exit 2
exit 0
