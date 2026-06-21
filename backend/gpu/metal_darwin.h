//go:build !ios

#ifndef METAL_DARWIN_H
#define METAL_DARWIN_H

#include <stdint.h>
#include <stdlib.h>

// Opaque Metal context — defined in metal_darwin.m.
typedef struct MetalCtx MetalCtx;

// Packed draw command matching Go drawCmd layout.
typedef struct {
	uint64_t textureID;
	int32_t  firstVert;
	int32_t  vertCount;
} CDrawCmd;

MetalCtx* metalInit(void *metalLayer);
uint64_t  metalNewTex(MetalCtx *ctx, int w, int h);
void      metalUpdateTex(MetalCtx *ctx, uint64_t tid,
                         void *data, int w, int h);
void      metalDeleteTex(MetalCtx *ctx, uint64_t tid);
int       metalRender(MetalCtx *ctx,
                      void *verts, int vertCount,
                      void *cmds,  int cmdCount,
                      float clearR, float clearG,
                      float clearB, float clearA,
                      int logicalW, int logicalH);
void      metalDestroy(MetalCtx *ctx);
void      metalGetDrawableSize(MetalCtx *ctx, int *w, int *h);

#endif
