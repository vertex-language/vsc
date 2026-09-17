// print, and String(describing:).
#include "vertex/abi.h"
#include "vertex/platform.h"
#include "text.h"

using namespace vertex;

extern "C" {

// vertex_print is `print(_ items: Any..., separator:, terminator:)`: each
// item described, the separator between them, the terminator after, and
// all of it written to standard output at once.
void vertex_print(ArrayStorage* items, u64 sep0, u64 sep1, u64 term0, u64 term1) {
  Text t;
  textInit(t);
  auto* at = reinterpret_cast<const u8*>(items) + arrayStorageElements;
  for (i64 i = 0; i < items->count; i++) {
    if (i > 0)
      describeString(t, String{sep0, sep1});
    auto* any = reinterpret_cast<const AnyExistential*>(at + i * sizeof(AnyExistential));
    describe(t, anyContents(any), any->type);
  }
  describeString(t, String{term0, term1});
  vertex_pal_write(1, t.bytes, t.count);
  textFree(t);
}

// vertex_read_line is `readLine(strippingNewline:)`: the bytes of standard
// input up to and including the next newline, or to its end, as a
// String -- with the newline, or a carriage return and newline, taken
// off where asked -- or nil at the end of input with nothing read. Bytes
// that are not UTF-8 read as U+FFFD, as Swift's own readLine repairs them.
String vertex_read_line(bool strippingNewline) {
  Text raw;
  textInit(raw);
  int c = vertex_pal_read_byte();
  if (c < 0) {
    textFree(raw);
    return {0, 0};
  }
  while (c >= 0) {
    textByte(raw, static_cast<u8>(c));
    if (c == '\n')
      break;
    c = vertex_pal_read_byte();
  }
  usize n = raw.count;
  if (strippingNewline && n > 0 && raw.bytes[n - 1] == '\n') {
    n--;
    if (n > 0 && raw.bytes[n - 1] == '\r')
      n--;
  }
  Text valid;
  textInit(valid);
  appendRepairedUTF8(valid, raw.bytes, n);
  String s = makeString(valid.bytes, valid.count);
  textFree(valid);
  textFree(raw);
  return s;
}

// vertex_command_line_arguments is CommandLine.arguments: an Array of the
// program's arguments as Strings, its own path first.
ArrayStorage* vertex_command_line_arguments(void) {
  int count = 0;
  const char* const* argv = vertex_pal_arguments(&count);
  ArrayAllocation got = vertex_array_allocate(count, &vertex_metadata_String.metadata);
  for (int i = 0; i < count; i++) {
    usize n = 0;
    while (argv[i][n] != 0)
      n++;
    Text t;
    textInit(t);
    appendRepairedUTF8(t, reinterpret_cast<const u8*>(argv[i]), n);
    String s = makeString(t.bytes, t.count);
    textFree(t);
    copyBytes(got.elements + static_cast<usize>(i) * sizeof(String), &s, sizeof(String));
  }
  return got.array;
}

bool vertex_string_is_empty(u64 countAndFlags, u64 object) {
  u8 scratch[16];
  return bytesOf(String{countAndFlags, object}, scratch).count == 0;
}

// vertex_string_description is String.description: the string itself,
// with a reference of the caller's own.
String vertex_string_description(u64 countAndFlags, u64 object) {
  vertex_string_retain(object);
  return {countAndFlags, object};
}

// vertex_string_debug_description is String.debugDescription: quoted, and
// escaped as Swift escapes it.
String vertex_string_debug_description(u64 countAndFlags, u64 object) {
  Text t;
  textInit(t);
  debugString(t, String{countAndFlags, object});
  String s = makeString(t.bytes, t.count);
  textFree(t);
  return s;
}

// vertex_describe is `String(describing:)`: the value by address, since
// its size is known only to its metadata, which follows it.
String vertex_describe(const void* value, const Metadata* type) {
  Text t;
  textInit(t);
  describe(t, value, type);
  String s = makeString(t.bytes, t.count);
  textFree(t);
  return s;
}

}
