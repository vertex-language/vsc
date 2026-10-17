package gpu

// Rendering: textures, libraries compiled from Metal source while the
// program runs, render pipelines and passes. What gpu/raster draws with
// on a GPU. Metal only: on the CPU device these make nothing (nil, or a
// RenderError), and callers draw on the CPU instead.
//
// Values that name Metal's own enums (blend factors, compare functions,
// vertex formats) are passed as Metal numbers them; gpu/raster maps GL's
// and its own to them.

@_silgen_name("vertex_gpu_texture_create") func _textureCreate(_ d: UnsafeMutableRawPointer, _ kind: int, _ format: int, _ w: int, _ h: int, _ levels: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_texture_release") func _textureRelease(_ t: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_texture_write") func _textureWrite(_ t: UnsafeMutableRawPointer, _ level: int, _ slice: int, _ x: int, _ y: int, _ w: int, _ h: int, _ bytes: UnsafeRawPointer, _ bytesPerRow: int)
@_silgen_name("vertex_gpu_texture_read") func _textureRead(_ t: UnsafeMutableRawPointer, _ level: int, _ slice: int, _ x: int, _ y: int, _ w: int, _ h: int, _ bytes: UnsafeMutableRawPointer, _ bytesPerRow: int)
@_silgen_name("vertex_gpu_library_source") func _librarySource(_ d: UnsafeMutableRawPointer, _ src: UnsafePointer<uint8>, _ n: int, _ err: UnsafeMutablePointer<uint8>, _ cap: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_library_function") func _libraryFunction(_ lib: UnsafeMutableRawPointer, _ name: UnsafePointer<uint8>, _ n: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_object_release") func _objectRelease(_ o: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_render_pipeline") func _renderPipeline(_ d: UnsafeMutableRawPointer, _ v: UnsafeMutableRawPointer, _ f: UnsafeMutableRawPointer, _ desc: UnsafePointer<int>, _ err: UnsafeMutablePointer<uint8>, _ cap: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_depth_stencil_state") func _depthStencilState(_ d: UnsafeMutableRawPointer, _ desc: UnsafePointer<int>) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_sampler_state") func _samplerState(_ d: UnsafeMutableRawPointer, _ desc: UnsafePointer<int>, _ lods: UnsafePointer<float32>) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_render_begin") func _renderBegin(_ d: UnsafeMutableRawPointer, _ color: UnsafeMutableRawPointer?, _ level: int, _ slice: int, _ colorLoad: int, _ clear: UnsafePointer<float32>, _ depth: UnsafeMutableRawPointer?, _ depthLoad: int, _ clearDepth: float32, _ stencilLoad: int, _ clearStencil: int) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_render_end") func _renderEnd(_ e: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_render_set_pipeline") func _renderSetPipeline(_ e: UnsafeMutableRawPointer, _ p: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_render_set_depth_stencil") func _renderSetDepthStencil(_ e: UnsafeMutableRawPointer, _ s: UnsafeMutableRawPointer, _ front: int, _ back: int)
@_silgen_name("vertex_gpu_render_set_viewport") func _renderSetViewport(_ e: UnsafeMutableRawPointer, _ x: float32, _ y: float32, _ w: float32, _ h: float32, _ n: float32, _ f: float32)
@_silgen_name("vertex_gpu_render_set_scissor") func _renderSetScissor(_ e: UnsafeMutableRawPointer, _ x: int, _ y: int, _ w: int, _ h: int)
@_silgen_name("vertex_gpu_render_set_raster") func _renderSetRaster(_ e: UnsafeMutableRawPointer, _ cull: int, _ winding: int, _ bias: float32, _ slope: float32)
@_silgen_name("vertex_gpu_render_set_blend_color") func _renderSetBlendColor(_ e: UnsafeMutableRawPointer, _ r: float32, _ g: float32, _ b: float32, _ a: float32)
@_silgen_name("vertex_gpu_render_set_bytes") func _renderSetBytes(_ e: UnsafeMutableRawPointer, _ stage: int, _ p: UnsafeRawPointer, _ n: int, _ index: int)
@_silgen_name("vertex_gpu_render_set_texture") func _renderSetTexture(_ e: UnsafeMutableRawPointer, _ stage: int, _ t: UnsafeMutableRawPointer?, _ index: int)
@_silgen_name("vertex_gpu_render_set_sampler") func _renderSetSampler(_ e: UnsafeMutableRawPointer, _ stage: int, _ s: UnsafeMutableRawPointer, _ index: int)
@_silgen_name("vertex_gpu_render_draw") func _renderDraw(_ e: UnsafeMutableRawPointer, _ prim: int, _ start: int, _ count: int, _ instances: int)
@_silgen_name("vertex_gpu_render_draw_indexed") func _renderDrawIndexed(_ e: UnsafeMutableRawPointer, _ prim: int, _ count: int, _ indexSize: int, _ indices: UnsafeRawPointer, _ n: int, _ instances: int)
@_silgen_name("vertex_gpu_render_set_buffer") func _renderSetBuffer(_ e: UnsafeMutableRawPointer, _ stage: int, _ b: UnsafeMutableRawPointer, _ offset: int, _ index: int)
@_silgen_name("vertex_gpu_render_draw_indexed_buffer") func _renderDrawIndexedBuffer(_ e: UnsafeMutableRawPointer, _ prim: int, _ count: int, _ indexSize: int, _ b: UnsafeMutableRawPointer, _ offset: int, _ instances: int)
@_silgen_name("vertex_gpu_render_commit") func _renderCommit(_ d: UnsafeMutableRawPointer, _ wait: int) -> int32
@_silgen_name("vertex_gpu_texture_upload") func _textureUpload(_ t: UnsafeMutableRawPointer, _ level: int, _ slice: int, _ x: int, _ y: int, _ w: int, _ h: int, _ bytes: UnsafeRawPointer, _ bytesPerRow: int)
@_silgen_name("vertex_gpu_layer_attach") func _layerAttach(_ d: UnsafeMutableRawPointer, _ view: UnsafeMutableRawPointer, _ w: int, _ h: int, _ scale: float64) -> UnsafeMutableRawPointer?
@_silgen_name("vertex_gpu_layer_resize") func _layerResize(_ l: UnsafeMutableRawPointer, _ w: int, _ h: int, _ scale: float64)
@_silgen_name("vertex_gpu_layer_present") func _layerPresent(_ l: UnsafeMutableRawPointer, _ scene: UnsafeMutableRawPointer, _ overlay: UnsafeMutableRawPointer?, _ params: UnsafePointer<float32>) -> int32
@_silgen_name("vertex_gpu_layer_release") func _layerRelease(_ l: UnsafeMutableRawPointer)
@_silgen_name("vertex_gpu_layer_capture") func _layerCapture(_ l: UnsafeMutableRawPointer, _ scene: UnsafeMutableRawPointer, _ overlay: UnsafeMutableRawPointer?, _ params: UnsafePointer<float32>, _ into: UnsafeMutableRawPointer) -> int32

/// RenderError is what making a rendering object can fail with.
public enum RenderError: Error {
    /// unsupported: the device has no render pipeline (the CPU).
    case unsupported
    /// compile: Metal source that didn't compile, with the compiler's log.
    case compile(string)
    /// pipeline: a pipeline Metal refused, with its reason.
    case pipeline(string)
    /// noFunction: the library has no function of that name.
    case noFunction(string)
}

/// How a texture's texels are stored on the device.
public enum TextureFormat: Equatable {
    case rgba8
    case rgba32f
    /// 32-bit float depth with 8-bit stencil; device memory only.
    case depthStencil

    var code: int {
        switch self {
        case .rgba8: return 0
        case .rgba32f: return 1
        case .depthStencil: return 2
        }
    }
}

/// A texture in device memory: 2D or a cube, with mip levels.
public final class Texture {
    public let _h: UnsafeMutableRawPointer
    public let Width: int
    public let Height: int
    public let Format: TextureFormat
    public let Levels: int
    public let Cube: bool

    /// A texture on `device`, or nil where the device can't make one.
    /// (A factory, not init?: failable class initializers aren't lowered yet.)
    public static func Create(device: Device, width: int, height: int, format: TextureFormat, levels: int = 1, cube: bool = false) -> Texture? {
        guard let h = _textureCreate(device._h, cube ? 1 : 0, format.code, width, height, levels) else { return nil }
        return Texture(_h: h, width: width, height: height, format: format, levels: levels, cube: cube)
    }

    init(_h: UnsafeMutableRawPointer, width: int, height: int, format: TextureFormat, levels: int, cube: bool) {
        self._h = _h
        Width = width
        Height = height
        Format = format
        Levels = levels
        Cube = cube
    }

    deinit {
        _textureRelease(_h)
    }

    /// Writes host bytes into a region of a level (and cube face).
    public func Write(level: int = 0, slice: int = 0, x: int, y: int, width: int, height: int, bytes: UnsafeRawPointer, bytesPerRow: int) {
        _textureWrite(_h, level, slice, x, y, width, height, bytes, bytesPerRow)
    }

    /// Reads a region of a level into host bytes, after everything drawn so far.
    public func Read(level: int = 0, slice: int = 0, x: int, y: int, width: int, height: int, into bytes: UnsafeMutableRawPointer, bytesPerRow: int) {
        _textureRead(_h, level, slice, x, y, width, height, bytes, bytesPerRow)
    }

    /// Upload writes host bytes into a region of a level without waiting:
    /// they're copied now and land on the texture after everything already
    /// submitted and before anything submitted later (a pass still open
    /// sees the new texels). Write waits for the GPU first.
    public func Upload(level: int = 0, slice: int = 0, x: int, y: int, width: int, height: int, bytes: UnsafeRawPointer, bytesPerRow: int) {
        _textureUpload(_h, level, slice, x, y, width, height, bytes, bytesPerRow)
    }
}

/// Backdrop is a frosted panel a Layer draws under its overlay: the
/// frame blurred and lifted inside a rounded rectangle (drawable pixels),
/// the rest dimmed, by Strength (0 none, 1 full).
public struct Backdrop {
    public var X: float32
    public var Y: float32
    public var Width: float32
    public var Height: float32
    public var Radius: float32
    public var Strength: float32

    public init(x: float32, y: float32, width: float32, height: float32, radius: float32, strength: float32) {
        X = x
        Y = y
        Width = width
        Height = height
        Radius = radius
        Strength = strength
    }
}

/// Layer is a window's surface on the GPU: frames drawn on the device are
/// presented where they were drawn, with no copy back to the host. It's
/// made over a native view (an NSView on macOS: window.Surface.RawParts),
/// and each Present draws a scene texture scaled to the window, an
/// optional frosted backdrop, and an optional overlay (premultiplied
/// RGBA: a HUD drawn on the host and uploaded) over it.
public final class Layer {
    let _h: UnsafeMutableRawPointer
    let device: Device
    public private(set) var Width: int
    public private(set) var Height: int

    init(_h: UnsafeMutableRawPointer, device: Device, width: int, height: int) {
        self._h = _h
        self.device = device
        Width = width
        Height = height
    }

    /// Attach lays a layer over `view` for frames width × height pixels at
    /// the view's backing scale; nil off Metal.
    public static func Attach(device: Device, view: uint64, width: int, height: int, scale: float64) -> Layer? {
        guard let v = UnsafeMutableRawPointer(bitPattern: UInt(view)) else { return nil }
        guard let h = _layerAttach(device._h, v, width, height, scale) else { return nil }
        return Layer(_h: h, device: device, width: width, height: height)
    }

    /// Resize sets the frames' size in pixels (the window's, as it changes).
    public func Resize(width: int, height: int, scale: float64) {
        if width == Width && height == Height { return }
        Width = width
        Height = height
        _layerResize(_h, width, height, scale)
    }

    /// Present shows a frame; false when the window had no drawable to give.
    @discardableResult
    public func Present(_ scene: Texture, overlay: Texture? = nil, backdrop: Backdrop? = nil) -> bool {
        let params = parameters(overlay != nil, backdrop)
        let rc = params.withUnsafeBufferPointer { p in _layerPresent(_h, scene._h, overlay?._h, p.baseAddress!) }
        return rc == 0
    }

    /// Capture is what Present would show, read back: RGBA, top row first,
    /// Width × Height (a screenshot of the window). Nil off Metal.
    public func Capture(_ scene: Texture, overlay: Texture? = nil, backdrop: Backdrop? = nil) -> [uint8]? {
        guard let into = Texture.Create(device: device, width: Width, height: Height, format: .rgba8) else { return nil }
        let params = parameters(overlay != nil, backdrop)
        let rc = params.withUnsafeBufferPointer { p in _layerCapture(_h, scene._h, overlay?._h, p.baseAddress!, into._h) }
        if rc != 0 { return nil }
        var out = [uint8](repeating: 0, count: Width * Height * 4)
        out.withUnsafeMutableBytes { p in into.Read(x: 0, y: 0, width: Width, height: Height, into: p.baseAddress!, bytesPerRow: Width * 4) }
        return out
    }

    func parameters(_ overlay: bool, _ backdrop: Backdrop?) -> [float32] {
        let b = backdrop ?? Backdrop(x: 0, y: 0, width: 0, height: 0, radius: 0, strength: 0)
        return [b.X, b.Y, b.Width, b.Height, b.Radius, b.Strength, overlay ? 1 : 0, 0, float32(Width), float32(Height), 0, 0]
    }

    deinit {
        _layerRelease(_h)
    }
}

/// Shader functions compiled from Metal source while the program runs.
public final class Library {
    public let _h: UnsafeMutableRawPointer

    public init(device: Device, source: string) throws {
        if device.IsCPU { throw RenderError.unsupported }
        let src = Array(source.utf8)
        var err = [uint8](repeating: 0, count: 4096)
        var made: UnsafeMutableRawPointer? = nil
        src.withUnsafeBufferPointer { s in
            err.withUnsafeMutableBufferPointer { e in
                made = _librarySource(device._h, s.baseAddress!, src.count, e.baseAddress!, 4096)
            }
        }
        guard let h = made else {
            var n = 0
            while n < err.count && err[n] != 0 { n += 1 }
            throw RenderError.compile(string(decoding: err[0..<n], as: UTF8.self))
        }
        _h = h
    }

    deinit {
        _objectRelease(_h)
    }

    /// The function of this name.
    public func Function(_ name: string) throws -> ShaderFunction {
        let n = Array(name.utf8)
        var made: UnsafeMutableRawPointer? = nil
        n.withUnsafeBufferPointer { p in made = _libraryFunction(_h, p.baseAddress!, n.count) }
        guard let h = made else { throw RenderError.noFunction(name) }
        return ShaderFunction(_h: h)
    }
}

/// One function of a library: a vertex or fragment stage.
public final class ShaderFunction {
    public let _h: UnsafeMutableRawPointer

    public init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    deinit {
        _objectRelease(_h)
    }
}

/// How a pipeline blends, writes and reads vertices; numbers are Metal's.
public struct PipelineDescriptor {
    /// The color target's format; nil when there's none.
    public var Color: TextureFormat? = .rgba8
    /// Whether there is a depth-stencil target.
    public var DepthStencil = false
    public var Blending = false
    public var RGBOperation = 0
    public var AlphaOperation = 0
    public var SourceRGB = 1
    public var DestinationRGB = 0
    public var SourceAlpha = 1
    public var DestinationAlpha = 0
    /// MTLColorWriteMask: red 8, green 4, blue 2, alpha 1.
    public var WriteMask = 15
    public var Attributes: [VertexAttribute] = []
    public var Layouts: [VertexLayout] = []

    public init() {}
}

/// Where a vertex attribute comes from: its buffer slot, offset and MTLVertexFormat.
public struct VertexAttribute {
    public let Index: int
    public let Format: int
    public let Offset: int
    public let Buffer: int

    public init(index: int, format: int, offset: int, buffer: int) {
        Index = index
        Format = format
        Offset = offset
        Buffer = buffer
    }
}

/// How a vertex buffer slot steps: stride, MTLVertexStepFunction (0 constant, 1 per vertex, 2 per instance) and rate.
public struct VertexLayout {
    public let Buffer: int
    public let Stride: int
    public let Step: int
    public let Rate: int

    public init(buffer: int, stride: int, step: int, rate: int) {
        Buffer = buffer
        Stride = stride
        Step = step
        Rate = rate
    }
}

/// A render pipeline: a vertex and fragment function with fixed state.
public final class RenderPipeline {
    public let _h: UnsafeMutableRawPointer

    public init(device: Device, vertex: ShaderFunction, fragment: ShaderFunction, _ d: PipelineDescriptor) throws {
        var desc: [int] = [d.Color == nil ? -1 : d.Color!.code, d.DepthStencil ? 2 : -1, d.Blending ? 1 : 0,
                           d.RGBOperation, d.AlphaOperation, d.SourceRGB, d.DestinationRGB, d.SourceAlpha, d.DestinationAlpha,
                           d.WriteMask, d.Attributes.count]
        for a in d.Attributes { desc += [a.Index, a.Format, a.Offset, a.Buffer] }
        desc.append(d.Layouts.count)
        for l in d.Layouts { desc += [l.Buffer, l.Stride, l.Step, l.Rate] }
        var err = [uint8](repeating: 0, count: 4096)
        var made: UnsafeMutableRawPointer? = nil
        desc.withUnsafeBufferPointer { p in
            err.withUnsafeMutableBufferPointer { e in
                made = _renderPipeline(device._h, vertex._h, fragment._h, p.baseAddress!, e.baseAddress!, 4096)
            }
        }
        guard let h = made else {
            var n = 0
            while n < err.count && err[n] != 0 { n += 1 }
            throw RenderError.pipeline(string(decoding: err[0..<n], as: UTF8.self))
        }
        _h = h
    }

    deinit {
        _objectRelease(_h)
    }
}

/// One face's stencil test; numbers are Metal's (MTLCompareFunction, MTLStencilOperation).
public struct StencilDescriptor {
    public var Compare = 7
    public var Fail = 0
    public var DepthFail = 0
    public var Pass = 0
    public var ReadMask = 0xFF
    public var WriteMask = 0xFF

    public init() {}
}

/// Depth and stencil tests.
public final class DepthStencilState {
    public let _h: UnsafeMutableRawPointer

    public static func Create(device: Device, compare: int, write: bool, stencil: bool, front: StencilDescriptor, back: StencilDescriptor) -> DepthStencilState? {
        var desc: [int] = [compare, write ? 1 : 0, stencil ? 1 : 0]
        for s in [front, back] { desc += [s.Compare, s.Fail, s.DepthFail, s.Pass, s.ReadMask, s.WriteMask] }
        var made: UnsafeMutableRawPointer? = nil
        desc.withUnsafeBufferPointer { p in made = _depthStencilState(device._h, p.baseAddress!) }
        guard let h = made else { return nil }
        return DepthStencilState(_h: h)
    }

    init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    deinit {
        _objectRelease(_h)
    }
}

/// How a texture is sampled; numbers are Metal's.
public final class SamplerState {
    public let _h: UnsafeMutableRawPointer

    /// filters: MTLSamplerMinMagFilter (0 nearest, 1 linear), mip: MTLSamplerMipFilter (0 none, 1 nearest, 2 linear),
    /// address: MTLSamplerAddressMode (0 clamp to edge, 2 repeat, 3 mirror repeat), compare: MTLCompareFunction or -1.
    public static func Create(device: Device, min: int, mag: int, mip: int, s: int, t: int, r: int, compare: int, minLod: float32, maxLod: float32) -> SamplerState? {
        let desc: [int] = [min, mag, mip, s, t, r, compare]
        let lods: [float32] = [minLod, maxLod]
        var made: UnsafeMutableRawPointer? = nil
        desc.withUnsafeBufferPointer { p in
            lods.withUnsafeBufferPointer { l in made = _samplerState(device._h, p.baseAddress!, l.baseAddress!) }
        }
        guard let h = made else { return nil }
        return SamplerState(_h: h)
    }

    init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    deinit {
        _objectRelease(_h)
    }
}

/// A render pass: draws into a color and depth-stencil target, encoded
/// into the device's open command buffer until End. Device.Commit runs them.
public final class RenderPass {
    public let _h: UnsafeMutableRawPointer
    var ended = false

    /// A pass into `color` (a level and face) and `depth`. With clear set, a target starts as it says; else as it was.
    public static func Begin(device: Device, color: Texture?, level: int = 0, slice: int = 0, clearColor: [float32]? = nil,
                             depth: Texture?, clearDepth: float32? = nil, clearStencil: int? = nil) -> RenderPass? {
        let cc: [float32] = clearColor ?? [0, 0, 0, 0]
        var made: UnsafeMutableRawPointer? = nil
        cc.withUnsafeBufferPointer { p in
            made = _renderBegin(device._h, color?._h, level, slice, clearColor != nil ? 1 : 0, p.baseAddress!,
                                depth?._h, clearDepth != nil ? 1 : 0, clearDepth ?? 1, clearStencil != nil ? 1 : 0, clearStencil ?? 0)
        }
        guard let h = made else { return nil }
        return RenderPass(_h: h)
    }

    init(_h: UnsafeMutableRawPointer) {
        self._h = _h
    }

    deinit {
        End()
    }

    public func End() {
        if ended { return }
        ended = true
        _renderEnd(_h)
    }

    public func SetPipeline(_ p: RenderPipeline) { _renderSetPipeline(_h, p._h) }
    public func SetDepthStencil(_ s: DepthStencilState, front: int, back: int) { _renderSetDepthStencil(_h, s._h, front, back) }
    /// In pixels from the target's top-left corner, with the depth range.
    public func SetViewport(x: float32, y: float32, width: float32, height: float32, near: float32, far: float32) {
        _renderSetViewport(_h, x, y, width, height, near, far)
    }
    public func SetScissor(x: int, y: int, width: int, height: int) { _renderSetScissor(_h, x, y, width, height) }
    /// cull: 0 none, 1 front, 2 back; counterClockwise: which winding faces front; depth bias and slope.
    public func SetRaster(cull: int, counterClockwise: bool, bias: float32, slope: float32) {
        _renderSetRaster(_h, cull, counterClockwise ? 1 : 0, bias, slope)
    }
    public func SetBlendColor(_ r: float32, _ g: float32, _ b: float32, _ a: float32) { _renderSetBlendColor(_h, r, g, b, a) }
    /// Bytes for a buffer slot of the vertex (fragment: false) or fragment stage.
    public func SetBytes(fragment: bool, _ p: UnsafeRawPointer, count: int, index: int) { _renderSetBytes(_h, fragment ? 1 : 0, p, count, index) }
    /// A device buffer's bytes from `offset` for a buffer slot: vertices
    /// sent once and drawn many times.
    public func SetBuffer(fragment: bool, _ b: Buffer<uint8>, offset: int, index: int) {
        _renderSetBuffer(_h, fragment ? 1 : 0, b._memory._h, b._offset + offset, index)
    }
    public func SetTexture(fragment: bool, _ t: Texture?, index: int) { _renderSetTexture(_h, fragment ? 1 : 0, t?._h, index) }
    public func SetSampler(fragment: bool, _ s: SamplerState, index: int) { _renderSetSampler(_h, fragment ? 1 : 0, s._h, index) }
    /// primitive: 0 points, 1 lines, 2 line strip, 3 triangles, 4 triangle strip.
    public func Draw(primitive: int, start: int, count: int, instances: int) { _renderDraw(_h, primitive, start, count, instances) }
    /// Indexed by 2- or 4-byte indices, from host memory.
    public func DrawIndexed(primitive: int, count: int, indexSize: int, indices: UnsafeRawPointer, bytes: int, instances: int) {
        _renderDrawIndexed(_h, primitive, count, indexSize, indices, bytes, instances)
    }
    /// Indexed by 2- or 4-byte indices in a device buffer, from `offset` bytes.
    public func DrawIndexed(primitive: int, count: int, indexSize: int, buffer: Buffer<uint8>, offset: int, instances: int) {
        _renderDrawIndexedBuffer(_h, primitive, count, indexSize, buffer._memory._h, buffer._offset + offset, instances)
    }
}

extension Device {
    /// Runs the render passes encoded so far; with wait, until they're done.
    public func Commit(wait: bool) throws {
        let r = _renderCommit(_h, wait ? 1 : 0)
        if r != 0 { throw KernelError.failed(r) }
    }
}
