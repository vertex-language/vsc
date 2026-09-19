// Collections: hashing, Array mutation, and the hash table Dictionary and
// Set are made of.
//
// Everything here is type-erased. A value arrives by address with the
// metadata of its type, and is copied, moved and destroyed through that
// type's value witnesses. Hashing and equality are answered for the types
// the core declares -- the integers, Bool, Float, Double and String --
// by which metadata record the value's type is.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"

extern "C" {
void vertex_retain(vertex::HeapObject* obj);
void vertex_release(vertex::HeapObject* obj);
bool vertex_array_equal(vertex::ArrayStorage* a, vertex::ArrayStorage* b, const vertex::Metadata* element);
}

namespace vertex {

// ---- hashing: SipHash-1-3, as Swift's Hasher uses ----

// A Hasher's state, laid out as core's Hasher struct is: the four SipHash
// words, the bytes not yet a word, and how many bytes have gone in. The
// tail's fill is the length modulo eight.
struct Sip {
  u64 v0, v1, v2, v3;
  u64 tail;
  u64 length;
};

inline u64 rotl(u64 x, u32 b) { return (x << b) | (x >> (64 - b)); }

inline void sipRound(Sip& s) {
  s.v0 += s.v1; s.v1 = rotl(s.v1, 13); s.v1 ^= s.v0; s.v0 = rotl(s.v0, 32);
  s.v2 += s.v3; s.v3 = rotl(s.v3, 16); s.v3 ^= s.v2;
  s.v0 += s.v3; s.v3 = rotl(s.v3, 21); s.v3 ^= s.v0;
  s.v2 += s.v1; s.v1 = rotl(s.v1, 17); s.v1 ^= s.v2; s.v2 = rotl(s.v2, 32);
}

inline void sipInit(Sip& s, u64 k0, u64 k1) {
  s.v0 = k0 ^ 0x736f6d6570736575ull;
  s.v1 = k1 ^ 0x646f72616e646f6dull;
  s.v2 = k0 ^ 0x6c7967656e657261ull;
  s.v3 = k1 ^ 0x7465646279746573ull;
  s.tail = 0;
  s.length = 0;
}

inline void sipWord(Sip& s, u64 m) {
  s.v3 ^= m;
  sipRound(s);
  s.v0 ^= m;
}

inline void sipByte(Sip& s, u8 b) {
  u64 fill = s.length & 7;
  s.tail |= static_cast<u64>(b) << (8 * fill);
  s.length++;
  if ((s.length & 7) == 0) {
    sipWord(s, s.tail);
    s.tail = 0;
  }
}

inline void sipBytes(Sip& s, const u8* p, usize n) {
  for (usize i = 0; i < n; i++)
    sipByte(s, p[i]);
}

inline u64 sipFinish(Sip& s) {
  sipWord(s, s.tail | (s.length << 56));
  s.v2 ^= 0xff;
  sipRound(s);
  sipRound(s);
  sipRound(s);
  return s.v0 ^ s.v1 ^ s.v2 ^ s.v3;
}

// The per-process seed, from the platform's entropy the first time a
// value is hashed. Iteration order over a Dictionary or Set therefore
// differs between runs, as Swift's does.
struct Seed {
  u64 k0, k1;
  bool ready;
};

inline Seed& seed() {
  static Seed s = {0, 0, false};
  if (!s.ready) {
    vertex_pal_entropy(reinterpret_cast<u8*>(&s.k0), sizeof(u64));
    vertex_pal_entropy(reinterpret_cast<u8*>(&s.k1), sizeof(u64));
    s.ready = true;
  }
  return s;
}

} // namespace vertex

extern "C" {
extern const vertex::FullMetadata vertex_metadata_Bool, vertex_metadata_Int8,
    vertex_metadata_UInt8, vertex_metadata_Int16, vertex_metadata_UInt16,
    vertex_metadata_Int32, vertex_metadata_UInt32, vertex_metadata_Float,
    vertex_metadata_Int, vertex_metadata_UInt, vertex_metadata_Int64,
    vertex_metadata_UInt64, vertex_metadata_Double, vertex_metadata_String;
}

