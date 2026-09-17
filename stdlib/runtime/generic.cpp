// Value witnesses shared by every instance of a generic type: the
// functions a compiled record for `Int32?` or `[String]` points its rows
// at. What differs between instances is read out of the metadata each
// witness is handed, and the size and flags the record itself carries.
#include "vertex/abi.h"
#include "mem.h"

extern "C" {
void vertex_retain(vertex::HeapObject* obj);
void vertex_release(vertex::HeapObject* obj);
}

namespace vertex {

inline bool optionalIsNone(const void* value, const OptionalMetadata* type) {
  auto* at = static_cast<const u8*>(value) + type->tagOffset;
  u64 got = 0;
  for (u32 i = 0; i < type->tagBytes; i++)
    got |= static_cast<u64>(at[i]) << (8 * i);
  return got == type->none;
}

inline usize sizeOf(const Metadata* type) {
  return witnesses(type)->size;
}

} // namespace vertex

using namespace vertex;

extern "C" {

// ---- Optional ----

void* vertex_vw_optional_copy(void* dest, void* src, const Metadata* type) {
  auto* o = static_cast<const OptionalMetadata*>(type);
  copyBytes(dest, src, sizeOf(type));
  if (!optionalIsNone(src, o))
    witnesses(o->payload)->initializeWithCopy(dest, src, o->payload);
  return dest;
}

void vertex_vw_optional_destroy(void* value, const Metadata* type) {
  auto* o = static_cast<const OptionalMetadata*>(type);
  if (!optionalIsNone(value, o))
    witnesses(o->payload)->destroy(value, o->payload);
}

void* vertex_vw_optional_assign_copy(void* dest, void* src, const Metadata* type) {
  if (dest == src)
    return dest;
  vertex_vw_optional_destroy(dest, type);
  return vertex_vw_optional_copy(dest, src, type);
}

void* vertex_vw_optional_assign_take(void* dest, void* src, const Metadata* type) {
  vertex_vw_optional_destroy(dest, type);
  copyBytes(dest, src, sizeOf(type));
  return dest;
}

// ---- one reference: an Array, a class instance ----

void* vertex_vw_reference_copy(void* dest, void* src, const Metadata*) {
  auto* obj = *static_cast<HeapObject**>(src);
  vertex_retain(obj);
  *static_cast<HeapObject**>(dest) = obj;
  return dest;
}

void vertex_vw_reference_destroy(void* value, const Metadata*) {
  vertex_release(*static_cast<HeapObject**>(value));
}

void* vertex_vw_reference_assign_copy(void* dest, void* src, const Metadata*) {
  auto* obj = *static_cast<HeapObject**>(src);
  vertex_retain(obj);
  auto* old = *static_cast<HeapObject**>(dest);
  *static_cast<HeapObject**>(dest) = obj;
  vertex_release(old);
  return dest;
}

void* vertex_vw_reference_assign_take(void* dest, void* src, const Metadata*) {
  auto* old = *static_cast<HeapObject**>(dest);
  *static_cast<HeapObject**>(dest) = *static_cast<HeapObject**>(src);
  vertex_release(old);
  return dest;
}

// ---- moving bytes, and the enum-tag rows no one reaches ----

void* vertex_vw_take(void* dest, void* src, const Metadata* type) {
  copyBytes(dest, src, sizeOf(type));
  return dest;
}

vertex::u32 vertex_vw_get_enum_tag(const void*, vertex::u32, const Metadata*) {
  __builtin_trap();
  return 0;
}

void vertex_vw_store_enum_tag(void*, vertex::u32, vertex::u32, const Metadata*) {
  __builtin_trap();
}

}
