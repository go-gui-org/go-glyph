//go:build !ios

#include "metal_darwin.h"

#import <Metal/Metal.h>
#import <QuartzCore/CAMetalLayer.h>

// MetalCtx holds all Metal state.
struct MetalCtx {
	id<MTLDevice>              device;
	id<MTLCommandQueue>        queue;
	id<MTLRenderPipelineState> pipeline;
	id<MTLSamplerState>        sampler;
	CAMetalLayer              *layer;
	NSMutableDictionary       *textures; // NSNumber(uint64) -> id<MTLTexture>
	id<MTLTexture>             whiteTex;
	SDL_MetalView              view;
	uint64_t                   nextTexID;
};

// MSL shader source.
static const char *shaderSource =
	"#include <metal_stdlib>\n"
	"using namespace metal;\n"
	"struct VertexIn {\n"
	"    float2 position [[attribute(0)]];\n"
	"    uchar4 color    [[attribute(1)]];\n"
	"    float2 texcoord [[attribute(2)]];\n"
	"};\n"
	"struct VertexOut {\n"
	"    float4 position [[position]];\n"
	"    float4 color;\n"
	"    float2 texcoord;\n"
	"};\n"
	"vertex VertexOut vs(VertexIn in [[stage_in]],\n"
	"                    constant float4x4& proj [[buffer(1)]]) {\n"
	"    VertexOut out;\n"
	"    out.position = proj * float4(in.position, 0, 1);\n"
	"    out.color = float4(in.color) / 255.0;\n"
	"    out.texcoord = in.texcoord;\n"
	"    return out;\n"
	"}\n"
	"fragment float4 fs(VertexOut in [[stage_in]],\n"
	"                   texture2d<float> tex [[texture(0)]],\n"
	"                   sampler smp [[sampler(0)]]) {\n"
	"    return tex.sample(smp, in.texcoord) * in.color;\n"
	"}\n";

