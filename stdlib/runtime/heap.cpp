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

// The count word, beside the immortal bit: the strong count in the low 32
// bits; the weak count -- weak and unowned references both -- above it;
// dead once the strong count has reached zero and the object is ending or
// ended, when a weak reference reads nil and an unowned one traps; and
// deallocated once its destroyer is done with the memory, which is freed
// then or, while weak references remain, by the last of them.
inline constexpr u64 strongMask = 0xffffffffull;
inline constexpr u64 weakUnit = 1ull << 32;
inline constexpr u64 weakMask = ((1ull << 29) - 1) << 32;
inline constexpr u64 deallocated = 1ull << 61;
inline constexpr u64 dead = 1ull << 62;

// orCount sets bits in an object's count, answering what it was.
inline u64 orCount(HeapObject* obj, u64 bits) {
  u64 was = __builtin_atomic_load(&obj->refcount);
  for (;;) {
    u64 seen = __builtin_atomic_cas(&obj->refcount, was, was | bits);
    if (seen == was)
      return was;
    was = seen;
  }
}

// counted reports whether an object's references are counted: it is
// there, and not a literal's immortal storage. One that is ending is
// immortal too, but dead, and its weak references are still counted.
inline bool counted(HeapObject* obj) {
  if (obj == nullptr)
    return false;
  u64 c = __builtin_atomic_load(&obj->refcount);
  return (c & immortal) == 0 || (c & dead) != 0;
}

// strongFromWeak is a strong reference to what a weak or unowned one
// names, or null where the object has ended or is ending.
inline HeapObject* strongFromWeak(HeapObject* obj) {
  if (obj == nullptr)
    return nullptr;
  u64 was = __builtin_atomic_load(&obj->refcount);
  for (;;) {
    if ((was & immortal) != 0)
      return (was & dead) != 0 ? nullptr : obj;
    if ((was & strongMask) == 0)
      return nullptr;
    u64 seen = __builtin_atomic_cas(&obj->refcount, was, was + 1);
    if (seen == was)
      return obj;
    was = seen;
  }
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
  // While weak references remain, the last of them frees it.
  u64 was = orCount(obj, deallocated);
  if ((was & weakMask) == 0)
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
  if ((__builtin_atomic_sub(&obj->refcount, 1ull) & strongMask) != 1)
    return;
  // Immortal while it ends: a deinit that retains and releases self, or
  // hands it somewhere for the length of a call, must not take the count
  // back to zero and end it a second time. Dead, so that a weak reference
  // to it reads nil from here on.
  orCount(obj, immortal | dead);
  if (obj->metadata != nullptr && obj->metadata->destroy != nullptr) {
    obj->metadata->destroy(obj);
    return;
  }
  vertex_dealloc(obj);
}

// vertex_weak_retain adds a weak or unowned reference to an object,
// which keeps its memory, and not the object, alive.
void vertex_weak_retain(HeapObject* obj) {
  if (!counted(obj))
    return;
  __builtin_atomic_add(&obj->refcount, weakUnit);
}

// vertex_weak_release drops a weak or unowned reference, and frees the
// object's memory where it was the last reference of any kind.
void vertex_weak_release(HeapObject* obj) {
  if (!counted(obj))
    return;
  u64 was = __builtin_atomic_sub(&obj->refcount, weakUnit);
  if ((was & weakMask) == weakUnit && (was & deallocated) != 0)
    vertex_pal_free(obj, 0, 16);
}

// vertex_weak_load reads a weak reference held at slot: a strong
// reference to its object, which the caller owns, or null where the
// object has ended.
HeapObject* vertex_weak_load(HeapObject* const* slot) {
  return strongFromWeak(*slot);
}

// vertex_unowned_load reads an unowned reference held at slot: a strong
// reference to its object, which the caller owns. The object outlives it
// or the program is wrong, and it traps, as Swift's does.
HeapObject* vertex_unowned_load(HeapObject* const* slot) {
  HeapObject* obj = *slot;
  HeapObject* strong = strongFromWeak(obj);
  if (obj != nullptr && strong == nullptr)
    fatal("Attempted to read an unowned reference but the object was already deallocated");
  return strong;
}

