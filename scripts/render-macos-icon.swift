// Build-time only: rasterize Ding's shared vector mark using native AppKit.
// Usage: xcrun swift scripts/render-macos-icon.swift MARK.svg OUTPUT.iconset
import AppKit
import Foundation

guard CommandLine.arguments.count == 3,
      let mark = NSImage(contentsOfFile: CommandLine.arguments[1]),
      mark.size.width > 0, mark.size.height > 0 else {
    fatalError("Expected the shared SVG mark and an output iconset directory")
}
let output = URL(fileURLWithPath: CommandLine.arguments[2], isDirectory: true)
try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)

for points in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let pixels = points * scale
        guard let bitmap = NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels,
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
            isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
        ), let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
            fatalError("Could not allocate the app icon")
        }
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = context
        context.imageInterpolation = .high
        let transform = NSAffineTransform()
        transform.scale(by: CGFloat(pixels) / 1024)
        transform.concat()
        // The same warm ivory backing used behind the bell in Ding Console.
        NSColor(srgbRed: 247 / 255, green: 246 / 255, blue: 242 / 255, alpha: 1).setFill()
        NSBezierPath(roundedRect: NSRect(x: 64, y: 64, width: 896, height: 896),
                     xRadius: 196, yRadius: 196).fill()
        let height = 740 * mark.size.height / mark.size.width
        mark.draw(in: NSRect(x: 142, y: (1024 - height) / 2, width: 740, height: height),
                  from: .zero, operation: .sourceOver, fraction: 1)
        NSGraphicsContext.restoreGraphicsState()
        guard let png = bitmap.representation(using: .png, properties: [:]) else {
            fatalError("Could not encode the app icon")
        }
        let suffix = scale == 2 ? "@2x" : ""
        try png.write(to: output.appendingPathComponent("icon_\(points)x\(points)\(suffix).png"))
    }
}
