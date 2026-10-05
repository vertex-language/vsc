// The platform layer for macOS: libSystem.
#pragma once

#include "vertex/platform.h"

struct __sFILE;

extern "C" {
struct mach_header_64;
vertex::u32 _dyld_image_count(void);
const mach_header_64* _dyld_get_image_header(vertex::u32 image);
vertex::u8* getsectiondata(const mach_header_64* header, const char* segment, const char* section,
                           unsigned long* size);
struct _xlocale;
double strtod_l(const char* text, char** end, _xlocale* locale);
void* malloc(vertex::usize size);
void  free(void* p);
void* memcpy(void* dest, const void* src, vertex::usize count);
void* memset(void* dest, int byte, vertex::usize count);
[[noreturn]] void abort(void);
vertex::usize fwrite(const void* bytes, vertex::usize size, vertex::usize count, __sFILE* stream);
extern __sFILE* __stdoutp;
extern __sFILE* __stderrp;
extern __sFILE* __stdinp;
int getc(__sFILE* stream);
int fflush(__sFILE* stream);

void* vertex_pal_alloc(vertex::usize size, vertex::usize) { return malloc(size); }
void  vertex_pal_free(void* p, vertex::usize, vertex::usize) { free(p); }
void  vertex_pal_copy(void* dest, const void* src, vertex::usize count) { memcpy(dest, src, count); }
void  vertex_pal_fill(void* dest, vertex::u8 byte, vertex::usize count) { memset(dest, byte, count); }
void  vertex_pal_abort(void) { abort(); }

// Conformance records are in each image's __TEXT,__vertex_proto, which
// vsc's lower writes; dyld says where each image is.
vertex::u32 vertex_pal_image_count(void) { return _dyld_image_count(); }

const vertex::i32* vertex_pal_conformance_records(vertex::u32 image, vertex::usize* bytes) {
  *bytes = 0;
  const mach_header_64* header = _dyld_get_image_header(image);
  if (header == nullptr)
    return nullptr;
  unsigned long size = 0;
  vertex::u8* at = getsectiondata(header, "__TEXT", "__vertex_proto", &size);
  if (at == nullptr)
    return nullptr;
  *bytes = size;
  return reinterpret_cast<const vertex::i32*>(at);
}

// Through stdio rather than write(2), so that what a Vertex program
// prints and what C code in the same process prints share one buffer
// and come out in the order they were written.
void vertex_pal_write(int stream, const vertex::u8* bytes, vertex::usize count) {
  fwrite(bytes, 1, count, stream == 2 ? __stderrp : __stdoutp);
}

void vertex_pal_flush(void) { fflush(__stdoutp); }

// strtod_l with the C locale (a null locale is it), over a terminated copy.
bool vertex_pal_parse_double(const vertex::u8* bytes, vertex::usize count, double* out) {
  char scratch[128];
  char* text = scratch;
  if (count + 1 > sizeof(scratch)) {
    text = static_cast<char*>(malloc(count + 1));
    if (text == nullptr)
      abort();
  }
  for (vertex::usize i = 0; i < count; i++)
    text[i] = static_cast<char>(bytes[i]);
  text[count] = 0;
  char* end = nullptr;
  *out = strtod_l(text, &end, nullptr);
  bool whole = count > 0 && end == text + count;
  if (text != scratch)
    free(text);
  return whole;
}

void arc4random_buf(void* bytes, vertex::usize count);

void vertex_pal_entropy(vertex::u8* bytes, vertex::usize count) {
  arc4random_buf(bytes, count);
}

int* _NSGetArgc(void);
char*** _NSGetArgv(void);

// What dyld handed main, which libSystem keeps: UTF-8 already.
const char* const* vertex_pal_arguments(int* count) {
  *count = *_NSGetArgc();
  return const_cast<const char* const*>(*_NSGetArgv());
}

// Through stdio as well, flushing what was printed first so that a
// prompt appears before the program waits on its answer.
int vertex_pal_read_byte(void) {
  fflush(__stdoutp);
  return getc(__stdinp);
}

struct timespec {
  long tv_sec;
  long tv_nsec;
};
vertex::u64 clock_gettime_nsec_np(int clock);
int nanosleep(const timespec* request, timespec* remaining);

// CLOCK_UPTIME_RAW: monotonic, and stopped while the machine sleeps,
// which is what a deadline measured in the program's own time wants.
vertex::u64 vertex_pal_now(void) {
  return clock_gettime_nsec_np(8);
}

void vertex_pal_sleep(vertex::u64 nanoseconds) {
  timespec request{static_cast<long>(nanoseconds / 1000000000u),
                   static_cast<long>(nanoseconds % 1000000000u)};
  nanosleep(&request, nullptr);
}

// struct kevent, under a name of its own: the function shares its name.
struct KEvent {
  vertex::usize ident;
  vertex::i16   filter;
  vertex::u16   flags;
  vertex::u32   fflags;
  vertex::i64   data;
  void*         udata;
};
int kqueue(void);
int kevent(int kq, const KEvent* changes, int nchanges, KEvent* events, int nevents, const timespec* timeout);

// A kqueue per executor. EVFILT_USER (-10) with NOTE_TRIGGER is how
// another thread ends a wait on it.
//
// A registration is not a syscall of its own. It waits in `pending` and
// goes to the kernel as the changelist of the next kevent that waits for
// events -- the one call that could report it firing anyway. A server
// parks a task on its socket once per request, so this is one kevent per
// request saved. The registrations stay one-shot: a descriptor closed and
// its number reused needs nothing from here, which a registration kept
// for the descriptor's life (EV_CLEAR) would.
//
// pending is the owning executor's alone: only that thread registers,
// unregisters and waits. vertex_pal_io_wake, from other threads, uses kq
// directly and never touches it.
inline constexpr int ioPendingMax = 64;

struct IoQueue {
  int kq;
  int npending;
  KEvent pending[ioPendingMax];
};

inline constexpr vertex::i16 evfiltUser = -10;
inline constexpr vertex::usize wakeIdent = 1;

void* vertex_pal_io_open(void) {
  int kq = kqueue();
  if (kq < 0)
    return nullptr;
  auto* q = static_cast<IoQueue*>(malloc(sizeof(IoQueue)));
  if (q == nullptr)
    return nullptr;
  q->kq = kq;
  q->npending = 0;
  // The wake event, added once: EV_ADD | EV_CLEAR, so that a trigger is
  // delivered once and the event stays.
  KEvent change{};
  change.ident = wakeIdent;
  change.filter = evfiltUser;
  change.flags = 0x1 | 0x20;
  kevent(kq, &change, 1, nullptr, 0, nullptr);
  return q;
}

// A kqueue is itself a descriptor, readable while it holds an event.
vertex::i32 vertex_pal_io_descriptor(void* queue) {
  auto* q = static_cast<IoQueue*>(queue);
  return q == nullptr ? -1 : q->kq;
}

// EVFILT_READ or EVFILT_WRITE, once: EV_ADD | EV_ONESHOT, queued for the
// next wait (see IoQueue). Only when the queue is full is it a kevent of
// its own. A registration the kernel refuses comes back from that wait as
// an EV_ERROR event for the token, which wakes the task to find the error
// itself.
int vertex_pal_io_register(void* queue, vertex::i32 fd, vertex::i32 events, void* token) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return -1;
  KEvent change{};
  change.ident = static_cast<vertex::usize>(fd);
  change.filter = events == 2 ? -2 : -1;
  change.flags = 0x1 | 0x10;
  change.udata = token;
  if (q->npending < ioPendingMax) {
    q->pending[q->npending++] = change;
    return 0;
  }
  return kevent(q->kq, &change, 1, nullptr, 0, nullptr) < 0 ? -1 : 0;
}

