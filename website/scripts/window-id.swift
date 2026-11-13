// Prints the window number of the first on-screen window owned by the
// process named in argv[1], for screencapture -l.
import CoreGraphics
import Foundation

let owner = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "agio-demo"
let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
guard let windows = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] else {
    exit(1)
}
for window in windows {
    guard let name = window[kCGWindowOwnerName as String] as? String, name == owner else { continue }
    guard let number = window[kCGWindowNumber as String] as? Int else { continue }
    print(number)
    exit(0)
}
exit(1)
