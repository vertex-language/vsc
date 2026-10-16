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
//   - CUDA, opened the same way (libcuda.so.1, the driver API) where the
//     target is not Apple's. A kernel is the PTX the compiler embedded
//     beside it, which the driver compiles the first time it is launched.
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
  const char* label;         // the kernel's own (mangled) name
  const char* ptx;           // null where there is no PTX image
  i64 ptxSize;
};
}

namespace {

constexpr i64 kernelUsesBarrier = 1;

enum Kind : i64 { KindCPU = 0, KindMetal = 1, KindCUDA = 2 };

struct Device;

struct Buffer {
  Device* device;
  // Host-visible, on every kind: Apple's memory is unified, and CUDA's
  // is managed -- one address the host and the device both use.
  u8* contents;
  i64 bytes;
  void* metal;  // id<MTLBuffer>, or null on the CPU
  bool wrapped;  // contents belong to the caller (a mapped file): not freed
  bool cudaOwned;  // contents are CUDA managed memory, freed by the driver
};

// A pipeline made for one kernel on one Metal or CUDA device.
struct Pipeline {
  const vertex_gpu_kernel* kernel;
  void* state;  // id<MTLComputePipelineState>, or a CUfunction
  i64 maxThreads;
  i64 width;
  Pipeline* next;
  void* module;  // the CUmodule the CUfunction is in
};

// How many command buffers a device keeps committed and not waited for.
constexpr int kCommitted = 16;
// The most launches a command buffer takes before it is committed.
constexpr i64 kFlushMost = 64;
// How many timestamps VERTEX_GPU_PROFILE=3 takes between syncs.
constexpr int kKernelSamples = 4096;

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
  i64 pendingLaunches;   // encoded into pendingBuffer
  i64 flushAt;           // launches the open buffer takes before it is committed
  // Command buffers committed but not yet waited for, oldest first: every
  // few launches the open one is committed, so the device runs a chain
  // while the host encodes the rest of it (see runMetal).
  void* committed[kCommitted];
  int committedCount;
  Lock pendingLock;
  // Render passes not yet run: one open command buffer, retained, that
  // passes encode into; renderFlush commits it.
  void* renderBuffer;  // id<MTLCommandBuffer>
  void* renderLast;    // the last one committed, until it's waited for
  // CUDA: the device's primary context. Launches go on its null stream,
  // in order, and pendingLaunches counts those not yet waited for.
  void* cuda;  // CUcontext
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

// ---- CUDA, through the driver API ----

namespace cuda {

using Result = i32;

Result (*init)(u32);
Result (*deviceGetCount)(i32*);
Result (*deviceGet)(i32*, i32);
Result (*deviceGetAttribute)(i32*, i32, i32);
Result (*deviceTotalMem)(usize*, i32);
Result (*primaryCtxRetain)(void**, i32);
Result (*ctxSetCurrent)(void*);
Result (*ctxSynchronize)();
Result (*memAllocManaged)(u64*, usize, u32);
Result (*memsetD8)(u64, u8, usize);  // optional: zeroing on the device
Result (*memFree)(u64);
Result (*memcpyHtoD)(u64, const void*, usize);
Result (*pointerGetAttribute)(void*, i32, u64);
Result (*moduleLoadDataEx)(void**, const void*, u32, i32*, void**);
Result (*moduleGetFunction)(void**, void*, const char*);
Result (*funcGetAttribute)(i32*, i32, void*);
Result (*launchKernel)(void*, u32, u32, u32, u32, u32, u32, u32, void*, void**, void**);
Result (*getErrorString)(Result, const char**);

constexpr i32 attrWarpSize = 10;
constexpr i32 funcMaxThreadsPerBlock = 0;
constexpr i32 pointerIsManaged = 8;
constexpr u32 memAttachGlobal = 1;
constexpr i32 jitErrorLogBuffer = 5;
constexpr i32 jitErrorLogBufferSize = 6;

bool loaded = false;
bool available = false;

template <typename F>
bool find(void* lib, const char* name, F* f) {
  *f = reinterpret_cast<F>(dlsym(lib, name));
  return *f != nullptr;
}

bool load() {
  if (loaded) return available;
  loaded = true;
  void* lib = dlopen("libcuda.so.1", 2);
  if (!lib) lib = dlopen("libcuda.so", 2);
  if (!lib) {
    if (vertex_pal_getenv("VERTEX_GPU_DEBUG")) say("gpu: dlopen libcuda failed\n");
    return false;
  }
  available = find(lib, "cuInit", &init) && find(lib, "cuDeviceGetCount", &deviceGetCount) &&
              find(lib, "cuDeviceGet", &deviceGet) && find(lib, "cuDeviceGetAttribute", &deviceGetAttribute) &&
              find(lib, "cuDeviceTotalMem_v2", &deviceTotalMem) &&
              find(lib, "cuDevicePrimaryCtxRetain", &primaryCtxRetain) &&
              find(lib, "cuCtxSetCurrent", &ctxSetCurrent) && find(lib, "cuCtxSynchronize", &ctxSynchronize) &&
              find(lib, "cuMemAllocManaged", &memAllocManaged) && find(lib, "cuMemFree_v2", &memFree) &&
              find(lib, "cuMemcpyHtoD_v2", &memcpyHtoD) && find(lib, "cuModuleLoadDataEx", &moduleLoadDataEx) &&
              find(lib, "cuPointerGetAttribute", &pointerGetAttribute) &&
              find(lib, "cuModuleGetFunction", &moduleGetFunction) &&
              find(lib, "cuFuncGetAttribute", &funcGetAttribute) && find(lib, "cuLaunchKernel", &launchKernel) &&
              find(lib, "cuGetErrorString", &getErrorString);
  if (!available) {
    say("gpu: libcuda is here, but not the driver API it expects\n");
    return false;
  }
  find(lib, "cuMemsetD8_v2", &memsetD8);
  if (init(0) != 0) {
    if (vertex_pal_getenv("VERTEX_GPU_DEBUG")) say("gpu: cuInit failed\n");
    available = false;
  }
  return available;
}

// report says what failed, and the driver's word for why.
void report(const char* what, Result r) {
  say("gpu: ");
  say(what);
  const char* s = nullptr;
  if (getErrorString && getErrorString(r, &s) == 0 && s) {
    say(": ");
    say(s);
  }
  say("\n");
}

}  // namespace cuda

// ---- devices ----

Device cpuDevice = {KindCPU, nullptr, nullptr, 0, 1, nullptr, {}};
Device metalDevice = {KindMetal, nullptr, nullptr, 0, 32, nullptr, {}};
Device cudaDevice = {KindCUDA, nullptr, nullptr, 0, 32, nullptr, {}};
bool metalTried = false;
bool cudaTried = false;
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

// The first CUDA device, in its primary context; null where there is none.
Device* nvidia() {
  devicesLock.lock();
  if (!cudaTried) {
    cudaTried = true;
    i32 n = 0;
    i32 dev = 0;
    void* ctx = nullptr;
    if (cuda::load() && cuda::deviceGetCount(&n) == 0 && n > 0 && cuda::deviceGet(&dev, 0) == 0 &&
        cuda::primaryCtxRetain(&ctx, dev) == 0) {
      usize bytes = 0;
      i32 warp = 32;
      cuda::ctxSetCurrent(ctx);
      cuda::deviceTotalMem(&bytes, dev);
      cuda::deviceGetAttribute(&warp, cuda::attrWarpSize, dev);
      // Set here, not only where cudaDevice is defined: what a static
      // initializer leaves is not every target's to rely on.
      cudaDevice.kind = KindCUDA;
      cudaDevice.cuda = ctx;
      cudaDevice.memory = static_cast<i64>(bytes);
      cudaDevice.waveSize = warp;
    }
  }
  devicesLock.unlock();
  return cudaDevice.cuda ? &cudaDevice : nullptr;
}

// The device a GPU is: Metal's where there is one, else CUDA's.
Device* gpuDevice() {
  Device* m = metal();
  return m ? m : nvidia();
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

#if defined(__x86_64__)
// The frame vertex_gpu_switch pops: MXCSR and the x87 control word, R15
// to R12, RBX, RBP, and the return address -- and a word above it in a
// fiber's first, so that it starts with its stack as a call leaves it.
constexpr usize switchFrame = 72;
constexpr usize switchReturn = 56;  // where the return address is in it
#else
// The frame vertex_gpu_switch pops: x19-x28, x29, x30, d8-d15.
constexpr usize switchFrame = 160;
constexpr usize switchReturn = 88;  // where x30 is in it
#endif

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
#if defined(__x86_64__)
        // The control words a thread starts with: every floating-point
        // exception masked, round to nearest.
        *reinterpret_cast<u32*>(top) = 0x1f80;
        *reinterpret_cast<u16*>(top + 4) = 0x037f;
#endif
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
          if (vertex_pal_getenv("VERTEX_GPU_PROFILE")) {
            // How many threads a group of it may have: fewer, the more
            // registers it takes.
            say("gpu-pipeline ");
            say(k->label ? k->label : k->name);
            char buf[16];
            int n = 0;
            char digits[12];
            i64 v = made->maxThreads;
            do { digits[n++] = static_cast<char>('0' + v % 10); v /= 10; } while (v > 0);
            int at = 0;
            buf[at++] = ' ';
            while (n > 0) buf[at++] = digits[--n];
            buf[at++] = '\n';
            buf[at] = 0;
            say(buf);
          }
        }
      }
    }
  }
  d->lock.unlock();
  return made;
}

