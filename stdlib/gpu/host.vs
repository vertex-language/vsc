package gpu

// The host half of the built-in gpu module: devices, buffers, and the
// launches the compiler writes. See proposed_vertex_kernel.md §7.
//
// Each call here is the gpu runtime unit's (stdlib/runtime/gpu), which is
// linked into a program that imports gpu and into no other.

@_silgen_name("vertex_gpu_device_default") func _deviceDefault() -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_device_cpu") func _deviceCPU() -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_device_count") func _deviceCount() -> int
@_silgen_name("vertex_gpu_device_at") func _deviceAt(_ i: int) -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_device_kind") func _deviceKind(_ d: UnsafeMutableRawPointer) -> int
@_silgen_name("vertex_gpu_device_memory") func _deviceMemory(_ d: UnsafeMutableRawPointer) -> int
@_silgen_name("vertex_gpu_device_wave_size") func _deviceWaveSize(_ d: UnsafeMutableRawPointer) -> int
@_silgen_name("vertex_gpu_buffer_create") func _bufferCreate(_ d: UnsafeMutableRawPointer, _ bytes: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_buffer_release") func _bufferRelease(_ b: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_buffer_contents") func _bufferContents(_ b: UnsafeMutableRawPointer) -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_buffer_device") func _bufferDevice(_ b: UnsafeMutableRawPointer) -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_launch_begin") func _launchBegin(_ kernel: UnsafeRawPointer) -> UnsafeMutableRawPointer
@_silgen_name("vertex_gpu_launch_buffer") func _launchBuffer(_ l: UnsafeMutableRawPointer, _ b: UnsafeMutableRawPointer, _ offset: int)
@_silgen_name("vertex_gpu_launch_i8") func _launchI8(_ l: UnsafeMutableRawPointer, _ v: int8)
@_silgen_name("vertex_gpu_launch_i16") func _launchI16(_ l: UnsafeMutableRawPointer, _ v: int16)
@_silgen_name("vertex_gpu_launch_i32") func _launchI32(_ l: UnsafeMutableRawPointer, _ v: int32)
@_silgen_name("vertex_gpu_launch_i64") func _launchI64(_ l: UnsafeMutableRawPointer, _ v: int)
@_silgen_name("vertex_gpu_launch_f32") func _launchF32(_ l: UnsafeMutableRawPointer, _ v: float32)
@_silgen_name("vertex_gpu_launch_f64") func _launchF64(_ l: UnsafeMutableRawPointer, _ v: float64)
@_silgen_name("vertex_gpu_launch_device") func _launchDevice(_ l: UnsafeMutableRawPointer, _ d: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_launch_end") func _launchEnd(_ l: UnsafeMutableRawPointer, _ gx: int, _ gy: int, _ gz: int, _ wx: int, _ wy: int, _ wz: int) -> int32

/// DeviceError is what asking a device for something can fail with.
public enum DeviceError: Error {
    /// noMemory: the device could not allocate the buffer.
    case noMemory
    /// mismatchedDevices: a buffer of one device used with another's.
    case mismatchedDevices
    /// badCount: a buffer of a count the operation cannot take.
    case badCount
}

/// KernelError is what a launch can fail with.
public enum KernelError: Error {
    /// trapped: a work-item trapped, as an index out of a span does.
    case trapped
    /// failed: the device refused the launch, with the runtime's code.
    case failed(int32)
}

/// Device is one processor kernels run on: a GPU, or the CPU.
public final class Device {
    public let _h: UnsafeMutableRawPointer

    public init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    /// Name says which kind of device this is: "metal", "cuda", "hip" or "cpu".
    public var Name: string {
        let k = _deviceKind(_h)
        if k == 1 { return "metal" }
        if k == 2 { return "cuda" }
        if k == 3 { return "hip" }
        return "cpu"
    }

    /// IsCPU is whether this is the CPU device.
    public var IsCPU: bool { return _deviceKind(_h) == 0 }

    /// Memory is how many bytes the device has, as its driver says.
    public var Memory: int { return _deviceMemory(_h) }

    /// WaveSize is how wide this device's waves are.
    public var WaveSize: int { return _deviceWaveSize(_h) }

    /// CreateBuffer is a buffer of count elements, not yet filled.
    public func CreateBuffer<T>(of: T.Type, count: int) throws -> Buffer<T> {
        if count < 0 { throw DeviceError.badCount }
        let bytes = count * MemoryLayout<T>.stride
        guard let h = _bufferCreate(_h, bytes) else { throw DeviceError.noMemory }
        return Buffer<T>(_memory: _Memory(_h: h), device: self, offset: 0, count: count)
    }

    /// Upload is a new buffer holding host's elements.
    public func Upload<T>(_ host: [T]) async throws -> Buffer<T> {
        let b = try CreateBuffer(of: T.self, count: host.count)
        try await b.Upload(host)
        return b
    }
}

/// Default is the best device this machine has, and the CPU where it has
/// none. VERTEX_GPU=cpu or VERTEX_GPU=metal chooses one instead.
public func Default() -> Device {
    return Device(_h: _deviceDefault())
}

/// CPU is the CPU device, which every machine has.
public func CPU() -> Device {
    return Device(_h: _deviceCPU())
}

/// Devices is every device this machine has, the CPU last.
public func Devices() -> [Device] {
    var out: [Device] = []
    var i = 0
    let n = _deviceCount()
    while i < n {
        out.append(Device(_h: _deviceAt(i)))
        i += 1
    }
    return out
}

/// _Memory is a buffer's device memory, released when the last buffer
/// viewing it goes.
public final class _Memory {
    public let _h: UnsafeMutableRawPointer

    public init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    deinit {
        _bufferRelease(_h)
    }
}

/// Buffer is device memory holding count elements of T. It is a handle:
/// passing it around shares it, and the memory goes with the last one.
public final class Buffer<T> {
    public let _memory: _Memory
    public let _offset: int
    /// Device is the device the buffer's memory is on.
    public let Device: Device
    /// count is how many elements the buffer holds.
    public let count: int

    public init(_memory: _Memory, device: Device, offset: int, count: int) {
        self._memory = _memory
        self.Device = device
        self._offset = offset
        self.count = count
    }

    /// _elements is where the buffer's elements are, as the host sees them.
    public var _elements: UnsafeMutablePointer<T> {
        return UnsafeMutablePointer<T>(_bufferContents(_memory._h)) + _offset
    }

    /// Upload copies host's elements into the buffer, which must hold as many.
    public func Upload(_ host: [T]) async throws {
        if host.count != count { throw DeviceError.badCount }
        let p = _elements
        var i = 0
        while i < count {
            p[i] = host[i]
            i += 1
        }
    }

    /// Download is the buffer's elements, copied to the host.
    public func Download() async throws -> [T] {
        let p = _elements
        var out: [T] = []
        out.reserveCapacity(count)
        var i = 0
        while i < count {
            out.append(p[i])
            i += 1
        }
        return out
    }

    /// Fill sets every element to v.
    public func Fill(_ v: T) async throws {
        let p = _elements
        var i = 0
        while i < count {
            p[i] = v
            i += 1
        }
    }

    /// Copy copies another buffer's elements into this one.
    public func Copy(from other: Buffer<T>) async throws {
        if other.count != count { throw DeviceError.badCount }
        let p = _elements
        let q = other._elements
        var i = 0
        while i < count {
            p[i] = q[i]
            i += 1
        }
    }

    /// Slice is count elements of this buffer from `from`: a view, not a copy.
    public func Slice(from: int, count n: int) -> Buffer<T> {
        if from < 0 { fatalError("gpu.Buffer.Slice: a start before the buffer") }
        if n < 0 { fatalError("gpu.Buffer.Slice: a negative count") }
        if from + n > count { fatalError("gpu.Buffer.Slice: past the end of the buffer") }
        return Buffer<T>(_memory: _memory, device: Device, offset: _offset + from, count: n)
    }
}

/// _Launch is a launch the compiler builds for `k.Launch(...)`: the
/// kernel's descriptor, then each argument in the kernel's order, then
/// the grid, then the run. Nothing outside the compiler's own code calls it.
public final class _Launch {
    public let _h: UnsafeMutableRawPointer
    var gx = 1
    var gy = 1
    var gz = 1
    var wx = 0
    var wy = 0
    var wz = 0

    public init(_kernel: UnsafeRawPointer) {
        _h = _launchBegin(_kernel)
    }

    public func _buffer<T>(_ b: Buffer<T>) -> _Launch {
        _launchBuffer(_h, b._memory._h, b._offset * MemoryLayout<T>.stride)
        _launchI64(_h, b.count)
        return self
    }
    public func _int8(_ v: int8) -> _Launch { _launchI8(_h, v); return self }
    public func _uint8(_ v: uint8) -> _Launch { _launchI8(_h, int8(bitPattern: v)); return self }
    public func _int16(_ v: int16) -> _Launch { _launchI16(_h, v); return self }
    public func _uint16(_ v: uint16) -> _Launch { _launchI16(_h, int16(bitPattern: v)); return self }
    public func _int32(_ v: int32) -> _Launch { _launchI32(_h, v); return self }
    public func _uint32(_ v: uint32) -> _Launch { _launchI32(_h, int32(bitPattern: v)); return self }
    public func _int(_ v: int) -> _Launch { _launchI64(_h, v); return self }
    public func _uint(_ v: uint) -> _Launch { _launchI64(_h, int(bitPattern: v)); return self }
    public func _int64(_ v: int64) -> _Launch { _launchI64(_h, int(v)); return self }
    public func _uint64(_ v: uint64) -> _Launch { _launchI64(_h, int(bitPattern: uint(v))); return self }
    public func _float32(_ v: float32) -> _Launch { _launchF32(_h, v); return self }
    public func _float64(_ v: float64) -> _Launch { _launchF64(_h, v); return self }
    public func _bool(_ v: bool) -> _Launch { _launchI8(_h, v ? 1 : 0); return self }

    public func _over1(_ x: int) -> _Launch { gx = x; return self }
    public func _over2(_ g: (int, int)) -> _Launch { gx = g.0; gy = g.1; return self }
    public func _over3(_ g: (int, int, int)) -> _Launch { gx = g.0; gy = g.1; gz = g.2; return self }
    public func _group1(_ x: int) -> _Launch { wx = x; return self }
    public func _group2(_ g: (int, int)) -> _Launch { wx = g.0; wy = g.1; return self }
    public func _group3(_ g: (int, int, int)) -> _Launch { wx = g.0; wy = g.1; wz = g.2; return self }

    public func _run() async throws {
        if _mismatched { throw DeviceError.badCount }
        let code = _launchEnd(_h, gx, gy, gz, wx, wy, wz)
        if code == 1 { throw KernelError.trapped }
        if code != 0 { throw KernelError.failed(code) }
    }

    // ---- Map: an element kernel over buffers (§4.2) ----
    //
    // The compiler writes the grid kernel an element kernel stands for,
    // whose parameters are the output, then each argument: a mapped one
    // as a buffer, a broadcast one as itself. The grid is the mapped
    // buffers' count, which they must all have.

    var _count = -1
    var _mismatched = false
    var _outMemory: _Memory? = nil
    var _outDevice: Device? = nil

    public func _mapped<T>(_ b: Buffer<T>) -> _Launch {
        if _count < 0 {
            _count = b.count
            gx = b.count
            _outDevice = b.Device
        } else if _count != b.count {
            _mismatched = true
        }
        return _buffer(b)
    }

    public func _into<T>(_ b: Buffer<T>) -> _Launch {
        _count = b.count
        gx = b.count
        return _buffer(b)
    }

    // The output of a Map with no into:, made once the mapped buffers are
    // known -- after them, so a Map's output is the last parameter.
    func _makeOut(_ stride: int) throws -> _Memory {
        guard let d = _outDevice else { throw DeviceError.badCount }
        guard let h = _bufferCreate(d._h, _count * stride) else { throw DeviceError.noMemory }
        let m = _Memory(_h: h)
        _launchBuffer(_h, h, 0)
        _launchI64(_h, _count)
        _outMemory = m
        return m
    }

    public func _mapRun_int8() async throws -> Buffer<int8> { let m = try _makeOut(1); try await _run(); return Buffer<int8>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_uint8() async throws -> Buffer<uint8> { let m = try _makeOut(1); try await _run(); return Buffer<uint8>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_int16() async throws -> Buffer<int16> { let m = try _makeOut(2); try await _run(); return Buffer<int16>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_uint16() async throws -> Buffer<uint16> { let m = try _makeOut(2); try await _run(); return Buffer<uint16>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_int32() async throws -> Buffer<int32> { let m = try _makeOut(4); try await _run(); return Buffer<int32>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_uint32() async throws -> Buffer<uint32> { let m = try _makeOut(4); try await _run(); return Buffer<uint32>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_int() async throws -> Buffer<int> { let m = try _makeOut(8); try await _run(); return Buffer<int>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_uint() async throws -> Buffer<uint> { let m = try _makeOut(8); try await _run(); return Buffer<uint>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_float32() async throws -> Buffer<float32> { let m = try _makeOut(4); try await _run(); return Buffer<float32>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_float64() async throws -> Buffer<float64> { let m = try _makeOut(8); try await _run(); return Buffer<float64>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
    public func _mapRun_bool() async throws -> Buffer<bool> { let m = try _makeOut(1); try await _run(); return Buffer<bool>(_memory: m, device: _outDevice!, offset: 0, count: _count) }
}

// The descriptor of the kernel a launch names: the compiler answers each
// call for the kernel it was written for (vsc/lower, gpu.go).
@_silgen_name("vertex_gpu_kernel_descriptor") public func _kernelDescriptor() -> UnsafeRawPointer
