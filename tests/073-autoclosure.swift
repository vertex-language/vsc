// @autoclosure delays evaluating an argument until it is asked for.
func log(_ enabled: Bool, _ message: @autoclosure () -> String) {
    if enabled { print(message()) }
}
func expensive() -> String {
    print("computing")
    return "the message"
}
log(false, expensive())
log(true, expensive())
