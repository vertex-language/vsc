// swift-tools-version: 6.0
import PackageDescription

// Swift over Objective-C++ that links frameworks by name.
//
// The Objective-C++ target is written for ARC, the way SwiftPM compiles it:
// a strong static holds an autoreleased collection past the pool it came
// from, which without ARC is a dangling reference. And it links Foundation
// and CoreGraphics through linkedFramework rather than by accident.
let package = Package(
    name: "ObjCFrameworks",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "registry", targets: ["App"]),
    ],
    targets: [
        .target(
            name: "Registry",
            linkerSettings: [
                .linkedFramework("Foundation"),
                .linkedFramework("CoreGraphics"),
            ]
        ),
        .executableTarget(name: "App", dependencies: ["Registry"]),
    ]
)
