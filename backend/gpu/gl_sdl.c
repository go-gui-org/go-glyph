//go:build windows

#include "gl_sdl.h"
#include "SDL_opengl.h"
#include <string.h>
#include <stdio.h>

// GL 3.3 core function pointers loaded via SDL_GL_GetProcAddress.
typedef unsigned int GLenum;
typedef int          GLint;
typedef unsigned int GLuint;
typedef int          GLsizei;
typedef float        GLfloat;
typedef ptrdiff_t    GLsizeiptr;
typedef char         GLchar;
typedef unsigned char GLboolean;
typedef unsigned int  GLbitfield;

#define MY_GL_ARRAY_BUFFER         0x8892
#define MY_GL_DYNAMIC_DRAW         0x88E8
#define MY_GL_FRAGMENT_SHADER      0x8B30
#define MY_GL_VERTEX_SHADER        0x8B31
#define MY_GL_COMPILE_STATUS       0x8B81
#define MY_GL_LINK_STATUS          0x8B82
#define MY_GL_TEXTURE_2D           0x0DE1
#define MY_GL_TEXTURE_MIN_FILTER   0x2801
#define MY_GL_TEXTURE_MAG_FILTER   0x2800
#define MY_GL_TEXTURE_WRAP_S       0x2802
#define MY_GL_TEXTURE_WRAP_T       0x2803
#define MY_GL_LINEAR               0x2601
#define MY_GL_CLAMP_TO_EDGE        0x812F
#define MY_GL_RGBA                 0x1908
#define MY_GL_UNSIGNED_BYTE        0x1401
#define MY_GL_FLOAT                0x1406
#define MY_GL_FALSE                0
#define MY_GL_TRUE                 1
#define MY_GL_TRIANGLES            0x0004
#define MY_GL_BLEND                0x0BE2
#define MY_GL_SRC_ALPHA            0x0302
#define MY_GL_ONE_MINUS_SRC_ALPHA  0x0303
#define MY_GL_COLOR_BUFFER_BIT     0x00004000
#define MY_GL_UNPACK_ROW_LENGTH   0x0CF2
#define MY_GL_INFO_LOG_LENGTH      0x8B84

// GL function pointer types and storage.
#define GLFUNC(ret, name, ...) typedef ret (*PFN_##name)(__VA_ARGS__); static PFN_##name p_##name = NULL

GLFUNC(void,   glEnable, GLenum);
GLFUNC(void,   glDisable, GLenum);
GLFUNC(void,   glBlendFunc, GLenum, GLenum);
GLFUNC(void,   glClearColor, GLfloat, GLfloat, GLfloat, GLfloat);
GLFUNC(void,   glClear, GLbitfield);
GLFUNC(void,   glViewport, GLint, GLint, GLsizei, GLsizei);
GLFUNC(void,   glDrawArrays, GLenum, GLint, GLsizei);
GLFUNC(void,   glGenTextures, GLsizei, GLuint*);
GLFUNC(void,   glDeleteTextures, GLsizei, const GLuint*);
GLFUNC(void,   glBindTexture, GLenum, GLuint);
GLFUNC(void,   glTexImage2D, GLenum, GLint, GLint, GLsizei, GLsizei, GLint, GLenum, GLenum, const void*);
GLFUNC(void,   glTexSubImage2D, GLenum, GLint, GLint, GLint, GLsizei, GLsizei, GLenum, GLenum, const void*);
GLFUNC(void,   glTexParameteri, GLenum, GLenum, GLint);
GLFUNC(void,   glPixelStorei, GLenum, GLint);
GLFUNC(void,   glActiveTexture, GLenum);

// GL 2.0+ / 3.3 core
GLFUNC(GLuint, glCreateShader, GLenum);
GLFUNC(void,   glShaderSource, GLuint, GLsizei, const GLchar**, const GLint*);
GLFUNC(void,   glCompileShader, GLuint);
GLFUNC(void,   glGetShaderiv, GLuint, GLenum, GLint*);
GLFUNC(void,   glGetShaderInfoLog, GLuint, GLsizei, GLsizei*, GLchar*);
GLFUNC(void,   glDeleteShader, GLuint);
GLFUNC(GLuint, glCreateProgram, void);
GLFUNC(void,   glAttachShader, GLuint, GLuint);
GLFUNC(void,   glLinkProgram, GLuint);
GLFUNC(void,   glGetProgramiv, GLuint, GLenum, GLint*);
GLFUNC(void,   glGetProgramInfoLog, GLuint, GLsizei, GLsizei*, GLchar*);
GLFUNC(void,   glUseProgram, GLuint);
GLFUNC(void,   glDeleteProgram, GLuint);
GLFUNC(GLint,  glGetUniformLocation, GLuint, const GLchar*);
GLFUNC(void,   glUniformMatrix4fv, GLint, GLsizei, GLboolean, const GLfloat*);
GLFUNC(void,   glUniform1i, GLint, GLint);

