// The platform layer for Linux, whichever libc is above it: what bionic and
// glibc share, which is the kernel's interfaces and POSIX's, declared here
// the way darwin.h declares libSystem so that no NDK or sysroot is needed to
// build the runtime.
//
// A libc's own header (android.h, linux.h) declares what differs -- where
// errno is -- as vertex_errno, and then includes this one.
#pragma once

#include "vertex/platform.h"

extern "C" {
void* malloc(vertex::usize size);
void  free(void* p);
void* memcpy(void* dest, const void* src, vertex::usize count);
void* memset(void* dest, int byte, vertex::usize count);
[[noreturn]] void abort(void);
long  write(int fd, const void* bytes, vertex::usize count);
long  read(int fd, void* bytes, vertex::usize count);
int   close(int fd);
double strtod(const char* text, char** end);

void* vertex_pal_alloc(vertex::usize size, vertex::usize) { return malloc(size); }
void  vertex_pal_free(void* p, vertex::usize, vertex::usize) { free(p); }
void  vertex_pal_copy(void* dest, const void* src, vertex::usize count) { memcpy(dest, src, count); }
void  vertex_pal_fill(void* dest, vertex::u8 byte, vertex::usize count) { memset(dest, byte, count); }
void  vertex_pal_abort(void) { abort(); }

inline constexpr int errnoInterrupted = 4;   // EINTR
inline constexpr int errnoExists = 17;       // EEXIST

// Conformance records are in the image's vertex_proto section, which the
// linker brackets with these two symbols. The runtime's own object puts a
// zero record there (see stdlib.TaskAsm), so the section exists in every
// image even when no module declared a conformance; a zero record is
// skipped. A library and the program that loads it each find their own.
extern const vertex::i32 __start_vertex_proto[];
extern const vertex::i32 __stop_vertex_proto[];

vertex::u32 vertex_pal_image_count(void) { return 1; }

const vertex::i32* vertex_pal_conformance_records(vertex::u32 image, vertex::usize* bytes) {
  *bytes = 0;
  if (image != 0)
    return nullptr;
  *bytes = static_cast<vertex::usize>(reinterpret_cast<const char*>(__stop_vertex_proto) -
                                      reinterpret_cast<const char*>(__start_vertex_proto));
  return __start_vertex_proto;
}

// write(2) straight to the descriptor: libc's stdio globals are data a
// program linked against a stub of libc would rather not reach for, and
// there is nothing to flush.
void vertex_pal_write(int stream, const vertex::u8* bytes, vertex::usize count) {
  int fd = stream == 2 ? 2 : 1;
  vertex::usize done = 0;
  while (done < count) {
    long n = write(fd, &bytes[done], count - done);
    if (n < 0) {
      if (*vertex_errno() == errnoInterrupted)
        continue;
      return;
    }
    done += static_cast<vertex::usize>(n);
  }
}

void vertex_pal_flush(void) {}

// strtod is the C locale's in a program that never calls setlocale; over a
// terminated copy.
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
  *out = strtod(text, &end);
  bool whole = count > 0 && end == text + count;
  if (text != scratch)
    free(text);
  return whole;
}

// Both libcs have it: bionic always, glibc since 2.36.
void arc4random_buf(void* bytes, vertex::usize count);

void vertex_pal_entropy(vertex::u8* bytes, vertex::usize count) {
  arc4random_buf(bytes, count);
}

// The kernel's initial stack, [argc, argv..., 0, envp..., 0], which the
// program's _start (vsc's build/elflink.go) saves here before handing it to
// libc. A library loaded by another program has no arguments of its own.
void* vertex_raw_args = nullptr;

const char* const* vertex_pal_arguments(int* count) {
  auto* raw = static_cast<long*>(vertex_raw_args);
  if (raw == nullptr) {
    *count = 0;
    static const char* const none[] = {nullptr};
    return none;
  }
  *count = static_cast<int>(raw[0]);
  return reinterpret_cast<const char* const*>(raw + 1);
}

int vertex_pal_read_byte(void) {
  unsigned char c = 0;
  for (;;) {
    long n = read(0, &c, 1);
    if (n == 1)
      return c;
    if (n < 0 && *vertex_errno() == errnoInterrupted)
      continue;
    return -1;
  }
}

struct timespec {
  long tv_sec;
  long tv_nsec;
};
int clock_gettime(int clock, timespec* ts);
int nanosleep(const timespec* request, timespec* remaining);

// CLOCK_MONOTONIC: stopped while the machine is suspended, as the program's
// own time should be.
vertex::u64 vertex_pal_now(void) {
  timespec ts{};
  clock_gettime(1, &ts);
  return static_cast<vertex::u64>(ts.tv_sec) * 1000000000u + static_cast<vertex::u64>(ts.tv_nsec);
}

void vertex_pal_sleep(vertex::u64 nanoseconds) {
  timespec request{static_cast<long>(nanoseconds / 1000000000u),
                   static_cast<long>(nanoseconds % 1000000000u)};
  timespec remaining{};
  while (nanosleep(&request, &remaining) != 0 && *vertex_errno() == errnoInterrupted)
    request = remaining;
}

