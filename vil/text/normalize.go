package text

import (
	"regexp"
	"strings"
)

// silAttrs are the `@` words that are part of the language rather
// than names of things. They must survive normalization untouched:
// `@owned` against `@guaranteed` is exactly the kind of difference
// this harness exists to catch.
var silAttrs = map[string]bool{
	"@convention": true, "@owned": true, "@guaranteed": true,
	"@unowned": true, "@in": true, "@in_guaranteed": true,
	"@inout": true, "@inout_aliasable": true, "@out": true,
	"@error": true, "@yields": true, "@yield_once": true,
	"@yield_many": true, "@thin": true, "@thick": true,
	"@objc_metatype": true, "@escaping": true, "@noescape": true,
	"@callee_guaranteed": true, "@callee_owned": true,
	"@autoreleased": true, "@opened": true, "@async": true,
	"@objc": true, "@block_storage": true, "@sil_isolated": true,
	"@sil_sending": true, "@pack_guaranteed": true, "@pack_owned": true,
	"@dynamic_self": true, "@moveOnly": true, "@isolated": true,
}

var (
	reSymbol  = regexp.MustCompile(`@\$?[A-Za-z_$][A-Za-z0-9_$]*`)
	reValue   = regexp.MustCompile(`%[0-9]+`)
	reComment = regexp.MustCompile(`\s*//.*$`)
	reOpened  = regexp.MustCompile(`@opened\("[^"]*"`)
)

// Normalize reduces SIL text to what two compilers must agree on.
//
// It is the one licence the differential harness takes, and it covers
// exactly three things that differ between VIL and SIL without
// meaning anything:
//
//   - symbols, because VIL does not clone Swift's mangling, which
//     encodes Swift's own module and declaration grammar
//   - the %n numbering, which follows from the symbols
//   - the trailing // user: and // id: cross-references, which
//     restate the def-use graph the instructions already carry
//
// The `@` words that are part of the language are held back from it
// by name: `@owned` against `@guaranteed` is exactly the kind of
// difference the harness exists to catch, and a normalizer that
// collapsed both into a placeholder would hide it.
func Normalize(text string) string {
	var out []string
	symbols := map[string]string{}
	values := map[string]string{}

	for _, line := range strings.Split(text, "\n") {
		line = reComment.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		line = reOpened.ReplaceAllString(line, `@opened("_"`)
		line = reSymbol.ReplaceAllStringFunc(line, func(s string) string {
			if silAttrs[s] {
				return s
			}
			if _, ok := symbols[s]; !ok {
				symbols[s] = "@f" + itoa(len(symbols))
			}
			return symbols[s]
		})
		line = reValue.ReplaceAllStringFunc(line, func(s string) string {
			if _, ok := values[s]; !ok {
				values[s] = "%" + itoa(len(values))
			}
			return values[s]
		})
		out = append(out, strings.TrimRight(line, " "))
	}
	return strings.Join(out, "\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
