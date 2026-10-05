// The runtime's floating-point descriptions, one per line, for every
// value in values.h: build/runtime_test.go compares them with swiftc's.
#include "runtime.h"
#include "platform.h"
#include "values.h"

using namespace vertex;

int main() {
  Text t;
  textInit(t);
  for (usize i = 0; i < sizeof(doubles) / sizeof(doubles[0]); i++) {
    describeFloat(t, doubles[i], 53, 11);
    textByte(t, '\n');
  }
  for (usize i = 0; i < sizeof(floats) / sizeof(floats[0]); i++) {
    describeFloat(t, floats[i], 24, 8);
    textByte(t, '\n');
  }
  vertex_pal_write(1, t.bytes, t.count);
  return 0;
}
