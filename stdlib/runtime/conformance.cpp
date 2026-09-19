// Conformances found at run time: which witness table, if any, says that
// a type conforms to a protocol, asked of the type's metadata.
//
// Every conformance a module declares is listed in its image's
// conformance records (see vsc/lower's ConformanceSection): a relative
// pointer to the conformance's descriptor, which points at the protocol's
// descriptor, the type's, and the table. Swift keeps the same in
// __swift5_proto and asks it the same question; this is that, under a
// name of Vertex's own.
#include "vertex/abi.h"
#include "vertex/platform.h"

namespace vertex {

// A protocol's descriptor: Swift's layout, of which only the address is
// read here. The standard library's are defined below, under the names
// swiftc gives them, since conformances to them point at those names.
struct ProtocolDescriptor {
  u32 flags;
  i32 parent;
  i32 name;
  u32 numRequirementsInSignature;
  u32 numRequirements;
  i32 associatedTypeNames;
};

// A conformance's descriptor, as vsc/lower writes it.
struct ConformanceDescriptor {
  i32 protocol;   // relative; the low bit says it is to a pointer to it
  i32 type;       // relative, to the type's descriptor
  i32 table;      // relative, to the witness table
  u32 flags;
};

}  // namespace vertex

extern "C" {
// Kind 3 is a protocol. Linked with Swift's runtime, libswiftCore defines
// these under the same names, and those are the ones.
#ifndef VERTEX_WITH_SWIFT_RUNTIME
extern const vertex::ProtocolDescriptor vertex_protocol_Error __asm("_$ss5ErrorMp");
const vertex::ProtocolDescriptor vertex_protocol_Error = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_Equatable __asm("_$sSQMp");
const vertex::ProtocolDescriptor vertex_protocol_Equatable = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_Comparable __asm("_$sSLMp");
const vertex::ProtocolDescriptor vertex_protocol_Comparable = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_Hashable __asm("_$sSHMp");
const vertex::ProtocolDescriptor vertex_protocol_Hashable = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_CustomStringConvertible
    __asm("_$ss23CustomStringConvertibleMp");
const vertex::ProtocolDescriptor vertex_protocol_CustomStringConvertible = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_Sequence __asm("_$sSTMp");
const vertex::ProtocolDescriptor vertex_protocol_Sequence = {3, 0, 0, 0, 0, 0};
extern const vertex::ProtocolDescriptor vertex_protocol_IteratorProtocol __asm("_$sStMp");
const vertex::ProtocolDescriptor vertex_protocol_IteratorProtocol = {3, 0, 0, 0, 0, 0};

#else
extern const vertex::ProtocolDescriptor vertex_protocol_Error __asm("_$ss5ErrorMp");
extern const vertex::ProtocolDescriptor vertex_protocol_Equatable __asm("_$sSQMp");
extern const vertex::ProtocolDescriptor vertex_protocol_Comparable __asm("_$sSLMp");
extern const vertex::ProtocolDescriptor vertex_protocol_Hashable __asm("_$sSHMp");
extern const vertex::ProtocolDescriptor vertex_protocol_CustomStringConvertible
    __asm("_$ss23CustomStringConvertibleMp");
#endif

// Calls a witness whose only argument is the conformer, by address, in the
// self register: a property's getter. What it answers comes back in the
// first two registers. Assembly, since nothing C++ loads x20; see
// stdlib.TaskAsm.
vertex::String vertex_witness_call(void (*fn)(), const void* self, const vertex::Metadata* type,
                                   const void* const* table);

// Calls a witness taking one argument besides the conformer: hash(into:),
// which is handed the hasher's address.
void vertex_witness_call1(void (*fn)(), const void* self, void* arg, const vertex::Metadata* type,
                          const void* const* table);

// Calls a static operator's witness: both operands by address, and the
// conformer's type in the self register. Its Bool comes back in w0.
bool vertex_witness_call_static2(void (*fn)(), const void* a, const void* b, const vertex::Metadata* type,
                                 const void* const* table);
}

