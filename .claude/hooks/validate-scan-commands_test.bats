#!/usr/bin/env bats

SUT="$BATS_TEST_DIRNAME/validate-scan-commands.sh"

setup() {
  export HOME="$BATS_TMPDIR"
}

run_hook() {
  local input
  input=$(jq -n --arg cmd "$1" '{ tool_input: { command: $cmd } }')
  run bash -c 'printf "%s" "$1" | "$2"' _ "$input" "$SUT"
}

assert_allow() {
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecision == "allow"' >/dev/null
}

assert_deny() {
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.hookSpecificOutput.permissionDecision == "deny"' >/dev/null
}

assert_passthrough() {
  [ "$status" -eq 0 ]
  [ -z "$output" ]
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

@test "passthrough: =bar invalid variable prefix" {
  run_hook "=bar golangci-lint run ./..."
  assert_passthrough
}

# ── Deny: dangerous env-prefix values ─────────────────────────────

@test "deny: env var with command substitution" {
  run_hook 'FOO=$(id) golangci-lint run ./...'
  assert_deny
}

@test "deny: env var with backtick substitution" {
  run_hook 'FOO=`id` golangci-lint run ./...'
  assert_deny
}

@test "deny: PATH manipulation" {
  run_hook "PATH=/tmp/evil golangci-lint run ./..."
  assert_deny
}

@test "deny: env PATH manipulation" {
  run_hook "env PATH=/tmp/evil golangci-lint run ./..."
  assert_deny
}

@test "deny: LD_PRELOAD injection" {
  run_hook "LD_PRELOAD=/tmp/evil.so golangci-lint run ./..."
  assert_deny
}

# ── Deny: unknown tokens ──────────────────────────────────────────

@test "deny: unknown flag" {
  run_hook "golangci-lint run --evil ./..."
  assert_deny
}

@test "deny: unknown subcommand" {
  run_hook "golangci-lint exploit ./..."
  assert_deny
}

@test "deny: arbitrary path argument" {
  run_hook "golangci-lint run /etc/passwd"
  assert_deny
}

# ── Deny: injection attempts ──────────────────────────────────────

@test "deny: semicolon injection" {
  run_hook "golangci-lint run; rm -rf /"
  assert_deny
}

@test "deny: pipe injection" {
  run_hook "golangci-lint run | cat /etc/shadow"
  assert_deny
}

@test "deny: ampersand injection" {
  run_hook "golangci-lint run && curl evil.com"
  assert_deny
}

@test "deny: backtick injection" {
  run_hook 'golangci-lint run `whoami`'
  assert_deny
}

@test "deny: dollar-paren injection" {
  run_hook 'golangci-lint run $(id)'
  assert_deny
}

# ── Deny: invalid flag arguments ──────────────────────────────────

@test "deny: --config with shell injection" {
  run_hook "golangci-lint run --config ';rm -rf /'"
  assert_deny
}

@test "deny: --config with non-yaml extension" {
  run_hook "golangci-lint run --config exploit.sh"
  assert_deny
}

@test "deny: --timeout with invalid format" {
  run_hook "golangci-lint run --timeout forever"
  assert_deny
}

@test "deny: --new-from-rev with .. path traversal" {
  run_hook "golangci-lint run --new-from-rev ../../etc/passwd"
  assert_deny
}

@test "deny: --new-from-rev=.. combined form" {
  run_hook "golangci-lint run --new-from-rev=../main"
  assert_deny
}

@test "deny: --config=malicious combined form" {
  run_hook "golangci-lint run --config=;rm.sh"
  assert_deny
}

@test "deny: --timeout=bad combined form" {
  run_hook "golangci-lint run --timeout=abc"
  assert_deny
}

@test "deny: --config= empty value combined form" {
  run_hook "golangci-lint run --config= ./..."
  assert_deny
}

@test "deny: -c= short flag combined form" {
  run_hook "golangci-lint run -c=file.yaml ./..."
  assert_deny
}

@test "deny: --new-from-rev with reflog syntax (metacharacter)" {
  run_hook "golangci-lint run --new-from-rev HEAD@{0} ./..."
  assert_deny
}

# ── Deny: flag missing argument ───────────────────────────────────

@test "deny: --config at end of command" {
  run_hook "golangci-lint run --config"
  assert_deny
}

@test "deny: -c at end of command" {
  run_hook "golangci-lint run -c"
  assert_deny
}

@test "deny: --timeout at end of command" {
  run_hook "golangci-lint run --timeout"
  assert_deny
}

# ── Deny: standalone flag rejects = value ──────────────────────────

@test "deny: --version=something (standalone flag given a value)" {
  run_hook "golangci-lint --version=1.2.3"
  assert_deny
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

# ── Malformed input ────────────────────────────────────────────────

@test "deny: empty JSON object" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{}' "$SUT"
  [ "$status" -eq 0 ]
  assert_deny
}

@test "deny: missing command field" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {}}' "$SUT"
  [ "$status" -eq 0 ]
  assert_deny
}

@test "deny: non-JSON input" {
  run bash -c 'printf "%s" "$1" | "$2"' _ 'not json at all' "$SUT"
  [ "$status" -eq 0 ]
  assert_deny
}

@test "deny: empty input" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '' "$SUT"
  [ "$status" -eq 0 ]
  assert_deny
}

@test "deny: null command value" {
  run bash -c 'printf "%s" "$1" | "$2"' _ '{"tool_input": {"command": null}}' "$SUT"
  [ "$status" -eq 0 ]
  assert_deny
}

# ── Missing dependencies ──────────────────────────────────────────

@test "exits with error when jq is not available" {
  local fake_path="$BATS_TMPDIR/no-jq-bin"
  mkdir -p "$fake_path"
  ln -sf "$(command -v bash)" "$fake_path/bash"
  ln -sf "$(command -v cat)" "$fake_path/cat"
  ln -sf "$(command -v echo)" "$fake_path/echo"
  ln -sf "$(command -v date)" "$fake_path/date"
  ln -sf "$(command -v mkdir)" "$fake_path/mkdir"
  run env PATH="$fake_path" bash "$SUT" <<< '{}'
  [ "$status" -eq 0 ]
  local decision
  decision=$(echo "$output" | jq -r '.hookSpecificOutput.permissionDecision')
  [ "$decision" = "deny" ]
  [[ "$output" == *"jq is required"* ]]
}
