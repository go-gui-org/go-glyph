//go:build linux && !android

#include "gl_glx.h"
#include <X11/Xlib.h>
#include <GL/glx.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>

// GL 3.3 core rendering with a native GLX context. GL 1.1 entry points
// are exported directly by libGL and called by name; GL 2.0+/3.3 entry
// points are resolved at runtime via glXGetProcAddressARB.

// glXCreateContextAttribsARB tokens (may predate the installed glxext.h).
#ifndef GLX_CONTEXT_MAJOR_VERSION_ARB
#define GLX_CONTEXT_MAJOR_VERSION_ARB      0x2091
#define GLX_CONTEXT_MINOR_VERSION_ARB      0x2092
#define GLX_CONTEXT_PROFILE_MASK_ARB       0x9126
#define GLX_CONTEXT_CORE_PROFILE_BIT_ARB   0x00000001
#endif

#define GL_TEXTURE0 0x84C0

// Function-pointer typedefs for the dynamically loaded GL entry points.
// Signatures use plain char/ptrdiff_t to avoid depending on GLchar /
// GLsizeiptr, which live in glext.h rather than base gl.h.
#define GLFUNC(ret, name, ...) \
	typedef ret (*PFN_##name)(__VA_ARGS__); static PFN_##name p_##name = NULL

GLFUNC(void,   glActiveTexture, GLenum);
GLFUNC(GLuint, glCreateShader, GLenum);
GLFUNC(void,   glShaderSource, GLuint, GLsizei, const char**, const GLint*);
GLFUNC(void,   glCompileShader, GLuint);
GLFUNC(void,   glGetShaderiv, GLuint, GLenum, GLint*);
GLFUNC(void,   glGetShaderInfoLog, GLuint, GLsizei, GLsizei*, char*);
GLFUNC(void,   glDeleteShader, GLuint);
GLFUNC(GLuint, glCreateProgram, void);
GLFUNC(void,   glAttachShader, GLuint, GLuint);
GLFUNC(void,   glLinkProgram, GLuint);
GLFUNC(void,   glGetProgramiv, GLuint, GLenum, GLint*);
GLFUNC(void,   glGetProgramInfoLog, GLuint, GLsizei, GLsizei*, char*);
GLFUNC(void,   glUseProgram, GLuint);
GLFUNC(void,   glDeleteProgram, GLuint);
GLFUNC(GLint,  glGetUniformLocation, GLuint, const char*);
GLFUNC(void,   glUniformMatrix4fv, GLint, GLsizei, GLboolean, const GLfloat*);
GLFUNC(void,   glUniform1i, GLint, GLint);
GLFUNC(void,   glGenVertexArrays, GLsizei, GLuint*);
GLFUNC(void,   glDeleteVertexArrays, GLsizei, const GLuint*);
GLFUNC(void,   glBindVertexArray, GLuint);
GLFUNC(void,   glGenBuffers, GLsizei, GLuint*);
GLFUNC(void,   glDeleteBuffers, GLsizei, const GLuint*);
GLFUNC(void,   glBindBuffer, GLenum, GLuint);
GLFUNC(void,   glBufferData, GLenum, ptrdiff_t, const void*, GLenum);
GLFUNC(void,   glEnableVertexAttribArray, GLuint);
GLFUNC(void,   glVertexAttribPointer, GLuint, GLint, GLenum, GLboolean,
       GLsizei, const void*);

static int loadGL(void) {
	#define LOAD(name) \
		p_##name = (PFN_##name)glXGetProcAddressARB((const GLubyte*)#name); \
		if (!p_##name) return -1
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
	Display     *dpy;
	Window       win;
	GLXContext   glc;
	GLuint       program;
	GLuint       vao;
	GLuint       vbo;
	GLuint       whiteTex;
	GLint        uProj;
	GLint        uTex;
	TexSlot     *texSlots;
	int          texCap;
	uint64_t     nextTexID;
};

static GLuint compileShader(GLenum type, const char *src) {
	GLuint s = p_glCreateShader(type);
	p_glShaderSource(s, 1, &src, NULL);
	p_glCompileShader(s);
	GLint ok;
	p_glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
	if (!ok) {
		char buf[512];
		p_glGetShaderInfoLog(s, sizeof(buf), NULL, buf);
		fprintf(stderr, "gl shader compile: %s\n", buf);
		p_glDeleteShader(s);
		return 0;
	}
	return s;
}

static GLuint buildProgram(void) {
	GLuint vs = compileShader(GL_VERTEX_SHADER, vertSrc);
	GLuint fs = compileShader(GL_FRAGMENT_SHADER, fragSrc);
	if (!vs || !fs) return 0;

	GLuint prog = p_glCreateProgram();
	p_glAttachShader(prog, vs);
	p_glAttachShader(prog, fs);
	p_glLinkProgram(prog);
	p_glDeleteShader(vs);
	p_glDeleteShader(fs);

	GLint ok;
	p_glGetProgramiv(prog, GL_LINK_STATUS, &ok);
	if (!ok) {
		char buf[512];
		p_glGetProgramInfoLog(prog, sizeof(buf), NULL, buf);
		fprintf(stderr, "gl program link: %s\n", buf);
		p_glDeleteProgram(prog);
		return 0;
	}
	return prog;
}

// Find the GLXFBConfig whose visual matches the caller's window, so the
// context is compatible with the already-created drawable.
static int findFBConfig(Display *dpy, int screen, VisualID vid,
                        GLXFBConfig *out) {
	int n = 0;
	GLXFBConfig *cfgs = glXGetFBConfigs(dpy, screen, &n);
	if (!cfgs || n == 0) return -1;
	for (int i = 0; i < n; i++) {
		int fbvid = 0, rtype = 0, dtype = 0;
		glXGetFBConfigAttrib(dpy, cfgs[i], GLX_VISUAL_ID, &fbvid);
		if ((VisualID)fbvid != vid) continue;
		glXGetFBConfigAttrib(dpy, cfgs[i], GLX_RENDER_TYPE, &rtype);
		if (!(rtype & GLX_RGBA_BIT)) continue;
		// Require a window-renderable config; a visual can also back
		// pixmap/pbuffer-only configs that would fail glXMakeCurrent.
		glXGetFBConfigAttrib(dpy, cfgs[i], GLX_DRAWABLE_TYPE, &dtype);
		if (!(dtype & GLX_WINDOW_BIT)) continue;
		*out = cfgs[i];
		XFree(cfgs);
		return 0;
	}
	XFree(cfgs);
	return -1;
}

GLCtx* glCtxInit(void *display, unsigned long window, float dpiScale) {
	(void)dpiScale;
	Display *dpy = (Display *)display;
	Window   win = (Window)window;

	XWindowAttributes wa;
	if (!XGetWindowAttributes(dpy, win, &wa)) {
		fprintf(stderr, "gl: XGetWindowAttributes failed\n");
		return NULL;
	}
	int screen = XScreenNumberOfScreen(wa.screen);
	VisualID vid = XVisualIDFromVisual(wa.visual);

	GLXFBConfig fbc;
	if (findFBConfig(dpy, screen, vid, &fbc) != 0) {
		fprintf(stderr, "gl: no GLX FBConfig matches window visual\n");
		return NULL;
	}

	typedef GLXContext (*PFN_createCtx)(Display *, GLXFBConfig, GLXContext,
	                                    Bool, const int *);
	PFN_createCtx createCtx = (PFN_createCtx)glXGetProcAddressARB(
	    (const GLubyte *)"glXCreateContextAttribsARB");

	GLXContext glc = NULL;
	if (createCtx) {
		int attribs[] = {
			GLX_CONTEXT_MAJOR_VERSION_ARB, 3,
			GLX_CONTEXT_MINOR_VERSION_ARB, 3,
			GLX_CONTEXT_PROFILE_MASK_ARB,
			GLX_CONTEXT_CORE_PROFILE_BIT_ARB,
			0
		};
		glc = createCtx(dpy, fbc, NULL, True, attribs);
	}
	if (!glc) {
		glc = glXCreateNewContext(dpy, fbc, GLX_RGBA_TYPE, NULL, True);
	}
	if (!glc) {
		fprintf(stderr, "gl: glXCreateContext failed\n");
		return NULL;
	}
	if (!glXMakeCurrent(dpy, win, glc)) {
		fprintf(stderr, "gl: glXMakeCurrent failed\n");
		glXDestroyContext(dpy, glc);
		return NULL;
	}

	// Enable vsync when the extension is available.
	typedef void (*PFN_swapInterval)(Display *, GLXDrawable, int);
	PFN_swapInterval swapInterval = (PFN_swapInterval)glXGetProcAddressARB(
	    (const GLubyte *)"glXSwapIntervalEXT");
	if (swapInterval) swapInterval(dpy, win, 1);

	if (loadGL() != 0) {
		fprintf(stderr, "gl: failed to load GL functions\n");
		glXMakeCurrent(dpy, None, NULL);
		glXDestroyContext(dpy, glc);
		return NULL;
	}

	GLuint prog = buildProgram();
	if (!prog) {
		glXMakeCurrent(dpy, None, NULL);
		glXDestroyContext(dpy, glc);
		return NULL;
	}

	GLCtx *ctx = (GLCtx *)calloc(1, sizeof(GLCtx));
	ctx->dpy     = dpy;
	ctx->win     = win;
	ctx->glc     = glc;
	ctx->program = prog;
	ctx->uProj   = p_glGetUniformLocation(prog, "uProj");
	ctx->uTex    = p_glGetUniformLocation(prog, "uTex");

	// VAO + VBO
	p_glGenVertexArrays(1, &ctx->vao);
	p_glBindVertexArray(ctx->vao);
	p_glGenBuffers(1, &ctx->vbo);
	p_glBindBuffer(GL_ARRAY_BUFFER, ctx->vbo);

	// Vertex layout: 20 bytes per vertex.
	// attr 0: 2×float pos @ offset 0
	p_glEnableVertexAttribArray(0);
	p_glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 20, (void*)0);
	// attr 1: 4×ubyte color @ offset 8 (normalized)
	p_glEnableVertexAttribArray(1);
	p_glVertexAttribPointer(1, 4, GL_UNSIGNED_BYTE, GL_TRUE, 20, (void*)8);
	// attr 2: 2×float texcoord @ offset 12
	p_glEnableVertexAttribArray(2);
	p_glVertexAttribPointer(2, 2, GL_FLOAT, GL_FALSE, 20, (void*)12);

	// 1×1 white texture for DrawFilledRect.
	glGenTextures(1, &ctx->whiteTex);
	glBindTexture(GL_TEXTURE_2D, ctx->whiteTex);
	unsigned char white[4] = {255, 255, 255, 255};
	glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, 1, 1, 0,
	             GL_RGBA, GL_UNSIGNED_BYTE, white);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);

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
	// Reject non-positive dimensions: a negative size wraps through
	// GLsizei into a huge glTexImage2D allocation.
	if (w <= 0 || h <= 0) return 0;
	ctx->nextTexID++;
	uint64_t tid = ctx->nextTexID;
	ensureTexSlot(ctx, tid);

	GLuint tex;
	glGenTextures(1, &tex);
	glBindTexture(GL_TEXTURE_2D, tex);
	glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, w, h, 0,
	             GL_RGBA, GL_UNSIGNED_BYTE, NULL);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);

	ctx->texSlots[tid].glTex = tex;
	ctx->texSlots[tid].used  = 1;
	return tid;
}

