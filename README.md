# ADBPureFlow, 4devs,2devs.

[![Build & Test](https://github.com/flessan/adbpureflow/actions/workflows/ci.yml/badge.svg)](https://github.com/flessan/adbpureflow/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/flessan/adbpureflow?filename=GUI%2Fgo.mod)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/flessan/adbpureflow)](https://github.com/flessan/adbpureflow/releases)
[![License](https://img.shields.io/github/license/flessan/adbpureflow)](./LICENSE)
[![Security Policy](https://img.shields.io/badge/Security-Supported-brightgreen.svg)](./SECURITY.md)

An elegant, portable, cross-platform utility built in **Go** to completely automate the Android APK lifecycle: automatically download standard ADB binaries, run high-speed screen mirroring, detect newly installed package IDs without guesswork, and perform secure uninstalls.

---

## +_= | Highlights & Key Features

*   **Zero-Configuration Portable Engine:** Checks and automatically provisions platform tools (ADB) and screen mirrors (`scrcpy`) for Windows, macOS, and Linux out-of-the-box.
*   **High-Speed Mirroring:** Instantly launches screen mirroring powered by hardware-accelerated Scrcpy.
*   **Intelligent App-ID Identifier:** Grabs package listings before and after installation to accurately detect your new package ID (averting hidden names or spoofing).
*   **Auto-Launch Category:** Instantly opens the app post-installation on the device.
*   **Guaranteed Clean Cleanup:** Wipe the app on demand and confirm with absolute verification that no leftover data is left behind.
*   **Fully Secure Extractions:** Patched with Zip Slip and Tar Slip protections for safe component decompression.

---

## +_= | System Architecture

ADBPureFlow functions as a fast, high-performance mediator between your machine and Android's SDK binaries.

```
       +---------------------------------------------+
       |             ADBPureFlow Interface           |
       |             (Fyne GUI / CLI console)        |
       +-------+-----------------------------+-------+
               |                             |
               v (File Drop)                 v (Toolbar Action)
+---------------+--------------+     +--------+--------------+
|     Automated Installer      |     |    Scrcpy Mirroring   |
|                              |     |                       |
| 1. Audits existing packages  |     | 1. Query device port  |
| 2. Standard ADB install -r-d |     | 2. Run high-FPS stream|
| 3. Snapshot Diff -> App ID   |     | 3. Set custom title   |
| 4. Auto-Launch via Monkey    |     |                       |
+--------------+--------------+      +-------+---------------+
               |                             |
               +--------------+--------------+
                              |
                              v
                +-------------+-------------+
                |    Target Android Device  |
                +---------------------------+
```

---

## +_= | Installation & Zero-Config Setup

ADBPureFlow is shipped as a portable, single-command tool requiring **zero pre-existing configurations**.

### Method 1: Using Compiled Release Binaries (Recommended)
1. Head over to the [GitHub Releases](https://github.com/flessan/adbpureflow/releases) tab.
2. Download the binary matching your platform:
   - **Windows:** `adbpureflow-gui-windows-amd64.exe` / `adbpureflow-cli-windows-amd64.exe`
   - **macOS:** `adbpureflow-gui-darwin-amd64` / `adbpureflow-cli-darwin-amd64`
   - **Linux:** `adbpureflow-gui-linux-amd64` / `adbpureflow-cli-linux-amd64`
3. Run and execute! (On macOS/Linux, make sure to grant run permissions: `chmod +x adbpureflow-*`).

### Method 2: Building From Source

#### Prerequisites
- **Go**: Version 1.20 or newer.
- **GCC compiler** (only required to compile Fyne GUI dependencies):
  - **Linux:** Install OpenGL/X11 tools: `sudo apt-get install -y libgl1-mesa-dev libegl1-mesa-dev libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev`.
  - **macOS:** Included default in Xcode tools.
  - **Windows:** MinGW-w64.

#### Run the CLI Interface
```bash
cd CLI
go run main.go
```

#### Run the GUI Interface
```bash
cd GUI
go run main.go app.go
```

---

## +_= | Detailed Usage Instructions

### Using the GUI Dashboard
1. Connect your Android device via USB (or over local WiFi). Ensure **USB Debugging** is toggled on inside Developer Options.
2. Launch the GUI. The device selector will scan and display your phone model automatically.
3. **Install & Run:** Drag and drop any `.apk` file into the window (or click the File icon). The logs will live-update as it gets deployed, and the application will instantly start playing on your phone!
4. **Mirror Screen:** Click the Play icon (`MediaPlayIcon`) to start streaming your phone's screen in 60fps.
5. **Uninstall:** Click the Trash icon (`DeleteIcon`), type the package ID, and ADBPureFlow will securely purge the software.

### Using the CLI Console
1. Run the `adbpureflow-cli` executable.
2. Drag-and-drop or type the path to your `.apk` package and press Enter.
3. The console automatically coordinates package diffing and triggers an automatic launch.
4. It prompts you: `Hapus aplikasi sekarang? / Delete application now? (y/n)`. Typing `y` immediately sweeps the app, confirming deep deletion.

---

## +_= | Environment Configurations

ADBPureFlow is portable, but can also be fine-tuned using these environment variables or local structures:

| Parameter | Default | Description |
| --------- | ------- | ----------- |
| System PATH | `adb` | If you have ADB pre-installed in your environment, ADBPureFlow uses it instantly. |
| `scrcpy_core/` | Directory | Local component storage where automated binaries get safely unzipped. |

---

## +_= | Troubleshooting FAQ

#### Q1: No devices are showing up in the selector.
- **A:** Ensure your phone is connected and USB Debugging is turned on. Run `adb devices` in your command shell to verify your machine acknowledges the hardware. Try replugging your cable.

#### Q2: It fails to launch Screen Mirroring.
- **A:** Mirroring requires a device with USB Debugging enabled. Ensure your device screen is unlocked and not in a sleep state.

#### Q3: Compiling the GUI on Linux errors with missing packages.
- **A:** Make sure you installed all OpenGL/X11 header requirements using the command described in the [Developer Setup](#prerequisites) section.

---

## +_= | Security, Standards & Verification

- **Secure Archive Extractors:** Decompression routines strictly validate paths to prevent directory traversal exploits.
- **Validated Inputs:** Raw file parameters are explicitly checked for system existence before executing commands.
- **No Hardcoded Credentials:** Contains zero hardcoded tokens, hashes, or tracking telemetry.

---

## +_= | Project Contribution & Community

We are fully open-source and welcoming of active contributors! Please inspect our documentation to learn more about how we build features:
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Disclosures](./SECURITY.md)
- [Project Roadmap](./ROADMAP.md)

*Built with passion for a cleaner, modern Android development experience. Made portable and fast using Go.*
