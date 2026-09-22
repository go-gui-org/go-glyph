#include "gles_android.h"

#include <EGL/egl.h>
#include <GLES3/gl3.h>
#include <android/native_window.h>
#include <string.h>
#include <stdio.h>

// GLSL ES 300 shaders.
static const char *vertSrc =
    "#version 300 es\n"
    "layout(location=0) in vec2 aPos;\n"
    "layout(location=1) in vec4 aColor;\n"
    "layout(location=2) in vec2 aTexCoord;\n"
    "uniform mat4 uProj;\n"
    "out vec4 vColor;\n"
    "out vec2 vTexCoord;\n"
    "void main() {\n"
    "    gl_Position = uProj * vec4(aPos, 0.0, 1.0);\n"
    "    vColor = aColor;\n"
    "    vTexCoord = aTexCoord;\n"
    "}\n";

static const char *fragSrc =
    "#version 300 es\n"
    "precision mediump float;\n"
    "in vec4 vColor;\n"
    "in vec2 vTexCoord;\n"
    "uniform sampler2D uTex;\n"
    "out vec4 fragColor;\n"
    "void main() {\n"
    "    fragColor = texture(uTex, vTexCoord) * vColor;\n"
    "}\n";

// Texture slot in a dynamic array.
typedef struct {
    GLuint glTex;
    int    used;
} TexSlot;

struct GLESCtx {
    EGLDisplay    display;
    EGLSurface    surface;
    EGLContext    context;
    ANativeWindow *window;
    int            ownsEGL;
    GLuint         program;
    GLuint         vao;
    GLuint         vbo;
    GLuint         whiteTex;
    GLint          uProj;
    GLint          uTex;
    TexSlot       *texSlots;
    int            texCap;
    uint64_t       nextTexID;
    int            surfW;
    int            surfH;
    int            vboCap;  // allocated VBO capacity in vertices
};

static GLuint compileShader(GLenum type, const char *src) {
    GLuint s = glCreateShader(type);
    glShaderSource(s, 1, &src, NULL);
    glCompileShader(s);
    GLint ok;
    glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        char buf[512];
        glGetShaderInfoLog(s, sizeof(buf), NULL, buf);
        fprintf(stderr, "gles shader compile: %s\n", buf);
        glDeleteShader(s);
        return 0;
    }
    return s;
}

static GLuint buildProgram(void) {
    GLuint vs = compileShader(GL_VERTEX_SHADER, vertSrc);
    GLuint fs = compileShader(GL_FRAGMENT_SHADER, fragSrc);
    if (!vs || !fs) return 0;

    GLuint prog = glCreateProgram();
    glAttachShader(prog, vs);
    glAttachShader(prog, fs);
    glLinkProgram(prog);
    glDeleteShader(vs);
    glDeleteShader(fs);

    GLint ok;
    glGetProgramiv(prog, GL_LINK_STATUS, &ok);
    if (!ok) {
        char buf[512];
        glGetProgramInfoLog(prog, sizeof(buf), NULL, buf);
        fprintf(stderr, "gles program link: %s\n", buf);
        glDeleteProgram(prog);
        return 0;
    }
    return prog;
}