// EV_DELETE for whichever filter was registered. A registration that has
// already fired is gone, and kevent says so; there is nothing to do about
// it either way.
void vertex_pal_io_unregister(void* queue, vertex::i32 fd, vertex::i32 events) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return;
  vertex::i16 filter = events == 2 ? -2 : -1;
  // Still queued: the kernel never heard of it.
  for (int i = 0; i < q->npending; i++) {
    if (q->pending[i].ident == static_cast<vertex::usize>(fd) && q->pending[i].filter == filter) {
      q->pending[i] = q->pending[--q->npending];
      return;
    }
  }
  KEvent change{};
  change.ident = static_cast<vertex::usize>(fd);
  change.filter = filter;
  change.flags = 0x2;  // EV_DELETE
  kevent(q->kq, &change, 1, nullptr, 0, nullptr);
}

// NOTE_TRIGGER (0x01000000) on the user event.
void vertex_pal_io_wake(void* queue) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return;
  KEvent change{};
  change.ident = wakeIdent;
  change.filter = evfiltUser;
  change.fflags = 0x01000000;
  kevent(q->kq, &change, 1, nullptr, 0, nullptr);
}

struct PollFd {
  int   fd;
  short events;
  short revents;
};
int poll(PollFd* fds, unsigned long nfds, int timeout);

