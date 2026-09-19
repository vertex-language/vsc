// Heap objects: allocation, and the two reference-count operations
// ownership becomes once the compiler has lowered it away.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"

namespace vertex {

// fatal ends the program as a Swift precondition does: what was printed
// so far is flushed, the reason goes to standard error, and the process
// traps.
[[noreturn]] inline void fatal(const char* message) {
  vertex_pal_flush();
  static const char prefix[] = "Fatal error: ";
  vertex_pal_write(2, reinterpret_cast<const u8*>(prefix), sizeof(prefix) - 1);
  usize n = 0;
  while (message[n] != 0)
    n++;
  vertex_pal_write(2, reinterpret_cast<const u8*>(message), n);
  vertex_pal_write(2, reinterpret_cast<const u8*>("\n"), 1);
  __builtin_trap();
}

}  // namespace vertex

using namespace vertex;

extern "C" {

// vertex_fatal is where a failed check the compiler inserted goes: a nil
// unwrapped, an overflow, a range out of order.
[[noreturn]] void vertex_fatal(const char* message) { fatal(message); }

// vertex_alloc makes an instance with room for size bytes of stored
// properties after the header, holding one reference: the caller's.
HeapObject* vertex_alloc(u64 size) {
  auto* obj = static_cast<HeapObject*>(vertex_pal_alloc(sizeof(HeapObject) + size, 16));
  if (obj == nullptr)
    vertex_pal_abort();
  obj->metadata = nullptr;
  obj->refcount = 1;
  // Stored properties start as zero. An initializer's first write to a
  // property is lowered as an assignment, which releases what was there
  // before -- so what was there has to be null, which release ignores.
  fillBytes(reinterpret_cast<u8*>(obj + 1), 0, static_cast<usize>(size));
  return obj;
}

// vertex_dealloc returns an object's memory, once what it owned has been
// released. A class's destroyer ends with it.
void vertex_dealloc(HeapObject* obj) {
  vertex_pal_free(obj, 0, 16);
}

// A box's end: its value through the value's own witnesses, then the
// memory.
static void destroyBox(HeapObject* obj) {
  const Metadata* type = obj->metadata->type;
  auto* vwt = reinterpret_cast<const ValueWitnessTable* const*>(type)[-1];
  vwt->destroy(reinterpret_cast<u8*>(obj) + boxValueOffset(vwt->flags), type);
  vertex_pal_free(obj, 0, 16);
}

// vertex_box_allocate makes a box for one value of a type, holding one
// reference, with the value not yet written. The caller writes it at
// boxValueOffset of the type's flags.
HeapObject* vertex_box_allocate(const Metadata* type) {
  auto* vwt = reinterpret_cast<const ValueWitnessTable* const*>(type)[-1];
  usize value = boxValueOffset(vwt->flags);
  usize own = (value + vwt->size + 7) & ~static_cast<usize>(7);
  auto* obj = static_cast<HeapObject*>(vertex_pal_alloc(own + sizeof(HeapMetadata), 16));
  if (obj == nullptr)
    vertex_pal_abort();
  auto* meta = reinterpret_cast<HeapMetadata*>(reinterpret_cast<u8*>(obj) + own);
  meta->destroy = destroyBox;
  meta->type = type;
  obj->metadata = meta;
  obj->refcount = 1;
  return obj;
}

// vertex_retain adds a reference. Null is no reference, and retaining
// or releasing it does nothing, as Swift's own do.
void vertex_retain(HeapObject* obj) {
  if (obj == nullptr || (obj->refcount & immortal))
    return;
  __builtin_atomic_add(&obj->refcount, 1ull);
}

// vertex_release drops a reference, and ends the object when it was the
// last. The immortal bit is read without a barrier: it is set before an
// object is shared and never cleared, so every thread that can see the
// object sees the bit.
void vertex_release(HeapObject* obj) {
  if (obj == nullptr || (obj->refcount & immortal))
    return;
  // The count as it was: one means this call took it to zero.
  if (__builtin_atomic_sub(&obj->refcount, 1ull) != 1)
    return;
  if (obj->metadata != nullptr && obj->metadata->destroy != nullptr) {
    // Immortal while it ends: a deinit that retains and releases self, or
    // hands it somewhere for the length of a call, must not take the
    // count back to zero and end it a second time.
    obj->refcount = immortal;
    obj->metadata->destroy(obj);
    return;
  }
  vertex_pal_free(obj, 0, 16);
}

// vertex_is_unique reports whether the caller holds the only reference,
// which is the question copy-on-write asks before it writes. An immortal
// object is never unique: it may not be written to.
bool vertex_is_unique(HeapObject* obj) {
  return __builtin_atomic_load(&obj->refcount) == 1;
}

}
