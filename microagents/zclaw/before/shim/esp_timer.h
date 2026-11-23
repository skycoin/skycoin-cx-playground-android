/* The uptime clock of the host harness. It replaces test/host/esp_timer.h,
 * which reads the wall clock, with the uptime of the message being
 * processed, taken from the tape. */
#ifndef ESP_TIMER_H
#define ESP_TIMER_H

#include <stdint.h>

int64_t esp_timer_get_time(void);

#endif // ESP_TIMER_H