// POLLIN is 0x1 and POLLOUT is 0x4.
int vertex_pal_io_wait_one(vertex::i32 fd, vertex::i32 events, vertex::i64 timeout) {
  PollFd p{};
  p.fd = fd;
  p.events = events == 2 ? 0x4 : 0x1;
  int ms = -1;
  if (timeout >= 0) {
    vertex::i64 rounded = (timeout + 999999) / 1000000;
    ms = rounded > 0x7fffffff ? 0x7fffffff : static_cast<int>(rounded);
  }
  int n = poll(&p, 1, ms);
  if (n < 0)
    return -1;
  return n > 0 ? 1 : 0;
}

int vertex_pal_io_wait(void* queue, vertex::i64 timeout, void** tokens, int max) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return 0;
  KEvent ready[64];
  if (max > 64)
    max = 64;
  timespec wait{};
  timespec* until = nullptr;
  if (timeout >= 0) {
    wait.tv_sec = static_cast<long>(timeout / 1000000000);
    wait.tv_nsec = static_cast<long>(timeout % 1000000000);
    until = &wait;
  }
  // The queued registrations go in with the wait. However it ends they
  // have been applied (kevent(2): an interrupted call has still applied
  // its changelist), so the queue starts again empty.
  int n = kevent(q->kq, q->pending, q->npending, ready, max, until);
  q->npending = 0;
  if (n < 0)
    return 0;
  for (int i = 0; i < n; i++)
    tokens[i] = ready[i].filter == evfiltUser ? nullptr : ready[i].udata;
  return n;
}

// Threads: pthreads, declared here as the rest of libSystem is. A
// pthread_t is a pointer and a key an unsigned long on Darwin.
typedef void* PThread;
typedef unsigned long PThreadKey;
int pthread_create(PThread* thread, const void* attr, void* (*start)(void*), void* arg);
int pthread_detach(PThread thread);
int pthread_key_create(PThreadKey* key, void (*destructor)(void*));
int pthread_setspecific(PThreadKey key, const void* value);
void* pthread_getspecific(PThreadKey key);
long sysconf(int name);
char* getenv(const char* name);

struct ThreadStart {
  void (*body)(void*);
  void* arg;
};

static void* threadMain(void* p) {
  auto* s = static_cast<ThreadStart*>(p);
  void (*body)(void*) = s->body;
  void* arg = s->arg;
  free(s);
  body(arg);
  return nullptr;
}

bool vertex_pal_thread_start(void (*body)(void*), void* arg) {
  auto* s = static_cast<ThreadStart*>(malloc(sizeof(ThreadStart)));
  if (s == nullptr)
    return false;
  s->body = body;
  s->arg = arg;
  PThread t = nullptr;
  if (pthread_create(&t, nullptr, threadMain, s) != 0) {
    free(s);
    return false;
  }
  pthread_detach(t);
  return true;
}

// _SC_NPROCESSORS_ONLN is 58 on Darwin.
int vertex_pal_cpus(void) {
  long n = sysconf(58);
  return n < 1 ? 1 : static_cast<int>(n);
}

static PThreadKey threadSlot;
static bool threadSlotMade;

void vertex_pal_thread_set(void* value) {
  if (!threadSlotMade) {
    pthread_key_create(&threadSlot, nullptr);
    threadSlotMade = true;
  }
  pthread_setspecific(threadSlot, value);
}

void* vertex_pal_thread_get(void) {
  if (!threadSlotMade)
    return nullptr;
  return pthread_getspecific(threadSlot);
}

const char* vertex_pal_getenv(const char* name) { return getenv(name); }
}
