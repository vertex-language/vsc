// String: storage, and the operations compiled code calls.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"
#include "unicode.h"
#include "text.h"

extern "C" {
void vertex_retain(vertex::HeapObject* obj);
void vertex_release(vertex::HeapObject* obj);
}

namespace vertex {

// The bytes of a string, and how many. A small string's are copied out
// of its words into scratch the caller provides, since they are not
// addressable where they are.
struct StringBytes {
  const u8* bytes;
  usize     count;
};

inline StringBytes bytesOf(const String& s, u8* scratch) {
  u64 tag = s.object & stringTagMask;
  if ((tag & stringTagSmall) == stringTagSmall) {
    usize n = static_cast<usize>((s.object >> 56) & 0xF);
    // The words are the bytes in order, little-endian: two stores, not
    // fifteen. The sixteenth byte is the tag, past n.
    *reinterpret_cast<u64*>(scratch) = s.countAndFlags;
    *reinterpret_cast<u64*>(scratch + 8) = s.object;
    return {scratch, n};
  }
  usize n = static_cast<usize>(s.countAndFlags & stringCountMask);
  if (tag == stringTagLiteral)
    return {reinterpret_cast<const u8*>(s.object & stringPointerMask), n};
  auto* storage = reinterpret_cast<const u8*>(s.object);
  return {storage + stringStorageBytes, n};
}

// countOf is a String's length in bytes, read from its words.
inline usize countOf(const String& s) {
  if ((s.object & stringTagSmall) == stringTagSmall)
    return static_cast<usize>((s.object >> 56) & 0xF);
  return static_cast<usize>(s.countAndFlags & stringCountMask);
}

inline bool allASCII(const u8* bytes, usize n) {
  for (usize i = 0; i < n; i++)
    if (bytes[i] >= 0x80)
      return false;
  return true;
}

inline String smallString(const u8* bytes, usize n) {
  String s{0, stringTagSmall | (static_cast<u64>(n) << 56)};
  for (usize i = 0; i < n; i++) {
    if (i < 8)
      s.countAndFlags |= static_cast<u64>(bytes[i]) << (8 * i);
    else
      s.object |= static_cast<u64>(bytes[i]) << (8 * (i - 8));
  }
  return s;
}

// A String owning a fresh copy of n bytes, in whichever form fits.
inline String makeString(const u8* bytes, usize n) {
  if (n <= stringSmallCapacity)
    return smallString(bytes, n);
  auto* storage = static_cast<StringStorage*>(vertex_pal_alloc(stringStorageBytes + n, 16));
  if (storage == nullptr)
    vertex_pal_abort();
  storage->header.metadata = nullptr;
  storage->header.refcount = 1;
  storage->capacity = n;
  copyBytes(reinterpret_cast<u8*>(storage) + stringStorageBytes, bytes, n);
  u64 flags = allASCII(bytes, n) ? stringFlagASCII : 0;
  return {static_cast<u64>(n) | flags, reinterpret_cast<u64>(storage)};
}

} // namespace vertex

using namespace vertex;

extern "C" {

// vertex_string_literal is the String a literal's bytes denote. The
// bytes are the object file's and live forever, so a long literal
// points at them rather than copying; a short one is small.
String vertex_string_literal(const u8* bytes, u64 count, bool) {
  if (count <= stringSmallCapacity)
    return smallString(bytes, count);
  u64 flags = allASCII(bytes, count) ? stringFlagASCII : 0;
  return {count | flags, stringTagLiteral | reinterpret_cast<u64>(bytes)};
}

// vertex_string_from_utf8 is a String owning a copy of count bytes of
// valid UTF-8.
String vertex_string_from_utf8(const u8* bytes, u64 count) {
  return makeString(bytes, count);
}

// vertex_string_utf8 is where a String's bytes are and how many there
// are. A small string's are copied into scratch, which has room for 16.
const u8* vertex_string_utf8(u64 countAndFlags, u64 object, u8* scratch, u64* count) {
  StringBytes b = bytesOf(String{countAndFlags, object}, scratch);
  *count = b.count;
  return b.bytes;
}

HeapObject* vertex_alloc(u64 size);

// vertex_string_cstring is a String's bytes with a NUL after them, in an
// object of their own: withCString. vertex_string_cstring_free lets it go.
u8* vertex_string_cstring(u64 countAndFlags, u64 object) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{countAndFlags, object}, scratch);
  HeapObject* obj = vertex_alloc(b.count + 1);
  u8* at = reinterpret_cast<u8*>(obj) + sizeof(HeapObject);
  for (usize i = 0; i < b.count; i++)
    at[i] = b.bytes[i];
  return at;
}