GLESCtx* glesInit(void *nativeWindow, float dpiScale) {
    (void)dpiScale;
    ANativeWindow *win = (ANativeWindow *)nativeWindow;
    EGLDisplay display;
    EGLSurface surface;
    EGLContext context;
    int ownsEGL = 0;

    if (win) {
        // Create our own EGL context from the native window.
        display = eglGetDisplay(EGL_DEFAULT_DISPLAY);
        if (display == EGL_NO_DISPLAY) return NULL;
        if (!eglInitialize(display, NULL, NULL)) return NULL;

        EGLint configAttribs[] = {
            EGL_RENDERABLE_TYPE, EGL_OPENGL_ES3_BIT,
            EGL_SURFACE_TYPE, EGL_WINDOW_BIT,
            EGL_RED_SIZE, 8,
            EGL_GREEN_SIZE, 8,
            EGL_BLUE_SIZE, 8,
            EGL_ALPHA_SIZE, 8,
            EGL_DEPTH_SIZE, 0,
            EGL_NONE
        };
        EGLConfig config;
        EGLint numConfigs;
        if (!eglChooseConfig(display, configAttribs, &config, 1,
                             &numConfigs) || numConfigs == 0) {
            eglTerminate(display);
            return NULL;
        }

        EGLint format;
        eglGetConfigAttrib(display, config,
                           EGL_NATIVE_VISUAL_ID, &format);
        ANativeWindow_setBuffersGeometry(win, 0, 0, format);

        surface = eglCreateWindowSurface(
            display, config, win, NULL);
        if (surface == EGL_NO_SURFACE) {
            eglTerminate(display);
            return NULL;
        }

        EGLint ctxAttribs[] = {
            EGL_CONTEXT_CLIENT_VERSION, 3,
            EGL_NONE
        };
        context = eglCreateContext(
            display, config, EGL_NO_CONTEXT, ctxAttribs);
        if (context == EGL_NO_CONTEXT) {
            eglDestroySurface(display, surface);
            eglTerminate(display);
            return NULL;
        }

        if (!eglMakeCurrent(display, surface, surface, context)) {
            eglDestroyContext(display, context);
            eglDestroySurface(display, surface);
            eglTerminate(display);
            return NULL;
        }
        ownsEGL = 1;
    } else {
        // GLSurfaceView: EGL context already current.
        display = eglGetCurrentDisplay();
        surface = eglGetCurrentSurface(EGL_DRAW);
        context = eglGetCurrentContext();
        if (display == EGL_NO_DISPLAY ||
            context == EGL_NO_CONTEXT) return NULL;
    }

    GLuint prog = buildProgram();
    if (!prog) {
        if (ownsEGL) {
            eglDestroyContext(display, context);
            eglDestroySurface(display, surface);
            eglTerminate(display);
        }
        return NULL;
    }

    GLESCtx *ctx = (GLESCtx *)calloc(1, sizeof(GLESCtx));
    if (!ctx) {
        glDeleteProgram(prog);
        if (ownsEGL) {
            eglDestroyContext(display, context);
            eglDestroySurface(display, surface);
            eglTerminate(display);
        }
        return NULL;
    }
    ctx->display  = display;
    ctx->surface  = surface;
    ctx->context  = context;
    ctx->window   = win;
    ctx->ownsEGL  = ownsEGL;
    ctx->program  = prog;
    ctx->uProj   = glGetUniformLocation(prog, "uProj");
    ctx->uTex    = glGetUniformLocation(prog, "uTex");

    // VAO + VBO
    glGenVertexArrays(1, &ctx->vao);
    glBindVertexArray(ctx->vao);
    glGenBuffers(1, &ctx->vbo);
    glBindBuffer(GL_ARRAY_BUFFER, ctx->vbo);

    // Vertex layout: 20 bytes per vertex.
    // attr 0: 2×float pos @ offset 0
    glEnableVertexAttribArray(0);
    glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE,
                          20, (void*)0);
    // attr 1: 4×ubyte color @ offset 8 (normalized)
    glEnableVertexAttribArray(1);
    glVertexAttribPointer(1, 4, GL_UNSIGNED_BYTE, GL_TRUE,
                          20, (void*)8);
    // attr 2: 2×float texcoord @ offset 12
    glEnableVertexAttribArray(2);
    glVertexAttribPointer(2, 2, GL_FLOAT, GL_FALSE,
                          20, (void*)12);

    // 1×1 white texture for DrawFilledRect.
    glGenTextures(1, &ctx->whiteTex);
    glBindTexture(GL_TEXTURE_2D, ctx->whiteTex);
    uint8_t white[4] = {255, 255, 255, 255};
    glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA,
                 1, 1, 0, GL_RGBA, GL_UNSIGNED_BYTE, white);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);

    ctx->texCap   = 64;
    ctx->texSlots = (TexSlot *)calloc(ctx->texCap, sizeof(TexSlot));
    if (!ctx->texSlots) {
        ctx->texCap = 0;
        glesDestroy(ctx);
        return NULL;
    }

    // Query initial surface size.
    eglQuerySurface(display, surface, EGL_WIDTH, &ctx->surfW);
    eglQuerySurface(display, surface, EGL_HEIGHT, &ctx->surfH);

    return ctx;
}

