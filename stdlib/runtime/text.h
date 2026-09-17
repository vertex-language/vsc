// A growable run of UTF-8 that descriptions are written into.
#pragma once

#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"

namespace vertex {

struct Text {
  u8*   bytes;
  usize count;
  usize capacity;
  u8    local[128];
};

inline void textInit(Text& t) {
  t.bytes = t.local;
  t.count = 0;
  t.capacity = sizeof(t.local);
}

inline void textFree(Text& t) {
  if (t.bytes != t.local)
    vertex_pal_free(t.bytes, t.capacity, 1);
}

inline void textAppend(Text& t, const u8* bytes, usize n) {
  if (t.count + n > t.capacity) {
    usize cap = t.capacity * 2;
    while (cap < t.count + n)
      cap *= 2;
    auto* grown = static_cast<u8*>(vertex_pal_alloc(cap, 1));
    if (grown == nullptr)
      vertex_pal_abort();
    copyBytes(grown, t.bytes, t.count);
    textFree(t);
    t.bytes = grown;
    t.capacity = cap;
  }
  copyBytes(t.bytes + t.count, bytes, n);
  t.count += n;
}

inline void textByte(Text& t, u8 b) { textAppend(t, &b, 1); }

inline void textString(Text& t, const char* s) {
  usize n = 0;
  while (s[n] != 0)
    n++;
  textAppend(t, reinterpret_cast<const u8*>(s), n);
}

inline void textUnsigned(Text& t, u64 v) {
  u8 digits[20];
  usize n = 0;
  do {
    digits[n++] = static_cast<u8>('0' + v % 10);
    v /= 10;
  } while (v != 0);
  while (n > 0)
    textByte(t, digits[--n]);
}

// appendRepairedUTF8 appends bytes that should be UTF-8, with each
// maximal ill-formed subsequence replaced by U+FFFD, as the Unicode
// standard recommends and Swift's String(decoding:as:) does.
inline void appendRepairedUTF8(Text& t, const u8* b, usize n) {
  static const u8 replacement[3] = {0xEF, 0xBF, 0xBD};
  usize i = 0;
  while (i < n) {
    u8 c = b[i];
    if (c < 0x80) {
      textByte(t, c);
      i++;
      continue;
    }
    usize need = 0;
    u8 lo = 0x80, hi = 0xBF;
    if (c >= 0xC2 && c <= 0xDF) need = 1;
    else if (c == 0xE0) { need = 2; lo = 0xA0; }
    else if (c >= 0xE1 && c <= 0xEC) need = 2;
    else if (c == 0xED) { need = 2; hi = 0x9F; }
    else if (c >= 0xEE && c <= 0xEF) need = 2;
    else if (c == 0xF0) { need = 3; lo = 0x90; }
    else if (c >= 0xF1 && c <= 0xF3) need = 3;
    else if (c == 0xF4) { need = 3; hi = 0x8F; }
    if (need == 0) {
      textAppend(t, replacement, 3);
      i++;
      continue;
    }
    usize k = 1;
    while (k <= need && i + k < n) {
      u8 x = b[i + k];
      u8 l = k == 1 ? lo : 0x80, h = k == 1 ? hi : 0xBF;
      if (x < l || x > h)
        break;
      k++;
    }
    if (k == need + 1) {
      textAppend(t, b + i, k);
    } else {
      textAppend(t, replacement, 3);
    }
    i += k;
  }
}

inline void textSigned(Text& t, i64 v) {
  if (v < 0) {
    textByte(t, '-');
    textUnsigned(t, static_cast<u64>(0) - static_cast<u64>(v));
    return;
  }
  textUnsigned(t, static_cast<u64>(v));
}

} // namespace vertex