namespace vertex {

inline bool record(const Metadata* type, const FullMetadata& r) {
  return static_cast<const void*>(type) == static_cast<const void*>(&r.metadata);
}

// The width of an integer or Bool type's value, or zero for any other.
inline usize integerBytes(const Metadata* t) {
  if (record(t, vertex_metadata_Bool) || record(t, vertex_metadata_Int8) || record(t, vertex_metadata_UInt8))
    return 1;
  if (record(t, vertex_metadata_Int16) || record(t, vertex_metadata_UInt16))
    return 2;
  if (record(t, vertex_metadata_Int32) || record(t, vertex_metadata_UInt32))
    return 4;
  if (record(t, vertex_metadata_Int) || record(t, vertex_metadata_UInt) ||
      record(t, vertex_metadata_Int64) || record(t, vertex_metadata_UInt64))
    return 8;
  return 0;
}

// hashInto feeds a value into a hash. Two values equal by valuesEqual feed
// it alike: a String by the scalars of its canonical form, -0.0 as 0.0, an
// Optional by whether it has a value and then the value, an Array by its
// count and its elements. A value of any other type is fed through its
// Hashable conformance -- its own hash(into:), or the one derived for it.
void hashInto(Sip& s, const void* value, const Metadata* type) {
  if (usize n = integerBytes(type)) {
    sipBytes(s, static_cast<const u8*>(value), n);
    return;
  }
  if (record(type, vertex_metadata_Double)) {
    u64 bits = *static_cast<const u64*>(value);
    if ((bits << 1) == 0)
      bits = 0;
    sipBytes(s, reinterpret_cast<const u8*>(&bits), 8);
    return;
  }
  if (record(type, vertex_metadata_Float)) {
    u32 bits = *static_cast<const u32*>(value);
    if ((bits << 1) == 0)
      bits = 0;
    sipBytes(s, reinterpret_cast<const u8*>(&bits), 4);
    return;
  }
  if (record(type, vertex_metadata_String)) {
    u8 scratch[16];
    StringBytes b = bytesOf(*static_cast<const String*>(value), scratch);
    if (asciiOnly(b.bytes, b.count)) {
      for (usize i = 0; i < b.count; i++) {
        u32 c = b.bytes[i];
        sipBytes(s, reinterpret_cast<const u8*>(&c), 4);
      }
    } else {
      auto* nfc = static_cast<u32*>(vertex_pal_alloc(4 * sizeof(u32) * (b.count + 1), 4));
      if (nfc == nullptr)
        vertex_pal_abort();
      usize n = normalizeNFC(b.bytes, b.count, nfc);
      for (usize i = 0; i < n; i++)
        sipBytes(s, reinterpret_cast<const u8*>(&nfc[i]), 4);
      vertex_pal_free(nfc, 0, 4);
    }
    // A terminator, so that ["ab", "c"] and ["a", "bc"] differ.
    sipByte(s, 0xff);
    return;
  }
  if (type->kind == kindOptional) {
    auto* o = static_cast<const OptionalMetadata*>(type);
    if (optionalIsNone(value, o)) {
      sipByte(s, 0);
      return;
    }
    sipByte(s, 1);
    hashInto(s, value, o->payload);
    return;
  }
  if (type->kind == kindArray) {
    auto* a = static_cast<const ArrayMetadata*>(type);
    auto* storage = *static_cast<const ArrayStorage* const*>(value);
    u64 count = static_cast<u64>(storage->count);
    sipBytes(s, reinterpret_cast<const u8*>(&count), 8);
    usize stride = witnesses(a->element)->stride;
    auto* elements = reinterpret_cast<const u8*>(storage) + arrayStorageElements;
    for (i64 i = 0; i < storage->count; i++)
      hashInto(s, elements + i * stride, a->element);
    return;
  }
  // Hashable's table: its descriptor, Equatable's table, then hash(into:).
  if (const void* const* table = vertex_conformance(type, &vertex_protocol_Hashable)) {
    auto* fn = static_cast<void (*)()>(const_cast<void*>(table[2]));
    vertex_witness_call1(fn, value, &s, type, table);
    return;
  }
  fatal("hashing a value whose type is not Hashable");
}

// hashValue is a value's hash under the process seed.
u64 hashValue(const void* value, const Metadata* type) {
  Seed& k = seed();
  Sip s;
  sipInit(s, k.k0, k.k1);
  hashInto(s, value, type);
  return sipFinish(s);
}

// valuesEqual is `==` on two values of one type: the runtime's own for
// the types it knows, and otherwise the type's Equatable conformance.
bool valuesEqual(const void* a, const void* b, const Metadata* type) {
  if (usize n = integerBytes(type))
    return equalBytes(static_cast<const u8*>(a), static_cast<const u8*>(b), n);
  if (record(type, vertex_metadata_Double)) {
    u64 x = *static_cast<const u64*>(a), y = *static_cast<const u64*>(b);
    if ((x << 1) == 0 && (y << 1) == 0)
      return true;
    // A NaN is equal to nothing; the bit pattern of one is unequal to
    // itself only through the exponent and mantissa test.
    bool nanX = (x & 0x7FF0000000000000ull) == 0x7FF0000000000000ull && (x & 0x000FFFFFFFFFFFFFull) != 0;
    return !nanX && x == y;
  }
  if (record(type, vertex_metadata_Float)) {
    u32 x = *static_cast<const u32*>(a), y = *static_cast<const u32*>(b);
    if ((x << 1) == 0 && (y << 1) == 0)
      return true;
    bool nanX = (x & 0x7F800000u) == 0x7F800000u && (x & 0x003FFFFFu) != 0;
    return !nanX && x == y;
  }
  if (record(type, vertex_metadata_String)) {
    u8 sa[16], sb[16];
    StringBytes x = bytesOf(*static_cast<const String*>(a), sa);
    StringBytes y = bytesOf(*static_cast<const String*>(b), sb);
    if (x.count == y.count && equalBytes(x.bytes, y.bytes, x.count))
      return true;
    return canonicalCompare(x.bytes, x.count, y.bytes, y.count) == 0;
  }
  if (type->kind == kindOptional) {
    auto* o = static_cast<const OptionalMetadata*>(type);
    bool noneA = optionalIsNone(a, o), noneB = optionalIsNone(b, o);
    if (noneA || noneB)
      return noneA == noneB;
    return valuesEqual(a, b, o->payload);
  }
  if (type->kind == kindArray) {
    auto* m = static_cast<const ArrayMetadata*>(type);
    auto* x = *static_cast<ArrayStorage* const*>(a);
    auto* y = *static_cast<ArrayStorage* const*>(b);
    return vertex_array_equal(x, y, m->element);
  }
  // Equatable's table: its descriptor, then ==.
  if (const void* const* table = vertex_conformance(type, &vertex_protocol_Equatable)) {
    auto* fn = static_cast<void (*)()>(const_cast<void*>(table[1]));
    return vertex_witness_call_static2(fn, a, b, type, table);
  }
  fatal("comparing values whose type is not Equatable");
}

// ---- Optionals written through their metadata ----

inline bool optionalTagged(const OptionalMetadata* o) {
  return o->tagOffset >= witnesses(o->payload)->size;
}

// writeNone makes the storage at out an empty Optional.
inline void writeNone(void* out, const OptionalMetadata* o) {
  auto* p = static_cast<u8*>(out);
  usize size = witnesses(o)->size;
  for (usize i = 0; i < size; i++)
    p[i] = 0;
  for (u32 i = 0; i < o->tagBytes; i++)
    p[o->tagOffset + i] = static_cast<u8>(o->none >> (8 * i));
}

// markSome finishes an Optional whose payload has been written at out.
inline void markSome(void* out, const OptionalMetadata* o) {
  if (optionalTagged(o)) {
    auto* p = static_cast<u8*>(out);
    for (u32 i = 0; i < o->tagBytes; i++)
      p[o->tagOffset + i] = 0;
  }
}

// ---- Array ----

inline u8* elementsOf(ArrayStorage* a) {
  return reinterpret_cast<u8*>(a) + arrayStorageElements;
}

void destroyArray(HeapObject* object);

// newArray is empty storage with room for capacity elements of a type.
ArrayStorage* newArray(i64 capacity, const Metadata* element) {
  usize stride = witnesses(element)->stride;
  auto* a = static_cast<ArrayStorage*>(
      vertex_pal_alloc(arrayStorageElements + static_cast<usize>(capacity) * stride, 16));
  if (a == nullptr)
    vertex_pal_abort();
  a->header.metadata = &arrayHeapMetadata;
  a->header.refcount = 1;
  a->count = 0;
  a->capacity = capacity;
  a->element = element;
  return a;
}

// uniqueArray makes the storage at slot the caller's alone, with room for
// at least `need` elements, and answers it. Shared storage is copied
// through the element's witnesses; storage of the caller's own that is
// too small is moved and its memory returned.
ArrayStorage* uniqueArray(ArrayStorage** slot, i64 need, const Metadata* element) {
  ArrayStorage* a = *slot;
  bool immortalStorage = (a->header.refcount & immortal) != 0;
  bool unique = !immortalStorage && a->header.refcount == 1;
  if (unique && a->capacity >= need)
    return a;
  i64 capacity = a->capacity;
  if (capacity < need) {
    capacity = capacity < 4 ? 4 : capacity * 2;
    if (capacity < need)
      capacity = need;
  }
  ArrayStorage* fresh = newArray(capacity, element);
  const ValueWitnessTable* vw = witnesses(element);
  u8* from = elementsOf(a);
  u8* to = elementsOf(fresh);
  if (isPOD(vw)) {
    // Bits are moved and copied the same way: all at once.
    copyBytes(to, from, static_cast<usize>(a->count) * vw->stride);
  } else {
    for (i64 i = 0; i < a->count; i++) {
      if (unique)
        vw->initializeWithTake(to + i * vw->stride, from + i * vw->stride, element);
      else
        vw->initializeWithCopy(to + i * vw->stride, from + i * vw->stride, element);
    }
  }
  fresh->count = a->count;
  if (unique)
    vertex_pal_free(a, 0, 16);
  else
    vertex_release(&a->header);
  *slot = fresh;
  return fresh;
}

inline void checkIndex(i64 index, i64 count) {
  if (index < 0 || index >= count)
    fatal("Index out of range");
}

} // namespace vertex

