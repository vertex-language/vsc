// Division, remainder and multiplication by constants on the device: powers
// of two (which the backend makes shifts and masks) and others, signed and
// unsigned, over negatives and both ends of the range. Rounding is toward
// zero, and a remainder takes the dividend's sign, as on the host.
import "gpu"

func ops(_ x: gpu.Span<int>, _ u: gpu.Span<uint32>, _ out: gpu.MutableSpan<int>, _ uout: gpu.MutableSpan<uint32>) kernel {
    let i = gpu.Index.x
    let v = x[i]
    out[i * 8 + 0] = v / 8
    out[i * 8 + 1] = v % 8
    out[i * 8 + 2] = v / 1
    out[i * 8 + 3] = v % 1024
    out[i * 8 + 4] = v / 7
    out[i * 8 + 5] = v % 7
    out[i * 8 + 6] = v &* 16
    out[i * 8 + 7] = v / 4611686018427387904
    let w = u[i]
    uout[i * 4 + 0] = w / 16
    uout[i * 4 + 1] = w % 16
    uout[i * 4 + 2] = w &* 4
    uout[i * 4 + 3] = w / 3
}

let values: [int] = [0, 1, 7, 8, 9, -1, -7, -8, -9, 1023, -1025, int.max, int.min, -4611686018427387904, 123456789, -123456789]
let uvalues: [uint32] = [0, 1, 15, 16, 17, 255, 4294967295, 2147483648, 3, 99, 1000, 65535, 65536, 7, 8, 9]
for d in [gpu.CPU(), gpu.Default()] {
    let out = try d.CreateBuffer(of: int.self, count: values.count * 8)
    let uout = try d.CreateBuffer(of: uint32.self, count: uvalues.count * 4)
    try await ops.Launch(try await d.Upload(values), try await d.Upload(uvalues), out, uout, over: values.count)
    let o = try await out.Download(), uo = try await uout.Download()
    var host: [int] = [], uhost: [uint32] = []
    for v in values { host += [v / 8, v % 8, v / 1, v % 1024, v / 7, v % 7, v &* 16, v / 4611686018427387904] }
    for w in uvalues { uhost += [w / 16, w % 16, w &* 4, w / 3] }
    print(o == host && uo == uhost)
}
// want: true
// want: true
