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

// ---- An existential of protocols: Any's four words, then a table each ----

static void existentialDestroyContents(AnyExistential* any) {
  const ValueWitnessTable* vw = witnesses(any->type);
  if (vw->flags & vwIsNonInline)
    vertex_release(reinterpret_cast<HeapObject*>(any->buffer[0]));
  else
    vw->destroy(any->buffer, any->type);
}

void* vertex_vw_existential_copy(void* dest, void* src, const Metadata* type) {
  auto* d = static_cast<AnyExistential*>(dest);
  auto* s = static_cast<AnyExistential*>(src);
  const ValueWitnessTable* vw = witnesses(s->type);
  if (vw->flags & vwIsNonInline) {
    vertex_retain(reinterpret_cast<HeapObject*>(s->buffer[0]));
    d->buffer[0] = s->buffer[0];
  } else {
    vw->initializeWithCopy(d->buffer, s->buffer, s->type);
  }
  d->type = s->type;
  auto* dt = reinterpret_cast<const void**>(d + 1);
  auto* st = reinterpret_cast<const void* const*>(s + 1);
  for (u64 i = 0; i < existentialTables(type); i++)
    dt[i] = st[i];
  return dest;
}

void vertex_vw_existential_destroy(void* value, const Metadata*) {
  existentialDestroyContents(static_cast<AnyExistential*>(value));
}

void* vertex_vw_existential_assign_copy(void* dest, void* src, const Metadata* type) {
  existentialDestroyContents(static_cast<AnyExistential*>(dest));
  return vertex_vw_existential_copy(dest, src, type);
}

void* vertex_vw_existential_assign_take(void* dest, void* src, const Metadata* type) {
  existentialDestroyContents(static_cast<AnyExistential*>(dest));
  copyBytes(dest, src, sizeof(AnyExistential) + existentialTables(type) * sizeof(void*));
  return dest;
}

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
