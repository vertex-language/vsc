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
//
// There is one executor per thread. Thread 0's is the main executor,
// which hosts a window system's event pump where there is one and runs
// what is marked @MainActor; the others are a pool of workers, one per
// core, which unannotated async code runs on. A task belongs to one
// executor at a time, and only that executor touches it; it moves by a
// hop -- a suspension that names the executor to carry on at -- which
// the executor performs once the task's frames have unwound. Between
// executors a task travels through a mailbox, a lock-free list the
// receiver drains, and a wake on the receiver's queue where it may be
// waiting. See docs/task_scheduler_v2.md.
#include "vertex/abi.h"
#include "vertex/platform.h"

// Enters a continuation: the async context in the context register, a
// closure's captures in the self register, and up to two words of
// whatever the suspension answered in the argument registers.
//
// Assembly, because nothing C++ says loads those two registers, and it
// saves them across the call because an async function does not.
extern "C" void vertex_task_enter(void (*code)(), void* async, void* self, u64 a0, u64 a1);

// What every thread reads and one thread writes once -- the hash seed,
// the conformance records -- is written before the pool starts.
extern "C" void vertex_runtime_warm(void);

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
  // The executor the task is on, which is the only one that runs it, and
  // the one a hop names for it to carry on at, or null.
  struct Executor* owner;
  struct Executor* target;
  // The executor the task calls home: where it was started, or the
  // worker it was given, and where its code that is not @MainActor
  // runs. A hop to the main executor is for @MainActor code, and what
  // follows it hops home again.
  struct Executor* home;
};

// A handle's contents: whether its task has finished, and the tasks
// waiting for it to, linked through their next. A task on one executor
// may join one on another, so the handle is under a spinlock.
struct TaskJoin {
  Task* waiters;
  bool  done;
  u32   lock;
};

static TaskJoin* joinOf(HeapObject* handle) {
  return reinterpret_cast<TaskJoin*>(handle + 1);
}

static void lockJoin(TaskJoin* j) {
  while (__builtin_atomic_cas(&j->lock, 0u, 1u) != 0u) {
  }
}

static void unlockJoin(TaskJoin* j) { __builtin_atomic_store(&j->lock, 0u); }

// An executor: one thread's tasks and how it waits for them.
struct Executor {
  // The tasks ready to run, first in first out.
  Task* runnableHead;
  Task* runnableTail;
  // The tasks waiting on a deadline, soonest first.
  Task* sleeping;
  // How many tasks wait on a descriptor: while any do, the executor asks
  // the platform which are ready before it lets the thread sleep.
  int waitingOnIO;
  // Switches since the executor last asked about descriptors, so that
  // tasks that only ever yield cannot keep one that waits on I/O from
  // running.
  int switchesSincePoll;
  Task* current;
  // An async function called off a task still needs its frames. They come
  // from an allocator of the thread's own, which behaves the same way.
  TaskAllocator rootAllocator;
  // The platform's readiness queue, this executor's own.
  void* io;
  // Tasks handed over by other executors, pushed under CAS and taken all
  // at once; and whether this executor is waiting, so that a push knows
  // to wake it.
  usize inbox;  // a Task*, as the word the atomics work on
  u32   idle;
  // How the executor waits when it has nothing to run, where something
  // other than the executor owns the thread's waiting -- the main
  // executor's, see vertex_task_set_idle_wait.
  void (*idleWait)(i64 timeoutNanos);
  int index;  // 0 for the main executor, 1.. for workers
};

inline constexpr int pollEvery = 64;
inline constexpr int maxWorkers = 64;

static Executor  mainExecutor;
static Executor* workers[maxWorkers];
static int       workerCount;
static bool      poolStarted;
static u32       nextWorker;  // round robin, for spawning onto the pool
static Task*     mainTask;
static i32       mainStatus;
// Tasks alive on every executor, so the main executor knows when there is
// nothing left anywhere.
static i64       liveTasks;

// executorHere is the executor of the calling thread: the main one on the
// main thread, or on any thread that is not a worker's.
static Executor* executorHere() {
  auto* e = static_cast<Executor*>(vertex_pal_thread_get());
  return e != nullptr ? e : &mainExecutor;
}

