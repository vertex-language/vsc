package gpu

// The device half of the built-in gpu module: what a kernel can ask of
// the hardware. See proposed_vertex_kernel.md §6.
//
// Everything here bottoms out in an intrinsic, a function named
// vertex_gpu_* with no body. On a GPU the compiler lowers each one to
// the VIR verb it names (vsc/lower, device mode). On the CPU device they
// are the gpu runtime unit's functions (stdlib/runtime/gpu), which read
// the work-item the runtime is running. A kernel never sees the
// difference.

// ---- intrinsics ----

@_silgen_name("vertex_gpu_workitem_id_x") public func _workitemIDX() -> int32
@_silgen_name("vertex_gpu_workitem_id_y") public func _workitemIDY() -> int32
@_silgen_name("vertex_gpu_workitem_id_z") public func _workitemIDZ() -> int32
@_silgen_name("vertex_gpu_workgroup_id_x") public func _workgroupIDX() -> int32
@_silgen_name("vertex_gpu_workgroup_id_y") public func _workgroupIDY() -> int32
@_silgen_name("vertex_gpu_workgroup_id_z") public func _workgroupIDZ() -> int32
@_silgen_name("vertex_gpu_workgroup_size_x") public func _workgroupSizeX() -> int32
@_silgen_name("vertex_gpu_workgroup_size_y") public func _workgroupSizeY() -> int32
@_silgen_name("vertex_gpu_workgroup_size_z") public func _workgroupSizeZ() -> int32
@_silgen_name("vertex_gpu_num_workgroups_x") public func _numWorkgroupsX() -> int32
@_silgen_name("vertex_gpu_num_workgroups_y") public func _numWorkgroupsY() -> int32
@_silgen_name("vertex_gpu_num_workgroups_z") public func _numWorkgroupsZ() -> int32
@_silgen_name("vertex_gpu_lane_id") public func _laneID() -> int32
@_silgen_name("vertex_gpu_wave_size") public func _waveSize() -> int32
@_silgen_name("vertex_gpu_barrier") public func _barrier()
@_silgen_name("vertex_gpu_trap") public func _trap() -> Never

@_silgen_name("vertex_gpu_shfl_idx") public func _shflIdx(_ v: uint32, _ lane: int32) -> uint32
@_silgen_name("vertex_gpu_shfl_up") public func _shflUp(_ v: uint32, _ delta: int32) -> uint32
@_silgen_name("vertex_gpu_shfl_down") public func _shflDown(_ v: uint32, _ delta: int32) -> uint32
@_silgen_name("vertex_gpu_shfl_xor") public func _shflXor(_ v: uint32, _ mask: int32) -> uint32
@_silgen_name("vertex_gpu_readfirstlane") public func _readFirstLane(_ v: uint32) -> uint32
@_silgen_name("vertex_gpu_ballot") public func _ballot(_ c: bool) -> uint64
@_silgen_name("vertex_gpu_wave_any") public func _waveAny(_ c: bool) -> bool
@_silgen_name("vertex_gpu_wave_all") public func _waveAll(_ c: bool) -> bool

// Workgroup storage: bytes, the same for every work-item of a group.
// The size must come to a constant once the kernel is inlined.
@_silgen_name("vertex_gpu_shared") public func _shared(_ bytes: int, _ align: int) -> UnsafeMutableRawPointer