MetalCtx* metalInit(void *sdlWindow, float dpiScale) {
	MetalCtx *ctx = (MetalCtx *)calloc(1, sizeof(MetalCtx));
	if (!ctx) return NULL;

	SDL_Window *win = (SDL_Window *)sdlWindow;

	// Create Metal view from SDL window.
	ctx->view = SDL_Metal_CreateView(win);
	if (!ctx->view) {
		free(ctx);
		return NULL;
	}

	ctx->layer = (__bridge CAMetalLayer *)SDL_Metal_GetLayer(ctx->view);
	ctx->layer.presentsWithTransaction = YES;

	ctx->device = MTLCreateSystemDefaultDevice();
	ctx->layer.device = ctx->device;
	ctx->layer.pixelFormat = MTLPixelFormatBGRA8Unorm;

	ctx->queue = [ctx->device newCommandQueue];

	// Compile shaders.
	NSError *err = nil;
	NSString *src = [NSString stringWithUTF8String:shaderSource];
	id<MTLLibrary> lib = [ctx->device newLibraryWithSource:src
	                                               options:nil
	                                                 error:&err];
	if (!lib) {
		NSLog(@"metalInit: shader compile failed: %@", err);
		SDL_Metal_DestroyView(ctx->view);
		free(ctx);
		return NULL;
	}
	id<MTLFunction> vertFunc = [lib newFunctionWithName:@"vs"];
	id<MTLFunction> fragFunc = [lib newFunctionWithName:@"fs"];

	// Vertex descriptor: pos(float2), color(uchar4), texcoord(float2).
	MTLVertexDescriptor *vd = [MTLVertexDescriptor new];
	vd.attributes[0].format = MTLVertexFormatFloat2;
	vd.attributes[0].offset = 0;
	vd.attributes[0].bufferIndex = 0;
	vd.attributes[1].format = MTLVertexFormatUChar4;
	vd.attributes[1].offset = 8;
	vd.attributes[1].bufferIndex = 0;
	vd.attributes[2].format = MTLVertexFormatFloat2;
	vd.attributes[2].offset = 12;
	vd.attributes[2].bufferIndex = 0;
	vd.layouts[0].stride = 20;
	vd.layouts[0].stepFunction = MTLVertexStepFunctionPerVertex;

	MTLRenderPipelineDescriptor *pd = [MTLRenderPipelineDescriptor new];
	pd.vertexFunction = vertFunc;
	pd.fragmentFunction = fragFunc;
	pd.vertexDescriptor = vd;
	pd.colorAttachments[0].pixelFormat = MTLPixelFormatBGRA8Unorm;
	// Standard alpha blending.
	pd.colorAttachments[0].blendingEnabled = YES;
	pd.colorAttachments[0].sourceRGBBlendFactor = MTLBlendFactorSourceAlpha;
	pd.colorAttachments[0].destinationRGBBlendFactor = MTLBlendFactorOneMinusSourceAlpha;
	pd.colorAttachments[0].sourceAlphaBlendFactor = MTLBlendFactorSourceAlpha;
	pd.colorAttachments[0].destinationAlphaBlendFactor = MTLBlendFactorOneMinusSourceAlpha;

	ctx->pipeline = [ctx->device newRenderPipelineStateWithDescriptor:pd error:&err];
	if (!ctx->pipeline) {
		NSLog(@"metalInit: pipeline failed: %@", err);
		SDL_Metal_DestroyView(ctx->view);
		free(ctx);
		return NULL;
	}

	// Sampler: linear filtering, clamp to edge.
	MTLSamplerDescriptor *sd = [MTLSamplerDescriptor new];
	sd.minFilter = MTLSamplerMinMagFilterLinear;
	sd.magFilter = MTLSamplerMinMagFilterLinear;
	sd.sAddressMode = MTLSamplerAddressModeClampToEdge;
	sd.tAddressMode = MTLSamplerAddressModeClampToEdge;
	ctx->sampler = [ctx->device newSamplerStateWithDescriptor:sd];

	ctx->textures = [NSMutableDictionary new];

	// Create 1x1 white texture for DrawFilledRect.
	MTLTextureDescriptor *td = [MTLTextureDescriptor
		texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
		                             width:1
		                            height:1
		                         mipmapped:NO];
	td.usage = MTLTextureUsageShaderRead;
	ctx->whiteTex = [ctx->device newTextureWithDescriptor:td];
	uint8_t white[4] = {255, 255, 255, 255};
	[ctx->whiteTex replaceRegion:MTLRegionMake2D(0,0,1,1)
	                 mipmapLevel:0
	                   withBytes:white
	                 bytesPerRow:4];

	return ctx;
}

uint64_t metalNewTex(MetalCtx *ctx, int w, int h) {
	@autoreleasepool {
		MTLTextureDescriptor *td = [MTLTextureDescriptor
			texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
			                             width:w
			                            height:h
			                         mipmapped:NO];
		td.usage = MTLTextureUsageShaderRead;
		id<MTLTexture> tex = [ctx->device newTextureWithDescriptor:td];
		ctx->nextTexID++;
		uint64_t tid = ctx->nextTexID;
		ctx->textures[@(tid)] = tex;
		return tid;
	}
}

void metalUpdateTex(MetalCtx *ctx, uint64_t tid,
                    void *data, int w, int h) {
	@autoreleasepool {
		id<MTLTexture> tex = ctx->textures[@(tid)];
		if (!tex) return;
		[tex replaceRegion:MTLRegionMake2D(0, 0, w, h)
		       mipmapLevel:0
		         withBytes:data
		       bytesPerRow:w * 4];
	}
}

void metalDeleteTex(MetalCtx *ctx, uint64_t tid) {
	@autoreleasepool {
		[ctx->textures removeObjectForKey:@(tid)];
	}
}

