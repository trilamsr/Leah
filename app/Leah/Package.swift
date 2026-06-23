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
      path: "Tests/LeahAppTests",
      resources: [.copy("Fixtures")]
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
    .target(
      name: "LeahAudio",
      path: "Sources/LeahAudio"
    ),
    .testTarget(
      name: "LeahAudioTests",
      dependencies: ["LeahAudio"],
      path: "Tests/LeahAudioTests"
    ),
    .target(
      name: "LeahWake",
      path: "Sources/LeahWake",
      resources: [.copy("Models/wake-leah.mlmodel")]
    ),
    .testTarget(
      name: "LeahWakeTests",
      dependencies: ["LeahWake"],
      path: "Tests/LeahWakeTests"
    ),
    .target(
      name: "LeahVoice",
      dependencies: ["LeahIPC"],
      path: "Sources/LeahVoice"
    ),
    .testTarget(
      name: "LeahVoiceTests",
      dependencies: ["LeahVoice", "LeahIPC"],
      path: "Tests/LeahVoiceTests"
    ),
    .target(
      name: "LeahVision",
      dependencies: ["LeahIPC"],
      path: "Sources/LeahVision"
    ),
    .testTarget(
      name: "LeahVisionTests",
      dependencies: ["LeahVision", "LeahIPC"],
      path: "Tests/LeahVisionTests"
    ),
  ]
)