void glCtxUpdateTex(GLCtx *ctx, uint64_t tid,
                    void *data, int w, int h) {
	if ((int)tid >= ctx->texCap || !ctx->texSlots[tid].used) return;
	glBindTexture(GL_TEXTURE_2D, ctx->texSlots[tid].glTex);
	glPixelStorei(GL_UNPACK_ROW_LENGTH, 0);
	glTexSubImage2D(GL_TEXTURE_2D, 0, 0, 0, w, h,
	                GL_RGBA, GL_UNSIGNED_BYTE, data);
}

void glCtxDeleteTex(GLCtx *ctx, uint64_t tid) {
	if ((int)tid >= ctx->texCap || !ctx->texSlots[tid].used) return;
	glDeleteTextures(1, &ctx->texSlots[tid].glTex);
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
	glCtxGetDrawableSize(ctx, &physW, &physH);
	if (physW == 0 || physH == 0) return -1;
	// Guard the orthographic projection against a zero/negative logical
	// size, which would divide by zero and yield an Inf matrix.
	if (logicalW <= 0 || logicalH <= 0) return -1;

	glViewport(0, 0, physW, physH);
	glClearColor(clearR, clearG, clearB, clearA);
	glClear(GL_COLOR_BUFFER_BIT);

	glEnable(GL_BLEND);
	glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);

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
	p_glUniformMatrix4fv(ctx->uProj, 1, GL_FALSE, proj);

	// Upload vertex data.
	p_glBindVertexArray(ctx->vao);
	p_glBindBuffer(GL_ARRAY_BUFFER, ctx->vbo);
	if (vertCount > 0 && verts) {
		p_glBufferData(GL_ARRAY_BUFFER,
		               (ptrdiff_t)vertCount * 20,
		               verts, GL_DYNAMIC_DRAW);
	}

	// Set texture unit 0.
	p_glActiveTexture(GL_TEXTURE0);
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
			glBindTexture(GL_TEXTURE_2D, tex);
			glDrawArrays(GL_TRIANGLES, dc->firstVert, dc->vertCount);
		}
	}

	glXSwapBuffers(ctx->dpy, ctx->win);
	return 0;
}

