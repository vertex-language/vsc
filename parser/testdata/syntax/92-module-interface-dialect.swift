// The spellings a .swiftinterface is written with: access levels on
// imports, underscored modifiers, and the full set of compilation
// conditions.

@_exported public import Swift
internal import Darwin
public import struct Foundation.Data
@_implementationOnly import Dispatch

@frozen public struct Slot {
    // A keyword may name a parameter.
    public init(_ default: Int, a case: String, _ let: Bool) {}

    public func at(_ index: _const Int) -> Int { 0 }

    public var value: Int {
        get { 0 }
        set {}
    }

    public var observed: Int = 0 {
        willSet(incoming) { _ = incoming }
        didSet { _ = oldValue }
    }
}

public protocol Emitter {}

public struct Node {}

extension Node: nonisolated Emitter {}

@available(macOS 15.0, *)
public struct Later: @unchecked Sendable, nonisolated Emitter {}

distributed actor Worker {
    distributed func run() {}
}

public func closures() {
    let inoutTaker = { (state: inout (Int, Bool)) -> Int in state.0 }
    let borrower = { (s: borrowing String) -> Int in s.count }
    let consumer = { (s: consuming String) in _ = s }
    _ = inoutTaker
    _ = borrower
    _ = consumer
}

#if canImport(Cxx, _version: 6.2.0.9)
public let cxx = true
#endif

#if _compiler_version("5.9")
public let versioned = true
#endif

#if hasFeature(RegionBasedIsolation) && _runtime(_ObjC) && _endian(little)
public let features = true
#elseif _pointerBitWidth(_64) && _hasAtomicBitWidth(_64) && _ptrauth(_arm64e)
public let widths = true
#endif

#if !(os(Windows) || os(Android) || ($Embedded && !os(Linux)))
public let hosted = true
#endif

public enum Rule { case up, down }

public func rounded(_ r: Rule) -> Int {
    switch r {
    case .up: return 1
#if !$Embedded
    @unknown default: return 0
#endif
    }
}

public func packs<each T>(_ values: repeat each T) {
    var sink = [Any]()
    repeat sink.append(each values)
    _ = sink
}
