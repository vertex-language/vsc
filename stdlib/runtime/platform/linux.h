// The platform layer for Linux: glibc, declared here the way darwin.h
// declares libSystem, so that no sysroot is needed to build the runtime. All
// but where errno is, glibc has in common with bionic: see linux_kernel.h.
#pragma once

#include "vertex/platform.h"

extern "C" {
int* __errno_location(void);
inline int* vertex_errno(void) { return __errno_location(); }
}

#include "linux_kernel.h"
