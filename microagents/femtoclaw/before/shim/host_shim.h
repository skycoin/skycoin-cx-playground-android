/* Host stand-ins for the ESP-IDF and FreeRTOS APIs that the femtoclaw core
 * uses, so the unmodified core files build and run on Linux. */
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>

typedef int esp_err_t;
#define ESP_OK 0
#define ESP_FAIL -1
#define ESP_ERR_NO_MEM 0x101
#define ESP_ERR_INVALID_ARG 0x102
#define ESP_ERR_INVALID_STATE 0x103
#define ESP_ERR_INVALID_SIZE 0x104
#define ESP_ERR_NOT_FOUND 0x105
#define ESP_ERR_HTTP_CONNECT 0x7003
#define ESP_ERR_HTTP_WRITE_DATA 0x7004
const char *esp_err_to_name(esp_err_t err);
#define ESP_ERROR_CHECK(x) do { esp_err_t err_rc_ = (x); (void)err_rc_; } while (0)

#define ESP_LOGE(tag, ...) do { (void)(tag); } while (0)
#define ESP_LOGW(tag, ...) do { (void)(tag); } while (0)
#define ESP_LOGI(tag, ...) do { (void)(tag); } while (0)
#define ESP_LOGD(tag, ...) do { (void)(tag); } while (0)

#define MALLOC_CAP_SPIRAM 1
#define MALLOC_CAP_INTERNAL 2
#define heap_caps_calloc(n, size, caps) calloc((n), (size))
#define heap_caps_realloc(p, size, caps) realloc((p), (size))
#define heap_caps_get_free_size(caps) ((size_t)0)
#define heap_caps_get_largest_free_block(caps) ((size_t)0)

typedef int BaseType_t;
typedef void *QueueHandle_t;
typedef void *TaskHandle_t;
#define pdPASS 1
#define portMAX_DELAY UINT32_MAX
int xPortGetCoreID(void);
void vTaskDelete(void *task);
BaseType_t xTaskCreatePinnedToCore(void (*fn)(void *), const char *name, uint32_t stack,
                                   void *arg, int prio, void *handle, int core);

typedef uint32_t nvs_handle_t;
#define NVS_READONLY 0
#define NVS_READWRITE 1
esp_err_t nvs_open(const char *ns, int mode, nvs_handle_t *handle);
esp_err_t nvs_set_str(nvs_handle_t handle, const char *key, const char *value);
esp_err_t nvs_get_str(nvs_handle_t handle, const char *key, char *out, size_t *len);
esp_err_t nvs_erase_key(nvs_handle_t handle, const char *key);
esp_err_t nvs_commit(nvs_handle_t handle);
void nvs_close(nvs_handle_t handle);

typedef enum { HTTP_METHOD_GET, HTTP_METHOD_POST, HTTP_METHOD_HEAD } esp_http_client_method_t;
typedef enum { HTTP_EVENT_ERROR, HTTP_EVENT_ON_HEADER, HTTP_EVENT_ON_DATA, HTTP_EVENT_ON_FINISH } esp_http_client_event_id_t;
typedef struct esp_http_client *esp_http_client_handle_t;
typedef struct {
    esp_http_client_event_id_t event_id;
    esp_http_client_handle_t client;
    void *data;
    int data_len;
    void *user_data;
} esp_http_client_event_t;
typedef esp_err_t (*http_event_handle_cb)(esp_http_client_event_t *evt);
typedef struct {
    const char *url;
    esp_http_client_method_t method;
    http_event_handle_cb event_handler;
    void *user_data;
    int timeout_ms;
    int buffer_size;
    int buffer_size_tx;
    esp_err_t (*crt_bundle_attach)(void *conf);
} esp_http_client_config_t;
esp_http_client_handle_t esp_http_client_init(const esp_http_client_config_t *config);
esp_err_t esp_http_client_set_method(esp_http_client_handle_t client, esp_http_client_method_t method);
esp_err_t esp_http_client_set_header(esp_http_client_handle_t client, const char *key, const char *value);
esp_err_t esp_http_client_set_post_field(esp_http_client_handle_t client, const char *data, int len);
esp_err_t esp_http_client_perform(esp_http_client_handle_t client);
int esp_http_client_get_status_code(esp_http_client_handle_t client);
esp_err_t esp_http_client_get_header(esp_http_client_handle_t client, const char *key, char **value);
esp_err_t esp_http_client_cleanup(esp_http_client_handle_t client);
esp_err_t esp_crt_bundle_attach(void *conf);

/* File and clock calls go through the harness: /spiffs/ paths map to the
 * run directory, and time() reads the clock records of the tape. */
FILE *host_fopen(const char *path, const char *mode);
time_t host_time(time_t *out);
#define fopen(path, mode) host_fopen((path), (mode))
#define time(out) host_time(out)
