# Terminology Rules

> Audience: documentation contributors and maintainers.
> This page defines writing terms. It is not product user guidance.

## Canonical Terms

- **System Account (`User`)**: the account inside Memoh.
- **Platform Account (`ChannelIdentity`)**: the user's account on an external access platform, not a Memoh account.
- **Access Platform (`channel`)**: the external platform carrying inbound messages.
- **Account Linking (`bind`)**: linking a Platform Account to a System Account.
- **Bind Code**: one-time code used for account linking.
- **Bot**: resource and authorization boundary managed by a System Account.

## Preferred Wording

- Write **"platform account"** instead of "actor" in user-facing docs.
- Write **"access platform"** instead of "channel" when describing product behavior.
- Keep code aliases in parentheses on first mention:
  - `Platform Account (ChannelIdentity)`
  - `System Account (User)`
  - `Account Linking (bind)`

## Disallowed or Discouraged Terms

- Avoid plain **actor** in conceptual docs (except when quoting code symbols).
- Avoid ambiguous **platform user** phrasing (it does not distinguish system vs platform account).
- Avoid wording that implies Platform Account is created inside Memoh.

## Example Sentences

- Correct: "A platform account is the user's TG account, not a Memoh account."
- Correct: "Account linking binds a platform account to a system account."
- Incorrect: "Actor is a user in Memoh."

## Contributor Checklist

- Is every "account" term clearly scoped (system vs platform)?
- Is "channel" replaced by "access platform" in prose?
- Are code aliases kept only as parenthetical references?
