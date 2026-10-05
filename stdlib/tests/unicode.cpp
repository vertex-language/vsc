// The runtime's Unicode against the Unicode Consortium's own conformance
// files: every grapheme break case, and every normalization case.
// build/runtime_test.go generates cases.h from testdata/.
#include "runtime.h"
#include "platform.h"
#include "cases.h"

using namespace vertex;

int main() {
  Text t;
  textInit(t);
  u64 failures = 0;
  for (usize i = 0; i < sizeof(graphemeCases) / sizeof(graphemeCases[0]); i++) {
    const GraphemeCase& c = graphemeCases[i];
    usize got = graphemeCount(c.bytes, c.count);
    if (got != c.clusters) {
      failures++;
      textString(t, "GraphemeBreakTest line ");
      textUnsigned(t, c.line);
      textString(t, ": ");
      textUnsigned(t, got);
      textString(t, " clusters, want ");
      textUnsigned(t, c.clusters);
      textByte(t, '\n');
    }
  }
  u32 nfc[256];
  for (usize i = 0; i < sizeof(normalizationCases) / sizeof(normalizationCases[0]); i++) {
    const NormalizationCase& c = normalizationCases[i];
    usize n = normalizeNFC(c.source, c.sourceCount, nfc);
    bool ok = n == c.nfcCount;
    for (usize k = 0; ok && k < n; k++)
      ok = nfc[k] == c.nfc[k];
    ok = ok && canonicalCompare(c.source, c.sourceCount, c.composed, c.composedCount) == 0 &&
         canonicalCompare(c.source, c.sourceCount, c.decomposed, c.decomposedCount) == 0;
    if (!ok) {
      failures++;
      textString(t, "NormalizationTest line ");
      textUnsigned(t, c.line);
      textByte(t, '\n');
    }
  }
  vertex_pal_write(1, t.bytes, t.count);
  return failures == 0 ? 0 : 1;
}
