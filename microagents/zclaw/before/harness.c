/* Host harness for the unmodified zclaw agent ("before").
 *
 *   zclaw-before TAPE BACKEND_URL CLIENT_COMMAND
 *
 * It builds on the upstream host test mocks in original/test/host (queues,
 * memory, logging) and drives agent.c through its TEST_BUILD hooks, as
 * test_agent.c does. Inbound messages, their uptime and the clock reads of
 * the rate limiter come from the tape the CX port recorded. llm_request
 * posts to BACKEND_URL through CLIENT_COMMAND (backendstore post), so a
 * replay store answers only byte identical requests. Responses are printed
 * in the format the port prints; the mock logs go to stderr. */

#define _GNU_SOURCE
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "agent.h"
#include "config.h"
#include "llm.h"
#include "messages.h"
#include "nvs.h"
#include "tools_handlers.h"
#include "freertos/queue.h"

#undef time

typedef struct {
    char **fields;
    int field_count;
} tape_record_t;

static tape_record_t *s_records;
static int s_record_count;
static int s_next_inbound;
static int s_next_clock;
static const char *s_backend_url;
static const char *s_client_command;
static int64_t s_uptime_us;
static FILE *s_out;

static void harness_fail(const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    fprintf(s_out, "FATAL ");
    vfprintf(s_out, fmt, args);
    fprintf(s_out, "\n");
    va_end(args);
    fflush(s_out);
    exit(3);
}

static char *tape_unescape(const char *s, size_t len)
{
    char *out = malloc(len + 1);
    size_t n = 0;
    for (size_t i = 0; i < len; i++) {
        if (s[i] == '\\' && i + 1 < len) {
            i++;
            out[n++] = s[i] == 'n' ? '\n' : s[i] == 't' ? '\t' : s[i] == 'r' ? '\r' : s[i];
        } else {
            out[n++] = s[i];
        }
    }
    out[n] = '\0';
    return out;
}

static void tape_load(const char *path)
{
    FILE *f = fopen(path, "rb");
    if (!f) harness_fail("cannot read tape %s", path);
    fseek(f, 0, SEEK_END);
    long size = ftell(f);
    fseek(f, 0, SEEK_SET);
    char *text = malloc(size + 1);
    if (fread(text, 1, size, f) != (size_t)size) harness_fail("short read of %s", path);
    text[size] = '\0';
    fclose(f);

    char *line = text;
    while (*line) {
        char *end = strchr(line, '\n');
        if (!end) end = line + strlen(line);
        if (end > line) {
            s_records = realloc(s_records, (s_record_count + 1) * sizeof(*s_records));
            tape_record_t *rec = &s_records[s_record_count++];
            rec->fields = NULL;
            rec->field_count = 0;
            char *field = line;
            for (;;) {
                char *tab = memchr(field, '\t', end - field);
                char *field_end = tab ? tab : end;
                rec->fields = realloc(rec->fields, (rec->field_count + 1) * sizeof(char *));
                rec->fields[rec->field_count++] = tape_unescape(field, field_end - field);
                if (!tab) break;
                field = tab + 1;
            }
        }
        line = *end ? end + 1 : end;
    }
    free(text);
}

static int tape_find(const char *kind, int from)
{
    for (int i = from; i < s_record_count; i++) {
        if (strcmp(s_records[i].fields[0], kind) == 0) return i;
    }
    return -1;
}

time_t host_time(time_t *out)
{
    int i = tape_find("clock", s_next_clock);
    if (i < 0) harness_fail("the original read the clock more often than the tape records");
    s_next_clock = i + 1;
    time_t t = (time_t)strtoll(s_records[i].fields[1], NULL, 10);
    if (out) *out = t;
    return t;
}

/* The uptime does not move while a message is processed. */
int64_t esp_timer_get_time(void)
{
    return s_uptime_us;
}

/* llm.c without its HTTP and diagnostics code: the anthropic backend with
 * its default model, a status check and the response buffer limit. */
esp_err_t llm_init(void) { return ESP_OK; }
bool llm_is_stub_mode(void) { return false; }
llm_backend_t llm_get_backend(void) { return LLM_BACKEND_ANTHROPIC; }
const char *llm_get_api_url(void) { return LLM_API_URL_ANTHROPIC; }
const char *llm_get_default_model(void) { return LLM_DEFAULT_MODEL_ANTHROPIC; }
const char *llm_get_model(void) { return LLM_DEFAULT_MODEL_ANTHROPIC; }
bool llm_is_openai_format(void) { return false; }

esp_err_t llm_request(const char *request_json, char *response_buf, size_t response_buf_size)
{
    response_buf[0] = '\0';
    FILE *body = fopen("net-request.json", "wb");
    if (!body) return ESP_FAIL;
    fwrite(request_json, 1, strlen(request_json), body);
    fclose(body);

    const char *path = strstr(llm_get_api_url(), "://");
    path = strchr(path + 3, '/');
    char command[1024];
    snprintf(command, sizeof(command), "%s post -url %s%s -body net-request.json",
             s_client_command, s_backend_url, path);
    FILE *p = popen(command, "r");
    if (!p) return ESP_FAIL;
    char *out = NULL;
    size_t len = 0;
    char chunk[4096];
    size_t n;
    while ((n = fread(chunk, 1, sizeof(chunk), p)) > 0) {
        out = realloc(out, len + n + 1);
        memcpy(out + len, chunk, n);
        len += n;
    }
    if (pclose(p) != 0 || !out) {
        free(out);
        return ESP_FAIL;
    }
    out[len] = '\0';
    char *newline = strchr(out, '\n');
    if (!newline) {
        free(out);
        return ESP_FAIL;
    }
    int status = atoi(out);
    size_t body_len = len - (size_t)(newline + 1 - out);
    bool truncated = body_len > response_buf_size - 1;
    size_t copy = truncated ? response_buf_size - 1 : body_len;
    memcpy(response_buf, newline + 1, copy);
    response_buf[copy] = '\0';
    free(out);
    if (status != 200) return ESP_FAIL;
    if (truncated) return ESP_ERR_NO_MEM;
    return ESP_OK;
}

