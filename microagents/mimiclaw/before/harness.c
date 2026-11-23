/* Host harness for the unmodified mimiclaw core ("before").
 *
 *   mimiclaw-before TAPE BACKEND_URL CLIENT_COMMAND
 *
 * Inbound messages and clock reads come from the tape the CX port recorded.
 * Files are real, under spiffs/ in the working directory. Model requests go
 * to BACKEND_URL through CLIENT_COMMAND (backendstore post), with the path of
 * the URL the core asked for, so a replay store answers only byte identical
 * requests. The HEAD request of get_current_time is answered from the
 * net-head records of the tape. Outbound messages are printed in the format
 * the port prints. */

#include "../original/main/agent/agent_loop.c"

#include "bus/message_bus.h"
#include "llm/llm_proxy.h"
#include "proxy/http_proxy.h"
#include "tools/tool_cron.h"
#include "tools/tool_gpio.h"
#include "tools/tool_registry.h"
#include "tools/tool_web_search.h"

#undef fopen
#undef time
#undef opendir
#undef readdir
#undef closedir

#include <stdarg.h>

typedef struct {
    char **fields;
    int field_count;
} tape_record_t;

static tape_record_t *s_records;
static int s_record_count;
static int s_next_inbound;
static int s_next_clock;
static int s_next_head;
static const char *s_backend_url;
static const char *s_client_command;

