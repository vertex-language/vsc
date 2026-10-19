package mangle

import "strconv"

// Word substitutions in Swift symbol mangling.
//
// Swift ABI mangling compresses repeated identifier words (>= 2 characters)
// using back-references. Repeated words are encoded as:
//
//	'0' ( length text )? ( letter ( length text )? )* length text?
//
// A leading '0' indicates substitution mode. Intermediate substitutions use
// lowercase letters ('a'-'z'), and the final substitution uses uppercase ('A'-'Z').
//
// Words are delineated by camelCase/PascalCase boundaries, uppercase runs
// preceding lowercase letters, and digits/underscores adhering to preceding words.

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
