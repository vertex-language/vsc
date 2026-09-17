package mangle

import "strconv"

// Word substitutions.
//
// Swift's mangler does not spell a name twice. It splits every
// identifier it writes into words, remembers each one that is at
// least two characters, and writes a back-reference where a later
// identifier repeats one -- so `sumPair(_ p: Pair)` spells "Pair"
// once, in the function's own name, and the parameter's type is a
// letter pointing back at it.
//
// This is not an optimization that can be skipped. A symbol is either
// the name swiftc gives it or it is a different symbol, and a
// compiler that spells the name in full links to nothing where swiftc
// linked to something. It is why calling a real Swift library needed
// this before it needed anything else.
//
// # The encoding
//
// An identifier with no repeated word is a length and the text.
// One with repeated words is
//
//	'0' ( length text )? ( letter ( length text )? )* length text?
//
// -- a leading zero to say which form this is, then the text and the
// substitutions in the order they appear, then whatever is left, with
// its length, always written even when it is empty. A substitution is
// a letter: lowercase for every one but the last, uppercase for the
// last, which is what tells a reader where the run ends.
//
//	sumPair      Pair               0C0          word 2, nothing left
//	sumPairAgain PairBox            0C3Box       word 2, then "Box"
//	sumBox       PairBox            04PairC0     "Pair", then word 2
//	makeHttpRequest HttpRequestHandler 0cD7Handler  words 2 and 3, then "Handler"
//
// Every one of those is swiftc's own answer, read out of its object
// file; mangle/testdata/words.swift is the corpus and the oracle test
// compares against it.
//
// # What a word is
//
// A word begins at the start of the identifier, at an uppercase
// letter following a lowercase one, and at the last uppercase letter
// of a run that is followed by a lowercase one -- so HttpRequest is
// two words and HTTPRequest is "HTTP" and "Request". A digit or an
// underscore belongs to the word it follows and starts none.
//
// The comparison is exact. `box` and `Box` are two words, which is
// why `boxPair` substitutes only the Pair.

// maxWords is how many words a symbol remembers. The letters that
// spell a substitution are a-z and A-Z used as one alphabet of
// twenty-six, so there is nowhere to put a twenty-seventh.
const maxWords = 26

// splitWords is the words of an identifier, in order.
func splitWords(name string) []string {
	var out []string
	start := 0
	for i := 1; i <= len(name); i++ {
		if i == len(name) {
			if i > start {
				out = append(out, name[start:i])
			}
			break
		}
		if !wordStart(name, i) {
			continue
		}
		if i > start {
			out = append(out, name[start:i])
		}
		start = i
	}
	return out
}

// wordStart reports whether a word begins at i.
func wordStart(name string, i int) bool {
	if i == 0 || i >= len(name) {
		return false
	}
	c := name[i]
	if !isUpper(c) {
		return false
	}
	// An uppercase letter after a lowercase one begins a word:
	// the P of sumPair.
	if isLower(name[i-1]) {
		return true
	}
	// The last uppercase of a run begins one, when a lowercase
	// follows: HTTPRequest is HTTP and Request, not HTTPR and equest.
	if isUpper(name[i-1]) && i+1 < len(name) && isLower(name[i+1]) {
		return true
	}
	return false
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' }

// rememberWords adds an identifier's words to the table.
//
// Only words of two characters or more: a one-letter word costs a
// letter to point at and a letter to spell, so Swift does not
// remember one.
func (m *mangler) rememberWords(name string) {
	for _, w := range splitWords(name) {
		if len(w) < 2 || len(m.words) >= maxWords {
			continue
		}
		if m.wordIndex(w) >= 0 {
			continue
		}
		m.words = append(m.words, w)
	}
}

// wordIndex is where a word sits in the table, or -1.
func (m *mangler) wordIndex(w string) int {
	for i, have := range m.words {
		if have == w {
			return i
		}
	}
	return -1
}

// substitutedIdentifier writes a name using the words already
// written, and reports whether it did. It writes nothing and returns
// false when no word of the name has been seen before, which is the
// ordinary length-and-text form the caller then writes.
func (m *mangler) substitutedIdentifier(name string) bool {
	words := splitWords(name)
	// Where each word sits in the table, or -1.
	at := make([]int, len(words))
	any := false
	for i, w := range words {
		at[i] = -1
		if len(w) >= 2 {
			at[i] = m.wordIndex(w)
			if at[i] >= 0 {
				any = true
			}
		}
	}
	if !any {
		return false
	}

	// The last substitution is the one that carries the uppercase
	// letter, so it has to be known before any of them is written.
	last := -1
	for i := range words {
		if at[i] >= 0 {
			last = i
		}
	}

	m.write("0")
	var pending string
	for i, w := range words {
		if at[i] < 0 {
			pending += w
			continue
		}
		if pending != "" {
			m.write(strconv.Itoa(len(pending)))
			m.write(pending)
			pending = ""
		}
		m.writeByte(wordLetter(at[i], i == last))
	}
	// Whatever is left, with its length -- written even when it is
	// nothing, because a reader has to be told the run has ended.
	m.write(strconv.Itoa(len(pending)))
	m.write(pending)
	return true
}

// wordLetter spells one substitution index. Lowercase says another
// follows; uppercase says this is the last.
func wordLetter(i int, last bool) byte {
	if last {
		return byte('A' + i)
	}
	return byte('a' + i)
}