using namespace vertex;

extern "C" {

ArrayAllocation vertex_array_allocate(i64 count, const Metadata* element);

// roomFor is uniqueArray's answer without the call in the case an append
// nearly always is: storage that is the slot's alone with room to spare.
// An immortal array's refcount carries the immortal bit, so it is never 1.
inline ArrayStorage* roomFor(ArrayStorage** slot, i64 need, const Metadata* element) {
  ArrayStorage* a = *slot;
  if (a->header.refcount == 1 && a->capacity >= need)
    return a;
  return uniqueArray(slot, need, element);
}

// vertex_array_append takes the value at `value` onto the end. A plain
// value of a machine word or less -- a byte, an Int, a pointer, a small
// struct of them -- is moved as its bits, without the witness call.
void vertex_array_append(ArrayStorage** slot, void* value, const Metadata* element) {
  ArrayStorage* a = roomFor(slot, (*slot)->count + 1, element);
  const ValueWitnessTable* vw = witnesses(element);
  u8* at = elementsOf(a) + a->count * vw->stride;
  if (isPOD(vw)) {
    switch (vw->stride) {
    case 1:
      *at = *static_cast<u8*>(value);
      break;
    case 8:
      *reinterpret_cast<u64*>(at) = *static_cast<u64*>(value);
      break;
    default:
      copyBytes(at, value, static_cast<usize>(vw->stride));
      break;
    }
  } else {
    vw->initializeWithTake(at, value, element);
  }
  a->count++;
}

// vertex_array_unique_elements is where the array at slot keeps its
// elements once that storage is the slot's alone, so that what is written
// through the address changes this array and no other.
u8* vertex_array_unique_elements(ArrayStorage** slot, const Metadata* element) {
  return elementsOf(uniqueArray(slot, (*slot)->count, element));
}

const u8* vertex_string_utf8(u64 countAndFlags, u64 object, u8* scratch, u64* count);

// vertex_string_utf8_array is a String's UTF-8 bytes, as an array of them.
ArrayStorage* vertex_string_utf8_array(u64 countAndFlags, u64 object) {
  u8 scratch[16];
  u64 count = 0;
  const u8* bytes = vertex_string_utf8(countAndFlags, object, scratch, &count);
  ArrayStorage* a = newArray(static_cast<i64>(count), &vertex_metadata_UInt8.metadata);
  copyBytes(elementsOf(a), bytes, static_cast<usize>(count));
  a->count = static_cast<i64>(count);
  return a;
}

// vertex_array_append_utf8 copies a String's bytes onto the end of an
// array of bytes: `bytes.append(contentsOf: s.utf8)`, which in Swift
// walks a view and here is one copy.
void vertex_array_append_utf8(ArrayStorage** slot, u64 s0, u64 s1) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  if (b.count == 0)
    return;
  ArrayStorage* a = roomFor(slot, (*slot)->count + static_cast<i64>(b.count),
                             &vertex_metadata_UInt8.metadata);
  copyBytes(elementsOf(a) + a->count, b.bytes, b.count);
  a->count += static_cast<i64>(b.count);
}

// vertex_array_append_contents copies every element of other onto the end.
void vertex_array_append_contents(ArrayStorage** slot, ArrayStorage* other, const Metadata* element) {
  i64 n = other->count;
  if (n == 0)
    return;
  // other is held across the growth in case it is this array's own
  // storage, which growing frees. When it is not, nothing can free it.
  bool self = other == *slot;
  if (self)
    vertex_retain(&other->header);
  ArrayStorage* a = roomFor(slot, (*slot)->count + n, element);
  const ValueWitnessTable* vw = witnesses(element);
  if (isPOD(vw)) {
    copyBytes(elementsOf(a) + a->count * vw->stride, elementsOf(other),
              static_cast<usize>(n) * vw->stride);
  } else {
    for (i64 i = 0; i < n; i++)
      vw->initializeWithCopy(elementsOf(a) + (a->count + i) * vw->stride,
                             elementsOf(other) + i * vw->stride, element);
  }
  a->count += n;
  if (self)
    vertex_release(&other->header);
}

