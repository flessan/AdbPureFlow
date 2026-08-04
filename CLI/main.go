package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var adbURLs = map[string]string{
	"windows": "https://dl.google.com/android/repository/platform-tools-latest-windows.zip",
	"darwin":  "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip",
	"linux":   "https://dl.google.com/android/repository/platform-tools-latest-linux.zip",
}

const engineDir = "adb_engine"

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("=================================================================")
	fmt.Println("               ADBPureFlow CLI Pro v5.0                          ")
	fmt.Println("         Automated Android APK Lifecycle Companion               ")
	fmt.Println("=================================================================")

	// 1. Setup ADB
	adbPath := setupADB()
	if adbPath == "" {
		fmt.Println("\n[!] Gagal menyiapkan ADB. Cek koneksi internet.")
		fmt.Println("[!] Failed to set up ADB. Check your internet connection or path.")
		tungguEnter(reader)
		return
	}

	fmt.Printf("\n[*] ADB Engine active: %s\n", adbPath)

	// 2. Input APK
	fmt.Print("\n[1] Tarik file APK ke sini & Tekan Enter:\n    Drag the APK file here & Press Enter: ")
	apkPathRaw, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("[!] Error reading input: %v\n", err)
		return
	}
	apkPath := strings.TrimSpace(strings.Trim(apkPathRaw, "\"\r\n'"))
	if apkPath == "" {
		fmt.Println("[!] Path kosong. Membalkan operasi.\n[!] Empty path. Cancelling operation.")
		tungguEnter(reader)
		return
	}

	// Verify APK file exists
	if _, err := os.Stat(apkPath); os.IsNotExist(err) {
		fmt.Printf("[!] File APK tidak ditemukan di path: %s\n[!] APK file not found at: %s\n", apkPath, apkPath)
		tungguEnter(reader)
		return
	}

	// 3. Scan Sebelum Install
	fmt.Println("\n[2] Memindai daftar aplikasi di HP...\n    Scanning list of applications on your device...")
	beforeList := getPackageList(adbPath)

	// 4. Install
	fmt.Println("\n[3] Memasang aplikasi ke HP...\n    Installing the application to your device...")
	installCmd := exec.Command(adbPath, "install", "-r", "-d", apkPath)
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		fmt.Printf("[!] Error saat instalasi: %v\n[!] Installation error: %v\n", err, err)
	}

	// 5. Scan Sesudah & Identifikasi
	fmt.Println("\n[4] Mencari ID aplikasi baru...\n    Searching for new application ID...")
	afterList := getPackageList(adbPath)
	packageName := findNewPackage(beforeList, afterList)

	if packageName == "" {
		fmt.Println("\n[!] Tidak ada ID baru terdeteksi. Aplikasi mungkin sudah ada atau gagal terpasang.")
		fmt.Println("[!] No new ID found. The application may already exist or failed to install.")
		fmt.Print("Ketik ID manual (contoh: com.example.app) atau Tekan Enter untuk batal: \nType the manual ID (example: com.example.app) or press Enter to cancel: ")
		manual, _ := reader.ReadString('\n')
		packageName = strings.TrimSpace(manual)
		if packageName == "" {
			return
		}
	}

	fmt.Printf("\nSUCCESS! Terdeteksi / Detected: %s\n", packageName)

	// 6. Auto-Launch
	fmt.Println("\n[5] Menunggu sistem... membuka aplikasi otomatis...")
	fmt.Println("    Waiting for system... launching the application automatically...")
	time.Sleep(1500 * time.Millisecond) // Wait 1.5 seconds for device to be ready
	
	launchCmd := exec.Command(adbPath, "shell", "monkey", "-p", packageName, "-c", "android.intent.category.LAUNCHER", "1")
	if err := launchCmd.Run(); err != nil {
		fmt.Printf("[!] Gagal meluncurkan aplikasi otomatis: %v\n[!] Failed to auto-launch application: %v\n", err, err)
	}

	// 7. Menu Konfirmasi & Verifikasi Uninstall
	fmt.Println("\n=================================================================")
	fmt.Println("                  APLIKASI AKTIF DI PERANGKAT                    ")
	fmt.Println("=================================================================")
	fmt.Print("Hapus aplikasi sekarang? (y/n) [Default: n]: \nDelete application now? (y/n) [Default: n]: ")

	pilihan, _ := reader.ReadString('\n')
	pilihanClean := strings.ToLower(strings.TrimSpace(pilihan))
	if pilihanClean == "y" || pilihanClean == "yes" {
		fmt.Printf("\n-----> Menghapus %s...\n", packageName)
		uninstallCmd := exec.Command(adbPath, "uninstall", packageName)
		uninstallCmd.Stdout = os.Stdout
		uninstallCmd.Stderr = os.Stderr
		uninstallCmd.Run()

		// Verifikasi Akhir
		if isPackageStillExists(adbPath, packageName) {
			fmt.Println("[!] ERROR: Aplikasi gagal dihapus! Coba hapus manual via HP.")
			fmt.Println("[!] ERROR: Application failed to delete! Try deleting it manually from your device.")
		} else {
			fmt.Println("[OK] Konfirmasi: Aplikasi telah benar-benar terhapus.")
			fmt.Println("[OK] Confirmation: The application has been completely deleted.")
		}
	} else {
		fmt.Println("\n-----> Aplikasi dibiarkan terpasang.\n-----> Application left installed.")
	}

	tungguEnter(reader)
}

