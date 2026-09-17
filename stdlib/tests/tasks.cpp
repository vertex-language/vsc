// The task allocator exercised directly: the bump pointer an async
// function's frames come from.
//
// Frames are allocated and freed in the order a call chain enters and
// leaves, so the allocator only has to be a pointer that moves forward
// and comes back. What it must not do is hand out overlapping memory,
// lose a slab when the chain gets deep, or forget where a frame started.
//
// Prints nothing when every check holds; build/runtime_test.go runs it.
#include "runtime.h"
#include "platform.h"

using namespace vertex;

extern "C" {
void* vertex_task_alloc(u64 size);
void  vertex_task_dealloc(void* frame);
}

namespace {

u64 failures = 0;
Text report;

void check(bool ok, const char* what, i64 n) {
  if (ok)
    return;
  failures++;
  textString(report, what);
  textByte(report, ' ');
  textSigned(report, n);
  textByte(report, '\n');
}

usize addr(void* p) { return reinterpret_cast<usize>(p); }

// fill writes a known byte through a whole allocation, so that anything
// overlapping it is caught by the reader rather than by luck.
void fill(void* p, u64 size, u8 b) {
  auto* q = static_cast<u8*>(p);
  for (u64 i = 0; i < size; i++)
    q[i] = b;
}

bool holds(void* p, u64 size, u8 b) {
  auto* q = static_cast<u8*>(p);
  for (u64 i = 0; i < size; i++) {
    if (q[i] != b)
      return false;
  }
  return true;
}

// Every frame is aligned enough for anything a frame holds.
void alignment() {
  for (u64 size = 1; size <= 256; size *= 2) {
    void* p = vertex_task_alloc(size);
    check(p != nullptr, "alloc returned nothing at size", static_cast<i64>(size));
    check(addr(p) % 16 == 0, "frame is not 16-byte aligned at size", static_cast<i64>(size));
    vertex_task_dealloc(p);
  }
}

// Two live frames do not overlap, whatever is written through them.
void distinct() {
  void* a = vertex_task_alloc(64);
  void* b = vertex_task_alloc(64);
  check(a != b, "two live frames are the same address", 0);
  fill(a, 64, 0xA1);
  fill(b, 64, 0xB2);
  check(holds(a, 64, 0xA1), "the first frame was written through the second", 0);
  check(holds(b, 64, 0xB2), "the second frame was written through the first", 0);
  vertex_task_dealloc(b);
  vertex_task_dealloc(a);
}

// Freeing the last frame hands the same memory back: this is the whole
// reason a bump pointer is enough, and a call that returns and calls
// again reuses one frame rather than growing.
void reuse() {
  void* a = vertex_task_alloc(96);
  vertex_task_dealloc(a);
  void* b = vertex_task_alloc(96);
  check(a == b, "a freed frame was not handed back", 0);
  vertex_task_dealloc(b);
}

// Freeing a frame frees everything allocated after it, which is what a
// return does to the calls it made.
void stackDiscipline() {
  void* outer = vertex_task_alloc(32);
  void* inner = vertex_task_alloc(32);
  check(inner != outer, "nested frames are the same address", 0);
  // Back to the outer frame: the inner one goes with it.
  vertex_task_dealloc(outer);
  void* again = vertex_task_alloc(32);
  check(again == outer, "freeing a frame did not free the ones after it", 0);
  vertex_task_dealloc(again);
}

// A chain deeper than one slab keeps going, and unwinds back to where it
// started. The first slab is 4 KiB, so a thousand frames of 64 bytes
// crosses several.
void deepChain() {
  constexpr int depth = 1000;
  void* frames[depth];
  void* first = nullptr;
  for (int i = 0; i < depth; i++) {
    frames[i] = vertex_task_alloc(64);
    if (i == 0)
      first = frames[i];
    check(frames[i] != nullptr, "a deep chain ran out of memory at depth", i);
    fill(frames[i], 64, static_cast<u8>(i));
  }
  // Every frame still holds what was written to it: no slab was reused
  // while its frames were live.
  for (int i = 0; i < depth; i++)
    check(holds(frames[i], 64, static_cast<u8>(i)), "a live frame was overwritten at depth", i);
  // Unwind, and the allocator comes back to where it began.
  for (int i = depth - 1; i >= 0; i--)
    vertex_task_dealloc(frames[i]);
  void* again = vertex_task_alloc(64);
  check(again == first, "unwinding a deep chain did not come back to the start", 0);
  vertex_task_dealloc(again);
}

// Freeing the outermost frame of a deep chain frees the slabs under it
// in one go, which is what a task finishing does.
void unwindAtOnce() {
  void* first = vertex_task_alloc(64);
  for (int i = 0; i < 500; i++)
    (void)vertex_task_alloc(64);
  vertex_task_dealloc(first);
  void* again = vertex_task_alloc(64);
  check(again == first, "freeing the outermost frame left the slabs behind", 0);
  vertex_task_dealloc(again);
}

// A frame larger than a slab gets a slab of its own rather than failing.
void oversized() {
  constexpr u64 big = 64 * 1024;
  void* p = vertex_task_alloc(big);
  check(p != nullptr, "an oversized frame returned nothing", static_cast<i64>(big));
  fill(p, big, 0x5C);
  check(holds(p, big, 0x5C), "an oversized frame is not all there", 0);
  void* after = vertex_task_alloc(64);
  check(after != nullptr, "allocating after an oversized frame failed", 0);
  check(holds(p, big, 0x5C), "allocating after an oversized frame overwrote it", 0);
  vertex_task_dealloc(after);
  vertex_task_dealloc(p);
}

// Freeing nothing is nothing, which a function with an empty frame does.
void freeingNull() {
  vertex_task_dealloc(nullptr);
  void* p = vertex_task_alloc(16);
  check(p != nullptr, "allocating after freeing null failed", 0);
  vertex_task_dealloc(p);
}

} // namespace

int main() {
  textInit(report);
  alignment();
  distinct();
  reuse();
  stackDiscipline();
  deepChain();
  unwindAtOnce();
  oversized();
  freeingNull();

  vertex_pal_write(1, report.bytes, report.count);
  return failures == 0 ? 0 : 1;
}
