#!/usr/bin/env bash
set -euo pipefail

FREETYPE_VER="2.13.3"
HARFBUZZ_VER="10.2.0"
NJOBS=$(nproc)
PREFIX=/build/deps
BUILD_DIR=/tmp/build_deps

rm -rf "$BUILD_DIR" "$PREFIX"
mkdir -p "$BUILD_DIR" "$PREFIX/lib/linux_amd64" "$PREFIX/lib/linux_arm64" "$PREFIX/include"

build_freetype() {
    local CC="$1" CXX="$2" AR="$3" RANLIB="$4" HOST="$5" OUTDIR="$6"
    local FT_DIR="$BUILD_DIR/freetype-$FREETYPE_VER"
    if [ ! -d "$FT_DIR" ]; then
        echo "=== Downloading FreeType $FREETYPE_VER ==="
        curl -sfL "https://downloads.sourceforge.net/project/freetype/freetype2/$FREETYPE_VER/freetype-$FREETYPE_VER.tar.xz" \
            -o "$BUILD_DIR/ft.tar.xz"
        tar xf "$BUILD_DIR/ft.tar.xz" -C "$BUILD_DIR"
    fi
    cd "$FT_DIR"
    make clean 2>/dev/null || true
    make distclean 2>/dev/null || true
    ./configure --host="$HOST" --prefix="$PREFIX" \
        --enable-static --disable-shared \
        --with-zlib=no --with-bzip2=no --with-png=no --with-brotli=no \
        CC="$CC" CXX="$CXX" AR="$AR" RANLIB="$RANLIB" \
        CFLAGS="-O2 -fPIC"
    make -j"$NJOBS" install
    cp "$PREFIX/lib/libfreetype.a" "$OUTDIR/"
}

build_harfbuzz() {
    local CC="$1" CXX="$2" AR="$3" RANLIB="$4" HOST="$5" OUTDIR="$6"
    local HB_DIR="$BUILD_DIR/harfbuzz-$HARFBUZZ_VER"
    if [ ! -d "$HB_DIR" ]; then
        echo "=== Downloading HarfBuzz $HARFBUZZ_VER ==="
        curl -sfL "https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VER/harfbuzz-$HARFBUZZ_VER.tar.xz" \
            -o "$BUILD_DIR/hb.tar.xz"
        tar xf "$BUILD_DIR/hb.tar.xz" -C "$BUILD_DIR"
    fi
    cd "$HB_DIR"
    rm -rf build
    mkdir build && cd build
    if [ "$HOST" = "x86_64-linux-gnu" ]; then
        cmake .. \
            -DCMAKE_INSTALL_PREFIX="$PREFIX" \
            -DBUILD_SHARED_LIBS=OFF \
            -DHB_HAVE_FREETYPE=ON \
            -DHB_HAVE_GLIB=OFF -DHB_HAVE_GOBJECT=OFF \
            -DHB_HAVE_CAIRO=OFF -DHB_HAVE_ICU=OFF \
            -DHB_HAVE_GRAPHITE2=OFF -DHB_HAVE_WASM=OFF \
            -DFREETYPE_LIBRARY="$PREFIX/lib/libfreetype.a" \
            -DFREETYPE_INCLUDE_DIRS="$PREFIX/include/freetype2"
    else
        cmake .. \
            -DCMAKE_SYSTEM_NAME=Linux \
            -DCMAKE_SYSTEM_PROCESSOR=aarch64 \
            -DCMAKE_C_COMPILER="$CC" \
            -DCMAKE_CXX_COMPILER="$CXX" \
            -DCMAKE_INSTALL_PREFIX="$PREFIX" \
            -DBUILD_SHARED_LIBS=OFF \
            -DHB_HAVE_FREETYPE=ON \
            -DHB_HAVE_GLIB=OFF -DHB_HAVE_GOBJECT=OFF \
            -DHB_HAVE_CAIRO=OFF -DHB_HAVE_ICU=OFF \
            -DHB_HAVE_GRAPHITE2=OFF -DHB_HAVE_WASM=OFF \
            -DFREETYPE_LIBRARY="$PREFIX/lib/libfreetype.a" \
            -DFREETYPE_INCLUDE_DIRS="$PREFIX/include/freetype2"
    fi
    make -j"$NJOBS" install
    cp "$PREFIX/lib/libharfbuzz.a" "$OUTDIR/"
}

echo "=== Building for amd64 (native) ==="
build_freetype gcc g++ ar ranlib x86_64-linux-gnu "$PREFIX/lib/linux_amd64"
build_harfbuzz gcc g++ ar ranlib x86_64-linux-gnu "$PREFIX/lib/linux_amd64"

echo "=== Building for arm64 (cross) ==="
build_freetype aarch64-linux-gnu-gcc aarch64-linux-gnu-g++ aarch64-linux-gnu-ar aarch64-linux-gnu-ranlib aarch64-linux-gnu "$PREFIX/lib/linux_arm64"
make -C "$BUILD_DIR/freetype-$FREETYPE_VER" clean 2>/dev/null || true
build_harfbuzz aarch64-linux-gnu-gcc aarch64-linux-gnu-g++ aarch64-linux-gnu-ar aarch64-linux-gnu-ranlib aarch64-linux-gnu "$PREFIX/lib/linux_arm64"

echo "=== Done ==="
ls -la "$PREFIX/lib/linux_amd64/"
ls -la "$PREFIX/lib/linux_arm64/"