void vertex_string_cstring_free(u8* bytes) {
  vertex_release(reinterpret_cast<HeapObject*>(bytes - sizeof(HeapObject)));
}

// vertex_string_from_cstring is the String of the bytes before a NUL.
String vertex_string_from_cstring(const u8* s) {
  usize n = 0;
  while (s[n] != 0)
    n++;
  return makeString(s, n);
}

void vertex_string_retain(u64 object) {
  if ((object & stringTagMask) == 0)
    vertex_retain(reinterpret_cast<HeapObject*>(object));
}

void vertex_string_release(u64 object) {
  if ((object & stringTagMask) == 0)
    vertex_release(reinterpret_cast<HeapObject*>(object));
}

// vertex_string_count is String.count: extended grapheme clusters.
i64 vertex_string_count(u64 countAndFlags, u64 object) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{countAndFlags, object}, scratch);
  return static_cast<i64>(graphemeCount(b.bytes, b.count));
}

// ---- Characters, by byte offset ----
//
// A String's indices are byte offsets into its UTF-8, each the first byte
// of a Character. These say where the Character at an index ends and
// where the one before it starts, which is all that walking a String by
// Characters needs; the core's String.Index is built on them.

} // extern "C"

namespace vertex {

// characterEnd is where the extended grapheme cluster starting at byte at
// ends: the next boundary, or count.
inline usize characterEnd(const u8* bytes, usize count, usize at) {
  if (at >= count)
    return count;
  // ASCII other than CR, with ASCII after it, is a Character alone: no
  // rule joins two ASCII scalars but CR LF.
  if (bytes[at] < 0x80 && bytes[at] != '\r' && (at + 1 == count || bytes[at + 1] < 0x80))
    return at + 1;
  usize i = at;
  GraphemeState state{0, 0, 0};
  GraphemeProperty prev = graphemeProperty(decodeScalar(bytes, count, &i));
  advance(state, prev);
  while (i < count) {
    usize here = i;
    GraphemeProperty next = graphemeProperty(decodeScalar(bytes, count, &i));
    if (breaksBetween(prev, next, state))
      return here;
    advance(state, next);
    prev = next;
  }
  return count;
}

// characterStart is where the Character that ends at byte at starts. A
// boundary depends on what came before it, so this reads from the start;
// ASCII before an ASCII scalar other than LF is the fast way out.
inline usize characterStart(const u8* bytes, usize count, usize at) {
  if (at == 0)
    return 0;
  if (at > count)
    at = count;
  if (bytes[at - 1] < 0x80 && bytes[at - 1] != '\n' && (at == 1 || bytes[at - 2] < 0x80))
    return at - 1;
  usize start = 0;
  while (start < at) {
    usize end = characterEnd(bytes, count, start);
    if (end >= at)
      return start;
    start = end;
  }
  return start;
}

} // namespace vertex

extern "C" {

// vertex_string_character_end is String.index(after:): where the Character
// at byte offset at ends.
i64 vertex_string_character_end(u64 s0, u64 s1, i64 at) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  return static_cast<i64>(characterEnd(b.bytes, b.count, at < 0 ? 0 : static_cast<usize>(at)));
}

// vertex_string_character_start is String.index(before:): where the
// Character that ends at byte offset at starts.
i64 vertex_string_character_start(u64 s0, u64 s1, i64 at) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  return static_cast<i64>(characterStart(b.bytes, b.count, at < 0 ? 0 : static_cast<usize>(at)));
}

// vertex_string_slice is the String of bytes from up to to: a Character,
// or the text of a Substring. The offsets are boundaries the caller found.
String vertex_string_slice(u64 s0, u64 s1, i64 from, i64 to) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  usize lo = from < 0 ? 0 : static_cast<usize>(from);
  usize hi = to < 0 ? 0 : static_cast<usize>(to);
  if (hi > b.count)
    hi = b.count;
  if (lo > hi)
    lo = hi;
  return makeString(b.bytes + lo, hi - lo);
}

} // extern "C"