@_silgen_name("vertex_gpu_atomic_add_i32") public func _atomicAddI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_sub_i32") public func _atomicSubI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_min_i32") public func _atomicMinI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_max_i32") public func _atomicMaxI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_and_i32") public func _atomicAndI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_or_i32") public func _atomicOrI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_xor_i32") public func _atomicXorI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_xchg_i32") public func _atomicXchgI32(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32
@_silgen_name("vertex_gpu_atomic_cas_i32") public func _atomicCasI32(_ p: UnsafeMutablePointer<int32>, _ expected: int32, _ new: int32) -> int32
@_silgen_name("vertex_gpu_atomic_add_u32") public func _atomicAddU32(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32
@_silgen_name("vertex_gpu_atomic_min_u32") public func _atomicMinU32(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32
@_silgen_name("vertex_gpu_atomic_max_u32") public func _atomicMaxU32(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32
@_silgen_name("vertex_gpu_atomic_add_f32") public func _atomicAddF32(_ p: UnsafeMutablePointer<float32>, _ v: float32) -> float32

// ---- where am I (§6.1) ----

/// Index is this work-item in the whole grid.
public enum Index {
    public static var x: int { return int(_workgroupIDX()) * int(_workgroupSizeX()) + int(_workitemIDX()) }
    public static var y: int { return int(_workgroupIDY()) * int(_workgroupSizeY()) + int(_workitemIDY()) }
    public static var z: int { return int(_workgroupIDZ()) * int(_workgroupSizeZ()) + int(_workitemIDZ()) }
}

/// LocalIndex is this work-item within its workgroup.
public enum LocalIndex {
    public static var x: int { return int(_workitemIDX()) }
    public static var y: int { return int(_workitemIDY()) }
    public static var z: int { return int(_workitemIDZ()) }
}

/// GroupIndex is this workgroup within the grid.
public enum GroupIndex {
    public static var x: int { return int(_workgroupIDX()) }
    public static var y: int { return int(_workgroupIDY()) }
    public static var z: int { return int(_workgroupIDZ()) }
}

/// GroupSize is how many work-items a workgroup has.
public enum GroupSize {
    public static var x: int { return int(_workgroupSizeX()) }
    public static var y: int { return int(_workgroupSizeY()) }
    public static var z: int { return int(_workgroupSizeZ()) }
}

/// GroupCount is how many workgroups the grid has.
public enum GroupCount {
    public static var x: int { return int(_numWorkgroupsX()) }
    public static var y: int { return int(_numWorkgroupsY()) }
    public static var z: int { return int(_numWorkgroupsZ()) }
}

// ---- working together (§6.2) ----

/// Barrier waits for every work-item of the workgroup, and makes what
/// each wrote before it visible to all of them after it.
public func Barrier() {
    _barrier()
}

/// Wave is the work-items that run in lock-step: a warp, a SIMD-group.
public enum Wave {
    /// Lane is this work-item's place in its wave.
    public static var Lane: int { return int(_laneID()) }
    /// Size is how wide a wave is: 32 or 64 on a GPU, 1 on the CPU.
    public static var Size: int { return int(_waveSize()) }

    public static func Shuffle(_ v: int32, from lane: int) -> int32 {
        return int32(bitPattern: _shflIdx(uint32(bitPattern: v), int32(lane)))
    }
    public static func Shuffle(_ v: uint32, from lane: int) -> uint32 {
        return _shflIdx(v, int32(lane))
    }
    public static func Shuffle(_ v: float32, from lane: int) -> float32 {
        return float32(bitPattern: _shflIdx(v.bitPattern, int32(lane)))
    }
    public static func ShuffleDown(_ v: int32, by delta: int) -> int32 {
        return int32(bitPattern: _shflDown(uint32(bitPattern: v), int32(delta)))
    }
    public static func ShuffleDown(_ v: uint32, by delta: int) -> uint32 {
        return _shflDown(v, int32(delta))
    }
    public static func ShuffleDown(_ v: float32, by delta: int) -> float32 {
        return float32(bitPattern: _shflDown(v.bitPattern, int32(delta)))
    }
    public static func ShuffleUp(_ v: int32, by delta: int) -> int32 {
        return int32(bitPattern: _shflUp(uint32(bitPattern: v), int32(delta)))
    }
    public static func ShuffleUp(_ v: float32, by delta: int) -> float32 {
        return float32(bitPattern: _shflUp(v.bitPattern, int32(delta)))
    }
    public static func ShuffleXor(_ v: int32, mask: int) -> int32 {
        return int32(bitPattern: _shflXor(uint32(bitPattern: v), int32(mask)))
    }
    public static func ShuffleXor(_ v: uint32, mask: int) -> uint32 {
        return _shflXor(v, int32(mask))
    }
    public static func ShuffleXor(_ v: float32, mask: int) -> float32 {
        return float32(bitPattern: _shflXor(v.bitPattern, int32(mask)))
    }

    /// Any is whether c holds in any lane of the wave.
    public static func `Any`(_ c: bool) -> bool { return _waveAny(c) }
    /// All is whether c holds in every lane of the wave.
    public static func All(_ c: bool) -> bool { return _waveAll(c) }
    /// Ballot is one bit per lane: set where c holds.
    public static func Ballot(_ c: bool) -> uint64 { return _ballot(c) }
    /// First is the value the wave's first active lane has.
    public static func First(_ v: int32) -> int32 {
        return int32(bitPattern: _readFirstLane(uint32(bitPattern: v)))
    }
    public static func First(_ v: float32) -> float32 {
        return float32(bitPattern: _readFirstLane(v.bitPattern))
    }

    /// Sum is v added across the wave, in every lane.
    public static func Sum(_ v: float32) -> float32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            s = s + ShuffleXor(s, mask: m)
            m = m / 2
        }
        return s
    }
    public static func Sum(_ v: int32) -> int32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            s = s &+ ShuffleXor(s, mask: m)
            m = m / 2
        }
        return s
    }
    /// Min is the least v across the wave, in every lane.
    public static func Min(_ v: float32) -> float32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            let o = ShuffleXor(s, mask: m)
            if o < s { s = o }
            m = m / 2
        }
        return s
    }
    public static func Min(_ v: int32) -> int32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            let o = ShuffleXor(s, mask: m)
            if o < s { s = o }
            m = m / 2
        }
        return s
    }
    /// Max is the greatest v across the wave, in every lane.
    public static func Max(_ v: float32) -> float32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            let o = ShuffleXor(s, mask: m)
            if o > s { s = o }
            m = m / 2
        }
        return s
    }
    public static func Max(_ v: int32) -> int32 {
        var s = v
        var m = Size / 2
        while m > 0 {
            let o = ShuffleXor(s, mask: m)
            if o > s { s = o }
            m = m / 2
        }
        return s
    }
}

