# femtoclaw in CX

A port of the agent core of
[manjunathshiva/femtoclaw](https://github.com/manjunathshiva/femtoclaw) at
commit `389effb450e4db2d2806af5c78010f4b38c114ad` to CX. femtoclaw is MIT
licensed; its license is in [LICENSE](LICENSE) and covers `original/` and the
port, which is a derivative work. femtoclaw is forked from
[memovai/mimiclaw](https://github.com/memovai/mimiclaw), also MIT, whose
notice is kept in [LICENSE-mimiclaw](LICENSE-mimiclaw).

- `original/` holds the unmodified upstream files the host harness compiles.
- `before/` is the host harness. `shim/` stands in for ESP-IDF and FreeRTOS,
  and `harness.c` feeds the original inbound messages and clock reads from a
  tape, maps `/spiffs/` to the working directory, sends model requests to the
  backend store and prints outbound messages.
- `port/` is the CX port, one declaration per `nnn-name.cx` file.
- `tests/` has one folder per test.
- `record.sh TEST_DIR` records a test again.

## What is ported

The Anthropic provider path of the agent loop: build the system prompt from
SOUL.md, USER.md, MEMORY.md and the daily notes of the last three days, load
the chat session, call the model with the tool list, run the tools it asks
for, loop until a final answer, save the session and send the reply. Two
tools are ported, `get_current_time` and `read_file`. The other nine stay
registered, so the tool list sent to the model is the original one, and
return `Error: tool '<name>' is not available in the host build` in both the
port and the harness.

Left out: Wi-Fi, Telegram, WebSocket, GPIO, display and button drivers, the
OpenAI provider path, the HTTP CONNECT proxy, NVS, payload logging, the
FreeRTOS task setup, the cron_add input patch (it only changes what cron_add
receives), skill enumeration (it never matches on a host directory, so it is
empty in both), the HTTP Date fallback of `get_current_time`, and the fixed C
buffer sizes. Time is formatted in UTC, the harness runs with `TZ=UTC0`.

## File mapping

| Original file : function | Port file |
|---|---|
| femto_config.h, bus/message_bus.h channel names | 000-configuration-values.cx |
| (new) text building without sprintf | 001-join-strings.cx |
| (new) wrappers: mode, state, byte table, tape | 002 to 020 |
| (new) file wrapper | 021-file-read.cx, 022-file-append.cx |
| (new) clock wrapper, the port of `time(NULL)` | 023-clock-now.cx |
| (new) network wrapper | 024-live-net-post.cx, 025-net-post.cx |
| bus/message_bus.h : femto_msg_t | 026-inbound-message.cx |
| bus/message_bus.c : message_bus_pop_inbound (wrapper) | 027-inbound-next.cx |
| cJSON : print_string, parse, print unformatted | 028 to 039 |
| memory/memory_store.c : localtime_r, strftime | 040 to 043 |
| memory/memory_store.c : get_date_str | 044-memory-date-string.cx |
| memory/memory_store.c : memory_read_long_term | 045-memory-read-long-term.cx |
| memory/memory_store.c : memory_read_recent | 046-memory-read-recent.cx |
| agent/context_builder.c : append_file | 047-context-append-file.cx |
| agent/context_builder.c : context_build_system_prompt | 048-context-build-system-prompt.cx |
| memory/session_mgr.c : session_path | 049-session-path.cx |
| memory/session_mgr.c : session_append | 050-session-append.cx |
| memory/session_mgr.c : session_get_history_json | 051-session-history-messages.cx |
| tools/tool_registry.h : femto_tool_t | 052-tool-definition.cx |
| tools/tool_registry.c : tool_registry_init | 053-tool-definitions.cx |
| tools/tool_registry.c : build_tools_json | 054-tools-json.cx |
| tools/tool_files.c : validate_path | 055-tool-validate-path.cx |
| tools/tool_files.c : tool_read_file_execute | 056-tool-read-file.cx |
| tools/tool_get_time.c : tool_get_time_execute | 057-tool-get-time.cx |
| before/harness.c : tool stubs | 058-tool-unavailable.cx |
| tools/tool_registry.c : tool_registry_execute | 059-tool-registry-execute.cx |
| llm/llm_proxy.h : llm_tool_call_t | 060-tool-call.cx |
| llm/llm_proxy.h : llm_response_t | 061-llm-reply.cx |
| llm/llm_proxy.c : llm_api_url | 062-llm-api-url.cx |
| llm/llm_proxy.c : llm_chat_tools, request body | 063-llm-request-body.cx |
| llm/llm_proxy.c : llm_chat_tools, response parsing | 064-llm-parse-reply.cx |
| llm/llm_proxy.c : llm_chat_tools | 065-llm-chat-tools.cx |
| agent/agent_loop.c : build_assistant_content | 066-agent-assistant-content.cx |
| agent/agent_loop.c : append_turn_context_prompt | 067-agent-turn-context.cx |
| agent/agent_loop.c : build_tool_results | 068-agent-tool-results.cx |
| before/harness.c : message_bus_push_outbound | 069-message-bus-push-outbound.cx |
| agent/agent_loop.c : agent_loop_task, one message | 070-agent-process-message.cx |
| agent/agent_loop.c : agent_loop_task, the loop | 071-agent-loop-run.cx |
| main/femto.c : app_main, reduced to start and stop | 072-main.cx |

## Running the port

```
cx port/*.cx ++mode=replay ++tape=tape.txt
cx port/*.cx ++mode=record ++tape=tape.txt ++inbound=inbound.txt ++client=backendstore ++backend=http://127.0.0.1:18080
```

Run it in the directory that holds `spiffs/`, since CX confines file access
to the working directory. `inbound.txt` has one message per line as channel,
chat id and content separated by tabs, with `\n`, `\t` and `\\` escapes.
`live` mode is `record` without the tape.