void telegram_pause_polling(void) {}
void telegram_resume_polling(void) {}

/* Tools whose sources need drivers, NTP, OTA or the NVS iterator. The CX
 * port prints the same text for them. */
static bool tool_unavailable(const char *name, char *result, size_t result_len)
{
    snprintf(result, result_len, "Error: tool '%s' is not available in the host build", name);
    return false;
}

#define UNAVAILABLE(handler, name) \
    bool handler(const cJSON *input, char *result, size_t result_len) \
    { (void)input; return tool_unavailable(name, result, result_len); }

UNAVAILABLE(tools_gpio_write_handler, "gpio_write")
UNAVAILABLE(tools_gpio_read_handler, "gpio_read")
UNAVAILABLE(tools_gpio_read_all_handler, "gpio_read_all")
UNAVAILABLE(tools_i2c_scan_handler, "i2c_scan")
UNAVAILABLE(tools_i2c_write_handler, "i2c_write")
UNAVAILABLE(tools_i2c_read_handler, "i2c_read")
UNAVAILABLE(tools_i2c_write_read_handler, "i2c_write_read")
UNAVAILABLE(tools_dht_read_handler, "dht_read")
UNAVAILABLE(tools_memory_list_handler, "memory_list")
UNAVAILABLE(tools_cron_set_handler, "cron_set")
UNAVAILABLE(tools_cron_list_handler, "cron_list")
UNAVAILABLE(tools_cron_delete_handler, "cron_delete")
UNAVAILABLE(tools_get_time_handler, "get_time")
UNAVAILABLE(tools_set_timezone_handler, "set_timezone")
UNAVAILABLE(tools_get_timezone_handler, "get_timezone")
UNAVAILABLE(tools_get_diagnostics_handler, "get_diagnostics")

/* The NVS calls of the renamed memory_list handler, which never runs. */
esp_err_t nvs_open(const char *ns, nvs_open_mode_t mode, nvs_handle_t *handle) { (void)ns; (void)mode; (void)handle; return ESP_FAIL; }
void nvs_close(nvs_handle_t handle) { (void)handle; }
esp_err_t nvs_entry_find_in_handle(nvs_handle_t handle, nvs_type_t type, nvs_iterator_t *it) { (void)handle; (void)type; (void)it; return ESP_FAIL; }
esp_err_t nvs_entry_info(nvs_iterator_t it, nvs_entry_info_t *info) { (void)it; (void)info; return ESP_FAIL; }
esp_err_t nvs_entry_next(nvs_iterator_t *it) { (void)it; return ESP_FAIL; }
void nvs_release_iterator(nvs_iterator_t it) { (void)it; }

static void print_responses(QueueHandle_t channel_queue, QueueHandle_t telegram_queue)
{
    channel_output_msg_t channel_msg;
    while (xQueueReceive(channel_queue, &channel_msg, 0) == pdTRUE) {
        fprintf(s_out, "--- channel bytes=%zu\n%s\n", strlen(channel_msg.text), channel_msg.text);
    }
    static telegram_msg_t telegram_msg;
    while (xQueueReceive(telegram_queue, &telegram_msg, 0) == pdTRUE) {
        fprintf(s_out, "--- telegram chat_id=%lld bytes=%zu\n%s\n", (long long)telegram_msg.chat_id,
                strlen(telegram_msg.text), telegram_msg.text);
    }
}

int main(int argc, char **argv)
{
    if (argc != 4) {
        fprintf(stderr, "usage: %s TAPE BACKEND_URL CLIENT_COMMAND\n", argv[0]);
        return 2;
    }
    s_out = fdopen(dup(STDOUT_FILENO), "w");
    dup2(STDERR_FILENO, STDOUT_FILENO);
    setenv("TZ", "UTC0", 1);
    tzset();
    tape_load(argv[1]);
    s_backend_url = argv[2];
    s_client_command = argv[3];

    QueueHandle_t channel_queue = xQueueCreate(OUTPUT_QUEUE_LENGTH, sizeof(channel_output_msg_t));
    QueueHandle_t telegram_queue = xQueueCreate(TELEGRAM_OUTPUT_QUEUE_LENGTH, sizeof(telegram_msg_t));
    agent_test_reset();
    agent_test_set_queues(channel_queue, telegram_queue);

    for (;;) {
        int i = tape_find("inbound", s_next_inbound);
        if (i < 0) break;
        s_next_inbound = i + 1;
        tape_record_t *rec = &s_records[i];
        s_uptime_us = strtoll(rec->fields[3], NULL, 10);
        if (strcmp(rec->fields[1], "telegram") == 0) {
            agent_test_process_message_for_chat(rec->fields[4], strtoll(rec->fields[2], NULL, 10));
        } else {
            agent_test_process_message(rec->fields[4]);
        }
        print_responses(channel_queue, telegram_queue);
    }
    if (tape_find("clock", s_next_clock) >= 0) {
        harness_fail("the original left clock records of the tape unread");
    }
    fflush(s_out);
    return 0;
}
