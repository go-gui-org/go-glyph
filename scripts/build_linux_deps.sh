#!/usr/bin/env bash
# build_linux_deps.sh — build FreeType + HarfBuzz static libs for Linux.
#
# Usage: ./scripts/build_linux_deps.sh
#
# Fast path: if system static libs are already installed (libfreetype-dev +
# libharfbuzz-dev), copies them and their headers into deps/ in seconds.
#
# Slow path (fallback): downloads source tarballs and compiles from scratch.
# Use this for CI, containers, or when version pinning is required.
#
# Output: deps/lib/linux_amd64/{libfreetype.a,libharfbuzz.a}  (on x86_64)
#         deps/lib/linux_arm64/{libfreetype.a,libharfbuzz.a}  (on aarch64)
#         deps/include/{ft2build.h,freetype/,hb.h,...}        (shared)
#
# Fast-path prerequisites (Debian/Ubuntu):
#   apt install libfreetype-dev libharfbuzz-dev pkg-config
# Slow-path prerequisites:
#   apt install build-essential cmake ninja-build pkg-config zlib1g-dev

set -euo pipefail

FREETYPE_VER="2.13.3"
HARFBUZZ_VER="10.2.0"

NJOBS=$(nproc)

# Resolve output arch dir from uname -m.
case "$(uname -m)" in
    x86_64)  ARCH_DIR=linux_amd64 ;;
    aarch64) ARCH_DIR=linux_arm64 ;;
    *)
        echo "ERROR: unsupported arch $(uname -m)" >&2
        exit 1
        ;;
esac

# Prefer clang if available; fall back to gcc.
if command -v clang &>/dev/null; then
    CC=clang
    CXX=clang++
else
    CC=gcc
    CXX=g++
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$ROOT_DIR/.build_linux_deps"
PREFIX="$ROOT_DIR/deps"
LIB_OUT="$PREFIX/lib/$ARCH_DIR"

mkdir -p "$LIB_OUT" "$PREFIX/include"

# ── Fast path ────────────────────────────────────────────────────────────────
# Use system static libs if pkg-config can find both. Copies the .a files and
# headers into deps/ so the CGo CFLAGS/-L paths resolve without change.
use_system_libs() {
    command -v pkg-config &>/dev/null || return 1
    pkg-config --exists freetype2 harfbuzz 2>/dev/null || return 1

    FT_LIBDIR=$(pkg-config --variable=libdir freetype2)
    HB_LIBDIR=$(pkg-config --variable=libdir harfbuzz)

    [ -f "$FT_LIBDIR/libfreetype.a" ] || return 1
    [ -f "$HB_LIBDIR/libharfbuzz.a" ] || return 1

    echo "=== Fast path: copying system static libs ==="
    cp "$FT_LIBDIR/libfreetype.a" "$LIB_OUT/"
    cp "$HB_LIBDIR/libharfbuzz.a" "$LIB_OUT/"

    # Copy FreeType headers.
    FT_INCDIR=$(pkg-config --variable=includedir freetype2)
    cp -r "$FT_INCDIR/freetype2" "$PREFIX/include/" 2>/dev/null || true
    # ft2build.h may sit one level up from the freetype2/ subdir.
    [ -f "$FT_INCDIR/ft2build.h" ] && cp "$FT_INCDIR/ft2build.h" "$PREFIX/include/"
    [ -f "$FT_INCDIR/freetype2/ft2build.h" ] && \
        cp "$FT_INCDIR/freetype2/ft2build.h" "$PREFIX/include/"

    # Copy HarfBuzz headers.
    HB_INCDIR=$(pkg-config --variable=includedir harfbuzz)
    mkdir -p "$PREFIX/include/harfbuzz"
    cp "$HB_INCDIR/harfbuzz"/hb*.h "$PREFIX/include/harfbuzz/" 2>/dev/null || \
        cp "$HB_INCDIR"/hb*.h "$PREFIX/include/harfbuzz/" 2>/dev/null || true

    echo "=== Done (system libs) ==="
    echo "Static libs: $LIB_OUT/"
    ls -la "$LIB_OUT/"
    return 0
}

if use_system_libs; then
    exit 0
fi
echo "System static libs not found — building from source..."

# ── Slow path ─────────────────────────────────────────────────────────────────
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

echo "=== Building FreeType $FREETYPE_VER for $ARCH_DIR ==="
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

cd "$BUILD_DIR/freetype-$FREETYPE_VER"
./configure \
    --prefix="$PREFIX" \
    --enable-static --disable-shared \
    --with-zlib=no --with-bzip2=no --with-png=no --with-brotli=no \
    CC="$CC" CXX="$CXX" \
    CFLAGS="-O2 -fPIC"
make -j"$NJOBS" install
cp "$PREFIX/lib/libfreetype.a" "$LIB_OUT/"

echo "=== Building HarfBuzz $HARFBUZZ_VER for $ARCH_DIR ==="
HARFBUZZ_TAR="$BUILD_DIR/harfbuzz-$HARFBUZZ_VER.tar.xz"
curl -sL "https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VER/harfbuzz-$HARFBUZZ_VER.tar.xz" \
    -o "$HARFBUZZ_TAR"
tar xf "$HARFBUZZ_TAR" -C "$BUILD_DIR"

cd "$BUILD_DIR/harfbuzz-$HARFBUZZ_VER"

if command -v meson &>/dev/null && command -v ninja &>/dev/null; then
    meson setup build \
        --prefix="$PREFIX" \
        --default-library=static \
        --buildtype=release \
        -Dfreetype=enabled \
        -Dglib=disabled \
        -Dcairo=disabled \
        -Dtests=disabled \
        -Ddocs=disabled \
        -Dpkg_config_path="$PREFIX/lib/pkgconfig"
    ninja -C build install
elif command -v cmake &>/dev/null; then
    mkdir -p build && cd build
    cmake .. \
        -DCMAKE_INSTALL_PREFIX="$PREFIX" \
        -DCMAKE_FIND_ROOT_PATH="$PREFIX" \
        -DCMAKE_C_COMPILER="$CC" \
        -DCMAKE_CXX_COMPILER="$CXX" \
        -DBUILD_SHARED_LIBS=OFF \
        -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
        -DHB_HAVE_FREETYPE=ON \
        -DFREETYPE_LIBRARY="$PREFIX/lib/libfreetype.a" \
        -DFREETYPE_INCLUDE_DIRS="$PREFIX/include/freetype2" \
        -DHB_HAVE_GLIB=OFF \
        -DHB_HAVE_CAIRO=OFF \
        -DHB_HAVE_GOBJECT=OFF \
        -DHB_HAVE_ICU=OFF \
        -DHB_HAVE_GRAPHITE2=OFF \
        -DHB_HAVE_WASM=OFF
    make -j"$NJOBS" install
else
    echo "ERROR: meson+ninja or cmake required to build HarfBuzz" >&2
    exit 1
fi

cp "$PREFIX/lib/libharfbuzz.a" "$LIB_OUT/" 2>/dev/null || \
    cp "$PREFIX/lib64/libharfbuzz.a" "$LIB_OUT/"

echo "=== Cleaning up ==="
rm -rf "$BUILD_DIR"

echo "=== Done ==="
echo "Static libs: $LIB_OUT/"
echo "Headers:     $PREFIX/include/"
ls -la "$LIB_OUT/"