static void harness_fail(const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    printf("FATAL ");
    vprintf(fmt, args);
    printf("\n");
    va_end(args);
    fflush(stdout);
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

FILE *host_fopen(const char *path, const char *mode)
{
    if (strncmp(path, "/spiffs/", 8) == 0) path++;
    return fopen(path, mode);
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

typedef struct {
    char **names;
    int count;
    int next;
    struct dirent entry;
} host_dir_t;

DIR *host_opendir(const char *path)
{
    if (strcmp(path, "/spiffs") != 0) return NULL;
    char command[1024];
    snprintf(command, sizeof(command), "%s listdir -root spiffs", s_client_command);
    FILE *p = popen(command, "r");
    if (!p) return NULL;
    host_dir_t *d = calloc(1, sizeof(*d));
    char line[1024];
    while (fgets(line, sizeof(line), p)) {
        line[strcspn(line, "\n")] = '\0';
        d->names = realloc(d->names, (d->count + 1) * sizeof(char *));
        d->names[d->count++] = strdup(line);
    }
    if (pclose(p) != 0) {
        host_closedir((DIR *)d);
        return NULL;
    }
    return (DIR *)d;
}

struct dirent *host_readdir(DIR *dir)
{
    host_dir_t *d = (host_dir_t *)dir;
    if (d->next >= d->count) return NULL;
    memset(&d->entry, 0, sizeof(d->entry));
    strncpy(d->entry.d_name, d->names[d->next++], sizeof(d->entry.d_name) - 1);
    return &d->entry;
}

int host_closedir(DIR *dir)
{
    host_dir_t *d = (host_dir_t *)dir;
    for (int i = 0; i < d->count; i++) free(d->names[i]);
    free(d->names);
    free(d);
    return 0;
}

esp_err_t message_bus_pop_inbound(mimi_msg_t *msg, uint32_t timeout_ms)
{
    (void)timeout_ms;
    int i = tape_find("inbound", s_next_inbound);
    if (i < 0) {
        if (tape_find("clock", s_next_clock) >= 0) {
            harness_fail("the original left clock records of the tape unread");
        }
        fflush(stdout);
        exit(0);
    }
    s_next_inbound = i + 1;
    memset(msg, 0, sizeof(*msg));
    strncpy(msg->channel, s_records[i].fields[1], sizeof(msg->channel) - 1);
    strncpy(msg->chat_id, s_records[i].fields[2], sizeof(msg->chat_id) - 1);
    msg->content = strdup(s_records[i].fields[3]);
    return ESP_OK;
}

esp_err_t message_bus_push_outbound(const mimi_msg_t *msg)
{
    printf("--- outbound channel=%s chat_id=%s bytes=%zu\n%s\n",
           msg->channel, msg->chat_id, strlen(msg->content), msg->content);
    free(msg->content);
    return ESP_OK;
}

/* ESP-IDF and FreeRTOS stand-ins. */

const char *esp_err_to_name(esp_err_t err)
{
    switch (err) {
    case ESP_OK: return "ESP_OK";
    case ESP_FAIL: return "ESP_FAIL";
    case ESP_ERR_NO_MEM: return "ESP_ERR_NO_MEM";
    case ESP_ERR_INVALID_ARG: return "ESP_ERR_INVALID_ARG";
    case ESP_ERR_INVALID_STATE: return "ESP_ERR_INVALID_STATE";
    case ESP_ERR_INVALID_SIZE: return "ESP_ERR_INVALID_SIZE";
    case ESP_ERR_NOT_FOUND: return "ESP_ERR_NOT_FOUND";
    case ESP_ERR_HTTP_CONNECT: return "ESP_ERR_HTTP_CONNECT";
    case ESP_ERR_HTTP_WRITE_DATA: return "ESP_ERR_HTTP_WRITE_DATA";
    default: return "UNKNOWN ERROR";
    }
}

int host_settimeofday(const struct timeval *tv, const void *tz) { (void)tv; (void)tz; return 0; }

int xPortGetCoreID(void) { return 1; }
void vTaskDelete(void *task) { (void)task; exit(1); }
BaseType_t xTaskCreatePinnedToCore(void (*fn)(void *), const char *name, uint32_t stack,
                                   void *arg, int prio, void *handle, int core)
{
    (void)fn; (void)name; (void)stack; (void)arg; (void)prio; (void)handle; (void)core;
    return pdPASS;
}

esp_err_t nvs_open(const char *ns, int mode, nvs_handle_t *handle) { (void)ns; (void)mode; *handle = 1; return ESP_OK; }
esp_err_t nvs_set_str(nvs_handle_t handle, const char *key, const char *value) { (void)handle; (void)key; (void)value; return ESP_OK; }
esp_err_t nvs_get_str(nvs_handle_t handle, const char *key, char *out, size_t *len) { (void)handle; (void)key; (void)out; (void)len; return ESP_ERR_NOT_FOUND; }
esp_err_t nvs_erase_key(nvs_handle_t handle, const char *key) { (void)handle; (void)key; return ESP_OK; }
esp_err_t nvs_commit(nvs_handle_t handle) { (void)handle; return ESP_OK; }
void nvs_close(nvs_handle_t handle) { (void)handle; }

struct esp_http_client {
    char *url;
    http_event_handle_cb handler;
    void *user_data;
    char *post;
    int post_len;
    int status;
    esp_http_client_method_t method;
};

esp_http_client_handle_t esp_http_client_init(const esp_http_client_config_t *config)
{
    struct esp_http_client *c = calloc(1, sizeof(*c));
    c->url = strdup(config->url);
    c->handler = config->event_handler;
    c->user_data = config->user_data;
    c->method = config->method;
    return c;
}

/* url_path returns the path of url, which the harness appends to BACKEND_URL. */
static const char *url_path(const char *url)
{
    const char *p = strstr(url, "://");
    p = p ? p + 3 : url;
    const char *slash = strchr(p, '/');
    return slash ? slash : "/";
}

/* http_head_from_tape answers a HEAD request with the Date header of the
 * next net-head record: URL, then "ok" or "error", then the Date value. */
static esp_err_t http_head_from_tape(esp_http_client_handle_t c)
{
    int i = tape_find("net-head", s_next_head);
    if (i < 0) harness_fail("the original sent a HEAD request the tape does not record");
    s_next_head = i + 1;
    if (strcmp(s_records[i].fields[1], c->url) != 0) {
        harness_fail("HEAD %s, the tape has %s", c->url, s_records[i].fields[1]);
    }
    if (strcmp(s_records[i].fields[2], "ok") != 0) return ESP_FAIL;
    c->status = 200;
    if (s_records[i].fields[3][0] != '\0') {
        esp_http_client_event_t evt = {
            .event_id = HTTP_EVENT_ON_HEADER,
            .client = c,
            .user_data = c->user_data,
            .header_key = "Date",
            .header_value = s_records[i].fields[3],
        };
        if (c->handler) c->handler(&evt);
    }
    return ESP_OK;
}

esp_err_t esp_http_client_set_method(esp_http_client_handle_t c, esp_http_client_method_t m) { c->method = m; return ESP_OK; }
esp_err_t esp_http_client_set_header(esp_http_client_handle_t c, const char *k, const char *v) { (void)c; (void)k; (void)v; return ESP_OK; }

esp_err_t esp_http_client_set_post_field(esp_http_client_handle_t c, const char *data, int len)
{
    c->post = malloc(len);
    memcpy(c->post, data, len);
    c->post_len = len;
    return ESP_OK;
}

esp_err_t esp_http_client_perform(esp_http_client_handle_t c)
{
    if (c->method == HTTP_METHOD_HEAD) return http_head_from_tape(c);

    FILE *body = fopen("net-request.json", "wb");
    if (!body) return ESP_FAIL;
    fwrite(c->post, 1, c->post_len, body);
    fclose(body);

    char command[1024];
    snprintf(command, sizeof(command), "%s post -url %s%s -body net-request.json", s_client_command, s_backend_url, url_path(c->url));
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
    c->status = atoi(out);
    esp_http_client_event_t evt = {
        .event_id = HTTP_EVENT_ON_DATA,
        .client = c,
        .data = newline + 1,
        .data_len = (int)(len - (newline + 1 - out)),
        .user_data = c->user_data,
    };
    if (evt.data_len > 0) c->handler(&evt);
    free(out);
    return ESP_OK;
}

int esp_http_client_get_status_code(esp_http_client_handle_t c) { return c->status; }
esp_err_t esp_http_client_get_header(esp_http_client_handle_t c, const char *k, char **v) { (void)c; (void)k; *v = NULL; return ESP_OK; }

esp_err_t esp_http_client_cleanup(esp_http_client_handle_t c)
{
    free(c->url);
    free(c->post);
    free(c);
    return ESP_OK;
}

esp_err_t esp_crt_bundle_attach(void *conf) { (void)conf; return ESP_OK; }

/* No HTTP proxy on the host. */
esp_err_t http_proxy_init(void) { return ESP_OK; }
bool http_proxy_is_enabled(void) { return false; }
esp_err_t http_proxy_set(const char *host, uint16_t port, const char *type) { (void)host; (void)port; (void)type; return ESP_OK; }
esp_err_t http_proxy_clear(void) { return ESP_OK; }
proxy_conn_t *proxy_conn_open(const char *host, int port, int timeout_ms) { (void)host; (void)port; (void)timeout_ms; return NULL; }
int proxy_conn_write(proxy_conn_t *conn, const char *data, int len) { (void)conn; (void)data; (void)len; return -1; }
int proxy_conn_read(proxy_conn_t *conn, char *buf, int len, int timeout_ms) { (void)conn; (void)buf; (void)len; (void)timeout_ms; return -1; }
void proxy_conn_close(proxy_conn_t *conn) { (void)conn; }

/* Tools whose sources need ESP-IDF drivers or services. The CX port prints
 * the same text for them. */
static esp_err_t tool_unavailable(const char *name, char *output, size_t size)
{
    snprintf(output, size, "Error: tool '%s' is not available in the host build", name);
    return ESP_ERR_NOT_FOUND;
}

esp_err_t tool_web_search_init(void) { return ESP_OK; }
esp_err_t tool_web_search_set_key(const char *api_key) { (void)api_key; return ESP_OK; }
esp_err_t tool_web_search_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("web_search", out, size); }
esp_err_t tool_cron_add_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("cron_add", out, size); }
esp_err_t tool_cron_list_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("cron_list", out, size); }
esp_err_t tool_cron_remove_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("cron_remove", out, size); }
esp_err_t tool_gpio_init(void) { return ESP_OK; }
esp_err_t tool_gpio_write_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("gpio_write", out, size); }
esp_err_t tool_gpio_read_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("gpio_read", out, size); }
esp_err_t tool_gpio_read_all_execute(const char *in, char *out, size_t size) { (void)in; return tool_unavailable("gpio_read_all", out, size); }

int main(int argc, char **argv)
{
    if (argc != 4) {
        fprintf(stderr, "usage: %s TAPE BACKEND_URL CLIENT_COMMAND\n", argv[0]);
        return 2;
    }
    setvbuf(stdout, NULL, _IOFBF, 1 << 16);
    setenv("TZ", "UTC0", 1);
    tzset();
    tape_load(argv[1]);
    s_backend_url = argv[2];
    s_client_command = argv[3];
    llm_proxy_init();
    tool_registry_init();
    agent_loop_task(NULL);
    return 1;
}
