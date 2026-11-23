/* Included before every source of the zclaw host harness: time() reads the
 * clock records of the tape. */
#pragma once

#include <time.h>

time_t host_time(time_t *out);
#define time(out) host_time(out)
