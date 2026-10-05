package mangle

import "strconv"

// Swift writes a name that is not ASCII in Punycode (RFC 3492), in a
// variant of its own: the digits are a-z then A-J, so that no digit can
// be read as a length, and the delimiter after the ASCII part is `_`.
// The identifier is marked with a leading 00 before its length: swiftc
// writes `héllo` as 008hllo_bpa.

const (
	punyBase        = 36
	punyTMin        = 1
	punyTMax        = 26
	punySkew        = 38
	punyDamp        = 700
	punyInitialBias = 72
	punyInitialN    = 128
)

func punyDigit(d int) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('A' + d - 26)
}

func punyAdapt(delta, points int, first bool) int {
	if first {
		delta /= punyDamp
	} else {
		delta /= 2
	}
	delta += delta / points
	k := 0
	for delta > ((punyBase-punyTMin)*punyTMax)/2 {
		delta /= punyBase - punyTMin
		k += punyBase
	}
	return k + (punyBase-punyTMin+1)*delta/(delta+punySkew)
}

// punycode is s encoded as swiftc encodes an identifier's characters.
func punycode(s string) string {
	runes := []rune(s)
	var out []byte
	for _, r := range runes {
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	h, b := len(out), len(out)
	if b > 0 {
		out = append(out, '_')
	}
	n, delta, bias := punyInitialN, 0, punyInitialBias
	for h < len(runes) {
		m := -1
		for _, r := range runes {
			if int(r) >= n && (m < 0 || int(r) < m) {
				m = int(r)
			}
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range runes {
			if int(r) < n {
				delta++
			}
			if int(r) != n {
				continue
			}
			q := delta
			for k := punyBase; ; k += punyBase {
				t := k - bias
				if t < punyTMin {
					t = punyTMin
				} else if t > punyTMax {
					t = punyTMax
				}
				if q < t {
					break
				}
				out = append(out, punyDigit(t+(q-t)%(punyBase-t)))
				q = (q - t) / (punyBase - t)
			}
			out = append(out, punyDigit(q))
			bias = punyAdapt(delta, h+1, h == b)
			delta = 0
			h++
		}
		delta++
		n++
	}
	return string(out)
}

// isASCII reports whether s is all ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// writePunycode writes a name that is not ASCII: 00, the length of its
// encoding, and the encoding, with a `_` first where the encoding would
// otherwise start with something a length could be read into.
func (m *mangler) writePunycode(name string) {
	enc := punycode(name)
	m.write("00")
	m.write(strconv.Itoa(len(enc)))
	if c := enc[0]; c == '_' || c >= '0' && c <= '9' {
		m.writeByte('_')
	}
	m.write(enc)
}
