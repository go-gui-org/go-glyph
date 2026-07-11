# showcase_android

Android showcase app for Go-Glyph. Compiles the library as a C-shared `.so`
and loads it from a minimal Kotlin activity via JNI.

## Prerequisites

- **Go 1.26+**
- **Android NDK** — install via Android Studio SDK Manager or `brew install android-ndk`
- **Android SDK** — platform 35, build-tools (`~/Library/Android/sdk` or set `ANDROID_HOME`)
- **JDK 21** — Gradle requires JDK 21; JDK 25 is not yet supported
- **Gradle** — for initial wrapper generation (`brew install gradle`)
- **cmake or meson** — for building HarfBuzz
- **adb** — for deploying to a device

Install JDK 21 if needed:

```sh
brew install openjdk@21
```

Set the NDK path:

```sh
export ANDROID_NDK_HOME=/opt/homebrew/share/android-ndk  # adjust to your path
```

The go-glyph library itself is pure Go and links no native text libraries —
no FreeType/HarfBuzz cross-compilation step is needed. The NDK is required
only for this example app's own JNI/GLES shell.

## Setup

Create `android/local.properties` with the SDK path:

```sh
echo "sdk.dir=$HOME/Library/Android/sdk" > android/local.properties
```

Generate the Gradle wrapper (one-time):

```sh
make setup
```

## Build & Run

```sh
make build      # cross-compile libglyph.so
make apk        # build + assemble debug APK
make install    # build + install APK to connected device
make run        # install + launch GlyphActivity
make clean      # remove .so and Gradle build artifacts
```

## Device requirements

The `.so` is built for **arm64-v8a** only. Use a physical ARM64 device
connected via USB/adb. Standard x86_64 emulators will not load the library.

## Project structure

```
showcase_android/
├── main.go                     # Go entry point (c-shared exports)
├── go.mod
├── Makefile
└── android/
    ├── build.gradle.kts        # root Gradle config
    ├── settings.gradle.kts
    ├── gradle.properties       # JDK 21 path, JVM args
    ├── local.properties        # Android SDK path (not committed)
    └── app/
        ├── build.gradle.kts
        └── src/main/
            ├── AndroidManifest.xml
            ├── java/.../GlyphActivity.kt
            ├── java/.../GlyphNative.kt
            └── jniLibs/arm64-v8a/libglyph.so  (generated)
```