// A weak cell: an object of its own holding one weak or unowned reference,
// which is how a closure's capture list holds `[weak self]` -- the closure
// holds the cell as it holds anything else, strongly, and the cell's end
// is its reference's.
struct WeakCell {
  HeapObject header;
  HeapObject* held;
};

static void destroyWeakCell(HeapObject* obj) {
  vertex_weak_release(reinterpret_cast<WeakCell*>(obj)->held);
  vertex_pal_free(obj, 0, 16);
}

static const HeapMetadata weakCellMetadata = {destroyWeakCell, nullptr};

// vertex_weak_cell makes a cell holding a weak reference to obj, which
// the caller lends; the caller owns the cell.
HeapObject* vertex_weak_cell(HeapObject* obj) {
  auto* cell = static_cast<WeakCell*>(vertex_pal_alloc(sizeof(WeakCell), 16));
  if (cell == nullptr)
    vertex_pal_abort();
  cell->header.metadata = &weakCellMetadata;
  cell->header.refcount = 1;
  cell->held = obj;
  vertex_weak_retain(obj);
  return &cell->header;
}

// vertex_weak_cell_load is a strong reference to what a cell holds, which
// the caller owns, or null where it has ended; vertex_unowned_cell_load
// traps there instead.
HeapObject* vertex_weak_cell_load(HeapObject* cell) {
  return vertex_weak_load(&reinterpret_cast<WeakCell*>(cell)->held);
}

HeapObject* vertex_unowned_cell_load(HeapObject* cell) {
  return vertex_unowned_load(&reinterpret_cast<WeakCell*>(cell)->held);
}

// vertex_weak_assign puts obj, borrowed from the caller, in a weak or
// unowned reference held at slot, letting go of the one there.
void vertex_weak_assign(HeapObject** slot, HeapObject* obj) {
  vertex_weak_retain(obj);
  HeapObject* old = *slot;
  *slot = obj;
  vertex_weak_release(old);
}

// A weak existential of a class-bound protocol -- `weak var delegate:
// Delegate?` -- is an existential's words whose buffer's first holds a
// weak reference; the type and the witness tables beside it are plain
// words. A null type is nil. size is the container's bytes.

// vertex_weak_existential_load writes into out the existential held
// weakly at slot, holding a strong reference the caller owns, or nil
// where there is none or its object has ended.
void vertex_weak_existential_load(const u64* slot, u64* out, i64 size) {
  i64 words = size / 8;
  HeapObject* strong = slot[3] ? strongFromWeak(reinterpret_cast<HeapObject*>(slot[0])) : nullptr;
  if (strong == nullptr) {
    for (i64 i = 0; i < words; i++)
      out[i] = 0;
    return;
  }
  out[0] = reinterpret_cast<u64>(strong);
  out[1] = 0;
  out[2] = 0;
  for (i64 i = 3; i < words; i++)
    out[i] = slot[i];
}

// vertex_weak_existential_assign puts the existential at value, borrowed
// from the caller, in the weak existential at slot, letting go of the
// reference there.
void vertex_weak_existential_assign(u64* slot, const u64* value, i64 size) {
  i64 words = size / 8;
  HeapObject* obj = value[3] ? reinterpret_cast<HeapObject*>(value[0]) : nullptr;
  vertex_weak_retain(obj);
  HeapObject* old = slot[3] ? reinterpret_cast<HeapObject*>(slot[0]) : nullptr;
  slot[0] = reinterpret_cast<u64>(obj);
  slot[1] = 0;
  slot[2] = 0;
  for (i64 i = 3; i < words; i++)
    slot[i] = obj ? value[i] : 0;
  vertex_weak_release(old);
}

// vertex_is_unique reports whether the caller holds the only reference,
// which is the question copy-on-write asks before it writes. An immortal
// object is never unique: it may not be written to.
bool vertex_is_unique(HeapObject* obj) {
  return (__builtin_atomic_load(&obj->refcount) & (immortal | strongMask)) == 1;
}

// vertex_is_uniquely_referenced is isKnownUniquelyReferenced(&x): the
// same question, of the reference held at slot.
bool vertex_is_uniquely_referenced(HeapObject* const* slot) {
  return *slot != nullptr && vertex_is_unique(*slot);
}

}
