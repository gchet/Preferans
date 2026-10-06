# Security

The project is intended for games between trusted participants. The host knows
the full deal; this is not a cheat-resistant competitive platform.
The project author's servers are private and are not offered as a public service.

For online use, deploy your own signaling/TURN infrastructure. Prefer HTTPS for
signaling, protect server access and change the initial administrator password
before exposing the administrator site. The initial password is admin; replacement
is currently offered rather than enforced. Administrator login requires TLS or
server-loopback access (for example an SSH tunnel).

Room membership is based on random device secrets, not verified user accounts.
An empty room can be renamed/deleted without an administrator account.
The legacy singleton endpoint is unauthenticated. Protect the server independently
if it is reachable by untrusted clients. WebRTC encrypts the game/voice channels;
this does not encrypt HTTP signaling or local diagnostic files.

Android Wi-Fi updates use a user-supplied server and do not currently verify a
download hash in the app. Android still checks the package signature. Use a trusted
LAN/server. Distributed APKs currently use the developer's existing debug signing key.
Never publish that keystore, device identity, reconnect secrets, API keys or log files.

AI diagnostic logs may include cards visible to the bot and its explanation.
Share only reviewed/redacted logs. API keys are protected locally with Windows
DPAPI or Android Keystore, but backups and export files still require care.

Report vulnerabilities privately using GitHub's private vulnerability reporting
when enabled for the public repository. Do not put credentials in public issues.
CI performs secret and dependency checks; these are not a full security audit.
