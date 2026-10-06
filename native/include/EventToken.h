// Minimal WinRT event token ABI, missing from Zig's MinGW headers.
#ifndef PREFERANS_EVENTTOKEN_H
#define PREFERANS_EVENTTOKEN_H
#include <stdint.h>
typedef struct EventRegistrationToken { int64_t value; } EventRegistrationToken;
#endif
