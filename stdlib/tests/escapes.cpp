// The runtime's debug description of each string in strings.h, one per
// line: build/runtime_test.go compares them with swiftc's.
#include "runtime.h"
#include "platform.h"
#include "strings.h"

using namespace vertex;

int main() {
  Text t;
  textInit(t);
  for (usize i = 0; i < sizeof(strings) / sizeof(strings[0]); i++) {
    debugString(t, vertex_string_from_utf8(strings[i].bytes, strings[i].count));
    textByte(t, '\n');
  }
  vertex_pal_write(1, t.bytes, t.count);
  return 0;
}
