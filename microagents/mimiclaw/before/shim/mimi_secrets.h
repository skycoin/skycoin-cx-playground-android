/* Build-time secrets of the host harness. The key only has to be set, since
 * llm_chat_tools refuses to run without one; it is not part of the request
 * body, so neither the store nor the tape sees it. */
#pragma once
#define MIMI_SECRET_API_KEY "host-harness-key"
