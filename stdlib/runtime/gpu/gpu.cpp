// The gpu runtime unit: the host half of the built-in gpu module
// (proposed_vertex_kernel.md §3.2, GUIDELINES.md §1.1).
//
// It is linked only into a program that imports gpu. It knows two kinds
// of device:
//
//   - The CPU, always there, and the oracle every GPU result is tested
//     against. A kernel runs as the host function the compiler built
//     from it, once per work-item, on threads this unit owns. The device
//     intrinsics (vertex_gpu_workitem_id_x and the rest) are defined here
//     for it, and answer from the work-item the thread is running.
//
//   - Metal, opened with dlopen the first time a device is asked for and
//     driven through objc_msgSend, so that a program that never asks
//     never loads it, and nothing here needs an Objective-C compiler.
//     A kernel is the .metallib the compiler embedded beside it.
//
// A kernel is known by its descriptor, which the compiler writes beside
// every kernel (vsc/lower, gpu.go). A launch is built an argument slot at
// a time, in the order of the kernel's parameters as the device sees
// them, and the grid ends it.

#include "vertex/platform.h"

using namespace vertex;

extern "C" {
void* dlopen(const char* path, int mode);
void* dlsym(void* handle, const char* name);
}

namespace {

// ---- small things the unit needs and has no library for ----

template <typename T>
T* allocate(usize n) {
  if (n == 0) n = 1;
  void* p = vertex_pal_alloc(n * sizeof(T), alignof(T) < 16 ? 16 : alignof(T));
  vertex_pal_fill(p, 0, n * sizeof(T));
  return static_cast<T*>(p);
}

void release(void* p, usize bytes, usize align) {
  if (p) vertex_pal_free(p, bytes == 0 ? 1 : bytes, align);
}

usize length(const char* s) {
  usize n = 0;
  while (s && s[n]) n++;
  return n;
}

bool same(const char* a, const char* b) {
  if (!a || !b) return false;
  usize i = 0;
  while (a[i] && a[i] == b[i]) i++;
  return a[i] == b[i];
}

void say(const char* s) { vertex_pal_write(2, reinterpret_cast<const u8*>(s), length(s)); }

// A lock that spins, then sleeps: launches are rare and short.
struct Lock {
  u32 word = 0;
  void lock() {
    int spins = 0;
    while (__builtin_atomic_cas(&word, 0u, 1u) != 0u) {
      if (++spins > 64) vertex_pal_sleep(1000);
    }
  }
  void unlock() { __builtin_atomic_store(&word, 0u); }
};

}  // namespace

// ---- what the compiler writes beside each kernel ----

extern "C" {
struct vertex_gpu_kernel {
  const char* name;          // the kernel's function in the metallib
  const u8* metallib;        // null where there is no Metal image
  i64 metallibSize;
  void (*cpu)(void** slots);  // the kernel on the CPU device
  i64 flags;                 // kernelUsesBarrier
  i64 params;                // how many slots a launch fills
};
}