int metalRender(MetalCtx *ctx,
                void *verts, int vertCount,
                void *cmds,  int cmdCount,
                float clearR, float clearG,
                float clearB, float clearA,
                int logicalW, int logicalH) {
	if (!ctx) return -1;

	// Wrap the entire frame in an autorelease pool. Without this,
	// per-frame autoreleased objects (vertex MTLBuffer, command
	// buffer, encoder, drawable, NSNumber boxes for @(tid)) pile
	// up forever — the Go-locked OS thread has no NSRunLoop to
	// drain the outer pool. ARC does not save us here.
	@autoreleasepool {

	// Read current drawable size.
	CGSize sz = ctx->layer.drawableSize;
	int physW = (int)sz.width;
	int physH = (int)sz.height;
	if (physW == 0 || physH == 0) return -1;

	id<CAMetalDrawable> drawable = [ctx->layer nextDrawable];
	if (!drawable) return -1;

	// Orthographic projection: logical coords -> NDC.
	float L = 0, R = (float)logicalW;
	float T = 0, B = (float)logicalH;
	float proj[16] = {
		2.0f/(R-L),     0,               0, 0,
		0,               2.0f/(T-B),     0, 0,
		0,               0,              -1, 0,
		-(R+L)/(R-L),   -(T+B)/(T-B),    0, 1,
	};

	id<MTLBuffer> vertBuf = nil;
	if (vertCount > 0 && verts) {
		vertBuf = [ctx->device newBufferWithBytes:verts
		                                   length:vertCount * 20
		                                  options:MTLResourceStorageModeShared];
	}

	MTLRenderPassDescriptor *rpd = [MTLRenderPassDescriptor new];
	rpd.colorAttachments[0].texture = drawable.texture;
	rpd.colorAttachments[0].loadAction = MTLLoadActionClear;
	rpd.colorAttachments[0].storeAction = MTLStoreActionStore;
	rpd.colorAttachments[0].clearColor =
		MTLClearColorMake(clearR, clearG, clearB, clearA);

	id<MTLCommandBuffer> cmdBuf = [ctx->queue commandBuffer];
	id<MTLRenderCommandEncoder> enc =
		[cmdBuf renderCommandEncoderWithDescriptor:rpd];

	[enc setRenderPipelineState:ctx->pipeline];
	[enc setFragmentSamplerState:ctx->sampler atIndex:0];
	[enc setVertexBytes:proj length:sizeof(proj) atIndex:1];

	MTLViewport vp = {0, 0, (double)physW, (double)physH, 0, 1};
	[enc setViewport:vp];

	if (vertBuf && cmdCount > 0) {
		[enc setVertexBuffer:vertBuf offset:0 atIndex:0];

		CDrawCmd *dcmds = (CDrawCmd *)cmds;
		for (int i = 0; i < cmdCount; i++) {
			CDrawCmd *dc = &dcmds[i];
			id<MTLTexture> tex = nil;
			if (dc->textureID == 0) {
				tex = ctx->whiteTex;
			} else {
				tex = ctx->textures[@(dc->textureID)];
			}
			if (!tex) tex = ctx->whiteTex;
			[enc setFragmentTexture:tex atIndex:0];
			[enc drawPrimitives:MTLPrimitiveTypeTriangle
			        vertexStart:dc->firstVert
			        vertexCount:dc->vertCount];
		}
	}

	[enc endEncoding];

	// Resize-sync: commit, wait, then present.
	[cmdBuf commit];
	[cmdBuf waitUntilScheduled];
	[drawable present];

	return 0;
	} // @autoreleasepool
}

void metalDestroy(MetalCtx *ctx) {
	if (!ctx) return;
	ctx->textures = nil;
	ctx->whiteTex = nil;
	ctx->pipeline = nil;
	ctx->sampler = nil;
	ctx->queue = nil;
	ctx->device = nil;
	if (ctx->view) {
		SDL_Metal_DestroyView(ctx->view);
	}
	free(ctx);
}

void metalGetDrawableSize(MetalCtx *ctx, int *w, int *h) {
	if (!ctx) { *w = 0; *h = 0; return; }
	CGSize sz = ctx->layer.drawableSize;
	*w = (int)sz.width;
	*h = (int)sz.height;
}

int metalWindowFlag(void) {
	return SDL_WINDOW_METAL;
}

void metalWindowDrawableSize(void *sdlWindow, int *w, int *h) {
	SDL_Metal_GetDrawableSize((SDL_Window *)sdlWindow, w, h);
}
