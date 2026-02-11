# Accounts and Linking

## Account Model

Memoh treats platform accounts and system accounts as two different entities:

- **Platform Account (`ChannelIdentity`)** is the user's account on an external access platform (for example, a TG account), not a Memoh internal account.
- **System Account (`User`)** is an internal account in Memoh.

A platform account can exist before linking.  
`bind` is the mechanism that links these two account types.

## Access Platform and Bot

- **Access Platform (`channel`)** is where inbound messages come from.
- **Bot** is an authorization and resource boundary inside Memoh.

Bots are managed by system accounts, while inbound messages are produced by platform accounts.

## Why Linking Is Account-Scoped

Account linking exists to establish account ownership, not to grant bot resources directly:

- It links platform accounts and system accounts independent of any single bot.
- It avoids coupling account linking with member management semantics.
- It keeps bot authorization and account linking decoupled.

## Linking Flow (Current Consensus)

1. A user requests a bind code under their own system account.
2. The platform account sends the code from a supported access-platform conversation.
3. Memoh validates the code and links platform account to system account.
4. Bot membership and authorization are handled by their own flows.

## Bot Type Semantics

- **Public bot**: supports member-based collaboration.
- **Personal bot**: conceptually single-owner, and should not rely on member semantics.

> Note: The conceptual model is documented here as product semantics.  
> Runtime behavior may still be in transition as implementations are tightened.
