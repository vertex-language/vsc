// Array storage: allocation, and what compiled code reads out of it.
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "mem.h"

extern "C" {
void vertex_release(vertex::HeapObject* obj);
extern const vertex::FullMetadata vertex_metadata_Int;
}

namespace vertex {

// Ending an array's storage: every element through its own destroy
// witness, then the memory.
void destroyArray(HeapObject* object) {
  auto* array = reinterpret_cast<ArrayStorage*>(object);
  const ValueWitnessTable* vw = witnesses(array->element);
  if (vw->flags & vwIsNonPOD) {
    auto* at = reinterpret_cast<u8*>(array) + arrayStorageElements;
    for (i64 i = 0; i < array->count; i++)
      vw->destroy(at + i * vw->stride, array->element);
  }
  vertex_pal_free(array, 0, 16);
}

static const HeapMetadata arrayHeapMetadata = {destroyArray, nullptr};

} // namespace vertex

using namespace vertex;

extern "C" {

// The empty array: one immortal storage every empty array literal
// shares. Its element type is immaterial, since it has no elements.
ArrayStorage vertex_empty_array = {
    {nullptr, immortal}, 0, 0, &vertex_metadata_Int.metadata};

// The storage vertex_array_allocate hands back, and where its elements
// go. Two words, the pair an array literal destructures.
struct ArrayAllocation {
  ArrayStorage* array;
  u8*           elements;
};

// vertex_array_allocate makes storage for count elements of a type,
// counted as holding them already: the caller initializes every one
// before anything reads the array.
ArrayAllocation vertex_array_allocate(i64 count, const Metadata* element) {
  if (count == 0)
    return {&vertex_empty_array, reinterpret_cast<u8*>(&vertex_empty_array) + arrayStorageElements};
  if (count < 0)
    fatal("Can't construct Array with count < 0");
  usize stride = witnesses(element)->stride;
  auto* array = static_cast<ArrayStorage*>(
      vertex_pal_alloc(arrayStorageElements + static_cast<usize>(count) * stride, 16));
  if (array == nullptr)
    vertex_pal_abort();
  array->header.metadata = &arrayHeapMetadata;
  array->header.refcount = 1;
  array->count = count;
  array->capacity = count;
  array->element = element;
  return {array, reinterpret_cast<u8*>(array) + arrayStorageElements};
}

i64 vertex_array_count(ArrayStorage* array, const Metadata*) {
  return array->count;
}

bool vertex_array_is_empty(ArrayStorage* array, const Metadata*) {
  return array->count == 0;
}

// vertex_array_element is the address of one element, after the bounds
// check Swift's subscript makes: an index outside the array traps.
u8* vertex_array_element(i64 index, ArrayStorage* array, const Metadata* element) {
  if (index < 0 || index >= array->count)
    fatal("Index out of range");
  return reinterpret_cast<u8*>(array) + arrayStorageElements +
         static_cast<usize>(index) * witnesses(element)->stride;
}

// vertex_array_repeating makes an array of count copies of a value, which
// it takes: copied into every element but the last, moved into the last,
// and let go of where there are none.
ArrayStorage* vertex_array_repeating(i64 count, u8* value, const Metadata* element) {
  const ValueWitnessTable* vw = witnesses(element);
  ArrayAllocation got = vertex_array_allocate(count, element);
  if (count == 0) {
    vw->destroy(value, element);
    return got.array;
  }
  if (isPOD(vw)) {
    // A byte repeated is a fill; anything wider is one copy of the value
    // and then the run so far doubled until it is all there.
    usize stride = vw->stride;
    if (stride == 1) {
      fillBytes(got.elements, *value, static_cast<usize>(count));
    } else {
      usize total = static_cast<usize>(count) * stride;
      copyBytes(got.elements, value, stride);
      usize have = stride;
      while (have < total) {
        usize chunk = have < total - have ? have : total - have;
        copyBytes(got.elements + have, got.elements, chunk);
        have += chunk;
      }
    }
    return got.array;
  }
  for (i64 i = 0; i < count - 1; i++)
    vw->initializeWithCopy(got.elements + static_cast<usize>(i) * vw->stride, value, element);
  vw->initializeWithTake(got.elements + static_cast<usize>(count - 1) * vw->stride, value, element);
  return got.array;
}

}
