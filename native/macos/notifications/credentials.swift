import Foundation
import Security
import LocalAuthentication

struct CredentialRequest: Decodable {
    let operation: String
    let service: String
    let name: String
    let value: String?
}

func credentials() -> Int32 {
    do {
        let data = FileHandle.standardInput.readDataToEndOfFile()
        guard data.count <= 131072 else { return 1 }
        let request = try JSONDecoder().decode(CredentialRequest.self, from: data)
        guard request.service.hasPrefix("ing.ding.credentials."), request.service.count <= 128,
              !request.name.isEmpty, request.name.count <= 256 else { return 1 }
        let context = LAContext()
        context.interactionNotAllowed = true
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: request.service, kSecAttrAccount as String: request.name,
            kSecUseAuthenticationContext as String: context]
        switch request.operation {
        case "get":
            var lookup = query
            lookup[kSecReturnData as String] = true
            lookup[kSecMatchLimit as String] = kSecMatchLimitOne
            var result: CFTypeRef?
            guard SecItemCopyMatching(lookup as CFDictionary, &result) == errSecSuccess,
                  let value = result as? Data else { return 1 }
            FileHandle.standardOutput.write(value)
            return 0
        case "set":
            guard let value = request.value, !value.isEmpty, value.utf8.count <= 65536 else { return 1 }
            let attributes = [kSecValueData as String: Data(value.utf8)]
            let updated = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
            if updated == errSecSuccess { return 0 }
            guard updated == errSecItemNotFound else { return 1 }
            var newItem = query
            newItem[kSecValueData as String] = Data(value.utf8)
            newItem[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            return SecItemAdd(newItem as CFDictionary, nil) == errSecSuccess ? 0 : 1
        case "delete":
            let result = SecItemDelete(query as CFDictionary)
            return result == errSecSuccess || result == errSecItemNotFound ? 0 : 1
        default: return 1
        }
    } catch { return 1 }
}
