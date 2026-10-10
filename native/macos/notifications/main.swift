import AppKit
import Foundation
import UserNotifications
import Darwin

struct Message: Decodable {
    let id: String
    let title: String
    let body: String
    let requestPermission: Bool
    let watchId: String?
    let stateDir: String?
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().delegate = self
        if CommandLine.arguments.dropFirst() == ["--deliver"] {
            Task { await deliver() }
        } else {
            // A notification click relaunches the helper without stdin. Wait for
            // the OS response callback instead of trying to decode an empty pipe.
            DispatchQueue.main.asyncAfter(deadline: .now() + 10) {
                NSApplication.shared.terminate(nil)
            }
        }
    }

    func deliver() async {
        do {
            let data = FileHandle.standardInput.readDataToEndOfFile()
            guard data.count <= 32768 else { throw Failure.invalidInput }
            let message = try JSONDecoder().decode(Message.self, from: data)
            guard !message.id.isEmpty, message.id.utf8.count <= 256,
                  message.title.utf8.count <= 512, message.body.utf8.count <= 4096
            else { throw Failure.invalidInput }
            let center = UNUserNotificationCenter.current()
            center.delegate = self
            if message.requestPermission {
                guard try await center.requestAuthorization(options: [.alert, .sound, .badge])
                else { throw Failure.permissionDenied }
            }
            let authorization: Int = await withCheckedContinuation { continuation in
                center.getNotificationSettings { settings in
                    continuation.resume(returning: settings.authorizationStatus.rawValue)
                }
            }
            guard authorization == UNAuthorizationStatus.authorized.rawValue || authorization == UNAuthorizationStatus.provisional.rawValue
            else { throw Failure.permissionDenied }
            let content = UNMutableNotificationContent()
            content.title = message.title
            content.body = message.body
            content.sound = .default
            if let state = message.stateDir, state.hasPrefix("/"), state.utf8.count <= 4096 {
                content.userInfo["dingStateDir"] = state
                if let watch = message.watchId { content.userInfo["dingWatchId"] = watch }
            }
            try await center.add(UNNotificationRequest(identifier: message.id, content: content, trigger: nil))
            FileHandle.standardOutput.write(Data("{\"accepted\":true}\n".utf8))
            NSApplication.shared.terminate(nil)
        } catch {
            FileHandle.standardError.write(Data("Ding notification unavailable; inspect OS notification permissions.\n".utf8))
            exit(1)
        }
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter,
                                willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list, .sound])
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter,
                                didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        defer {
            completionHandler()
            DispatchQueue.main.async { NSApplication.shared.terminate(nil) }
        }
        guard response.actionIdentifier == UNNotificationDefaultActionIdentifier,
              let state = response.notification.request.content.userInfo["dingStateDir"] as? String,
              state.hasPrefix("/"), state.utf8.count <= 4096, !state.contains("\0") else { return }
        var args = ["ui", "--state-dir", state]
        if let watch = response.notification.request.content.userInfo["dingWatchId"] as? String {
            guard watch.range(of: "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$", options: .regularExpression) != nil else { return }
            args += ["--watch", watch]
        }
        let process = Process()
        // Only the signed sibling binary may mint the fresh one-use browser link.
        // Notification payloads contain no URL, admin token or expiring session.
        process.executableURL = Bundle.main.bundleURL.deletingLastPathComponent().appendingPathComponent("ding")
        process.arguments = args
        process.standardInput = FileHandle.nullDevice
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        try? process.run()
    }

    enum Failure: Error { case invalidInput, permissionDenied }
}

@main
struct NotificationApplication {
    @MainActor static func main() {
        if CommandLine.arguments.dropFirst() == ["--credentials"] {
            exit(credentials())
        }
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.setActivationPolicy(.accessory)
        app.delegate = delegate
        app.run()
        withExtendedLifetime(delegate) {}
    }
}