// Grow texture slot array if needed.
static int ensureTexSlot(GLESCtx *ctx, uint64_t id) {
    if (id >= (uint64_t)ctx->texCap) {
        int newCap = ctx->texCap * 2;
        while (id >= (uint64_t)newCap) newCap *= 2;
        TexSlot *p = (TexSlot *)realloc(ctx->texSlots,
                                        newCap * sizeof(TexSlot));
        if (!p) return -1;
        ctx->texSlots = p;
        memset(ctx->texSlots + ctx->texCap, 0,
               (newCap - ctx->texCap) * sizeof(TexSlot));
        ctx->texCap = newCap;
    }
    return 0;
}

uint64_t glesNewTex(GLESCtx *ctx, int w, int h) {
    ctx->nextTexID++;
    uint64_t tid = ctx->nextTexID;
    if (ensureTexSlot(ctx, tid) != 0) return 0;

    GLuint tex;
    glGenTextures(1, &tex);
    glBindTexture(GL_TEXTURE_2D, tex);
    glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA,
                 w, h, 0, GL_RGBA, GL_UNSIGNED_BYTE, NULL);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S,
                    GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T,
                    GL_CLAMP_TO_EDGE);

    ctx->texSlots[tid].glTex = tex;
    ctx->texSlots[tid].used  = 1;
    return tid;
}

void glesUpdateTex(GLESCtx *ctx, uint64_t tid,
                   void *data, int w, int h) {
    if (tid >= (uint64_t)ctx->texCap || !ctx->texSlots[tid].used)
        return;
    glBindTexture(GL_TEXTURE_2D, ctx->texSlots[tid].glTex);
    glPixelStorei(GL_UNPACK_ROW_LENGTH, 0);
    glTexSubImage2D(GL_TEXTURE_2D, 0, 0, 0, w, h,
                    GL_RGBA, GL_UNSIGNED_BYTE, data);
}

void glesUpdateTexRect(GLESCtx *ctx, uint64_t tid, void *data,
                       int rowBytes, int x, int y, int w, int h) {
    if (tid >= (uint64_t)ctx->texCap || !ctx->texSlots[tid].used)
        return;
    glBindTexture(GL_TEXTURE_2D, ctx->texSlots[tid].glTex);
    // Rows in data are a whole atlas page apart, not w pixels apart.
    // GL_UNPACK_ROW_LENGTH is core in GLES 3.
    glPixelStorei(GL_UNPACK_ROW_LENGTH, rowBytes / 4);
    glTexSubImage2D(GL_TEXTURE_2D, 0, x, y, w, h,
                    GL_RGBA, GL_UNSIGNED_BYTE, data);
    glPixelStorei(GL_UNPACK_ROW_LENGTH, 0);
}

void glesDeleteTex(GLESCtx *ctx, uint64_t tid) {
    if (tid >= (uint64_t)ctx->texCap || !ctx->texSlots[tid].used)
        return;
    glDeleteTextures(1, &ctx->texSlots[tid].glTex);
    ctx->texSlots[tid].glTex = 0;
    ctx->texSlots[tid].used  = 0;
}

