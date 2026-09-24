// throw, try, and do-catch with a catch for each case of an error enum.
enum ParseError: Error {
    case empty
    case bad(Character)
}
func digit(_ s: String) throws -> Int {
    guard let c = s.first else { throw ParseError.empty }
    guard let d = c.wholeNumberValue else { throw ParseError.bad(c) }
    return d
}
for s in ["7", "", "x"] {
    do {
        print("digit", try digit(s))
    } catch ParseError.empty {
        print("empty")
    } catch ParseError.bad(let c) {
        print("bad", c)
    } catch {
        print("other", error)
    }
}
