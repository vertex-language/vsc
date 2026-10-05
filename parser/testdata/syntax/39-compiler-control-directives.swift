// §5.6 Deep Compiler Control Directives

#if os(macOS) || os(iOS)
    #if arch(arm64)
        let architecture = "Apple Silicon"
    #elseif arch(x86_64)
        let architecture = "Intel"
    #else
        let architecture = "Other"
    #endif
#else
    let architecture = "Non-Apple"
#endif

#if hasAttribute(propertyWrapper)
let supportsPropertyWrappers = true
#else
let supportsPropertyWrappers = false
#endif

#if swift(>=5.9) && compiler(>=5.9)
let isModern = true
#endif

#sourceLocation(file: "SourceGen.swift", line: 500)
let generatedLine = #line
let generatedFile = #file
#sourceLocation()

#if false
#warning("This warning is in dead code")
#error("This error is in dead code")
#endif
