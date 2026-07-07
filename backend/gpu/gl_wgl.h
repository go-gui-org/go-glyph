//go:build windows

#ifndef GL_WGL_H
#define GL_WGL_H

#include <stdint.h>
#include <stdlib.h>

// Opaque OpenGL context — defined in gl_wgl.c.
typedef struct GLCtx GLCtx;

// Packed draw command matching Go drawCmd layout.
typedef struct {
	uint64_t textureID;
	int32_t  firstVert;
	int32_t  vertCount;
} CDrawCmd;

// glCtxInit creates a GL 3.3 core context via WGL on a caller-provided
// Win32 window. hwnd is a Win32 HWND passed as an integer handle.
GLCtx*    glCtxInit(uintptr_t hwnd, float dpiScale);
uint64_t  glCtxNewTex(GLCtx *ctx, int w, int h);
void      glCtxUpdateTex(GLCtx *ctx, uint64_t tid,
                         void *data, int w, int h);
void      glCtxDeleteTex(GLCtx *ctx, uint64_t tid);
int       glCtxRender(GLCtx *ctx,
                      void *verts, int vertCount,
                      void *cmds,  int cmdCount,
                      float clearR, float clearG,
                      float clearB, float clearA,
                      int logicalW, int logicalH);
void      glCtxDestroy(GLCtx *ctx);
void      glCtxGetDrawableSize(GLCtx *ctx, int *w, int *h);

#endif
