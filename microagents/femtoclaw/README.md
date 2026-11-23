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
- `tests/` has one folder per test. The `tool-`, `pair-` and `chain-` folders
  are generated from `tests.table`, see the main README.
- `record.sh TEST_DIR` records a test again.

## What is ported

The Anthropic provider path of the agent loop: build the system prompt from
SOUL.md, USER.md, MEMORY.md and the daily notes of the last three days, load
the chat session, call the model with the tool list, run the tools it asks
for, loop until a final answer, save the session and send the reply, with
the skill summary of `skills/` in the system prompt. Six tools are ported:
`get_current_time`, `read_file`, `write_file`, `edit_file`, `list_dir` and
`send_message`. The other five (`web_search`, the three cron tools and
`gpio_control`) stay registered, so the tool list sent to the model is the
original one, and return
`Error: tool '<name>' is not available in the host build` in both the port
and the harness.

SPIFFS has no directories, only file names with slashes in them, and
`list_dir` and the skill summary read it with `opendir("/spiffs")`. The
harness shims `opendir` and `readdir` to list every file below `spiffs/` by
its relative path in byte order, and the port's `dir-list` wrapper lists the
same way, both through `backendstore listdir`. A device lists in SPIFFS
order, which is not covered.

Left out: Wi-Fi, Telegram, WebSocket, GPIO, display and button drivers, the
OpenAI provider path, the HTTP CONNECT proxy, NVS, payload logging, the
FreeRTOS task setup, the cron_add input patch (it only changes what cron_add
receives), the HTTP Date fallback of `get_current_time`, and the fixed C
buffer sizes, among them the 8 KB tool output and the 2 KB skill summary.
Time is formatted in UTC, the harness runs with `TZ=UTC0`.

## File mapping

| Original file : function | Port file |
|---|---|
| femto_config.h, bus/message_bus.h channel names | 000-configuration-values.cx |
| (new) text building without sprintf | 001-join-strings.cx |
| (new) wrappers: mode, state, byte table, tape | 002 to 020 |
| (new) file wrapper | 021-file-read.cx, 022-file-append.cx, 023-file-write.cx |
| (new) directory wrapper, the port of opendir and readdir on SPIFFS | 024-dir-list.cx |
| (new) clock wrapper, the port of `time(NULL)` | 025-clock-now.cx |
| (new) network wrapper | 026-live-net-post.cx, 027-net-post.cx |
| bus/message_bus.h : femto_msg_t | 028-inbound-message.cx |
| bus/message_bus.c : message_bus_pop_inbound (wrapper) | 029-inbound-next.cx |
| cJSON : print_string, parse, print unformatted, GetObjectItem | 030 to 042 |
| memory/memory_store.c : localtime_r, strftime | 043 to 046 |
| memory/memory_store.c : get_date_str | 047-memory-date-string.cx |
| memory/memory_store.c : memory_read_long_term | 048-memory-read-long-term.cx |
| memory/memory_store.c : memory_read_recent | 049-memory-read-recent.cx |
| (new) fgets into a fixed buffer | 050-skill-read-line.cx |
| skills/skill_loader.c : extract_title | 051-skill-extract-title.cx |
| skills/skill_loader.c : extract_description | 052-skill-extract-description.cx |
| skills/skill_loader.c : skill_loader_build_summary | 053-skill-build-summary.cx |
| agent/context_builder.c : append_file | 054-context-append-file.cx |
| agent/context_builder.c : context_build_system_prompt | 055-context-build-system-prompt.cx |
| memory/session_mgr.c : session_path | 056-session-path.cx |
| memory/session_mgr.c : session_append | 057-session-append.cx |
| memory/session_mgr.c : session_get_history_json | 058-session-history-messages.cx |
| tools/tool_registry.h : femto_tool_t | 059-tool-definition.cx |
| tools/tool_registry.c : tool_registry_init | 060-tool-definitions.cx |
| tools/tool_registry.c : build_tools_json | 061-tools-json.cx |
| before/harness.c : message_bus_push_outbound | 062-message-bus-push-outbound.cx |
| tools/tool_files.c : validate_path | 063-tool-validate-path.cx |
| tools/tool_files.c : tool_read_file_execute | 064-tool-read-file.cx |
| tools/tool_files.c : tool_write_file_execute | 065-tool-write-file.cx |
| tools/tool_files.c : tool_edit_file_execute | 066-tool-edit-file.cx |
| tools/tool_files.c : tool_list_dir_execute | 067-tool-list-dir.cx |
| tools/tool_send_message.c : tool_send_message_execute | 068-tool-send-message.cx |
| tools/tool_get_time.c : tool_get_time_execute | 069-tool-get-time.cx |
| before/harness.c : tool stubs | 070-tool-unavailable.cx |
| tools/tool_registry.c : tool_registry_execute | 071-tool-registry-execute.cx |
| llm/llm_proxy.h : llm_tool_call_t | 072-tool-call.cx |
| llm/llm_proxy.h : llm_response_t | 073-llm-reply.cx |
| llm/llm_proxy.c : llm_api_url | 074-llm-api-url.cx |
| llm/llm_proxy.c : llm_chat_tools, request body | 075-llm-request-body.cx |
| llm/llm_proxy.c : llm_chat_tools, response parsing | 076-llm-parse-reply.cx |
| llm/llm_proxy.c : llm_chat_tools | 077-llm-chat-tools.cx |
| agent/agent_loop.c : build_assistant_content | 078-agent-assistant-content.cx |
| agent/agent_loop.c : append_turn_context_prompt | 079-agent-turn-context.cx |
| agent/agent_loop.c : build_tool_results | 080-agent-tool-results.cx |
| agent/agent_loop.c : agent_loop_task, one message | 081-agent-process-message.cx |
| agent/agent_loop.c : agent_loop_task, the loop | 082-agent-loop-run.cx |
| main/femto.c : app_main, reduced to start and stop | 083-main.cx |

## Running the port

```
cx port/*.cx ++mode=replay ++tape=tape.txt
cx port/*.cx ++mode=record ++tape=tape.txt ++inbound=inbound.txt ++client=backendstore ++backend=http://127.0.0.1:18080
```

Run it in the directory that holds `spiffs/`, since CX confines file access
to the working directory. `inbound.txt` has one message per line as channel,
chat id and content separated by tabs, with `\n`, `\t` and `\\` escapes.
`live` mode is `record` without the tape.
