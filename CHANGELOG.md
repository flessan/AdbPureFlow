# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [5.0.0] - 2026-08-04

### Added
- **Full Cross-Platform Support:** Refactored both CLI and GUI setups to automatically download, identify, and configure platform tools (ADB, Scrcpy) for macOS, Linux, and Windows, rather than forcing Windows-specific `.exe` tools.
- **Zip & Tar Slip Preventions:** Patched major security loopholes in file decompression helpers in both CLI and GUI backends, preventing directory traversal vulnerabilities.
- **Modernized Landing Page:** Completely redesigned the static website interface into an ultra-modern dark/light mode dashboard with a live browser-based simulator.
- **Robust Multi-Platform CI/CD Pipeline:** Fully configured a GitHub Actions pipeline compiling both Go Fyne GUI and Go CLI binaries, verifying format compliance, running test coverage, and publishing tagged releases automatically.
- **Detailed Unit Testing:** Wrote initial test suites with mocking mechanisms covering string package parsing and directory indexing.

### Changed
- Improved error feedback on ADB download interruptions, missing permissions, and non-existent APK file targets.
- Cleaned up obsolete platform tool dependencies and redundant legacy files.

---

## [4.0.0] - 2026-03-12

### Added
- Integrated basic support for launching screen mirroring using `scrcpy`.
- Developed interactive desktop panel utilizing the Fyne toolkit.

### Fixed
- Fixed critical directory lookup bugs for device selection.

---

## [1.0.0] - 2025-11-20

### Added
- Initial release of the CLI utility.
- Basic drag-and-drop mechanics for installing `.apk` packages over USB.
- Core automated package identity-verifications using before-and-after state snapshots.
