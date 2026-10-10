import AppKit
import Foundation
import UserNotifications
import Darwin

struct Message: Decodable {
    let id: String
    let title: String
    let body: String
    let requestPermission: Bool
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        Task { await deliver() }
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
