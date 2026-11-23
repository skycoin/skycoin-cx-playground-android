/* tools_memory.c includes nvs.h for memory_list, which the harness does not
 * build (see run-tests.sh). This header only has to make it compile. */
#pragma once

#include "mock_esp.h"

typedef uint32_t nvs_handle_t;
typedef void *nvs_iterator_t;
typedef enum { NVS_READONLY, NVS_READWRITE } nvs_open_mode_t;
typedef enum { NVS_TYPE_STR = 0x21 } nvs_type_t;
typedef struct { char key[16]; } nvs_entry_info_t;

esp_err_t nvs_open(const char *ns, nvs_open_mode_t mode, nvs_handle_t *handle);
void nvs_close(nvs_handle_t handle);
esp_err_t nvs_entry_find_in_handle(nvs_handle_t handle, nvs_type_t type, nvs_iterator_t *it);
esp_err_t nvs_entry_info(nvs_iterator_t it, nvs_entry_info_t *info);
esp_err_t nvs_entry_next(nvs_iterator_t *it);
void nvs_release_iterator(nvs_iterator_t it);
