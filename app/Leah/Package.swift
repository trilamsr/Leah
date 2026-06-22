// swift-tools-version:5.9
import PackageDescription

let package = Package(
  name: "Leah",
  platforms: [.macOS(.v14)],
  products: [
    .executable(name: "Leah", targets: ["LeahApp"]),
  ],
  dependencies: [
    .package(url: "https://github.com/sparkle-project/Sparkle", from: "2.6.0"),
  ],
  targets: [
    .executableTarget(
      name: "LeahApp",
      dependencies: ["LeahUI", "LeahIPC"],
      path: "Sources/LeahApp",
      resources: [.process("Leah.entitlements")]
    ),
    .testTarget(
      name: "LeahAppTests",
      dependencies: ["LeahApp"],
      path: "Tests/LeahAppTests"
    ),
    .target(
      name: "LeahAuth",
      path: "Sources/LeahAuth"
    ),
    .testTarget(
      name: "LeahAuthTests",
      dependencies: ["LeahAuth"],
      path: "Tests/LeahAuthTests"
    ),
    .target(
      name: "LeahUI",
      dependencies: ["LeahIPC", "LeahAuth"],
      path: "Sources/LeahUI"
    ),
    .testTarget(
      name: "LeahUITests",
      dependencies: ["LeahUI", "LeahAuth", "LeahIPC"],
      path: "Tests/LeahUITests"
    ),
    .target(
      name: "LeahWidgets",
      path: "Sources/LeahWidgets"
    ),
    .testTarget(
      name: "LeahWidgetsTests",
      dependencies: ["LeahWidgets"],
      path: "Tests/LeahWidgetsTests"
    ),
    .target(
      name: "LeahUpdate",
      dependencies: [.product(name: "Sparkle", package: "Sparkle")],
      path: "Sources/LeahUpdate"
    ),
    .testTarget(
      name: "LeahUpdateTests",
      dependencies: ["LeahUpdate"],
      path: "Tests/LeahUpdateTests"
    ),
    .target(
      name: "LeahIPC",
      path: "Sources/LeahIPC"
    ),
    .testTarget(
      name: "LeahIPCTests",
      dependencies: ["LeahIPC"],
      path: "Tests/LeahIPCTests"
    ),
    .testTarget(
      name: "LeahSettingsTests",
      dependencies: ["LeahUI"],
      path: "Tests/LeahSettingsTests"
    ),
  ]
)