namespace vertex {

// caseMapped is s with each scalar replaced by its full mapping in table,
// as Swift's uppercased() and lowercased() are: scalar by scalar, without
// regard to what is around it. ASCII maps by arithmetic.
inline String caseMapped(const String& s, const u32* table, u32 count, bool upper) {
  u8 scratch[16];
  StringBytes b = bytesOf(s, scratch);
  if (allASCII(b.bytes, b.count)) {
    Text t;
    textInit(t);
    for (usize i = 0; i < b.count; i++) {
      u8 c = b.bytes[i];
      if (upper && c >= 'a' && c <= 'z')
        c = static_cast<u8>(c - 32);
      else if (!upper && c >= 'A' && c <= 'Z')
        c = static_cast<u8>(c + 32);
      textByte(t, c);
    }
    String out = makeString(t.bytes, t.count);
    textFree(t);
    return out;
  }
  Text t;
  textInit(t);
  usize i = 0;
  u8 enc[4];
  while (i < b.count) {
    u32 c = decodeScalar(b.bytes, b.count, &i);
    u32 n = 0;
    const u32* mapped = caseMapping(table, count, c, &n);
    if (mapped == nullptr) {
      textAppend(t, enc, encodeScalar(c, enc));
      continue;
    }
    for (u32 k = 0; k < n; k++)
      textAppend(t, enc, encodeScalar(mapped[k], enc));
  }
  String out = makeString(t.bytes, t.count);
  textFree(t);
  return out;
}

} // namespace vertex

extern "C" {

// vertex_string_scalar reads the scalar at byte offset at of s: its value
// in the low 21 bits, the offset after it above them. unicodeScalars walks
// a String with it.
i64 vertex_string_scalar(u64 s0, u64 s1, i64 at) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  usize i = at < 0 ? 0 : static_cast<usize>(at);
  if (i >= b.count)
    return static_cast<i64>(b.count) << 21;
  u32 c = decodeScalar(b.bytes, b.count, &i);
  return static_cast<i64>(i) << 21 | static_cast<i64>(c);
}

// vertex_scalar_string is the String of one scalar.
String vertex_scalar_string(u32 c) {
  u8 enc[4];
  return makeString(enc, encodeScalar(c, enc));
}

// vertex_string_uppercased is String.uppercased().
String vertex_string_uppercased(u64 s0, u64 s1) {
  return caseMapped(String{s0, s1}, ucd::uppercaseMappings, ucd::uppercaseMappingsCount, true);
}

// vertex_string_lowercased is String.lowercased().
String vertex_string_lowercased(u64 s0, u64 s1) {
  return caseMapped(String{s0, s1}, ucd::lowercaseMappings, ucd::lowercaseMappingsCount, false);
}

// vertex_scalar_properties is what Unicode.Scalar.Properties reads: the
// flags in the low byte, the general category in the next, the numeric
// type in the one after.
u32 vertex_scalar_properties(u32 c) {
  return static_cast<u32>(flagsOf(c)) | static_cast<u32>(generalCategoryOf(c)) << 8 |
         static_cast<u32>(numericTypeOf(c)) << 16;
}

// vertex_scalar_whole_number is a scalar's numeric value where it is a
// whole number, or -1 where it has none or one with a fraction.
i64 vertex_scalar_whole_number(u32 c) {
  u64 n = 0;
  if (!wholeNumberOf(c, &n))
    return -1;
  return static_cast<i64>(n);
}

// vertex_string_utf8_count is how many bytes a String's UTF-8 is:
// utf8.count, and the offset of its endIndex.
i64 vertex_string_utf8_count(u64 s0, u64 s1) {
  return static_cast<i64>(countOf(String{s0, s1}));
}

