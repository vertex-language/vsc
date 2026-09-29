// The platform layer for Android: bionic, declared here the way darwin.h
// declares libSystem, so that no NDK is needed to build the runtime. All but
// where errno is, bionic has in common with glibc: see linux_kernel.h.
#pragma once

#include "vertex/platform.h"

extern "C" {
int* __errno(void);
inline int* vertex_errno(void) { return __errno(); }
}

#include "linux_kernel.h"
