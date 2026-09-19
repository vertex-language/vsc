// The platform layer for Windows: the static UCRT.
#pragma once

#include "vertex/platform.h"

struct _iobuf;

extern "C" {
void* malloc(vertex::usize size);
void  free(void* p);
[[noreturn]] void abort(void);
vertex::usize fwrite(const void* bytes, vertex::usize size, vertex::usize count, _iobuf* stream);
_iobuf* __acrt_iob_func(unsigned index);
int getc(_iobuf* stream);
int fflush(_iobuf* stream);

void* vertex_pal_alloc(vertex::usize size, vertex::usize) { return malloc(size); }
void  vertex_pal_free(void* p, vertex::usize, vertex::usize) { free(p); }
// No conformance records are listed on Windows yet: a conformance is
// not found at run time there, and a value is described by reflection.
vertex::u32 vertex_pal_image_count(void) { return 0; }
const vertex::i32* vertex_pal_conformance_records(vertex::u32, vertex::usize* bytes) {
  *bytes = 0;
  return nullptr;
}

void  vertex_pal_abort(void) { abort(); }

void vertex_pal_write(int stream, const vertex::u8* bytes, vertex::usize count) {
  fwrite(bytes, 1, count, __acrt_iob_func(stream == 2 ? 2 : 1));
}

void vertex_pal_flush(void) { fflush(__acrt_iob_func(1)); }

double strtod(const char* text, char** end);

bool vertex_pal_parse_double(const vertex::u8* bytes, vertex::usize count, double* out) {
  char scratch[128];
  if (count + 1 > sizeof(scratch))
    return false;
  for (vertex::usize i = 0; i < count; i++)
    scratch[i] = static_cast<char>(bytes[i]);
  scratch[count] = 0;
  char* end = nullptr;
  *out = strtod(scratch, &end);
  return count > 0 && end == scratch + count;
}

int rand_s(unsigned* value);

void vertex_pal_entropy(vertex::u8* bytes, vertex::usize count) {
  for (vertex::usize i = 0; i < count; i++) {
    unsigned v = 0;
    rand_s(&v);
    bytes[i] = static_cast<vertex::u8>(v);
  }
}

int* __p___argc(void);
wchar_t*** __p___wargv(void);
int WideCharToMultiByte(unsigned codePage, unsigned long flags, const wchar_t* wide, int wideCount,
                        char* multi, int multiCount, const char* defaultChar, int* usedDefault);

// The CRT's wide arguments, converted to UTF-8 once.
const char* const* vertex_pal_arguments(int* count) {
  static char** converted = nullptr;
  int n = *__p___argc();
  *count = n;
  if (converted != nullptr)
    return const_cast<const char* const*>(converted);
  wchar_t** wide = *__p___wargv();
  converted = static_cast<char**>(malloc(sizeof(char*) * static_cast<vertex::usize>(n + 1)));
  for (int i = 0; i < n; i++) {
    int bytes = WideCharToMultiByte(65001, 0, wide[i], -1, nullptr, 0, nullptr, nullptr);
    converted[i] = static_cast<char*>(malloc(static_cast<vertex::usize>(bytes > 0 ? bytes : 1)));
    if (bytes > 0)
      WideCharToMultiByte(65001, 0, wide[i], -1, converted[i], bytes, nullptr, nullptr);
    else
      converted[i][0] = 0;
  }
  converted[n] = nullptr;
  return const_cast<const char* const*>(converted);
}

int vertex_pal_read_byte(void) {
  fflush(__acrt_iob_func(1));
  return getc(__acrt_iob_func(0));
}

int QueryPerformanceCounter(vertex::i64* count);
int QueryPerformanceFrequency(vertex::i64* frequency);
void Sleep(unsigned long milliseconds);

vertex::u64 vertex_pal_now(void) {
  vertex::i64 count = 0, frequency = 1;
  QueryPerformanceCounter(&count);
  QueryPerformanceFrequency(&frequency);
  vertex::u64 c = static_cast<vertex::u64>(count), f = static_cast<vertex::u64>(frequency);
  return c / f * 1000000000u + c % f * 1000000000u / f;
}

// No readiness registration yet, so a task's wait falls back to
// vertex_pal_io_wait_one and stops the thread, as a wait outside a task
// does. That is slower than the executor's, never wrong.
void* vertex_pal_io_open(void) { return nullptr; }
int vertex_pal_io_register(void*, vertex::i32, vertex::i32, void*) { return -1; }
int vertex_pal_io_wait(void*, vertex::i64, void**, int) { return 0; }
void vertex_pal_io_unregister(void*, vertex::i32, vertex::i32) {}
vertex::i32 vertex_pal_io_descriptor(void*) { return -1; }
void vertex_pal_io_wake(void*) {}

// No worker threads yet: the executor asks for none, and everything runs
// on the main thread as it did. The thread slot is then one global.
bool vertex_pal_thread_start(void (*)(void*), void*) { return false; }
int  vertex_pal_cpus(void) { return 1; }
static void* threadSlot;
void  vertex_pal_thread_set(void* value) { threadSlot = value; }
void* vertex_pal_thread_get(void) { return threadSlot; }
char* getenv(const char* name);
const char* vertex_pal_getenv(const char* name) { return getenv(name); }

struct WSAPollFd {
  vertex::usize fd;
  short         events;
  short         revents;
};
int WSAPoll(WSAPollFd* fds, unsigned long nfds, int timeout);

// POLLRDNORM is 0x100 and POLLWRNORM 0x10: WSAPoll takes those, not the
// POLLIN and POLLOUT that poll(2) does.
int vertex_pal_io_wait_one(vertex::i32 fd, vertex::i32 events, vertex::i64 timeout) {
  WSAPollFd p{};
  p.fd = static_cast<vertex::usize>(fd);
  p.events = events == 2 ? 0x10 : 0x100;
  int ms = -1;
  if (timeout >= 0) {
    vertex::i64 rounded = (timeout + 999999) / 1000000;
    ms = rounded > 0x7fffffff ? 0x7fffffff : static_cast<int>(rounded);
  }
  int n = WSAPoll(&p, 1, ms);
  if (n < 0)
    return -1;
  return n > 0 ? 1 : 0;
}

// Sleep counts in milliseconds; a shorter wait rounds up to one.
void vertex_pal_sleep(vertex::u64 nanoseconds) {
  vertex::u64 ms = (nanoseconds + 999999u) / 1000000u;
  Sleep(static_cast<unsigned long>(ms));
}
}
