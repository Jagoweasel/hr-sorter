# HeadHunter (HH) Connection Process

This document describes the flow of adding and authorizing a HeadHunter integration in `hr-sorter`.

| Step | Function (File) | Log Prefix | What happens |
| :--- | :--- | :--- | :--- |
| **1. Create** | `handleCreateIntegration` (`handlers.go`) | `[HH]` | You click "+ Add HH". It saves the integration to the database with `pending_auth` status and immediately triggers the background worker. |
| **2. Worker** | `StartIntegration` (`manager.go`) | `[HH]` | Initializes a background context and a 5-minute ticker for the specific integration. |
| **3. Sync** | `Sync` (`manager.go`) | `[HH]` | Runs immediately and then every 5 minutes. If status is `pending_auth`, it logs a skip message and waits. |
| **4. Auth** | **(Manual Action)** | N/A | User clicks "Finish Auth", follows the HH login link. HH redirects to `hhandroid://?code=XYZ...`. User copies the code. |
| **5. Submit** | `handleSubmitHHCode` (`handlers.go`) | `[HH]` | User pastes the code into the modal. The backend exchanges it for an `access_token` and `refresh_token`, updating status to `active`. |
| **6. Fetch** | `Sync` (`manager.go`) | `[HH]` | The background worker's next tick (or manual trigger) sees the `active` status and begins downloading negotiations and messages. |

## Log Categories
- Use `-debug-hh` to see `[HH]` and `[SYNC]` logs.
- Backend now uses explicit `log.Printf` for critical lifecycle events to ensure visibility even if debug flags are misconfigured.