void glCtxDestroy(GLCtx *ctx) {
	if (!ctx) return;
	// Delete user textures.
	for (int i = 0; i < ctx->texCap; i++) {
		if (ctx->texSlots[i].used) {
			glDeleteTextures(1, &ctx->texSlots[i].glTex);
		}
	}
	free(ctx->texSlots);
	if (ctx->whiteTex) glDeleteTextures(1, &ctx->whiteTex);
	if (ctx->vbo)      p_glDeleteBuffers(1, &ctx->vbo);
	if (ctx->vao)      p_glDeleteVertexArrays(1, &ctx->vao);
	if (ctx->program)  p_glDeleteProgram(ctx->program);
	if (ctx->glc) {
		glXMakeCurrent(ctx->dpy, None, NULL);
		glXDestroyContext(ctx->dpy, ctx->glc);
	}
	free(ctx);
}

void glCtxGetDrawableSize(GLCtx *ctx, int *w, int *h) {
	if (!ctx) { *w = 0; *h = 0; return; }
	Window root;
	int x, y;
	unsigned int ww, hh, bw, depth;
	if (!XGetGeometry(ctx->dpy, ctx->win, &root, &x, &y,
	                  &ww, &hh, &bw, &depth)) {
		*w = 0; *h = 0; return;
	}
	*w = (int)ww;
	*h = (int)hh;
}
