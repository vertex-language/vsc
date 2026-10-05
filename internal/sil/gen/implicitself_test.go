package gen

import (
	"regexp"
	"strings"
	"testing"
)

// TestImplicitSelfInMutatingMethod: inside a mutating method self is an
// address, and a method that does not mutate, called without writing
// self, takes self's value -- a copy loaded out of the address, as
// `self.nonce()` would pass. Passing the address itself went unnoticed
// until the callee was in another module, where lowering failed with an
// arity mismatch.
func TestImplicitSelfInMutatingMethod(t *testing.T) {
	got, diags := generate(t, "lib", `
struct Cipher {
    var key: [Int]
    func nonce() -> [Int] { return key }
    mutating func next() -> Int {
        let n = nonce()
        key = n
        return n.count
    }
}
`)
	for _, d := range diags {
		t.Fatalf("gen: %s", d.Message)
	}
	i := strings.Index(got, "@$s3lib6CipherV4nextSiyF")
	if i < 0 {
		t.Fatalf("no next() in:\n%s", got)
	}
	body := got[i:]
	call := regexp.MustCompile(`apply %\d+\((%\d+)\) : \$@convention\(method\) \(@guaranteed Cipher\)`).FindStringSubmatch(body)
	if call == nil {
		t.Fatalf("no call of nonce() in:\n%s", body)
	}
	if !regexp.MustCompile(regexp.QuoteMeta(call[1]) + ` = load `).MatchString(body) {
		t.Errorf("nonce() is passed %s, which is not a value loaded from self:\n%s", call[1], body)
	}
}