// --- FUNGSI HELPERS ---

func getPackageList(adbPath string) map[string]bool {
	list := make(map[string]bool)
	out, err := exec.Command(adbPath, "shell", "pm", "list", "packages", "-3").Output()
	if err != nil {
		// Try without -3 fallback
		out, err = exec.Command(adbPath, "shell", "pm", "list", "packages").Output()
		if err != nil {
			return list
		}
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(strings.Replace(line, "package:", "", 1))
		if name != "" {
			list[name] = true
		}
	}
	return list
}

func findNewPackage(before, after map[string]bool) string {
	for pkg := range after {
		if !before[pkg] {
			return pkg
		}
	}
	return ""
}

func isPackageStillExists(adbPath, pkgName string) bool {
	out, err := exec.Command(adbPath, "shell", "pm", "list", "packages", pkgName).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "package:"+pkgName)
}

func setupADB() string {
	base, err := os.Getwd()
	if err != nil {
		base = "."
	}
	adbName := "adb"
	if runtime.GOOS == "windows" {
		adbName = "adb.exe"
	}
	
	adbFile := filepath.Join(base, engineDir, "platform-tools", adbName)
	if _, err := os.Stat(adbFile); err == nil {
		return adbFile
	}

	// Fallback to checking system PATH
	if path, err := exec.LookPath(adbName); err == nil {
		return path
	}

	url, ok := adbURLs[runtime.GOOS]
	if !ok {
		fmt.Printf("[!] Sistem operasi %s tidak didukung untuk pengunduhan otomatis.\n", runtime.GOOS)
		fmt.Printf("[!] OS %s not supported for auto-download.\n", runtime.GOOS)
		return ""
	}

	fmt.Printf("[*] Engine ADB tidak ditemukan. Mendownload untuk %s...\n", runtime.GOOS)
	fmt.Println("[*] ADB Engine not found. Downloading...")

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("[!] Download gagal: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[!] HTTP status error: %s\n", resp.Status)
		return ""
	}

	zipName := "adb.zip"
	f, err := os.Create(zipName)
	if err != nil {
		fmt.Printf("[!] Gagal membuat file zip temp: %v\n", err)
		return ""
	}
	
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		fmt.Printf("[!] Gagal mendownload content: %v\n", err)
		os.Remove(zipName)
		return ""
	}

	fmt.Println("[*] Mengekstrak platform-tools...")
	err = unzip(zipName, engineDir)
	os.Remove(zipName)
	if err != nil {
		fmt.Printf("[!] Gagal mengekstrak zip: %v\n", err)
		return ""
	}

	// Set permissions for macOS/Linux
	if runtime.GOOS != "windows" {
		_ = os.Chmod(adbFile, 0755)
	}

	if _, err := os.Stat(adbFile); err == nil {
		return adbFile
	}
	return ""
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		
		// Prevent Zip Slip vulnerability
		fpathAbs, err := filepath.Abs(fpath)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(fpathAbs, destAbs+string(filepath.Separator)) && fpathAbs != destAbs {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return err
			}
			continue
		}
		
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}
		
		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func tungguEnter(r *bufio.Reader) {
	fmt.Println("\nTekan Enter untuk keluar...\nPress Enter to exit...")
	_, _ = r.ReadString('\n')
}
