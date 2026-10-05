// The metadata the runtime owns: the types the core declares, whose
// records compiled code refers to by symbol rather than builds itself.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"

extern "C" {
void vertex_string_retain(vertex::u64 object);
void vertex_string_release(vertex::u64 object);
void vertex_retain(vertex::HeapObject* obj);
void vertex_release(vertex::HeapObject* obj);
}

namespace vertex {

// ---- the witnesses of a value that owns nothing ----

static void* trivialCopy(void* dest, void* src, const Metadata* type) {
  copyBytes(dest, src, witnesses(type)->size);
  return dest;
}

static void trivialDestroy(void*, const Metadata*) {}

// The enum-tag rows trap, as vsc/lower's own do: nothing reaches them
// until an Optional of an opened existential is formed, and a wrong tag
// written silently is worse than a trap that says where.
static u32 noEnumTag(const void*, u32, const Metadata*) {
  __builtin_trap();
  return 0;
}

static void noStoreEnumTag(void*, u32, u32, const Metadata*) {
  __builtin_trap();
}

#define VERTEX_TRIVIAL_WITNESSES(name, bytes)                                         \
  static const ValueWitnessTable name = {                                             \
      trivialCopy, trivialDestroy, trivialCopy, trivialCopy, trivialCopy, trivialCopy, \
      noEnumTag,   noStoreEnumTag, bytes,       bytes,       (bytes) - 1, 0};

VERTEX_TRIVIAL_WITNESSES(trivial1, 1)
VERTEX_TRIVIAL_WITNESSES(trivial2, 2)
VERTEX_TRIVIAL_WITNESSES(trivial4, 4)
VERTEX_TRIVIAL_WITNESSES(trivial8, 8)

// The empty tuple, Void: no bytes, a stride of one, as Swift lays it out.
static const ValueWitnessTable trivial0 = {
    trivialCopy, trivialDestroy, trivialCopy, trivialCopy, trivialCopy, trivialCopy,
    noEnumTag,   noStoreEnumTag, 0,           1,           0,           0};

// ---- String ----

static void* stringCopy(void* dest, void* src, const Metadata*) {
  auto* s = static_cast<String*>(src);
  vertex_string_retain(s->object);
  copyBytes(dest, src, sizeof(String));
  return dest;
}

static void stringDestroy(void* value, const Metadata*) {
  vertex_string_release(static_cast<String*>(value)->object);
}

static void* stringAssignCopy(void* dest, void* src, const Metadata*) {
  auto* d = static_cast<String*>(dest);
  auto* s = static_cast<String*>(src);
  vertex_string_retain(s->object);
  u64 old = d->object;
  copyBytes(dest, src, sizeof(String));
  vertex_string_release(old);
  return dest;
}

static void* stringAssignTake(void* dest, void* src, const Metadata*) {
  u64 old = static_cast<String*>(dest)->object;
  copyBytes(dest, src, sizeof(String));
  vertex_string_release(old);
  return dest;
}

static const ValueWitnessTable stringWitnesses = {
    stringCopy, stringDestroy,  stringCopy,     stringAssignCopy, trivialCopy, stringAssignTake,
    noEnumTag,  noStoreEnumTag, sizeof(String), sizeof(String),   7 | vwIsNonPOD, 0};

// ---- Any: whatever is inside, through its own witnesses ----

static void* anyCopy(void* dest, void* src, const Metadata*) {
  auto* d = static_cast<AnyExistential*>(dest);
  auto* s = static_cast<AnyExistential*>(src);
  const ValueWitnessTable* vw = witnesses(s->type);
  d->type = s->type;
  if (vw->flags & vwIsNonInline) {
    vertex_retain(reinterpret_cast<HeapObject*>(s->buffer[0]));
    d->buffer[0] = s->buffer[0];
  } else {
    vw->initializeWithCopy(d->buffer, s->buffer, s->type);
  }
  return dest;
}

static void anyDestroy(void* value, const Metadata*) {
  auto* any = static_cast<AnyExistential*>(value);
  const ValueWitnessTable* vw = witnesses(any->type);
  if (vw->flags & vwIsNonInline)
    vertex_release(reinterpret_cast<HeapObject*>(any->buffer[0]));
  else
    vw->destroy(any->buffer, any->type);
}

static void* anyAssignCopy(void* dest, void* src, const Metadata* type) {
  anyDestroy(dest, type);
  return anyCopy(dest, src, type);
}

static void* anyAssignTake(void* dest, void* src, const Metadata* type) {
  anyDestroy(dest, type);
  copyBytes(dest, src, sizeof(AnyExistential));
  return dest;
}

static const ValueWitnessTable anyWitnesses = {
    anyCopy,   anyDestroy,     anyCopy,
    anyAssignCopy, trivialCopy, anyAssignTake,
    noEnumTag, noStoreEnumTag, sizeof(AnyExistential),
    sizeof(AnyExistential), 7 | vwIsNonPOD, 0};

// ---- A protocol existential: Any's four words, then one witness table ----

struct TableExistential {
  u64             buffer[3];
  const Metadata* type;
  const void*     table;
};

static void* existential1Copy(void* dest, void* src, const Metadata* m) {
  anyCopy(dest, src, m);
  static_cast<TableExistential*>(dest)->table = static_cast<TableExistential*>(src)->table;
  return dest;
}

static void* existential1AssignCopy(void* dest, void* src, const Metadata* type) {
  anyDestroy(dest, type);
  return existential1Copy(dest, src, type);
}

static void* existential1AssignTake(void* dest, void* src, const Metadata* type) {
  anyDestroy(dest, type);
  copyBytes(dest, src, sizeof(TableExistential));
  return dest;
}

static const ValueWitnessTable existential1Witnesses = {
    existential1Copy,       anyDestroy,     existential1Copy,
    existential1AssignCopy, trivialCopy,    existential1AssignTake,
    noEnumTag,              noStoreEnumTag, sizeof(TableExistential),
    sizeof(TableExistential), 7 | vwIsNonPOD, 0};

// ---- Functions: a code pointer, then the context that owns captures ----

struct Function {
  void*       code;
  HeapObject* context;
};

static void* functionCopy(void* dest, void* src, const Metadata*) {
  vertex_retain(static_cast<Function*>(src)->context);
  copyBytes(dest, src, sizeof(Function));
  return dest;
}

static void functionDestroy(void* value, const Metadata*) {
  vertex_release(static_cast<Function*>(value)->context);
}

static void* functionAssignCopy(void* dest, void* src, const Metadata*) {
  vertex_retain(static_cast<Function*>(src)->context);
  HeapObject* old = static_cast<Function*>(dest)->context;
  copyBytes(dest, src, sizeof(Function));
  vertex_release(old);
  return dest;
}

static void* functionAssignTake(void* dest, void* src, const Metadata*) {
  HeapObject* old = static_cast<Function*>(dest)->context;
  copyBytes(dest, src, sizeof(Function));
  vertex_release(old);
  return dest;
}

static const ValueWitnessTable functionWitnesses = {
    functionCopy, functionDestroy, functionCopy,     functionAssignCopy, trivialCopy,
    functionAssignTake, noEnumTag, noStoreEnumTag,   sizeof(Function),   sizeof(Function),
    7 | vwIsNonPOD, 0};

} // namespace vertex