// The environment's say over launches, read once: VERTEX_GPU_PROFILE,
// VERTEX_GPU_SYNC, VERTEX_GPU_DEBUG, and VERTEX_GPU_FLUSH -- how many
// launches the first command buffer after a sync takes before it is
// committed (each after takes twice as many as the one before, up to
// kFlushMost).
struct Settings {
  bool read;
  char profile;  // '1', '2' or 0
  bool sync;
  bool debug;
  i64 flushEvery;
};
Settings settings;

const Settings& env() {
  if (!settings.read) {
    const char* p = vertex_pal_getenv("VERTEX_GPU_PROFILE");
    settings.profile = p ? p[0] : 0;
    settings.debug = vertex_pal_getenv("VERTEX_GPU_DEBUG") != nullptr;
    settings.sync = vertex_pal_getenv("VERTEX_GPU_SYNC") != nullptr || settings.debug;
    settings.flushEvery = 4;
    if (const char* f = vertex_pal_getenv("VERTEX_GPU_FLUSH")) {
      i64 n = 0;
      for (; *f >= '0' && *f <= '9'; f++) n = n * 10 + (*f - '0');
      if (n > 0) settings.flushEvery = n;
    }
    settings.read = true;
  }
  return settings;
}

// commitPendingLocked commits d's open command buffer, if it has one,
// without waiting, and adds it to those committed. pendingLock is held.
void commitPendingLocked(Device* d) {
  objc::id cb = d->pendingBuffer;
  if (!cb) return;
  objc::id enc = d->pendingEncoder;
  d->pendingBuffer = nullptr;
  d->pendingEncoder = nullptr;
  d->pendingLaunches = 0;
  if (enc) {
    objc::send<void>(enc, "endEncoding");
    objc::send<void>(enc, "release");
  }
  objc::send<void>(cb, "commit");
  d->committed[d->committedCount++] = cb;
}

// waitCommitted waits for command buffers committed to one queue, oldest
// first -- they run in order, so waiting for the last is waiting for all
// -- and releases them. 0 when they ran; 7 when one failed.
i32 waitCommitted(objc::id* cbs, int n) {
  if (n == 0) return 0;
  objc::send<void>(cbs[n - 1], "waitUntilCompleted");
  i32 result = 0;
  i64 us = 0;
  for (int i = 0; i < n; i++) {
    objc::id cb = cbs[i];
    // A buffer before the last may still be completing its handlers.
    if (i < n - 1) objc::send<void>(cb, "waitUntilCompleted");
    i64 status = static_cast<i64>(objc::send<u64>(cb, "status"));
    if (status == 5 && result == 0) {  // MTLCommandBufferStatusError
      objc::report("a launch failed", objc::send<objc::id>(cb, "error"));
      result = 7;
    }
    if (env().profile == '2') {
      double start = objc::send<double>(cb, "GPUStartTime");
      double end = objc::send<double>(cb, "GPUEndTime");
      us += static_cast<i64>((end - start) * 1e6);
    }
    objc::send<void>(cb, "release");
  }
  // VERTEX_GPU_PROFILE=2: the device time of the command buffers a sync
  // waited for, as "gpu-profile commit <microseconds>".
  if (env().profile == '2') {
    char buf[32];
    char digits[24];
    int n = 0;
    if (us < 0) us = 0;
    do { digits[n++] = static_cast<char>('0' + us % 10); us /= 10; } while (us > 0);
    int at = 0;
    while (n > 0) buf[at++] = digits[--n];
    buf[at++] = '\n';
    buf[at] = 0;
    say("gpu-profile commit ");
    say(buf);
  }
  return result;
}

// VERTEX_GPU_PROFILE=3: each launch in an encoder of its own whose start
// and end the device timestamps into a counter sample buffer, so each
// kernel's device time is measured where it runs -- in the model's chain,
// at the clocks the chain runs at. At each sync, every launch's time is
// said as "gpu-kernel <kernel> <work-items> <nanoseconds>".
struct KernelTimes {
  objc::id samples;  // id<MTLCounterSampleBuffer>, two samples a launch
  i64 used;
  bool tried;
  const char* names[kKernelSamples / 2];
  i64 grids[kKernelSamples / 2];  // each launch's work-items
};
KernelTimes kernelTimes;

struct Range {
  u64 location, length;
};

