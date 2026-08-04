# ADBPureFlow Project Roadmap

This document outlines the planned future features, enhancements, and milestones for ADBPureFlow.

---

## +_= | Strategic Vision
Our goal is to make **ADBPureFlow** the absolute easiest, most performant, and secure open-source companion for Android developers, QA engineers, and reverse engineers. We focus on zero-configuration setup, cross-platform speed, and automated lifecycle management.

---

## +_= | Milestones & Target Features

### Phase 1: Stability & Security (Current - Q3 2026)
- [x] Complete cross-platform capabilities (Windows, Linux, macOS).
- [x] Integrate security checks (Zip/Tar Slip traversal preventions).
- [x] Set up rigorous automated multi-platform builds & tests in CI/CD.
- [x] Fully interactive web preview interface.

### Phase 2: Enhanced QA Tools (Q4 2026)
- [ ] **Multi-device simultaneous installations:** Ability to broadcast a single APK installation and automatic launch to *all* connected devices in parallel.
- [ ] **Advanced Logcat Viewer:** Embedded, live-filtering logcat output widget in the GUI to track application crashes immediately without opening a shell.
- [ ] **Screen Recording:** Build screen recording commands straight into the GUI toolbar (powered by Scrcpy recording flags or adb native recorders).

### Phase 3: Wireless ADB & Networking (Q1 2027)
- [ ] **Wireless Pairing Wizard:** Auto-detect and guide wireless pairing (Android 11+ QR/pairing codes) directly from the GUI.
- [ ] **Local Network Broadcasting:** Enable sharing connected device mirrors to a local web server (e.g. standardizing developer visual presentations).

### Phase 4: Plugin & Custom Actions (Q2 2027)
- [ ] **Pre/Post-Install Hooks:** Support running custom bash/batch scripts before or after installing an APK (e.g., seeding mockup sqlite databases, setting custom shared preferences).
- [ ] **Integration with App Stores:** Support drag-and-drop or one-click extraction of packages directly from online package repositories or APK mirrors.
