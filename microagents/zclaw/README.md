# zclaw in CX

A port of the agent of [tnm/zclaw](https://github.com/tnm/zclaw) at commit
`e3ad271244c1f2e01f4df6e81c2bab346e95d1b1` to CX. zclaw is MIT licensed; its
license is in [LICENSE](LICENSE) and covers `original/` and the port, which
is a derivative work.

zclaw does not share code with femtoclaw and mimiclaw. It keeps the
conversation in RAM, stores user memory and settings in NVS, has slash
commands, cooldowns against repeated messages, a rate limiter, retries and
personas. The port shares only the wrappers, the tape and the JSON helpers
with the other two.

- `original/` holds the unmodified upstream files the harness compiles: the
  agent sources from `main/` and the host test mocks from `test/host/`.
- `before/` is the host harness. It drives `agent.c` through its
  `TEST_BUILD` hooks, as upstream `test/host/test_agent.c` does, with the
  upstream mocks for queues, memory and logging. `shim/` replaces the wall
  clock uptime of `test/host/esp_timer.h` with the uptime from the tape.
- `port/` is the CX port, one declaration per `nnn-name.cx` file.
- `tests/` has one folder per test. The `tool-`, `pair-` and `chain-` folders
  are generated from `tests.table`, see the main README.
- `record.sh TEST_DIR` records a test again.

## What is ported

`process_message` of `agent.c` with all of its branches except the local
admin commands on the serial console: `/resume`, `/settings`, `/help`,
`/stop`, `/start` with its 30 second cooldown, the 20 second suppression of
a repeated message, the rolling history of 24 messages with orphan tool
results skipped, the rate limiter, three LLM attempts per round, rollback of
the turn on errors, up to five tool rounds, the persona in the system prompt,
and the refusal of `cron_set` in a cron triggered turn. From Telegram the
local admin commands get their refusal, as in C. On the serial console they
drive the hardware, and the port stops with a fatal error.

Twelve tools are ported: `memory_set`, `memory_get`, `memory_delete`,
`set_persona`, `get_persona`, `reset_persona`, `delay`, `get_version`,
`get_health`, `create_tool`, `list_user_tools` and `delete_user_tool`. The
other 16 stay in the tool list sent to the model and answer
`Error: tool '<name>' is not available in the host build` in both the port
and the harness. Every text the C code writes into a fixed buffer is cut to
that buffer in the port as well.

The harness behaves as the upstream mocks do, and the port follows it:

- User tools are `test/host/mock_user_tools.c`: `create_tool` keeps up to 8
  tools, cut to their buffers, and they follow the built-in tools in the
  tool list of the next request. The mock finds none by name, so calling one
  answers `Unknown tool`, and `delete_user_tool` answers `not found`.
- `get_version` and `get_health` take their answers from
  `test/host/mock_system_diag_deps.c`: version `test-version`, clock synced,
  zone `UTC0`, and 0 bytes of heap in a `TEST_BUILD`. `delay` counts the
  delay in `mock_freertos.c`, and no time passes.
- Memory is `test/host/mock_memory.c`, 16 entries with keys of up to 31 bytes
  and values of up to 511 bytes, empty at start.
- `llm.c` is replaced by an `llm_request` that posts the request and keeps
  the status check and the response buffer limit of the real one; backend
  anthropic, model `claude-sonnet-4-6`.
- The uptime of `esp_timer_get_time` is the arrival time of the message and
  does not move while it is processed. The retry delays (`vTaskDelay`) and
  the retry budget therefore take no time.
- Device target is `esp32-family`, as `CONFIG_IDF_TARGET` is not defined in
  a host build, and the rate limiter works in UTC.
- `tools_validate_string_input` compares a plain `char` with 0x20. `char` is
  signed on the x86 host, so a memory value with UTF-8 in it is refused as
  an invalid character. The port does the same. On an ESP32 the result
  depends on the signedness of `char` for that target, which is not tested.

Left out: Wi-Fi, Telegram polling, the serial channel task, cron and the
time tools that live beside it, GPIO, I2C, DHT, OTA, `get_diagnostics`,
`memory_list` (it needs the NVS iterator, which the mocks do not have), the
OpenAI, OpenRouter and Ollama formats, the HTTP and TLS diagnostics of
`llm.c`, metrics logging, and Telegram poll pausing. The harness stubs the
left out tools, or renames their handlers away when it builds an upstream
source for the other tools in it. Numbers in tool inputs are copied into
the history as written, which matches cJSON for integers and plain
decimals. A number cJSON prints in another form, such as `1.5e3`, which it
prints as `1500`, makes the next request differ from the original's, so the
tests leave it out. `delay` reads its number as cJSON's `valueint` does.

## Tape

```
inbound	telegram	777	3000000	Hello zclaw
clock	1795431355
net-request	https://api.anthropic.com/v1/messages	{"model":"claude-sonnet-4-6",...}
net-reply	200	{"id":"msg_stub_...",...}
inbound-end
```

An inbound record holds the source (`telegram` or `channel`), the chat id,
the uptime in microseconds and the text. The clock records are the
`time()` reads of the rate limiter, two for each successful round. In live
and record mode `inbound.txt` has one message per line: source, chat id,
uptime in milliseconds and text.

## File mapping

| Original file : function | Port file |
|---|---|
| config.h, nvs_keys.h, memory_keys.h, mock_memory.c limits | 000-configuration-values.cx |
| (new) text building without sprintf | 001-join-strings.cx |
| (new) wrappers: mode, state, byte table, tape | 002 to 020 |
| messages.h : channel_msg_t, with the uptime | 013-inbound-message.cx |
| (new) clock wrapper, the port of `time()` | 021-clock-now.cx |
| (new) network wrapper | 022-url-path.cx to 024-net-post.cx |
| agent.c : agent_task, xQueueReceive (wrapper) | 025-inbound-next.cx |
| cJSON : print_string, parse, print unformatted, GetObjectItem | 026 to 038 |
| cJSON : parse_number, valueint | 039-json-number-int.cx |
| libc : localtime_r in UTC | 040-civil-date.cx, 041-days-from-civil.cx |
| (new) strncpy and snprintf into a fixed buffer | 042-truncate-bytes.cx |
| json_util.h : conversation_msg_t | 043-conversation-message.cx |
| agent.c, ratelimit.c static state, mock_memory.c and mock_user_tools.c slots | 044-agent-state.cx, 045-agent-state-global.cx |
| test/host/mock_memory.c : find_slot_index, memory_get, memory_set, memory_delete | 046 to 049 |
| agent_commands.c : is_whitespace_char | 050-agent-is-whitespace.cx, 051-agent-skip-whitespace.cx |
| agent_commands.c : agent_is_command | 052-agent-is-command.cx |
| agent_commands.c : agent_is_slash_command | 053-agent-is-slash-command.cx |
| agent_commands.c : agent_is_cron_trigger_message | 054-agent-is-cron-trigger.cx |
| agent.c : is_local_admin_command, local_admin.c : local_admin_is_command | 055-local-admin-is-command.cx |
| agent_prompt.c : persona_instruction | 056-agent-persona-instruction.cx |
| agent_prompt.c : agent_parse_persona_name | 057-agent-parse-persona-name.cx |
| agent_prompt.c : agent_build_system_prompt, build_gpio_policy_summary | 058-agent-build-system-prompt.cx |
| ratelimit.c : update_time_window | 059-ratelimit-update-time-window.cx |
| ratelimit.c : ratelimit_check | 060-ratelimit-check.cx |
| ratelimit.c : ratelimit_record_request | 061-ratelimit-record-request.cx |
| agent.c : history_add | 062-history-add.cx |
| agent.c : history_rollback_to | 063-history-rollback.cx |
| json_util.c : history_has_prior_tool_use | 064-history-has-prior-tool-use.cx |
| tools.h : tool_def_t | 065-tool-definition.cx |
| builtin_tools.def | 066-tool-definitions.cx |
| json_util.c : build_anthropic_request | 067-json-build-request.cx |
| json_util.c : json_parse_response outputs | 068-parsed-response.cx |
| json_util.c : json_parse_response, parse_anthropic_response | 069-json-parse-response.cx |
| llm.c : llm_request, after the HTTP exchange | 070-llm-request.cx |
| tools_common.c : tools_validate_string_input | 071-tools-validate-string-input.cx |
| tools_common.c : tools_validate_nvs_key, tools_validate_user_memory_key | 072-tools-validate-nvs-key.cx |
| memory_keys.c : memory_keys_is_sensitive | 073-memory-keys-is-sensitive.cx |
| tools_memory.c : the key checks of each handler | 074-tool-key-check.cx |
| tools_memory.c : tools_memory_set_handler | 075-tool-memory-set.cx |
| tools_memory.c : tools_memory_get_handler | 076-tool-memory-get.cx |
| tools_memory.c : tools_memory_delete_handler | 077-tool-memory-delete.cx |
| tools_persona.c : canonicalize_persona_name | 078-canonicalize-persona-name.cx |
| tools_persona.c : tools_set_persona_handler | 079-tool-set-persona.cx |
| tools_persona.c : tools_get_persona_handler, load_current_persona | 080-tool-get-persona.cx |
| tools_persona.c : tools_reset_persona_handler | 081-tool-reset-persona.cx |
| tools_gpio.c : tools_delay_handler | 082-tool-delay.cx |
| tools_system.c : tools_get_version_handler | 083-tool-get-version.cx |
| tools_system.c : tools_get_health_handler | 084-tool-get-health.cx |
| tools_system.c : tools_create_tool_handler, mock_user_tools.c : user_tools_create | 085-tool-create-tool.cx |
| tools_system.c : tools_list_user_tools_handler, mock_user_tools.c : user_tools_list | 086-tool-list-user-tools.cx |
| tools_system.c : tools_delete_user_tool_handler, mock_user_tools.c : user_tools_delete | 087-tool-delete-user-tool.cx |
| before/harness.c : tool stubs | 088-tool-unavailable.cx |
| tools.c : tools_execute | 089-tools-execute.cx |
| agent.c : send_response, with the harness queues | 090-send-response.cx |
| agent.c : handle_start_command | 091-handle-start-command.cx |
| agent.c : handle_settings_command | 092-handle-settings-command.cx |
| agent.c : process_message, the tool step | 093-agent-tool-result.cx |
| agent.c : process_message, the round loop | 094-agent-run-turn.cx |
| agent.c : process_message, commands and cooldowns | 095-process-message.cx |
| agent.c : agent_task, agent_test_reset | 096-agent-loop-run.cx |
| main.c : app_main, reduced to start and stop | 097-main.cx |

## Running the port

```
cx port/*.cx ++mode=replay ++tape=tape.txt
cx port/*.cx ++mode=record ++tape=tape.txt ++inbound=inbound.txt ++client=backendstore ++backend=http://127.0.0.1:18080
```