objc::id kernelSamples(Device* d) {
  if (kernelTimes.tried) return kernelTimes.samples;
  kernelTimes.tried = true;
  if (!objc::send<bool>(d->metal, "supportsCounterSampling:", static_cast<u64>(0))) {  // at stage boundaries
    say("gpu: this device cannot timestamp an encoder's start and end\n");
    return nullptr;
  }
  objc::id sets = objc::send<objc::id>(d->metal, "counterSets");
  objc::id timestamps = nullptr;
  u64 n = sets ? objc::send<u64>(sets, "count") : 0;
  for (u64 i = 0; i < n; i++) {
    objc::id set = objc::send<objc::id>(sets, "objectAtIndex:", i);
    const char* name = objc::send<const char*>(objc::send<objc::id>(set, "name"), "UTF8String");
    if (name && name[0] == 't' && name[1] == 'i' && name[2] == 'm' && name[3] == 'e') timestamps = set;
  }
  if (!timestamps) {
    say("gpu: no timestamp counter set\n");
    return nullptr;
  }
  objc::id desc = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLCounterSampleBufferDescriptor"), "alloc"), "init");
  objc::send<void>(desc, "setCounterSet:", timestamps);
  objc::send<void>(desc, "setStorageMode:", static_cast<u64>(0));
  objc::send<void>(desc, "setSampleCount:", static_cast<u64>(kKernelSamples));
  objc::id error = nullptr;
  kernelTimes.samples = objc::send<objc::id>(d->metal, "newCounterSampleBufferWithDescriptor:error:", desc, &error);
  if (!kernelTimes.samples) objc::report("newCounterSampleBufferWithDescriptor", error);
  return kernelTimes.samples;
}

// sayKernelTimes says the times of the launches sampled since the last
// call, all of which have run, and starts the buffer over.
void sayKernelTimes() {
  if (!kernelTimes.samples || kernelTimes.used == 0) return;
  Range r = {0, static_cast<u64>(kernelTimes.used)};
  objc::id data = objc::send<objc::id>(kernelTimes.samples, "resolveCounterRange:", r);
  const u64* ts = data ? static_cast<const u64*>(objc::send<const void*>(data, "bytes")) : nullptr;
  for (i64 e = 0; ts && e < kernelTimes.used / 2; e++) {
    u64 a = ts[2 * e], b = ts[2 * e + 1];
    if (a == ~0ull || b == ~0ull || b < a) continue;
    // "gpu-kernel <kernel> <work-items> <nanoseconds>"
    char buf[64];
    int at = 0;
    u64 vals[2] = {static_cast<u64>(kernelTimes.grids[e]), b - a};
    for (int f = 0; f < 2; f++) {
      char digits[24];
      int n = 0;
      u64 v = vals[f];
      do { digits[n++] = static_cast<char>('0' + v % 10); v /= 10; } while (v > 0);
      buf[at++] = ' ';
      while (n > 0) buf[at++] = digits[--n];
    }
    buf[at++] = '\n';
    buf[at] = 0;
    say("gpu-kernel ");
    say(kernelTimes.names[e]);
    say(buf);
  }
  kernelTimes.used = 0;
}

i32 syncCUDA(Device* d);

// syncDevice runs what has been encoded on d and waits for it, and for
// what was committed before: what the host reads from a buffer, or
// writes to one, is then what the launches before made of it. 0 when
// they ran; 7 when one failed.
i32 syncDevice(Device* d) {
  if (d && d->kind == KindCUDA) return syncCUDA(d);
  if (!d || d->kind != KindMetal) return 0;
  objc::id cbs[kCommitted];
  d->pendingLock.lock();
  if (!d->pendingBuffer && d->committedCount == 0) {
    d->pendingLock.unlock();
    return 0;
  }
  void* pool = objc::poolPush();
  commitPendingLocked(d);
  d->flushAt = env().flushEvery;
  int n = d->committedCount;
  for (int i = 0; i < n; i++) cbs[i] = d->committed[i];
  d->committedCount = 0;
  d->pendingLock.unlock();
  i32 result = waitCommitted(cbs, n);
  if (env().profile == '3') sayKernelTimes();
  objc::poolPop(pool);
  return result;
}

i32 runMetal(Launch* l) {
  void* pool = objc::poolPush();
  Pipeline* p = pipelineFor(l->device, l->kernel);
  if (env().debug) say(p ? "gpu: pipeline made\n" : "gpu: no pipeline\n");
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
  }
  if (!d->pendingEncoder) {
    // A serial encoder: each dispatch runs after the one before, which is
    // what a model's chain of dependent launches needs. (A concurrent one
    // with a barrier before every dispatch was measured slower on such a
    // chain, though faster for independent ones.)
    objc::id samples = env().profile == '3' ? kernelSamples(d) : nullptr;
    objc::id menc = nullptr;
    if (samples && kernelTimes.used + 2 <= kKernelSamples) {
      objc::id desc = objc::send<objc::id>(objc::getClass("MTLComputePassDescriptor"), "computePassDescriptor");
      objc::id att = objc::send<objc::id>(objc::send<objc::id>(desc, "sampleBufferAttachments"), "objectAtIndexedSubscript:", static_cast<u64>(0));
      objc::send<void>(att, "setSampleBuffer:", samples);
      objc::send<void>(att, "setStartOfEncoderSampleIndex:", static_cast<u64>(kernelTimes.used));
      objc::send<void>(att, "setEndOfEncoderSampleIndex:", static_cast<u64>(kernelTimes.used + 1));
      kernelTimes.names[kernelTimes.used / 2] = l->kernel->label ? l->kernel->label : l->kernel->name;
      kernelTimes.grids[kernelTimes.used / 2] = l->grid[0] * l->grid[1] * l->grid[2];
      kernelTimes.used += 2;
      menc = objc::send<objc::id>(d->pendingBuffer, "computeCommandEncoderWithDescriptor:", desc);
    } else {
      menc = objc::send<objc::id>(d->pendingBuffer, "computeCommandEncoder");
    }
    d->pendingEncoder = objc::send<objc::id>(menc, "retain");
    if (env().profile == '3') {
      // VERTEX_GPU_PROFILE=3: an encoder a launch, named for its kernel,
      // in the command buffers launches go in anyway -- for Instruments'
      // Metal System Trace to time each kernel where it runs.
      objc::id name = objc::send<objc::id>(objc::getClass("NSString"), "stringWithUTF8String:",
                                           l->kernel->label ? l->kernel->label : l->kernel->name);
      objc::send<void>(menc, "setLabel:", name);
    }
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
  if (env().profile == '3') {
    objc::send<void>(enc, "endEncoding");
    objc::send<void>(enc, "release");
    d->pendingEncoder = nullptr;
  }
  // Every few launches the open command buffer is committed, without a
  // wait: the device starts on a chain while the host is still encoding
  // the rest of it, rather than sitting idle until the host reads a
  // result. Buffers on one queue run in order, so the chain keeps its.
  // The first buffer after a sync is short, to start the device soon;
  // each after is twice as long, since the device pauses between two
  // buffers (some tens of microseconds on Apple's) and a decoded token
  // is a hundred launches or so.
  if (d->flushAt == 0) d->flushAt = env().flushEvery;
  if (++d->pendingLaunches >= d->flushAt && env().profile != '1') {
    d->flushAt = d->flushAt * 2 < kFlushMost ? d->flushAt * 2 : kFlushMost;
    objc::id oldest = nullptr;
    if (d->committedCount == kCommitted - 1) {
      // Full but for the one a sync commits: wait for the oldest, which
      // has most likely run by now -- not for all, which would leave the
      // device idle while the host encodes the next few.
      oldest = d->committed[0];
      for (int i = 1; i < d->committedCount; i++) d->committed[i - 1] = d->committed[i];
      d->committedCount--;
    }
    commitPendingLocked(d);
    if (oldest) {
      d->pendingLock.unlock();
      i32 r = waitCommitted(&oldest, 1);
      objc::poolPop(pool);
      return r;
    }
  }
  d->pendingLock.unlock();
  i32 result = 0;
  // VERTEX_GPU_PROFILE=1: each launch alone in its command buffer, its
  // device time said: "gpu-profile <kernel> <microseconds>".
  if (env().profile == '1') {
    d->pendingLock.lock();
    objc::id pcb = d->pendingBuffer;
    objc::id penc = d->pendingEncoder;
    d->pendingBuffer = nullptr;
    d->pendingEncoder = nullptr;
    d->pendingLaunches = 0;
    d->pendingLock.unlock();
    objc::send<void>(penc, "endEncoding");
    objc::send<void>(pcb, "commit");
    objc::send<void>(pcb, "waitUntilCompleted");
    double start = objc::send<double>(pcb, "GPUStartTime");
    double end = objc::send<double>(pcb, "GPUEndTime");
    i64 tenths = static_cast<i64>((end - start) * 1e7);  // tenths of a microsecond
    if (tenths < 0) tenths = 0;
    say("gpu-profile ");
    say(l->kernel->label ? l->kernel->label : l->kernel->name);
    char buf[32];
    char digits[24];
    int n = 0;
    i64 whole = tenths / 10;
    do { digits[n++] = static_cast<char>('0' + whole % 10); whole /= 10; } while (whole > 0);
    int at = 0;
    buf[at++] = ' ';
    while (n > 0) buf[at++] = digits[--n];
    buf[at++] = '.';
    buf[at++] = static_cast<char>('0' + tenths % 10);
    buf[at++] = '\n';
    buf[at] = 0;
    say(buf);
    objc::send<void>(penc, "release");
    objc::send<void>(pcb, "release");
  }
  // VERTEX_GPU_SYNC=1 waits after every launch, as launches once did: a
  // failure is then reported at the launch that caused it.
  if (env().sync) {
    result = syncDevice(d);
    if (env().debug) say(result == 0 ? "gpu: metal launch ran\n" : "gpu: metal launch failed\n");
  }
  objc::poolPop(pool);
  return result;
}