// struct epoll_event, whose layout is the architecture's: the kernel packs
// it on x86-64, so its eight bytes of data follow the four of events with no
// padding and the struct is twelve bytes; on arm64 it is not packed, and is
// sixteen. The data is kept as two words here so that the struct has the
// packed layout without an attribute, and read and written through
// epollData and epollSetData.
#if defined(__x86_64__)
struct EpollEvent {
  vertex::u32 events;
  vertex::u32 data[2];
};
inline void* epollData(const EpollEvent& e) {
  void* p = nullptr;
  memcpy(&p, e.data, sizeof(p));
  return p;
}
inline void epollSetData(EpollEvent& e, void* p) { memcpy(e.data, &p, sizeof(p)); }
#else
struct EpollEvent {
  vertex::u32 events;
  void*       data;
};
inline void* epollData(const EpollEvent& e) { return e.data; }
inline void  epollSetData(EpollEvent& e, void* p) { e.data = p; }
#endif
int epoll_create1(int flags);
int epoll_ctl(int epfd, int op, int fd, EpollEvent* event);
int epoll_wait(int epfd, EpollEvent* events, int maxevents, int timeout);
int eventfd(unsigned int initval, int flags);

// An epoll instance per executor, and an eventfd registered on it for
// another thread to end a wait with. A registration is one-shot
// (EPOLLONESHOT): once it fires the descriptor stays in the set, disabled,
// and the next registration re-arms it with EPOLL_CTL_MOD. A descriptor
// closed meanwhile has left the set by itself, so ADD is tried first.
struct IoQueue {
  int ep;
  int wake;
};

inline constexpr vertex::u32 epollIn = 0x1;
inline constexpr vertex::u32 epollOut = 0x4;
inline constexpr vertex::u32 epollOneShot = 1u << 30;
inline constexpr int epollCtlAdd = 1;
inline constexpr int epollCtlDel = 2;
inline constexpr int epollCtlMod = 3;

void* vertex_pal_io_open(void) {
  int ep = epoll_create1(02000000);  // EPOLL_CLOEXEC
  if (ep < 0)
    return nullptr;
  int wake = eventfd(0, 02000000 | 04000);  // EFD_CLOEXEC | EFD_NONBLOCK
  if (wake < 0) {
    close(ep);
    return nullptr;
  }
  auto* q = static_cast<IoQueue*>(malloc(sizeof(IoQueue)));
  if (q == nullptr) {
    close(wake);
    close(ep);
    return nullptr;
  }
  q->ep = ep;
  q->wake = wake;
  EpollEvent e{};
  e.events = epollIn;
  epollSetData(e, nullptr);
  epoll_ctl(ep, epollCtlAdd, wake, &e);
  return q;
}

// An epoll instance is itself a descriptor, readable while it holds an event.
vertex::i32 vertex_pal_io_descriptor(void* queue) {
  auto* q = static_cast<IoQueue*>(queue);
  return q == nullptr ? -1 : q->ep;
}

int vertex_pal_io_register(void* queue, vertex::i32 fd, vertex::i32 events, void* token) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return -1;
  EpollEvent e{};
  e.events = (events == 2 ? epollOut : epollIn) | epollOneShot;
  epollSetData(e, token);
  if (epoll_ctl(q->ep, epollCtlAdd, fd, &e) == 0)
    return 0;
  if (*vertex_errno() != errnoExists)
    return -1;
  return epoll_ctl(q->ep, epollCtlMod, fd, &e) == 0 ? 0 : -1;
}

void vertex_pal_io_unregister(void* queue, vertex::i32 fd, vertex::i32) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return;
  EpollEvent e{};
  epoll_ctl(q->ep, epollCtlDel, fd, &e);
}

void vertex_pal_io_wake(void* queue) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return;
  vertex::u64 one = 1;
  write(q->wake, &one, sizeof(one));
}

struct PollFd {
  int   fd;
  short events;
  short revents;
};
int poll(PollFd* fds, unsigned long nfds, int timeout);

static int timeoutMillis(vertex::i64 timeout) {
  if (timeout < 0)
    return -1;
  vertex::i64 rounded = (timeout + 999999) / 1000000;
  return rounded > 0x7fffffff ? 0x7fffffff : static_cast<int>(rounded);
}

// POLLIN is 0x1 and POLLOUT is 0x4.
int vertex_pal_io_wait_one(vertex::i32 fd, vertex::i32 events, vertex::i64 timeout) {
  PollFd p{};
  p.fd = fd;
  p.events = events == 2 ? 0x4 : 0x1;
  int n = poll(&p, 1, timeoutMillis(timeout));
  if (n < 0)
    return -1;
  return n > 0 ? 1 : 0;
}

int vertex_pal_io_wait(void* queue, vertex::i64 timeout, void** tokens, int max) {
  auto* q = static_cast<IoQueue*>(queue);
  if (q == nullptr)
    return 0;
  EpollEvent ready[64];
  if (max > 64)
    max = 64;
  int n = epoll_wait(q->ep, ready, max, timeoutMillis(timeout));
  if (n < 0)
    return 0;
  for (int i = 0; i < n; i++) {
    tokens[i] = epollData(ready[i]);
    if (tokens[i] == nullptr) {
      // The wake: drain the counter so the next wait blocks again.
      vertex::u64 count = 0;
      read(q->wake, &count, sizeof(count));
    }
  }
  return n;
}

// Threads: pthreads. A pthread_t is a word and a key an int on both libcs.
typedef unsigned long PThread;
typedef unsigned int  PThreadKey;
int pthread_create(PThread* thread, const void* attr, void* (*start)(void*), void* arg);
int pthread_detach(PThread thread);
int pthread_key_create(PThreadKey* key, void (*destructor)(void*));
int pthread_setspecific(PThreadKey key, const void* value);
void* pthread_getspecific(PThreadKey key);
int get_nprocs(void);
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
  PThread t = 0;
  if (pthread_create(&t, nullptr, threadMain, s) != 0) {
    free(s);
    return false;
  }
  pthread_detach(t);
  return true;
}

int vertex_pal_cpus(void) {
  int n = get_nprocs();
  return n < 1 ? 1 : n;
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
