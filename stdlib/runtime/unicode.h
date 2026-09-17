// Unicode: what String needs of it. Counting extended grapheme clusters
// (UAX #29) and comparing by canonical equivalence (UAX #15), over the
// tables in unicode_tables.h.
#pragma once

#include "vertex/abi.h"
#include "vertex/platform.h"
#include "unicode_tables.h"

namespace vertex {

// decodeScalar reads one scalar at bytes[*at] and advances past it.
// Swift's Strings are valid UTF-8 by construction, so this does not
// validate; a truncated sequence reads as U+FFFD rather than running off
// the end.
inline u32 decodeScalar(const u8* bytes, usize count, usize* at) {
  usize i = *at;
  u32 b0 = bytes[i];
  if (b0 < 0x80) {
    *at = i + 1;
    return b0;
  }
  usize n = b0 >= 0xF0 ? 4 : b0 >= 0xE0 ? 3 : 2;
  if (i + n > count) {
    *at = count;
    return 0xFFFD;
  }
  u32 c = b0 & (0x7F >> n);
  for (usize k = 1; k < n; k++)
    c = (c << 6) | (bytes[i + k] & 0x3F);
  *at = i + n;
  return c;
}

// rangeValue finds c in a range table and answers its value, or zero.
inline u8 rangeValue(const u32* table, u32 count, u32 c) {
  u32 lo = 0, hi = count / 2;
  while (lo < hi) {
    u32 mid = (lo + hi) / 2;
    if (table[2 * mid] <= c)
      lo = mid + 1;
    else
      hi = mid;
  }
  if (lo == 0)
    return 0;
  u32 packed = table[2 * (lo - 1) + 1];
  return (packed >> 8) >= c ? static_cast<u8>(packed & 0xFF) : 0;
}

// ---- grapheme clusters ----

enum : u8 {
  gcbOther, gcbCR, gcbLF, gcbControl, gcbExtend, gcbZWJ, gcbRI, gcbPrepend,
  gcbSpacingMark, gcbL, gcbV, gcbT, gcbLV, gcbLVT,
};

enum : u8 { incbNone, incbConsonant, incbExtend, incbLinker };

struct GraphemeProperty {
  u8 gcb;
  bool pictographic;
  u8 incb;
};

inline GraphemeProperty graphemeProperty(u32 c) {
  u8 v = c < 0x20 ? (c == 0x0D ? gcbCR : c == 0x0A ? gcbLF : gcbControl)
         : c < 0x7F ? 0
                    : rangeValue(ucd::graphemeProperties, ucd::graphemePropertiesCount, c);
  return {static_cast<u8>(v & 0xF), (v & 0x10) != 0, static_cast<u8>((v >> 5) & 0x3)};
}

// What the rules that look further back than one scalar need to know.
struct GraphemeState {
  u32 regionalIndicators; // consecutive RI up to and including the previous
  u8  emoji;              // 1: ExtPict Extend*   2: ExtPict Extend* ZWJ
  u8  conjunct;           // 1: Consonant [Extend|Linker]*   2: ... with a Linker
};

inline bool isControl(u8 g) { return g == gcbCR || g == gcbLF || g == gcbControl; }

// breaksBetween is UAX #29's rules GB3 through GB999, for the boundary
// between a scalar with property prev and the next with property next.
inline bool breaksBetween(GraphemeProperty prev, GraphemeProperty next, const GraphemeState& s) {
  u8 p = prev.gcb, n = next.gcb;
  if (p == gcbCR && n == gcbLF) return false;                          // GB3
  if (isControl(p) || isControl(n)) return true;                       // GB4, GB5
  if (p == gcbL && (n == gcbL || n == gcbV || n == gcbLV || n == gcbLVT)) return false; // GB6
  if ((p == gcbLV || p == gcbV) && (n == gcbV || n == gcbT)) return false; // GB7
  if ((p == gcbLVT || p == gcbT) && n == gcbT) return false;           // GB8
  if (n == gcbExtend || n == gcbZWJ) return false;                     // GB9
  if (n == gcbSpacingMark) return false;                               // GB9a
  if (p == gcbPrepend) return false;                                   // GB9b
  if (next.incb == incbConsonant && s.conjunct == 2) return false;     // GB9c
  if (p == gcbZWJ && next.pictographic && s.emoji == 2) return false;  // GB11
  if (p == gcbRI && n == gcbRI && (s.regionalIndicators & 1)) return false; // GB12, GB13
  return true;                                                         // GB999
}

inline void advance(GraphemeState& s, GraphemeProperty c) {
  s.regionalIndicators = c.gcb == gcbRI ? s.regionalIndicators + 1 : 0;

  if (c.pictographic)
    s.emoji = 1;
  else if (s.emoji == 1 && c.gcb == gcbExtend)
    s.emoji = 1;
  else if (s.emoji == 1 && c.gcb == gcbZWJ)
    s.emoji = 2;
  else
    s.emoji = 0;

  if (c.incb == incbConsonant)
    s.conjunct = 1;
  else if (s.conjunct != 0 && c.incb == incbExtend)
    s.conjunct = s.conjunct;
  else if (s.conjunct != 0 && c.incb == incbLinker)
    s.conjunct = 2;
  else
    s.conjunct = 0;
}

// graphemeCount is how many extended grapheme clusters the bytes hold.
inline usize graphemeCount(const u8* bytes, usize count) {
  if (count == 0)
    return 0;
  // Printable ASCII breaks everywhere: no rule applies between two such
  // scalars. CR LF is the one ASCII pair that does not break, and it is
  // not printable, so a run of printable ASCII counts as its length.
  usize i = 0;
  usize clusters = 0;
  while (i < count && bytes[i] >= 0x20 && bytes[i] < 0x7F)
    i++;
  if (i == count)
    return count;
  if (i > 0) {
    clusters = i - 1;
    i--;
  }
  GraphemeState state{0, 0, 0};
  GraphemeProperty prev = graphemeProperty(decodeScalar(bytes, count, &i));
  advance(state, prev);
  clusters++;
  while (i < count) {
    GraphemeProperty next = graphemeProperty(decodeScalar(bytes, count, &i));
    if (breaksBetween(prev, next, state))
      clusters++;
    advance(state, next);
    prev = next;
  }
  return clusters;
}

// ---- canonical equivalence ----

inline u8 combiningClass(u32 c) {
  if (c < 0x300)
    return 0;
  return rangeValue(ucd::combiningClasses, ucd::combiningClassesCount, c);
}

inline constexpr u32 hangulS = 0xAC00, hangulL = 0x1100, hangulV = 0x1161, hangulT = 0x11A7;
inline constexpr u32 hangulVCount = 21, hangulTCount = 28, hangulSCount = 11172;

// decompose appends c's full canonical decomposition to out and answers
// how many scalars it wrote.
inline usize decompose(u32 c, u32* out) {
  if (c - hangulS < hangulSCount) {
    u32 s = c - hangulS;
    out[0] = hangulL + s / (hangulVCount * hangulTCount);
    out[1] = hangulV + (s % (hangulVCount * hangulTCount)) / hangulTCount;
    u32 t = s % hangulTCount;
    if (t == 0)
      return 2;
    out[2] = hangulT + t;
    return 3;
  }
  if (c >= 0xC0) {
    u32 lo = 0, hi = ucd::decompositionIndexCount / 2;
    while (lo < hi) {
      u32 mid = (lo + hi) / 2;
      u32 key = ucd::decompositionIndex[2 * mid];
      if (key == c) {
        u32 packed = ucd::decompositionIndex[2 * mid + 1];
        u32 at = packed >> 8, n = packed & 0xFF;
        for (u32 k = 0; k < n; k++)
          out[k] = ucd::decompositionPool[at + k];
        return n;
      }
      if (key < c)
        lo = mid + 1;
      else
        hi = mid;
    }
  }
  out[0] = c;
  return 1;
}

// compose is the primary composite of a starter and the scalar after
// it, or zero.
inline u32 compose(u32 a, u32 b) {
  if (a - hangulL < 19 && b - hangulV < hangulVCount)
    return hangulS + ((a - hangulL) * hangulVCount + (b - hangulV)) * hangulTCount;
  if (a - hangulS < hangulSCount && (a - hangulS) % hangulTCount == 0 && b - hangulT - 1 < hangulTCount - 1)
    return a + (b - hangulT);
  u32 lo = 0, hi = ucd::compositionsCount / 3;
  while (lo < hi) {
    u32 mid = (lo + hi) / 2;
    u32 x = ucd::compositions[3 * mid], y = ucd::compositions[3 * mid + 1];
    if (x == a && y == b)
      return ucd::compositions[3 * mid + 2];
    if (x < a || (x == a && y < b))
      lo = mid + 1;
    else
      hi = mid;
  }
  return 0;
}

// normalizeNFC writes the NFC form of the bytes' scalars into out, which
// has room for four scalars per byte, and answers how many.
inline usize normalizeNFC(const u8* bytes, usize count, u32* out) {
  usize n = 0;
  for (usize i = 0; i < count;)
    n += decompose(decodeScalar(bytes, count, &i), out + n);
  // Canonical ordering: a stable sort of each run of non-starters by
  // combining class.
  for (usize i = 1; i < n; i++) {
    u8 cc = combiningClass(out[i]);
    if (cc == 0)
      continue;
    usize j = i;
    u32 c = out[i];
    while (j > 0 && combiningClass(out[j - 1]) > cc) {
      out[j] = out[j - 1];
      j--;
    }
    out[j] = c;
  }
  if (n == 0)
    return 0;
  // Canonical composition, as UAX #15's reference implementation writes it.
  usize starter = 0;
  u32 starterScalar = out[0];
  i32 lastClass = combiningClass(starterScalar) == 0 ? 0 : 256;
  usize kept = 1;
  for (usize i = 1; i < n; i++) {
    u32 c = out[i];
    i32 cc = combiningClass(c);
    u32 composite = compose(starterScalar, c);
    if (composite != 0 && (lastClass < cc || lastClass == 0)) {
      out[starter] = composite;
      starterScalar = composite;
      continue;
    }
    if (cc == 0) {
      starter = kept;
      starterScalar = c;
    }
    lastClass = cc;
    out[kept++] = c;
  }
  return kept;
}

inline bool asciiOnly(const u8* bytes, usize count) {
  for (usize i = 0; i < count; i++)
    if (bytes[i] >= 0x80)
      return false;
  return true;
}

// canonicalCompare orders two strings by the scalar values of their NFC
// forms, which is Swift's order, and answers -1, 0 or 1.
inline i32 canonicalCompare(const u8* a, usize an, const u8* b, usize bn) {
  if (asciiOnly(a, an) && asciiOnly(b, bn)) {
    usize n = an < bn ? an : bn;
    for (usize i = 0; i < n; i++)
      if (a[i] != b[i])
        return a[i] < b[i] ? -1 : 1;
    return an == bn ? 0 : an < bn ? -1 : 1;
  }
  auto* na = static_cast<u32*>(vertex_pal_alloc(4 * sizeof(u32) * (an + 1), 4));
  auto* nb = static_cast<u32*>(vertex_pal_alloc(4 * sizeof(u32) * (bn + 1), 4));
  if (na == nullptr || nb == nullptr)
    vertex_pal_abort();
  usize ca = normalizeNFC(a, an, na);
  usize cb = normalizeNFC(b, bn, nb);
  i32 order = 0;
  usize n = ca < cb ? ca : cb;
  for (usize i = 0; i < n && order == 0; i++)
    if (na[i] != nb[i])
      order = na[i] < nb[i] ? -1 : 1;
  if (order == 0 && ca != cb)
    order = ca < cb ? -1 : 1;
  vertex_pal_free(na, 0, 4);
  vertex_pal_free(nb, 0, 4);
  return order;
}

} // namespace vertex