// ---- running on CUDA ----

// bind makes d's context the calling thread's: an async function may
// resume on any thread.
void bind(Device* d) { cuda::ctxSetCurrent(d->cuda); }

// syncCUDA waits for what was launched on d. 0 when it ran; 7 when one
// failed -- a trap, or a fault, after which the context is lost.
i32 syncCUDA(Device* d) {
  d->pendingLock.lock();
  if (d->pendingLaunches == 0) {
    d->pendingLock.unlock();
    return 0;
  }
  bind(d);
  cuda::Result r = cuda::ctxSynchronize();
  d->pendingLaunches = 0;
  d->pendingLock.unlock();
  if (r != 0) {
    cuda::report("a launch failed", r);
    return 7;
  }
  return 0;
}

// cudaKernelFor is k loaded on d: its PTX compiled by the driver, the
// first time it is asked for. Null where it cannot be.
Pipeline* cudaKernelFor(Device* d, const vertex_gpu_kernel* k) {
  d->lock.lock();
  for (Pipeline* p = d->pipelines; p; p = p->next) {
    if (p->kernel == k) {
      d->lock.unlock();
      return p;
    }
  }
  Pipeline* made = nullptr;
  bind(d);
  char log[8192];
  log[0] = 0;
  i32 options[2] = {cuda::jitErrorLogBuffer, cuda::jitErrorLogBufferSize};
  void* values[2] = {log, reinterpret_cast<void*>(static_cast<u64>(sizeof(log)))};
  void* module = nullptr;
  void* fn = nullptr;
  cuda::Result r = cuda::moduleLoadDataEx(&module, k->ptx, 2, options, values);
  if (r != 0) {
    cuda::report("the driver would not compile a kernel's PTX", r);
    say(k->label ? k->label : k->name);
    say("\n");
    say(log);
    say("\n");
  } else if ((r = cuda::moduleGetFunction(&fn, module, k->name)) != 0) {
    cuda::report("the PTX has no entry", r);
  } else {
    made = allocate<Pipeline>(1);
    made->kernel = k;
    made->state = fn;
    made->module = module;
    i32 most = 1024;
    cuda::funcGetAttribute(&most, cuda::funcMaxThreadsPerBlock, fn);
    made->maxThreads = most;
    made->width = d->waveSize;
    made->next = d->pipelines;
    d->pipelines = made;
  }
  d->lock.unlock();
  return made;
}

// runCUDA launches l on its CUDA device's null stream, without a wait:
// launches there run in order, and the host waits when it next touches
// a buffer's memory (syncCUDA). The kernel's entry takes the grid's size
// after its parameters, and a thread outside the grid -- in the last
// block on an axis it does not fill -- returns at once.
i32 runCUDA(Launch* l) {
  Device* d = l->device;
  Pipeline* p = cudaKernelFor(d, l->kernel);
  if (!p) return 5;
  i64 per = l->per[0] * l->per[1] * l->per[2];
  if (per > p->maxThreads) {
    say("gpu: a workgroup larger than this kernel can have on this device\n");
    return 6;
  }
  if (l->groups[0] > 0x7fffffff || l->groups[1] > 65535 || l->groups[2] > 65535) {
    say("gpu: a grid larger than CUDA launches\n");
    return 6;
  }
  i32 grid[3] = {static_cast<i32>(l->grid[0]), static_cast<i32>(l->grid[1]), static_cast<i32>(l->grid[2])};
  void** params = allocate<void*>(static_cast<usize>(l->count + 3));
  for (i64 i = 0; i < l->count; i++) params[i] = &l->values[i];
  for (int a = 0; a < 3; a++) params[l->count + a] = &grid[a];
  d->pendingLock.lock();
  bind(d);
  cuda::Result r = cuda::launchKernel(p->state, static_cast<u32>(l->groups[0]), static_cast<u32>(l->groups[1]),
                                      static_cast<u32>(l->groups[2]), static_cast<u32>(l->per[0]),
                                      static_cast<u32>(l->per[1]), static_cast<u32>(l->per[2]), 0, nullptr,
                                      params, nullptr);
  if (r == 0) d->pendingLaunches++;
  d->pendingLock.unlock();
  release(params, sizeof(void*) * static_cast<usize>(l->count + 3), 16);
  if (r != 0) {
    cuda::report("cuLaunchKernel", r);
    say(l->kernel->label ? l->kernel->label : l->kernel->name);
    say("\n");
    return 5;
  }
  if (env().sync) {
    i32 result = syncCUDA(d);
    if (env().debug) say(result == 0 ? "gpu: cuda launch ran\n" : "gpu: cuda launch failed\n");
    return result;
  }
  return 0;
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
  Device* g = gpuDevice();
  if (g) return g;
  if (want && (same(want, "metal") || same(want, "cuda"))) {
    say("gpu: VERTEX_GPU asks for a GPU, and this machine has none; using the CPU\n");
  }
  return &cpuDevice;
}

i64 vertex_gpu_device_count() { return gpuDevice() ? 2 : 1; }

void* vertex_gpu_device_at(i64 i) {
  Device* g = gpuDevice();
  if (g && i == 0) return g;
  return &cpuDevice;
}

