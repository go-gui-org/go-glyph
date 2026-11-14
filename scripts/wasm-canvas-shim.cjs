// Canvas2D stand-in for running the js/wasm tests under node.
//
// node has no DOM, so NewContext (context_wasm.go) falls back to
// OffscreenCanvas. This file defines it before the Go test binary starts
// (NODE_OPTIONS=--require). Metrics are fixed and simple: every code point
// is 0.6em wide, ascent is 0.8em and descent is 0.2em. Tests check layout
// logic, not real font metrics.
"use strict";

class Ctx2D {
  constructor() {
    this.font = "10px sans-serif";
  }
  px() {
    const m = /(\d+(?:\.\d+)?)px/.exec(this.font);
    return m ? parseFloat(m[1]) : 10;
  }
  measureText(s) {
    const px = this.px();
    const w = [...String(s)].length * px * 0.6;
    return {
      width: w,
      fontBoundingBoxAscent: px * 0.8,
      fontBoundingBoxDescent: px * 0.2,
      actualBoundingBoxAscent: px * 0.7,
      actualBoundingBoxDescent: px * 0.1,
      actualBoundingBoxLeft: 0,
      actualBoundingBoxRight: w,
    };
  }
}

globalThis.OffscreenCanvas = class {
  constructor(w, h) {
    this.width = w;
    this.height = h;
  }
  getContext() {
    return new Ctx2D();
  }
};
