// withUnsafeBufferPointer on a class's stored array, the closure writing
// through a captured inout parameter.
final class Level {
    var Bytes: [UInt8]
    init(_ b: [UInt8]) { Bytes = b }
}
struct Request {
    var Result: [Float] = [0, 0, 0, 0]
    var Names: [String] = ["r", "g", "b", "a"]
}
func sample(_ r: inout Request, _ lv: Level) {
    lv.Bytes.withUnsafeBufferPointer { p in
        for i in 0..<4 { r.Result[i] = Float(p[i]) / 255 }
    }
}
var req = Request()
sample(&req, Level([0, 51, 255, 102]))
print(req.Result, req.Names)