i64 vertex_gpu_device_kind(void* d) { return static_cast<Device*>(d)->kind; }
i64 vertex_gpu_device_memory(void* d) { return static_cast<Device*>(d)->memory; }
i64 vertex_gpu_device_wave_size(void* d) { return static_cast<Device*>(d)->waveSize; }

// ---- buffers ----

void vertex_gpu_buffer_release(void* buf);

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
  } else if (d->kind == KindCUDA) {
    u64 p = 0;
    bind(d);
    cuda::Result r = cuda::memAllocManaged(&p, static_cast<usize>(n), cuda::memAttachGlobal);
    if (r != 0) {
      if (env().debug) cuda::report("cuMemAllocManaged", r);
      release(b, sizeof(Buffer), alignof(Buffer));
      return nullptr;
    }
    b->contents = reinterpret_cast<u8*>(p);
    b->cudaOwned = true;
    // Managed memory is not zeroed; the other kinds' is. Zeroed on the
    // device, in launch order: zeroed by the host, every page would be
    // faulted in on the host and then migrated to the device at the first
    // launch that touches it -- for every buffer, every time one is made.
    if (!cuda::memsetD8 || cuda::memsetD8(p, 0, static_cast<usize>(n)) != 0) {
      vertex_pal_fill(b->contents, 0, static_cast<usize>(n));
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

// A buffer over memory the caller has -- a mapped weight file -- with no
// copy: Apple's GPUs read host memory, so Metal takes the pages as they are
// (newBufferWithBytesNoCopy), and the CPU device reads them directly. ptr
// is page-aligned, as a mapping is; the length is rounded up to whole
// 16 KB pages, which the mapping's last page covers. The memory must
// outlive the buffer. Null when the device will not take it.
void* vertex_gpu_buffer_wrap(void* dev, void* ptr, i64 bytes) {
  Device* d = static_cast<Device*>(dev);
  const u64 page = 16384;
  if (!ptr || bytes <= 0) return nullptr;
  if (d->kind == KindCUDA) {
    // Memory the driver already manages -- another buffer's -- is the
    // device's as it is, and is taken in place.
    u32 managed = 0;
    bind(d);
    if (cuda::pointerGetAttribute(&managed, cuda::pointerIsManaged, reinterpret_cast<u64>(ptr)) == 0 && managed) {
      Buffer* b = allocate<Buffer>(1);
      b->device = d;
      b->bytes = bytes;
      b->contents = static_cast<u8*>(ptr);
      b->wrapped = true;
      return b;
    }
    // Host memory a discrete GPU would read across the bus, every time:
    // a mapped weight file, which is what this is for. Its bytes are
    // copied to the device once instead, and the caller's memory -- which
    // is the caller's to leave unchanged -- is not kept.
    Buffer* b = static_cast<Buffer*>(vertex_gpu_buffer_create(dev, bytes));
    if (!b) return nullptr;
    bind(d);
    cuda::Result r = cuda::memcpyHtoD(reinterpret_cast<u64>(b->contents), ptr, static_cast<usize>(bytes));
    if (r != 0) {
      cuda::report("cuMemcpyHtoD", r);
      vertex_gpu_buffer_release(b);
      return nullptr;
    }
    return b;
  }
  // Metal takes whole pages; the CPU device reads any memory.
  if (d->kind == KindMetal && (reinterpret_cast<u64>(ptr) & (4096 - 1)) != 0) return nullptr;
  Buffer* b = allocate<Buffer>(1);
  b->device = d;
  b->bytes = bytes;
  b->contents = static_cast<u8*>(ptr);
  b->wrapped = true;
  if (d->kind == KindMetal) {
    u64 length = (static_cast<u64>(bytes) + page - 1) / page * page;
    void* pool = objc::poolPush();
    b->metal = objc::send<objc::id>(d->metal, "newBufferWithBytesNoCopy:length:options:deallocator:", ptr, length,
                                    static_cast<u64>(0), static_cast<objc::id>(nullptr));
    objc::poolPop(pool);
    if (!b->metal) {
      release(b, sizeof(Buffer), alignof(Buffer));
      return nullptr;
    }
  }
  return b;
}

void vertex_gpu_buffer_release(void* buf) {
  Buffer* b = static_cast<Buffer*>(buf);
  if (!b) return;
  // A launch still pending may read or write this buffer: the one a
  // temporary made for it -- `k.Launch(try await d.Upload(xs), ...)`,
  // `k.Map(try await k.Map(xs))` -- ends as soon as the call returns,
  // before anything has run it. Letting the memory go is touching it, so
  // what was launched runs first, as for the host's view of it.
  if (b->device != nullptr && syncDevice(b->device) != 0) vertex_pal_abort();
  if (b->metal) {
    objc::send<void>(b->metal, "release");
  } else if (b->wrapped) {
    // The caller's memory.
  } else if (b->cudaOwned) {
    bind(b->device);
    cuda::memFree(reinterpret_cast<u64>(b->contents));
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
    // A kernel with no image for its device -- one AIR cannot express,
    // as a float64 on Apple's GPUs -- runs on the CPU: Metal's buffers
    // are host memory on Apple's machines, and CUDA's managed memory is
    // the host's too, so its arguments already are.
    if (l->device->kind == KindMetal && l->kernel->metallib) {
      result = runMetal(l);
    } else if (l->device->kind == KindCUDA && l->kernel->ptx) {
      result = runCUDA(l);
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

// ---- rendering (Metal only) ----
//
// Textures, libraries compiled from Metal source while the program runs,
// render pipelines and render passes: what gpu/raster draws with. Each
// export is a few Metal calls; what to draw is decided in Vertex
// (stdlib/gpu/render.vs). Passes encode into the device's open render
// command buffer; reading or writing a texture from the host commits it
// and waits first, so the host sees what was drawn.

namespace {

struct Texture {
  Device* device;
  objc::id metal;  // id<MTLTexture>
  i64 format;
};

struct Encoder {
  Device* device;
  objc::id metal;  // id<MTLRenderCommandEncoder>, retained
};

struct Viewport {
  double x, y, w, h, znear, zfar;
};
struct Scissor {
  u64 x, y, w, h;
};
struct ClearColor {
  double r, g, b, a;
};
struct Region {
  u64 x, y, z, w, h, d;
};

objc::id nsString(const u8* p, i64 n) {
  char* c = allocate<char>(static_cast<usize>(n + 1));
  if (n > 0) vertex_pal_copy(c, p, static_cast<usize>(n));
  c[n] = 0;
  objc::id s = objc::send<objc::id>(objc::getClass("NSString"), "stringWithUTF8String:", c);
  release(c, static_cast<usize>(n + 1), 1);
  return s;
}

// The error's text into out (NUL-terminated, cut to cap).
void errorText(objc::id error, char* out, i64 cap) {
  if (!out || cap <= 0) return;
  out[0] = 0;
  if (!error) return;
  objc::id text = objc::send<objc::id>(error, "localizedDescription");
  const char* s = text ? objc::send<const char*>(text, "UTF8String") : nullptr;
  if (!s) return;
  i64 n = 0;
  while (s[n] && n < cap - 1) {
    out[n] = s[n];
    n++;
  }
  out[n] = 0;
}

// The open render command buffer, made if there is none. Caller holds d->lock.
objc::id renderBufferOf(Device* d) {
  if (!d->renderBuffer) {
    objc::id cb = objc::send<objc::id>(d->queue, "commandBuffer");
    objc::send<objc::id>(cb, "retain");
    d->renderBuffer = cb;
  }
  return d->renderBuffer;
}

// Commits what passes encoded and, when wait, waits for everything
// committed so far (buffers run in order: waiting for the last is enough).
// 0, or 7 when the GPU failed.
i32 renderFlush(Device* d, bool wait) {
  d->lock.lock();
  objc::id cb = d->renderBuffer;
  d->renderBuffer = nullptr;
  objc::id last = d->renderLast;
  d->renderLast = nullptr;
  d->lock.unlock();
  if (!cb && !last) return 0;
  void* pool = objc::poolPush();
  if (cb) {
    objc::send<void>(cb, "commit");
    if (last) objc::send<void>(last, "release");
    last = cb;
  }
  i32 result = 0;
  if (wait) {
    objc::send<void>(last, "waitUntilCompleted");
    if (objc::send<u64>(last, "status") == 5) {
      objc::report("a render pass failed", objc::send<objc::id>(last, "error"));
      result = 7;
    }
    objc::send<void>(last, "release");
  } else {
    d->lock.lock();
    if (d->renderLast) {
      objc::send<void>(d->renderLast, "release");
    }
    d->renderLast = last;
    d->lock.unlock();
  }
  objc::poolPop(pool);
  return result;
}

// A buffer holding a copy of bytes, for data too big for set*Bytes (4 KB).
objc::id tempBuffer(Device* d, const void* p, i64 n) {
  return objc::send<objc::id>(d->metal, "newBufferWithBytes:length:options:", p, static_cast<u64>(n > 0 ? n : 4),
                              static_cast<u64>(0));
}

}  // namespace

extern "C" {

// Textures. kind: 0 2D, 1 cube. format: 0 RGBA8, 1 RGBA32F, 2 depth32F+stencil8.
void* vertex_gpu_texture_create(void* dev, i64 kind, i64 format, i64 width, i64 height, i64 levels) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal || width <= 0 || height <= 0) return nullptr;
  void* pool = objc::poolPush();
  objc::id desc = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLTextureDescriptor"), "alloc"), "init");
  u64 pf = format == 1 ? 125 : (format == 2 ? 260 : 70);
  objc::send<void>(desc, "setTextureType:", static_cast<u64>(kind == 1 ? 5 : 2));
  objc::send<void>(desc, "setPixelFormat:", pf);
  objc::send<void>(desc, "setWidth:", static_cast<u64>(width));
  objc::send<void>(desc, "setHeight:", static_cast<u64>(height));
  objc::send<void>(desc, "setMipmapLevelCount:", static_cast<u64>(levels > 0 ? levels : 1));
  objc::send<void>(desc, "setUsage:", static_cast<u64>(1 | 4));  // shader read, render target
  // Depth and stencil live in GPU memory; color is shared, so the host reads and writes it.
  objc::send<void>(desc, "setStorageMode:", static_cast<u64>(format == 2 ? 2 : 0));
  objc::id t = objc::send<objc::id>(d->metal, "newTextureWithDescriptor:", desc);
  objc::send<void>(desc, "release");
  objc::poolPop(pool);
  if (!t) return nullptr;
  Texture* tex = allocate<Texture>(1);
  tex->device = d;
  tex->metal = t;
  tex->format = format;
  return tex;
}

void vertex_gpu_texture_release(void* t) {
  Texture* tex = static_cast<Texture*>(t);
  if (!tex) return;
  // A pass may still read or write it: finish them first.
  renderFlush(tex->device, true);
  objc::send<void>(tex->metal, "release");
  release(tex, sizeof(Texture), alignof(Texture));
}

// Host bytes into a region of one level and slice (cube face).
void vertex_gpu_texture_write(void* t, i64 level, i64 slice, i64 x, i64 y, i64 w, i64 h, const void* bytes, i64 bytesPerRow) {
  Texture* tex = static_cast<Texture*>(t);
  if (!tex || tex->format == 2 || w <= 0 || h <= 0) return;
  renderFlush(tex->device, true);
  void* pool = objc::poolPush();
  Region r = {static_cast<u64>(x), static_cast<u64>(y), 0, static_cast<u64>(w), static_cast<u64>(h), 1};
  objc::send<void>(tex->metal, "replaceRegion:mipmapLevel:slice:withBytes:bytesPerRow:bytesPerImage:", r,
                   static_cast<u64>(level), static_cast<u64>(slice), bytes, static_cast<u64>(bytesPerRow), static_cast<u64>(0));
  objc::poolPop(pool);
}

// A region of one level and slice into host bytes, after what was drawn.
void vertex_gpu_texture_read(void* t, i64 level, i64 slice, i64 x, i64 y, i64 w, i64 h, void* bytes, i64 bytesPerRow) {
  Texture* tex = static_cast<Texture*>(t);
  if (!tex || tex->format == 2 || w <= 0 || h <= 0) return;
  renderFlush(tex->device, true);
  void* pool = objc::poolPush();
  Region r = {static_cast<u64>(x), static_cast<u64>(y), 0, static_cast<u64>(w), static_cast<u64>(h), 1};
  objc::send<void>(tex->metal, "getBytes:bytesPerRow:bytesPerImage:fromRegion:mipmapLevel:slice:", bytes,
                   static_cast<u64>(bytesPerRow), static_cast<u64>(0), r, static_cast<u64>(level), static_cast<u64>(slice));
  objc::poolPop(pool);
}

// A library compiled from Metal source; null, with the compiler's log in err, when it doesn't compile.
void* vertex_gpu_library_source(void* dev, const u8* source, i64 length, char* err, i64 cap) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return nullptr;
  void* pool = objc::poolPush();
  objc::id src = nsString(source, length);
  objc::id error = nullptr;
  objc::id lib = objc::send<objc::id>(d->metal, "newLibraryWithSource:options:error:", src, static_cast<objc::id>(nullptr), &error);
  if (!lib) errorText(error, err, cap);
  objc::poolPop(pool);
  return lib;
}

void* vertex_gpu_library_function(void* lib, const u8* name, i64 length) {
  if (!lib) return nullptr;
  void* pool = objc::poolPush();
  objc::id fn = objc::send<objc::id>(lib, "newFunctionWithName:", nsString(name, length));
  objc::poolPop(pool);
  return fn;
}

// Releases a library, function, pipeline, depth-stencil or sampler state.
void vertex_gpu_object_release(void* o) {
  if (o) objc::send<void>(o, "release");
}

// A render pipeline. desc, in order: color format (-1 none, else as
// textures), depth-stencil format (-1 none), blending (0/1), RGB op,
// alpha op, source RGB, destination RGB, source alpha, destination alpha
// (Metal's enums), write mask; then the attribute count and per
// attribute: index, MTLVertexFormat, offset, buffer index; then the
// buffer-layout count and per layout: buffer index, stride, step
// function, step rate.
void* vertex_gpu_render_pipeline(void* dev, void* vfn, void* ffn, const i64* desc, char* err, i64 cap) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return nullptr;
  void* pool = objc::poolPush();
  objc::id pd = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLRenderPipelineDescriptor"), "alloc"), "init");
  objc::send<void>(pd, "setVertexFunction:", vfn);
  objc::send<void>(pd, "setFragmentFunction:", ffn);
  i64 at = 0;
  i64 color = desc[at++];
  i64 depth = desc[at++];
  objc::id ca = objc::send<objc::id>(objc::send<objc::id>(pd, "colorAttachments"), "objectAtIndexedSubscript:", static_cast<u64>(0));
  if (color >= 0) {
    objc::send<void>(ca, "setPixelFormat:", static_cast<u64>(color == 1 ? 125 : 70));
  }
  i64 blending = desc[at++];
  objc::send<void>(ca, "setBlendingEnabled:", static_cast<signed char>(blending != 0 ? 1 : 0));
  objc::send<void>(ca, "setRgbBlendOperation:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setAlphaBlendOperation:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setSourceRGBBlendFactor:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setDestinationRGBBlendFactor:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setSourceAlphaBlendFactor:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setDestinationAlphaBlendFactor:", static_cast<u64>(desc[at++]));
  objc::send<void>(ca, "setWriteMask:", static_cast<u64>(desc[at++]));
  if (depth >= 0) {
    objc::send<void>(pd, "setDepthAttachmentPixelFormat:", static_cast<u64>(260));
    objc::send<void>(pd, "setStencilAttachmentPixelFormat:", static_cast<u64>(260));
  }
  objc::id vd = objc::send<objc::id>(objc::getClass("MTLVertexDescriptor"), "vertexDescriptor");
  i64 attributes = desc[at++];
  for (i64 k = 0; k < attributes; k++) {
    objc::id a = objc::send<objc::id>(objc::send<objc::id>(vd, "attributes"), "objectAtIndexedSubscript:", static_cast<u64>(desc[at]));
    objc::send<void>(a, "setFormat:", static_cast<u64>(desc[at + 1]));
    objc::send<void>(a, "setOffset:", static_cast<u64>(desc[at + 2]));
    objc::send<void>(a, "setBufferIndex:", static_cast<u64>(desc[at + 3]));
    at += 4;
  }
  i64 layouts = desc[at++];
  for (i64 k = 0; k < layouts; k++) {
    objc::id l = objc::send<objc::id>(objc::send<objc::id>(vd, "layouts"), "objectAtIndexedSubscript:", static_cast<u64>(desc[at]));
    objc::send<void>(l, "setStride:", static_cast<u64>(desc[at + 1]));
    objc::send<void>(l, "setStepFunction:", static_cast<u64>(desc[at + 2]));
    objc::send<void>(l, "setStepRate:", static_cast<u64>(desc[at + 3]));
    at += 4;
  }
  objc::send<void>(pd, "setVertexDescriptor:", vd);
  objc::id error = nullptr;
  objc::id state = objc::send<objc::id>(d->metal, "newRenderPipelineStateWithDescriptor:error:", pd, &error);
  if (!state) errorText(error, err, cap);
  objc::send<void>(pd, "release");
  objc::poolPop(pool);
  return state;
}

// Depth-stencil state. desc: depth compare, depth write (0/1), stencil
// (0/1), then front and back: compare, fail, depth fail, pass, read mask,
// write mask (Metal's enums).
void* vertex_gpu_depth_stencil_state(void* dev, const i64* desc) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return nullptr;
  void* pool = objc::poolPush();
  objc::id dd = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLDepthStencilDescriptor"), "alloc"), "init");
  objc::send<void>(dd, "setDepthCompareFunction:", static_cast<u64>(desc[0]));
  objc::send<void>(dd, "setDepthWriteEnabled:", static_cast<signed char>(desc[1] != 0 ? 1 : 0));
  if (desc[2] != 0) {
    const char* faces[2] = {"setFrontFaceStencil:", "setBackFaceStencil:"};
    for (int f = 0; f < 2; f++) {
      const i64* s = desc + 3 + f * 6;
      objc::id sd = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLStencilDescriptor"), "alloc"), "init");
      objc::send<void>(sd, "setStencilCompareFunction:", static_cast<u64>(s[0]));
      objc::send<void>(sd, "setStencilFailureOperation:", static_cast<u64>(s[1]));
      objc::send<void>(sd, "setDepthFailureOperation:", static_cast<u64>(s[2]));
      objc::send<void>(sd, "setDepthStencilPassOperation:", static_cast<u64>(s[3]));
      objc::send<void>(sd, "setReadMask:", static_cast<u32>(s[4]));
      objc::send<void>(sd, "setWriteMask:", static_cast<u32>(s[5]));
      objc::send<void>(dd, faces[f], sd);
      objc::send<void>(sd, "release");
    }
  }
  objc::id state = objc::send<objc::id>(d->metal, "newDepthStencilStateWithDescriptor:", dd);
  objc::send<void>(dd, "release");
  objc::poolPop(pool);
  return state;
}

// A sampler. desc: min, mag, mip filter, address S, T, R, compare (-1 none); lods: min, max.
void* vertex_gpu_sampler_state(void* dev, const i64* desc, const float* lods) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return nullptr;
  void* pool = objc::poolPush();
  objc::id sd = objc::send<objc::id>(objc::send<objc::id>(objc::getClass("MTLSamplerDescriptor"), "alloc"), "init");
  objc::send<void>(sd, "setMinFilter:", static_cast<u64>(desc[0]));
  objc::send<void>(sd, "setMagFilter:", static_cast<u64>(desc[1]));
  objc::send<void>(sd, "setMipFilter:", static_cast<u64>(desc[2]));
  objc::send<void>(sd, "setSAddressMode:", static_cast<u64>(desc[3]));
  objc::send<void>(sd, "setTAddressMode:", static_cast<u64>(desc[4]));
  objc::send<void>(sd, "setRAddressMode:", static_cast<u64>(desc[5]));
  if (desc[6] >= 0) objc::send<void>(sd, "setCompareFunction:", static_cast<u64>(desc[6]));
  objc::send<void>(sd, "setLodMinClamp:", lods[0]);
  objc::send<void>(sd, "setLodMaxClamp:", lods[1]);
  objc::id state = objc::send<objc::id>(d->metal, "newSamplerStateWithDescriptor:", sd);
  objc::send<void>(sd, "release");
  objc::poolPop(pool);
  return state;
}

