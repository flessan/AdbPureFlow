# AdbPureFlow
[English](#english) | [Bahasa Indonesia](#bahasa-indonesia)

Lightweight Go utility to automate Android APK lifecycle: setup ADB, install, auto-detect package ID, and perform clean uninstalls.

---

## English

### +_= | Description
**AdbPureFlow** is a lightweight Go-based utility designed to automate the Android APK lifecycle. It handles everything from ADB environment setup to clean uninstallation, ensuring a "zero-residue" testing process.

### +_= | Key Features
*   **Zero Config**: Automatically downloads and sets up ADB platform-tools.
*   **Identity Verification**: Compares package lists to identify the actual App ID (Anti-Liar feature).
*   **Instant Launch**: Automatically starts the app after a successful installation.
*   **Guaranteed Cleanup**: Verifies that the app is completely removed from the device.

---

## Bahasa Indonesia

### +_= | Deskripsi
**AdbPureFlow** adalah alat ringan berbasis Go yang dirancang untuk mengotomatisasi siklus hidup APK Android. Program ini menangani segalanya mulai dari penyiapan lingkungan ADB hingga penghapusan instalasi secara bersih, memastikan proses pengujian tanpa sisa sampah.

### +_= | Fitur Utama
*   **Tanpa Konfigurasi**: Mengunduh dan menyiapkan ADB platform-tools secara otomatis.
*   **Verifikasi Identitas**: Membandingkan daftar paket untuk mengidentifikasi ID Aplikasi yang sebenarnya (fitur Anti-Liar).
*   **Peluncuran Instan**: Menjalankan aplikasi secara otomatis setelah berhasil dipasang.
*   **Pembersihan Terjamin**: Memverifikasi bahwa aplikasi telah benar-benar dihapus dari perangkat.

---

### +_= | How to Use / Cara Penggunaan

1.  **Clone & Run**:
    ```bash
    git clone https://github.com
    cd AdbPureFlow
    go run main.go
    ```
2.  **Workflow**:
    *   Drag your `.apk` file into the terminal.
    *   Watch it install and launch automatically.
    *   Confirm uninstallation to keep your device clean.

---

**Built with Go for a cleaner Android development workflow. +_=**