namespace vertex {

// relativeTo is where a relative field points, reading through the
// pointer it names where its low bit says the target is indirect.
inline const void* relativeTo(const i32* field, bool mayBeIndirect) {
  i32 v = *field;
  if (v == 0)
    return nullptr;
  bool indirect = mayBeIndirect && (v & 1);
  if (indirect)
    v &= ~1;
  const void* at = static_cast<const char*>(static_cast<const void*>(field)) + v;
  if (indirect)
    return *static_cast<const void* const*>(at);
  return at;
}

// The images' record lists, gathered once. An image loaded afterwards is
// not looked in.
struct RecordList {
  const i32* at;
  usize count;
};

inline constexpr u32 maxRecordLists = 32;

struct Records {
  RecordList lists[maxRecordLists];
  u32 count;
  bool ready;
};

inline Records& records() {
  static Records r = {};
  if (!r.ready) {
    u32 images = vertex_pal_image_count();
    for (u32 i = 0; i < images && r.count < maxRecordLists; i++) {
      usize bytes = 0;
      const i32* at = vertex_pal_conformance_records(i, &bytes);
      if (at != nullptr && bytes >= sizeof(i32))
        r.lists[r.count++] = RecordList{at, bytes / sizeof(i32)};
    }
    r.ready = true;
  }
  return r;
}

// typeDescriptorOf is the nominal descriptor a type's metadata points at,
// for the kinds that have one.
inline const void* typeDescriptorOf(const Metadata* type) {
  switch (type->kind) {
  case kindStruct:
  case kindEnum:
  case kindClass:
    return reinterpret_cast<const StructMetadata*>(type)->description;
  }
  return nullptr;
}

// A small cache of answers, misses included, keyed by type and protocol.
// Tasks run on several threads, so the cache is under a spinlock: an
// entry is three words, and a reader must not see one thread's type
// beside another's table.
struct CachedConformance {
  const Metadata* type;
  const void* protocol;
  const void* const* table;
};

inline constexpr usize conformanceCacheSize = 64;

static u32 conformanceLock;

inline void lockConformances() {
  while (__builtin_atomic_cas(&conformanceLock, 0u, 1u) != 0u) {
  }
}

inline void unlockConformances() { __builtin_atomic_store(&conformanceLock, 0u); }

inline CachedConformance& cachedConformance(const Metadata* type, const void* protocol) {
  static CachedConformance cache[conformanceCacheSize] = {};
  usize h = (reinterpret_cast<usize>(type) >> 3) ^ (reinterpret_cast<usize>(protocol) >> 5);
  return cache[h % conformanceCacheSize];
}

// warmConformances gathers the images' records before any worker thread
// exists, so that the once-only gathering is never raced.
inline void warmConformances() { records(); }

}  // namespace vertex

extern "C" {

// vertex_conformance is the witness table by which type conforms to the
// protocol with this descriptor, or null where it does not.
const void* const* vertex_conformance(const vertex::Metadata* type, const void* protocol) {
  using namespace vertex;
  if (type == nullptr || protocol == nullptr)
    return nullptr;
  const void* want = typeDescriptorOf(type);
  if (want == nullptr)
    return nullptr;
  lockConformances();
  CachedConformance& c = cachedConformance(type, protocol);
  if (c.type == type && c.protocol == protocol) {
    const void* const* table = c.table;
    unlockConformances();
    return table;
  }
  unlockConformances();
  const void* const* found = nullptr;
  Records& r = records();
  for (u32 l = 0; l < r.count && found == nullptr; l++) {
    for (usize i = 0; i < r.lists[l].count; i++) {
      auto* d = static_cast<const ConformanceDescriptor*>(relativeTo(r.lists[l].at + i, false));
      if (d == nullptr || relativeTo(&d->protocol, true) != protocol || relativeTo(&d->type, false) != want)
        continue;
      found = static_cast<const void* const*>(relativeTo(&d->table, false));
      break;
    }
  }
  lockConformances();
  c = CachedConformance{type, protocol, found};
  unlockConformances();
  return found;
}

}