// currentTask is the task running on this thread, or null off any task.
static Task* currentTask() { return executorHere()->current; }

static void makeRunnable(Executor* e, Task* t) {
  t->next = nullptr;
  if (e->runnableTail != nullptr)
    e->runnableTail->next = t;
  else
    e->runnableHead = t;
  e->runnableTail = t;
}

static Task* nextRunnable(Executor* e) {
  Task* t = e->runnableHead;
  if (t != nullptr) {
    e->runnableHead = t->next;
    if (e->runnableHead == nullptr)
      e->runnableTail = nullptr;
    t->next = nullptr;
  }
  return t;
}

// deliver hands a task to the executor that owns it: onto its runnable
// list from its own thread, and into its inbox from any other, with a
// wake where it may be waiting.
static void deliver(Executor* e, Task* t) {
  if (e == executorHere()) {
    makeRunnable(e, t);
    return;
  }
  for (;;) {
    usize head = __builtin_atomic_load(&e->inbox);
    t->next = reinterpret_cast<Task*>(head);
    if (__builtin_atomic_cas(&e->inbox, head, reinterpret_cast<usize>(t)) == head)
      break;
  }
  if (__builtin_atomic_load(&e->idle) != 0u)
    vertex_pal_io_wake(e->io);
}

// drainInbox moves what other executors handed over onto the runnable
// list, in the order it arrived.
static void drainInbox(Executor* e) {
  Task* list = reinterpret_cast<Task*>(__builtin_atomic_xchg(&e->inbox, static_cast<usize>(0)));
  if (list == nullptr)
    return;
  Task* reversed = nullptr;
  while (list != nullptr) {
    Task* next = list->next;
    list->next = reversed;
    reversed = list;
    list = next;
  }
  while (reversed != nullptr) {
    Task* next = reversed->next;
    makeRunnable(e, reversed);
    reversed = next;
  }
}

// dropSleeper takes t off the sleeping list, where it is on it.
static void dropSleeper(Executor* e, Task* t) {
  Task** at = &e->sleeping;
  while (*at != nullptr) {
    if (*at == t) {
      *at = t->next;
      t->next = nullptr;
      return;
    }
    at = &(*at)->next;
  }
}

