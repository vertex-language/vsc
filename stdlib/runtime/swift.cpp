// The bridge to Swift: converting a String or an Array between Vertex's
// representation and the one libswiftCore uses, at a call into a module
// swiftc compiled.
//
// It is its own translation unit because it names libswiftCore, and a
// program that never calls into Swift must link without it. build links
// this object, and the libswiftCore stub, only for a program that
// imports a module swiftc built.
//
// Every conversion copies. A Vertex String handed to Swift becomes a
// Swift String with its own storage, released after the call; a Swift
// String handed back becomes a Vertex one, and the Swift one is released
// here. Neither side ever holds the other's storage.
#include "vertex/abi.h"

using namespace vertex;

extern "C" {
String vertex_string_from_utf8(const u8* bytes, u64 count);
const u8* vertex_string_utf8(u64 countAndFlags, u64 object, u8* scratch, u64* count);

struct VertexArrayAllocation {
  ArrayStorage* array;
  u8*           elements;
};
VertexArrayAllocation vertex_array_allocate(i64 count, const Metadata* element);

extern const FullMetadata vertex_metadata_Bool, vertex_metadata_Int8, vertex_metadata_UInt8,
    vertex_metadata_Int16, vertex_metadata_UInt16, vertex_metadata_Int32, vertex_metadata_UInt32,
    vertex_metadata_Float, vertex_metadata_Int, vertex_metadata_UInt, vertex_metadata_Int64,
    vertex_metadata_UInt64, vertex_metadata_Double;

void swift_release(void* object);
void swift_bridgeObjectRelease(void* object);
}

namespace vertex {

// The two words of a Swift String, whose meaning is libswiftCore's.
struct SwiftString {
  u64 countAndFlags;
  u64 object;
};

struct SwiftMetadataResponse {
  const void* metadata;
  u64         state;
};

struct SwiftArrayAllocation {
  void* array;
  u8*   elements;
};

// String._uncheckedFromUTF8(_: UnsafeBufferPointer<UInt8>) -> String:
// a String with storage of its own, copied from the bytes.
SwiftString swiftStringFromUTF8(const u8* start, i64 count)
    __asm("_$sSS18_uncheckedFromUTF8ySSSRys5UInt8VGFZ");

// String.utf8CString: the bytes and a NUL, as a ContiguousArray<CChar>.
void* swiftUTF8CString(u64 countAndFlags, u64 object)
    __asm("_$sSS11utf8CStrings15ContiguousArrayVys4Int8VGvg");

// _allocateUninitializedArray<T>(_: Builtin.Word) -> ([T], Builtin.RawPointer)
SwiftArrayAllocation swiftAllocateArray(i64 count, const void* elementMetadata)
    __asm("_$ss27_allocateUninitializedArrayySayxG_BptBwlF");

SwiftMetadataResponse swiftMetadataBool(u64) __asm("_$sSbMa");
SwiftMetadataResponse swiftMetadataInt8(u64) __asm("_$ss4Int8VMa");
SwiftMetadataResponse swiftMetadataUInt8(u64) __asm("_$ss5UInt8VMa");
SwiftMetadataResponse swiftMetadataInt16(u64) __asm("_$ss5Int16VMa");
SwiftMetadataResponse swiftMetadataUInt16(u64) __asm("_$ss6UInt16VMa");
SwiftMetadataResponse swiftMetadataInt32(u64) __asm("_$ss5Int32VMa");
SwiftMetadataResponse swiftMetadataUInt32(u64) __asm("_$ss6UInt32VMa");
SwiftMetadataResponse swiftMetadataFloat(u64) __asm("_$sSfMa");
SwiftMetadataResponse swiftMetadataInt(u64) __asm("_$sSiMa");
SwiftMetadataResponse swiftMetadataUInt(u64) __asm("_$sSuMa");
SwiftMetadataResponse swiftMetadataInt64(u64) __asm("_$ss5Int64VMa");
SwiftMetadataResponse swiftMetadataUInt64(u64) __asm("_$ss6UInt64VMa");
SwiftMetadataResponse swiftMetadataDouble(u64) __asm("_$sSdMa");

// Swift's contiguous array storage: a heap object, the count, the
// capacity and flags, then the elements.
inline constexpr usize swiftArrayCount = 16;
inline constexpr usize swiftArrayElements = 32;

inline bool same(const Metadata* type, const FullMetadata& record) {
  return static_cast<const void*>(type) == static_cast<const void*>(&record.metadata);
}

// swiftMetadata is Swift's metadata for the element type Vertex's
// metadata names. Only the types whose values are the same bytes in
// both languages can be copied across, and those are the ones here.
const void* swiftMetadata(const Metadata* type) {
  if (same(type, vertex_metadata_Bool)) return swiftMetadataBool(0).metadata;
  if (same(type, vertex_metadata_Int8)) return swiftMetadataInt8(0).metadata;
  if (same(type, vertex_metadata_UInt8)) return swiftMetadataUInt8(0).metadata;
  if (same(type, vertex_metadata_Int16)) return swiftMetadataInt16(0).metadata;
  if (same(type, vertex_metadata_UInt16)) return swiftMetadataUInt16(0).metadata;
  if (same(type, vertex_metadata_Int32)) return swiftMetadataInt32(0).metadata;
  if (same(type, vertex_metadata_UInt32)) return swiftMetadataUInt32(0).metadata;
  if (same(type, vertex_metadata_Float)) return swiftMetadataFloat(0).metadata;
  if (same(type, vertex_metadata_Int)) return swiftMetadataInt(0).metadata;
  if (same(type, vertex_metadata_UInt)) return swiftMetadataUInt(0).metadata;
  if (same(type, vertex_metadata_Int64)) return swiftMetadataInt64(0).metadata;
  if (same(type, vertex_metadata_UInt64)) return swiftMetadataUInt64(0).metadata;
  if (same(type, vertex_metadata_Double)) return swiftMetadataDouble(0).metadata;
  __builtin_trap();
  return nullptr;
}

inline void copy(u8* dest, const u8* src, usize n) {
  for (usize i = 0; i < n; i++)
    dest[i] = src[i];
}

} // namespace vertex

