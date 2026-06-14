#!/usr/bin/env bash
# build_android_deps.sh — cross-compile FreeType + HarfBuzz for
# Android arm64-v8a using the NDK toolchain.
#
# Usage: ANDROID_NDK_HOME=/path/to/ndk ./scripts/build_android_deps.sh
#
# Output: deps/lib/arm64-v8a/{libfreetype.a,libharfbuzz.a}
#         deps/include/{ft2build.h,freetype/,hb.h,...}

set -euo pipefail

FREETYPE_VER="2.13.3"
HARFBUZZ_VER="10.2.0"
API_LEVEL="${ANDROID_API_LEVEL:-21}"
ARCH=aarch64
ABI=arm64-v8a

: "${ANDROID_NDK_HOME:?Set ANDROID_NDK_HOME to your NDK path}"

case "$(uname -s)" in
    Darwin) NDK_HOST=darwin-x86_64; NJOBS=$(sysctl -n hw.ncpu) ;;
    *)      NDK_HOST=linux-x86_64;  NJOBS=$(nproc)             ;;
esac

TOOLCHAIN="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$NDK_HOST"
CC="$TOOLCHAIN/bin/${ARCH}-linux-android${API_LEVEL}-clang"
CXX="$TOOLCHAIN/bin/${ARCH}-linux-android${API_LEVEL}-clang++"
AR="$TOOLCHAIN/bin/llvm-ar"
RANLIB="$TOOLCHAIN/bin/llvm-ranlib"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$ROOT_DIR/.build_android_deps"
PREFIX="$ROOT_DIR/deps"

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR" "$PREFIX/lib/$ABI" "$PREFIX/include"

echo "=== Downloading FreeType $FREETYPE_VER ==="
FREETYPE_TAR="$BUILD_DIR/freetype-$FREETYPE_VER.tar.xz"
FREETYPE_URLS=(
    "https://downloads.sourceforge.net/project/freetype/freetype2/$FREETYPE_VER/freetype-$FREETYPE_VER.tar.xz"
    "https://download.savannah.gnu.org/releases/freetype/freetype-$FREETYPE_VER.tar.xz"
)
for url in "${FREETYPE_URLS[@]}"; do
    echo "  Trying $url"
    if curl -sfL "$url" -o "$FREETYPE_TAR"; then
        if file "$FREETYPE_TAR" | grep -q 'XZ compressed'; then
            break
        fi
        echo "  WARNING: downloaded file is not XZ, retrying..."
    fi
done
if ! file "$FREETYPE_TAR" | grep -q 'XZ compressed'; then
    echo "FATAL: failed to download valid FreeType tarball" >&2
    exit 1
fi
tar xf "$FREETYPE_TAR" -C "$BUILD_DIR"

echo "=== Building FreeType ==="
cd "$BUILD_DIR/freetype-$FREETYPE_VER"
./configure \
    --host="${ARCH}-linux-android" \
    --prefix="$PREFIX" \
    --enable-static --disable-shared \
    --with-zlib=no --with-bzip2=no --with-png=no --with-brotli=no \
    CC="$CC" CXX="$CXX" AR="$AR" RANLIB="$RANLIB" \
    CFLAGS="-O2 -fPIC"
make -j"$NJOBS" install
cp "$PREFIX/lib/libfreetype.a" "$PREFIX/lib/$ABI/"

echo "=== Downloading HarfBuzz $HARFBUZZ_VER ==="
HARFBUZZ_TAR="$BUILD_DIR/harfbuzz-$HARFBUZZ_VER.tar.xz"
curl -sL "https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VER/harfbuzz-$HARFBUZZ_VER.tar.xz" \
    -o "$HARFBUZZ_TAR"
tar xf "$HARFBUZZ_TAR" -C "$BUILD_DIR"

echo "=== Building HarfBuzz ==="
cd "$BUILD_DIR/harfbuzz-$HARFBUZZ_VER"

# HarfBuzz uses meson; fall back to cmake if available.
if command -v meson &>/dev/null; then
    meson setup build \
        --cross-file /dev/stdin <<CROSSEOF
[binaries]
c = '$CC'
cpp = '$CXX'
ar = '$AR'
ranlib = '$RANLIB'

[host_machine]
system = 'android'
cpu_family = 'aarch64'
cpu = 'aarch64'
endian = 'little'

[properties]
c_args = ['-O2', '-fPIC']
cpp_args = ['-O2', '-fPIC']
CROSSEOF
    meson setup build \
        --prefix="$PREFIX" \
        --default-library=static \
        -Dfreetype=enabled \
        -Dglib=disabled \
        -Dcairo=disabled \
        -Dtests=disabled \
        -Ddocs=disabled
    ninja -C build install
elif command -v cmake &>/dev/null; then
    mkdir -p build && cd build
    cmake .. \
        -DCMAKE_SYSTEM_NAME=Android \
        -DCMAKE_ANDROID_NDK="$ANDROID_NDK_HOME" \
        -DCMAKE_ANDROID_ARCH_ABI="$ABI" \
        -DCMAKE_ANDROID_API="$API_LEVEL" \
        -DCMAKE_INSTALL_PREFIX="$PREFIX" \
        -DCMAKE_FIND_ROOT_PATH="$PREFIX" \
        -DFREETYPE_LIBRARY="$PREFIX/lib/libfreetype.a" \
        -DFREETYPE_INCLUDE_DIRS="$PREFIX/include/freetype2" \
        -DBUILD_SHARED_LIBS=OFF \
        -DHB_HAVE_FREETYPE=ON \
        -DHB_HAVE_GLIB=OFF \
        -DHB_HAVE_CAIRO=OFF \
        -DHB_HAVE_GOBJECT=OFF \
        -DHB_HAVE_ICU=OFF \
        -DHB_HAVE_GRAPHITE2=OFF \
        -DHB_HAVE_WASM=OFF
    make -j"$NJOBS" install
else
    echo "ERROR: meson or cmake required to build HarfBuzz" >&2
    exit 1
fi

cp "$PREFIX/lib/libharfbuzz.a" "$PREFIX/lib/$ABI/" 2>/dev/null || true

echo "=== Cleaning up ==="
rm -rf "$BUILD_DIR"

echo "=== Done ==="
echo "Static libs: $PREFIX/lib/$ABI/"
echo "Headers:     $PREFIX/include/"
ls -la "$PREFIX/lib/$ABI/"