// Begins a render pass into the device's open command buffer. color and
// depth may be null; load: 0 keep, 1 clear (to clear, clearDepth, clearStencil).
void* vertex_gpu_render_begin(void* dev, void* color, i64 level, i64 slice, i64 colorLoad, const float* clear,
                              void* depth, i64 depthLoad, float clearDepth, i64 stencilLoad, i64 clearStencil) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return nullptr;
  void* pool = objc::poolPush();
  objc::id rp = objc::send<objc::id>(objc::getClass("MTLRenderPassDescriptor"), "renderPassDescriptor");
  if (color) {
    objc::id ca = objc::send<objc::id>(objc::send<objc::id>(rp, "colorAttachments"), "objectAtIndexedSubscript:", static_cast<u64>(0));
    objc::send<void>(ca, "setTexture:", static_cast<Texture*>(color)->metal);
    objc::send<void>(ca, "setLevel:", static_cast<u64>(level));
    objc::send<void>(ca, "setSlice:", static_cast<u64>(slice));
    objc::send<void>(ca, "setLoadAction:", static_cast<u64>(colorLoad != 0 ? 2 : 1));
    objc::send<void>(ca, "setStoreAction:", static_cast<u64>(1));
    ClearColor cc = {clear[0], clear[1], clear[2], clear[3]};
    objc::send<void>(ca, "setClearColor:", cc);
  }
  if (depth) {
    objc::id t = static_cast<Texture*>(depth)->metal;
    objc::id da = objc::send<objc::id>(rp, "depthAttachment");
    objc::send<void>(da, "setTexture:", t);
    objc::send<void>(da, "setLoadAction:", static_cast<u64>(depthLoad != 0 ? 2 : 1));
    objc::send<void>(da, "setStoreAction:", static_cast<u64>(1));
    objc::send<void>(da, "setClearDepth:", static_cast<double>(clearDepth));
    objc::id sa = objc::send<objc::id>(rp, "stencilAttachment");
    objc::send<void>(sa, "setTexture:", t);
    objc::send<void>(sa, "setLoadAction:", static_cast<u64>(stencilLoad != 0 ? 2 : 1));
    objc::send<void>(sa, "setStoreAction:", static_cast<u64>(1));
    objc::send<void>(sa, "setClearStencil:", static_cast<u32>(clearStencil));
  }
  d->lock.lock();
  objc::id cb = renderBufferOf(d);
  d->lock.unlock();
  objc::id enc = objc::send<objc::id>(cb, "renderCommandEncoderWithDescriptor:", rp);
  if (enc) objc::send<objc::id>(enc, "retain");
  objc::poolPop(pool);
  if (!enc) return nullptr;
  Encoder* e = allocate<Encoder>(1);
  e->device = d;
  e->metal = enc;
  return e;
}