extern "C" {

SwiftString vertex_swift_string_to_swift(u64 countAndFlags, u64 object) {
  u8 scratch[16];
  u64 count = 0;
  const u8* bytes = vertex_string_utf8(countAndFlags, object, scratch, &count);
  return swiftStringFromUTF8(bytes, static_cast<i64>(count));
}

String vertex_swift_string_from_swift(u64 countAndFlags, u64 object) {
  void* array = swiftUTF8CString(countAndFlags, object);
  auto* at = static_cast<u8*>(array);
  i64 count = *reinterpret_cast<i64*>(at + swiftArrayCount) - 1; // less the NUL
  String s = vertex_string_from_utf8(at + swiftArrayElements, static_cast<u64>(count));
  swift_release(array);
  return s;
}

void vertex_swift_string_release(u64 object) {
  swift_bridgeObjectRelease(reinterpret_cast<void*>(object));
}

// An Array of a type whose values are the same bytes in both languages.
void* vertex_swift_array_to_swift(ArrayStorage* array) {
  SwiftArrayAllocation got = swiftAllocateArray(array->count, swiftMetadata(array->element));
  copy(got.elements, reinterpret_cast<u8*>(array) + arrayStorageElements,
       static_cast<usize>(array->count) * witnesses(array->element)->stride);
  return got.array;
}

ArrayStorage* vertex_swift_array_from_swift(void* swiftArray, const Metadata* element) {
  auto* at = static_cast<u8*>(swiftArray);
  i64 count = *reinterpret_cast<i64*>(at + swiftArrayCount);
  VertexArrayAllocation got = vertex_array_allocate(count, element);
  copy(got.elements, at + swiftArrayElements,
       static_cast<usize>(count) * witnesses(element)->stride);
  swift_release(swiftArray);
  return got.array;
}

void vertex_swift_release(void* object) {
  swift_release(object);
}

}
