# mimiclaw in CX

A port of the agent core of
[memovai/mimiclaw](https://github.com/memovai/mimiclaw) at commit
`dcc1c8c7bcccfc60bc336c76666b67d4bdb50648` to CX. mimiclaw is MIT
licensed; its license is in [LICENSE](LICENSE) and covers `original/` and the
port, which is a derivative work.

mimiclaw is the codebase femtoclaw was forked from, so this port starts from
the [femtoclaw port](../femtoclaw/README.md). The wrappers, the tape, the JSON
helpers and the session and memory code are the same. What differs is the
context text, the tool list, the working status, the fixed Anthropic URL, the
timezone handling and `get_current_time`.

- `original/` holds the unmodified upstream files the host harness compiles.
- `before/` is the host harness, adapted from the femtoclaw one. `shim/`
  stands in for ESP-IDF and FreeRTOS.
- `port/` is the CX port, one declaration per `nnn-name.cx` file.
- `tests/` has one folder per test. The `tool-`, `pair-` and `chain-` folders
  are generated from `tests.table`, see the main README.
- `record.sh TEST_DIR` records a test again.

## What is ported

The Anthropic provider path of the agent loop: build the system prompt from
SOUL.md, USER.md, MEMORY.md and the daily notes of the last three days, load
the chat session, call the model with the tool list, run the tools it asks
for, loop until a final answer, save the session and send the reply, with
the skill summary of `skills/` in the system prompt. Five tools are ported:
`get_current_time`, `read_file`, `write_file`, `edit_file` and `list_dir`.
The other seven stay registered, so the tool list sent to the model is the
original one. `web_search`, the cron tools and the GPIO tools return
`Error: tool '<name>' is not available in the host build` in both the port
and the harness.

`get_current_time` in mimiclaw has no local clock branch. It sends a HEAD
request to `https://api.telegram.org/` and formats its Date header. Before it
formats, it sets `TZ` to `PST8PDT,M3.2.0,M11.1.0`, and that stays set, so
every later daily note date is a Pacific date. The port does the same: the
HEAD request goes through a `net-head` wrapper, `gLocalTimezone` starts as UTC
and switches to Pacific, and local time is computed for those two zones.

Left out: Wi-Fi, Telegram, Feishu, WebSocket, GPIO, OTA, the serial CLI, the
OpenAI provider path, the HTTP CONNECT proxy (and its chunked decoder), NVS,
the FreeRTOS task setup, the cron_add input patch (it only changes what
cron_add receives), `settimeofday` (the clock wrapper is the clock), and
the fixed C buffer sizes.

SPIFFS has no directories, only file names with slashes in them, and
`list_dir` and the skill summary read it with `opendir("/spiffs")`. The
harness shims `opendir` and `readdir` to list every file below `spiffs/` by
its relative path in byte order, and the port's `dir-list` wrapper lists the
same way, both through `backendstore listdir`. A device lists in SPIFFS
order, which is not covered.

The API key is not part of the request body. The harness sets one in
`before/shim/mimi_secrets.h` because `llm_chat_tools` refuses to run without
it.

## Tape

The records are those of the femtoclaw port, `file-write` and `dir-list`
among them, plus one:

```
net-head	https://api.telegram.org/	ok	Mon, 23 Nov 2026 10:44:24 GMT
```

The fields are the URL, `ok` or `error`, and the Date header. In live and
record mode the port gets them from `backendstore head`. Model requests keep
the URL the agent asked for, `https://api.anthropic.com/v1/messages`, on the
tape; the wrapper and the harness send them to the `++backend` server with the
same path.

## File mapping

| Original file : function | Port file |
|---|---|
| mimi_config.h, bus/message_bus.h channel names | 000-configuration-values.cx |
| (new) text building without sprintf | 001-join-strings.cx |
| (new) wrappers: mode, state, byte table, tape | 002 to 020 |
| (new) file wrapper | 021-file-read.cx, 022-file-append.cx, 023-file-write.cx |
| (new) directory wrapper, the port of opendir and readdir on SPIFFS | 024-dir-list.cx |
| (new) clock wrapper, the port of `time(NULL)` | 025-clock-now.cx |
| (new) network wrapper for POST | 026-url-path.cx to 028-net-post.cx |
| (new) network wrapper for HEAD | 029-live-net-head.cx, 030-net-head.cx |
| bus/message_bus.h : mimi_msg_t | 031-inbound-message.cx |
| bus/message_bus.c : message_bus_pop_inbound (wrapper) | 032-inbound-next.cx |
| cJSON : print_string, parse, print unformatted, GetObjectItem | 033 to 045 |
| libc : localtime_r, mktime, strftime for UTC0 and PST8PDT | 046 to 052 |
| memory/memory_store.c : get_date_str | 053-memory-date-string.cx |
| memory/memory_store.c : memory_read_long_term | 054-memory-read-long-term.cx |
| memory/memory_store.c : memory_read_recent | 055-memory-read-recent.cx |
| (new) fgets into a fixed buffer | 056-skill-read-line.cx |
| skills/skill_loader.c : extract_title | 057-skill-extract-title.cx |
| skills/skill_loader.c : extract_description | 058-skill-extract-description.cx |
| skills/skill_loader.c : skill_loader_build_summary | 059-skill-build-summary.cx |
| agent/context_builder.c : append_file | 060-context-append-file.cx |
| agent/context_builder.c : context_build_system_prompt | 061-context-build-system-prompt.cx |
| memory/session_mgr.c : session_path | 062-session-path.cx |
| memory/session_mgr.c : session_append | 063-session-append.cx |
| memory/session_mgr.c : session_get_history_json | 064-session-history-messages.cx |
| tools/tool_registry.h : mimi_tool_t | 065-tool-definition.cx |
| tools/tool_registry.c : tool_registry_init | 066-tool-definitions.cx |
| tools/tool_registry.c : build_tools_json | 067-tools-json.cx |
| tools/tool_files.c : validate_path | 068-tool-validate-path.cx |
| tools/tool_files.c : tool_read_file_execute | 069-tool-read-file.cx |
| tools/tool_files.c : tool_write_file_execute | 070-tool-write-file.cx |
| tools/tool_files.c : tool_edit_file_execute | 071-tool-edit-file.cx |
| tools/tool_files.c : tool_list_dir_execute | 072-tool-list-dir.cx |
| tools/tool_get_time.c : parse_and_set_time | 073-http-date-words.cx, 074-time-parse-http-date.cx |
| tools/tool_get_time.c : tool_get_time_execute, fetch_time_direct | 075-tool-get-time.cx |
| before/harness.c : tool stubs | 076-tool-unavailable.cx |
| tools/tool_registry.c : tool_registry_execute | 077-tool-registry-execute.cx |
| llm/llm_proxy.h : llm_tool_call_t | 078-tool-call.cx |
| llm/llm_proxy.h : llm_response_t | 079-llm-reply.cx |
| llm/llm_proxy.c : llm_chat_tools, request body | 080-llm-request-body.cx |
| llm/llm_proxy.c : llm_chat_tools, response parsing | 081-llm-parse-reply.cx |
| llm/llm_proxy.c : llm_chat_tools | 082-llm-chat-tools.cx |
| agent/agent_loop.c : build_assistant_content | 083-agent-assistant-content.cx |
| agent/agent_loop.c : append_turn_context_prompt | 084-agent-turn-context.cx |
| agent/agent_loop.c : build_tool_results | 085-agent-tool-results.cx |
| before/harness.c : message_bus_push_outbound | 086-message-bus-push-outbound.cx |
| agent/agent_loop.c : agent_loop_task, one message | 087-agent-process-message.cx |
| agent/agent_loop.c : agent_loop_task, the loop | 088-agent-loop-run.cx |
| main/mimi.c : app_main, reduced to start and stop | 089-main.cx |

## Running the port

```
cx port/*.cx ++mode=replay ++tape=tape.txt
cx port/*.cx ++mode=record ++tape=tape.txt ++inbound=inbound.txt ++client=backendstore ++backend=http://127.0.0.1:18080
```

Run it in the directory that holds `spiffs/`. `inbound.txt` has the format of
the femtoclaw port. Recording needs network access for the HEAD request;
when it fails the tape records `error` and the tool reports
`Error: failed to fetch time (ESP_FAIL)`, as the original does.