// VAO/VBO
GLFUNC(void,   glGenVertexArrays, GLsizei, GLuint*);
GLFUNC(void,   glDeleteVertexArrays, GLsizei, const GLuint*);
GLFUNC(void,   glBindVertexArray, GLuint);
GLFUNC(void,   glGenBuffers, GLsizei, GLuint*);
GLFUNC(void,   glDeleteBuffers, GLsizei, const GLuint*);
GLFUNC(void,   glBindBuffer, GLenum, GLuint);
GLFUNC(void,   glBufferData, GLenum, GLsizeiptr, const void*, GLenum);
GLFUNC(void,   glEnableVertexAttribArray, GLuint);
GLFUNC(void,   glVertexAttribPointer, GLuint, GLint, GLenum, GLboolean, GLsizei, const void*);

static int loadGL(void) {
    #define LOAD(name) p_##name = (PFN_##name)SDL_GL_GetProcAddress(#name); if (!p_##name) return -1
    LOAD(glEnable);
    LOAD(glDisable);
    LOAD(glBlendFunc);
    LOAD(glClearColor);
    LOAD(glClear);
    LOAD(glViewport);
    LOAD(glDrawArrays);
    LOAD(glGenTextures);
    LOAD(glDeleteTextures);
    LOAD(glBindTexture);
    LOAD(glTexImage2D);
    LOAD(glTexSubImage2D);
    LOAD(glTexParameteri);
    LOAD(glPixelStorei);
    LOAD(glActiveTexture);
    LOAD(glCreateShader);
    LOAD(glShaderSource);
    LOAD(glCompileShader);
    LOAD(glGetShaderiv);
    LOAD(glGetShaderInfoLog);
    LOAD(glDeleteShader);
    LOAD(glCreateProgram);
    LOAD(glAttachShader);
    LOAD(glLinkProgram);
    LOAD(glGetProgramiv);
    LOAD(glGetProgramInfoLog);
    LOAD(glUseProgram);
    LOAD(glDeleteProgram);
    LOAD(glGetUniformLocation);
    LOAD(glUniformMatrix4fv);
    LOAD(glUniform1i);
    LOAD(glGenVertexArrays);
    LOAD(glDeleteVertexArrays);
    LOAD(glBindVertexArray);
    LOAD(glGenBuffers);
    LOAD(glDeleteBuffers);
    LOAD(glBindBuffer);
    LOAD(glBufferData);
    LOAD(glEnableVertexAttribArray);
    LOAD(glVertexAttribPointer);
    #undef LOAD
    return 0;
}

// GLSL 330 core shaders.
static const char *vertSrc =
    "#version 330 core\n"
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
    "#version 330 core\n"
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

struct GLCtx {
    SDL_Window    *window;
    SDL_GLContext  glctx;
    GLuint         program;
    GLuint         vao;
    GLuint         vbo;
    GLuint         whiteTex;
    GLint          uProj;
    GLint          uTex;
    TexSlot       *texSlots;
    int            texCap;
    uint64_t       nextTexID;
};

static GLuint compileShader(GLenum type, const char *src) {
    GLuint s = p_glCreateShader(type);
    p_glShaderSource(s, 1, &src, NULL);
    p_glCompileShader(s);
    GLint ok;
    p_glGetShaderiv(s, MY_GL_COMPILE_STATUS, &ok);
    if (!ok) {
        GLint len;
        p_glGetShaderiv(s, MY_GL_INFO_LOG_LENGTH, &len);
        char buf[512];
        if (len > (GLint)sizeof(buf)) len = sizeof(buf);
        p_glGetShaderInfoLog(s, len, NULL, buf);
        fprintf(stderr, "gl shader compile: %s\n", buf);
        p_glDeleteShader(s);
        return 0;
    }
    return s;
}

static GLuint buildProgram(void) {
    GLuint vs = compileShader(MY_GL_VERTEX_SHADER, vertSrc);
    GLuint fs = compileShader(MY_GL_FRAGMENT_SHADER, fragSrc);
    if (!vs || !fs) return 0;

    GLuint prog = p_glCreateProgram();
    p_glAttachShader(prog, vs);
    p_glAttachShader(prog, fs);
    p_glLinkProgram(prog);
    p_glDeleteShader(vs);
    p_glDeleteShader(fs);

    GLint ok;
    p_glGetProgramiv(prog, MY_GL_LINK_STATUS, &ok);
    if (!ok) {
        char buf[512];
        p_glGetProgramInfoLog(prog, sizeof(buf), NULL, buf);
        fprintf(stderr, "gl program link: %s\n", buf);
        p_glDeleteProgram(prog);
        return 0;
    }
    return prog;
}

