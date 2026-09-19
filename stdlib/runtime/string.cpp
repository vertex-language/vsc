// String: storage, and the operations compiled code calls.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"
#include "unicode.h"

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
    for (usize i = 0; i < 8; i++)
      scratch[i] = static_cast<u8>(s.countAndFlags >> (8 * i));
    for (usize i = 0; i < 7; i++)
      scratch[8 + i] = static_cast<u8>(s.object >> (8 * i));
    return {scratch, n};
  }
  usize n = static_cast<usize>(s.countAndFlags & stringCountMask);
  if (tag == stringTagLiteral)
    return {reinterpret_cast<const u8*>(s.object & stringPointerMask), n};
  auto* storage = reinterpret_cast<const u8*>(s.object);
  return {storage + stringStorageBytes, n};
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

// Ordering is by Unicode scalar value of the canonical forms.
bool vertex_string_less(u64 a0, u64 a1, u64 b0, u64 b1) {
  u8 sa[16], sb[16];
  StringBytes a = bytesOf(String{a0, a1}, sa);
  StringBytes b = bytesOf(String{b0, b1}, sb);
  return canonicalCompare(a.bytes, a.count, b.bytes, b.count) < 0;
}

}