namespace {

constexpr i64 kernelUsesBarrier = 1;

enum Kind : i64 { KindCPU = 0, KindMetal = 1 };

struct Device;

struct Buffer {
  Device* device;
  u8* contents;  // host-visible, on both kinds: Apple's memory is unified
  i64 bytes;
  void* metal;  // id<MTLBuffer>, or null on the CPU
};

// A pipeline made for one kernel on one Metal device.
struct Pipeline {
  const vertex_gpu_kernel* kernel;
  void* state;  // id<MTLComputePipelineState>
  i64 maxThreads;
  i64 width;
  Pipeline* next;
};

struct Device {
  Kind kind;
  void* metal;  // id<MTLDevice>
  void* queue;  // id<MTLCommandQueue>
  i64 memory;
  i64 waveSize;
  Pipeline* pipelines;
  Lock lock;
  // Launches not yet run: an open command buffer and its serial compute
  // encoder, retained. A launch encodes into them and returns; the host
  // asking for a buffer's memory runs them and waits (syncDevice).
  void* pendingBuffer;   // id<MTLCommandBuffer>
  void* pendingEncoder;  // id<MTLComputeCommandEncoder>
  Lock pendingLock;
};

// ---- Metal, through the Objective-C runtime ----

namespace objc {
using id = void*;
using SEL = void*;

void* (*msgSend)();
SEL (*selector)(const char*);
id (*getClass)(const char*);
void* (*poolPush)();
void (*poolPop)(void*);
id (*createDevice)();
void* (*dataCreate)(const void*, usize, void*, void*);

struct Size {
  u64 width, height, depth;
};

template <typename R, typename... A>
R send(id self, const char* sel, A... args) {
  auto f = reinterpret_cast<R (*)(id, SEL, A...)>(msgSend);
  return f(self, selector(sel), args...);
}

bool loaded = false;
bool available = false;

bool load() {
  if (loaded) return available;
  loaded = true;
  void* metal = dlopen("/System/Library/Frameworks/Metal.framework/Metal", 1);
  if (!metal) {
    if (vertex_pal_getenv("VERTEX_GPU_DEBUG")) say("gpu: dlopen Metal failed\n");
    return false;
  }
  void* objcLib = dlopen("/usr/lib/libobjc.A.dylib", 1);
  void* dispatch = dlopen("/usr/lib/system/libdispatch.dylib", 1);
  if (!objcLib || !dispatch) return false;
  msgSend = reinterpret_cast<void* (*)()>(dlsym(objcLib, "objc_msgSend"));
  selector = reinterpret_cast<SEL (*)(const char*)>(dlsym(objcLib, "sel_registerName"));
  getClass = reinterpret_cast<id (*)(const char*)>(dlsym(objcLib, "objc_getClass"));
  poolPush = reinterpret_cast<void* (*)()>(dlsym(objcLib, "objc_autoreleasePoolPush"));
  poolPop = reinterpret_cast<void (*)(void*)>(dlsym(objcLib, "objc_autoreleasePoolPop"));
  createDevice = reinterpret_cast<id (*)()>(dlsym(metal, "MTLCreateSystemDefaultDevice"));
  dataCreate = reinterpret_cast<void* (*)(const void*, usize, void*, void*)>(dlsym(dispatch, "dispatch_data_create"));
  available = msgSend && selector && getClass && poolPush && poolPop && createDevice && dataCreate;
  if (!available && vertex_pal_getenv("VERTEX_GPU_DEBUG")) {
    say("gpu: Metal is here, but the Objective-C runtime or libdispatch is not what it expects\n");
  }
  return available;
}

// The text of an NSError, to standard error.
void report(const char* what, id error) {
  say("gpu: ");
  say(what);
  if (error) {
    id text = send<id>(error, "localizedDescription");
    const char* s = text ? send<const char*>(text, "UTF8String") : nullptr;
    if (s) {
      say(": ");
      say(s);
    }
  }
  say("\n");
}
}  // namespace objc

// ---- devices ----

Device cpuDevice = {KindCPU, nullptr, nullptr, 0, 1, nullptr, {}};
Device metalDevice = {KindMetal, nullptr, nullptr, 0, 32, nullptr, {}};
bool metalTried = false;
Lock devicesLock;

Device* metal() {
  devicesLock.lock();
  if (!metalTried) {
    metalTried = true;
    if (objc::load()) {
      void* pool = objc::poolPush();
      objc::id d = objc::createDevice();
      if (d) {
        metalDevice.metal = d;
        metalDevice.queue = objc::send<objc::id>(d, "newCommandQueue");
        metalDevice.memory = static_cast<i64>(objc::send<u64>(d, "recommendedMaxWorkingSetSize"));
      }
      objc::poolPop(pool);
    }
  }
  devicesLock.unlock();
  return metalDevice.metal && metalDevice.queue ? &metalDevice : nullptr;
}

// ---- the CPU device ----
//
// Work runs on threads this unit starts, never the caller's: the thread's
// one slot (vertex_pal_thread_set) holds the work-item it is running,
// and the caller's slot is its executor's.

struct Launch;
i32 syncDevice(Device* d);

// A place in a kernel that asks for workgroup storage, and where in the
// group's storage its allocation is.
struct SharedSite {
  void* key;
  i64 at;
};

constexpr int maxSharedSites = 64;

struct Group {
  i64 id[3];
  u8* shared;       // the group's workgroup storage
  i64 sharedSize;
  i64 sharedUsed;   // how much of it the sites have taken
  SharedSite sites[maxSharedSites];
  u32 siteCount;
  Lock siteLock;
  u32 arrived;      // at the barrier, this phase
  u32 phase;
  u32 members;      // work-items still running the kernel
};

struct Fiber;

struct WorkItem {
  i32 local[3];
  i32 group[3];
  i32 size[3];
  i32 groups[3];
  Group* g;
  Launch* launch;
  Fiber* fiber;  // the fiber running it, where the group runs on fibers
};

// ---- fibers: a group's work-items on one thread ----
//
// A kernel with a barrier needs every work-item of its group to reach the
// barrier before any passes it. Running each work-item on a thread of its
// own does that, and costs a group of 256 two hundred and fifty-six
// threads waiting on each other at every barrier. Here a group runs on
// one worker instead, each work-item a fiber with a stack of its own: a
// barrier switches to the next fiber, and when every fiber has reached
// it, they all pass. Groups run on as many workers as there are
// processors, as groups without barriers do. This is how CPU
// implementations of OpenCL have long run workgroups.
//
// A switch is vertex_gpu_switch, assembly the build appends to this unit
// (vsc/stdlib, GPUAsm), which is also what defines VERTEX_GPU_FIBERS:
// where there is no switch for the target, a group with barriers runs on
// a thread per work-item instead.
#if VERTEX_GPU_FIBERS
extern "C" void vertex_gpu_switch(void** from, void** to);

// The frame vertex_gpu_switch pops: x19-x28, x29, x30, d8-d15.
constexpr usize switchFrame = 160;
constexpr usize switchReturn = 88;  // where x30 is in it

// A work-item's stack. A kernel's host function inlines what it calls,
// so it needs little; the CPU device's stacks are this size.
constexpr usize fiberStack = 64 * 1024;

struct Fiber {
  void* sp;          // its stack pointer, while it is not running
  void** scheduler;  // where the worker's is, to switch back to
  WorkItem item;
  u8* stack;
  bool done;
};
#endif

struct Launch {
  const vertex_gpu_kernel* kernel;
  Device* device;
  i64 count;
  i64 capacity;
  void** slots;  // what the kernel reads each parameter from
  u64* values;   // the parameters' bytes; a buffer's is its address
  Buffer** buffers;
  i64* offsets;
  bool mismatched;
  u32 trapped;
  // The grid, and how it is cut into groups.
  i64 grid[3];
  i64 per[3];
  i64 groups[3];
  i64 nextGroup;  // the next group a worker takes (no barrier)
  u32 finished;   // work done, counted in groups or work-items
  // Workers still holding the launch. The launch outlives its last
  // group: a worker that finishes one goes back for another, and finds
  // there are none only by looking -- so the launch is freed only when
  // every worker it was given to has stopped looking.
  u32 holders;
};

// A job a worker runs: one group of a launch without barriers, or one
// work-item of a group with them.
struct Job {
  Launch* launch;
  Group* group;
  i32 local[3];
  bool loop;  // take groups until there are none
};

struct Worker {
  Job job;
  u32 busy;  // 1: job is set and running
  bool parked;
};

constexpr i64 maxWorkers = 1024;
Worker* workers[maxWorkers];
i64 workerCount = 0;
Lock poolLock;

WorkItem* current() { return static_cast<WorkItem*>(vertex_pal_thread_get()); }

i64 groupIndex(const Launch* l, i64 gx, i64 gy, i64 gz) { return (gz * l->groups[1] + gy) * l->groups[0] + gx; }

void runItem(Launch* l, Group* g, i32 lx, i32 ly, i32 lz, WorkItem* w) {
  w->local[0] = lx;
  w->local[1] = ly;
  w->local[2] = lz;
  w->g = g;
  for (int a = 0; a < 3; a++) {
    w->group[a] = static_cast<i32>(g->id[a]);
    w->size[a] = static_cast<i32>(l->per[a]);
    w->groups[a] = static_cast<i32>(l->groups[a]);
  }
  w->launch = l;
  l->kernel->cpu(l->slots);
}

// Whether a work-item is inside the grid: the last group in each
// dimension may be partly outside it, and those work-items are not run,
// as Metal's dispatchThreads does not run them.
bool inGrid(const Launch* l, const Group* g, i64 lx, i64 ly, i64 lz) {
  return g->id[0] * l->per[0] + lx < l->grid[0] && g->id[1] * l->per[1] + ly < l->grid[1] &&
         g->id[2] * l->per[2] + lz < l->grid[2];
}

#if VERTEX_GPU_FIBERS
// Where a fiber starts: the kernel, then back to the scheduler for good.
void fiberMain() {
  WorkItem* w = static_cast<WorkItem*>(vertex_pal_thread_get());
  Fiber* f = w->fiber;
  Launch* l = w->launch;
  l->kernel->cpu(l->slots);
  f->done = true;
  vertex_gpu_switch(&f->sp, f->scheduler);
}

// A worker's fibers, kept between groups: stacks are made once each.
struct Fibers {
  Fiber* all;
  i64 count;
};

// runGroupOnFibers runs one group of a kernel with barriers: a fiber per
// work-item, each run until it reaches a barrier or ends, round and round
// until all have ended.
void runGroupOnFibers(Launch* l, i64 index, Fibers* pool, WorkItem* worker) {
  Group g = {};
  g.id[0] = index % l->groups[0];
  g.id[1] = (index / l->groups[0]) % l->groups[1];
  g.id[2] = index / (l->groups[0] * l->groups[1]);
  i64 per = l->per[0] * l->per[1] * l->per[2];
  if (pool->count < per) {
    Fiber* grown = allocate<Fiber>(static_cast<usize>(per));
    for (i64 i = 0; i < pool->count; i++) grown[i] = pool->all[i];
    if (pool->all) release(pool->all, static_cast<usize>(pool->count) * sizeof(Fiber), alignof(Fiber));
    pool->all = grown;
    pool->count = per;
  }
  void* scheduler = nullptr;
  i64 n = 0;
  for (i64 z = 0; z < l->per[2]; z++)
    for (i64 y = 0; y < l->per[1]; y++)
      for (i64 x = 0; x < l->per[0]; x++) {
        if (!inGrid(l, &g, x, y, z)) continue;
        Fiber* f = &pool->all[n++];
        if (!f->stack) f->stack = static_cast<u8*>(vertex_pal_alloc(fiberStack, 16));
        WorkItem* w = &f->item;
        w->local[0] = static_cast<i32>(x);
        w->local[1] = static_cast<i32>(y);
        w->local[2] = static_cast<i32>(z);
        w->g = &g;
        for (int a = 0; a < 3; a++) {
          w->group[a] = static_cast<i32>(g.id[a]);
          w->size[a] = static_cast<i32>(l->per[a]);
          w->groups[a] = static_cast<i32>(l->groups[a]);
        }
        w->launch = l;
        w->fiber = f;
        f->done = false;
        f->scheduler = &scheduler;
        // A frame for the first switch to pop: every register zero, and
        // the return address where the fiber starts.
        u8* top = f->stack + fiberStack - switchFrame;
        vertex_pal_fill(top, 0, switchFrame);
        *reinterpret_cast<void**>(top + switchReturn) = reinterpret_cast<void*>(&fiberMain);
        f->sp = top;
      }
  g.members = static_cast<u32>(n);
  i64 alive = n;
  while (alive > 0) {
    for (i64 i = 0; i < n; i++) {
      Fiber* f = &pool->all[i];
      if (f->done) continue;
      vertex_pal_thread_set(&f->item);
      vertex_gpu_switch(&scheduler, &f->sp);
      if (f->done) alive--;
    }
  }
  vertex_pal_thread_set(worker);
  if (g.shared) release(g.shared, static_cast<usize>(g.sharedSize), 16);
}
#endif

void runGroup(Launch* l, i64 index, WorkItem* w) {
  Group g = {};
  g.id[0] = index % l->groups[0];
  g.id[1] = (index / l->groups[0]) % l->groups[1];
  g.id[2] = index / (l->groups[0] * l->groups[1]);
  for (i64 z = 0; z < l->per[2]; z++)
    for (i64 y = 0; y < l->per[1]; y++)
      for (i64 x = 0; x < l->per[0]; x++) {
        if (!inGrid(l, &g, x, y, z)) continue;
        runItem(l, &g, static_cast<i32>(x), static_cast<i32>(y), static_cast<i32>(z), w);
      }
  if (g.shared) release(g.shared, static_cast<usize>(g.sharedSize), 16);
}

void workerMain(void* arg) {
  Worker* me = static_cast<Worker*>(arg);
  WorkItem item = {};
  vertex_pal_thread_set(&item);
#if VERTEX_GPU_FIBERS
  Fibers fibers = {};
#endif
  int idle = 0;
  for (;;) {
    if (__builtin_atomic_load(&me->busy) == 0u) {
      if (++idle > 256) vertex_pal_sleep(idle > 4096 ? 200000 : 2000);
      continue;
    }
    idle = 0;
    Job job = me->job;
    Launch* l = job.launch;
    if (job.loop) {
      i64 total = l->groups[0] * l->groups[1] * l->groups[2];
      for (;;) {
        i64 n = static_cast<i64>(__builtin_atomic_add(reinterpret_cast<u64*>(&l->nextGroup), 1ull));
        if (n >= total) break;
#if VERTEX_GPU_FIBERS
        if (l->kernel->flags & kernelUsesBarrier) {
          runGroupOnFibers(l, n, &fibers, &item);
        } else {
          runGroup(l, n, &item);
        }
#else
        runGroup(l, n, &item);
#endif
        __builtin_atomic_add(&l->finished, 1u);
      }
      // The last this worker touches of the launch.
      __builtin_atomic_sub(&l->holders, 1u);
    } else {
      runItem(l, job.group, job.local[0], job.local[1], job.local[2], &item);
      __builtin_atomic_sub(&job.group->members, 1u);
      __builtin_atomic_add(&l->finished, 1u);
    }
    __builtin_atomic_store(&me->busy, 0u);
  }
}

// A worker that is not running anything, started if there are not enough.
Worker* idleWorker(i64 from) {
  for (i64 i = from; i < workerCount; i++) {
    Worker* w = workers[i];
    if (!w->parked && __builtin_atomic_load(&w->busy) == 0u) return w;
  }
  if (workerCount >= maxWorkers) return nullptr;
  Worker* w = allocate<Worker>(1);
  if (!vertex_pal_thread_start(workerMain, w)) return nullptr;
  workers[workerCount++] = w;
  return w;
}

void give(Worker* w, const Job& job) {
  w->job = job;
  __builtin_atomic_store(&w->busy, 1u);
}

void waitFor(Launch* l, u32 target) {
  int spins = 0;
  while (__builtin_atomic_load(&l->finished) < target) {
    if (++spins > 128) vertex_pal_sleep(spins > 4096 ? 100000 : 1000);
  }
}

i32 runCPU(Launch* l) {
  // On a Metal device -- a kernel with no Metal image -- the host runs it
  // over the buffers' memory, so what was launched before runs first.
  if (syncDevice(l->device) != 0) return 7;
  poolLock.lock();
  i64 total = l->groups[0] * l->groups[1] * l->groups[2];
#if VERTEX_GPU_FIBERS
  bool byGroups = true;  // a group with barriers runs on one worker's fibers
#else
  bool byGroups = !(l->kernel->flags & kernelUsesBarrier);
#endif
  if (byGroups) {
    // Groups are independent: as many workers as there are processors,
    // each taking groups until none are left. A group with barriers is
    // run whole by the worker that takes it, on fibers.
    i64 n = vertex_pal_cpus();
    if (n < 1) n = 1;
    if (n > total) n = total;
    if (n > 64) n = 64;
    i64 given = 0;
    for (i64 i = 0; i < n; i++) {
      Worker* w = idleWorker(0);
      if (!w) break;
      __builtin_atomic_add(&l->holders, 1u);
      give(w, Job{l, nullptr, {0, 0, 0}, true});
      given++;
    }
    if (given == 0) {
      poolLock.unlock();
      return 3;
    }
    waitFor(l, static_cast<u32>(total));
    int spins = 0;
    while (__builtin_atomic_load(&l->holders) != 0u) {
      if (++spins > 128) vertex_pal_sleep(1000);
    }
  } else {
    // A barrier needs every work-item of the group running at once: one
    // worker each, a group at a time.
    i64 per = l->per[0] * l->per[1] * l->per[2];
    if (per > maxWorkers) {
      poolLock.unlock();
      return 4;
    }
    u32 done = 0;
    for (i64 index = 0; index < total; index++) {
      Group* g = allocate<Group>(1);
      g->id[0] = index % l->groups[0];
      g->id[1] = (index / l->groups[0]) % l->groups[1];
      g->id[2] = index / (l->groups[0] * l->groups[1]);
      u32 members = 0;
      for (i64 z = 0; z < l->per[2]; z++)
        for (i64 y = 0; y < l->per[1]; y++)
          for (i64 x = 0; x < l->per[0]; x++)
            if (inGrid(l, g, x, y, z)) members++;
      g->members = members;
      i64 from = 0;
      for (i64 z = 0; z < l->per[2]; z++)
        for (i64 y = 0; y < l->per[1]; y++)
          for (i64 x = 0; x < l->per[0]; x++) {
            if (!inGrid(l, g, x, y, z)) continue;
            Worker* w = idleWorker(from);
            if (!w) {
              poolLock.unlock();
              return 3;
            }
            give(w, Job{l, g, {static_cast<i32>(x), static_cast<i32>(y), static_cast<i32>(z)}, false});
          }
      done += members;
      waitFor(l, done);
      if (g->shared) release(g->shared, static_cast<usize>(g->sharedSize), 16);
      release(g, sizeof(Group), alignof(Group));
    }
  }
  poolLock.unlock();
  return __builtin_atomic_load(&l->trapped) ? 1 : 0;
}

// ---- launches on Metal ----

Pipeline* pipelineFor(Device* d, const vertex_gpu_kernel* k) {
  d->lock.lock();
  for (Pipeline* p = d->pipelines; p; p = p->next) {
    if (p->kernel == k) {
      d->lock.unlock();
      return p;
    }
  }
  Pipeline* made = nullptr;
  if (k->metallib && k->metallibSize > 0) {
    void* data = objc::dataCreate(k->metallib, static_cast<usize>(k->metallibSize), nullptr, nullptr);
    objc::id error = nullptr;
    objc::id lib = objc::send<objc::id>(d->metal, "newLibraryWithData:error:", data, &error);
    if (!lib) {
      objc::report("newLibraryWithData", error);
    } else {
      objc::id name = objc::send<objc::id>(objc::getClass("NSString"), "stringWithUTF8String:", k->name);
      objc::id fn = objc::send<objc::id>(lib, "newFunctionWithName:", name);
      if (!fn) {
        say("gpu: the metallib has no function ");
        say(k->name);
        say("\n");
      } else {
        error = nullptr;
        objc::id state = objc::send<objc::id>(d->metal, "newComputePipelineStateWithFunction:error:", fn, &error);
        if (!state) {
          objc::report("newComputePipelineStateWithFunction", error);
        } else {
          made = allocate<Pipeline>(1);
          made->kernel = k;
          made->state = state;
          made->maxThreads = static_cast<i64>(objc::send<u64>(state, "maxTotalThreadsPerThreadgroup"));
          made->width = static_cast<i64>(objc::send<u64>(state, "threadExecutionWidth"));
          made->next = d->pipelines;
          d->pipelines = made;
        }
      }
    }
  }
  d->lock.unlock();
  return made;
}

// syncDevice runs what has been encoded on d and waits for it: what the
// host reads from a buffer, or writes to one, is then what the launches
// before made of it. 0 when they ran; 7 when one failed.
i32 syncDevice(Device* d) {
  if (!d || d->kind != KindMetal) return 0;
  d->pendingLock.lock();
  objc::id cb = d->pendingBuffer;
  objc::id enc = d->pendingEncoder;
  d->pendingBuffer = nullptr;
  d->pendingEncoder = nullptr;
  d->pendingLock.unlock();
  if (!cb) return 0;
  void* pool = objc::poolPush();
  objc::send<void>(enc, "endEncoding");
  objc::send<void>(cb, "commit");
  objc::send<void>(cb, "waitUntilCompleted");
  i64 status = static_cast<i64>(objc::send<u64>(cb, "status"));
  i32 result = 0;
  if (status == 5) {  // MTLCommandBufferStatusError
    objc::report("a launch failed", objc::send<objc::id>(cb, "error"));
    result = 7;
  }
  objc::send<void>(enc, "release");
  objc::send<void>(cb, "release");
  objc::poolPop(pool);
  return result;
}

i32 runMetal(Launch* l) {
  void* pool = objc::poolPush();
  Pipeline* p = pipelineFor(l->device, l->kernel);
  if (vertex_pal_getenv("VERTEX_GPU_DEBUG")) say(p ? "gpu: pipeline made\n" : "gpu: no pipeline\n");
  if (!p) {
    objc::poolPop(pool);
    return 5;
  }
  i64 per = l->per[0] * l->per[1] * l->per[2];
  if (per > p->maxThreads) {
    say("gpu: a workgroup larger than this kernel can have on this device\n");
    objc::poolPop(pool);
    return 6;
  }
  // Encode into the device's open command buffer, starting one if there
  // is none. A serial encoder runs each dispatch after the one before, its
  // writes seen, so launches keep their order without a wait between them.
  Device* d = l->device;
  d->pendingLock.lock();
  if (!d->pendingBuffer) {
    objc::id made = objc::send<objc::id>(d->queue, "commandBuffer");
    d->pendingBuffer = objc::send<objc::id>(made, "retain");
    objc::id menc = objc::send<objc::id>(made, "computeCommandEncoder");
    d->pendingEncoder = objc::send<objc::id>(menc, "retain");
  }
  objc::id enc = d->pendingEncoder;
  objc::send<void>(enc, "setComputePipelineState:", p->state);
  for (i64 i = 0; i < l->count; i++) {
    if (l->buffers[i]) {
      objc::send<void>(enc, "setBuffer:offset:atIndex:", l->buffers[i]->metal, static_cast<u64>(l->offsets[i]),
                       static_cast<u64>(i));
    } else {
      objc::send<void>(enc, "setBytes:length:atIndex:", static_cast<const void*>(&l->values[i]),
                       static_cast<u64>(l->offsets[i]), static_cast<u64>(i));
    }
  }
  objc::Size grid = {static_cast<u64>(l->grid[0]), static_cast<u64>(l->grid[1]), static_cast<u64>(l->grid[2])};
  objc::Size group = {static_cast<u64>(l->per[0]), static_cast<u64>(l->per[1]), static_cast<u64>(l->per[2])};
  objc::send<void>(enc, "dispatchThreads:threadsPerThreadgroup:", grid, group);
  d->pendingLock.unlock();
  i32 result = 0;
  // VERTEX_GPU_SYNC=1 waits after every launch, as launches once did: a
  // failure is then reported at the launch that caused it.
  if (vertex_pal_getenv("VERTEX_GPU_SYNC") || vertex_pal_getenv("VERTEX_GPU_DEBUG")) {
    result = syncDevice(d);
    if (vertex_pal_getenv("VERTEX_GPU_DEBUG")) say(result == 0 ? "gpu: metal launch ran\n" : "gpu: metal launch failed\n");
  }
  objc::poolPop(pool);
  return result;
}

void grow(Launch* l) {
  if (l->count < l->capacity) return;
  i64 cap = l->capacity ? l->capacity * 2 : 16;
  void** slots = allocate<void*>(static_cast<usize>(cap));
  u64* values = allocate<u64>(static_cast<usize>(cap));
  Buffer** buffers = allocate<Buffer*>(static_cast<usize>(cap));
  i64* offsets = allocate<i64>(static_cast<usize>(cap));
  for (i64 i = 0; i < l->count; i++) {
    values[i] = l->values[i];
    buffers[i] = l->buffers[i];
    offsets[i] = l->offsets[i];
  }
  if (l->capacity) {
    release(l->slots, sizeof(void*) * l->capacity, 16);
    release(l->values, sizeof(u64) * l->capacity, 16);
    release(l->buffers, sizeof(Buffer*) * l->capacity, 16);
    release(l->offsets, sizeof(i64) * l->capacity, 16);
  }
  l->slots = slots;
  l->values = values;
  l->buffers = buffers;
  l->offsets = offsets;
  l->capacity = cap;
}

void scalar(Launch* l, u64 bits, i64 size) {
  grow(l);
  l->values[l->count] = bits;
  l->buffers[l->count] = nullptr;
  l->offsets[l->count] = size;  // a scalar's slot keeps its size here
  l->count++;
}

// The workgroup a launch runs in when it names none: as Metal would pick
// for a 1D grid, and one row of it for 2D and 3D.
void chooseGroup(Launch* l, i64 wx, i64 wy, i64 wz) {
  if (wx > 0) {
    l->per[0] = wx;
    l->per[1] = wy > 0 ? wy : 1;
    l->per[2] = wz > 0 ? wz : 1;
    return;
  }
  i64 want = 64;
  if (l->grid[1] > 1 || l->grid[2] > 1) {
    l->per[0] = l->grid[0] < 8 ? l->grid[0] : 8;
    l->per[1] = l->grid[1] < 8 ? l->grid[1] : 8;
    l->per[2] = 1;
  } else {
    l->per[0] = l->grid[0] < want ? l->grid[0] : want;
    l->per[1] = 1;
    l->per[2] = 1;
  }
  for (int a = 0; a < 3; a++)
    if (l->per[a] < 1) l->per[a] = 1;
}

}  // namespace