GLCtx* glCtxInit(void *sdlWindow, float dpiScale) {
    (void)dpiScale;
    SDL_Window *win = (SDL_Window *)sdlWindow;

    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 3);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK,
                        SDL_GL_CONTEXT_PROFILE_CORE);
    SDL_GL_SetAttribute(SDL_GL_DOUBLEBUFFER, 1);

    SDL_GLContext glctx = SDL_GL_CreateContext(win);
    if (!glctx) {
        fprintf(stderr, "gl: SDL_GL_CreateContext: %s\n",
                SDL_GetError());
        return NULL;
    }
    SDL_GL_MakeCurrent(win, glctx);
    SDL_GL_SetSwapInterval(1); // vsync

    if (loadGL() != 0) {
        fprintf(stderr, "gl: failed to load GL functions\n");
        SDL_GL_DeleteContext(glctx);
        return NULL;
    }

    GLuint prog = buildProgram();
    if (!prog) {
        SDL_GL_DeleteContext(glctx);
        return NULL;
    }

    GLCtx *ctx = (GLCtx *)calloc(1, sizeof(GLCtx));
    ctx->window  = win;
    ctx->glctx   = glctx;
    ctx->program = prog;
    ctx->uProj   = p_glGetUniformLocation(prog, "uProj");
    ctx->uTex    = p_glGetUniformLocation(prog, "uTex");

    // VAO + VBO
    p_glGenVertexArrays(1, &ctx->vao);
    p_glBindVertexArray(ctx->vao);
    p_glGenBuffers(1, &ctx->vbo);
    p_glBindBuffer(MY_GL_ARRAY_BUFFER, ctx->vbo);

    // Vertex layout: 20 bytes per vertex.
    // attr 0: 2×float pos @ offset 0
    p_glEnableVertexAttribArray(0);
    p_glVertexAttribPointer(0, 2, MY_GL_FLOAT, MY_GL_FALSE,
                            20, (void*)0);
    // attr 1: 4×ubyte color @ offset 8 (normalized)
    p_glEnableVertexAttribArray(1);
    p_glVertexAttribPointer(1, 4, MY_GL_UNSIGNED_BYTE, MY_GL_TRUE,
                            20, (void*)8);
    // attr 2: 2×float texcoord @ offset 12
    p_glEnableVertexAttribArray(2);
    p_glVertexAttribPointer(2, 2, MY_GL_FLOAT, MY_GL_FALSE,
                            20, (void*)12);

    // 1×1 white texture for DrawFilledRect.
    p_glGenTextures(1, &ctx->whiteTex);
    p_glBindTexture(MY_GL_TEXTURE_2D, ctx->whiteTex);
    uint8_t white[4] = {255, 255, 255, 255};
    p_glTexImage2D(MY_GL_TEXTURE_2D, 0, MY_GL_RGBA,
                   1, 1, 0, MY_GL_RGBA, MY_GL_UNSIGNED_BYTE, white);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_MIN_FILTER,
                      MY_GL_LINEAR);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_MAG_FILTER,
                      MY_GL_LINEAR);

    ctx->texCap   = 64;
    ctx->texSlots = (TexSlot *)calloc(ctx->texCap, sizeof(TexSlot));

    return ctx;
}

// Grow texture slot array if needed.
static void ensureTexSlot(GLCtx *ctx, uint64_t id) {
    if ((int)id >= ctx->texCap) {
        int newCap = ctx->texCap * 2;
        while ((int)id >= newCap) newCap *= 2;
        ctx->texSlots = (TexSlot *)realloc(ctx->texSlots,
                                           newCap * sizeof(TexSlot));
        memset(ctx->texSlots + ctx->texCap, 0,
               (newCap - ctx->texCap) * sizeof(TexSlot));
        ctx->texCap = newCap;
    }
}

uint64_t glCtxNewTex(GLCtx *ctx, int w, int h) {
    ctx->nextTexID++;
    uint64_t tid = ctx->nextTexID;
    ensureTexSlot(ctx, tid);

    GLuint tex;
    p_glGenTextures(1, &tex);
    p_glBindTexture(MY_GL_TEXTURE_2D, tex);
    p_glTexImage2D(MY_GL_TEXTURE_2D, 0, MY_GL_RGBA,
                   w, h, 0, MY_GL_RGBA, MY_GL_UNSIGNED_BYTE, NULL);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_MIN_FILTER,
                      MY_GL_LINEAR);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_MAG_FILTER,
                      MY_GL_LINEAR);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_WRAP_S,
                      MY_GL_CLAMP_TO_EDGE);
    p_glTexParameteri(MY_GL_TEXTURE_2D, MY_GL_TEXTURE_WRAP_T,
                      MY_GL_CLAMP_TO_EDGE);

    ctx->texSlots[tid].glTex = tex;
    ctx->texSlots[tid].used  = 1;
    return tid;
}

