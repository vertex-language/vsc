// #if chooses code at compile time; #line and #function report where they are.
#if swift(>=5.9)
let modern = true
#else
let modern = false
#endif
#if arch(arm64) || arch(x86_64)
let width = 64
#else
let width = 32
#endif
#if DEBUG_NEVER_DEFINED
print("never")
#endif
#if canImport(Swift)
let hasSwift = true
#else
let hasSwift = false
#endif
func here(line: Int = #line, function: String = #function) -> String { "\(function):\(line)" }
func caller() -> String { here() }
print(modern, width, hasSwift, #line, caller())