void vertex_gpu_render_end(void* enc) {
  Encoder* e = static_cast<Encoder*>(enc);
  if (!e) return;
  objc::send<void>(e->metal, "endEncoding");
  objc::send<void>(e->metal, "release");
  release(e, sizeof(Encoder), alignof(Encoder));
}

void vertex_gpu_render_set_pipeline(void* enc, void* p) {
  objc::send<void>(static_cast<Encoder*>(enc)->metal, "setRenderPipelineState:", p);
}

void vertex_gpu_render_set_depth_stencil(void* enc, void* s, i64 frontRef, i64 backRef) {
  objc::id e = static_cast<Encoder*>(enc)->metal;
  objc::send<void>(e, "setDepthStencilState:", s);
  objc::send<void>(e, "setStencilFrontReferenceValue:backReferenceValue:", static_cast<u32>(frontRef), static_cast<u32>(backRef));
}

void vertex_gpu_render_set_viewport(void* enc, float x, float y, float w, float h, float znear, float zfar) {
  Viewport v = {x, y, w, h, znear, zfar};
  objc::send<void>(static_cast<Encoder*>(enc)->metal, "setViewport:", v);
}

void vertex_gpu_render_set_scissor(void* enc, i64 x, i64 y, i64 w, i64 h) {
  Scissor s = {static_cast<u64>(x), static_cast<u64>(y), static_cast<u64>(w), static_cast<u64>(h)};
  objc::send<void>(static_cast<Encoder*>(enc)->metal, "setScissorRect:", s);
}