extern "C" {

// ---- devices ----

void* vertex_gpu_device_cpu() { return &cpuDevice; }

void* vertex_gpu_device_default() {
  const char* want = vertex_pal_getenv("VERTEX_GPU");
  if (want && same(want, "cpu")) return &cpuDevice;
  Device* m = metal();
  if (m) return m;
  if (want && same(want, "metal")) {
    say("gpu: VERTEX_GPU=metal, and this machine has no Metal device; using the CPU\n");
  }
  return &cpuDevice;
}

i64 vertex_gpu_device_count() { return metal() ? 2 : 1; }

void* vertex_gpu_device_at(i64 i) {
  Device* m = metal();
  if (m && i == 0) return m;
  return &cpuDevice;
}

i64 vertex_gpu_device_kind(void* d) { return static_cast<Device*>(d)->kind; }
i64 vertex_gpu_device_memory(void* d) { return static_cast<Device*>(d)->memory; }
i64 vertex_gpu_device_wave_size(void* d) { return static_cast<Device*>(d)->waveSize; }

// ---- buffers ----

void* vertex_gpu_buffer_create(void* dev, i64 bytes) {
  Device* d = static_cast<Device*>(dev);
  Buffer* b = allocate<Buffer>(1);
  b->device = d;
  b->bytes = bytes;
  i64 n = bytes > 0 ? bytes : 16;
  if (d->kind == KindMetal) {
    void* pool = objc::poolPush();
    b->metal = objc::send<objc::id>(d->metal, "newBufferWithLength:options:", static_cast<u64>(n), static_cast<u64>(0));
    if (b->metal) b->contents = static_cast<u8*>(objc::send<void*>(b->metal, "contents"));
    objc::poolPop(pool);
    if (!b->metal) {
      release(b, sizeof(Buffer), alignof(Buffer));
      return nullptr;
    }
  } else {
    b->contents = static_cast<u8*>(vertex_pal_alloc(static_cast<usize>(n), 64));
    if (!b->contents) {
      release(b, sizeof(Buffer), alignof(Buffer));
      return nullptr;
    }
    vertex_pal_fill(b->contents, 0, static_cast<usize>(n));
  }
  return b;
}

void vertex_gpu_buffer_release(void* buf) {
  Buffer* b = static_cast<Buffer*>(buf);
  if (!b) return;
  if (b->metal) {
    objc::send<void>(b->metal, "release");
  } else if (b->contents) {
    release(b->contents, static_cast<usize>(b->bytes > 0 ? b->bytes : 16), 64);
  }
  release(b, sizeof(Buffer), alignof(Buffer));
}

// Copies bytes between host memory and a buffer's (after any pending
// launches: callers ask for the buffer's contents first). What Upload and
// Download are, for element types that are plain data.
void vertex_gpu_copy(void* dst, const void* src, i64 bytes) {
  if (bytes > 0) vertex_pal_copy(dst, src, static_cast<usize>(bytes));
}

// The host's view of a buffer's memory. What was launched before is run
// first, so the host reads what it wrote and writes nothing a pending
// launch has yet to read.
void* vertex_gpu_buffer_contents(void* buf) {
  Buffer* b = static_cast<Buffer*>(buf);
  if (syncDevice(b->device) != 0) vertex_pal_abort();
  return b->contents;
}
void* vertex_gpu_buffer_device(void* buf) { return static_cast<Buffer*>(buf)->device; }

// ---- launches ----

void* vertex_gpu_launch_begin(const void* kernel) {
  Launch* l = allocate<Launch>(1);
  l->kernel = static_cast<const vertex_gpu_kernel*>(kernel);
  return l;
}

void vertex_gpu_launch_device(void* launch, void* dev) {
  Launch* l = static_cast<Launch*>(launch);
  if (l->device && l->device != dev) l->mismatched = true;
  l->device = static_cast<Device*>(dev);
}

void vertex_gpu_launch_buffer(void* launch, void* buf, i64 offset) {
  Launch* l = static_cast<Launch*>(launch);
  Buffer* b = static_cast<Buffer*>(buf);
  vertex_gpu_launch_device(launch, b->device);
  grow(l);
  l->values[l->count] = reinterpret_cast<u64>(b->contents + offset);
  l->buffers[l->count] = b;
  l->offsets[l->count] = offset;
  l->count++;
}

void vertex_gpu_launch_i8(void* l, i8 v) { scalar(static_cast<Launch*>(l), static_cast<u8>(v), 1); }
void vertex_gpu_launch_i16(void* l, i16 v) { scalar(static_cast<Launch*>(l), static_cast<u16>(v), 2); }
void vertex_gpu_launch_i32(void* l, i32 v) { scalar(static_cast<Launch*>(l), static_cast<u32>(v), 4); }
void vertex_gpu_launch_i64(void* l, i64 v) { scalar(static_cast<Launch*>(l), static_cast<u64>(v), 8); }
void vertex_gpu_launch_f32(void* l, float v) {
  u32 bits;
  vertex_pal_copy(&bits, &v, 4);
  scalar(static_cast<Launch*>(l), bits, 4);
}
void vertex_gpu_launch_f64(void* l, double v) {
  u64 bits;
  vertex_pal_copy(&bits, &v, 8);
  scalar(static_cast<Launch*>(l), bits, 8);
}

// Ends the launch: runs it over the grid and waits. 0 when it ran; 1
// when a work-item trapped; otherwise the device refused it.
i32 vertex_gpu_launch_end(void* launch, i64 gx, i64 gy, i64 gz, i64 wx, i64 wy, i64 wz) {
  Launch* l = static_cast<Launch*>(launch);
  i32 result = 0;
  if (!l->device) l->device = static_cast<Device*>(vertex_gpu_device_default());
  if (l->mismatched) {
    say("gpu: a launch with buffers of two devices\n");
    result = 8;
  } else if (l->count != l->kernel->params) {
    say("gpu: a launch with the wrong number of arguments for its kernel\n");
    result = 9;
  } else if (gx > 0 && gy > 0 && gz > 0) {
    l->grid[0] = gx;
    l->grid[1] = gy;
    l->grid[2] = gz;
    chooseGroup(l, wx, wy, wz);
    for (int a = 0; a < 3; a++) l->groups[a] = (l->grid[a] + l->per[a] - 1) / l->per[a];
    for (i64 i = 0; i < l->count; i++) l->slots[i] = &l->values[i];
    // A kernel with no Metal image -- one AIR cannot express, as a
    // float64 on Apple's GPUs -- runs on the CPU: Metal's buffers are
    // host memory on Apple's machines, so its arguments already are.
    if (l->device->kind == KindMetal && l->kernel->metallib) {
      result = runMetal(l);
    } else {
      result = runCPU(l);
    }
  }
  if (l->capacity) {
    release(l->slots, sizeof(void*) * l->capacity, 16);
    release(l->values, sizeof(u64) * l->capacity, 16);
    release(l->buffers, sizeof(Buffer*) * l->capacity, 16);
    release(l->offsets, sizeof(i64) * l->capacity, 16);
  }
  release(l, sizeof(Launch), alignof(Launch));
  return result;
}

// The declaration a launch names its kernel by. The compiler answers
// each use of it with the kernel's descriptor, so nothing calls it.
void* vertex_gpu_kernel_descriptor() { return nullptr; }

// ---- the device intrinsics, for the CPU device ----

i32 vertex_gpu_workitem_id_x() { return current()->local[0]; }
i32 vertex_gpu_workitem_id_y() { return current()->local[1]; }
i32 vertex_gpu_workitem_id_z() { return current()->local[2]; }
i32 vertex_gpu_workgroup_id_x() { return current()->group[0]; }
i32 vertex_gpu_workgroup_id_y() { return current()->group[1]; }
i32 vertex_gpu_workgroup_id_z() { return current()->group[2]; }
i32 vertex_gpu_workgroup_size_x() { return current()->size[0]; }
i32 vertex_gpu_workgroup_size_y() { return current()->size[1]; }
i32 vertex_gpu_workgroup_size_z() { return current()->size[2]; }
i32 vertex_gpu_num_workgroups_x() { return current()->groups[0]; }
i32 vertex_gpu_num_workgroups_y() { return current()->groups[1]; }
i32 vertex_gpu_num_workgroups_z() { return current()->groups[2]; }

// A wave on the CPU is one work-item wide.
i32 vertex_gpu_lane_id() { return 0; }
i32 vertex_gpu_wave_size() { return 1; }
u32 vertex_gpu_shfl_idx(u32 v, i32) { return v; }
u32 vertex_gpu_shfl_up(u32 v, i32) { return v; }
u32 vertex_gpu_shfl_down(u32 v, i32) { return v; }
u32 vertex_gpu_shfl_xor(u32 v, i32) { return v; }
u32 vertex_gpu_readfirstlane(u32 v) { return v; }
u64 vertex_gpu_ballot(bool c) { return c ? 1 : 0; }
bool vertex_gpu_wave_any(bool c) { return c; }
bool vertex_gpu_wave_all(bool c) { return c; }

// Every work-item of the group arrives before any leaves; a group
// without barriers runs its work-items one after another, where there is
// nobody to wait for.
void vertex_gpu_barrier() {
  WorkItem* w = current();
  if (!(w->launch->kernel->flags & kernelUsesBarrier)) return;
#if VERTEX_GPU_FIBERS
  // The next fiber runs; this one resumes when every fiber of the group
  // has reached a barrier.
  vertex_gpu_switch(&w->fiber->sp, w->fiber->scheduler);
  vertex_pal_thread_set(w);
  return;
#else
  Group* g = w->g;
  u32 phase = __builtin_atomic_load(&g->phase);
  u32 arrived = __builtin_atomic_add(&g->arrived, 1u) + 1;
  if (arrived >= __builtin_atomic_load(&g->members)) {
    __builtin_atomic_store(&g->arrived, 0u);
    __builtin_atomic_add(&g->phase, 1u);
    return;
  }
  int spins = 0;
  while (__builtin_atomic_load(&g->phase) == phase) {
    if (++spins > 64) vertex_pal_sleep(spins > 2048 ? 20000 : 0);
  }
#endif
}

// The group's workgroup storage. A GPU gives each place in a kernel that
// asks for storage (gpu.Shared) one allocation per group, however often
// that place runs -- a group reduction called in a loop reuses its
// storage every time round, as it does on Metal, where each place is a
// shared variable of its own. Here a place is known by where it called
// from: the frame record chain names the return address into the code
// that constructed the gpu.Shared, which differs for every place in the
// program. The first work-item to reach a place allocates for it; the
// rest find it.
void* vertex_gpu_shared(i64 bytes, i64 align) {
  WorkItem* w = current();
  Group* g = w->g;
  if (align < 1) align = 1;
  // This function's frame record, then the caller's (gpu.Shared's
  // initializer): the return address it holds is the place.
  void** frame = static_cast<void**>(__builtin_frame_address(0));
  void** up = frame ? static_cast<void**>(frame[0]) : nullptr;
  void* key = up ? up[1] : __builtin_return_address(0);
  constexpr i64 limit = 64 * 1024;
  g->siteLock.lock();
  if (!g->shared) {
    u8* made = static_cast<u8*>(vertex_pal_alloc(limit, 16));
    vertex_pal_fill(made, 0, limit);
    g->shared = made;
    g->sharedSize = limit;
  }
  i64 at = -1;
  for (u32 i = 0; i < g->siteCount; i++) {
    if (g->sites[i].key == key) {
      at = g->sites[i].at;
      break;
    }
  }
  if (at < 0) {
    at = (g->sharedUsed + align - 1) / align * align;
    if (at + bytes > limit || g->siteCount >= maxSharedSites) {
      g->siteLock.unlock();
      say("gpu: more than 64 KB of shared storage in one workgroup\n");
      vertex_pal_abort();
    }
    g->sharedUsed = at + bytes;
    g->sites[g->siteCount++] = SharedSite{key, at};
  }
  g->siteLock.unlock();
  return g->shared + at;
}

// A work-item that traps ends: the launch is marked, the work-item's
// group stops counting it, and its thread parks for good. The worker
// stays busy, so it is never handed work again, and another is started
// for the next launch that needs one.
[[noreturn]] void vertex_gpu_trap() {
  WorkItem* w = current();
  Launch* l = w->launch;
  __builtin_atomic_store(&l->trapped, 1u);
#if VERTEX_GPU_FIBERS
  // On fibers, the work-item ends and the rest of its group goes on; the
  // worker counts the group done when every fiber has ended.
  if (l->kernel->flags & kernelUsesBarrier) {
    w->fiber->done = true;
    vertex_gpu_switch(&w->fiber->sp, w->fiber->scheduler);
  }
#endif
  Group* g = w->g;
  if (l->kernel->flags & kernelUsesBarrier) {
    __builtin_atomic_sub(&g->members, 1u);
    // Wake a barrier the others are waiting at, if they all are now.
    if (__builtin_atomic_load(&g->arrived) >= __builtin_atomic_load(&g->members) &&
        __builtin_atomic_load(&g->members) > 0) {
      __builtin_atomic_store(&g->arrived, 0u);
      __builtin_atomic_add(&g->phase, 1u);
    }
    __builtin_atomic_add(&l->finished, 1u);
  } else {
    // The group this worker was running is abandoned: count it done, and
    // take no more.
    __builtin_atomic_add(&l->finished, 1u);
  }
  for (;;) vertex_pal_sleep(1000000000ull);
}

// Atomics on device memory. On the CPU device, work-items are threads.
i32 vertex_gpu_atomic_add_i32(i32* p, i32 v) {
  return static_cast<i32>(__builtin_atomic_add(reinterpret_cast<u32*>(p), static_cast<u32>(v)));
}
i32 vertex_gpu_atomic_sub_i32(i32* p, i32 v) {
  return static_cast<i32>(__builtin_atomic_sub(reinterpret_cast<u32*>(p), static_cast<u32>(v)));
}
u32 vertex_gpu_atomic_add_u32(u32* p, u32 v) { return __builtin_atomic_add(p, v); }

template <typename F>
u32 casLoop(u32* p, F f) {
  u32 was = __builtin_atomic_load(p);
  for (;;) {
    u32 want = f(was);
    u32 seen = __builtin_atomic_cas(p, was, want);
    if (seen == was) return was;
    was = seen;
  }
}

i32 vertex_gpu_atomic_min_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32 was) {
    return static_cast<i32>(was) < v ? was : static_cast<u32>(v);
  }));
}
i32 vertex_gpu_atomic_max_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32 was) {
    return static_cast<i32>(was) > v ? was : static_cast<u32>(v);
  }));
}
u32 vertex_gpu_atomic_min_u32(u32* p, u32 v) {
  return casLoop(p, [v](u32 was) { return was < v ? was : v; });
}
u32 vertex_gpu_atomic_max_u32(u32* p, u32 v) {
  return casLoop(p, [v](u32 was) { return was > v ? was : v; });
}
i32 vertex_gpu_atomic_and_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32 was) { return was & static_cast<u32>(v); }));
}
i32 vertex_gpu_atomic_or_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32 was) { return was | static_cast<u32>(v); }));
}
i32 vertex_gpu_atomic_xor_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32 was) { return was ^ static_cast<u32>(v); }));
}
i32 vertex_gpu_atomic_xchg_i32(i32* p, i32 v) {
  return static_cast<i32>(casLoop(reinterpret_cast<u32*>(p), [v](u32) { return static_cast<u32>(v); }));
}
i32 vertex_gpu_atomic_cas_i32(i32* p, i32 expected, i32 desired) {
  return static_cast<i32>(__builtin_atomic_cas(reinterpret_cast<u32*>(p), static_cast<u32>(expected),
                                               static_cast<u32>(desired)));
}
float vertex_gpu_atomic_add_f32(float* p, float v) {
  u32 was = casLoop(reinterpret_cast<u32*>(p), [v](u32 bits) {
    float f;
    vertex_pal_copy(&f, &bits, 4);
    f += v;
    u32 out;
    vertex_pal_copy(&out, &f, 4);
    return out;
  });
  float f;
  vertex_pal_copy(&f, &was, 4);
  return f;
}

}  // extern "C"