using namespace vertex;

// The records. A pointer to a type's metadata is eight bytes into its
// record, past the witness table pointer, which is what vsc's
// MetadataGlobal adds.
extern "C" {
extern const FullMetadata vertex_metadata_Bool   = {&trivial1, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Int8   = {&trivial1, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_UInt8  = {&trivial1, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Int16  = {&trivial2, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_UInt16 = {&trivial2, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Int32  = {&trivial4, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_UInt32 = {&trivial4, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Float  = {&trivial4, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Float16  = {&trivial2, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_BFloat16 = {&trivial2, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Int    = {&trivial8, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_UInt   = {&trivial8, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Int64  = {&trivial8, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_UInt64 = {&trivial8, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Double = {&trivial8, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_String = {&stringWitnesses, {kindStruct, nullptr}};
extern const FullMetadata vertex_metadata_Any    = {&anyWitnesses, {kindExistential, nullptr}};
// Void is a tuple of nothing: the word after the kind is its count, zero.
extern const FullMetadata vertex_metadata_Void   = {&trivial0, {kindTuple, nullptr}};
// Every existential of one protocol is laid out alike, so one record serves
// them all: what is inside says the rest.
extern const FullMetadata vertex_metadata_Existential1 = {&existential1Witnesses, {kindExistential, nullptr}};
extern const FullMetadata vertex_metadata_Function = {&functionWitnesses, {kindFunction, nullptr}};
// Every metatype is one word, the metadata of the type it names, so one
// record serves them all: the value says which type.
extern const FullMetadata vertex_metadata_Metatype = {&trivial8, {kindMetatype, nullptr}};
// Every pointer: one uncounted word.
extern const FullMetadata vertex_metadata_Pointer = {&trivial8, {kindStruct, nullptr}};
}

// vertex_existential_type is `type(of: x)` for an existential x: the type
// of what it holds, and for a class instance the object's own dynamic
// type rather than the one it was boxed as. Every existential starts with
// the three-word buffer and the metadata word; see AnyExistential.
extern "C" const vertex::Metadata* vertex_existential_type(const void* container) {
  using namespace vertex;
  auto* any = static_cast<const AnyExistential*>(container);
  const Metadata* type = any->type;
  if (type != nullptr && type->kind == kindClass) {
    auto* object = reinterpret_cast<const HeapObject*>(any->buffer[0]);
    if (object != nullptr && object->metadata != nullptr && object->metadata->type != nullptr)
      return object->metadata->type;
  }
  return type;
}
