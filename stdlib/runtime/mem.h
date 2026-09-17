// The byte operations the runtime needs, written here so that a
// freestanding link does not have to supply them.
#pragma once

#include "vertex/abi.h"

namespace vertex {

inline void copyBytes(void* dest, const void* src, usize n) {
  auto* d = static_cast<u8*>(dest);
  auto* s = static_cast<const u8*>(src);
  for (usize i = 0; i < n; i++)
    d[i] = s[i];
}

inline bool equalBytes(const u8* a, const u8* b, usize n) {
  for (usize i = 0; i < n; i++)
    if (a[i] != b[i])
      return false;
  return true;
}

} // namespace vertex
