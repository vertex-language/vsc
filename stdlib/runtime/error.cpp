// Errors: the box a thrown value travels in.
//
// A thrown value is an `any Error`: a three-word buffer, the value's
// metadata, and its Error witness table. It is moved into a heap object of
// its own, so that one pointer -- the error register -- carries it out of
// the function that threw, and whoever catches it copies it back out and
// releases the box.
#include "vertex/abi.h"
#include "vertex/platform.h"

namespace vertex {

// What the box holds: the existential, as a protocol existential is laid
// out, one word wider than Any for the witness table.
struct ErrorExistential {
  u64             buffer[3];
  const Metadata* type;
  const void*     conformance;
};

// A box's end: what the existential holds, through its own witnesses --
// or its own box, for a value too wide for the buffer -- then the memory.
static void destroyErrorBox(HeapObject* obj) {
  auto* e = reinterpret_cast<ErrorExistential*>(obj + 1);
  const ValueWitnessTable* vw = witnesses(e->type);
  if (vw->flags & vwIsNonInline)
    vertex_release(reinterpret_cast<HeapObject*>(e->buffer[0]));
  else
    vw->destroy(e->buffer, e->type);
  vertex_dealloc(obj);
}

static const HeapMetadata errorBoxMetadata = {destroyErrorBox, nullptr};

void* errorValue(HeapObject* obj);

// fatalError ends the program over the error in a box: what was printed is
// flushed, and the reason is written, then the error as String(reflecting:)
// writes it.
[[noreturn]] inline void fatalError(HeapObject* obj, const char* reason) {
  auto* e = reinterpret_cast<ErrorExistential*>(obj + 1);
  Text t;
  textInit(t);
  textString(t, reason);
  debugDescribe(t, errorValue(obj), e->type);
  textByte(t, 0);
  fatal(reinterpret_cast<const char*>(t.bytes));
}

// errorValue is where a box's error value is. See vertex_error_project.
inline void* errorValue(HeapObject* obj) {
  auto* e = reinterpret_cast<ErrorExistential*>(obj + 1);
  const ValueWitnessTable* vw = witnesses(e->type);
  if (vw->flags & vwIsNonInline)
    return reinterpret_cast<u8*>(e->buffer[0]) + boxValueOffset(vw->flags);
  return reinterpret_cast<void*>(e->buffer);
}

} // namespace vertex

using namespace vertex;

extern "C" {

// vertex_error_box moves an initialized `any Error` into a new box holding
// one reference. The caller's container is uninitialized afterwards.
HeapObject* vertex_error_box(void* existential) {
  HeapObject* obj = vertex_alloc(sizeof(ErrorExistential));
  auto* to = reinterpret_cast<ErrorExistential*>(obj + 1);
  auto* from = static_cast<ErrorExistential*>(existential);
  *to = *from;
  obj->metadata = &errorBoxMetadata;
  return obj;
}

// vertex_error_contents is where a box's `any Error` is, for a catch to
// copy it out of.
void* vertex_error_contents(HeapObject* obj) {
  return reinterpret_cast<void*>(obj + 1);
}

// vertex_error_matches is 1 when a box's error is a value of exactly this
// type, which is what `catch is T` and `catch let e as T` ask.
u64 vertex_error_matches(HeapObject* obj, const Metadata* type) {
  return reinterpret_cast<ErrorExistential*>(obj + 1)->type == type ? 1 : 0;
}

// vertex_error_project is where a box's error value is: in the
// existential's buffer, or past the header of the box the buffer points
// at, for a value too wide for it.
void* vertex_error_project(HeapObject* obj) {
  auto* e = reinterpret_cast<ErrorExistential*>(obj + 1);
  const ValueWitnessTable* vw = witnesses(e->type);
  if (vw->flags & vwIsNonInline)
    return reinterpret_cast<u8*>(e->buffer[0]) + boxValueOffset(vw->flags);
  return reinterpret_cast<void*>(e->buffer);
}

// vertex_error_in_main is where an error a throwing main let out goes:
// what was printed is flushed, the error is written as String(reflecting:)
// writes it, and the program traps -- Swift's "Error raised at top level".
[[noreturn]] void vertex_error_in_main(HeapObject* obj) {
  fatalError(obj, "Error raised at top level: ");
}

// vertex_try_failed is where an error a `try!` did not expect goes: it is
// reported as Swift reports it, and the program traps.
[[noreturn]] void vertex_try_failed(HeapObject* obj) {
  fatalError(obj, "'try!' expression unexpectedly raised an error: ");
}

// vertex_existential_is is 1 when the existential at `at` holds a value of
// exactly this type: what `x as? T` asks of one that is not in a box.
u64 vertex_existential_is(const ErrorExistential* at, const Metadata* type) {
  return at->type == type ? 1 : 0;
}

// vertex_existential_project is where the existential at `at` keeps its
// value: in its buffer, or past the header of the box the buffer holds.
void* vertex_existential_project(ErrorExistential* at) {
  const ValueWitnessTable* vw = witnesses(at->type);
  if (vw->flags & vwIsNonInline)
    return reinterpret_cast<u8*>(at->buffer[0]) + boxValueOffset(vw->flags);
  return reinterpret_cast<void*>(at->buffer);
}

}
