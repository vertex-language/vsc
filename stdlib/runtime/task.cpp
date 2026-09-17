// Tasks, and the executor that runs them.
//
// A task has no stack. An async function keeps whatever outlives a
// suspension in a frame the task allocator owns, and leaves by tail
// calling, so a chain of async calls is flat and a task at rest is
// nothing but a continuation and the context to hand it.
//
// That makes the executor small. It calls a task's continuation; the
// chain of tail calls runs; and whatever suspends writes down where to
// carry on and returns -- which lands back here, because nothing in
// between built a frame to return through.
//
// The model is Swift's, measured rather than guessed. See
// docs/vertex_swift_async.md.
#include "vertex/abi.h"
#include "vertex/platform.h"

// Enters a continuation: the async context in the context register, a
// closure's captures in the self register, and up to two words of
// whatever the suspension answered in the argument registers.
//
// Assembly, because nothing C++ says loads those two registers, and it
// saves them across the call because an async function does not.
extern "C" void vertex_task_enter(void (*code)(), void* async, void* self, u64 a0, u64 a1);

namespace vertex {

// ---- the task allocator ----
//
// Async frames are allocated and freed in the order a call chain enters
// and leaves, which is stack discipline, so they come from a bump
// pointer rather than the heap. Swift's swift_task_alloc is the same
// thing for the same reason: a frame per async call through malloc would
// cost more than the call.
//
// The slabs are a chain, so a deep chain of calls grows rather than
// failing, and freeing walks back down it. A first slab big enough for
// most call chains means most tasks never allocate a second.

inline constexpr usize firstSlabBytes = 4 * 1024;

struct Slab {
  Slab*  prev;
  usize  size;   // the whole allocation, header included
  u8*    end;    // one past the last usable byte
};

struct TaskAllocator {
  Slab* slab;  // the slab being allocated from, or null before the first
  u8*   next;  // the bump pointer
};

struct Task {
  // Where to carry on and with what. A task at rest is these two words
  // and the frames its allocator holds; there is no stack.
  void          (*resume)();
  AsyncContext* ctx;
  // What to put in the self register on the way in, and up to two words
  // of whatever the task was waiting on for the argument registers.
  //
  // self is the closure's captures on the first entry and nothing
  // afterwards: a continuation's self register is where a throwing async
  // function's error arrives, and handing it a closure context would be
  // a task that had thrown something. The body does not need it twice --
  // the context is a declared parameter, so the prologue wrote it into
  // the frame and every resume reads it from there.
  void* self;
  // captures is the same object, kept for as long as the task runs so
  // that it can be let go of when the task has finished.
  HeapObject* captures;
  u64   arg[2];
  i32   status;
  u64   wake;
  Task* next;
  bool  done;
  HeapObject* handle;      // what a Task value holds of it, or null
  // A wait on a descriptor that may also have a deadline. The task is
  // registered with the platform and, where it gave a deadline, is on the
  // sleeping list as well: whichever comes first takes it off the other.
  i32  waitFd;
  i32  waitEvents;
  bool waitingFd;      // registered with the platform, not yet woken
  bool waitDeadline;   // on the sleeping list for that registration
  // Where this task's async frames come from. Freed with the task.
  TaskAllocator alloc;
};

// A handle's contents: whether its task has finished, and the tasks
// waiting for it to, linked through their next.
struct TaskJoin {
  Task* waiters;
  bool  done;
};

static TaskJoin* joinOf(HeapObject* handle) {
  return reinterpret_cast<TaskJoin*>(handle + 1);
}

// The tasks ready to run, first in first out.
static Task* runnableHead;
static Task* runnableTail;

// The tasks waiting on a deadline, soonest first.
static Task* sleeping;

// How many tasks wait on a descriptor: while any do, the executor asks the
// platform which are ready before it lets the thread sleep.
static int waitingOnIO;

// Switches since the executor last asked about descriptors, so that tasks
// that only ever yield cannot keep one that waits on I/O from running.
static int switchesSincePoll;
inline constexpr int pollEvery = 64;

static Task* current;
static Task* mainTask;

// How the executor waits when it has nothing to run, where something other
// than the executor owns the thread's waiting. See vertex_task_set_idle_wait.
static void (*idleWait)(i64 timeoutNanos);
static i32   mainStatus;

static void makeRunnable(Task* t) {
  t->next = nullptr;
  if (runnableTail != nullptr)
    runnableTail->next = t;
  else
    runnableHead = t;
  runnableTail = t;
}

static Task* nextRunnable() {
  Task* t = runnableHead;
  if (t != nullptr) {
    runnableHead = t->next;
    if (runnableHead == nullptr)
      runnableTail = nullptr;
    t->next = nullptr;
  }
  return t;
}

// dropSleeper takes t off the sleeping list, where it is on it.
static void dropSleeper(Task* t) {
  Task** at = &sleeping;
  while (*at != nullptr) {
    if (*at == t) {
      *at = t->next;
      t->next = nullptr;
      return;
    }
    at = &(*at)->next;
  }
}

static void addSleeper(Task* t) {
  Task** at = &sleeping;
  while (*at != nullptr && (*at)->wake <= t->wake)
    at = &(*at)->next;
  t->next = *at;
  *at = t;
}

// The allocator's own helpers, defined below: a task's root context comes
// from its allocator, and spawn makes one before the file gets there.
static void* allocFrom(TaskAllocator* a, usize size);

// Where a task's own function returns to.
//
// Every async function ends by tail calling the continuation its context
// names. The outermost call of a task has no caller to name one, so its
// root context names this. Whatever it answered is in the argument
// registers and is nobody's business here: a task that returns a value
// has already written it into the cell its Task value reads.
// It is the thunk that is named and not the C function behind it: a
// return into a continuation puts the context in the async register and
// the results from the first argument register, which is the shape the
// thunk turns into a C call. See stdlib.TaskAsm.
extern "C" void vertex_task_done();

// spawn makes a task that runs the function fp names, with a closure's
// captures for the self register, entered at a context of its own.
//
// fp is the record beside that function rather than the function: the
// caller of an async function allocates its frame, here that caller is
// the runtime, and only the record says how big the frame has to be.
// See AsyncFunctionPointer.
static Task* spawn(const AsyncFunctionPointer* fp, void* self) {
  if (fp == nullptr)
    vertex_pal_abort();
  void (*code)() = asyncCode(fp);
  u64 frameBytes = fp->size;
  auto* t = static_cast<Task*>(vertex_pal_alloc(sizeof(Task), 16));
  if (t == nullptr)
    vertex_pal_abort();
  t->alloc.slab = nullptr;
  t->alloc.next = nullptr;
  if (frameBytes < sizeof(AsyncContext))
    frameBytes = sizeof(AsyncContext);
  auto* root = static_cast<AsyncContext*>(allocFrom(&t->alloc, static_cast<usize>(frameBytes)));
  if (root == nullptr)
    vertex_pal_abort();
  root->parent = nullptr;
  root->resumeParent = &vertex_task_done;
  t->ctx = root;
  t->resume = code;
  // The caller lets its own copy of the function value go; the task keeps
  // the captures alive until it has finished.
  t->self = self;
  t->captures = static_cast<HeapObject*>(self);
  vertex_retain(t->captures);
  t->arg[0] = 0;
  t->arg[1] = 0;
  t->status = 0;
  t->wake = 0;
  t->next = nullptr;
  t->done = false;
  t->handle = nullptr;
  t->waitFd = -1;
  t->waitEvents = 0;
  t->waitingFd = false;
  t->waitDeadline = false;
  makeRunnable(t);
  return t;
}

// A slab's usable bytes start after its header, aligned.
static u8* slabStart(Slab* s) {
  usize at = (reinterpret_cast<usize>(s) + sizeof(Slab) + 15) & ~static_cast<usize>(15);
  return reinterpret_cast<u8*>(at);
}

static bool inSlab(Slab* s, void* p) {
  auto* b = static_cast<u8*>(p);
  return s != nullptr && b >= slabStart(s) && b < s->end;
}

// grow adds a slab that can hold at least want bytes.
static bool grow(TaskAllocator* a, usize want) {
  usize bytes = firstSlabBytes;
  while (bytes < sizeof(Slab) + 16 + want)
    bytes *= 2;
  auto* s = static_cast<Slab*>(vertex_pal_alloc(bytes, 16));
  if (s == nullptr)
    return false;
  s->prev = a->slab;
  s->size = bytes;
  s->end = reinterpret_cast<u8*>(s) + bytes;
  a->slab = s;
  a->next = slabStart(s);
  return true;
}

static void* allocFrom(TaskAllocator* a, usize size) {
  size = (size + 15) & ~static_cast<usize>(15);
  if (a->slab == nullptr || a->next + size > a->slab->end) {
    if (!grow(a, size))
      return nullptr;
  }
  u8* p = a->next;
  a->next = p + size;
  return p;
}

// deallocTo frees back to p, which is where the matching allocation
// started. Anything allocated after it is freed with it, which is what
// stack discipline means and what a return does.
static void deallocTo(TaskAllocator* a, void* p) {
  while (a->slab != nullptr && !inSlab(a->slab, p)) {
    Slab* dead = a->slab;
    a->slab = dead->prev;
    a->next = a->slab != nullptr ? a->slab->end : nullptr;
    vertex_pal_free(dead, dead->size, 16);
  }
  if (a->slab != nullptr)
    a->next = static_cast<u8*>(p);
}

static void freeSlabs(TaskAllocator* a) {
  while (a->slab != nullptr) {
    Slab* dead = a->slab;
    a->slab = dead->prev;
    vertex_pal_free(dead, dead->size, 16);
  }
  a->next = nullptr;
}

// An async function called off a task still needs its frames. They come
// from an allocator of the thread's own, which behaves the same way.
static TaskAllocator rootAllocator;

// The allocator an async frame comes from: the running task's, or the
// thread's where there is no task.
static TaskAllocator* allocatorOf() {
  return current != nullptr ? &current->alloc : &rootAllocator;
}

// park writes down where a task carries on, which is the whole of
// suspending now. The primitive that calls it returns, and because every
// async call between here and the executor was a tail call, that return
// lands in the executor.
//
// ctx is the context of the primitive being suspended in, so carrying on
// means calling its resumeParent with it -- the stub whoever called the
// primitive put there.
static void park(Task* t, AsyncContext* ctx) {
  t->ctx = ctx;
  t->resume = ctx->resumeParent;
  t->arg[0] = 0;
  t->arg[1] = 0;
  // A continuation is entered with the error in the self register, so
  // null is what a resume puts there. None of these primitives is
  // declared as failing, so nothing reads it today; leaving a closure
  // context there would be a task that had thrown one the moment the
  // first of them was. See Task::self.
  t->self = nullptr;
}

// wakeReady makes runnable every task whose descriptor is ready, waiting
// up to timeout nanoseconds for one to be, or not at all for 0, or until
// one is for -1.
static void wakeReady(i64 timeout) {
  void* ready[64];
  int n = vertex_pal_io_wait(timeout, ready, 64);
  for (int i = 0; i < n; i++) {
    if (ready[i] == nullptr)
      continue;
    Task* t = static_cast<Task*>(ready[i]);
    // The registration was one-shot, so it is spent either way.
    if (!t->waitingFd)
      continue;
    t->waitingFd = false;
    if (t->waitDeadline) {
      dropSleeper(t);
      t->waitDeadline = false;
    }
    t->arg[0] = 1;
    makeRunnable(t);
    waitingOnIO--;
  }
}

static void wakeSleepers() {
  if (sleeping == nullptr)
    return;
  u64 now = vertex_pal_now();
  while (sleeping != nullptr && sleeping->wake <= now) {
    Task* t = sleeping;
    sleeping = t->next;
    t->next = nullptr;
    // A deadline that came first ends the wait on the descriptor too, and
    // the registration has to go back: left in place it would hand the
    // executor this task again, long after it had run on.
    if (t->waitDeadline) {
      t->waitDeadline = false;
      if (t->waitingFd) {
        vertex_pal_io_unregister(t->waitFd, t->waitEvents);
        t->waitingFd = false;
        waitingOnIO--;
      }
      t->arg[0] = 0;
    }
    makeRunnable(t);
  }
}

} // namespace vertex

