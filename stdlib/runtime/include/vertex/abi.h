// The contract between compiled Vertex code and the runtime.
//
// Every layout here is also written down in vsc/lower, in Go, and the
// two are held together by a test that asks vcx for the layout of each
// struct in this file and compares. Change one without the other and
// that test fails, rather than a program misreading its own objects.
//
// The shapes of metadata, value witness tables and existentials are
// Swift's, because they are good ones and vsc already emits them. The
// symbols, the String and Array representations and the object header
// are Vertex's. See ABI.md.
#pragma once

namespace vertex {

typedef unsigned char      u8;
typedef unsigned short     u16;
typedef unsigned int       u32;
typedef unsigned long long u64;
typedef signed char        i8;
typedef short              i16;
typedef int                i32;
typedef long long          i64;
typedef u64                usize;

struct Metadata;
struct FullMetadata;
struct HeapMetadata;

// ---- heap objects ----

// What a heap object is: a header, then its stored properties.
//
//   +0  metadata   what the object is, or null for an object whose
//                  deallocation is only its memory
//   +8  refcount   live references, with the immortal bit on top
//
// Two words, which is stdlib.HeaderBytes and what lower computes a
// property's address from.
struct HeapObject {
  const HeapMetadata*        metadata;
  u64                        refcount;
};

// What a heap object's metadata word points at, where it points at
// anything: how the object ends. destroy releases what the object owns
// and frees its memory; null is an object whose ending is only its
// memory. A class's dispatch table is one of these, with its methods
// in the rows after destroy.
struct HeapMetadata {
  void (*destroy)(HeapObject* object);
  // The object's dynamic type, for a class instance: what describing it
  // names. Null for storage that is not an instance of a type.
  const Metadata* type;
};

// ---- async contexts ----

// What an async function has instead of a stack frame.
//
// A suspended function has given its thread back, so it has no stack to
// keep anything on. What it has is this: a chain of contexts, one per
// async call, each holding what that call needs to survive a suspension.
//
//   +0  parent        the caller's context
//   +8  resumeParent  where to go when this function returns
//   +16 ...           this function's own frame, laid out by the compiler
//
// The layout is Swift's, checked against what swiftc emits: its async
// functions read their frame at context + 16, and return by loading the
// second word and tail calling it. See docs/vertex_swift_async.md.
//
// Returning is that tail call. An async function does not return a value
// in a register to whoever called it -- it calls its caller's
// continuation with the value as an argument, which is how a function
// that may have been resumed on another thread gets its answer home.
struct AsyncContext {
  AsyncContext* parent;
  void          (*resumeParent)();
};

// The frame begins after the two header words.
inline constexpr u64 asyncContextHeaderBytes = 16;

// The record beside an async function, which is what an async function
// value names rather than the code itself: whoever calls one has to size
// the frame before it can call it at all, and only this says how big.
//
//	+0  function  the distance from here to the code
//	+4  size      the context that code wants
//
// The layout is Swift's -- it is the `Tu` global beside every async
// function swiftc emits. See vsc's lower/asyncsplit.go.
struct AsyncFunctionPointer {
  i32 function;
  u32 size;
};

// code is what an async function pointer names.
inline void (*asyncCode(const AsyncFunctionPointer* fp))() {
  auto at = reinterpret_cast<const u8*>(fp) + static_cast<i64>(fp->function);
  return reinterpret_cast<void (*)()>(const_cast<u8*>(at));
}

// An immortal object is never counted and never freed: a literal's
// storage, an empty collection's singleton. The top bit is one a real
// count cannot reach.
inline constexpr u64 immortal = 1ull << 63;

// ---- value witnesses and type metadata ----

// What is done to a value whose type is known only at run time. Eight
// functions, then size, stride, flags and the count of spare
// representations -- Swift's layout, so the offsets lower reads are
// Swift's: flags at 0x50, destroy at 0x8.
struct ValueWitnessTable {
  void* (*initializeBufferWithCopyOfBuffer)(void* dest, void* src, const Metadata* type);
  void  (*destroy)(void* value, const Metadata* type);
  void* (*initializeWithCopy)(void* dest, void* src, const Metadata* type);
  void* (*assignWithCopy)(void* dest, void* src, const Metadata* type);
  void* (*initializeWithTake)(void* dest, void* src, const Metadata* type);
  void* (*assignWithTake)(void* dest, void* src, const Metadata* type);
  u32   (*getEnumTagSinglePayload)(const void* value, u32 emptyCases, const Metadata* type);
  void  (*storeEnumTagSinglePayload)(void* value, u32 whichCase, u32 emptyCases, const Metadata* type);
  usize size;
  usize stride;
  u32   flags;
  u32   extraInhabitantCount;
};

// The flags word: the alignment mask in the low byte, and these.
inline constexpr u32 vwAlignMask   = 0xff;
inline constexpr u32 vwIsNonPOD    = 1u << 16;
inline constexpr u32 vwIsNonInline = 1u << 17;

// The kinds a metadata record's first word says.
inline constexpr usize kindStruct      = 0x200;
inline constexpr usize kindEnum        = 0x201;
inline constexpr usize kindOptional    = 0x202;
inline constexpr usize kindTuple       = 0x301;
inline constexpr usize kindFunction    = 0x302;
inline constexpr usize kindExistential = 0x303;
// A metatype: the value is a pointer to the metadata of the type it is.
inline constexpr usize kindMetatype    = 0x304;
// Vertex's own kinds, in a range Swift's metadata does not use.
inline constexpr usize kindArray       = 0x800;
inline constexpr usize kindClass       = 0x801;
inline constexpr usize kindDictionary  = 0x802;
inline constexpr usize kindSet         = 0x803;

// A type's metadata. A pointer to one points here, at the kind; the
// value witness table is the word before it.
struct Metadata {
  usize kind;
};

// The descriptor a nominal type's metadata points at: what it is called.
// Relative pointers, as vsc/lower emits them.
struct NominalTypeDescriptor {
  u32 flags;
  i32 parent;
  i32 name;
  i32 accessor;
  i32 fields;
  u32 numFields;
  u32 fieldOffsetVectorOffset;
};

struct StructMetadata : Metadata {
  const NominalTypeDescriptor* description;
  // u32 field offsets follow, one per stored property, at +16.
};

inline constexpr usize structFieldOffsets = 16;

// A class's metadata: its descriptor, then its dispatch table -- how a
// metatype reaches its class methods -- and its superclass's metadata, or
// null for a root class or one whose superclass another module declares.
struct ClassMetadata : Metadata {
  const NominalTypeDescriptor* description;
  const void* table;
  const ClassMetadata* superclass;
};

// What a struct's fields are called and what their types are: the
// descriptor's `fields` points here. A count, then a name and a metadata
// record per field. Vertex's layout rather than Swift's reflection
// records, which name types by mangled string and need a demangler to
// read.
struct FieldRecord {
  const char*               name;
  const FullMetadata*        type;
};

struct FieldDescriptor {
  u64 count;
  // FieldRecord records[count] follows.
};

// A module's descriptor: the name is the third field.
struct ModuleDescriptor {
  u32 flags;
  i32 parent;
  i32 name;
};

// An Optional: what it wraps, and where and how the empty case is
// written. A tagged Optional has a byte after its payload, 0 for a value
// and 1 for none; one whose payload has a spare representation is the
// payload's bytes, with none written as that representation -- a null
// word for a reference, 2 in a Bool's byte.
struct OptionalMetadata : Metadata {
  const Metadata* payload;
  u32             tagOffset;
  u32             tagBytes;
  u64             none;
};

// A tuple, as Swift lays its metadata out: how many elements, their
// labels as one string -- each label followed by a space, an unlabelled
// element an empty one, null where none is labelled -- and, per element,
// its type and where it sits.
struct TupleMetadata : Metadata {
  usize       numElements;
  const char* labels;
  struct Element {
    const Metadata* type;
    usize           offset;
  } elements[1];
};

// An Array: what it holds.
struct ArrayMetadata : Metadata {
  const Metadata* element;
};

// A Dictionary: its keys and its values.
struct DictionaryMetadata : Metadata {
  const Metadata* key;
  const Metadata* value;
};

// A Set: what it holds.
struct SetMetadata : Metadata {
  const Metadata* element;
};

// A record as it sits in memory: the witness table, then the metadata
// a pointer to the type points at.
struct FullMetadata {
  const ValueWitnessTable* vwt;
  StructMetadata           metadata;
};

inline const ValueWitnessTable* witnesses(const Metadata* type) {
  return reinterpret_cast<const ValueWitnessTable* const*>(type)[-1];
}

// A relative pointer's target: the field's own address plus what it
// holds, or null for a field that holds zero.
inline const void* relative(const i32* field) {
  if (*field == 0)
    return nullptr;
  return static_cast<const char*>(static_cast<const void*>(field)) + *field;
}

inline const char* relativeString(const i32* field) {
  return static_cast<const char*>(relative(field));
}

// A box: a heap object holding one value too wide for an existential's
// buffer, whose pointer the buffer's first word holds. The value is past
// the header at the value's own alignment -- where Swift puts it, so an
// existential opens the same way whichever of the two made its box --
// and after the value is a HeapMetadata of the box's own, which the
// header's metadata word points at: how to end the value, and its type.
inline usize boxValueOffset(u32 flags) {
  usize mask = flags & vwAlignMask;
  return (sizeof(HeapObject) + mask) & ~mask;
}

// `Any`: three words of inline buffer and the metadata of what is in it.
struct AnyExistential {
  u64             buffer[3];
  const Metadata* type;
};

// ---- String ----

// A String is two words, and which of three forms it is lives in the
// top byte of the second:
//
//   0x00        native: the word is a StringStorage*, reference counted;
//               the first word is the count and flags
//   0x40        literal: the low 56 bits address immortal bytes that
//               need no header; the first word is the count and flags
//   0xE0 | n    small: n <= 15 bytes, the first eight in the first word
//               and the rest in the low seven bytes of this one
//
// The first word of a native or literal string is the count in the low
// 48 bits and flags above it.
struct String {
  u64 countAndFlags;
  u64 object;
};

inline constexpr u64 stringTagMask     = 0xFFull << 56;
inline constexpr u64 stringTagLiteral  = 0x40ull << 56;
inline constexpr u64 stringTagSmall    = 0xE0ull << 56;
inline constexpr u64 stringPointerMask = (1ull << 56) - 1;
inline constexpr u64 stringCountMask   = (1ull << 48) - 1;
inline constexpr u64 stringFlagASCII   = 1ull << 63;
inline constexpr usize stringSmallCapacity = 15;

// The storage a native String points at: a heap object, its capacity,
// then its bytes.
struct StringStorage {
  HeapObject header;
  u64        capacity;
  // u8 bytes[capacity] follows, at +24.
};

inline constexpr usize stringStorageBytes = 24;

// ---- Array ----

// An Array is one word: a pointer to its storage. The empty array is an
// immortal singleton, so an array is never null.
struct ArrayStorage {
  HeapObject      header;
  i64             count;
  i64             capacity;
  const Metadata* element;
  // the elements follow, at +40, each a stride of the element type
};

inline constexpr usize arrayStorageElements = 40;

// ---- Dictionary and Set ----

// A Dictionary or a Set is one word: a HashTable*, never null. Open
// addressing with linear probing and deletion by backward shift, so there
// are no tombstones; a power-of-two bucket count at most three-quarters
// full. A Set is a table whose value type is null. The empty table is one
// immortal global.
struct HashTable {
  HeapObject      header;
  i64             count;
  i64             buckets;
  const Metadata* key;
  const Metadata* value;
  u64             keysOffset;
  u64             valuesOffset;
  // u8 used[buckets] follows at +64, then the keys, then the values.
};

inline constexpr usize hashTableUsed = 64;

} // namespace vertex