// vertex_array_assign is `a[index] = value`, taking the value.
void vertex_array_assign(ArrayStorage** slot, i64 index, void* value, const Metadata* element) {
  checkIndex(index, (*slot)->count);
  ArrayStorage* a = uniqueArray(slot, (*slot)->count, element);
  const ValueWitnessTable* vw = witnesses(element);
  vw->assignWithTake(elementsOf(a) + index * vw->stride, value, element);
}

// vertex_array_element_for_write is where a[index] is, once the storage is
// the slot's alone: Array's mutable addressor, which `a[i].x = v` and
// `a[i].mutate()` write through.
u8* vertex_array_element_for_write(ArrayStorage** slot, i64 index, const Metadata* element) {
  checkIndex(index, (*slot)->count);
  ArrayStorage* a = uniqueArray(slot, (*slot)->count, element);
  return elementsOf(a) + index * witnesses(element)->stride;
}

// vertex_array_insert takes the value into the array at index, which may
// be the count.
void vertex_array_insert(ArrayStorage** slot, void* value, i64 index, const Metadata* element) {
  if (index < 0 || index > (*slot)->count)
    fatal("Array index is out of range");
  ArrayStorage* a = uniqueArray(slot, (*slot)->count + 1, element);
  const ValueWitnessTable* vw = witnesses(element);
  u8* at = elementsOf(a);
  for (i64 i = a->count; i > index; i--)
    vw->initializeWithTake(at + i * vw->stride, at + (i - 1) * vw->stride, element);
  vw->initializeWithTake(at + index * vw->stride, value, element);
  a->count++;
}

// vertex_array_remove_at moves the element at index into out, closing the
// gap. Swift traps on an index out of range, and so does this.
void vertex_array_remove_at(ArrayStorage** slot, i64 index, void* out, const Metadata* element) {
  checkIndex(index, (*slot)->count);
  ArrayStorage* a = uniqueArray(slot, (*slot)->count, element);
  const ValueWitnessTable* vw = witnesses(element);
  u8* at = elementsOf(a);
  vw->initializeWithTake(out, at + index * vw->stride, element);
  for (i64 i = index; i + 1 < a->count; i++)
    vw->initializeWithTake(at + i * vw->stride, at + (i + 1) * vw->stride, element);
  a->count--;
}

// vertex_array_remove_last moves the last element into out. An empty
// array traps, as Swift's removeLast does.
void vertex_array_remove_last(ArrayStorage** slot, void* out, const Metadata* element) {
  if ((*slot)->count == 0)
    fatal("Can't remove last element from an empty collection");
  vertex_array_remove_at(slot, (*slot)->count - 1, out, element);
}

// vertex_array_pop_last moves the last element into the Optional at out,
// or writes nil for an empty array, as Swift's popLast does.
void vertex_array_pop_last(ArrayStorage** slot, void* out, const OptionalMetadata* result) {
  if ((*slot)->count == 0) {
    writeNone(out, result);
    return;
  }
  vertex_array_remove_at(slot, (*slot)->count - 1, out, result->payload);
  markSome(out, result);
}

// vertex_array_remove_all empties the array, keeping nothing.
void vertex_array_remove_all(ArrayStorage** slot, const Metadata* element) {
  vertex_release(&(*slot)->header);
  ArrayAllocation empty = vertex_array_allocate(0, element);
  *slot = empty.array;
}

// vertex_array_remove_all_keeping empties the array and, where it was
// asked to and the storage is the array's own, keeps the storage for
// what comes next: a buffer refilled each time round a loop.
void vertex_array_remove_all_keeping(ArrayStorage** slot, bool keep, const Metadata* element) {
  ArrayStorage* a = *slot;
  bool own = (a->header.refcount & immortal) == 0 && a->header.refcount == 1;
  if (!keep || !own) {
    vertex_array_remove_all(slot, element);
    return;
  }
  const ValueWitnessTable* vw = witnesses(element);
  if (!isPOD(vw)) {
    u8* at = elementsOf(a);
    for (i64 i = 0; i < a->count; i++)
      vw->destroy(at + i * vw->stride, element);
  }
  a->count = 0;
}

// vertex_array_first and vertex_array_last write the element, or nil, into
// the Optional at out.
void vertex_array_first(ArrayStorage* a, void* out, const OptionalMetadata* result) {
  if (a->count == 0) {
    writeNone(out, result);
    return;
  }
  witnesses(result->payload)->initializeWithCopy(out, elementsOf(a), result->payload);
  markSome(out, result);
}

void vertex_array_last(ArrayStorage* a, void* out, const OptionalMetadata* result) {
  if (a->count == 0) {
    writeNone(out, result);
    return;
  }
  const ValueWitnessTable* vw = witnesses(result->payload);
  vw->initializeWithCopy(out, elementsOf(a) + (a->count - 1) * vw->stride, result->payload);
  markSome(out, result);
}

// vertex_array_equal is `==` on two arrays of an element type the runtime
// compares: the same count, and each element equal to the one beside it.
bool vertex_array_equal(ArrayStorage* a, ArrayStorage* b, const Metadata* element) {
  if (a == b)
    return true;
  if (a->count != b->count)
    return false;
  usize stride = witnesses(element)->stride;
  for (i64 i = 0; i < a->count; i++)
    if (!valuesEqual(elementsOf(a) + i * stride, elementsOf(b) + i * stride, element))
      return false;
  return true;
}

// vertex_array_contains is `contains(_:)` for an element type the runtime
// compares.
bool vertex_array_contains(ArrayStorage* a, const void* value, const Metadata* element) {
  usize stride = witnesses(element)->stride;
  for (i64 i = 0; i < a->count; i++)
    if (valuesEqual(elementsOf(a) + i * stride, value, element))
      return true;
  return false;
}

}