// cull: 0 none, 1 front, 2 back; winding: 0 clockwise, 1 counter-clockwise front faces.
void vertex_gpu_render_set_raster(void* enc, i64 cull, i64 winding, float bias, float slope) {
  objc::id e = static_cast<Encoder*>(enc)->metal;
  objc::send<void>(e, "setCullMode:", static_cast<u64>(cull));
  objc::send<void>(e, "setFrontFacingWinding:", static_cast<u64>(winding));
  objc::send<void>(e, "setDepthBias:slopeScale:clamp:", bias, slope, 0.0f);
}

void vertex_gpu_render_set_blend_color(void* enc, float r, float g, float b, float a) {
  objc::send<void>(static_cast<Encoder*>(enc)->metal, "setBlendColorRed:green:blue:alpha:", r, g, b, a);
}

// Bytes for a vertex (stage 0) or fragment (stage 1) buffer slot: inline when small, else a copy.
void vertex_gpu_render_set_bytes(void* enc, i64 stage, const void* p, i64 n, i64 index) {
  Encoder* e = static_cast<Encoder*>(enc);
  if (n <= 4096) {
    objc::send<void>(e->metal, stage == 0 ? "setVertexBytes:length:atIndex:" : "setFragmentBytes:length:atIndex:", p,
                     static_cast<u64>(n > 0 ? n : 4), static_cast<u64>(index));
    return;
  }
  objc::id b = tempBuffer(e->device, p, n);
  objc::send<void>(e->metal, stage == 0 ? "setVertexBuffer:offset:atIndex:" : "setFragmentBuffer:offset:atIndex:", b,
                   static_cast<u64>(0), static_cast<u64>(index));
  objc::send<void>(b, "release");  // the command buffer holds it
}

void vertex_gpu_render_set_texture(void* enc, i64 stage, void* t, i64 index) {
  objc::id tex = t ? static_cast<Texture*>(t)->metal : nullptr;
  objc::send<void>(static_cast<Encoder*>(enc)->metal, stage == 0 ? "setVertexTexture:atIndex:" : "setFragmentTexture:atIndex:", tex,
                   static_cast<u64>(index));
}

void vertex_gpu_render_set_sampler(void* enc, i64 stage, void* s, i64 index) {
  objc::send<void>(static_cast<Encoder*>(enc)->metal,
                   stage == 0 ? "setVertexSamplerState:atIndex:" : "setFragmentSamplerState:atIndex:", s, static_cast<u64>(index));
}

// primitive: 0 point, 1 line, 2 line strip, 3 triangle, 4 triangle strip.
void vertex_gpu_render_draw(void* enc, i64 primitive, i64 start, i64 count, i64 instances) {
  objc::send<void>(static_cast<Encoder*>(enc)->metal, "drawPrimitives:vertexStart:vertexCount:instanceCount:",
                   static_cast<u64>(primitive), static_cast<u64>(start), static_cast<u64>(count), static_cast<u64>(instances));
}

// Indexed: the indices are host bytes, 2 or 4 each.
void vertex_gpu_render_draw_indexed(void* enc, i64 primitive, i64 count, i64 indexSize, const void* indices, i64 n, i64 instances) {
  Encoder* e = static_cast<Encoder*>(enc);
  objc::id b = tempBuffer(e->device, indices, n);
  objc::send<void>(e->metal, "drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:",
                   static_cast<u64>(primitive), static_cast<u64>(count), static_cast<u64>(indexSize == 4 ? 1 : 0), b,
                   static_cast<u64>(0), static_cast<u64>(instances));
  objc::send<void>(b, "release");
}

// Commits what was drawn; when wait, until the GPU is done. 0, or 7 when it failed.
i32 vertex_gpu_render_commit(void* dev, i64 wait) {
  Device* d = static_cast<Device*>(dev);
  if (!d || d->kind != KindMetal) return 0;
  return renderFlush(d, wait != 0);
}

}  // extern "C"