String vertex_string_concat(u64 a0, u64 a1, u64 b0, u64 b1) {
  u8 sa[16], sb[16];
  StringBytes a = bytesOf(String{a0, a1}, sa);
  StringBytes b = bytesOf(String{b0, b1}, sb);
  usize n = a.count + b.count;
  if (n <= stringSmallCapacity) {
    u8 joined[16];
    copyBytes(joined, a.bytes, a.count);
    copyBytes(joined + a.count, b.bytes, b.count);
    return smallString(joined, n);
  }
  auto* storage = static_cast<StringStorage*>(vertex_pal_alloc(stringStorageBytes + n, 16));
  if (storage == nullptr)
    vertex_pal_abort();
  storage->header.metadata = nullptr;
  storage->header.refcount = 1;
  storage->capacity = n;
  u8* dest = reinterpret_cast<u8*>(storage) + stringStorageBytes;
  copyBytes(dest, a.bytes, a.count);
  copyBytes(dest + a.count, b.bytes, b.count);
  u64 flags = allASCII(dest, n) ? stringFlagASCII : 0;
  return {static_cast<u64>(n) | flags, reinterpret_cast<u64>(storage)};
}

// vertex_fatal_error is fatalError: what was printed is flushed, the
// message goes to standard error after Swift's prefix, and the process
// traps.
[[noreturn]] void vertex_fatal_error(u64 m0, u64 m1) {
  u8 scratch[16];
  StringBytes m = bytesOf(String{m0, m1}, scratch);
  vertex_pal_flush();
  static const char prefix[] = "Fatal error: ";
  vertex_pal_write(2, reinterpret_cast<const u8*>(prefix), sizeof(prefix) - 1);
  vertex_pal_write(2, m.bytes, m.count);
  vertex_pal_write(2, reinterpret_cast<const u8*>("\n"), 1);
  __builtin_trap();
}

// Equality is canonical equivalence, as Swift's is: "é" precomposed and
// decomposed are one string.
bool vertex_string_equal(u64 a0, u64 a1, u64 b0, u64 b1) {
  u8 sa[16], sb[16];
  StringBytes a = bytesOf(String{a0, a1}, sa);
  StringBytes b = bytesOf(String{b0, b1}, sb);
  if (a.count == b.count && equalBytes(a.bytes, b.bytes, a.count))
    return true;
  return canonicalCompare(a.bytes, a.count, b.bytes, b.count) == 0;
}

// vertex_string_equal_fold is equality with ASCII letters' case set
// aside: what a protocol that says its names are case-insensitive, HTTP's
// header names among them, compares by. Bytes beyond ASCII must match
// exactly.
bool vertex_string_equal_fold(u64 a0, u64 a1, u64 b0, u64 b1) {
  // Lengths first: different ones are the usual answer, and need no bytes.
  if (countOf(String{a0, a1}) != countOf(String{b0, b1}))
    return false;
  u8 sa[16], sb[16];
  StringBytes a = bytesOf(String{a0, a1}, sa);
  StringBytes b = bytesOf(String{b0, b1}, sb);
  if (a.count != b.count)
    return false;
  for (usize i = 0; i < a.count; i++) {
    u8 x = a.bytes[i], y = b.bytes[i];
    if (x == y)
      continue;
    if (x >= 'A' && x <= 'Z')
      x += 32;
    if (y >= 'A' && y <= 'Z')
      y += 32;
    if (x != y)
      return false;
  }
  return true;
}

// vertex_string_equal_fold_bytes is vertex_string_equal_fold between
// count bytes that are not a String and one that is: what a parser that
// holds a header name as a span of its buffer compares it to a name by,
// without making the span into a String first.
bool vertex_string_equal_fold_bytes(const u8* bytes, u64 count, u64 b0, u64 b1) {
  if (countOf(String{b0, b1}) != count)
    return false;
  u8 sb[16];
  StringBytes b = bytesOf(String{b0, b1}, sb);
  if (b.count != count)
    return false;
  for (usize i = 0; i < count; i++) {
    u8 x = bytes[i], y = b.bytes[i];
    if (x == y)
      continue;
    if (x >= 'A' && x <= 'Z')
      x += 32;
    if (y >= 'A' && y <= 'Z')
      y += 32;
    if (x != y)
      return false;
  }
  return true;
}

// Ordering is by Unicode scalar value of the canonical forms.
bool vertex_string_less(u64 a0, u64 a1, u64 b0, u64 b1) {
  u8 sa[16], sb[16];
  StringBytes a = bytesOf(String{a0, a1}, sa);
  StringBytes b = bytesOf(String{b0, b1}, sb);
  return canonicalCompare(a.bytes, a.count, b.bytes, b.count) < 0;
}

}