// ---- the hash table ----

namespace vertex {



inline u8* usedOf(HashTable* t) { return reinterpret_cast<u8*>(t) + hashTableUsed; }
inline u8* keyAt(HashTable* t, i64 i) {
  return reinterpret_cast<u8*>(t) + t->keysOffset + static_cast<usize>(i) * witnesses(t->key)->stride;
}
inline u8* valueAt(HashTable* t, i64 i) {
  return reinterpret_cast<u8*>(t) + t->valuesOffset + static_cast<usize>(i) * witnesses(t->value)->stride;
}

void destroyHashTable(HeapObject* object);
static const HeapMetadata hashTableHeapMetadata = {destroyHashTable, nullptr};

inline usize alignTo(usize n, usize align) { return (n + align - 1) & ~(align - 1); }

HashTable* newHashTable(i64 buckets, const Metadata* key, const Metadata* value) {
  const ValueWitnessTable* kw = witnesses(key);
  usize keyAlign = (kw->flags & vwAlignMask) + 1;
  usize keys = alignTo(hashTableUsed + static_cast<usize>(buckets), keyAlign);
  usize end = keys + static_cast<usize>(buckets) * kw->stride;
  usize values = end;
  if (value != nullptr) {
    const ValueWitnessTable* vw = witnesses(value);
    values = alignTo(end, (vw->flags & vwAlignMask) + 1);
    end = values + static_cast<usize>(buckets) * vw->stride;
  }
  auto* t = static_cast<HashTable*>(vertex_pal_alloc(end, 16));
  if (t == nullptr)
    vertex_pal_abort();
  t->header.metadata = &hashTableHeapMetadata;
  t->header.refcount = 1;
  t->count = 0;
  t->buckets = buckets;
  t->key = key;
  t->value = value;
  t->keysOffset = keys;
  t->valuesOffset = values;
  u8* used = usedOf(t);
  for (i64 i = 0; i < buckets; i++)
    used[i] = 0;
  return t;
}

void destroyHashTable(HeapObject* object) {
  auto* t = reinterpret_cast<HashTable*>(object);
  const ValueWitnessTable* kw = witnesses(t->key);
  u8* used = usedOf(t);
  for (i64 i = 0; i < t->buckets; i++) {
    if (!used[i])
      continue;
    if (kw->flags & vwIsNonPOD)
      kw->destroy(keyAt(t, i), t->key);
    if (t->value != nullptr && (witnesses(t->value)->flags & vwIsNonPOD))
      witnesses(t->value)->destroy(valueAt(t, i), t->value);
  }
  vertex_pal_free(t, 0, 16);
}

// find is the bucket holding a key equal to key, or -1; *slot is where
// such a key would go.
inline i64 find(HashTable* t, const void* key, i64* slot) {
  if (t->buckets == 0) {
    *slot = -1;
    return -1;
  }
  u64 mask = static_cast<u64>(t->buckets) - 1;
  i64 i = static_cast<i64>(hashValue(key, t->key) & mask);
  u8* used = usedOf(t);
  while (used[i]) {
    if (valuesEqual(keyAt(t, i), key, t->key)) {
      *slot = i;
      return i;
    }
    i = static_cast<i64>((static_cast<u64>(i) + 1) & mask);
  }
  *slot = i;
  return -1;
}

// uniqueTable makes the table at slot the caller's alone, with room for
// one more entry, rehashing into a fresh table where it is shared, the
// empty singleton, or would be more than three-quarters full.
HashTable* uniqueTable(HashTable** slot, const Metadata* key, const Metadata* value) {
  HashTable* t = *slot;
  bool shared = (t->header.refcount & immortal) != 0 || t->header.refcount != 1;
  bool roomy = (t->count + 1) * 4 <= t->buckets * 3;
  if (!shared && roomy)
    return t;
  i64 buckets = t->buckets < 8 ? 8 : t->buckets;
  while ((t->count + 1) * 4 > buckets * 3)
    buckets *= 2;
  HashTable* fresh = newHashTable(buckets, key, value);
  const ValueWitnessTable* kw = witnesses(key);
  const ValueWitnessTable* vw = value != nullptr ? witnesses(value) : nullptr;
  u8* used = usedOf(t);
  for (i64 i = 0; i < t->buckets; i++) {
    if (!used[i])
      continue;
    i64 at = 0;
    find(fresh, keyAt(t, i), &at);
    usedOf(fresh)[at] = 1;
    if (shared) {
      kw->initializeWithCopy(keyAt(fresh, at), keyAt(t, i), key);
      if (vw)
        vw->initializeWithCopy(valueAt(fresh, at), valueAt(t, i), value);
    } else {
      kw->initializeWithTake(keyAt(fresh, at), keyAt(t, i), key);
      if (vw)
        vw->initializeWithTake(valueAt(fresh, at), valueAt(t, i), value);
    }
    fresh->count++;
  }
  if (shared)
    vertex_release(&t->header);
  else
    vertex_pal_free(t, 0, 16);
  *slot = fresh;
  return fresh;
}

// eraseAt removes the entry in bucket i, moving later entries of the same
// run back so that every key stays reachable from its home bucket.
inline void eraseAt(HashTable* t, i64 i) {
  u64 mask = static_cast<u64>(t->buckets) - 1;
  const ValueWitnessTable* kw = witnesses(t->key);
  const ValueWitnessTable* vw = t->value != nullptr ? witnesses(t->value) : nullptr;
  u8* used = usedOf(t);
  used[i] = 0;
  t->count--;
  i64 hole = i;
  i64 j = static_cast<i64>((static_cast<u64>(i) + 1) & mask);
  while (used[j]) {
    u64 home = hashValue(keyAt(t, j), t->key) & mask;
    // j's entry may fill the hole where its home is not in (hole, j].
    u64 h = static_cast<u64>(hole), jj = static_cast<u64>(j);
    bool between = h <= jj ? (home > h && home <= jj) : (home > h || home <= jj);
    if (!between) {
      kw->initializeWithTake(keyAt(t, hole), keyAt(t, j), t->key);
      if (vw)
        vw->initializeWithTake(valueAt(t, hole), valueAt(t, j), t->value);
      used[hole] = 1;
      used[j] = 0;
      hole = j;
    }
    j = static_cast<i64>((static_cast<u64>(j) + 1) & mask);
  }
}

} // namespace vertex

