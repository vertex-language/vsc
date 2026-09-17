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
[[noreturn]] void abort(void);
vertex::usize fwrite(const void* bytes, vertex::usize size, vertex::usize count, __sFILE* stream);
extern __sFILE* __stdoutp;
extern __sFILE* __stderrp;
extern __sFILE* __stdinp;
int getc(__sFILE* stream);
int fflush(__sFILE* stream);

void* vertex_pal_alloc(vertex::usize size, vertex::usize) { return malloc(size); }
void  vertex_pal_free(void* p, vertex::usize, vertex::usize) { free(p); }
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

static int ioQueue = -1;

// A kqueue is itself a descriptor, readable while it holds an event.
vertex::i32 vertex_pal_io_descriptor(void) {
  if (ioQueue < 0)
    ioQueue = kqueue();
  return ioQueue;
}

// EVFILT_READ or EVFILT_WRITE, once: EV_ADD | EV_ONESHOT.
int vertex_pal_io_register(vertex::i32 fd, vertex::i32 events, void* token) {
  if (ioQueue < 0) {
    ioQueue = kqueue();
    if (ioQueue < 0)
      return -1;
  }
  KEvent change{};
  change.ident = static_cast<vertex::usize>(fd);
  change.filter = events == 2 ? -2 : -1;
  change.flags = 0x1 | 0x10;
  change.udata = token;
  return kevent(ioQueue, &change, 1, nullptr, 0, nullptr) < 0 ? -1 : 0;
}

// EV_DELETE for whichever filter was registered. A registration that has
// already fired is gone, and kevent says so; there is nothing to do about
// it either way.
void vertex_pal_io_unregister(vertex::i32 fd, vertex::i32 events) {
  if (ioQueue < 0)
    return;
  KEvent change{};
  change.ident = static_cast<vertex::usize>(fd);
  change.filter = events == 2 ? -2 : -1;
  change.flags = 0x2;  // EV_DELETE
  kevent(ioQueue, &change, 1, nullptr, 0, nullptr);
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

int vertex_pal_io_wait(vertex::i64 timeout, void** tokens, int max) {
  if (ioQueue < 0)
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
  int n = kevent(ioQueue, nullptr, 0, ready, max, until);
  if (n < 0)
    return 0;
  for (int i = 0; i < n; i++)
    tokens[i] = ready[i].udata;
  return n;
}
}