using namespace vertex;

extern "C" {

// vertex_task_spawn starts a task running code, which runs when the
// executor next reaches it.
void vertex_task_spawn(const AsyncFunctionPointer* fp, void* self) {
  spawn(fp, self);
}

// vertex_task_start starts a task as vertex_task_spawn does, and is a
// handle to it: one reference for the caller, and one the task holds until
// it has finished.
HeapObject* vertex_task_start(const AsyncFunctionPointer* fp, void* self) {
  Task* t = spawn(fp, self);
  HeapObject* handle = vertex_alloc(sizeof(TaskJoin));
  joinOf(handle)->waiters = nullptr;
  joinOf(handle)->done = false;
  vertex_retain(handle);
  t->handle = handle;
  return handle;
}

// vertex_task_cell is a counted object of size bytes, zeroed, which a task
// that returns a value stores it into and its Task value reads it out of.
HeapObject* vertex_task_cell(u64 size) {
  return vertex_alloc(size);
}

// vertex_task_cell_contents is where a cell's bytes are.
void* vertex_task_cell_contents(HeapObject* cell) {
  return reinterpret_cast<void*>(cell + 1);
}

// vertex_task_cell_typed_contents is where a typed cell's value is.
void* vertex_task_cell_typed_contents(HeapObject* cell) {
  const Metadata* type = cell->metadata->type;
  return reinterpret_cast<void*>(reinterpret_cast<u8*>(cell) + boxValueOffset(witnesses(type)->flags));
}

HeapObject* vertex_box_allocate(const Metadata* type);

// vertex_task_cell_typed is a cell for a value of a type that owns
// something: a box, whose end destroys the value through the type's
// witnesses. The value starts as zero, which a task that never stored one
// leaves for that destroy to find, and releases nothing.
HeapObject* vertex_task_cell_typed(const Metadata* type) {
  HeapObject* obj = vertex_box_allocate(type);
  auto* bytes = static_cast<u8*>(vertex_task_cell_typed_contents(obj));
  for (usize i = 0; i < witnesses(type)->size; i++)
    bytes[i] = 0;
  return obj;
}

// The suspending primitives.
//
// Each is an async function in the ABI sense: it is handed a context and
// answers through the continuation that context names. What they all do
// is write down where to carry on and return -- see park.

// carryOn is what a primitive does when it has nothing to wait for: a
// join on a task that has already finished, a yield outside any task.
//
// Inside a task it is still a suspension. The task is parked and made
// runnable and the executor enters the continuation on its next turn,
// rather than this frame running the rest of the task underneath it --
// which would put a C frame on the stack per such call, and a loop of
// them would grow without bound.
//
// Outside a task there is no executor to hand it to, so the continuation
// runs from here and the rest of the chain with it. a0 is what the
// continuation is answered with, where the primitive answers at all.
static void carryOn(AsyncContext* ctx, u64 a0) {
  if (current != nullptr) {
    park(current, ctx);
    current->arg[0] = a0;
    makeRunnable(current);
    return;
  }
  vertex_task_enter(ctx->resumeParent, ctx, nullptr, a0, 0);
}

// vertex_task_done is where a task's outermost call returns to.
void vertex_task_done_at(AsyncContext* ctx, u64 status) {
  (void)ctx;
  if (current == nullptr)
    return;
  current->done = true;
  // An async main answers with a status, which is one word in the first
  // argument register. A task that answers with nothing leaves whatever
  // was there, and nothing reads it.
  current->status = static_cast<i32>(status);
}

// vertex_task_join carries on when the task a handle is to has finished.
void vertex_task_join_at(AsyncContext* ctx, HeapObject* handle) {
  TaskJoin* j = joinOf(handle);
  if (j->done || current == nullptr) {
    carryOn(ctx, 0);
    return;
  }
  park(current, ctx);
  current->next = j->waiters;
  j->waiters = current;
}

// vertex_task_yield lets every other runnable task run first.
void vertex_task_yield_at(AsyncContext* ctx) {
  carryOn(ctx, 0);
}

// vertex_task_sleep carries on after at least this long. Outside a task
// the thread sleeps, since there is nothing else for it to be doing.
void vertex_task_sleep_at(AsyncContext* ctx, u64 nanoseconds) {
  if (current == nullptr) {
    vertex_pal_sleep(nanoseconds);
    carryOn(ctx, 0);
    return;
  }
  park(current, ctx);
  current->wake = vertex_pal_now() + nanoseconds;
  addSleeper(current);
}

// vertex_task_alloc is an async frame: the memory an async function
// keeps what has to survive a suspension in.
//
// It comes from the running task's bump pointer, so it costs an add.
// Frames are freed in the order a call chain leaves, which is why a
// bump pointer is enough -- see the allocator above.
void* vertex_task_alloc(u64 size) {
  return allocFrom(allocatorOf(), static_cast<usize>(size));
}

// vertex_task_dealloc frees a frame and everything allocated after it,
// which under stack discipline is nothing.
void vertex_task_dealloc(void* frame) {
  if (frame != nullptr)
    deallocTo(allocatorOf(), frame);
}

// vertex_task_wait_fd waits until a descriptor is ready to be read from
// (events 1) or written to (2), or until timeout nanoseconds have passed;
// a negative timeout waits for as long as it takes. It is 1 where the
// descriptor is ready and 0 where the wait timed out.
//
// Which thread stops depends on where it is called. In a task it is the
// task: the executor is told to watch the descriptor and runs everything
// else until it is ready. Outside a task -- or on a platform with no
// readiness registration -- the thread itself waits, which is what the
// caller would have got from a blocking descriptor anyway.
//
// One caller can therefore serve both: a socket held non-blocking, read
// in a loop around this, is cooperative inside a task and ordinary
// blocking I/O outside one, with no second set of operations for it.
void vertex_task_wait_fd_at(AsyncContext* ctx, i32 fd, i32 events, i64 timeout) {
  // Nothing to suspend, or nowhere to register it: wait on the thread and
  // answer straight away, which is what a blocking descriptor would have
  // done to the caller anyway.
  if (current == nullptr || vertex_pal_io_register(fd, events, current) != 0) {
    u64 ready = vertex_pal_io_wait_one(fd, events, timeout) == 1 ? 1 : 0;
    carryOn(ctx, ready);
    return;
  }

  Task* t = current;
  park(t, ctx);
  t->waitFd = fd;
  t->waitEvents = events;
  t->waitingFd = true;
  waitingOnIO++;
  if (timeout >= 0) {
    t->wake = vertex_pal_now() + static_cast<u64>(timeout);
    t->waitDeadline = true;
    addSleeper(t);
  }
}

// vertex_task_set_idle_wait hands the executor's idle waiting to a host
// event loop, or back to the executor with null.
//
// A window system owns the thread it runs on: AppKit delivers nothing to a
// program that is not inside its own wait, and that wait is not a
// descriptor the executor can watch. So the executor becomes its guest,
// the way Swift's main executor is drained by the main run loop once an
// app has one. When it has nothing to run it calls wait with how long it
// can afford to wait -- -1 for as long as it takes, 0 not at all -- and
// the host waits in its own way.
//
// wait must return when the deadline passes, when something happened to
// the host, or when vertex_task_io_descriptor becomes readable, which is
// what a socket or pipe some task waits on becoming ready looks like.
void vertex_task_set_idle_wait(void (*wait)(i64 timeoutNanos)) {
  idleWait = wait;
}

// vertex_task_io_descriptor is a descriptor that is readable whenever a
// descriptor some task waits on is ready, for a host wait to watch; -1
// where the platform has none.
i32 vertex_task_io_descriptor(void) {
  return vertex_pal_io_descriptor();
}

// vertex_task_run runs tasks: the runnable ones in turn, and when only
// sleeping ones are left, the thread sleeps until the soonest is due. It
// returns when the main task has finished, or when no task is left.
void vertex_task_run(void) {
  for (;;) {
    wakeSleepers();
    if ((waitingOnIO > 0 || idleWait != nullptr) && ++switchesSincePoll >= pollEvery) {
      switchesSincePoll = 0;
      // A host loop gets its turn even while tasks keep the executor
      // busy, or a window would stop answering the moment a program
      // started computing.
      if (idleWait != nullptr)
        idleWait(0);
      if (waitingOnIO > 0)
        wakeReady(0);
    }
    Task* t = nextRunnable();
    if (t == nullptr) {
      if (sleeping == nullptr && waitingOnIO == 0)
        return;
      i64 timeout = -1;
      if (sleeping != nullptr) {
        u64 now = vertex_pal_now();
        timeout = sleeping->wake > now ? static_cast<i64>(sleeping->wake - now) : 0;
      }
      switchesSincePoll = 0;
      if (idleWait != nullptr) {
        // The host's own wait, for as long as the executor has nothing to
        // do. It returns when something happened to it, or when a
        // descriptor the executor watches became ready, or at the deadline;
        // whichever it was, what is ready is collected next.
        idleWait(timeout);
        if (waitingOnIO > 0)
          wakeReady(0);
      } else if (waitingOnIO > 0) {
        wakeReady(timeout);
      } else if (timeout > 0) {
        vertex_pal_sleep(static_cast<u64>(timeout));
      }
      continue;
    }
    // Into the continuation. It runs until something suspends and
    // returns, which lands here, because the chain between is tail calls
    // and built no frame to return through.
    current = t;
    void (*code)() = t->resume;
    t->resume = nullptr;
    vertex_task_enter(code, t->ctx, t->self, t->arg[0], t->arg[1]);
    current = nullptr;
    // Nothing wrote down where to carry on and it did not finish: there
    // is no such state, and treating it as finished would leak the task
    // rather than say so.
    if (!t->done && t->resume == nullptr)
      vertex_pal_abort();
    if (t->done) {
      if (t->handle != nullptr) {
        TaskJoin* j = joinOf(t->handle);
        j->done = true;
        while (Task* w = j->waiters) {
          j->waiters = w->next;
          makeRunnable(w);
        }
        vertex_release(t->handle);
      }
      bool wasMain = t == mainTask;
      if (wasMain)
        mainStatus = t->status;
      vertex_release(t->captures);
      freeSlabs(&t->alloc);
      vertex_pal_free(t, sizeof(Task), 16);
      // As in Swift: the program is over when its async main is, whatever
      // other tasks were still to run.
      if (wasMain)
        return;
    }
  }
}

// vertex_async_main runs an async `main` as the first task.
void vertex_async_main(const AsyncFunctionPointer* fp, void* self) {
  mainTask = spawn(fp, self);
  vertex_task_run();
}

// vertex_async_main_status runs an async `main() -> Int32` as the first
// task, and is the status it answered with.
//
// The status arrives at vertex_task_done in the first argument register,
// which is where an async function's single-word result goes.
i32 vertex_async_main_status(const AsyncFunctionPointer* fp, void* self) {
  mainTask = spawn(fp, self);
  vertex_task_run();
  return mainStatus;
}

}
