# Core Concepts

This section defines the core account and access concepts used by Memoh.

## Concept Map

- **System Account (`User`)**: an internal account in Memoh.
- **Platform Account (`ChannelIdentity`)**: a user's account on an external access platform, not a Memoh account (for example, the user's Telegram (TG) account).
- **Bot**: an access and resource boundary managed by a system account.
- **Account Linking (`bind`)**: the process that links a platform account to a system account.

## Why This Matters

Memoh receives messages from external access platforms, but manages permissions and resources inside the system.
To keep these concerns clear, the model separates platform accounts from system accounts, while keeping bot access control as an independent concern.

Terminology note: "platform account" always means the user's account on that platform (such as TG), not an internal account created by this project.

## In This Chapter

- [Accounts and Linking](/concepts/identity-and-binding.md)