static void addSleeper(Executor* e, Task* t) {
  Task** at = &e->sleeping;
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
static Task* spawn(Executor* e, const AsyncFunctionPointer* fp, void* self) {
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
  t->owner = e;
  t->target = nullptr;
  t->home = e;
  __builtin_atomic_add(&liveTasks, 1ll);
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

// The allocator an async frame comes from: the running task's, or the
// thread's where there is no task.
static TaskAllocator* allocatorOf() {
  Executor* e = executorHere();
  return e->current != nullptr ? &e->current->alloc : &e->rootAllocator;
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
static void wakeReady(Executor* e, i64 timeout) {
  void* ready[64];
  int n = vertex_pal_io_wait(e->io, timeout, ready, 64);
  for (int i = 0; i < n; i++) {
    // A null token is a wake from another executor: its inbox is drained
    // by the loop.
    if (ready[i] == nullptr)
      continue;
    Task* t = static_cast<Task*>(ready[i]);
    // The registration was one-shot, so it is spent either way.
    if (!t->waitingFd)
      continue;
    t->waitingFd = false;
    if (t->waitDeadline) {
      dropSleeper(e, t);
      t->waitDeadline = false;
    }
    t->arg[0] = 1;
    makeRunnable(e, t);
    e->waitingOnIO--;
  }
}

static void wakeSleepers(Executor* e) {
  if (e->sleeping == nullptr)
    return;
  u64 now = vertex_pal_now();
  while (e->sleeping != nullptr && e->sleeping->wake <= now) {
    Task* t = e->sleeping;
    e->sleeping = t->next;
    t->next = nullptr;
    // A deadline that came first ends the wait on the descriptor too, and
    // the registration has to go back: left in place it would hand the
    // executor this task again, long after it had run on.
    if (t->waitDeadline) {
      t->waitDeadline = false;
      if (t->waitingFd) {
        vertex_pal_io_unregister(e->io, t->waitFd, t->waitEvents);
        t->waitingFd = false;
        e->waitingOnIO--;
      }
      t->arg[0] = 0;
    }
    makeRunnable(e, t);
  }
}

// poolExecutor is a worker to run something on, chosen round robin, or the
// main executor where there are no workers.
static Executor* poolExecutor() {
  if (workerCount == 0)
    return &mainExecutor;
  u32 n = static_cast<u32>(__builtin_atomic_add(&nextWorker, 1u));
  return workers[n % static_cast<u32>(workerCount)];
}

// An executor is made ready before anything runs on it.
static void initExecutor(Executor* e, int index) {
  e->runnableHead = nullptr;
  e->runnableTail = nullptr;
  e->sleeping = nullptr;
  e->waitingOnIO = 0;
  e->switchesSincePoll = 0;
  e->current = nullptr;
  e->rootAllocator.slab = nullptr;
  e->rootAllocator.next = nullptr;
  e->io = vertex_pal_io_open();
  e->inbox = 0;
  e->idle = 0;
  e->idleWait = nullptr;
  e->index = index;
}

static void runExecutor(Executor* e);

static void workerMain(void* arg) {
  auto* e = static_cast<Executor*>(arg);
  vertex_pal_thread_set(e);
  runExecutor(e);
}

// startPool makes the workers, once: as many as VERTEX_WORKERS says, or
// one per processor but the main thread's. VERTEX_WORKERS=0 is no pool
// at all, and every hop to it stays on the main executor.
static int parseCount(const char* text) {
  if (text == nullptr)
    return -1;
  int n = 0;
  bool any = false;
  for (; *text >= '0' && *text <= '9'; text++) {
    n = n * 10 + (*text - '0');
    any = true;
    if (n > maxWorkers)
      return maxWorkers;
  }
  return any && *text == 0 ? n : -1;
}

static void startPool() {
  if (poolStarted)
    return;
  poolStarted = true;
  int want = parseCount(vertex_pal_getenv("VERTEX_WORKERS"));
  if (want < 0) {
    want = vertex_pal_cpus() - 1;
    if (want < 1)
      want = 1;
  }
  // What every thread will read is made before any other thread exists.
  vertex_runtime_warm();
  for (int i = 0; i < want; i++) {
    auto* e = static_cast<Executor*>(vertex_pal_alloc(sizeof(Executor), 16));
    if (e == nullptr)
      break;
    initExecutor(e, i + 1);
    // Two statements, not `a || !start()`: vcx runs the call in that
    // condition twice, and the second thread shares the executor.
    bool started = e->io != nullptr && vertex_pal_thread_start(workerMain, e);
    if (!started) {
      vertex_pal_free(e, sizeof(Executor), 16);
      break;
    }
    workers[workerCount++] = e;
  }
}

} // namespace vertex

using namespace vertex;

extern "C" {

// vertex_task_workers is how many workers the pool has, starting it if it
// has not started: what VERTEX_WORKERS asked for, or one per processor but
// the main thread's, less any the platform could not start. 0 where there
// is no pool (VERTEX_WORKERS=0, or no threads, as on Windows today). A
// package that spreads work over the pool -- net/tcp binding a listener
// per worker -- asks here rather than working the number out again.
i32 vertex_task_workers(void) {
  startPool();
  return workerCount;
}

// vertex_task_spawn starts a task running code, which runs when the
// executor next reaches it.
void vertex_task_spawn(const AsyncFunctionPointer* fp, void* self) {
  Executor* e = executorHere();
  deliver(e, spawn(e, fp, self));
}

static HeapObject* startOn(Executor* e, const AsyncFunctionPointer* fp, void* self) {
  HeapObject* handle = vertex_alloc(sizeof(TaskJoin));
  joinOf(handle)->waiters = nullptr;
  joinOf(handle)->done = false;
  joinOf(handle)->lock = 0;
  vertex_retain(handle);
  Task* t = spawn(e, fp, self);
  t->handle = handle;
  deliver(e, t);
  return handle;
}

// vertex_task_start starts a task as vertex_task_spawn does, and is a
// handle to it: one reference for the caller, and one the task holds until
// it has finished. The task starts on the executor that starts it, as
// Swift's Task {} inherits where it is made.
HeapObject* vertex_task_start(const AsyncFunctionPointer* fp, void* self) {
  return startOn(executorHere(), fp, self);
}

// vertex_task_start_detached starts a task on the pool, whatever thread
// starts it: Swift's Task.detached {}.
HeapObject* vertex_task_start_detached(const AsyncFunctionPointer* fp, void* self) {
  startPool();
  return startOn(poolExecutor(), fp, self);
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
  Executor* e = executorHere();
  Task* current = e->current;
  if (current != nullptr) {
    park(current, ctx);
    current->arg[0] = a0;
    makeRunnable(e, current);
    return;
  }
  vertex_task_enter(ctx->resumeParent, ctx, nullptr, a0, 0);
}

// vertex_task_done is where a task's outermost call returns to.
void vertex_task_done_at(AsyncContext* ctx, u64 status) {
  (void)ctx;
  Task* current = currentTask();
  if (current == nullptr)
    return;
  current->done = true;
  // An async main answers with a status, which is one word in the first
  // argument register. A task that answers with nothing leaves whatever
  // was there, and nothing reads it.
  current->status = static_cast<i32>(status);
}

// vertex_task_join carries on when the task a handle is to has finished.
// The task joined may finish on another executor: it hands this one back
// to its own.
void vertex_task_join_at(AsyncContext* ctx, HeapObject* handle) {
  TaskJoin* j = joinOf(handle);
  Task* current = currentTask();
  if (current == nullptr) {
    carryOn(ctx, 0);
    return;
  }
  lockJoin(j);
  if (j->done) {
    unlockJoin(j);
    carryOn(ctx, 0);
    return;
  }
  park(current, ctx);
  current->next = j->waiters;
  j->waiters = current;
  unlockJoin(j);
}

// hopTarget is the executor a hop names: 0 the main executor, 1 the
// pool -- this worker where this is one, else a worker -- and 2 the
// task's home.
static Executor* hopTarget(Executor* e, Task* current, u64 where) {
  switch (where) {
  case 1:
    startPool();
    return e->index > 0 ? e : poolExecutor();
  case 2:
    return current->home;
  }
  return &mainExecutor;
}

// vertex_task_needs_hop reports whether a hop to where would move the
// task: whether this is not already the executor it names. The compiler
// asks before each hop it emits, so that the common case -- @MainActor
// code awaiting @MainActor code, a task at home awaiting anything of its
// own -- costs a call and a compare and no suspension.
u64 vertex_task_needs_hop(u64 where) {
  Executor* e = executorHere();
  Task* current = e->current;
  if (current == nullptr)
    return 0;
  return hopTarget(e, current, where) != e ? 1 : 0;
}

// vertex_task_hop carries on at another executor; see hopTarget. On the
// executor named already, it is a yield; with no pool, a hop to it stays
// on the main executor. Outside a task there is nothing to move, so it
// carries straight on.
void vertex_task_hop_at(AsyncContext* ctx, u64 where) {
  Executor* e = executorHere();
  Task* current = e->current;
  if (current == nullptr) {
    carryOn(ctx, 0);
    return;
  }
  Executor* target = hopTarget(e, current, where);
  park(current, ctx);
  if (target == e) {
    makeRunnable(e, current);
    return;
  }
  // The executor moves it once the task's frames have unwound.
  current->target = target;
}

// vertex_task_on_main reports whether this is the main executor's thread.
bool vertex_task_on_main(void) {
  return executorHere() == &mainExecutor;
}

// vertex_task_assume_main is what MainActor.assumeIsolated checks before
// its body runs: that this is Thread 0. Elsewhere the program ends, with
// Swift's words for it.
void vertex_task_assume_main(void) {
  if (executorHere() != &mainExecutor)
    fatal("Incorrect actor executor assumption; Expected MainActor executor.");
}

// vertex_task_yield lets every other runnable task run first.
void vertex_task_yield_at(AsyncContext* ctx) {
  carryOn(ctx, 0);
}

// vertex_task_sleep carries on after at least this long. Outside a task
// the thread sleeps, since there is nothing else for it to be doing.
void vertex_task_sleep_at(AsyncContext* ctx, u64 nanoseconds) {
  Executor* e = executorHere();
  Task* current = e->current;
  if (current == nullptr) {
    vertex_pal_sleep(nanoseconds);
    carryOn(ctx, 0);
    return;
  }
  park(current, ctx);
  current->wake = vertex_pal_now() + nanoseconds;
  addSleeper(e, current);
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
  Executor* e = executorHere();
  Task* current = e->current;
  // Nothing to suspend, or nowhere to register it: wait on the thread and
  // answer straight away, which is what a blocking descriptor would have
  // done to the caller anyway.
  if (current == nullptr || vertex_pal_io_register(e->io, fd, events, current) != 0) {
    u64 ready = vertex_pal_io_wait_one(fd, events, timeout) == 1 ? 1 : 0;
    carryOn(ctx, ready);
    return;
  }

  Task* t = current;
  park(t, ctx);
  t->waitFd = fd;
  t->waitEvents = events;
  t->waitingFd = true;
  e->waitingOnIO++;
  if (timeout >= 0) {
    t->wake = vertex_pal_now() + static_cast<u64>(timeout);
    t->waitDeadline = true;
    addSleeper(e, t);
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
  mainExecutor.idleWait = wait;
}

// vertex_task_io_descriptor is a descriptor that is readable whenever a
// descriptor some task on the main executor waits on is ready, or a task
// has been handed to it from a worker, for a host wait to watch; -1 where
// the platform has none.
i32 vertex_task_io_descriptor(void) {
  if (mainExecutor.io == nullptr)
    initExecutor(&mainExecutor, 0);
  return vertex_pal_io_descriptor(mainExecutor.io);
}

namespace vertex {

// finish is what an executor does with a task that has finished: wakes
// what was waiting to join it, each on its own executor, and frees it.
static void finish(Executor* e, Task* t) {
  if (t->handle != nullptr) {
    TaskJoin* j = joinOf(t->handle);
    lockJoin(j);
    j->done = true;
    Task* waiters = j->waiters;
    j->waiters = nullptr;
    unlockJoin(j);
    while (Task* w = waiters) {
      waiters = w->next;
      deliver(w->owner, w);
    }
    vertex_release(t->handle);
  }
  bool wasMain = t == mainTask;
  if (wasMain)
    mainStatus = t->status;
  vertex_release(t->captures);
  freeSlabs(&t->alloc);
  vertex_pal_free(t, sizeof(Task), 16);
  __builtin_atomic_sub(&liveTasks, 1ll);
  (void)e;
}

// step runs one runnable task of the executor, or reports that it had
// none. It is the body of every executor's loop.
static bool step(Executor* e, bool* mainDone) {
  wakeSleepers(e);
  drainInbox(e);
  if ((e->waitingOnIO > 0 || e->idleWait != nullptr) && ++e->switchesSincePoll >= pollEvery) {
    e->switchesSincePoll = 0;
    // A host loop gets its turn even while tasks keep the executor
    // busy, or a window would stop answering the moment a program
    // started computing.
    if (e->idleWait != nullptr)
      e->idleWait(0);
    if (e->waitingOnIO > 0)
      wakeReady(e, 0);
    drainInbox(e);
  }
  Task* t = nextRunnable(e);
  if (t == nullptr)
    return false;
  // Into the continuation. It runs until something suspends and
  // returns, which lands here, because the chain between is tail calls
  // and built no frame to return through.
  e->current = t;
  void (*code)() = t->resume;
  t->resume = nullptr;
  vertex_task_enter(code, t->ctx, t->self, t->arg[0], t->arg[1]);
  e->current = nullptr;
  // Nothing wrote down where to carry on and it did not finish: there
  // is no such state, and treating it as finished would leak the task
  // rather than say so.
  if (!t->done && t->resume == nullptr)
    vertex_pal_abort();
  if (t->done) {
    bool wasMain = t == mainTask;
    finish(e, t);
    if (wasMain)
      *mainDone = true;
    return true;
  }
  // A hop: the task's frames have unwound, so it can be handed on.
  if (t->target != nullptr) {
    Executor* target = t->target;
    t->target = nullptr;
    t->owner = target;
    deliver(target, t);
  }
  return true;
}

// rest is how an executor waits with nothing to run: for the soonest
// deadline, a descriptor, or a task from another executor. The idle flag
// is set before the wait and read by whoever delivers, so that a delivery
// in between wakes it -- the inbox is looked at again after the flag.
static void rest(Executor* e) {
  i64 timeout = -1;
  if (e->sleeping != nullptr) {
    u64 now = vertex_pal_now();
    timeout = e->sleeping->wake > now ? static_cast<i64>(e->sleeping->wake - now) : 0;
  }
  e->switchesSincePoll = 0;
  __builtin_atomic_store(&e->idle, 1u);
  if (__builtin_atomic_load(&e->inbox) != 0) {
    __builtin_atomic_store(&e->idle, 0u);
    return;
  }
  if (e->idleWait != nullptr) {
    // The host's own wait, for as long as the executor has nothing to
    // do. It returns when something happened to it, or when a
    // descriptor the executor watches became ready, or at the deadline;
    // whichever it was, what is ready is collected next.
    e->idleWait(timeout);
    if (e->waitingOnIO > 0 || e->io != nullptr)
      wakeReady(e, 0);
  } else if (e->io != nullptr) {
    wakeReady(e, timeout);
  } else if (timeout > 0) {
    vertex_pal_sleep(static_cast<u64>(timeout));
  }
  __builtin_atomic_store(&e->idle, 0u);
}

// runExecutor is a worker's life: it runs what it has and waits for more,
// and never returns.
static void runExecutor(Executor* e) {
  bool mainDone = false;
  for (;;) {
    if (!step(e, &mainDone))
      rest(e);
  }
}

} // namespace vertex

// vertex_task_run runs tasks on the main executor: the runnable ones in
// turn, and when only sleeping or waiting ones are left, the thread waits
// until one is due, ready, or handed over by a worker. It returns when the
// main task has finished, or when no task is left anywhere.
void vertex_task_run(void) {
  Executor* e = &mainExecutor;
  if (e->io == nullptr)
    initExecutor(e, 0);
  vertex_pal_thread_set(e);
  startPool();
  bool mainDone = false;
  for (;;) {
    if (step(e, &mainDone)) {
      // As in Swift: the program is over when its async main is, whatever
      // other tasks were still to run.
      if (mainDone)
        return;
      continue;
    }
    if (e->sleeping == nullptr && e->waitingOnIO == 0 && __builtin_atomic_load(&liveTasks) == 0ll)
      return;
    rest(e);
  }
}

// vertex_runtime_warm makes what is made once and read from every thread
// before any thread but the main one exists.
void vertex_runtime_warm(void) {
  vertex::seed();
  vertex::warmConformances();
}

// vertex_async_main runs an async `main` as the first task.
void vertex_async_main(const AsyncFunctionPointer* fp, void* self) {
  if (mainExecutor.io == nullptr)
    initExecutor(&mainExecutor, 0);
  vertex_pal_thread_set(&mainExecutor);
  mainTask = spawn(&mainExecutor, fp, self);
  deliver(&mainExecutor, mainTask);
  vertex_task_run();
}

// vertex_async_main_status runs an async `main() -> Int32` as the first
// task, and is the status it answered with.
//
// The status arrives at vertex_task_done in the first argument register,
// which is where an async function's single-word result goes.
i32 vertex_async_main_status(const AsyncFunctionPointer* fp, void* self) {
  if (mainExecutor.io == nullptr)
    initExecutor(&mainExecutor, 0);
  vertex_pal_thread_set(&mainExecutor);
  mainTask = spawn(&mainExecutor, fp, self);
  deliver(&mainExecutor, mainTask);
  vertex_task_run();
  return mainStatus;
}

}
