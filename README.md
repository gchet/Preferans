# Preferans
[Русский](README.ru.md)

A Windows and Android preference card game for three or four players, using the Leningrad convention. Play offline with local bots or join a room on your own signaling/TURN infrastructure.

## Features
- Shared WebUI, Go game engine, Windows WebView2 and Android WebView shells.
- Rooms, automatic connections and reconnection, voice chat, configurable table agreements.
- Local bots; optional OpenAI-compatible model integrations and decision analysis.
- Saved unfinished host games, room history and player totals.
- Ukrainian (default), Russian and English interfaces; three editable SVG decks.

Only the Leningrad ruleset is implemented. Sochi, Classic and Rostov are reserved for future development.

## Build
Install PowerShell 7, Go (version in go.mod), Node.js, and Zig for Windows. Android builds also require JDK 17, Android SDK and NDK. Gradle is supplied through the pinned Wrapper.

Run from the project root:
~~~powershell
pwsh ./build.ps1 -Target All -Public -AI Off
~~~
Outputs: dist/Preferans.exe, dist/Preferans.apk and dist/version.json. Windows requires WebView2 Runtime; Android requires Android 8+ and ARM64. APKs currently use the existing debug signing scheme.

Public builds contain no personal server defaults. Configure your own room-service URL and TURN credentials in the app, or select **Bots only (offline)**. This project does not provide a public server. Developer-specific defaults belong in ignored project.local.json; see [build instructions](docs/BUILD.md).

## Documentation
- [Build and configuration](docs/BUILD.md)
- [Architecture, rooms and recovery](docs/ARCHITECTURE.md)
- [Own server and administration](docs/SERVER.md)
- [Rules and scoring](docs/RULES.md)
- [Bots and model integrations](docs/BOTS.md)
- [Interface and card assets](docs/UI.md)
- [Localization](docs/LOCALIZATION.md)
- [Testing](docs/TESTING.md)
- [Releases](docs/RELEASES.md)
- [Roadmap](docs/ROADMAP.md)

Detailed technical documentation is currently in Russian. See [SECURITY.md](SECURITY.md) before deploying a server. The app does not hide dealt cards from the host and is intended for trusted groups.

## License
Application code: [MIT](LICENSE). Bundled card artwork and dependencies retain their own licenses: [third-party notices](THIRD_PARTY_NOTICES.md).
