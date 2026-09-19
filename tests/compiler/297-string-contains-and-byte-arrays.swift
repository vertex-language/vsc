// A String contains another when the other's Characters appear in it as a
// run, compared as Characters are: canonically equivalent spellings match,
// and a combining mark joined to a letter is not that letter. Every string
// contains the empty one. [UInt8](s.utf8) is the string's bytes.

func count(_ haystack: String, _ needles: [String]) -> Int {
    var n = 0
    for needle in needles where haystack.contains(needle) {
        n += 1
    }
    return n
}

func main() -> Int32 {
    print("abc".contains(""), "".contains(""), "".contains("a"))
    print("café".contains("e"), "cafe\u{301}".contains("café"), "e\u{301}x".contains("e"))
    print("hello".contains("llo"), "hello".contains("lol"), "aaab".contains("aab"), "ab".contains("abc"))

    let sdp = "v=0\r\na=ice-ufrag:u\r\na=candidate:1 1 udp\r\n"
    let found = count(sdp, ["a=ice", "ufrag:u\r\n", "candidate:1", "x", "\r\n"])
    print(found)

    let bytes = [UInt8](sdp.utf8)
    var copy = [UInt8](bytes)
    copy.append(0)
    let words = [String](["a", "b"])
    print(bytes.count, copy.count, bytes[0], bytes[bytes.count - 1], words.count)
    let accented = [UInt8]("é".utf8)
    print(accented)
    return Int32(found * 10 + accented.count)
}
