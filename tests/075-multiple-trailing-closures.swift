// More than one trailing closure, the later ones labelled.
func perform(_ n: Int, success: (Int) -> Void, failure: (String) -> Void) {
    if n >= 0 { success(n * 2) } else { failure("negative \(n)") }
}
perform(4) { print("ok", $0) } failure: { print("err", $0) }
perform(-1) { print("ok", $0) } failure: { print("err", $0) }