/// Atomic is read-modify-write on an element of a span or of shared
/// storage, returning the value it held before. Device scope.
public enum Atomic {
    public static func Add(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicAddI32(p, v) }
    public static func Add(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32 { return _atomicAddU32(p, v) }
    public static func Add(_ p: UnsafeMutablePointer<float32>, _ v: float32) -> float32 { return _atomicAddF32(p, v) }
    public static func Sub(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicSubI32(p, v) }
    public static func Min(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicMinI32(p, v) }
    public static func Min(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32 { return _atomicMinU32(p, v) }
    public static func Max(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicMaxI32(p, v) }
    public static func Max(_ p: UnsafeMutablePointer<uint32>, _ v: uint32) -> uint32 { return _atomicMaxU32(p, v) }
    public static func And(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicAndI32(p, v) }
    public static func Or(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicOrI32(p, v) }
    public static func Xor(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicXorI32(p, v) }
    public static func Exchange(_ p: UnsafeMutablePointer<int32>, _ v: int32) -> int32 { return _atomicXchgI32(p, v) }
    public static func CompareExchange(_ p: UnsafeMutablePointer<int32>, expected: int32, desired: int32) -> int32 {
        return _atomicCasI32(p, expected, desired)
    }
}

// ---- memory views (§6.3) ----

/// Span is a read-only view of device memory: count elements from a base.
public struct Span<T> {
    public let _base: UnsafeMutablePointer<T>
    public let count: int

    public init(_base: UnsafeMutablePointer<T>, count: int) {
        self._base = _base
        self.count = count
    }

    /// The element at i; an index out of the span traps.
    public subscript(i: int) -> T {
        if i < 0 { _trap() }
        if i >= count { _trap() }
        return _base[i]
    }

    /// Unchecked is the element at i with no bounds check.
    public func Unchecked(_ i: int) -> T {
        return _base[i]
    }

    /// Slice is count elements from `from`, as a span of their own.
    public func Slice(from: int, count n: int) -> Span<T> {
        if from < 0 { _trap() }
        if n < 0 { _trap() }
        if from + n > count { _trap() }
        return Span<T>(_base: _base + from, count: n)
    }
}

/// MutableSpan is a writable view of device memory.
public struct MutableSpan<T> {
    public let _base: UnsafeMutablePointer<T>
    public let count: int

    public init(_base: UnsafeMutablePointer<T>, count: int) {
        self._base = _base
        self.count = count
    }

    /// The element at i; an index out of the span traps.
    public subscript(i: int) -> T {
        get {
            if i < 0 { _trap() }
            if i >= count { _trap() }
            return _base[i]
        }
        nonmutating set {
            if i < 0 { _trap() }
            if i >= count { _trap() }
            _base[i] = newValue
        }
    }

    /// Address is where the element at i is, for an atomic on it.
    public func Address(_ i: int) -> UnsafeMutablePointer<T> {
        if i < 0 { _trap() }
        if i >= count { _trap() }
        return _base + i
    }

    public func Unchecked(_ i: int) -> T {
        return _base[i]
    }

    public func SetUnchecked(_ i: int, _ v: T) {
        _base[i] = v
    }

    public func Slice(from: int, count n: int) -> MutableSpan<T> {
        if from < 0 { _trap() }
        if n < 0 { _trap() }
        if from + n > count { _trap() }
        return MutableSpan<T>(_base: _base + from, count: n)
    }
}

/// Shared is workgroup storage: one allocation per workgroup, whose
/// count must be a constant, indexed like a span.
public struct Shared<T> {
    public let _base: UnsafeMutablePointer<T>
    public let count: int

    public init(count: int) {
        self.count = count
        self._base = UnsafeMutablePointer<T>(_shared(count * MemoryLayout<T>.stride, MemoryLayout<T>.alignment))
    }

    public subscript(i: int) -> T {
        get {
            if i < 0 { _trap() }
            if i >= count { _trap() }
            return _base[i]
        }
        nonmutating set {
            if i < 0 { _trap() }
            if i >= count { _trap() }
            _base[i] = newValue
        }
    }

    public func Address(_ i: int) -> UnsafeMutablePointer<T> {
        if i < 0 { _trap() }
        if i >= count { _trap() }
        return _base + i
    }
}