extern "C" {

// The empty table every empty Dictionary and Set literal shares.
HashTable vertex_empty_hash_table = {
    {nullptr, immortal}, 0, 0, nullptr, nullptr, hashTableUsed, hashTableUsed};

HashTable* vertex_hash_table_empty(void) {
  return &vertex_empty_hash_table;
}

i64 vertex_hash_table_count(HashTable* t) {
  return t->count;
}

bool vertex_hash_table_is_empty(HashTable* t) {
  return t->count == 0;
}

// vertex_hash_table_next is the first bucket in use at or after from, or
// -1 past the last: how a for-in walks a Dictionary or a Set, in bucket
// order, which is why that order changes from run to run as Swift's does.
i64 vertex_hash_table_next(HashTable* t, i64 from) {
  u8* used = usedOf(t);
  for (i64 i = from < 0 ? 0 : from; i < t->buckets; i++)
    if (used[i])
      return i;
  return -1;
}

// vertex_hash_table_key_at copies the key in bucket i into out.
void vertex_hash_table_key_at(HashTable* t, i64 i, void* out, const Metadata* key) {
  witnesses(key)->initializeWithCopy(out, keyAt(t, i), key);
}

// vertex_dictionary_value_at copies the value in bucket i into out.
void vertex_dictionary_value_at(HashTable* t, i64 i, void* out, const Metadata* value) {
  witnesses(value)->initializeWithCopy(out, valueAt(t, i), value);
}

// vertex_dictionary_insert_literal is one entry of a Dictionary literal,
// taking the key and the value. A key already there is Swift's trap:
// a literal may not repeat one.
void vertex_dictionary_insert_literal(HashTable** slot, void* key, void* value,
                                      const Metadata* keyType, const Metadata* valueType) {
  HashTable* t = uniqueTable(slot, keyType, valueType);
  i64 at = 0;
  if (find(t, key, &at) >= 0)
    fatal("Dictionary literal contains duplicate keys");
  usedOf(t)[at] = 1;
  witnesses(keyType)->initializeWithTake(keyAt(t, at), key, keyType);
  witnesses(valueType)->initializeWithTake(valueAt(t, at), value, valueType);
  t->count++;
}

// vertex_dictionary_get is `d[key]`: the value, or nil, into out.
void vertex_dictionary_get(HashTable* t, const void* key, void* out, const OptionalMetadata* result) {
  i64 at = 0;
  if (t->buckets == 0 || find(t, key, &at) < 0) {
    writeNone(out, result);
    return;
  }
  witnesses(result->payload)->initializeWithCopy(out, valueAt(t, at), result->payload);
  markSome(out, result);
}

// vertex_dictionary_get_default is `d[key, default: value]`: the value for
// key, or the default, into out. The call takes the default either way.
void vertex_dictionary_get_default(HashTable* t, const void* key, void* fallback, void* out,
                                   const Metadata* value) {
  const ValueWitnessTable* vw = witnesses(value);
  i64 at = 0;
  if (t->buckets == 0 || find(t, key, &at) < 0) {
    vw->initializeWithTake(out, fallback, value);
    return;
  }
  vw->initializeWithCopy(out, valueAt(t, at), value);
  vw->destroy(fallback, value);
}

// vertex_dictionary_remove moves the value for key, or nil, into out, and
// removes the entry.
void vertex_dictionary_remove(HashTable** slot, const void* key, void* out,
                              const Metadata* keyType, const OptionalMetadata* result) {
  i64 at = 0;
  if ((*slot)->buckets == 0 || find(*slot, key, &at) < 0) {
    writeNone(out, result);
    return;
  }
  HashTable* t = uniqueTable(slot, keyType, result->payload);
  find(t, key, &at);
  const ValueWitnessTable* kw = witnesses(keyType);
  if (kw->flags & vwIsNonPOD)
    kw->destroy(keyAt(t, at), keyType);
  witnesses(result->payload)->initializeWithTake(out, valueAt(t, at), result->payload);
  markSome(out, result);
  eraseAt(t, at);
}

// vertex_dictionary_set is `d[key] = value` with value an Optional, taken:
// nil removes the entry, anything else inserts or replaces it. The key is
// borrowed and copied where a new entry needs it.
void vertex_dictionary_set(HashTable** slot, const void* key, void* value,
                           const Metadata* keyType, const OptionalMetadata* valueType) {
  const Metadata* payload = valueType->payload;
  bool none = true;
  {
    auto* p = static_cast<const u8*>(value);
    u64 got = 0;
    for (u32 i = 0; i < valueType->tagBytes; i++)
      got |= static_cast<u64>(p[valueType->tagOffset + i]) << (8 * i);
    none = got == valueType->none;
  }
  if (none) {
    u8 scratch[64];
    void* out = witnesses(valueType)->size <= sizeof(scratch)
                    ? static_cast<void*>(scratch)
                    : vertex_pal_alloc(witnesses(valueType)->size, 16);
    vertex_dictionary_remove(slot, key, out, keyType, valueType);
    witnesses(valueType)->destroy(out, valueType);
    if (out != scratch)
      vertex_pal_free(out, 0, 16);
    return;
  }
  HashTable* t = uniqueTable(slot, keyType, payload);
  i64 at = 0;
  if (find(t, key, &at) >= 0) {
    witnesses(payload)->assignWithTake(valueAt(t, at), value, payload);
    return;
  }
  usedOf(t)[at] = 1;
  witnesses(keyType)->initializeWithCopy(keyAt(t, at), const_cast<void*>(key), keyType);
  witnesses(payload)->initializeWithTake(valueAt(t, at), value, payload);
  t->count++;
}

// vertex_set_insert adds a copy of element where no equal one is there.
void vertex_set_insert(HashTable** slot, const void* element, const Metadata* type) {
  if ((*slot)->buckets != 0) {
    i64 at = 0;
    if (find(*slot, element, &at) >= 0)
      return;
  }
  HashTable* t = uniqueTable(slot, type, nullptr);
  i64 at = 0;
  find(t, element, &at);
  usedOf(t)[at] = 1;
  witnesses(type)->initializeWithCopy(keyAt(t, at), const_cast<void*>(element), type);
  t->count++;
}

bool vertex_set_contains(HashTable* t, const void* element, const Metadata*) {
  i64 at = 0;
  return t->buckets != 0 && find(t, element, &at) >= 0;
}

// vertex_set_remove moves the equal element, or nil, into out, and
// removes it.
void vertex_set_remove(HashTable** slot, const void* element, void* out, const OptionalMetadata* result) {
  i64 at = 0;
  if ((*slot)->buckets == 0 || find(*slot, element, &at) < 0) {
    writeNone(out, result);
    return;
  }
  HashTable* t = uniqueTable(slot, result->payload, nullptr);
  find(t, element, &at);
  witnesses(result->payload)->initializeWithTake(out, keyAt(t, at), result->payload);
  markSome(out, result);
  eraseAt(t, at);
}

}

