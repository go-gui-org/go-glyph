#ifndef GLES_ANDROID_H
#define GLES_ANDROID_H

#include <stdint.h>
#include <stdlib.h>

// Opaque GLES context — defined in gles_android.c.
typedef struct GLESCtx GLESCtx;

// Packed draw command matching Go drawCmd layout.
typedef struct {
	uint64_t textureID;
	int32_t  firstVert;
	int32_t  vertCount;
} CDrawCmd;

GLESCtx*  glesInit(void *nativeWindow, float dpiScale);
uint64_t  glesNewTex(GLESCtx *ctx, int w, int h);
void      glesUpdateTex(GLESCtx *ctx, uint64_t tid,
                        void *data, int w, int h);
// glesUpdateTexRect uploads the (x, y, w, h) region of a texture. data
// points at the region's first pixel; rows are rowBytes apart.
void      glesUpdateTexRect(GLESCtx *ctx, uint64_t tid, void *data,
                             int rowBytes, int x, int y, int w, int h);
void      glesDeleteTex(GLESCtx *ctx, uint64_t tid);
int       glesRender(GLESCtx *ctx,
                     void *verts, int vertCount,
                     void *cmds,  int cmdCount,
                     float clearR, float clearG,
                     float clearB, float clearA,
                     int logicalW, int logicalH);
void      glesDestroy(GLESCtx *ctx);
void      glesGetDrawableSize(GLESCtx *ctx, int *w, int *h);

#endif
