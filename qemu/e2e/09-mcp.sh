# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "MCP server (mcp.$DOMAIN/mcp)"
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}'
check "MCP without token -> 401" bash -c "[[ \$(curl -sk -o /dev/null -w '%{http_code}' --resolve mcp.$DOMAIN:$HTTPS_PORT:$HOST -X POST -H 'Content-Type: application/json' --data '{}' https://mcp.$DOMAIN:$HTTPS_PORT/mcp) == 401 ]]"
check "initialize returns server info" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_A' '$INIT' | jq -e '.result.serverInfo.name == \"fodycloud\"' >/dev/null"
TOOLS="$(mcp "$TOK_A" '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | jq -r '[.result.tools[].name] | sort | join(",")')"
check "tools/list exposes the app tools" bash -c "[[ '$TOOLS' == *create_app* && '$TOOLS' == *list_stacks* && '$TOOLS' == *confirm_stack* && '$TOOLS' == *get_build_logs* && '$TOOLS' == *delete_app* ]]"
check "no admin tools exposed over MCP" bash -c "[[ '$TOOLS' != *plan,* && '$TOOLS' != *set_plan* ]]"
check "delete_app is annotated destructive" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_A' '{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/list\"}' | jq -e '.result.tools[] | select(.name==\"delete_app\") | .annotations.destructiveHint == true' >/dev/null"
check "list_apps returns the user's app" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_A' '{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"list_apps\",\"arguments\":{}}}' | jq -r '.result.content[0].text' | grep -q '\"name\": \"$APP\"'"
check "user B's agent cannot read A's logs" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_B' '{\"jsonrpc\":\"2.0\",\"id\":5,\"method\":\"tools/call\",\"params\":{\"name\":\"get_build_logs\",\"arguments\":{\"app\":\"$APP\"}}}' | jq -e '.result.isError == true' >/dev/null"
check "read-only token refused on deploy" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_RO' '{\"jsonrpc\":\"2.0\",\"id\":6,\"method\":\"tools/call\",\"params\":{\"name\":\"deploy\",\"arguments\":{\"app\":\"$APP\"}}}' | jq -e '.result.isError == true and (.result.content[0].text | test(\"read-only\"))' >/dev/null"
check "create_app via MCP rejects a private repo" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_A' '{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"tools/call\",\"params\":{\"name\":\"create_app\",\"arguments\":{\"name\":\"e2e-mcp\",\"repo_url\":\"https://github.com/fodydev/definitely-not-a-repo-xyz\"}}}' | jq -e '.result.isError == true and (.result.content[0].text | test(\"not public\"))' >/dev/null"
check "get_plan_and_usage works" bash -c "$(declare -f mcp); HTTPS_PORT=$HTTPS_PORT HOST=$HOST DOMAIN=$DOMAIN mcp '$TOK_A' '{\"jsonrpc\":\"2.0\",\"id\":8,\"method\":\"tools/call\",\"params\":{\"name\":\"get_plan_and_usage\",\"arguments\":{}}}' | jq -r '.result.content[0].text' | grep -q '\"apps_used\": 1'"
mcp_call() { mcp "$TOK_A" "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" | jq -r '.result.content[0].text'; }
check "list_stacks offers every language" bash -c 'jq -e "[.[].id] == [\"static\",\"python\",\"node\",\"php\",\"go\",\"java\"]" <<<"$1" >/dev/null' _ "$(mcp_call list_stacks '{}')"
check "agent creates an app without a stack: analysis, no deploy" bash -c 'jq -e ".deployment.kind == \"detect\"" <<<"$1" >/dev/null' _ \
  "$(mcp_call create_app "{\"name\":\"e2e-mcp\",\"repo_url\":\"$SAMPLE_REPO\"}")"
for _ in $(seq 1 100); do [[ "$(mcp_call get_app '{"app":"e2e-mcp"}' | jq -r .status)" == review ]] && break; sleep 3; done
MCP_APP="$(mcp_call get_app '{"app":"e2e-mcp"}')"
check "get_app shows the detection to review" bash -c 'jq -e ".status == \"review\" and .detection.language == \"node\" and (.stack_confirmed|not)" <<<"$1" >/dev/null' _ "$MCP_APP"
check "agent confirms the user's choice -> build queued" bash -c 'jq -e ".deployment.kind == \"build\"" <<<"$1" >/dev/null' _ \
  "$(mcp_call confirm_stack '{"app":"e2e-mcp","language":"node","framework":"node-server"}')"
mcp_call delete_app '{"app":"e2e-mcp","confirm":"e2e-mcp"}' >/dev/null
