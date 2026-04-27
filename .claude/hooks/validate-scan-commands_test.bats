#!/usr/bin/env bats

SUT="$BATS_TEST_DIRNAME/validate-scan-commands.sh"

setup_file() {
  command -v jq >/dev/null 2>&1 || { echo "jq is required for the test suite" >&2; return 1; }
}

setup() {
  TEST_HOME="$(mktemp -d "$BATS_TMPDIR/hook-test-XXXXXX")" || return 1
  export HOME="$TEST_HOME"
}

teardown() {
  kill $(jobs -p) 2>/dev/null || true
  wait 2>/dev/null || true
  if [[ -n "${TEST_HOME:-}" && -d "$TEST_HOME" && "$TEST_HOME" == "$BATS_TMPDIR"/hook-test-* ]]; then
    chmod -R u+rwX "$TEST_HOME" 2>/dev/null || true
    rm -rf "$TEST_HOME"
  fi
}

run_hook() {
  local input
  input=$(jq -n --arg cmd "$1" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
}

assert_allow() {
  [ "$status" -eq 0 ] || { echo "Expected status 0, got $status. Output: $output" >&2; return 1; }
  local line_count
  line_count=$(echo "$output" | wc -l)
  [ "$line_count" -eq 1 ] || { echo "Expected 1 output line, got $line_count" >&2; return 1; }
  echo "$output" | jq -e '.hookSpecificOutput.hookEventName == "PreToolUse"' >/dev/null || {
    echo "Expected hookEventName 'PreToolUse', got: $output" >&2; return 1
  }
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecision == "allow"' >/dev/null || {
    echo "Expected allow, got: $output" >&2; return 1
  }
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecisionReason | type == "string" and length > 0' >/dev/null || {
    echo "Expected non-empty permissionDecisionReason, got: $output" >&2; return 1
  }
}

assert_deny() {
  local expected_reason="${1:-}"
  [ "$status" -eq 0 ] || { echo "Expected status 0, got $status. Output: $output" >&2; return 1; }
  local line_count
  line_count=$(echo "$output" | wc -l)
  [ "$line_count" -eq 1 ] || { echo "Expected 1 output line, got $line_count" >&2; return 1; }
  echo "$output" | jq -e '.hookSpecificOutput.hookEventName == "PreToolUse"' >/dev/null || {
    echo "Expected hookEventName 'PreToolUse', got: $output" >&2; return 1
  }
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecision == "deny"' >/dev/null || {
    echo "Expected deny, got: $output" >&2; return 1
  }
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecisionReason | type == "string" and length > 0' >/dev/null || {
    echo "Expected non-empty permissionDecisionReason, got: $output" >&2; return 1
  }
  if [[ -n "$expected_reason" ]]; then
    echo "$output" | jq -e --arg r "$expected_reason" '.hookSpecificOutput.permissionDecisionReason | contains($r)' >/dev/null || {
      echo "Expected reason containing '$expected_reason', got: $(echo "$output" | jq -r '.hookSpecificOutput.permissionDecisionReason')" >&2; return 1
    }
  fi
}

assert_passthrough() {
  [ "$status" -eq 0 ] || { echo "Expected status 0, got $status. Output: $output" >&2; return 1; }
  [ -z "$output" ] || { echo "Expected empty output, got: $output" >&2; return 1; }
}

get_perms() {
  if stat -c '%a' /dev/null >/dev/null 2>&1; then
    stat -c '%a' "$1"
  elif stat -f '%Lp' /dev/null >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    return 1
  fi
}

# ── Happy-path allows ──────────────────────────────────────────────

@test "allow: golangci-lint run ./..." {
  run_hook "golangci-lint run ./..."
  assert_allow
}

@test "allow: govulncheck ./..." {
  run_hook "govulncheck ./..."
  assert_allow
}

@test "allow: which golangci-lint" {
  run_hook "which golangci-lint"
  assert_allow
}

@test "allow: which govulncheck" {
  run_hook "which govulncheck"
  assert_allow
}

@test "allow: golangci-lint version" {
  run_hook "golangci-lint version"
  assert_allow
}

@test "allow: golangci-lint --version" {
  run_hook "golangci-lint --version"
  assert_allow
}

@test "allow: govulncheck -version" {
  run_hook "govulncheck -version"
  assert_allow
}

@test "allow: govulncheck version" {
  run_hook "govulncheck version"
  assert_allow
}

@test "allow: bare golangci-lint" {
  run_hook "golangci-lint"
  assert_allow
}

@test "allow: bare govulncheck" {
  run_hook "govulncheck"
  assert_allow
}

# ── Flag with separate argument ────────────────────────────────────

@test "allow: -c with yaml config" {
  run_hook "golangci-lint run -c .golangci.yaml ./..."
  assert_allow
}

@test "allow: --config with yml config" {
  run_hook "golangci-lint run --config path/to/config.yml ./..."
  assert_allow
}

@test "allow: --timeout with valid duration" {
  run_hook "golangci-lint run --timeout 5m ./..."
  assert_allow
}

@test "allow: --out-format with valid format" {
  run_hook "golangci-lint run --out-format json ./..."
  assert_allow
}

@test "allow: --new-from-rev with commit ref" {
  run_hook "golangci-lint run --new-from-rev HEAD~3 ./..."
  assert_allow
}

@test "allow: --new-from-rev with short SHA" {
  run_hook "golangci-lint run --new-from-rev abc123def ./..."
  assert_allow
}

@test "allow: --new-from-rev with full SHA" {
  run_hook "golangci-lint run --new-from-rev abc123def4567890abc123def4567890abc12345 ./..."
  assert_allow
}

@test "allow: --new-from-rev with tag ref" {
  run_hook "golangci-lint run --new-from-rev v1.2.3 ./..."
  assert_allow
}

@test "allow: --new-from-rev with caret syntax" {
  run_hook "golangci-lint run --new-from-rev HEAD^2 ./..."
  assert_allow
}

@test "allow: --out-format with hyphenated format" {
  run_hook "golangci-lint run --out-format colored-line-number ./..."
  assert_allow
}

@test "allow: govulncheck -show with valid value" {
  run_hook "govulncheck -show verbose ./..."
  assert_allow
}

# ── Flag=value combined form ───────────────────────────────────────

@test "allow: --config=file.yaml combined form" {
  run_hook "golangci-lint run --config=.golangci.yaml ./..."
  assert_allow
}

@test "allow: --timeout=10m combined form" {
  run_hook "golangci-lint run --timeout=10m ./..."
  assert_allow
}

@test "allow: --out-format=json combined form" {
  run_hook "golangci-lint run --out-format=json ./..."
  assert_allow
}

@test "allow: --new-from-rev=main combined form" {
  run_hook "golangci-lint run --new-from-rev=main ./..."
  assert_allow
}

@test "allow: multiple combined-form flags" {
  run_hook "golangci-lint run --config=.golangci.yaml --timeout=5m --out-format=json ./..."
  assert_allow
}

# ── Env-prefix normalization ──────────────────────────────────────

@test "allow: single VAR=val prefix" {
  run_hook "FOO=bar golangci-lint run ./..."
  assert_allow
}

@test "allow: multiple VAR=val prefixes" {
  run_hook "FOO=bar BAZ=qux golangci-lint run ./..."
  assert_allow
}

@test "allow: env keyword prefix" {
  run_hook "env golangci-lint run ./..."
  assert_allow
}

@test "allow: env with VAR=val prefix" {
  run_hook "env FOO=bar golangci-lint run ./..."
  assert_allow
}

@test "allow: chained env and VAR=val prefixes" {
  run_hook "FOO=bar env BAZ=qux golangci-lint run ./..."
  assert_allow
}

@test "allow: double env prefix" {
  run_hook "env env golangci-lint run ./..."
  assert_allow
}

@test "allow: env var with empty value" {
  run_hook "FOO= golangci-lint run ./..."
  assert_allow
}

@test "passthrough: =bar invalid variable prefix" {
  run_hook "=bar golangci-lint run ./..."
  assert_passthrough
}

# ── Deny: dangerous env-prefix values ─────────────────────────────

@test "deny: env var with command substitution" {
  run_hook 'FOO=$(id) golangci-lint run ./...'
  assert_deny "Unsafe value"
}

@test "deny: env var with backtick substitution" {
  run_hook 'FOO=`id` golangci-lint run ./...'
  assert_deny "Unsafe value"
}