void glCtxUpdateTex(GLCtx *ctx, uint64_t tid,
                    void *data, int w, int h) {
    if ((int)tid >= ctx->texCap || !ctx->texSlots[tid].used) return;
    p_glBindTexture(MY_GL_TEXTURE_2D, ctx->texSlots[tid].glTex);
    p_glPixelStorei(MY_GL_UNPACK_ROW_LENGTH, 0);
    p_glTexSubImage2D(MY_GL_TEXTURE_2D, 0, 0, 0, w, h,
                      MY_GL_RGBA, MY_GL_UNSIGNED_BYTE, data);
}

void glCtxDeleteTex(GLCtx *ctx, uint64_t tid) {
    if ((int)tid >= ctx->texCap || !ctx->texSlots[tid].used) return;
    p_glDeleteTextures(1, &ctx->texSlots[tid].glTex);
    ctx->texSlots[tid].glTex = 0;
    ctx->texSlots[tid].used  = 0;
}

int glCtxRender(GLCtx *ctx,
                void *verts, int vertCount,
                void *cmds,  int cmdCount,
                float clearR, float clearG,
                float clearB, float clearA,
                int logicalW, int logicalH) {
    if (!ctx) return -1;

    int physW, physH;
    SDL_GL_GetDrawableSize(ctx->window, &physW, &physH);
    if (physW == 0 || physH == 0) return -1;

    p_glViewport(0, 0, physW, physH);
    p_glClearColor(clearR, clearG, clearB, clearA);
    p_glClear(MY_GL_COLOR_BUFFER_BIT);

    p_glEnable(MY_GL_BLEND);
    p_glBlendFunc(MY_GL_SRC_ALPHA, MY_GL_ONE_MINUS_SRC_ALPHA);

    p_glUseProgram(ctx->program);

    // Orthographic projection: logical coords → NDC.
    float L = 0, R = (float)logicalW;
    float T = 0, B = (float)logicalH;
    float proj[16] = {
        2.0f/(R-L),     0,               0, 0,
        0,               2.0f/(T-B),     0, 0,
        0,               0,              -1, 0,
        -(R+L)/(R-L),   -(T+B)/(T-B),    0, 1,
    };
    p_glUniformMatrix4fv(ctx->uProj, 1, MY_GL_FALSE, proj);

    // Upload vertex data.
    p_glBindVertexArray(ctx->vao);
    p_glBindBuffer(MY_GL_ARRAY_BUFFER, ctx->vbo);
    if (vertCount > 0 && verts) {
        p_glBufferData(MY_GL_ARRAY_BUFFER,
                       (GLsizeiptr)vertCount * 20,
                       verts, MY_GL_DYNAMIC_DRAW);
    }

    // Set texture unit 0.
    p_glActiveTexture(0x84C0); // GL_TEXTURE0
    p_glUniform1i(ctx->uTex, 0);

    // Draw each command.
    if (cmdCount > 0 && cmds) {
        CDrawCmd *dcmds = (CDrawCmd *)cmds;
        for (int i = 0; i < cmdCount; i++) {
            CDrawCmd *dc = &dcmds[i];
            GLuint tex = ctx->whiteTex;
            if (dc->textureID != 0 &&
                (int)dc->textureID < ctx->texCap &&
                ctx->texSlots[dc->textureID].used) {
                tex = ctx->texSlots[dc->textureID].glTex;
            }
            p_glBindTexture(MY_GL_TEXTURE_2D, tex);
            p_glDrawArrays(MY_GL_TRIANGLES,
                           dc->firstVert, dc->vertCount);
        }
    }

    SDL_GL_SwapWindow(ctx->window);
    return 0;
}

void glCtxDestroy(GLCtx *ctx) {
    if (!ctx) return;
    // Delete user textures.
    for (int i = 0; i < ctx->texCap; i++) {
        if (ctx->texSlots[i].used) {
            p_glDeleteTextures(1, &ctx->texSlots[i].glTex);
        }
    }
    free(ctx->texSlots);
    if (ctx->whiteTex) p_glDeleteTextures(1, &ctx->whiteTex);
    if (ctx->vbo)      p_glDeleteBuffers(1, &ctx->vbo);
    if (ctx->vao)      p_glDeleteVertexArrays(1, &ctx->vao);
    if (ctx->program)  p_glDeleteProgram(ctx->program);
    if (ctx->glctx)    SDL_GL_DeleteContext(ctx->glctx);
    free(ctx);
}

void glCtxGetDrawableSize(GLCtx *ctx, int *w, int *h) {
    if (!ctx) { *w = 0; *h = 0; return; }
    SDL_GL_GetDrawableSize(ctx->window, w, h);
}