extern "C" {

// vertex_values_equal is `==` on two values of one type, lent by address.
bool vertex_values_equal(const void* a, const void* b, const vertex::Metadata* type) {
  return vertex::valuesEqual(a, b, type);
}

// vertex_hasher_init is `Hasher()`: a fresh state under the process seed.
void vertex_hasher_init(vertex::Sip* out) {
  vertex::Seed& k = vertex::seed();
  vertex::sipInit(*out, k.k0, k.k1);
}

// vertex_hasher_combine is `hasher.combine(value)`.
void vertex_hasher_combine(vertex::Sip* hasher, const void* value, const vertex::Metadata* type) {
  vertex::hashInto(*hasher, value, type);
}

// vertex_hasher_finalize is `hasher.finalize()`, which leaves the hasher
// as it was: it finishes a copy.
vertex::i64 vertex_hasher_finalize(const vertex::Sip* hasher) {
  vertex::Sip s = *hasher;
  return static_cast<vertex::i64>(vertex::sipFinish(s));
}

}

// ---- String: what a package reaches for first ----

namespace vertex {

// clusterEnds is where each extended grapheme cluster of the bytes ends,
// written to ends; the count of clusters is answered.
inline usize clusterEnds(const u8* bytes, usize count, usize* ends) {
  if (count == 0)
    return 0;
  usize n = 0;
  usize i = 0;
  GraphemeState state{0, 0, 0};
  GraphemeProperty prev = graphemeProperty(decodeScalar(bytes, count, &i));
  advance(state, prev);
  while (i < count) {
    usize at = i;
    GraphemeProperty next = graphemeProperty(decodeScalar(bytes, count, &i));
    if (breaksBetween(prev, next, state))
      ends[n++] = at;
    advance(state, next);
    prev = next;
  }
  ends[n++] = count;
  return n;
}

// Clusters is a string's bytes and where each of its Characters ends.
struct Clusters {
  const u8* bytes;
  usize* ends;
  usize count;
  u8 scratch[16];
};

inline void clustersOf(Clusters& c, u64 s0, u64 s1) {
  StringBytes b = bytesOf(String{s0, s1}, c.scratch);
  c.bytes = b.bytes;
  c.ends = static_cast<usize*>(vertex_pal_alloc(sizeof(usize) * (b.count + 1), alignof(usize)));
  if (c.ends == nullptr)
    vertex_pal_abort();
  c.count = clusterEnds(b.bytes, b.count, c.ends);
}

inline void freeClusters(Clusters& c) { vertex_pal_free(c.ends, 0, alignof(usize)); }

inline usize clusterStart(const Clusters& c, usize i) { return i == 0 ? 0 : c.ends[i - 1]; }

// sameCharacter is whether cluster i of a and cluster j of b are one
// Character: canonically equivalent.
inline bool sameCharacter(const Clusters& a, usize i, const Clusters& b, usize j) {
  usize as = clusterStart(a, i), an = a.ends[i] - as;
  usize bs = clusterStart(b, j), bn = b.ends[j] - bs;
  if (an == bn && equalBytes(a.bytes + as, b.bytes + bs, an))
    return true;
  return canonicalCompare(a.bytes + as, an, b.bytes + bs, bn) == 0;
}

}  // namespace vertex