@test "deny: PATH manipulation" {
  run_hook "PATH=/tmp/evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: env PATH manipulation" {
  run_hook "env PATH=/tmp/evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LD_PRELOAD injection" {
  run_hook "LD_PRELOAD=/tmp/evil.so golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PATH=. relative directory manipulation" {
  run_hook "PATH=. golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PATH=evildir relative directory manipulation" {
  run_hook "PATH=evildir golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LD_LIBRARY_PATH manipulation" {
  run_hook "LD_LIBRARY_PATH=. golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: DYLD_INSERT_LIBRARIES manipulation" {
  run_hook "DYLD_INSERT_LIBRARIES=evil.dylib golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: DYLD_LIBRARY_PATH manipulation" {
  run_hook "DYLD_LIBRARY_PATH=. golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: DYLD_FORCE_FLAT_NAMESPACE manipulation" {
  run_hook "DYLD_FORCE_FLAT_NAMESPACE=1 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: DYLD_FRAMEWORK_PATH manipulation" {
  run_hook "DYLD_FRAMEWORK_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: env PATH=. manipulation" {
  run_hook "env PATH=. golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOFLAGS manipulation" {
  run_hook "GOFLAGS=-insecure golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOPROXY manipulation" {
  run_hook "GOPROXY=evil.com golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GONOSUMCHECK manipulation" {
  run_hook "GONOSUMCHECK=on golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GONOSUMDB manipulation" {
  run_hook "GONOSUMDB=on golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOPRIVATE manipulation" {
  run_hook "GOPRIVATE=evil.com golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: HOME manipulation" {
  run_hook "HOME=tmp golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "passthrough: command empty after normalization (non-scan)" {
  run_hook "FOO=bar env"
  assert_passthrough
}

@test "deny: LD_PRELOAD with safe value blocked by name" {
  run_hook "LD_PRELOAD=safe golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "passthrough: env alone (non-scan)" {
  run_hook "env"
  assert_passthrough
}

@test "deny: env var value with special chars bypassing metachar check" {
  run_hook "FOO=val%ue golangci-lint run ./..."
  assert_deny "Unsafe value"
}

@test "deny: env var value with @ bypassing metachar check" {
  run_hook "FOO=val@ue golangci-lint run ./..."
  assert_deny "Unsafe value"
}

@test "deny: env var value containing equals sign" {
  run_hook "FOO=bar=baz golangci-lint run ./..."
  assert_deny "Unsafe value"
}

# ── Deny: unknown tokens ──────────────────────────────────────────

@test "deny: unknown flag" {
  run_hook "golangci-lint run --evil ./..."
  assert_deny "Unknown token"
}

@test "deny: unknown subcommand" {
  run_hook "golangci-lint exploit ./..."
  assert_deny "Unknown token"
}

@test "deny: arbitrary path argument" {
  run_hook "golangci-lint run /etc/passwd"
  assert_deny "Unknown token"
}

# ── Deny: injection attempts ──────────────────────────────────────

@test "deny: semicolon injection" {
  run_hook "golangci-lint run; rm -rf /"
  assert_deny "metacharacter"
}

@test "deny: pipe injection" {
  run_hook "golangci-lint run | cat /etc/shadow"
  assert_deny "metacharacter"
}

@test "deny: ampersand injection" {
  run_hook "golangci-lint run && curl evil.com"
  assert_deny "metacharacter"
}

@test "deny: backtick injection" {
  run_hook 'golangci-lint run `whoami`'
  assert_deny "metacharacter"
}

@test "deny: dollar-paren injection" {
  run_hook 'golangci-lint run $(id)'
  assert_deny "metacharacter"
}

# ── Deny: invalid flag arguments ──────────────────────────────────

@test "deny: --config with shell injection" {
  run_hook "golangci-lint run --config ';rm -rf /'"
  assert_deny "metacharacter"
}

@test "deny: --config with non-yaml extension" {
  run_hook "golangci-lint run --config exploit.sh"
  assert_deny "failed validation"
}

@test "deny: --config with .. path traversal" {
  run_hook "golangci-lint run --config ../../etc/config.yaml"
  assert_deny "failed validation"
}

@test "deny: --config=.. combined form path traversal" {
  run_hook "golangci-lint run --config=../../etc/config.yaml"
  assert_deny "failed validation"
}

@test "deny: --config with absolute path" {
  run_hook "golangci-lint run --config /etc/secrets/creds.yaml"
  assert_deny "failed validation"
}

@test "deny: --config= with absolute path" {
  run_hook "golangci-lint run --config=/etc/secrets/creds.yaml"
  assert_deny "failed validation"
}

@test "deny: -c with .. path traversal" {
  run_hook "golangci-lint run -c ../other/config.yml"
  assert_deny "failed validation"
}

@test "deny: --timeout with invalid format" {
  run_hook "golangci-lint run --timeout forever"
  assert_deny "failed validation"
}

@test "deny: --new-from-rev with .. path traversal" {
  run_hook "golangci-lint run --new-from-rev ../../etc/passwd"
  assert_deny "failed validation"
}

@test "deny: --new-from-rev=.. combined form" {
  run_hook "golangci-lint run --new-from-rev=../main"
  assert_deny "failed validation"
}

@test "deny: unknown --flag=value combined form" {
  run_hook "golangci-lint run --evil=value ./..."
  assert_deny "Unknown flag"
}

@test "deny: --config=malicious combined form" {
  run_hook "golangci-lint run --config=;rm.sh"
  assert_deny "metacharacter"
}

@test "deny: --timeout=bad combined form" {
  run_hook "golangci-lint run --timeout=abc"
  assert_deny "failed validation"
}

@test "deny: --config= empty value combined form" {
  run_hook "golangci-lint run --config= ./..."
  assert_deny "failed validation"
}

@test "deny: --timeout= empty value combined form" {
  run_hook "golangci-lint run --timeout= ./..."
  assert_deny "failed validation"
}

@test "deny: --out-format= empty value combined form" {
  run_hook "golangci-lint run --out-format= ./..."
  assert_deny "failed validation"
}

@test "deny: --out-format with unknown format" {
  run_hook "golangci-lint run --out-format xml ./..."
  assert_deny "failed validation"
}

@test "deny: -show=verbose single-dash combined form" {
  run_hook "govulncheck -show=verbose ./..."
  assert_deny "Unknown token"
}

@test "deny: -c=file.yaml single-dash equals form (caught as unknown token)" {
  run_hook "golangci-lint run -c=file.yaml ./..."
  assert_deny "Unknown token"
}

@test "deny: --new-from-rev with reflog syntax (metacharacter)" {
  run_hook "golangci-lint run --new-from-rev HEAD@{0} ./..."
  assert_deny "metacharacter"
}

@test "deny: --new-from-rev exceeding 128 char length limit" {
  local long_ref
  long_ref=$(printf 'a%.0s' {1..129})
  run_hook "golangci-lint run --new-from-rev $long_ref ./..."
  assert_deny "failed validation"
}

# ── Deny: flag missing argument ───────────────────────────────────

@test "deny: --config at end of command" {
  run_hook "golangci-lint run --config"
  assert_deny "requires an argument"
}

@test "deny: -c at end of command" {
  run_hook "golangci-lint run -c"
  assert_deny "requires an argument"
}

@test "deny: --timeout at end of command" {
  run_hook "golangci-lint run --timeout"
  assert_deny "requires an argument"
}

@test "deny: --out-format at end of command" {
  run_hook "golangci-lint run --out-format"
  assert_deny "requires an argument"
}

@test "deny: --new-from-rev at end of command" {
  run_hook "golangci-lint run --new-from-rev"
  assert_deny "requires an argument"
}

# ── Deny: standalone flag rejects = value ──────────────────────────

@test "deny: --version=something (standalone flag given a value)" {
  run_hook "golangci-lint --version=1.2.3"
  assert_deny "does not accept an argument"
}

# ── Passthrough: non-scan commands ─────────────────────────────────

@test "passthrough: ls" {
  run_hook "ls -la"
  assert_passthrough
}

@test "passthrough: go build" {
  run_hook "go build ./..."
  assert_passthrough
}

@test "passthrough: git status" {
  run_hook "git status"
  assert_passthrough
}

@test "passthrough: echo" {
  run_hook "echo hello"
  assert_passthrough
}

@test "passthrough: which with extra arguments" {
  run_hook "which golangci-lint foo"
  assert_passthrough
}

@test "passthrough: mixed-case GolangCI-Lint is not recognized" {
  run_hook "GolangCI-Lint run ./..."
  assert_passthrough
}

@test "passthrough: uppercase GOLANGCI-LINT is not recognized" {
  run_hook "GOLANGCI-LINT run ./..."
  assert_passthrough
}

# ── Malformed input ────────────────────────────────────────────────

@test "passthrough: empty JSON object" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{}' "$SUT"
  assert_passthrough
}

@test "passthrough: missing command field" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {}}' "$SUT"
  assert_passthrough
}

@test "passthrough: non-JSON input" {
  run bash -c 'printf "%s" "$1" | "$2"' _ 'not json at all' "$SUT"
  assert_passthrough
}

@test "passthrough: empty input" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '' "$SUT"
  assert_passthrough
}

@test "passthrough: null command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": null}}' "$SUT"
  assert_passthrough
}

# ── Logging failure resilience ────────────────────────────────────

run_hook_no_stderr() {
  local input
  input=$(jq -n --arg cmd "$1" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" 2>/dev/null' _ "$input" "$SUT"
}

@test "allow: hook works when log directory is not writable" {
  mkdir -p "$HOME/tmp"
  chmod 000 "$HOME/tmp"
  run_hook_no_stderr "golangci-lint run ./..."
  assert_allow
}

@test "deny: hook denies when log directory is not writable" {
  mkdir -p "$HOME/tmp"
  chmod 000 "$HOME/tmp"
  run_hook_no_stderr "golangci-lint run; rm -rf /"
  assert_deny "metacharacter"
}

@test "log file records allow decision" {
  mkdir -p "$HOME/tmp"
  run_hook "golangci-lint run ./..."
  assert_allow
  local log_file
  log_file=$(ls "$HOME/tmp"/scan-commands-*.log 2>/dev/null | head -1)
  [ -n "$log_file" ] && [ -f "$log_file" ]
  grep -q "decision=allow" "$log_file"
  grep -q "golangci-lint" "$log_file"
}

@test "log file records deny decision" {
  mkdir -p "$HOME/tmp"
  run_hook "golangci-lint run --evil ./..."
  assert_deny "Unknown token"
  local log_file
  log_file=$(ls "$HOME/tmp"/scan-commands-*.log 2>/dev/null | head -1)
  [ -n "$log_file" ] && [ -f "$log_file" ]
  grep -q "decision=deny" "$log_file"
}

@test "allow: hook works when HOME is nonexistent path" {
  export HOME="$TEST_HOME/does-not-exist-subdir"
  run_hook_no_stderr "golangci-lint run ./..."
  assert_allow
}

# ── Deny: input size limit ────────────────────────────────────────

@test "deny: input exceeding 1MB raw input limit" {
  [ -c /dev/zero ] || skip "/dev/zero not available"
  run bash -c 'head -c 1048577 < /dev/zero | tr "\0" "a" | "$1"' _ "$SUT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"1MB"* ]]
}

@test "size check: input at exactly 1MB with scan tool name passes size check" {
  [ -c /dev/zero ] || skip "/dev/zero not available"
  local prefix='{"tool_input":{"command":"golangci-lint run ./...'
  local suffix='"}}'
  local overhead=$(( ${#prefix} + ${#suffix} ))
  local padding_len=$(( 1048576 - overhead ))
  run bash -c '
    { printf "%s" "$1"; head -c "$3" < /dev/zero | tr "\0" "a"; printf "%s" "$2"; } | "$4"
  ' _ "$prefix" "$suffix" "$padding_len" "$SUT"
  [ "$status" -eq 0 ]
  [[ "$output" != *"1MB"* ]]
}

@test "deny: command exceeding 4096 characters" {
  local padding
  padding=$(printf 'a%.0s' {1..4077})
  run_hook "golangci-lint run ./$padding"
  assert_deny "maximum length"
}

@test "length check does not trigger at exactly 4096 characters" {
  # "golangci-lint run ./" = 20 chars; padding brings total to exactly 4096
  local padding
  padding=$(printf 'a%.0s' {1..4076})
  run_hook "golangci-lint run ./$padding"
  # Will be denied for unknown token, but NOT for the length check
  assert_deny "Unknown token"
  [[ "$output" != *"maximum length"* ]]
}

# ── Additional --out-format coverage ──────────────────────────────

@test "allow: --out-format sarif" {
  run_hook "golangci-lint run --out-format sarif ./..."
  assert_allow
}

@test "allow: --out-format github-actions" {
  run_hook "golangci-lint run --out-format github-actions ./..."
  assert_allow
}

@test "allow: --out-format tab" {
  run_hook "golangci-lint run --out-format tab ./..."
  assert_allow
}

@test "allow: --out-format checkstyle" {
  run_hook "golangci-lint run --out-format checkstyle ./..."
  assert_allow
}

@test "allow: --out-format html" {
  run_hook "golangci-lint run --out-format html ./..."
  assert_allow
}

@test "allow: --out-format teamcity" {
  run_hook "golangci-lint run --out-format teamcity ./..."
  assert_allow
}

# ── --new-from-rev boundary ───────────────────────────────────────

@test "allow: --new-from-rev at exactly 128 chars" {
  local ref
  ref=$(printf 'a%.0s' {1..128})
  run_hook "golangci-lint run --new-from-rev $ref ./..."
  assert_allow
}

# ── --timeout edge cases ─────────────────────────────────────────

@test "allow: --timeout 0s" {
  run_hook "golangci-lint run --timeout 0s ./..."
  assert_allow
}

@test "deny: --timeout missing unit" {
  run_hook "golangci-lint run --timeout 10 ./..."
  assert_deny "failed validation"
}

@test "deny: --timeout invalid unit" {
  run_hook "golangci-lint run --timeout 10d ./..."
  assert_deny "failed validation"
}

# ── Mixed safe + dangerous env vars ──────────────────────────────

@test "deny: safe env var followed by dangerous PATH" {
  run_hook "FOO=bar PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: dangerous env var followed by safe" {
  run_hook "GOFLAGS=-insecure FOO=bar golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Token ordering ───────────────────────────────────────────────

@test "passthrough: valid tokens in wrong order (subcommand first)" {
  run_hook "run golangci-lint ./..."
  assert_passthrough
}

@test "allow: ./... before subcommand (allowlist does not enforce order)" {
  run_hook "golangci-lint ./... run"
  assert_allow
}

# ── Duplicate flags ──────────────────────────────────────────────

@test "allow: same flag specified twice" {
  run_hook "golangci-lint run --timeout 5m --timeout 10m ./..."
  assert_allow
}

# ── Additional metacharacter coverage ────────────────────────────

@test "deny: open parenthesis metacharacter" {
  run_hook "golangci-lint run (./..."
  assert_deny "metacharacter"
}

@test "deny: close parenthesis metacharacter" {
  run_hook "golangci-lint run )./..."
  assert_deny "metacharacter"
}

@test "deny: open brace metacharacter" {
  run_hook "golangci-lint run {./..."
  assert_deny "metacharacter"
}

@test "deny: close brace metacharacter" {
  run_hook "golangci-lint run }./..."
  assert_deny "metacharacter"
}

@test "deny: less-than metacharacter" {
  run_hook "golangci-lint run < /etc/passwd"
  assert_deny "metacharacter"
}

@test "deny: greater-than metacharacter" {
  run_hook "golangci-lint run > /tmp/out"
  assert_deny "metacharacter"
}

@test "deny: exclamation metacharacter" {
  run_hook "golangci-lint run !./..."
  assert_deny "metacharacter"
}

@test "deny: backslash metacharacter" {
  run_hook 'golangci-lint run \.\/...'
  assert_deny "metacharacter"
}

# ── Missing dependencies ──────────────────────────────────────────

@test "deny: jq is not available" {
  local fake_path="$TEST_HOME/no-jq-bin"
  mkdir -p "$fake_path"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v head)" "$fake_path/head"
  ln -sf "$(command -v timeout)" "$fake_path/timeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  assert_deny "Required dependency"
}

# ── Control character injection (C2) ─────────────────────────────

@test "deny: newline injection in command" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run ./...\nmalicious-command' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
}

@test "deny: tab injection in command" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run\t./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
}

@test "JSON escape: deny reason with control chars produces valid JSON" {
  run_hook $'golangci-lint\x01run'
  assert_deny "control character"
  echo "$output" | jq -e . >/dev/null
}

@test "deny: carriage return injection in command" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run ./...\rmalicious' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
}

# ── Glob character rejection (H1) ────────────────────────────────

@test "deny: asterisk glob in command" {
  run_hook "golangci-lint run ./*"
  assert_deny "metacharacter"
}

@test "deny: question mark glob in command" {
  run_hook "golangci-lint run ./?.go"
  assert_deny "metacharacter"
}

@test "deny: square bracket glob in command" {
  run_hook "golangci-lint run ./[a-z].go"
  assert_deny "metacharacter"
}

@test "deny: hash character in command" {
  run_hook "golangci-lint run ./...#comment"
  assert_deny "metacharacter"
}

@test "deny: tilde in non-ref position" {
  run_hook "golangci-lint run ~/evil"
  assert_deny "Unknown token"
}

# ── Newly-blocked env vars (H2) ──────────────────────────────────

@test "deny: BASH_ENV manipulation" {
  run_hook "BASH_ENV=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: ENV manipulation" {
  run_hook "ENV=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: IFS manipulation" {
  run_hook "IFS=x golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOROOT manipulation" {
  run_hook "GOROOT=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CGO_CFLAGS manipulation" {
  run_hook "CGO_CFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CGO_LDFLAGS manipulation" {
  run_hook "CGO_LDFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GIT_DIR manipulation" {
  run_hook "GIT_DIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GIT_WORK_TREE manipulation" {
  run_hook "GIT_WORK_TREE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── -show flag edge cases (L7) ───────────────────────────────────

@test "deny: -show with uppercase value" {
  run_hook "govulncheck -show VERBOSE ./..."
  assert_deny "failed validation"
}

@test "deny: -show with numeric value" {
  run_hook "govulncheck -show 123 ./..."
  assert_deny "failed validation"
}

@test "deny: -show with empty value (next token)" {
  run_hook "govulncheck -show"
  assert_deny "requires an argument"
}

# ── Config extension near-misses (L8) ────────────────────────────

@test "deny: --config with .yamlx extension" {
  run_hook "golangci-lint run --config file.yamlx"
  assert_deny "failed validation"
}

@test "deny: --config with no extension" {
  run_hook "golangci-lint run --config golangci"
  assert_deny "failed validation"
}

@test "deny: --config with .json extension" {
  run_hook "golangci-lint run --config config.json"
  assert_deny "failed validation"
}

@test "deny: --config with .toml extension" {
  run_hook "golangci-lint run --config config.toml"
  assert_deny "failed validation"
}

# ── Multiple whitespace between tokens (L9) ──────────────────────

@test "allow: multiple spaces between tokens" {
  run_hook "golangci-lint   run   ./..."
  assert_allow
}

# ── --timeout unbounded value (L2) ───────────────────────────────

@test "deny: --timeout with more than 4 digits" {
  run_hook "golangci-lint run --timeout 99999h ./..."
  assert_deny "failed validation"
}

@test "allow: --timeout with exactly 4 digits" {
  run_hook "golangci-lint run --timeout 9999s ./..."
  assert_allow
}

# ── Blocked Go env vars (Finding 3/20) ──────────────────────────────

@test "deny: GOPATH manipulation" {
  run_hook "GOPATH=/tmp/malicious golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOBIN manipulation" {
  run_hook "GOBIN=/tmp/evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CGO_ENABLED manipulation" {
  run_hook "CGO_ENABLED=1 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── JSON escaping verification (Finding 9) ──────────────────────────

@test "deny: JSON output is valid when deny reason contains special chars" {
  run_hook "golangci-lint run --evil-flag ./..."
  assert_deny "Unknown token"
  echo "$output" | jq -e . >/dev/null || {
    echo "Invalid JSON output: $output" >&2; return 1
  }
}

@test "passthrough: scan tool name in env var value, command empty after normalization" {
  run_hook "FOO=golangci-lint env"
  assert_passthrough
}

@test "passthrough: empty-after-normalization (non-scan)" {
  run_hook "FOO=safe env"
  assert_passthrough
}

# ── Non-string command field types (Finding 10) ─────────────────────

@test "passthrough: numeric command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": 42}}' "$SUT"
  assert_passthrough
}

@test "passthrough: boolean command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": true}}' "$SUT"
  assert_passthrough
}

@test "passthrough: array command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": ["ls"]}}' "$SUT"
  assert_passthrough
}

@test "passthrough: object command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": {"cmd": "ls"}}}' "$SUT"
  assert_passthrough
}

# ── Whitespace-only command (Finding 11) ────────────────────────────

@test "passthrough: empty command string with scan tool name elsewhere in JSON" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": ""}, "tool_name": "golangci-lint"}' "$SUT"
  assert_passthrough
}

@test "passthrough: whitespace-only command" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": "   "}}' "$SUT"
  assert_passthrough
}

# ── Log file permissions (Finding 12) ───────────────────────────────

@test "log directory created with 700 permissions" {
  run_hook "golangci-lint run ./..."
  assert_allow
  local dir_perms
  dir_perms=$(get_perms "$HOME/tmp") || skip "Cannot determine stat format"
  [ "$dir_perms" = "700" ]
}

@test "log file created with 600 permissions" {
  run_hook "golangci-lint run ./..."
  assert_allow
  local log_file
  log_file=$(ls "$HOME/tmp"/scan-commands-*.log 2>/dev/null | head -1)
  [ -n "$log_file" ] && [ -f "$log_file" ]
  local file_perms
  file_perms=$(get_perms "$log_file") || skip "Cannot determine stat format"
  [ "$file_perms" = "600" ]
}

# ── Wildcard env var pattern coverage (Finding #16) ───────────────

@test "deny: GIT_CONFIG manipulation" {
  run_hook "GIT_CONFIG=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: DYLD_CUSTOM_VAR manipulation" {
  run_hook "DYLD_CUSTOM_VAR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Argument separator (Finding #17) ─────────────────────────────

@test "deny: double-dash argument separator" {
  run_hook "golangci-lint run -- ./..."
  assert_deny "Unknown token"
}

# ── Quote metacharacters (Finding #18) ────────────────────────────

@test "deny: double quote metacharacter" {
  run_hook 'golangci-lint run "./..."'
  assert_deny "metacharacter"
}

@test "deny: single quote metacharacter" {
  run_hook "golangci-lint run './...'"
  assert_deny "metacharacter"
}

# ── Newly-blocked env vars (Finding #5) ──────────────────────────

@test "deny: CDPATH manipulation" {
  run_hook "CDPATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: TMPDIR manipulation" {
  run_hook "TMPDIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOTOOLCHAIN manipulation" {
  run_hook "GOTOOLCHAIN=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOMODCACHE manipulation" {
  run_hook "GOMODCACHE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GONOPROXY manipulation" {
  run_hook "GONOPROXY=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── GOINSECURE blocklist (M9) ──────────────────────────────────────

@test "deny: GOINSECURE manipulation" {
  run_hook "GOINSECURE=evil.com golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Flag consuming another flag as argument (H8) ──────────────────

@test "deny: flag consuming another flag as its argument" {
  run_hook "golangci-lint run --config --timeout ./..."
  assert_deny "failed validation"
}

@test "deny: flag consuming subcommand as its argument" {
  run_hook "golangci-lint --config run"
  assert_deny "failed validation"
}

@test "deny: --config with directory path ending in slash (no extension)" {
  run_hook "golangci-lint run --config dir/"
  assert_deny "failed validation"
}

# ── Passthrough with metacharacters (M6) ──────────────────────────

@test "passthrough: non-scan command with metacharacters passes through" {
  run_hook "ls; rm -rf /"
  assert_passthrough
}

# ── Double env with dangerous var (M13) ───────────────────────────

@test "deny: env env PATH double env prefix" {
  run_hook "env env PATH=/tmp/evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Non-ASCII rejection (C2) ────────────────────────────────────

@test "deny: non-ASCII characters in command" {
  run_hook "golangci-lint run ./...ñ"
  assert_deny "non-ASCII"
}

# ── HOME completely unset (M7) ──────────────────────────────────

@test "allow: hook works when HOME is completely unset" {
  unset HOME
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" 2>/dev/null' _ "$input" "$SUT"
  assert_allow
}

# ── Expanded env var blocklist (C1) ─────────────────────────────

@test "deny: CGO_CXXFLAGS manipulation" {
  run_hook "CGO_CXXFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CGO_CPPFLAGS manipulation" {
  run_hook "CGO_CPPFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LD_AUDIT manipulation" {
  run_hook "LD_AUDIT=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: HTTP_PROXY manipulation" {
  run_hook "HTTP_PROXY=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: https_proxy manipulation" {
  run_hook "https_proxy=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: ALL_PROXY manipulation" {
  run_hook "ALL_PROXY=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: SSL_CERT_FILE manipulation" {
  run_hook "SSL_CERT_FILE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOVCS manipulation" {
  run_hook "GOVCS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOAUTH manipulation" {
  run_hook "GOAUTH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Symlink log file detection (C4) ─────────────────────────────

@test "log file symlink is detected and logging is disabled" {
  mkdir -p "$HOME/tmp"
  local expected_date
  expected_date=$(printf '%(%Y-%m-%d)T' -1)
  local log_file="$HOME/tmp/scan-commands-${expected_date}.log"
  local target_file="$HOME/tmp/symlink-target.log"
  ln -sf "$target_file" "$log_file"
  run_hook "golangci-lint run ./..."
  assert_allow
  [ ! -f "$target_file" ] || { ! grep -q "decision=" "$target_file"; }
}

# ── Missing env var blocklist tests (H6/L6) ─────────────────────

@test "deny: NO_PROXY manipulation" {
  run_hook "NO_PROXY=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: no_proxy manipulation" {
  run_hook "no_proxy=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: http_proxy manipulation" {
  run_hook "http_proxy=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: SSL_CERT_DIR manipulation" {
  run_hook "SSL_CERT_DIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Flag validation edge cases (H6) ────────────────────────────

@test "allow: --config with multiple dots in filename" {
  run_hook "golangci-lint run --config config.test.yaml ./..."
  assert_allow
}

@test "deny: --config=value with equals in value" {
  run_hook "golangci-lint run --config=path=file.yaml ./..."
  assert_deny "failed validation"
}

@test "allow: three consecutive arg-consuming flags" {
  run_hook "golangci-lint run --config .golangci.yaml --timeout 5m --out-format json ./..."
  assert_allow
}

# ── Edge case: token after valid flag pair, double equals, negative timeout ──

@test "deny: unknown token after valid flag-argument pair" {
  run_hook "golangci-lint run --config .golangci.yaml unknown ./..."
  assert_deny "Unknown token"
}

@test "deny: --config with double equals" {
  run_hook "golangci-lint run --config==file.yaml ./..."
  assert_deny "failed validation"
}

@test "deny: -show with hyphen value" {
  run_hook "govulncheck -show -verbose ./..."
  assert_deny "failed validation"
}

@test "deny: --timeout with negative number" {
  run_hook "golangci-lint run --timeout -5m ./..."
  assert_deny "failed validation"
}

# ── Token count limit ────────────────────────────────────────────

@test "deny: command with more than 100 tokens" {
  local cmd="golangci-lint run"
  for i in $(seq 1 100); do
    cmd+=" ./..."
  done
  run_hook "$cmd"
  assert_deny "Too many tokens"
}

# ── Malformed JSON with scan tool name (H5) ─────────────────────

@test "passthrough: JSON contains golangci-lint but not in command field" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"other_field": "golangci-lint run ./..."}' "$SUT"
  assert_passthrough
}

# ── Log directory symlink detection (M7) ─────────────────────────

@test "log directory symlink is detected and logging is disabled" {
  local target_dir="$TEST_HOME/symlink-target-dir"
  mkdir -p "$target_dir"
  ln -sf "$target_dir" "$HOME/tmp"
  run_hook "golangci-lint run ./..."
  assert_allow
  local expected_date
  expected_date=$(printf '%(%Y-%m-%d)T' -1)
  ! [ -f "$target_dir/scan-commands-${expected_date}.log" ]
}

# ── Missing --out-format coverage (M8) ───────────────────────────

@test "allow: --out-format text" {
  run_hook "golangci-lint run --out-format text ./..."
  assert_allow
}

@test "allow: --out-format line-number" {
  run_hook "golangci-lint run --out-format line-number ./..."
  assert_allow
}

@test "allow: --out-format colored-tab" {
  run_hook "golangci-lint run --out-format colored-tab ./..."
  assert_allow
}

@test "allow: --out-format code-climate" {
  run_hook "golangci-lint run --out-format code-climate ./..."
  assert_allow
}

@test "allow: --out-format junit-xml" {
  run_hook "golangci-lint run --out-format junit-xml ./..."
  assert_allow
}

# ── Expanded env var blocklist (H4) ──────────────────────────────

@test "deny: GOWORK manipulation" {
  run_hook "GOWORK=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOEXPERIMENT manipulation" {
  run_hook "GOEXPERIMENT=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CC manipulation" {
  run_hook "CC=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CXX manipulation" {
  run_hook "CXX=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: AR manipulation" {
  run_hook "AR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CFLAGS manipulation" {
  run_hook "CFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LDFLAGS manipulation" {
  run_hook "LDFLAGS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PKG_CONFIG_PATH manipulation" {
  run_hook "PKG_CONFIG_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Token count boundary (exactly 100) ─────────────────────────────

@test "allow: command with exactly 100 tokens" {
  local cmd="golangci-lint run"
  for i in $(seq 1 98); do
    cmd+=" ./..."
  done
  run_hook "$cmd"
  assert_allow
}

# ── Expanded env var blocklist (Finding 3.1) ───────────────────────

@test "deny: GODEBUG manipulation" {
  run_hook "GODEBUG=gctrace golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOTELEMETRY manipulation" {
  run_hook "GOTELEMETRY=off golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOTELEMETRYDIR manipulation" {
  run_hook "GOTELEMETRYDIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOCOVERDIR manipulation" {
  run_hook "GOCOVERDIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOGC manipulation" {
  run_hook "GOGC=off golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOMEMLIMIT manipulation" {
  run_hook "GOMEMLIMIT=1GiB golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GO111MODULE manipulation" {
  run_hook "GO111MODULE=off golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: --config with leading hyphen in filename" {
  run_hook "golangci-lint run --config -.yaml ./..."
  assert_deny "failed validation"
}

# ── Deny: env flag bypass (C1/C6) ──────────────────────────────────

@test "deny: env -i flag bypasses validation" {
  run_hook "env -i golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: env -u flag bypasses validation" {
  run_hook "env -u GONOSUMCHECK golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: env -P flag bypasses validation" {
  run_hook "env -P /evil golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: VAR=val followed by env flag" {
  run_hook "FOO=bar -i golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: env env -i double env with flag" {
  run_hook "env env -i golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

# ── Review fixes: additional coverage ────────────────────────────

@test "deny: HTTPS_PROXY uppercase manipulation" {
  run_hook "HTTPS_PROXY=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "passthrough: env var name starting with digit" {
  run_hook "9FOO=bar golangci-lint run ./..."
  assert_passthrough
}

@test "allow: --new-from-rev with slash in ref (origin/main)" {
  run_hook "golangci-lint run --new-from-rev origin/main ./..."
  assert_allow
}

@test "allow: --timeout with hours unit" {
  run_hook "golangci-lint run --timeout 2h ./..."
  assert_allow
}

@test "deny: LD_DEBUG manipulation" {
  run_hook "LD_DEBUG=all golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LD_DEBUG_OUTPUT manipulation" {
  run_hook "LD_DEBUG_OUTPUT=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LD_PROFILE manipulation" {
  run_hook "LD_PROFILE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Missing dependency: timeout (C3) ──────────────────────────────

@test "deny: timeout/gtimeout is not available" {
  local fake_path="$TEST_HOME/no-timeout-bin"
  mkdir -p "$fake_path"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v jq)" "$fake_path/jq"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  assert_deny "Required dependency"
}

# ── Newly-blocked env vars (H4) ──────────────────────────────────

@test "deny: SHELLOPTS manipulation" {
  run_hook "SHELLOPTS=posix golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: BASHOPTS manipulation" {
  run_hook "BASHOPTS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: XDG_CONFIG_HOME manipulation" {
  run_hook "XDG_CONFIG_HOME=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: XDG_CONFIG_DIRS manipulation" {
  run_hook "XDG_CONFIG_DIRS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PROMPT_COMMAND manipulation" {
  run_hook "PROMPT_COMMAND=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: TZ manipulation" {
  run_hook "TZ=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PERL5LIB manipulation" {
  run_hook "PERL5LIB=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PYTHONPATH manipulation" {
  run_hook "PYTHONPATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: INPUTRC manipulation" {
  run_hook "INPUTRC=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: LOCALDOMAIN manipulation" {
  run_hook "LOCALDOMAIN=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GLOBIGNORE manipulation" {
  run_hook "GLOBIGNORE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── jq parse failure on malformed JSON containing scan tool name (C1/H2) ──

@test "deny: malformed JSON containing scan tool name" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": golangci-lint}}' "$SUT"
  assert_deny "JSON parsing failed"
}

@test "deny: truncated JSON containing scan tool name" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": "golangci-lint run' "$SUT"
  assert_deny "JSON parsing failed"
}

# ── Input read failure (2.6) ──────────────────────────────────────

@test "passthrough: closed stdin produces passthrough" {
  run bash -c 'exec <&-; "$1"' _ "$SUT"
  assert_passthrough
}

# ── Token count boundary with env prefixes (3.10) ────────────────

@test "deny: token count over 100 with env prefixes pushing count" {
  local cmd="golangci-lint run"
  for i in $(seq 1 50); do
    cmd="V${i}=x $cmd"
  done
  for i in $(seq 1 49); do
    cmd+=" ./..."
  done
  run_hook "$cmd"
  assert_deny "Too many tokens"
}

# ── jq infrastructure failure (1.6) ──────────────────────────────

@test "deny: jq infrastructure failure (rc 125-127)" {
  local fake_path="$TEST_HOME/fake-jq-bin"
  mkdir -p "$fake_path"
  printf '#!/bin/bash\nexit 126\n' > "$fake_path/jq"
  chmod +x "$fake_path/jq"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v head)" "$fake_path/head"
  ln -sf "$(command -v timeout)" "$fake_path/timeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  [ "$status" -eq 0 ]
  [[ "$output" == *'"permissionDecision":"deny"'* ]]
  [[ "$output" == *"infrastructure failure"* ]]
}

# ── Passthrough: non-scan command after env normalization (3.9) ───

@test "passthrough: non-scan command after env normalization" {
  run_hook "FOO=bar ls -la"
  assert_passthrough
}

# ── Timeout and infrastructure failure paths (1.5) ───────────────

@test "deny: jq parsing timeout (rc 124)" {
  local fake_path="$TEST_HOME/fake-jq-timeout-bin"
  mkdir -p "$fake_path"
  printf '#!/bin/bash\ncat >/dev/null; exit 124\n' > "$fake_path/jq"
  chmod +x "$fake_path/jq"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v head)" "$fake_path/head"
  ln -sf "$(command -v timeout)" "$fake_path/timeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  [ "$status" -eq 0 ]
  [[ "$output" == *'"permissionDecision":"deny"'* ]]
  [[ "$output" == *"timed out"* ]]
}

@test "deny: input read timeout via slow pipe" {
  run bash -c 'HOOK_READ_TIMEOUT=1 "$1" < <(sleep 5) 2>/dev/null' _ "$SUT"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"permissionDecision":"deny"'* ]]
  [[ "$output" == *"Input read timed out or interrupted"* ]]
}

@test "passthrough: non-scan command works without timeout" {
  local fake_path="$TEST_HOME/no-timeout-bin2"
  mkdir -p "$fake_path"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v jq)" "$fake_path/jq"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"ls -la"}}'
  assert_passthrough
}

# ── --new-from-rev flag injection and tilde expansion (2.2, 3.3) ──

@test "deny: --new-from-rev with leading hyphen (flag injection)" {
  run_hook "golangci-lint run --new-from-rev --exec ./..."
  assert_deny "failed validation"
}

@test "deny: --new-from-rev=--exec combined form (flag injection)" {
  run_hook "golangci-lint run --new-from-rev=--exec ./..."
  assert_deny "failed validation"
}

@test "deny: --new-from-rev with tilde home expansion" {
  run_hook "golangci-lint run --new-from-rev ~/evil ./..."
  assert_deny "failed validation"
}

@test "deny: --new-from-rev with tilde after slash" {
  run_hook "golangci-lint run --new-from-rev origin/~evil ./..."
  assert_deny "failed validation"
}

# ── Additional coverage from code review ────────────────────────────

@test "deny: --new-from-rev with bare double-dot" {
  run_hook "golangci-lint run --new-from-rev .. ./..."
  assert_deny "failed validation"
}

@test "deny: --=value empty flag name" {
  run_hook "golangci-lint run --=value ./..."
  assert_deny "Unknown flag"
}

@test "deny: --out-format with uppercase JSON" {
  run_hook "golangci-lint run --out-format JSON ./..."
  assert_deny "failed validation"
}

@test "allow: --config with leading dot in directory name" {
  run_hook "golangci-lint run --config .hidden/config.yaml ./..."
  assert_allow
}

@test "deny: HOSTALIASES manipulation" {
  run_hook "HOSTALIASES=evil.txt golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: RESOLV_HOST_CONF manipulation" {
  run_hook "RESOLV_HOST_CONF=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: RES_OPTIONS manipulation" {
  run_hook "RES_OPTIONS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "passthrough: bare which" {
  run_hook "which"
  assert_passthrough
}

# ── Blocked Go env vars: GOENV, GOTMPDIR, GOCACHE (2.1) ────────────

@test "deny: GOENV manipulation" {
  run_hook "GOENV=evil.conf golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOTMPDIR manipulation" {
  run_hook "GOTMPDIR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOCACHE manipulation" {
  run_hook "GOCACHE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: all_proxy lowercase manipulation" {
  run_hook "all_proxy=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: command with exactly 101 tokens" {
  local cmd="golangci-lint run"
  for i in $(seq 1 99); do
    cmd+=" ./..."
  done
  run_hook "$cmd"
  assert_deny "Too many tokens"
}

@test "deny: backtick metacharacter standalone" {
  run_hook 'golangci-lint run ` ./...'
  assert_deny "metacharacter"
}

@test "passthrough: whitespace-only command with scan tool name in other field" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": "   ", "note": "golangci-lint"}}' "$SUT"
  assert_passthrough
}

# ── Review fix: M5 — config flag consuming positional arg ──────────

@test "deny: --config consumes ./... as its argument and fails validation" {
  run_hook "golangci-lint run --config ./... run"
  assert_deny "failed validation as argument to '--config'"
}

# ── Review fix: M6 — lowercase env var not in blocklist ────────────

@test "allow: lowercase path env var is not in blocklist" {
  run_hook "path=something golangci-lint run ./..."
  assert_allow
}

# ── Review fix: L4 — env -S injection vector ──────────────────────

@test "deny: env -S flag for string splitting" {
  run_hook "env -S golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

# ── Review fix: L17 — duplicate env var assignments ────────────────

@test "allow: duplicate env var assignments" {
  run_hook "FOO=bar FOO=baz golangci-lint run ./..."
  assert_allow
}

# ── C1 fix: deny exits non-zero when stdout is broken ────────────

@test "deny: exits non-zero when stdout write fails" {
  [ -c /dev/full ] || skip "/dev/full not available"
  local input
  input=$(jq -n --arg cmd "golangci-lint run --evil" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" >/dev/full 2>/dev/null' _ "$input" "$SUT"
  [ "$status" -eq 2 ]
}

# ── H1 fix: GOPACKAGESDRIVER and related env vars blocked ────────

@test "deny: GOPACKAGESDRIVER manipulation" {
  run_hook "GOPACKAGESDRIVER=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GCCGO manipulation" {
  run_hook "GCCGO=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PKG_CONFIG manipulation" {
  run_hook "PKG_CONFIG=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GCC_EXEC_PREFIX manipulation" {
  run_hook "GCC_EXEC_PREFIX=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── H1 fix: allow path exits non-zero when stdout is broken ────────

@test "allow: exits non-zero when stdout write fails on allow path" {
  [ -c /dev/full ] || skip "/dev/full not available"
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" >/dev/full 2>/dev/null' _ "$input" "$SUT"
  [ "$status" -eq 2 ]
}

# ── H4 fix: newly blocked env vars ─────────────────────────────────

@test "deny: CURL_CA_BUNDLE manipulation" {
  run_hook "CURL_CA_BUNDLE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: EDITOR manipulation" {
  run_hook "EDITOR=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: VISUAL manipulation" {
  run_hook "VISUAL=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── M6: env -- double-dash test ────────────────────────────────────

@test "deny: env -- end-of-options marker" {
  run_hook "env -- golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

# ── L6: --config with digit-starting filename ──────────────────────

@test "allow: --config starting with digit" {
  run_hook "golangci-lint run --config 1config.yaml ./..."
  assert_allow
}

# ── L7: -show with hyphenated value ────────────────────────────────

@test "deny: -show with hyphenated value" {
  run_hook "govulncheck -show some-thing ./..."
  assert_deny "failed validation"
}

# ── Review fix: H1 — missing interpreter injection env vars ──────

@test "deny: NODE_OPTIONS manipulation" {
  run_hook "NODE_OPTIONS=--require=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PERL5OPT manipulation" {
  run_hook "PERL5OPT=-Mevil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: RUBYOPT manipulation" {
  run_hook "RUBYOPT=-revil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: RUBYLIB manipulation" {
  run_hook "RUBYLIB=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: NODE_PATH manipulation" {
  run_hook "NODE_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Review fix: H3 — config path with hyphen after directory separator ──

@test "deny: --config with hyphen after directory separator" {
  run_hook "golangci-lint run --config dir/-evil.yaml ./..."
  assert_deny "failed validation"
}

@test "deny: --config=dir/-evil.yaml combined form" {
  run_hook "golangci-lint run --config=dir/-evil.yaml ./..."
  assert_deny "failed validation"
}

# ── M2 fix: newly blocked env vars ──────────────────────────────────

@test "deny: JAVA_TOOL_OPTIONS manipulation" {
  run_hook "JAVA_TOOL_OPTIONS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: _JAVA_OPTIONS manipulation" {
  run_hook "_JAVA_OPTIONS=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: POSIXLY_CORRECT manipulation" {
  run_hook "POSIXLY_CORRECT=1 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: BASH_XTRACEFD manipulation" {
  run_hook "BASH_XTRACEFD=5 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: EXECIGNORE manipulation" {
  run_hook "EXECIGNORE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── H3 fix: govulncheck --version (L11) ──────────────────────────────

@test "allow: govulncheck --version" {
  run_hook "govulncheck --version"
  assert_allow
}

# ── M10 fix: near-miss env var names allowed ─────────────────────────

@test "allow: PATH1 is not blocked (near-miss for PATH)" {
  run_hook "PATH1=safe golangci-lint run ./..."
  assert_allow
}

@test "allow: GOFLAGS1 is not blocked (near-miss for GOFLAGS)" {
  run_hook "GOFLAGS1=safe golangci-lint run ./..."
  assert_allow
}

@test "allow: LD is not blocked (near-miss for LD_*)" {
  run_hook "LD=safe golangci-lint run ./..."
  assert_allow
}

# ── M11 fix: --timeout with no digits ────────────────────────────────

@test "deny: --timeout with unit but no digits" {
  run_hook "golangci-lint run --timeout s ./..."
  assert_deny "failed validation"
}

# ── H5 fix: gtimeout code path ──────────────────────────────────────

@test "allow: hook works with gtimeout instead of timeout" {
  local fake_path="$TEST_HOME/gtimeout-bin"
  mkdir -p "$fake_path"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v jq)" "$fake_path/jq"
  ln -sf "$(command -v timeout)" "$fake_path/gtimeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  assert_allow
}

# ── L5 fix: --config with triple dots before extension ───────────────

@test "deny: --config with triple dots before extension" {
  run_hook "golangci-lint run --config ...config.yaml ./..."
  assert_deny "failed validation"
}

# ── L6 fix: --new-from-rev HEAD^ bare caret ─────────────────────────

@test "allow: --new-from-rev with bare caret HEAD^" {
  run_hook "golangci-lint run --new-from-rev HEAD^ ./..."
  assert_allow
}

# ── L7 fix: --timeout with 4-digit minutes and hours ────────────────

@test "allow: --timeout with 4-digit minutes" {
  run_hook "golangci-lint run --timeout 9999m ./..."
  assert_allow
}

@test "allow: --timeout with 4-digit hours" {
  run_hook "golangci-lint run --timeout 9999h ./..."
  assert_allow
}

# ── M7 fix: --new-from-rev rejects leading slash ────────────────────

@test "deny: --new-from-rev with leading slash (absolute path)" {
  run_hook "golangci-lint run --new-from-rev /etc/passwd ./..."
  assert_deny "failed validation"
}

# ── Finding 2.7: JSON validation timeout path ─────────────────────────

@test "deny: JSON validation timeout (rc 124 on second jq call)" {
  local fake_path="$TEST_HOME/fake-jq-val-timeout"
  mkdir -p "$fake_path"
  printf '#!/bin/bash\ncat >/dev/null\nif [[ "$*" == *"-r"* ]]; then exit 2; else exit 124; fi\n' > "$fake_path/jq"
  chmod +x "$fake_path/jq"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v head)" "$fake_path/head"
  ln -sf "$(command -v timeout)" "$fake_path/timeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  [ "$status" -eq 0 ]
  [[ "$output" == *'"permissionDecision":"deny"'* ]]
  [[ "$output" == *"JSON validation timed out"* ]]
}

@test "deny: jq validation infrastructure failure (rc 125-127 on second jq call)" {
  local fake_path="$TEST_HOME/fake-jq-val-infra"
  mkdir -p "$fake_path"
  printf '#!/bin/bash\ncat >/dev/null\nif [[ "$*" == *"-r"* ]]; then exit 2; else exit 126; fi\n' > "$fake_path/jq"
  chmod +x "$fake_path/jq"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v head)" "$fake_path/head"
  ln -sf "$(command -v timeout)" "$fake_path/timeout"
  run env PATH="$fake_path" bash "$SUT" <<< '{"tool_input":{"command":"golangci-lint run ./..."}}'
  [ "$status" -eq 0 ]
  [[ "$output" == *'"permissionDecision":"deny"'* ]]
  [[ "$output" == *"infrastructure failure"* ]]
}

# ── Finding 3.11: --new-from-rev= empty value combined form ───────────

@test "deny: --new-from-rev= empty value combined form" {
  run_hook "golangci-lint run --new-from-rev= ./..."
  assert_deny "failed validation"
}

# ── M8 fix: ESC and DEL control character tests ─────────────────────

@test "deny: ESC character injection in command" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run \x1b./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
}

@test "deny: form feed control character produces valid JSON" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run \f./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
  echo "$output" | jq -e . >/dev/null
}

@test "deny: vertical tab control character produces valid JSON" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run \v./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
  echo "$output" | jq -e . >/dev/null
}

@test "deny: backspace control character produces valid JSON" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run \b./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
  echo "$output" | jq -e . >/dev/null
}

@test "deny: DEL character injection in command" {
  local input
  input=$(jq -n --arg cmd $'golangci-lint run \x7f./...' '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_deny "control character"
}

# ── Review fix: H3 — newly blocked linker/OpenSSL env vars ──────

@test "deny: LIBRARY_PATH manipulation" {
  run_hook "LIBRARY_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: OPENSSL_CONF manipulation" {
  run_hook "OPENSSL_CONF=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: OPENSSL_ENGINES manipulation" {
  run_hook "OPENSSL_ENGINES=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Review fix: L7 — standalone & metacharacter ─────────────────

@test "deny: standalone ampersand background operator" {
  run_hook "golangci-lint run & wait"
  assert_deny "metacharacter"
}

# ── Review fix: L8 — minimal env var names ───────────────────────

@test "allow: single underscore env var name" {
  run_hook "_=value golangci-lint run ./..."
  assert_allow
}

@test "allow: double underscore env var name" {
  run_hook "__=value golangci-lint run ./..."
  assert_allow
}

# ── Review fix: 2.5 — OPENSSL_.* wildcard coverage ──────────────

@test "deny: OPENSSL_MODULES manipulation" {
  run_hook "OPENSSL_MODULES=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── Review fix: 3.9 — stderr contamination checks ───────────────

@test "stderr: allow path produces no stderr" {
  local input stderr_file="$TEST_HOME/stderr.txt"
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" 2>"$3"' _ "$input" "$SUT" "$stderr_file"
  assert_allow
  [ ! -s "$stderr_file" ] || { echo "Unexpected stderr: $(cat "$stderr_file")" >&2; return 1; }
}

@test "stderr: deny path produces no stderr" {
  local input stderr_file="$TEST_HOME/stderr.txt"
  input=$(jq -n --arg cmd "golangci-lint run --evil" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" 2>"$3"' _ "$input" "$SUT" "$stderr_file"
  assert_deny "Unknown token"
  [ ! -s "$stderr_file" ] || { echo "Unexpected stderr: $(cat "$stderr_file")" >&2; return 1; }
}

@test "stderr: passthrough produces no stderr" {
  local input stderr_file="$TEST_HOME/stderr.txt"
  input=$(jq -n --arg cmd "ls -la" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" 2>"$3"' _ "$input" "$SUT" "$stderr_file"
  assert_passthrough
  [ ! -s "$stderr_file" ] || { echo "Unexpected stderr: $(cat "$stderr_file")" >&2; return 1; }
}

# ── L6 fix: newly blocked env vars (defense-in-depth) ────────────

@test "deny: PYTHONSTARTUP manipulation" {
  run_hook "PYTHONSTARTUP=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: PYTHONHOME manipulation" {
  run_hook "PYTHONHOME=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CPATH manipulation" {
  run_hook "CPATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: C_INCLUDE_PATH manipulation" {
  run_hook "C_INCLUDE_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: CPLUS_INCLUDE_PATH manipulation" {
  run_hook "CPLUS_INCLUDE_PATH=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: MALLOC_CHECK_ manipulation" {
  run_hook "MALLOC_CHECK_=3 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: MALLOC_PERTURB_ manipulation" {
  run_hook "MALLOC_PERTURB_=1 golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GLIBC_TUNABLES manipulation" {
  run_hook "GLIBC_TUNABLES=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── H6 fix: log file path exists as directory ───────────────────

@test "allow: hook works when log file path exists as directory" {
  mkdir -p "$HOME/tmp"
  local expected_date
  expected_date=$(printf '%(%Y-%m-%d)T' -1)
  mkdir -p "$HOME/tmp/scan-commands-${expected_date}.log"
  run_hook_no_stderr "golangci-lint run ./..."
  assert_allow
}

# ── M10 fix: passthrough does not create log file ───────────────

@test "passthrough: does not create log file" {
  run_hook "ls -la"
  assert_passthrough
  local log_count
  log_count=$(find "$HOME/tmp" -name 'scan-commands-*.log' -type f 2>/dev/null | wc -l)
  [ "$log_count" -eq 0 ] || { echo "Expected no log files, found $log_count" >&2; return 1; }
}

# ── L9 fix: --config with ./ relative path ─────────────────────

@test "allow: --config with explicit ./ relative path" {
  run_hook "golangci-lint run --config ./config.yaml ./..."
  assert_allow
}

# ── L10 fix: --new-from-rev with hyphen in branch name ─────────

@test "allow: --new-from-rev with hyphen in branch name" {
  run_hook "golangci-lint run --new-from-rev feature-branch ./..."
  assert_allow
}

# ── L7 fix: env var name with metacharacter ─────────────────────────

@test "passthrough: env var name containing semicolon is not parsed as env prefix" {
  run_hook "FOO;BAR=val golangci-lint run ./..."
  assert_passthrough
}

# ── L8 fix: --timeout=value with equals in value ────────────────────

@test "deny: --timeout=5=m equals sign in combined-form value" {
  run_hook "golangci-lint run --timeout=5=m ./..."
  assert_deny "failed validation"
}

# ── L9 fix: tool name typo near-misses passthrough ──────────────────

@test "passthrough: golangcilint missing hyphen is not recognized" {
  run_hook "golangcilint run ./..."
  assert_passthrough
}

@test "passthrough: golangci-lin missing trailing t is not recognized" {
  run_hook "golangci-lin run ./..."
  assert_passthrough
}

# ── M8: --config with leading hyphen at start of path ────────────────

@test "deny: --config with leading hyphen at start of path" {
  run_hook "golangci-lint run --config -dir/config.yaml ./..."
  assert_deny "failed validation"
}

# ── H4 fix: tool-specific env vars blocked ──────────────────────────

@test "deny: GOLANGCI_LINT_CACHE manipulation" {
  run_hook "GOLANGCI_LINT_CACHE=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOLANGCI_LINT_CONFIG manipulation" {
  run_hook "GOLANGCI_LINT_CONFIG=evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

# ── M9: env long-form options ────────────────────────────────────────

@test "deny: env --ignore-environment long-form flag" {
  run_hook "env --ignore-environment golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: env --unset=PATH long-form flag" {
  run_hook "env --unset=PATH golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "deny: env --split-string long-form flag" {
  run_hook "env --split-string golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

# ── L4: --config with uppercase extension ────────────────────────────

@test "deny: --config with uppercase .YAML extension" {
  run_hook "golangci-lint run --config config.YAML ./..."
  assert_deny "failed validation"
}

@test "deny: --config with uppercase .YML extension" {
  run_hook "golangci-lint run --config config.YML ./..."
  assert_deny "failed validation"
}

# ── L5: --new-from-rev with all-special-character refs ───────────────

@test "allow: --new-from-rev with consecutive carets" {
  run_hook "golangci-lint run --new-from-rev HEAD^^^ ./..."
  assert_allow
}

@test "deny: --new-from-rev with consecutive dots (contains ..)" {
  run_hook "golangci-lint run --new-from-rev ... ./..."
  assert_deny "failed validation"
}

# ── H2 fix: timeout value validation (sanitized to defaults) ────────

@test "allow: invalid HOOK_READ_TIMEOUT is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_READ_TIMEOUT=abc bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

@test "allow: excessive HOOK_READ_TIMEOUT is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_READ_TIMEOUT=9999 bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

@test "allow: zero HOOK_READ_TIMEOUT is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_READ_TIMEOUT=0 bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

@test "allow: HOOK_READ_TIMEOUT at boundary value 300" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_READ_TIMEOUT=300 bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

@test "allow: HOOK_READ_TIMEOUT=301 is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_READ_TIMEOUT=301 bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

@test "allow: invalid HOOK_JQ_TIMEOUT is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_JQ_TIMEOUT=abc bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

# ── H1 fix: GOVULNDB and GOVULNCHECK_.* blocked ──────────────────────

@test "deny: GOVULNDB manipulation" {
  run_hook "GOVULNDB=evil.example.com govulncheck ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOVULNCHECK_CACHE manipulation" {
  run_hook "GOVULNCHECK_CACHE=evil govulncheck ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: GOVULNCHECK_FORMAT manipulation" {
  run_hook "GOVULNCHECK_FORMAT=evil govulncheck ./..."
  assert_deny "Dangerous environment variable"
}

@test "allow: excessive HOOK_JQ_TIMEOUT is sanitized to default" {
  local input
  input=$(jq -n --arg cmd "golangci-lint run ./..." '{ tool_input: { command: $cmd } }')
  run env HOOK_JQ_TIMEOUT=9999 bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
  assert_allow
}

# ── C3 fix: signal handler test coverage ────────────────────────────

@test "signal: SIGTERM during input read produces deny JSON" {
  local output_file="$TEST_HOME/signal-output.txt"
  local fifo="$TEST_HOME/sig-fifo"
  mkfifo "$fifo"
  sleep 60 > "$fifo" &
  local writer_pid=$!
  env HOOK_READ_TIMEOUT=60 bash "$SUT" < "$fifo" > "$output_file" 2>/dev/null &
  local hook_pid=$!
  sleep 0.5
  kill -TERM "$hook_pid" 2>/dev/null || true
  wait "$hook_pid" 2>/dev/null || true
  kill "$writer_pid" 2>/dev/null || true
  wait "$writer_pid" 2>/dev/null || true
  [ -s "$output_file" ] || { echo "No output produced" >&2; return 1; }
  local sig_output
  sig_output=$(cat "$output_file")
  echo "$sig_output" | jq -e '.hookSpecificOutput.permissionDecision == "deny"' >/dev/null || {
    echo "Expected deny, got: $sig_output" >&2; return 1
  }
  echo "$sig_output" | jq -e '.hookSpecificOutput.permissionDecisionReason | contains("signal")' >/dev/null || {
    echo "Expected signal reason, got: $sig_output" >&2; return 1
  }
}

@test "signal: SIGHUP during input read produces deny JSON" {
  local output_file="$TEST_HOME/signal-output.txt"
  local fifo="$TEST_HOME/sig-fifo"
  mkfifo "$fifo"
  sleep 60 > "$fifo" &
  local writer_pid=$!
  env HOOK_READ_TIMEOUT=60 bash "$SUT" < "$fifo" > "$output_file" 2>/dev/null &
  local hook_pid=$!
  sleep 0.5
  kill -HUP "$hook_pid" 2>/dev/null || true
  wait "$hook_pid" 2>/dev/null || true
  kill "$writer_pid" 2>/dev/null || true
  wait "$writer_pid" 2>/dev/null || true
  [ -s "$output_file" ] || { echo "No output produced" >&2; return 1; }
  local sig_output
  sig_output=$(cat "$output_file")
  echo "$sig_output" | jq -e '.hookSpecificOutput.permissionDecision == "deny"' >/dev/null || {
    echo "Expected deny, got: $sig_output" >&2; return 1
  }
  echo "$sig_output" | jq -e '.hookSpecificOutput.permissionDecisionReason | contains("signal")' >/dev/null || {
    echo "Expected signal reason, got: $sig_output" >&2; return 1
  }
}

# ── Review coverage gaps (T1, T3, T4, T5, T6) ──────────────────────

@test "deny: PATH= empty value is blocked by name" {
  run_hook "PATH= golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: --config with extension-only filename .yaml" {
  run_hook "golangci-lint run --config .yaml ./..."
  assert_deny "failed validation"
}

@test "allow: --config with consecutive slashes in path" {
  run_hook "golangci-lint run --config path//config.yaml ./..."
  assert_allow
}

@test "allow: --new-from-rev with leading dot in branch name" {
  run_hook "golangci-lint run --new-from-rev .hidden-branch ./..."
  assert_allow
}

@test "allow: --new-from-rev with underscore in branch name" {
  run_hook "golangci-lint run --new-from-rev my_branch ./..."
  assert_allow
}

# ── M6 fix: /usr/bin/env and /bin/env path variants ────────────────

@test "allow: /usr/bin/env golangci-lint run" {
  run_hook "/usr/bin/env golangci-lint run ./..."
  assert_allow
}

@test "allow: /bin/env golangci-lint run" {
  run_hook "/bin/env golangci-lint run ./..."
  assert_allow
}

@test "allow: /usr/bin/env with VAR=val prefix" {
  run_hook "/usr/bin/env FOO=bar golangci-lint run ./..."
  assert_allow
}

@test "allow: /usr/bin/env govulncheck" {
  run_hook "/usr/bin/env govulncheck ./..."
  assert_allow
}

@test "deny: /usr/bin/env with dangerous env var" {
  run_hook "/usr/bin/env PATH=/tmp/evil golangci-lint run ./..."
  assert_deny "Dangerous environment variable"
}

@test "deny: /usr/bin/env with flag" {
  run_hook "/usr/bin/env -i golangci-lint run ./..."
  assert_deny "Unsupported env flag"
}

@test "passthrough: /usr/bin/env alone" {
  run_hook "/usr/bin/env"
  assert_passthrough
}

# ── M10 fix: script sourcing prevention ─────────────────────────────

@test "script cannot be sourced" {
  run bash -c 'source "$1"' _ "$SUT"
  [ "$status" -ne 0 ]
}

# ── L10 fix: config path with trailing slash after extension ────────

@test "deny: --config with trailing slash after extension" {
  run_hook "golangci-lint run --config config.yaml/"
  assert_deny "failed validation"
}

# ── L11 fix: mixed-case --out-format value ──────────────────────────

@test "deny: --out-format with mixed-case value" {
  run_hook "golangci-lint run --out-format Json ./..."
  assert_deny "failed validation"
}

# ── M6 fix: passthrough with broken stdout ─────────────────────────

@test "passthrough: exits zero even when stdout is broken" {
  [ -c /dev/full ] || skip "/dev/full not available"
  local input
  input=$(jq -n --arg cmd "ls -la" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2" >/dev/full 2>/dev/null' _ "$input" "$SUT"
  [ "$status" -eq 0 ]
}
