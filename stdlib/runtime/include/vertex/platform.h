// The platform layer: the only place the runtime meets an operating
// system. Each target's platform file defines these, and nothing else
// in the runtime names anything the OS provides.
#pragma once

#include "vertex/abi.h"

extern "C" {

// Memory. Size and alignment both, so that an allocator which needs to
// be told what it is freeing has it.
void* vertex_pal_alloc(vertex::usize size, vertex::usize align);
void  vertex_pal_free(void* p, vertex::usize size, vertex::usize align);

// Bytes to a standard stream: 1 is output, 2 is error.
void vertex_pal_write(int stream, const vertex::u8* bytes, vertex::usize count);

// Whatever has been written to standard output and is still buffered goes
// out now: before the process ends without unwinding.
void vertex_pal_flush(void);

// The program's arguments, as UTF-8 strings, and how many there are.
const char* const* vertex_pal_arguments(int* count);

// Random bytes, for the hashing seed.
void vertex_pal_entropy(vertex::u8* bytes, vertex::usize count);

// The next byte of standard input, or -1 at its end.
int vertex_pal_read_byte(void);

// A monotonic clock, in nanoseconds from some fixed point: for deadlines,
// never for the time of day.
vertex::u64 vertex_pal_now(void);

// The thread sleeps for at least this long.
void vertex_pal_sleep(vertex::u64 nanoseconds);

// Readiness of descriptors. register asks to be told, once, when fd can
// be read from (events 1) or written to (2), with token handed back;
// wait hands back up to max tokens whose descriptors are ready, waiting
// up to timeout nanoseconds for one (-1: until one is; 0: not at all).
int vertex_pal_io_register(vertex::i32 fd, vertex::i32 events, void* token);
int vertex_pal_io_wait(vertex::i64 timeout, void** tokens, int max);

// A descriptor that is readable whenever a registration is ready, for a
// wait that is not this layer's to watch; -1 where there is none.
vertex::i32 vertex_pal_io_descriptor(void);

// Takes back a registration that has not fired, which is what a wait
// abandons when its deadline passes first.
void vertex_pal_io_unregister(vertex::i32 fd, vertex::i32 events);

// Waits on one descriptor with this thread, rather than through the
// executor: the whole thread stops until fd is ready or the timeout
// passes. It is how a wait outside any task waits, and the fallback
// wherever registration is not available. 1 ready, 0 timed out, -1 the
// platform could not wait. A negative timeout waits indefinitely.
int vertex_pal_io_wait_one(vertex::i32 fd, vertex::i32 events, vertex::i64 timeout);

// The program's conformance records: the n-th image's list of relative
// pointers to conformance descriptors, one i32 each, and how many bytes
// of them there are. Null where there is no n-th image, or it has none.
// Images are asked for from 0 until the count is reached.
vertex::u32 vertex_pal_image_count(void);
const vertex::i32* vertex_pal_conformance_records(vertex::u32 image, vertex::usize* bytes);

// A floating-point number read from the text, in the C locale: the whole
// of it has to be the number. False where it is not one.
bool vertex_pal_parse_double(const vertex::u8* bytes, vertex::usize count, double* out);

// The process ends here, without unwinding anything.
[[noreturn]] void vertex_pal_abort(void);

}