extern "C" {

// vertex_string_has_prefix is hasPrefix(_:) and starts(with:): the
// Characters of prefix begin self, compared as Characters are.
bool vertex_string_has_prefix(u64 s0, u64 s1, u64 p0, u64 p1) {
  Clusters s, p;
  clustersOf(s, s0, s1);
  clustersOf(p, p0, p1);
  bool yes = p.count <= s.count;
  for (usize i = 0; yes && i < p.count; i++)
    yes = sameCharacter(s, i, p, i);
  freeClusters(s);
  freeClusters(p);
  return yes;
}

// vertex_string_has_suffix is hasSuffix(_:).
bool vertex_string_has_suffix(u64 s0, u64 s1, u64 p0, u64 p1) {
  Clusters s, p;
  clustersOf(s, s0, s1);
  clustersOf(p, p0, p1);
  bool yes = p.count <= s.count;
  for (usize i = 0; yes && i < p.count; i++)
    yes = sameCharacter(s, s.count - p.count + i, p, i);
  freeClusters(s);
  freeClusters(p);
  return yes;
}

// vertex_string_contains is contains(_:) with a string: the Characters of
// other appear in self as a run, compared as Characters are. Every string
// contains the empty one.
bool vertex_string_contains(u64 s0, u64 s1, u64 o0, u64 o1) {
  Clusters s, o;
  clustersOf(s, s0, s1);
  clustersOf(o, o0, o1);
  bool yes = o.count == 0;
  for (usize at = 0; !yes && at + o.count <= s.count; at++) {
    yes = true;
    for (usize i = 0; yes && i < o.count; i++)
      yes = sameCharacter(s, at + i, o, i);
  }
  freeClusters(s);
  freeClusters(o);
  return yes;
}

String vertex_string_concat(u64 a0, u64 a1, u64 b0, u64 b1);

// vertex_string_append is append(_:): the string in the slot becomes itself
// and then other.
void vertex_string_append(String* slot, u64 b0, u64 b1) {
  String joined = vertex_string_concat(slot->countAndFlags, slot->object, b0, b1);
  vertex_string_release(slot->object);
  *slot = joined;
}

// vertex_string_decoding_utf8 is String(decoding: bytes, as: UTF8.self):
// the bytes as UTF-8, each ill-formed run replaced by U+FFFD.
String vertex_string_decoding_utf8(ArrayStorage* bytes) {
  Text t;
  textInit(t);
  appendRepairedUTF8(t, elementsOf(bytes), static_cast<usize>(bytes->count));
  String s = makeString(t.bytes, t.count);
  textFree(t);
  return s;
}

// vertex_integer_parse is Int(_:) and each integer type's like it over
// text: an optional sign then decimal digits, all of it, in range, or nil.
// Which integer is read from the Optional's payload.
void vertex_integer_parse(u64 s0, u64 s1, void* out, const OptionalMetadata* type) {
  usize width = integerBytes(type->payload);
  bool isSigned = record(type->payload, vertex_metadata_Int) || record(type->payload, vertex_metadata_Int8) ||
                  record(type->payload, vertex_metadata_Int16) || record(type->payload, vertex_metadata_Int32) ||
                  record(type->payload, vertex_metadata_Int64);
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  usize i = 0;
  bool negative = false;
  if (i < b.count && (b.bytes[i] == '+' || b.bytes[i] == '-')) {
    negative = b.bytes[i] == '-';
    i++;
  }
  u64 limit = width == 8 ? ~0ull : (1ull << (8 * width)) - 1;
  if (isSigned)
    limit = (limit >> 1) + (negative ? 1 : 0);
  if (!isSigned && negative)
    limit = 0;
  u64 v = 0;
  bool ok = width > 0 && i < b.count;
  for (; ok && i < b.count; i++) {
    u8 c = b.bytes[i];
    if (c < '0' || c > '9') {
      ok = false;
      break;
    }
    u64 d = c - '0';
    if (d > limit || v > (limit - d) / 10)
      ok = false;
    else
      v = v * 10 + d;
  }
  if (!ok) {
    writeNone(out, type);
    return;
  }
  u64 bits = negative ? 0 - v : v;
  copyBytes(static_cast<u8*>(out), reinterpret_cast<const u8*>(&bits), width);
  markSome(out, type);
}

// vertex_float_parse is Double(_:) and Float(_:) over text.
void vertex_float_parse(u64 s0, u64 s1, void* out, const OptionalMetadata* type) {
  u8 scratch[16];
  StringBytes b = bytesOf(String{s0, s1}, scratch);
  double d = 0;
  // Swift reads no leading space, which strtod would skip.
  bool ok = b.count > 0 && b.bytes[0] > ' ' && vertex_pal_parse_double(b.bytes, b.count, &d);
  if (!ok) {
    writeNone(out, type);
    return;
  }
  if (record(type->payload, vertex_metadata_Float)) {
    float f = static_cast<float>(d);
    copyBytes(static_cast<u8*>(out), reinterpret_cast<const u8*>(&f), 4);
  } else {
    copyBytes(static_cast<u8*>(out), reinterpret_cast<const u8*>(&d), 8);
  }
  markSome(out, type);
}

}

// ---- Casts: `x as? T`, `x as! T`, `x is T` ----

extern "C" {
extern const vertex::FullMetadata vertex_metadata_Any;
HeapObject* vertex_box_allocate(const vertex::Metadata* type);
}

namespace vertex {

// castInto copies the value at src, of type from, into dst as a to, and
// reports whether it is one: what Swift's dynamic cast asks. An existential
// is looked inside, an Optional unwrapped where it holds a value, and a
// value wrapped into an Optional or an Any where that is what is wanted.
bool castInto(void* dst, const void* src, const Metadata* from, const Metadata* to) {
  if (from == to) {
    witnesses(to)->initializeWithCopy(dst, const_cast<void*>(src), to);
    return true;
  }
  if (from->kind == kindExistential) {
    auto* any = static_cast<const AnyExistential*>(src);
    if (any->type == nullptr)
      return false;
    return castInto(dst, anyContents(any), any->type, to);
  }
  if (from->kind == kindOptional) {
    auto* o = static_cast<const OptionalMetadata*>(from);
    if (optionalIsNone(src, o))
      return to->kind == kindOptional && (writeNone(dst, static_cast<const OptionalMetadata*>(to)), true);
    return castInto(dst, src, o->payload, to);
  }
  if (to->kind == kindOptional) {
    auto* o = static_cast<const OptionalMetadata*>(to);
    if (!castInto(dst, src, from, o->payload))
      return false;
    markSome(dst, o);
    return true;
  }
  if (record(to, vertex_metadata_Any)) {
    auto* any = static_cast<AnyExistential*>(dst);
    const ValueWitnessTable* vw = witnesses(from);
    any->type = from;
    if (vw->flags & vwIsNonInline) {
      HeapObject* box = vertex_box_allocate(from);
      any->buffer[0] = reinterpret_cast<u64>(box);
      vw->initializeWithCopy(reinterpret_cast<u8*>(box) + boxValueOffset(vw->flags), const_cast<void*>(src), from);
    } else {
      vw->initializeWithCopy(any->buffer, const_cast<void*>(src), from);
    }
    return true;
  }
  return false;
}

}  // namespace vertex

extern "C" {

// vertex_dynamic_cast is castInto, answered as 1 or 0.
u64 vertex_dynamic_cast(void* dst, const void* src, const vertex::Metadata* from, const vertex::Metadata* to) {
  return vertex::castInto(dst, src, from, to) ? 1 : 0;
}

}
