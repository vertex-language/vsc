// The byte operations the runtime needs. Copying and filling are the
// platform's -- memcpy and memset, which use the machine's widest moves
// -- and comparing is a loop of its own.
#pragma once

#include "vertex/abi.h"
#include "vertex/platform.h"

namespace vertex {

inline void copyBytes(void* dest, const void* src, usize n) {
  if (n != 0)
    vertex_pal_copy(dest, src, n);
}

inline void fillBytes(void* dest, u8 byte, usize n) {
  if (n != 0)
    vertex_pal_fill(dest, byte, n);
}

// Whether a type's values are only bits: copied and moved by copying
// them, ended by nothing.
inline bool isPOD(const ValueWitnessTable* vw) { return (vw->flags & vwIsNonPOD) == 0; }

inline bool equalBytes(const u8* a, const u8* b, usize n) {
  for (usize i = 0; i < n; i++)
    if (a[i] != b[i])
      return false;
  return true;
}

} // namespace vertex
