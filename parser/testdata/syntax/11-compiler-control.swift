// §5.6 Compiler Control Statements

#if os(macOS)
let platformName = "macOS"
#elseif os(iOS)
let platformName = "iOS"
#elseif arch(arm64) && swift(>=5.9)
let platformName = "arm64 Swift 5.9+"
#else
let platformName = "unknown"
#endif

#if canImport(UIKit)
// import UIKit
#endif

#if targetEnvironment(simulator)
let isSimulator = true
#else
let isSimulator = false
#endif

#if compiler(>=5.9)
let modernCompiler = true
#endif

#sourceLocation(file: "Virtual.swift", line: 100)
let virtualLine = #line
#sourceLocation()

// DiagnosticStatement
#if false
#warning("This branch is disabled")
#error("This should never compile")
#endif