int glesRender(GLESCtx *ctx,
               void *verts, int vertCount,
               void *cmds,  int cmdCount,
               float clearR, float clearG,
               float clearB, float clearA,
               int logicalW, int logicalH) {
    if (!ctx) return -1;

    // Query surface size only when we own EGL (no resize events).
    if (ctx->ownsEGL) {
        eglQuerySurface(ctx->display, ctx->surface,
                        EGL_WIDTH, &ctx->surfW);
        eglQuerySurface(ctx->display, ctx->surface,
                        EGL_HEIGHT, &ctx->surfH);
    }
    int physW = ctx->surfW;
    int physH = ctx->surfH;
    if (physW == 0 || physH == 0) return -1;

    glViewport(0, 0, physW, physH);
    glClearColor(clearR, clearG, clearB, clearA);
    glClear(GL_COLOR_BUFFER_BIT);

    glEnable(GL_BLEND);
    glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);

    glUseProgram(ctx->program);

    // Orthographic projection: logical coords → NDC.
    float L = 0, R = (float)logicalW;
    float T = 0, B = (float)logicalH;
    float proj[16] = {
        2.0f/(R-L),     0,               0, 0,
        0,               2.0f/(T-B),     0, 0,
        0,               0,              -1, 0,
        -(R+L)/(R-L),   -(T+B)/(T-B),    0, 1,
    };
    glUniformMatrix4fv(ctx->uProj, 1, GL_FALSE, proj);

    // Upload vertex data.
    glBindVertexArray(ctx->vao);
    glBindBuffer(GL_ARRAY_BUFFER, ctx->vbo);
    if (vertCount > 0 && verts) {
        if (vertCount > ctx->vboCap) {
            glBufferData(GL_ARRAY_BUFFER,
                         (GLsizeiptr)vertCount * 20,
                         verts, GL_DYNAMIC_DRAW);
            ctx->vboCap = vertCount;
        } else {
            glBufferSubData(GL_ARRAY_BUFFER, 0,
                            (GLsizeiptr)vertCount * 20, verts);
        }
    }

    // Set texture unit 0.
    glActiveTexture(GL_TEXTURE0);
    glUniform1i(ctx->uTex, 0);

    // Draw each command.
    if (cmdCount > 0 && cmds) {
        CDrawCmd *dcmds = (CDrawCmd *)cmds;
        for (int i = 0; i < cmdCount; i++) {
            CDrawCmd *dc = &dcmds[i];
            GLuint tex = ctx->whiteTex;
            if (dc->textureID != 0 &&
                dc->textureID < (uint64_t)ctx->texCap &&
                ctx->texSlots[dc->textureID].used) {
                tex = ctx->texSlots[dc->textureID].glTex;
            }
            glBindTexture(GL_TEXTURE_2D, tex);
            glDrawArrays(GL_TRIANGLES,
                         dc->firstVert, dc->vertCount);
        }
    }

    if (ctx->ownsEGL)
        eglSwapBuffers(ctx->display, ctx->surface);
    return 0;
}

void glesDestroy(GLESCtx *ctx) {
    if (!ctx) return;
    // Delete user textures.
    for (int i = 0; i < ctx->texCap; i++) {
        if (ctx->texSlots[i].used) {
            glDeleteTextures(1, &ctx->texSlots[i].glTex);
        }
    }
    free(ctx->texSlots);
    if (ctx->whiteTex) glDeleteTextures(1, &ctx->whiteTex);
    if (ctx->vbo)      glDeleteBuffers(1, &ctx->vbo);
    if (ctx->vao)      glDeleteVertexArrays(1, &ctx->vao);
    if (ctx->program)  glDeleteProgram(ctx->program);
    if (ctx->ownsEGL) {
        if (ctx->context != EGL_NO_CONTEXT)
            eglDestroyContext(ctx->display, ctx->context);
        if (ctx->surface != EGL_NO_SURFACE)
            eglDestroySurface(ctx->display, ctx->surface);
        if (ctx->display != EGL_NO_DISPLAY)
            eglTerminate(ctx->display);
    }
    free(ctx);
}

void glesGetDrawableSize(GLESCtx *ctx, int *w, int *h) {
    if (!ctx) { *w = 0; *h = 0; return; }
    eglQuerySurface(ctx->display, ctx->surface, EGL_WIDTH, w);
    eglQuerySurface(ctx->display, ctx->surface, EGL_HEIGHT, h);
}
